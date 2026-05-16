package main

import (
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
)

var (
	isProbing      atomic.Bool
	latestReport   *DashboardReport
	latestReportMu sync.RWMutex
)

func registerAPIRoutes(mux *http.ServeMux, adminToken string) {
	adminHandler := adminAuthMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/admin/config", "/api/config":
			handleConfig(w, r)
		case "/api/admin/providers", "/api/providers":
			handleProviders(w, r)
		case "/api/admin/probe", "/api/probe":
			handleProbe(w, r)
		case "/api/admin/status":
			handleStatus(w, r)
		case "/api/admin/history":
			handleHistory(w, r)
		default:
			http.NotFound(w, r)
		}
	}), adminToken)

	mux.HandleFunc("/api/setup-status", handleSetupStatus)
	mux.HandleFunc("/api/setup", handleSetup)
	mux.HandleFunc("/api/login", handleLogin)
	mux.HandleFunc("/api/logout", handleLogout)

	for _, path := range []string{
		"/api/admin/config",
		"/api/admin/providers",
		"/api/admin/probe",
		"/api/admin/status",
		"/api/admin/history",
		"/api/config",
		"/api/providers",
		"/api/probe",
	} {
		mux.Handle(path, adminHandler)
	}

	mux.HandleFunc("/api/status", handleStatus)
	mux.HandleFunc("/api/history", handleHistory)
}

func handleSetupStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	json.NewEncoder(w).Encode(map[string]bool{"setup_required": !IsSetupComplete()})
}

