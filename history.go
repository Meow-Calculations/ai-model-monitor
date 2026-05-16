package main

import (
	"database/sql"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

func AppendHistoryRecord(providerID, model string, record HistoryRecord, maxSize int) error {
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	_, err = tx.Exec(
		"INSERT OR REPLACE INTO history(provider_id, model, status, latency_ms, checked_at) VALUES(?, ?, ?, ?, ?)",
		providerID, model, record.Status, record.LatencyMs, record.CheckedAt,
	)
	if err != nil {
		return fmt.Errorf("insert history: %w", err)
	}

	maxStore := maxSize * 16
	if maxStore > 0 {
		_, err = tx.Exec(`DELETE FROM history
			WHERE provider_id = ? AND model = ?
			AND id NOT IN (
				SELECT id FROM history
				WHERE provider_id = ? AND model = ?
				ORDER BY checked_at DESC
				LIMIT ?
			)`, providerID, model, providerID, model, maxStore)
		if err != nil {
			return fmt.Errorf("prune history: %w", err)
		}
	}

	cfg := GetConfig()
	windowStart := time.Now().AddDate(0, 0, -cfg.StatsWindowDays).Format("2006-01-02 15:04:05")
	_, err = tx.Exec(`DELETE FROM history WHERE provider_id = ? AND model = ? AND checked_at < ?`,
		providerID, model, windowStart)
	if err != nil {
		return fmt.Errorf("prune window: %w", err)
	}

	return tx.Commit()
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
		return nil, fmt.Errorf("query all history: %w", err)
	}
	defer rows.Close()

	result := make(map[string][]HistoryRecord)
	for rows.Next() {
		var providerID, model, status, checkedAt string
		var latencyMs int
		if err := rows.Scan(&providerID, &model, &status, &latencyMs, &checkedAt); err != nil {
			return nil, fmt.Errorf("scan history: %w", err)
		}
		key := fmt.Sprintf("%s::%s", providerID, model)
		result[key] = append(result[key], HistoryRecord{
			Status:    status,
			LatencyMs: latencyMs,
			CheckedAt: checkedAt,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate history: %w", err)
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

type computedStats struct {
	avgLatency24h string
	availability  string
	weeklySuccess string
}

func computeStats(records []HistoryRecord, statsWindowDays int) computedStats {
	now := time.Now()
	windowStart := now.AddDate(0, 0, -statsWindowDays)
	dayAgo := now.Add(-24 * time.Hour)

	var (
		total24hSum int
		total24hCnt int
		windowTotal int
		windowReach int
		windowSucc  int
	)

	for _, r := range records {
		t, err := time.Parse("2006-01-02 15:04:05", r.CheckedAt)
		if err != nil {
			continue
		}

		if t.After(windowStart) {
			windowTotal++
			if r.Status == "ok" || r.Status == "slow" {
				windowReach++
				windowSucc++
			}
		}

		if (r.Status == "ok" || r.Status == "slow") && t.After(dayAgo) {
			total24hSum += r.LatencyMs
			total24hCnt++
		}
	}

	var s computedStats

	if total24hCnt > 0 {
		s.avgLatency24h = strconv.Itoa(total24hSum/total24hCnt) + " ms"
	} else {
		s.avgLatency24h = "N/A"
	}

	if windowTotal > 0 {
		s.availability = fmt.Sprintf("%.2f%%", float64(windowReach)/float64(windowTotal)*100)
		s.weeklySuccess = fmt.Sprintf("%d/%d", windowSucc, windowTotal)
	} else {
		s.availability = "0.00%"
		s.weeklySuccess = "0/0"
	}

	return s
}
