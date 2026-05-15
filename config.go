package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	_ "modernc.org/sqlite"
)

const maskedAPIKey = "********"

var (
	configMu  sync.RWMutex
	appConfig *AppConfig
	db        *sql.DB
)

func DefaultConfig() *AppConfig {
	return &AppConfig{
		Title:               "模型连通性",
		Providers:           []Provider{},
		TimeoutSeconds:      30.0,
		SlowThresholdMs:     8000,
		Concurrency:         3,
		ProviderConcurrency: 1,
		ProbePrompt:         "只回复 OK 两个字母。",
		ProbeSystemPrompt:   "你是一个模型连通性探针。请只回复 OK，不要解释。",
		HistorySize:         30,
		StatsWindowDays:     7,
		ShowCurveChart:      true,
		ShowErrorDetail:     true,
		AutoCheckInterval:   0,
		Port:                8080,
	}
}

func maskAPIKey(key string) string {
	if key == "" {
		return ""
	}
	return maskedAPIKey
}

func isMaskedAPIKey(key string) bool {
	return key == maskedAPIKey
}

func maskProvider(p Provider) Provider {
	p.APIKey = maskAPIKey(p.APIKey)
	return p
}

func maskConfig(cfg *AppConfig) *AppConfig {
	masked := *cfg
	masked.Providers = make([]Provider, len(cfg.Providers))
	for i, p := range cfg.Providers {
		masked.Providers[i] = maskProvider(p)
	}
	return &masked
}

func NormalizeConfig(cfg *AppConfig) {
	defaults := DefaultConfig()
	if cfg.Title == "" {
		cfg.Title = defaults.Title
	}
	if cfg.TimeoutSeconds < 1 {
		cfg.TimeoutSeconds = 1
	}
	if cfg.SlowThresholdMs < 1 {
		cfg.SlowThresholdMs = 1
	}
	if cfg.Concurrency < 1 {
		cfg.Concurrency = 1
	}
	if cfg.ProviderConcurrency < 1 {
		cfg.ProviderConcurrency = 1
	}
	if cfg.HistorySize < 1 {
		cfg.HistorySize = 1
	}
	if cfg.StatsWindowDays < 1 {
		cfg.StatsWindowDays = 1
	}
	if cfg.AutoCheckInterval < 0 {
		cfg.AutoCheckInterval = 0
	}
	if cfg.Port < 1 || cfg.Port > 65535 {
		cfg.Port = defaults.Port
	}
	if cfg.ProbePrompt == "" {
		cfg.ProbePrompt = defaults.ProbePrompt
	}
	if cfg.ProbeSystemPrompt == "" {
		cfg.ProbeSystemPrompt = defaults.ProbeSystemPrompt
	}
	if cfg.Providers == nil {
		cfg.Providers = []Provider{}
	}
}

func InitDB(dataDir string) error {
	dbPath := filepath.Join(dataDir, "monitor.db")
	dsn := dbPath + "?_journal_mode=WAL&_busy_timeout=5000"
	var err error
	db, err = sql.Open("sqlite", dsn)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}

	// Enable WAL mode and set pragmas for performance
	pragmas := []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA synchronous=NORMAL",
		"PRAGMA cache_size=-64000",
		"PRAGMA foreign_keys=ON",
	}
	for _, p := range pragmas {
		if _, err := db.Exec(p); err != nil {
			return fmt.Errorf("set pragma %s: %w", p, err)
		}
	}

	// Create tables
	schema := `
	CREATE TABLE IF NOT EXISTS config (
		key   TEXT PRIMARY KEY,
		value TEXT NOT NULL
	);
	CREATE TABLE IF NOT EXISTS providers (
		id          TEXT PRIMARY KEY,
		name        TEXT NOT NULL DEFAULT '',
		type        TEXT NOT NULL DEFAULT 'openai',
		api_endpoint TEXT NOT NULL DEFAULT '',
		api_key     TEXT NOT NULL DEFAULT '',
		models      TEXT NOT NULL DEFAULT '[]',
		icon        TEXT NOT NULL DEFAULT '',
		sort_order  INTEGER NOT NULL DEFAULT 0
	);
	CREATE TABLE IF NOT EXISTS history (
		id         INTEGER PRIMARY KEY AUTOINCREMENT,
		provider_id TEXT NOT NULL,
		model      TEXT NOT NULL,
		status     TEXT NOT NULL,
		latency_ms INTEGER NOT NULL DEFAULT 0,
		checked_at TEXT NOT NULL,
		UNIQUE(provider_id, model, checked_at)
	);
	CREATE INDEX IF NOT EXISTS idx_history_key ON history(provider_id, model);
	CREATE INDEX IF NOT EXISTS idx_history_checked ON history(checked_at);
	`
	if _, err := db.Exec(schema); err != nil {
		return fmt.Errorf("create schema: %w", err)
	}

	return nil
}

func CloseDB() {
	if db != nil {
		db.Close()
	}
}