func handleSetup(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if IsSetupComplete() {
		http.Error(w, "setup already completed", http.StatusConflict)
		return
	}
	var req struct {
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := SaveAdminPassword(req.Password); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	issueSession(w)
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func handleLogin(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !IsSetupComplete() {
		writeAuthError(w, "setup required")
		return
	}

	ip := clientIP(r)
	if err := checkLoginRateLimit(ip); err != nil {
		w.WriteHeader(http.StatusTooManyRequests)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	var req struct {
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	valid := VerifyAdminPassword(req.Password)
	recordLoginAttempt(ip, valid)

	if !valid {
		writeAuthError(w, "invalid password")
		return
	}

	issueSession(w)
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func handleLogout(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if c, err := r.Cookie("amm_session"); err == nil {
		ClearSession(c.Value)
	}
	http.SetCookie(w, &http.Cookie{Name: "amm_session", Value: "", Path: "/", MaxAge: -1, HttpOnly: true, SameSite: http.SameSiteLaxMode})
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func issueSession(w http.ResponseWriter) {
	token, err := CreateSession()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: "amm_session", Value: token, Path: "/", MaxAge: int(sessionTTL.Seconds()), HttpOnly: true, SameSite: http.SameSiteLaxMode})
}

func handleConfig(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	switch r.Method {
	case http.MethodGet:
		cfg := GetConfig()
		json.NewEncoder(w).Encode(maskConfig(cfg))

	case http.MethodPut:
		var cfg AppConfig
		if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := SaveConfig(&cfg); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		json.NewEncoder(w).Encode(maskConfig(GetConfig()))

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func handleProviders(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	switch r.Method {
	case http.MethodGet:
		cfg := GetConfig()
		providers := make([]Provider, len(cfg.Providers))
		for i, p := range cfg.Providers {
			providers[i] = maskProvider(p)
		}
		json.NewEncoder(w).Encode(providers)

	case http.MethodPost:
		var p Provider
		if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if p.ID == "" {
			p.ID = uuid.New().String()[:8]
		}
		if err := AddProvider(p); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(maskProvider(p))

	case http.MethodPut:
		id := r.URL.Query().Get("id")
		if id == "" {
			http.Error(w, "missing id parameter", http.StatusBadRequest)
			return
		}
		var p Provider
		if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := UpdateProvider(id, p); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})

	case http.MethodDelete:
		id := r.URL.Query().Get("id")
		if id == "" {
			http.Error(w, "missing id parameter", http.StatusBadRequest)
			return
		}
		if err := RemoveProvider(id); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func handleProbe(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if !isProbing.CompareAndSwap(false, true) {
		json.NewEncoder(w).Encode(map[string]string{
			"status":  "already_probing",
			"message": "探测正在进行中，请稍后再试",
		})
		return
	}

	defer isProbing.Store(false)

	report := runProbe()

	latestReportMu.Lock()
	latestReport = report
	latestReportMu.Unlock()

	json.NewEncoder(w).Encode(report)
}

func handleStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	latestReportMu.RLock()
	report := latestReport
	latestReportMu.RUnlock()

	if report == nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":  "no_data",
			"message": "尚未执行过探测，请先点击「开始探测」",
		})
		return
	}

	json.NewEncoder(w).Encode(report)
}

func handleHistory(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	key := r.URL.Query().Get("key")
	if key != "" {
		// Parse key as "providerID::model"
		parts := strings.SplitN(key, "::", 2)
		if len(parts) != 2 {
			json.NewEncoder(w).Encode([]HistoryRecord{})
			return
		}
		records := LoadHistoryRecords(parts[0], parts[1], 0)
		if records == nil {
			records = []HistoryRecord{}
		}
		json.NewEncoder(w).Encode(records)
		return
	}

	h, err := LoadAllHistory()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	json.NewEncoder(w).Encode(h)
}

func buildReportFromHistory(cfg *AppConfig) *DashboardReport {
	startTime := time.Now()

	var results []ProbeResult
	for _, p := range cfg.Providers {
		for _, m := range p.Models {
			records := LoadHistoryRecords(p.ID, m, 1)
			if len(records) == 0 {
				continue
			}
			latest := records[len(records)-1]
			results = append(results, ProbeResult{
				ProviderID:   p.ID,
				ProviderName: p.Name,
				ProviderType: p.Type,
				ProviderIcon: p.Icon,
				Model:        m,
				Status:       latest.Status,
				LatencyMs:    latest.LatencyMs,
				CheckedAt:    latest.CheckedAt,
			})
		}
	}

	return buildReport(results, cfg, startTime)
}

func runProbe() *DashboardReport {
	cfg := GetConfig()
	startTime := time.Now()

	type probeJob struct {
		provider Provider
		model    string
	}

	var jobs []probeJob
	for _, p := range cfg.Providers {
		for _, m := range p.Models {
			jobs = append(jobs, probeJob{provider: p, model: m})
		}
	}

	if len(jobs) == 0 {
		return &DashboardReport{
			Title:               cfg.Title,
			GeneratedAt:         startTime.Format("2006-01-02 15:04:05"),
			ElapsedMs:           int(time.Since(startTime).Milliseconds()),
			OverallStatus:       "NO_MODELS",
			OverallClass:        "error",
			StatsWindowDays:     cfg.StatsWindowDays,
			HistorySize:         cfg.HistorySize,
			Concurrency:         cfg.Concurrency,
			ProviderConcurrency: cfg.ProviderConcurrency,
		}
	}

	globalSem := make(chan struct{}, cfg.Concurrency)
	providerSem := make(map[string]chan struct{})
	for _, p := range cfg.Providers {
		if _, ok := providerSem[p.ID]; !ok {
			providerSem[p.ID] = make(chan struct{}, cfg.ProviderConcurrency)
		}
	}

	var resultsMu sync.Mutex
	var results []ProbeResult

	var wg sync.WaitGroup
	for _, job := range jobs {
		wg.Add(1)
		go func(j probeJob) {
			defer wg.Done()
			pSem := providerSem[j.provider.ID]

			pSem <- struct{}{}
			defer func() { <-pSem }()

			globalSem <- struct{}{}
			defer func() { <-globalSem }()

			result := probeModel(j.provider, j.model, cfg)

			resultsMu.Lock()
			results = append(results, result)
			resultsMu.Unlock()

			AppendHistoryRecord(j.provider.ID, j.model, HistoryRecord{
				Status:    result.Status,
				LatencyMs: result.LatencyMs,
				CheckedAt: result.CheckedAt,
			}, cfg.HistorySize)
		}(job)
	}
	wg.Wait()

	return buildReport(results, cfg, startTime)
}

func buildReport(results []ProbeResult, cfg *AppConfig, startTime time.Time) *DashboardReport {
	grouped := make(map[string]*ProviderStatus)
	var providerOrder []string

	allHistory, err := LoadAllHistory()
	if err != nil {
		allHistory = make(map[string][]HistoryRecord)
	}

	for _, r := range results {
		if _, ok := grouped[r.ProviderID]; !ok {
			grouped[r.ProviderID] = &ProviderStatus{
				ProviderID:   r.ProviderID,
				ProviderName: r.ProviderName,
				ProviderType: r.ProviderType,
				ProviderIcon: r.ProviderIcon,
				Status:       "ok",
				StatusLabel:  "正常",
			}
			providerOrder = append(providerOrder, r.ProviderID)
		}
		p := grouped[r.ProviderID]

		key := fmt.Sprintf("%s::%s", r.ProviderID, r.Model)
		records := allHistory[key]
		if records == nil {
			records = []HistoryRecord{}
		}

		ms := ModelStatus{
			ProviderID:     r.ProviderID,
			ProviderName:   r.ProviderName,
			ProviderType:   r.ProviderType,
			ProviderIcon:   r.ProviderIcon,
			Model:          r.Model,
			Status:         r.Status,
			StatusLabel:    statusLabel(r.Status),
			LatencyMs:      r.LatencyMs,
			AvgLatency24h:  avgLatency24h(records),
			Availability:   availability(records),
			WeeklySuccess:  weeklySuccessText(records),
			ShowCurveChart: cfg.ShowCurveChart,
		}

		if cfg.ShowErrorDetail && r.Error != "" {
			ms.Error = r.Error
		}

		ms.History = historyBars(records, cfg.HistorySize)
		ms.LatencyCurve = latencyCurve(records, cfg.HistorySize)
		ms.TimeLabels = extractTimeLabels(records, cfg.HistorySize)

		p.Results = append(p.Results, ms)
		switch r.Status {
		case "ok":
			p.OkCount++
		case "slow":
			p.SlowCount++
		case "error":
			p.ErrorCount++
		}
	}

	var providers []ProviderStatus
	okCount, slowCount, errorCount := 0, 0, 0
	for _, id := range providerOrder {
		p := grouped[id]
		p.ModelCount = len(p.Results)
		if p.ErrorCount > 0 {
			p.Status = "error"
			p.StatusLabel = "异常"
		} else if p.SlowCount > 0 {
			p.Status = "slow"
			p.StatusLabel = "较慢"
		}
		okCount += p.OkCount
		slowCount += p.SlowCount
		errorCount += p.ErrorCount
		providers = append(providers, *p)
	}

	overallStatus := "OPERATIONAL"
	overallClass := "ok"
	if errorCount > 0 {
		overallStatus = "DEGRADED"
		overallClass = "error"
	}

	return &DashboardReport{
		Title:               cfg.Title,
		GeneratedAt:         startTime.Format("2006-01-02 15:04:05"),
		ElapsedMs:           int(time.Since(startTime).Milliseconds()),
		Total:               len(results),
		OkCount:             okCount,
		SlowCount:           slowCount,
		ErrorCount:          errorCount,
		ProviderCount:       len(providers),
		Providers:           providers,
		OverallStatus:       overallStatus,
		OverallClass:        overallClass,
		StatsWindowDays:     cfg.StatsWindowDays,
		HistorySize:         cfg.HistorySize,
		Concurrency:         cfg.Concurrency,
		ProviderConcurrency: cfg.ProviderConcurrency,
	}
}

func statusLabel(status string) string {
	switch status {
	case "ok":
		return "正常"
	case "slow":
		return "较慢"
	case "error":
		return "错误"
	default:
		return "未知"
	}
}

func authMiddleware(next http.Handler, token string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if token == "" || r.Method == http.MethodOptions {
			next.ServeHTTP(w, r)
			return
		}

		if !validToken(r, token) {
			writeAuthError(w, "unauthorized")
			return
		}

		next.ServeHTTP(w, r)
	})
}

