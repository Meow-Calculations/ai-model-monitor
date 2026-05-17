package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
)

var (
	alertRules   []AlertRule
	alertRulesMu sync.RWMutex
	firingAlerts map[string]int
	firingAlertsMu sync.Mutex
)

func initAlertTables() error {
	_, err := db.Exec(`
	CREATE TABLE IF NOT EXISTS alert_rules (
		id          TEXT PRIMARY KEY,
		name        TEXT NOT NULL,
		provider_id TEXT NOT NULL DEFAULT '',
		model_pattern TEXT NOT NULL DEFAULT '',
		metric_type TEXT NOT NULL,
		condition   TEXT NOT NULL,
		threshold   REAL NOT NULL DEFAULT 0,
		duration_min INTEGER NOT NULL DEFAULT 0,
		notify_channels TEXT NOT NULL DEFAULT '[]',
		webhook_url TEXT NOT NULL DEFAULT '',
		enabled     INTEGER NOT NULL DEFAULT 1
	);
	CREATE TABLE IF NOT EXISTS alert_events (
		id          INTEGER PRIMARY KEY AUTOINCREMENT,
		rule_id     TEXT NOT NULL,
		rule_name   TEXT NOT NULL,
		provider_id TEXT NOT NULL,
		model       TEXT NOT NULL,
		metric_type TEXT NOT NULL,
		actual_value REAL NOT NULL DEFAULT 0,
		threshold   REAL NOT NULL DEFAULT 0,
		status      TEXT NOT NULL DEFAULT 'firing',
		triggered_at TEXT NOT NULL,
		resolved_at TEXT NOT NULL DEFAULT '',
		notified    INTEGER NOT NULL DEFAULT 0
	);
	CREATE INDEX IF NOT EXISTS idx_alert_events_rule ON alert_events(rule_id, status);
	`)
	if err != nil {
		return fmt.Errorf("create alert tables: %w", err)
	}
	return loadAlertRules()
}

func loadAlertRules() error {
	alertRulesMu.Lock()
	defer alertRulesMu.Unlock()

	rows, err := db.Query("SELECT id, name, provider_id, model_pattern, metric_type, condition, threshold, duration_min, notify_channels, webhook_url, enabled FROM alert_rules")
	if err != nil {
		return fmt.Errorf("query alert rules: %w", err)
	}
	defer rows.Close()

	alertRules = nil
	for rows.Next() {
		var r AlertRule
		var channelsJSON, providerID, modelPattern, webhookURL string
		var enabledInt int
		if err := rows.Scan(&r.ID, &r.Name, &providerID, &modelPattern, &r.MetricType, &r.Condition, &r.Threshold, &r.DurationMin, &channelsJSON, &webhookURL, &enabledInt); err != nil {
			continue
		}
		r.ProviderID = providerID
		r.ModelPattern = modelPattern
		r.WebhookURL = webhookURL
		r.Enabled = enabledInt != 0
		json.Unmarshal([]byte(channelsJSON), &r.NotifyChannels)
		alertRules = append(alertRules, r)
	}
	return rows.Err()
}

func GetAlertRules() []AlertRule {
	alertRulesMu.RLock()
	defer alertRulesMu.RUnlock()
	result := make([]AlertRule, len(alertRules))
	copy(result, alertRules)
	return result
}

