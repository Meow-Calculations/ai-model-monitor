# AI Model Monitor

一个独立的网页版 AI 大模型状态监控程序，用于实时监控 AI 大模型的连通性和响应状态。

## 功能特性

- **模型连通性探测** — 向每个模型发送探针请求，检测响应状态和延迟
- **多 API 协议支持** — OpenAI 兼容 API（OpenAI / DeepSeek / Ollama / Groq 等）+ Anthropic 原生 Messages API
- **SSE 实时推送** — 探测完成后立即通过 Server-Sent Events 推送到前端，替代 30 秒轮询
- **智能重试** — 对超时、429 限流、5xx 服务端错误自动重试（最多 2 次，指数退避）
- **并发控制** — 全局并发 + 单 Provider 并发，防止 API 过载
- **历史记录** — SQLite 持久化，支持可用性、24h 平均延迟、统计窗口成功率
- **延迟曲线** — SVG 平滑贝塞尔曲线展示延迟趋势
- **自动检测** — 可配置间隔自动定期探测
- **Provider 管理** — Web UI 动态增删改 Provider 和模型
- **告警通知** — 可配置规则（延迟/错误/可用性），触发时通过 Webhook 推送
- **数据导出** — CSV 和 JSON 格式一键导出历史探测数据
- **面板分离** — 用户面板（公开只读）与管理面板（需认证）独立路由，互不干扰
- **双列瀑布流** — Provider 卡片 CSS Grid 双列布局，不同高度顶部对齐
- **首批初始化密码** — 首次运行必须在网页设置管理密码后才能使用管理功能
- **管理接口鉴权** — `/api/admin/*` 强制登录态鉴权，兼容环境变量 Token
- **API Key 加密存储** — 使用 AES-256-GCM 对 API Key 加密后写入 SQLite
- **登录速率限制** — 60 秒内连续 5 次登录失败临时锁定 1 分钟，基于 IP 的暴力破解防护
- **主题切换** — 支持日间、夜间、跟随系统和按时间自动切换，全局导航栏一键切换
- **毛玻璃主题** — 网格背景 + 辐射渐变 + 毛玻璃效果 + 深浅色双主题
- **代码分割** — React.lazy + Suspense 路由级懒加载，首屏 JS 体积 256 kB

## 技术栈

| 层级 | 技术 |
|------|------|
| 后端 | Go 1.26+ |
| 数据库 | SQLite (modernc.org/sqlite，纯 Go 无需 CGO) |
| 前端 | React 19 + Vite 6 + react-router-dom v7 |
| 样式 | CSS 自定义属性 + 毛玻璃效果 + 深浅色双主题 |

## 项目结构

```text
ai-model-monitor/
├── main.go                      # 入口：HTTP 服务、静态文件托管、自动检测循环
├── api.go                       # REST API 路由、CORS、鉴权、SSE、导出处理器
├── auth.go                      # 管理密码哈希、会话创建与校验、登录速率限制
├── crypto.go                    # AES-256-GCM API Key 加密解密
├── models.go                    # 数据模型定义（含 AlertRule / AlertEvent）
├── config.go                    # SQLite 配置读写、Provider CRUD、配置校验、密钥初始化
├── alert.go                     # 告警规则 CRUD、评估引擎、Webhook 通知
├── probe.go                     # 模型探测逻辑（策略模式，OpenAI 兼容 / Anthropic）
├── history.go                   # 历史记录持久化、统计计算、复合索引
├── migrate.go                   # YAML → SQLite 迁移辅助
├── config.yaml                  # 默认配置（可选，启动时自动迁移到 SQLite）
├── *_{test,api}.go              # 测试文件
├── go.mod / go.sum
└── frontend/                    # React 前端
    ├── index.html
    ├── vite.config.js           # Vite 配置 + API 代理
    ├── package.json
    └── src/
        ├── main.jsx              # 入口
        ├── App.jsx               # 路由、初始化、登录、导航栏、主题管理
        ├── api.js                # API 客户端、会话/Token 鉴权、图标映射
        ├── theme.js              # 主题模式管理（system/time/light/dark）
        ├── styles/
        │   ├── theme.css         # CSS 自定义属性（深浅色双主题）
        │   ├── layout.css        # 布局：面板、卡片、表单
        │   └── responsive.css    # 响应式适配
        └── components/
            ├── Dashboard.jsx     # 状态看板：Provider 卡片、模型指标、SVG 曲线
            ├── AdminPanel.jsx    # 管理面板：看板标签 + 配置标签 + 告警标签
            ├── UserPanel.jsx     # 用户面板：只读状态看板（公开访问）
            ├── ConfigPanel.jsx   # 配置管理：Provider CRUD、通用设置、主题、导出
            └── AlertPanel.jsx    # 告警管理：规则配置 + 事件查看（懒加载）
```

