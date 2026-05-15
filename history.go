package main

import (
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"time"
)

func AppendHistoryRecord(providerID, model string, record HistoryRecord, maxSize int) {
	// Insert the new record
	_, err := db.Exec(
		"INSERT OR REPLACE INTO history(provider_id, model, status, latency_ms, checked_at) VALUES(?, ?, ?, ?, ?)",
		providerID, model, record.Status, record.LatencyMs, record.CheckedAt,
	)
	if err != nil {
		return
	}

	// Prune old records: keep only up to maxSize*16 per key
	maxStore := maxSize * 16
	if maxStore > 0 {
		// Delete oldest records beyond maxStore
		db.Exec(`DELETE FROM history WHERE id IN (
			SELECT id FROM history
			WHERE provider_id = ? AND model = ?
			ORDER BY checked_at DESC
			LIMIT -1 OFFSET ?
		)`, providerID, model, maxStore)
	}

	// Prune records outside the stats window
	cfg := GetConfig()
	windowStart := time.Now().AddDate(0, 0, -cfg.StatsWindowDays).Format("2006-01-02 15:04:05")
	db.Exec(`DELETE FROM history WHERE provider_id = ? AND model = ? AND checked_at < ?`,
		providerID, model, windowStart)
}

func LoadHistoryRecords(providerID, model string, limit int) []HistoryRecord {
	query := "SELECT status, latency_ms, checked_at FROM history WHERE provider_id = ? AND model = ? ORDER BY checked_at ASC"
	if limit > 0 {
		// Get the most recent `limit` records
		query = `SELECT status, latency_ms, checked_at FROM history
			WHERE provider_id = ? AND model = ?
			ORDER BY checked_at DESC LIMIT ?`
	}

	var rows *sql.Rows
	var err error
	if limit > 0 {
		rows, err = db.Query(query, providerID, model, limit)
	} else {
		rows, err = db.Query(query, providerID, model)
	}
	if err != nil {
		return nil
	}
	defer rows.Close()

	var records []HistoryRecord
	for rows.Next() {
		var r HistoryRecord
		if err := rows.Scan(&r.Status, &r.LatencyMs, &r.CheckedAt); err != nil {
			continue
		}
		records = append(records, r)
	}

	// If we used DESC order (with limit), reverse to ASC
	if limit > 0 && len(records) > 1 {
		for i, j := 0, len(records)-1; i < j; i, j = i+1, j-1 {
			records[i], records[j] = records[j], records[i]
		}
	}

	return records
}

func LoadAllHistory() (map[string][]HistoryRecord, error) {
	rows, err := db.Query("SELECT provider_id, model, status, latency_ms, checked_at FROM history ORDER BY checked_at ASC")
	if err != nil {
		return make(map[string][]HistoryRecord), nil
	}
	defer rows.Close()

	result := make(map[string][]HistoryRecord)
	for rows.Next() {
		var providerID, model, status, checkedAt string
		var latencyMs int
		if err := rows.Scan(&providerID, &model, &status, &latencyMs, &checkedAt); err != nil {
			continue
		}
		key := fmt.Sprintf("%s::%s", providerID, model)
		result[key] = append(result[key], HistoryRecord{
			Status:    status,
			LatencyMs: latencyMs,
			CheckedAt: checkedAt,
		})
	}
	return result, nil
}

func historyBars(records []HistoryRecord, historySize int) []string {
	statuses := make([]string, 0, len(records))
	start := 0
	if len(records) > historySize {
		start = len(records) - historySize
	}
	for _, r := range records[start:] {
		statuses = append(statuses, r.Status)
	}
	padding := historySize - len(statuses)
	result := make([]string, 0, historySize)
	for i := 0; i < padding; i++ {
		result = append(result, "empty")
	}
	result = append(result, statuses...)
	return result
}

func latencyCurve(records []HistoryRecord, historySize int) []int {
	start := 0
	if len(records) > historySize {
		start = len(records) - historySize
	}
	var lats []int
	for _, r := range records[start:] {
		lats = append(lats, r.LatencyMs)
	}
	padding := historySize - len(lats)
	curve := make([]int, 0, historySize)
	for i := 0; i < padding; i++ {
		curve = append(curve, 0)
	}
	curve = append(curve, lats...)
	return curve
}

func avgLatency24h(records []HistoryRecord) string {
	windowStart := time.Now().Add(-24 * time.Hour)
	var valid []int
	for _, r := range records {
		if r.Status == "ok" || r.Status == "slow" {
			t, err := time.Parse("2006-01-02 15:04:05", r.CheckedAt)
			if err == nil && t.After(windowStart) {
				valid = append(valid, r.LatencyMs)
			}
		}
	}
	if len(valid) == 0 {
		return "N/A"
	}
	sum := 0
	for _, v := range valid {
		sum += v
	}
	avg := sum / len(valid)
	return fmt.Sprintf("%d ms", avg)
}

func availability(records []HistoryRecord) string {
	cfg := GetConfig()
	windowStart := time.Now().AddDate(0, 0, -cfg.StatsWindowDays)
	var total, reachable int
	for _, r := range records {
		t, err := time.Parse("2006-01-02 15:04:05", r.CheckedAt)
		if err != nil || t.After(windowStart) {
			total++
			if r.Status == "ok" || r.Status == "slow" {
				reachable++
			}
		}
	}
	if total == 0 {
		return "0.00%"
	}
	return fmt.Sprintf("%.2f%%", float64(reachable)/float64(total)*100)
}

func weeklySuccessText(records []HistoryRecord) string {
	cfg := GetConfig()
	windowStart := time.Now().AddDate(0, 0, -cfg.StatsWindowDays)
	var total, success int
	for _, r := range records {
		t, err := time.Parse("2006-01-02 15:04:05", r.CheckedAt)
		if err != nil || t.After(windowStart) {
			total++
			if r.Status == "ok" || r.Status == "slow" {
				success++
			}
		}
	}
	return fmt.Sprintf("%d/%d", success, total)
}

func extractTimeLabels(records []HistoryRecord, historySize int) []TimeLabel {
	start := 0
	if len(records) > historySize {
		start = len(records) - historySize
	}
	actualRecords := records[start:]
	n := len(actualRecords)
	if n <= 1 {
		return nil
	}

	padLen := historySize - n
	total := historySize

	numLabels := 4
	if n < 4 {
		numLabels = n
	}
	if numLabels <= 1 {
		return nil
	}

	indices := make([]int, numLabels)
	for i := range indices {
		indices[i] = int(float64(i) * float64(n-1) / float64(numLabels-1))
	}

	var labels []TimeLabel
	lastPct := -100.0
	for _, idx := range indices {
		rec := actualRecords[idx]
		t, err := time.Parse("2006-01-02 15:04:05", rec.CheckedAt)
		if err != nil {
			continue
		}
		xPct := float64(padLen+idx) / float64(total-1) * 100
		if xPct-lastPct < 15 {
			continue
		}
		lastPct = xPct
		labels = append(labels, TimeLabel{
			Text: t.Format("15:04"),
			X:    xPct,
		})
	}

	sort.Slice(labels, func(i, j int) bool {
		return labels[i].X < labels[j].X
	})

	return labels
}

func shortError(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) <= 120 {
		return s
	}
	return s[:119] + "..."
}