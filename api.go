package main

import (
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"log"
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

	sseClients   map[chan *DashboardReport]struct{}
	sseClientsMu sync.Mutex
)

func init() {
	sseClients = make(map[chan *DashboardReport]struct{})
}

func broadcastReport(report *DashboardReport) {
	sseClientsMu.Lock()
	defer sseClientsMu.Unlock()
	for ch := range sseClients {
		select {
		case ch <- report:
		default:
		}
	}
}

func handleSSE(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	fmt.Fprintf(w, "retry: 3000\n\n")
	flusher.Flush()

	ch := make(chan *DashboardReport, 4)
	sseClientsMu.Lock()
	sseClients[ch] = struct{}{}
	sseClientsMu.Unlock()

	defer func() {
		sseClientsMu.Lock()
		delete(sseClients, ch)
		sseClientsMu.Unlock()
	}()

	latestReportMu.RLock()
	if latestReport != nil {
		data, _ := json.Marshal(latestReport)
		fmt.Fprintf(w, "data: %s\n\n", data)
		flusher.Flush()
	}
	latestReportMu.RUnlock()

	ctx := r.Context()
	heartbeat := time.NewTicker(30 * time.Second)
	defer heartbeat.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-heartbeat.C:
			fmt.Fprintf(w, ": keepalive\n\n")
			flusher.Flush()
		case report, ok := <-ch:
			if !ok {
				return
			}
			data, err := json.Marshal(report)
			if err != nil {
				continue
			}
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		}
	}
}

func registerAPIRoutes(mux *http.ServeMux, adminToken string) {
	adminHandler := maxBodySizeMiddleware(adminAuthMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
		case "/api/admin/alerts/rules", "/api/alerts/rules":
			handleAlertRules(w, r)
		case "/api/admin/alerts/events", "/api/alerts/events":
			handleAlertEvents(w, r)
		case "/api/admin/export/csv":
			handleExportCSV(w, r)
		case "/api/admin/export/json":
			handleExportJSON(w, r)
		case "/api/admin/providers/fetch-models":
			handleFetchModels(w, r)
		default:
			http.NotFound(w, r)
		}
	}), adminToken))

	mux.HandleFunc("/api/setup-status", handleSetupStatus)
	mux.HandleFunc("/api/setup", maxBodySizeMiddleware(http.HandlerFunc(handleSetup)).ServeHTTP)
	mux.HandleFunc("/api/login", maxBodySizeMiddleware(http.HandlerFunc(handleLogin)).ServeHTTP)
	mux.HandleFunc("/api/logout", handleLogout)

	adminPaths := []string{
		"/api/admin/config",
		"/api/admin/providers",
		"/api/admin/probe",
		"/api/admin/status",
		"/api/admin/history",
		"/api/admin/alerts/rules",
		"/api/admin/alerts/events",
		"/api/admin/export/csv",
		"/api/admin/export/json",
		"/api/admin/providers/fetch-models",
		"/api/config",
		"/api/providers",
		"/api/probe",
		"/api/alerts/rules",
		"/api/alerts/events",
	}
	for _, path := range adminPaths {
		mux.Handle(path, adminHandler)
	}

	mux.HandleFunc("/api/status", handleStatus)
	mux.HandleFunc("/api/history", handleHistory)
	mux.HandleFunc("/api/events", handleSSE)
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
			writeJSONError(w, http.StatusBadRequest, err.Error())
			return
		}

		currentPort := GetConfig().Port
		if err := SaveConfig(&cfg); err != nil {
			writeJSONError(w, http.StatusInternalServerError, err.Error())
			return
		}

		resp := map[string]interface{}{
			"config": maskConfig(GetConfig()),
		}
		if cfg.Port != currentPort {
			resp["restart_required"] = true
			resp["warning"] = "服务端口已变更，需要重启服务后才能生效"
		}
		json.NewEncoder(w).Encode(resp)

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
			p.ID = generateProviderID()
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