func LoadConfig() (*AppConfig, error) {
	configMu.RLock()
	defer configMu.RUnlock()

	cfg := DefaultConfig()

	// Load config values
	rows, err := db.Query("SELECT key, value FROM config")
	if err != nil {
		return nil, fmt.Errorf("query config: %w", err)
	}
	defer rows.Close()

	kv := make(map[string]string)
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			continue
		}
		kv[k] = v
	}

	if v, ok := kv["title"]; ok {
		cfg.Title = v
	}
	if v, ok := kv["timeout_seconds"]; ok {
		fmt.Sscanf(v, "%f", &cfg.TimeoutSeconds)
	}
	if v, ok := kv["slow_threshold_ms"]; ok {
		fmt.Sscanf(v, "%d", &cfg.SlowThresholdMs)
	}
	if v, ok := kv["concurrency"]; ok {
		fmt.Sscanf(v, "%d", &cfg.Concurrency)
	}
	if v, ok := kv["provider_concurrency"]; ok {
		fmt.Sscanf(v, "%d", &cfg.ProviderConcurrency)
	}
	if v, ok := kv["probe_prompt"]; ok {
		cfg.ProbePrompt = v
	}
	if v, ok := kv["probe_system_prompt"]; ok {
		cfg.ProbeSystemPrompt = v
	}
	if v, ok := kv["history_size"]; ok {
		fmt.Sscanf(v, "%d", &cfg.HistorySize)
	}
	if v, ok := kv["stats_window_days"]; ok {
		fmt.Sscanf(v, "%d", &cfg.StatsWindowDays)
	}
	if v, ok := kv["show_curve_chart"]; ok {
		cfg.ShowCurveChart = v == "true"
	}
	if v, ok := kv["show_error_detail"]; ok {
		cfg.ShowErrorDetail = v == "true"
	}
	if v, ok := kv["auto_check_interval_seconds"]; ok {
		fmt.Sscanf(v, "%d", &cfg.AutoCheckInterval)
	}
	if v, ok := kv["port"]; ok {
		fmt.Sscanf(v, "%d", &cfg.Port)
	}

	// Load providers
	provRows, err := db.Query("SELECT id, name, type, api_endpoint, api_key, models, icon FROM providers ORDER BY sort_order, id")
	if err != nil {
		return nil, fmt.Errorf("query providers: %w", err)
	}
	defer provRows.Close()

	for provRows.Next() {
		var p Provider
		var modelsJSON string
		if err := provRows.Scan(&p.ID, &p.Name, &p.Type, &p.APIEndpoint, &p.APIKey, &modelsJSON, &p.Icon); err != nil {
			continue
		}
		json.Unmarshal([]byte(modelsJSON), &p.Models)
		cfg.Providers = append(cfg.Providers, p)
	}

	NormalizeConfig(cfg)
	appConfig = cfg
	return cfg, nil
}

