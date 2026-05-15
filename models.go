package main

type Provider struct {
	ID          string   `json:"id" yaml:"id"`
	Name        string   `json:"name" yaml:"name"`
	Type        string   `json:"type" yaml:"type"`
	APIEndpoint string   `json:"api_endpoint" yaml:"api_endpoint"`
	APIKey      string   `json:"api_key" yaml:"api_key"`
	Models      []string `json:"models" yaml:"models"`
	Icon        string   `json:"icon" yaml:"icon"`
}

type ProbeResult struct {
	ProviderID     string `json:"provider_id"`
	ProviderName   string `json:"provider_name"`
	ProviderType   string `json:"provider_type"`
	ProviderIcon   string `json:"provider_icon"`
	Model          string `json:"model"`
	Status         string `json:"status"`
	LatencyMs      int    `json:"latency_ms"`
	Error          string `json:"error,omitempty"`
	ResponseText   string `json:"response_preview,omitempty"`
	CheckedAt      string `json:"checked_at"`
}

type ModelStatus struct {
	ProviderID       string   `json:"provider_id"`
	ProviderName     string   `json:"provider_name"`
	ProviderType     string   `json:"provider_type"`
	ProviderIcon     string   `json:"provider_icon"`
	Model            string   `json:"model"`
	Status           string   `json:"status"`
	StatusLabel      string   `json:"status_label"`
	LatencyMs        int      `json:"latency_ms"`
	AvgLatency24h    string   `json:"avg_latency_24h"`
	Availability     string   `json:"availability"`
	WeeklySuccess    string   `json:"weekly_success_text"`
	Error            string   `json:"error,omitempty"`
	History          []string `json:"history"`
	LatencyCurve     []int    `json:"latency_curve,omitempty"`
	TimeLabels       []TimeLabel `json:"time_labels,omitempty"`
	ShowCurveChart   bool     `json:"show_curve_chart"`
}

type TimeLabel struct {
	Text string  `json:"text"`
	X    float64 `json:"x_pct"`
}

type ProviderStatus struct {
	ProviderID   string         `json:"provider_id"`
	ProviderName string         `json:"provider_name"`
	ProviderType string         `json:"provider_type"`
	ProviderIcon string         `json:"provider_logo"`
	ModelCount   int            `json:"model_count"`
	Status       string         `json:"status"`
	StatusLabel  string         `json:"status_label"`
	OkCount      int            `json:"ok_count"`
	SlowCount    int            `json:"slow_count"`
	ErrorCount   int            `json:"error_count"`
	Results      []ModelStatus  `json:"results"`
}

type DashboardReport struct {
	Title               string            `json:"title"`
	GeneratedAt         string            `json:"generated_at"`
	ElapsedMs           int               `json:"elapsed_ms"`
	Total               int               `json:"total"`
	OkCount             int               `json:"ok_count"`
	SlowCount           int               `json:"slow_count"`
	ErrorCount          int               `json:"error_count"`
	ProviderCount       int               `json:"provider_count"`
	Providers           []ProviderStatus  `json:"providers"`
	OverallStatus       string            `json:"overall_status"`
	OverallClass        string            `json:"overall_class"`
	StatsWindowDays     int               `json:"stats_window_days"`
	HistorySize         int               `json:"history_size"`
	Concurrency         int               `json:"global_concurrency"`
	ProviderConcurrency int               `json:"provider_concurrency"`
}

type HistoryRecord struct {
	Status    string `json:"status"`
	LatencyMs int    `json:"latency_ms"`
	CheckedAt string `json:"checked_at"`
}

type AppConfig struct {
	Title               string     `json:"title" yaml:"title"`
	Providers           []Provider `json:"providers" yaml:"providers"`
	TimeoutSeconds      float64    `json:"timeout_seconds" yaml:"timeout_seconds"`
	SlowThresholdMs     int        `json:"slow_threshold_ms" yaml:"slow_threshold_ms"`
	Concurrency         int        `json:"concurrency" yaml:"concurrency"`
	ProviderConcurrency int        `json:"provider_concurrency" yaml:"provider_concurrency"`
	ProbePrompt         string     `json:"probe_prompt" yaml:"probe_prompt"`
	ProbeSystemPrompt   string     `json:"probe_system_prompt" yaml:"probe_system_prompt"`
	HistorySize         int        `json:"history_size" yaml:"history_size"`
	StatsWindowDays     int        `json:"stats_window_days" yaml:"stats_window_days"`
	ShowCurveChart      bool       `json:"show_curve_chart" yaml:"show_curve_chart"`
	ShowErrorDetail     bool       `json:"show_error_detail" yaml:"show_error_detail"`
	AutoCheckInterval   int        `json:"auto_check_interval_seconds" yaml:"auto_check_interval_seconds"`
	Port                int        `json:"port" yaml:"port"`
}