func handleFetchModels(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		ProviderID string `json:"provider_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	cfg := GetConfig()
	var target *Provider
	for i, p := range cfg.Providers {
		if p.ID == req.ProviderID || p.Name == req.ProviderID {
			target = &cfg.Providers[i]
			break
		}
	}
	if target == nil {
		writeJSONError(w, http.StatusNotFound, "provider not found")
		return
	}

	models, err := FetchModels(*target)
	if err != nil {
		writeJSONError(w, http.StatusBadGateway, fmt.Sprintf("fetch models failed: %v", err))
		return
	}

	updated := *target
	updated.Models = models
	if err := UpdateProvider(target.ID, updated); err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":        "ok",
		"provider_id":   target.ID,
		"provider_name": target.Name,
		"models":        models,
		"count":         len(models),
	})
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

	broadcastReport(report)

	reportCopy := copyReport(report)
	go evaluateAlertRules(reportCopy)

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

func handleAlertRules(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	switch r.Method {
	case http.MethodGet:
		json.NewEncoder(w).Encode(GetAlertRules())

	case http.MethodPost:
		var rule AlertRule
		if err := json.NewDecoder(r.Body).Decode(&rule); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if rule.Name == "" || rule.MetricType == "" || rule.Condition == "" {
			http.Error(w, "name, metric_type, condition are required", http.StatusBadRequest)
			return
		}
		if err := SaveAlertRule(rule); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})

	case http.MethodDelete:
		id := r.URL.Query().Get("id")
		if id == "" {
			http.Error(w, "missing id", http.StatusBadRequest)
			return
		}
		if err := DeleteAlertRule(id); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func handleAlertEvents(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	events, err := GetAlertEvents(50)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if events == nil {
		events = []AlertEvent{}
	}
	json.NewEncoder(w).Encode(events)
}

func handleExportCSV(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition", "attachment; filename=history.csv")

	rows, err := db.Query("SELECT provider_id, model, status, latency_ms, checked_at FROM history ORDER BY checked_at DESC LIMIT 10000")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	w.Write([]byte("provider_id,model,status,latency_ms,checked_at\n"))
	for rows.Next() {
		var providerID, model, status, checkedAt string
		var latencyMs int
		if err := rows.Scan(&providerID, &model, &status, &latencyMs, &checkedAt); err != nil {
			continue
		}
		line := fmt.Sprintf("%s,%s,%s,%d,%s\n", providerID, model, status, latencyMs, checkedAt)
		w.Write([]byte(line))
	}
}

func handleExportJSON(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", "attachment; filename=history.json")

	latestReportMu.RLock()
	report := latestReport
	latestReportMu.RUnlock()

	if report != nil {
		json.NewEncoder(w).Encode(report)
	} else {
		json.NewEncoder(w).Encode(map[string]string{"message": "no data available"})
	}
}

func writeJSONError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func copyReport(r *DashboardReport) *DashboardReport {
	if r == nil {
		return nil
	}
	data, err := json.Marshal(r)
	if err != nil {
		return nil
	}
	var copy DashboardReport
	if err := json.Unmarshal(data, &copy); err != nil {
		return nil
	}
	return &copy
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
			globalSem <- struct{}{}

			var result ProbeResult
			func() {
				defer func() {
					<-globalSem
					<-pSem
					if r := recover(); r != nil {
						log.Printf("warn: probe panic for %s/%s: %v", j.provider.ID, j.model, r)
						result = ProbeResult{
							ProviderID:   j.provider.ID,
							ProviderName: j.provider.Name,
							Model:        j.model,
							Status:       "error",
							Error:        fmt.Sprintf("internal panic: %v", r),
							CheckedAt:    time.Now().Format("2006-01-02 15:04:05"),
						}
					}
				}()
				result = probeModel(j.provider, j.model, cfg)
			}()

			resultsMu.Lock()
			results = append(results, result)
			resultsMu.Unlock()

			if err := AppendHistoryRecord(j.provider.ID, j.model, HistoryRecord{
				Status:    result.Status,
				LatencyMs: result.LatencyMs,
				CheckedAt: result.CheckedAt,
			}, cfg.HistorySize); err != nil {
				log.Printf("warn: append history for %s/%s: %v", j.provider.ID, j.model, err)
			}
		}(job)
	}
	wg.Wait()

	return buildReport(results, cfg, startTime)
}

func buildReport(results []ProbeResult, cfg *AppConfig, startTime time.Time) *DashboardReport {
	grouped := make(map[string]*ProviderStatus)
	var providerOrder []string

	allHistory, err := LoadAllHistorySince(startTime.AddDate(0, 0, -cfg.StatsWindowDays).Format("2006-01-02 15:04:05"))
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
			ShowCurveChart: cfg.ShowCurveChart,
		}

		if len(records) > 0 {
			cs := computeStats(records, cfg.StatsWindowDays)
			ms.AvgLatency24h = cs.avgLatency24h
			ms.Availability = cs.availability
			ms.WeeklySuccess = cs.weeklySuccess
		} else {
			ms.AvgLatency24h = "N/A"
			ms.Availability = "0.00%"
			ms.WeeklySuccess = "0/0"
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
				w.Header().Set("Access-Control-Allow-Credentials", "true")
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-API-Key")
				if r.Method == "OPTIONS" {
					w.WriteHeader(http.StatusOK)
					return
				}
			}
		}
		next.ServeHTTP(w, r)
	})
}

func securityHeadersMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=(), interest-cohort=()")

		if !strings.HasPrefix(r.URL.Path, "/api/") {
			w.Header().Set("Content-Security-Policy", strings.Join([]string{
				"default-src 'self'",
				"script-src 'self'",
				"style-src 'self' 'unsafe-inline' https://fonts.googleapis.com",
				"font-src 'self' https://fonts.gstatic.com",
				"img-src 'self' data: https://cdn.jsdelivr.net",
				"connect-src 'self'",
				"frame-ancestors 'none'",
				"form-action 'self'",
			}, "; "))
		}
		next.ServeHTTP(w, r)
	})
}

const maxRequestBodyBytes = 1_048_576

func maxBodySizeMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Body != nil {
			r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodyBytes)
		}
		next.ServeHTTP(w, r)
	})
}

func generateProviderID() string {
	for attempts := 0; attempts < 20; attempts++ {
		candidate := uuid.New().String()[:12]
		cfg := GetConfig()
		exists := false
		for _, existing := range cfg.Providers {
			if existing.ID == candidate {
				exists = true
				break
			}
		}
		if !exists {
			return candidate
		}
	}
	return uuid.New().String()
}