func SaveConfig(cfg *AppConfig) error {
	configMu.Lock()
	defer configMu.Unlock()
	NormalizeConfig(cfg)

	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	// Upsert config values
	kv := map[string]string{
		"title":                     cfg.Title,
		"timeout_seconds":           fmt.Sprintf("%g", cfg.TimeoutSeconds),
		"slow_threshold_ms":         fmt.Sprintf("%d", cfg.SlowThresholdMs),
		"concurrency":               fmt.Sprintf("%d", cfg.Concurrency),
		"provider_concurrency":      fmt.Sprintf("%d", cfg.ProviderConcurrency),
		"probe_prompt":              cfg.ProbePrompt,
		"probe_system_prompt":       cfg.ProbeSystemPrompt,
		"history_size":              fmt.Sprintf("%d", cfg.HistorySize),
		"stats_window_days":         fmt.Sprintf("%d", cfg.StatsWindowDays),
		"show_curve_chart":          fmt.Sprintf("%v", cfg.ShowCurveChart),
		"show_error_detail":         fmt.Sprintf("%v", cfg.ShowErrorDetail),
		"auto_check_interval_seconds": fmt.Sprintf("%d", cfg.AutoCheckInterval),
		"port":                      fmt.Sprintf("%d", cfg.Port),
	}
	for k, v := range kv {
		_, err := tx.Exec("INSERT OR REPLACE INTO config(key, value) VALUES(?, ?)", k, v)
		if err != nil {
			return fmt.Errorf("upsert config %s: %w", k, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return err
	}
	appConfig = cfg
	return nil
}

func GetConfig() *AppConfig {
	configMu.RLock()
	defer configMu.RUnlock()
	if appConfig == nil {
		return DefaultConfig()
	}
	return appConfig
}

func AddProvider(p Provider) error {
	configMu.Lock()
	defer configMu.Unlock()

	if appConfig == nil {
		appConfig = DefaultConfig()
	}
	if p.ID == "" {
		return fmt.Errorf("provider id is required")
	}
	modelsJSON, _ := json.Marshal(p.Models)
	_, err := db.Exec(
		"INSERT OR REPLACE INTO providers(id, name, type, api_endpoint, api_key, models, icon) VALUES(?, ?, ?, ?, ?, ?, ?)",
		p.ID, p.Name, p.Type, p.APIEndpoint, p.APIKey, string(modelsJSON), p.Icon,
	)
	if err != nil {
		return err
	}

	appConfig.Providers = append(appConfig.Providers, p)
	return nil
}

func RemoveProvider(id string) error {
	configMu.Lock()
	defer configMu.Unlock()

	if appConfig == nil {
		appConfig = DefaultConfig()
	}
	_, err := db.Exec("DELETE FROM providers WHERE id = ?", id)
	if err != nil {
		return err
	}

	var filtered []Provider
	for _, p := range appConfig.Providers {
		if p.ID != id {
			filtered = append(filtered, p)
		}
	}
	appConfig.Providers = filtered
	return nil
}

func UpdateProvider(id string, updated Provider) error {
	configMu.Lock()
	defer configMu.Unlock()

	if appConfig == nil {
		appConfig = DefaultConfig()
	}
	updated.ID = id
	if isMaskedAPIKey(updated.APIKey) {
		for _, p := range appConfig.Providers {
			if p.ID == id {
				updated.APIKey = p.APIKey
				break
			}
		}
	}
	modelsJSON, _ := json.Marshal(updated.Models)
	_, err := db.Exec(
		"UPDATE providers SET name=?, type=?, api_endpoint=?, api_key=?, models=?, icon=? WHERE id=?",
		updated.Name, updated.Type, updated.APIEndpoint, updated.APIKey, string(modelsJSON), updated.Icon, id,
	)
	if err != nil {
		return err
	}

	for i, p := range appConfig.Providers {
		if p.ID == id {
			appConfig.Providers[i] = updated
			break
		}
	}
	return nil
}

func ExportConfigJSON() ([]byte, error) {
	cfg := GetConfig()
	return json.MarshalIndent(cfg, "", "  ")
}

// MigrateYAMLToDB migrates existing config.yaml data to SQLite if DB is empty
func MigrateYAMLToDB(yamlPath string) error {
	data, err := os.ReadFile(yamlPath)
	if err != nil {
		return nil // No YAML file, skip migration
	}

	// Check if DB already has providers
	var count int
	db.QueryRow("SELECT COUNT(*) FROM providers").Scan(&count)
	if count > 0 {
		return nil // Already migrated
	}

	// Parse YAML manually - import yaml only for migration
	type yamlProvider struct {
		ID          string   `yaml:"id"`
		Name        string   `yaml:"name"`
		Type        string   `yaml:"type"`
		APIEndpoint string   `yaml:"api_endpoint"`
		APIKey      string   `yaml:"api_key"`
		Models      []string `yaml:"models"`
		Icon        string   `yaml:"icon"`
	}
	type yamlConfig struct {
		Title               string        `yaml:"title"`
		Providers           []yamlProvider `yaml:"providers"`
		TimeoutSeconds      float64       `yaml:"timeout_seconds"`
		SlowThresholdMs     int           `yaml:"slow_threshold_ms"`
		Concurrency         int           `yaml:"concurrency"`
		ProviderConcurrency int           `yaml:"provider_concurrency"`
		ProbePrompt         string        `yaml:"probe_prompt"`
		ProbeSystemPrompt   string        `yaml:"probe_system_prompt"`
		HistorySize         int           `yaml:"history_size"`
		StatsWindowDays     int           `yaml:"stats_window_days"`
		ShowCurveChart      bool          `yaml:"show_curve_chart"`
		ShowErrorDetail     bool          `yaml:"show_error_detail"`
		AutoCheckInterval   int           `yaml:"auto_check_interval_seconds"`
		Port                int           `yaml:"port"`
	}

	var yc yamlConfig
	if err := parseYAML(data, &yc); err != nil {
		return nil // Can't parse, skip
	}

	cfg := &AppConfig{
		Title:               yc.Title,
		TimeoutSeconds:      yc.TimeoutSeconds,
		SlowThresholdMs:     yc.SlowThresholdMs,
		Concurrency:         yc.Concurrency,
		ProviderConcurrency: yc.ProviderConcurrency,
		ProbePrompt:         yc.ProbePrompt,
		ProbeSystemPrompt:   yc.ProbeSystemPrompt,
		HistorySize:         yc.HistorySize,
		StatsWindowDays:     yc.StatsWindowDays,
		ShowCurveChart:      yc.ShowCurveChart,
		ShowErrorDetail:     yc.ShowErrorDetail,
		AutoCheckInterval:   yc.AutoCheckInterval,
		Port:                yc.Port,
	}
	for _, yp := range yc.Providers {
		cfg.Providers = append(cfg.Providers, Provider{
			ID: yp.ID, Name: yp.Name, Type: yp.Type,
			APIEndpoint: yp.APIEndpoint, APIKey: yp.APIKey,
			Models: yp.Models, Icon: yp.Icon,
		})
	}

	return SaveConfig(cfg)
}