func adminAuthMiddleware(next http.Handler, token string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodOptions {
			next.ServeHTTP(w, r)
			return
		}
		if !IsSetupComplete() {
			writeAuthError(w, "setup required")
			return
		}
		if validSession(r) || validToken(r, token) {
			next.ServeHTTP(w, r)
			return
		}
		writeAuthError(w, "unauthorized")
	})
}

func validSession(r *http.Request) bool {
	c, err := r.Cookie("amm_session")
	return err == nil && ValidateSession(c.Value)
}

func validToken(r *http.Request, token string) bool {
	if token == "" {
		return false
	}
	provided := r.Header.Get("X-API-Key")
	if auth := r.Header.Get("Authorization"); strings.HasPrefix(auth, "Bearer ") {
		provided = strings.TrimSpace(strings.TrimPrefix(auth, "Bearer "))
	}
	return subtle.ConstantTimeCompare([]byte(provided), []byte(token)) == 1
}

func writeAuthError(w http.ResponseWriter, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	json.NewEncoder(w).Encode(map[string]string{"error": message})
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			origin := r.Header.Get("Origin")
			if origin != "" {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Vary", "Origin")
			} else {
				w.Header().Set("Access-Control-Allow-Origin", "*")
			}
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-API-Key")
			if r.Method == "OPTIONS" {
				w.WriteHeader(http.StatusOK)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}