**构建/运行生成**（不提交到 Git）：

- `frontend/node_modules/` — npm 依赖
- `static/` — Vite 构建的前端静态资源，由 Go 后端托管
- `data/` — SQLite 运行时数据目录
- `ai-model-monitor` / `ai-model-monitor.exe` — Go 编译产物

## 快速开始

### 方式 1：直接运行

```bash
go build -o ai-model-monitor.exe .
./ai-model-monitor.exe
```

浏览器打开 `http://localhost:8080`。首次运行会进入初始化页面，设置管理密码后即可登录管理。

### 方式 2：开发模式

```bash
# 终端 1：启动后端
go run .

# 终端 2：启动前端（自动代理 API 到 8080）
cd frontend
npm ci
npm run dev
```

浏览器打开 `http://localhost:5173`

## 构建部署

```bash
# 1. 构建前端
cd frontend && npm ci && npm run build && cd ..

# 2. 编译后端
go build -o ai-model-monitor.exe .

# 3. 部署 — 只需以下文件
#   ai-model-monitor.exe
#   static/          (前端资源)
```

## SSE 实时数据流

生产模式下（Go 编译后直接运行），前端通过 **EventSource** 订阅 `/api/events` 端点：

```
探测器 → runProbe() → broadcastReport() → 所有 SSE 连接 → 前端自动更新
              ↓
       evaluateAlertRules() → 触发告警 → Webhook 推送
```

- 每次探测完成自动广播到所有连接的浏览器
- 生产模式无需轮询，数据延迟 < 1s
- 开发模式（`npm run dev`）自动使用 30s 轮询，避免 Vite 代理的二次重试

## 告警系统

可在管理面板「告警管理」标签页中配置告警规则。

### 支持的指标

| 指标类型 | 说明 | 示例 |
|---------|------|------|
| `latency` | 延迟 (ms) | `latency > 8000` |
| `error_count` | 错误次数 (0/1) | `error_count > 0` |
| `availability` | 可用性百分比 | `availability < 99.0` |

### 通知渠道

- **Webhook URL** — 支持钉钉、飞书、Slack 格式自动识别
- 每次告警触发时自动发送通知
- 告警恢复时发送恢复通知

### 评估流程

1. 每次探测完成 → `evaluateAlertRules(report)`
2. 遍历所有启用规则 → 匹配 Provider/模型 → 计算指标
3. 首次触发 → 创建 `alert_events` 记录 + 发送 Webhook
4. 恢复时 → 更新事件状态为 `resolved`

## 数据导出

管理面板「配置管理 → 通用设置」底部提供一键导出：

| 格式 | 端点 | 内容 |
|------|------|------|
| CSV | `/api/admin/export/csv` | 最近 10000 条历史记录 |
| JSON | `/api/admin/export/json` | 最新完整探测报告 |

## API Key 加密存储

API Key 使用 **AES-256-GCM** 自动加密后写入 SQLite，读取时自动解密。

### 加密密钥来源（优先级由高到低）

1. **环境变量 `AMM_ENCRYPTION_KEY`**：64 位十六进制字符串（32 字节），推荐使用
2. **管理密码哈希**：从管理员密码 PBKDF2-SHA256 哈希派生
3. **无加密（兼容模式）**：仅首次启动过渡期

### 生成密钥

```bash
# Linux/macOS
openssl rand -hex 32

# Windows (PowerShell)
$bytes = [byte[]]::new(32); (New-Object Security.Cryptography.RNGCryptoServiceProvider).GetBytes($bytes); ($bytes | ForEach-Object { $_.ToString("x2") }) -join ''
```

```bash
set AMM_ENCRYPTION_KEY=<64位十六进制密钥>
./ai-model-monitor.exe
```

> **注意**：已加密的 API Key 无法在更换密钥后解密。如需更换密钥，请先清空 Provider 列表重新配置。

