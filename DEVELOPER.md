# Developer Guide

Architecture overview, code conventions, and extension guide for AI Model Monitor.

## Architecture Overview

```
┌────────────────────────────────────────────────────────────┐
│                     Browser (React SPA)                    │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌──────────┐  │
│  │UserPanel │  │AdminPanel│  │Dashboard │  │AlertPanel│  │
│  │  (public)│  │ (auth)   │  │(shared)  │  │(lazy)    │  │
│  └────┬─────┘  └────┬─────┘  └────┬─────┘  └────┬─────┘  │
│       │              │              │              │       │
│       └──────────────┴──────┬──────┴──────────────┘       │
│                             │                              │
│                    ┌────────┴────────┐                    │
│                    │    api.js       │                    │
│                    │  (fetch layer)  │                    │
│                    └────────┬────────┘                    │
└─────────────────────────────┼──────────────────────────────┘
                              │ HTTP / SSE
┌─────────────────────────────┼──────────────────────────────┐
│                     Go Backend                             │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌──────────┐  │
│  │  auth.go │  │ crypto.go│  │  probe.go│  │ alert.go │  │
│  │(sessions)│  │ (AES-256)│  │ (probe)  │  │(eval+web)│  │
│  └────┬─────┘  └────┬─────┘  └────┬─────┘  └────┬─────┘  │
│       └──────────────┴──────┬──────┴──────────────┘       │
│                             │                              │
│                    ┌────────┴────────┐                    │
│                    │    api.go       │                    │
│                    │ (router + SSE)  │                    │
│                    └────────┬────────┘                    │
│                             │                              │
│                    ┌────────┴────────┐                    │
│                    │    SQLite       │                    │
│                    │  (modernc.org)  │                    │
│                    └─────────────────┘                    │
└────────────────────────────────────────────────────────────┘
```

### Data Flow: Probe → Report → Broadcast → Alert

```
time.Ticker / POST /api/probe
         │
         ▼
     runProbe()
         │
         ├──► semaphore (concurrency control)
         │       │
         │       ├──► probeProvider() → OpenAI / Anthropic strategy
         │       │       │
         │       │       ├── success → save history, append result
         │       │       └── error  → retry (max 2, exponential backoff)
         │       │
         │       └──► probe complete
         │
         ▼
  DashboardReport (JSON)
         │
         ├──► latestReportMu ← latestReport (in-memory cache)
         │
         ├──► broadcastReport(report) → SSE clients
         │       │
         │       └──► each ch <- report → EventSource → frontend
         │
         └──► evaluateAlertRules(report)
                 │
                 ├── match rules → compare thresholds
                 ├── firing? → create alert_events row + Webhook POST
                 └── recovered? → update status = 'resolved'
```

## Backend Code Guide

### File Responsibilities

| File | Responsibility | Key Functions |
|------|---------------|---------------|
| `main.go` | Entry point, HTTP server, static file serving, auto-check loop | `autoCheckLoop()`, `buildReportFromHistory()` |
| `api.go` | Route registration, handlers, SSE, export | `registerAPIRoutes()`, `handleSSE()`, `handleProbe()`, `handleExportCSV()` |
| `auth.go` | Password hashing (PBKDF2), session management, rate limiting | `checkPassword()`, `createSession()`, `validateSession()` |
| `crypto.go` | AES-256-GCM key derivation, encrypt/decrypt | `encryptAPIKey()`, `decryptAPIKey()` |
| `config.go` | SQLite DB init, config CRUD, Provider CRUD, encryption key setup | `InitDB()`, `LoadConfig()`, `SaveConfig()`, `AddProvider()` |
| `models.go` | All data structures | `Provider`, `ProbeResult`, `DashboardReport`, `AlertRule`, `AlertEvent` |
| `probe.go` | Probe execution, retry logic, provider strategies | `runProbe()`, `probeProvider()`, `doProbeOpenAI()`, `doProbeAnthropic()` |
| `history.go` | History persistence, statistics, composite index | `SaveHistoryRecord()`, `LoadAllHistory()`, `computeStats()` |
| `alert.go` | Alert rules CRUD, evaluation engine, webhook dispatch | `evaluateAlertRules()`, `SaveAlertRule()`, `sendAlertNotification()` |

### Routing Pattern

Routes are registered in `registerAPIRoutes()` with a layered middleware approach:

```go
// Admin routes: body size limit + auth + handler
adminHandler := maxBodySizeMiddleware(adminAuthMiddleware(http.HandlerFunc(func(w, r) {
    switch r.URL.Path {
    case "/api/admin/config":
        handleConfig(w, r)
    // ...
    }
}), adminToken))

// Public routes: direct handler
mux.HandleFunc("/api/status", handleStatus)
mux.HandleFunc("/api/events", handleSSE)

// Admin path aliases registered separately
for _, path := range adminPaths {
    mux.Handle(path, adminHandler)
}
```

### SSE (Server-Sent Events)

[sse.go](file:///f:/project/llm_test/ai-model-monitor/api.go#L45-L92):

1. Clients connect to `GET /api/events`
2. Handler registers a `chan *DashboardReport` into `sseClients` map
3. On disconnect (`ctx.Done()`), handler removes itself
4. `broadcastReport()` iterates all channels and sends non-blocking
5. Initial latest report is sent immediately on connect

```go
var sseClients   map[chan *DashboardReport]struct{}
var sseClientsMu sync.Mutex

func broadcastReport(report *DashboardReport) {
    sseClientsMu.RLock()
    defer sseClientsMu.RUnlock()
    for ch := range sseClients {
        select {
        case ch <- report:
        default: // drop if client is slow
        }
    }
}
```

### Alert Evaluation Engine

[alert.go](file:///f:/project/llm_test/ai-model-monitor/alert.go#L134-L178):

- Called from `handleProbe()` and `autoCheckLoop()` via `evaluateAlertRules(report)`
- Loads all enabled rules, iterates providers/models
- Tracks firing state in `firingAlerts` map (key: `ruleID::providerID::model`)
- First trigger → `createAlertEvent()` + `sendAlertNotification()`
- Recovery → `resolveAlertEvent()` updates DB row

### Encryption Module

[crypto.go](file:///f:/project/llm_test/ai-model-monitor/crypto.go):

- AES-256-GCM with random 12-byte nonce per encryption
- Key derivation priority:
  1. `AMM_ENCRYPTION_KEY` env var (64 hex chars → 32 bytes)
  2. PBKDF2-SHA256 of admin password hash
  3. Fallback: no encryption (first-run transition)
- Encrypted output format: `base64(nonce + ciphertext)`

### Database Migrations

[config.go](file:///f:/project/llm_test/ai-model-monitor/config.go) `InitDB()`:

- Idempotent `CREATE TABLE IF NOT EXISTS` statements
- Index creation: `CREATE INDEX IF NOT EXISTS`
- YAML → SQLite migration in `migrate.go` (one-time on first startup)

## Frontend Code Guide

### File Responsibilities

| File | Responsibility |
|------|---------------|
| `main.jsx` | React DOM root, BrowserRouter wrapper |
| `App.jsx` | Route definitions, auth state, theme state, admin login flow, navigation bar |
| `api.js` | `request()` / `adminRequest()` wrappers, auth token management, Provider icon maps, export URL helper |
| `theme.js` | `resolveTheme()`, `getStoredThemeMode()`, `setStoredThemeMode()`, time-range detection |
| `Dashboard.jsx` | Report rendering: Provider cards, model status table, latency SVG curve, history chart |
| `AdminPanel.jsx` | Admin layout, tab navigation (dashboard/config/alert), probe trigger, export buttons |
| `UserPanel.jsx` | Public layout, SSE connection with DEV fallback to polling |
| `ConfigPanel.jsx` | Settings forms: theme, general settings, Provider CRUD, export download |
| `AlertPanel.jsx` | Alert rules CRUD form, alert events timeline |

### Lazy Loading Strategy

[App.jsx](file:///f:/project/llm_test/ai-model-monitor/frontend/src/App.jsx):

```jsx
const AdminPanel = lazy(() => import('./components/AdminPanel'))

// Wrapped in Suspense at route level:
<Suspense fallback={...}>
  <Routes>
    <Route path="/admin" element={<AdminPanel />} />
  </Routes>
</Suspense>
```

Nested lazy loading in [AdminPanel.jsx](file:///f:/project/llm_test/ai-model-monitor/frontend/src/components/AdminPanel.jsx):

```jsx
const ConfigPanel = lazy(() => import('./ConfigPanel'))
const AlertPanel = lazy(() => import('./AlertPanel'))
```

Build output shows separate chunks:

| Chunk | Size | Loaded |
|-------|------|--------|
| `index-*.js` | 256 kB | First paint |
| `AdminPanel-*.js` | 7 kB | On `/admin` navigation |
| `ConfigPanel-*.js` | 10 kB | On clicking "配置管理" tab |
| `AlertPanel-*.js` | 7 kB | On clicking "告警管理" tab |

### API Client Layer

[api.js](file:///f:/project/llm_test/ai-model-monitor/frontend/src/api.js):

```jsx
// Public API — no auth required
function request(path, options = {}, base = API_BASE) { ... }

// Admin API — automatically attaches auth header
function adminRequest(path, options = {}) {
  return request(path, options, ADMIN_API_BASE)
}

// Auth token is read from localStorage and set as Bearer header
// Fallback to HttpOnly cookie (set by login endpoint)
```

### Theme System

[theme.js](file:///f:/project/llm_test/ai-model-monitor/frontend/src/theme.js):

```
resolveTheme(mode, timeRange)
  ├── "system" → window.matchMedia('prefers-color-scheme: dark')
  ├── "time"   → current hour in [nightStart, nightEnd) ? 'dark' : 'light'
  ├── "light"  → 'light'
  └── "dark"   → 'dark'
```

Theme changes dispatch a `CustomEvent('themechange')` that all components listen for.

## How to Add a New Provider Type

### Backend

1. Add the type string to `PROVIDER_TYPES` in [ConfigPanel.jsx](file:///f:/project/llm_test/ai-model-monitor/frontend/src/components/ConfigPanel.jsx#L5-L8)

2. If the provider uses **OpenAI-compatible API**, no backend change is needed — just set `api_endpoint` in the UI

3. If the provider uses a **custom protocol** (like Anthropic), add a new probe strategy in [probe.go](file:///f:/project/llm_test/ai-model-monitor/probe.go):

```go
func doProbeCustom(ctx context.Context, p Provider, model string, prompt, systemPrompt string) ProbeResult {
    // Implement your custom API call here
}
```

4. Register it in `probeProvider()` by type:

```go
switch p.Type {
case "anthropic":
    return doProbeAnthropic(ctx, p, model, prompt, systemPrompt)
case "custom":
    return doProbeCustom(ctx, p, model, prompt, systemPrompt)
default:
    return doProbeOpenAI(ctx, p, model, prompt, systemPrompt)
}
```

### Frontend

1. Add icon URL and color to [api.js](file:///f:/project/llm_test/ai-model-monitor/frontend/src/api.js) in `PROVIDER_ICONS` and `PROVIDER_COLORS`:

```js
const PROVIDER_ICONS = {
  // ...
  myprovider: 'https://cdn.example.com/icon.svg',
}

const PROVIDER_COLORS = {
  // ...
  myprovider: '#ff6600',
}
```

## How to Add a New Database Table

1. Define model struct in [models.go](file:///f:/project/llm_test/ai-model-monitor/models.go)
2. Add `CREATE TABLE IF NOT EXISTS` in the relevant `init*()` function (or create a new one)
3. Register the init call in `InitDB()` in [config.go](file:///f:/project/llm_test/ai-model-monitor/config.go)
4. Add API handlers in [api.go](file:///f:/project/llm_test/ai-model-monitor/api.go)
5. Add frontend API methods in [api.js](file:///f:/project/llm_test/ai-model-monitor/frontend/src/api.js)
6. Create/update frontend component

## Testing

### Backend Tests

```bash
# Run all tests with race detector
go test ./... -race -count=1

# Run specific test
go test -run TestConfig -v

# Static analysis
go vet ./...
```

Test files: [config_test.go](file:///f:/project/llm_test/ai-model-monitor/config_test.go), [api_test.go](file:///f:/project/llm_test/ai-model-monitor/api_test.go)

### Frontend Tests

```bash
# Build verification (no test runner configured)
npm --prefix frontend run build
```

### Manual Verification Checklist

See README.md → 测试验证 section for the full manual checklist.

## Environment Variables

| Variable | Required | Description |
|----------|----------|-------------|
| `AMM_ENCRYPTION_KEY` | No | 64-char hex AES-256 key for API Key encryption |
| `AI_MODEL_MONITOR_TOKEN` | No | Static bearer token for API authentication |
| `PORT` | No | Override server port (default 8080) |

## Production Deployment

```bash
# Build
cd frontend && npm ci && npm run build && cd ..
go build -ldflags="-s -w" -o ai-model-monitor .

# Run
#   ./ai-model-monitor
#   or
#   AMM_ENCRYPTION_KEY=<key> AI_MODEL_MONITOR_TOKEN=<token> ./ai-model-monitor
```

The Go binary embeds no frontend assets — `static/` directory must be present alongside the binary.

## SQLite Performance Notes

- `history` table has a composite covering index: `idx_history_lookup(provider_id, model, checked_at, status, latency_ms)`
- `LoadAllHistorySince()` uses time-window filtering with this index
- `alert_events` has index on `(rule_id, status)` for firing-event lookups
- No WAL mode is configured (default rollback journal is sufficient for this workload)