func SaveAlertRule(r AlertRule) error {
	if r.ID == "" {
		r.ID = fmt.Sprintf("rule_%d", time.Now().UnixNano())
	}
	channelsJSON, _ := json.Marshal(r.NotifyChannels)
	enabledInt := 0
	if r.Enabled {
		enabledInt = 1
	}
	err := withRetry("SaveAlertRule", func() error {
		_, err := db.Exec(`INSERT OR REPLACE INTO alert_rules(id, name, provider_id, model_pattern, metric_type, condition, threshold, duration_min, notify_channels, webhook_url, enabled) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			r.ID, r.Name, r.ProviderID, r.ModelPattern, r.MetricType, r.Condition, r.Threshold, r.DurationMin, string(channelsJSON), r.WebhookURL, enabledInt)
		return err
	})
	if err != nil {
		return err
	}
	return loadAlertRules()
}

func DeleteAlertRule(id string) error {
	err := withRetry("DeleteAlertRule", func() error {
		_, err := db.Exec("DELETE FROM alert_rules WHERE id = ?", id)
		return err
	})
	if err != nil {
		return err
	}
	return loadAlertRules()
}

func evaluateAlertRules(report *DashboardReport) {
	alertRulesMu.RLock()
	rules := make([]AlertRule, len(alertRules))
	copy(rules, alertRules)
	alertRulesMu.RUnlock()

	now := time.Now().Format("2006-01-02 15:04:05")

	for _, rule := range rules {
		if !rule.Enabled {
			continue
		}

		for _, provider := range report.Providers {
			if rule.ProviderID != "" && rule.ProviderID != provider.ProviderID {
				continue
			}
			for _, ms := range provider.Results {
				if rule.ModelPattern != "" && !matchModelPattern(ms.Model, rule.ModelPattern) {
					continue
				}

				var actualValue float64
				switch rule.MetricType {
				case "latency":
					actualValue = float64(ms.LatencyMs)
				case "error_count":
					if ms.Status == "error" {
						actualValue = 1
					}
				case "availability":
					avail := parseAvailability(ms.Availability)
					actualValue = avail
				default:
					continue
				}

				triggered := compareThreshold(actualValue, rule.Threshold, rule.Condition)

				firingAlertsMu.Lock()
				key := fmt.Sprintf("%s::%s::%s", rule.ID, provider.ProviderID, ms.Model)
				prevStatus := firingAlerts[key]
				if triggered {
					firingAlerts[key]++
					if prevStatus == 0 {
						createAlertEvent(rule, provider.ProviderID, ms.Model, rule.MetricType, actualValue, now)
					}
				} else {
					delete(firingAlerts, key)
					if prevStatus > 0 {
						resolveAlertEvent(rule, provider.ProviderID, ms.Model, now)
					}
				}
				firingAlertsMu.Unlock()
			}
		}
	}
}

func matchModelPattern(model, pattern string) bool {
	if pattern == "" || pattern == "*" {
		return true
	}
	if strings.HasPrefix(pattern, "*") && strings.HasSuffix(pattern, "*") {
		mid := strings.Trim(pattern, "*")
		return strings.Contains(model, mid)
	}
	if strings.HasSuffix(pattern, "*") {
		return strings.HasPrefix(model, strings.TrimSuffix(pattern, "*"))
	}
	if strings.HasPrefix(pattern, "*") {
		return strings.HasSuffix(model, strings.TrimPrefix(pattern, "*"))
	}
	return model == pattern
}

func compareThreshold(actual, threshold float64, condition string) bool {
	switch condition {
	case "gt":
		return actual > threshold
	case "gte":
		return actual >= threshold
	case "lt":
		return actual < threshold
	case "lte":
		return actual <= threshold
	case "eq":
		return actual == threshold
	}
	return false
}

func parseAvailability(s string) float64 {
	s = strings.TrimRight(s, "%")
	var v float64
	fmt.Sscanf(s, "%f", &v)
	return v
}

func createAlertEvent(rule AlertRule, providerID, model, metricType string, actualValue float64, now string) {
	_, err := db.Exec(`INSERT INTO alert_events(rule_id, rule_name, provider_id, model, metric_type, actual_value, threshold, status, triggered_at) VALUES(?, ?, ?, ?, ?, ?, ?, 'firing', ?)`,
		rule.ID, rule.Name, providerID, model, metricType, actualValue, rule.Threshold, now)
	if err != nil {
		log.Printf("warn: create alert event: %v", err)
		return
	}
	if rule.WebhookURL != "" || len(rule.NotifyChannels) > 0 {
		go sendAlertNotification(rule, providerID, model, metricType, actualValue, "firing")
	}
}

func resolveAlertEvent(rule AlertRule, providerID, model string, now string) {
	db.Exec(`UPDATE alert_events SET status='resolved', resolved_at=? WHERE rule_id=? AND provider_id=? AND model=? AND status='firing'`,
		now, rule.ID, providerID, model)
}

func GetAlertEvents(limit int) ([]AlertEvent, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := db.Query(`SELECT id, rule_id, rule_name, provider_id, model, metric_type, actual_value, threshold, status, triggered_at, COALESCE(resolved_at,''), notified FROM alert_events ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("query alert events: %w", err)
	}
	defer rows.Close()

	var events []AlertEvent
	for rows.Next() {
		var e AlertEvent
		var notifiedInt int
		if err := rows.Scan(&e.ID, &e.RuleID, &e.RuleName, &e.ProviderID, &e.Model, &e.MetricType, &e.ActualValue, &e.Threshold, &e.Status, &e.TriggeredAt, &e.ResolvedAt, &notifiedInt); err != nil {
			continue
		}
		e.Notified = notifiedInt != 0
		events = append(events, e)
	}
	return events, rows.Err()
}

func sendAlertNotification(rule AlertRule, providerID, model, metricType string, actualValue float64, status string) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("warn: alert notification panic recovered for rule %s: %v", rule.ID, r)
		}
	}()

	title := fmt.Sprintf("🔔 [AI Model Monitor] %s", rule.Name)
	if status == "resolved" {
		title = fmt.Sprintf("✅ [AI Model Monitor] %s 已恢复", rule.Name)
	}

	text := fmt.Sprintf("规则: %s\nProvider: %s\n模型: %s\n指标: %s\n当前值: %.2f\n阈值: %.2f\n状态: %s",
		rule.Name, providerID, model, metricType, actualValue, rule.Threshold, status)

	if rule.WebhookURL != "" {
		payload := map[string]interface{}{
			"msgtype": "text",
			"text":    map[string]string{"content": text},
			"title":   title,
		}
		if strings.Contains(rule.WebhookURL, "hooks.slack.com") {
			payload = map[string]interface{}{
				"text": fmt.Sprintf("*%s*\n%s", title, text),
			}
		}
		if strings.Contains(rule.WebhookURL, "feishu") || strings.Contains(rule.WebhookURL, "larksuite") {
			payload = map[string]interface{}{
				"msg_type": "text",
				"content":  map[string]string{"text": text},
			}
		}

		data, _ := json.Marshal(payload)
		client := &http.Client{Timeout: 10 * time.Second}
		resp, err := client.Post(rule.WebhookURL, "application/json", bytes.NewReader(data))
		if err != nil {
			log.Printf("warn: webhook notify failed for rule %s: %v", rule.ID, err)
			return
		}
		resp.Body.Close()
	}

	for _, channel := range rule.NotifyChannels {
		log.Printf("alert: channel=%s rule=%s provider=%s model=%s value=%.2f status=%s", channel, rule.ID, providerID, model, actualValue, status)
	}
}