## API 接口

### 公开接口

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/setup-status` | 查询是否需要首次初始化 |
| POST | `/api/setup` | 首次设置管理密码 |
| POST | `/api/login` | 管理密码登录（含速率限制） |
| POST | `/api/logout` | 退出登录 |
| GET | `/api/status` | 获取最新探测报告 |
| GET | `/api/history?key=provider::model` | 查询历史记录 |
| GET | `/api/events` | SSE 实时推送端点 |

### 管理接口（需鉴权）

| 方法 | 路径 | 说明 |
|------|------|------|
| GET/PUT | `/api/admin/config` | 读写全局配置 |
| GET/POST/PUT/DELETE | `/api/admin/providers` | Provider CRUD |
| POST | `/api/admin/probe` | 手动触发探测 |
| GET | `/api/admin/status` | 获取最新报告 |
| GET | `/api/admin/history` | 查询全部历史 |
| GET/POST/DELETE | `/api/admin/alerts/rules` | 告警规则 CRUD |
| GET | `/api/admin/alerts/events` | 获取告警事件 |
| GET | `/api/admin/export/csv` | 导出 CSV |
| GET | `/api/admin/export/json` | 导出 JSON |

**鉴权方式**：HttpOnly Session Cookie（登录后自动携带）或 `Authorization: Bearer <token>` / `X-API-Key: <token>`（需设置 `AI_MODEL_MONITOR_TOKEN` 环境变量）。

## 面板路由

| 路由 | 面板 | 认证 |
|------|------|------|
| `/` | 用户面板（公开） | 无需登录 |
| `/admin` | 管理面板（看板 + 配置 + 告警） | 需登录 |
| `/admin/login` | 管理员登录 | 未认证时自动跳转 |

## 配置项

| 配置项 | 默认值 | 说明 |
|--------|--------|------|
| `title` | 模型连通性 | 看板标题 |
| `timeout_seconds` | 30 | 探测超时（秒） |
| `slow_threshold_ms` | 8000 | 慢响应阈值（毫秒） |
| `concurrency` | 3 | 全局最大并发 |
| `provider_concurrency` | 1 | 单 Provider 并发 |
| `history_size` | 30 | 历史条显示长度 |
| `stats_window_days` | 7 | 统计窗口天数 |
| `probe_prompt` | 只回复 OK 两个字母 | 探测用户提示词 |
| `probe_system_prompt` | 你是一个模型连通性探针… | 探测系统提示词 |
| `show_curve_chart` | true | 显示延迟曲线 |
| `show_error_detail` | true | 显示错误详情 |
| `auto_check_interval_seconds` | 0 | 自动检测间隔（0=关闭） |
| `port` | 8080 | 服务端口 |

## 支持的 Provider 类型

openai · anthropic · deepseek · google · ollama · groq · openrouter · nvidia · azure · xai · dashscope · volcengine · minimax · kimi · modelscope

## API Key 处理

- 读取列表时非空 API Key 返回 `********`（脱敏）
- 编辑时保持 `********`：不修改原 Key
- 清空 API Key 字段：删除原 Key
- 输入新值：替换为新 Key

## 主题模式

| 模式 | 说明 |
|------|------|
| 跟随系统 | 根据 `prefers-color-scheme` 自动切换 |
| 按时间 | 18:00~07:00 夜间，其余日间 |
| 日间 | 固定浅色 |
| 夜间 | 固定深色 |

偏好保存在 `localStorage`，仅影响当前浏览器。

## 数据存储

数据文件：`data/monitor.db`（SQLite）

### 表结构

| 表 | 说明 |
|----|------|
| `config` | 全局配置键值对、管理员密码哈希 |
| `providers` | Provider 信息（API Key 加密） |
| `history` | 探测历史记录（含复合索引 `idx_history_lookup`） |
| `sessions` | 管理登录会话 |
| `alert_rules` | 告警规则 |
| `alert_events` | 告警事件历史 |

首次启动时，如存在旧的 `config.yaml` 会自动迁移到 SQLite。

## 测试验证

```bash
# 后端测试 + 静态检查
go vet ./...
go test ./... -count=1

# 前端构建验证
npm --prefix frontend run build

# 编译
go build -o ai-model-monitor.exe .
```

## License

MIT
