package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const maxRetries = 2

func probeOpenAICompat(provider Provider, model string, cfg *AppConfig) ProbeResult {
	start := time.Now()
	result := ProbeResult{
		ProviderID:   provider.ID,
		ProviderName: provider.Name,
		ProviderType: provider.Type,
		ProviderIcon: provider.Icon,
		Model:        model,
		CheckedAt:    start.Format("2006-01-02 15:04:05"),
	}

	endpoint := strings.TrimRight(provider.APIEndpoint, "/")
	url := endpoint + "/chat/completions"

	requestBody := map[string]interface{}{
		"model": model,
		"messages": []map[string]string{
			{"role": "system", "content": cfg.ProbeSystemPrompt},
			{"role": "user", "content": cfg.ProbePrompt},
		},
		"max_tokens":  16,
		"temperature": 0,
	}

	bodyBytes, err := json.Marshal(requestBody)
	if err != nil {
		result.Status = "error"
		result.LatencyMs = int(time.Since(start).Milliseconds())
		result.Error = fmt.Sprintf("request marshal error: %s", err.Error())
		return result
	}

	var lastErr string
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Duration(attempt) * 500 * time.Millisecond) // backoff: 500ms, 1000ms
		}

		req, err := http.NewRequest("POST", url, bytes.NewReader(bodyBytes))
		if err != nil {
			result.Status = "error"
			result.LatencyMs = int(time.Since(start).Milliseconds())
			result.Error = fmt.Sprintf("create request error: %s", err.Error())
			return result
		}

		req.Header.Set("Content-Type", "application/json")
		if provider.APIKey != "" {
			req.Header.Set("Authorization", "Bearer "+provider.APIKey)
		}

		client := &http.Client{
			Timeout: time.Duration(cfg.TimeoutSeconds * float64(time.Second)),
		}

		resp, err := client.Do(req)
		latencyMs := int(time.Since(start).Milliseconds())
		result.LatencyMs = latencyMs

		if err != nil {
			lastErr = err.Error()
			// Retry on timeout or connection errors
			if strings.Contains(err.Error(), "timeout") || strings.Contains(err.Error(), "deadline") ||
				strings.Contains(err.Error(), "connection") || strings.Contains(err.Error(), "temporary") {
				continue
			}
			// Non-retriable error
			result.Status = "error"
			result.Error = shortError(lastErr)
			return result
		}

		respBody, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			lastErr = err.Error()
			continue
		}

		if resp.StatusCode >= 400 {
			// Retry on 429 (rate limit) and 5xx (server errors)
			if resp.StatusCode == 429 || resp.StatusCode >= 500 {
				lastErr = fmt.Sprintf("HTTP %d", resp.StatusCode)
				continue
			}
			// Non-retriable client error
			result.Status = "error"
			var errResp map[string]interface{}
			if json.Unmarshal(respBody, &errResp) == nil {
				if e, ok := errResp["error"].(map[string]interface{}); ok {
					if msg, ok := e["message"].(string); ok {
						result.Error = shortError(msg)
						return result
					}
				}
			}
			result.Error = fmt.Sprintf("HTTP %d: %s", resp.StatusCode, shortError(string(respBody)))
			return result
		}

		var chatResp map[string]interface{}
		if err := json.Unmarshal(respBody, &chatResp); err != nil {
			result.Status = "error"
			result.Error = shortError(fmt.Sprintf("parse response error: %s", err.Error()))
			return result
		}

		var completionText string
		if choices, ok := chatResp["choices"].([]interface{}); ok && len(choices) > 0 {
			if choice, ok := choices[0].(map[string]interface{}); ok {
				if msg, ok := choice["message"].(map[string]interface{}); ok {
					if content, ok := msg["content"].(string); ok {
						completionText = strings.TrimSpace(content)
					}
				}
			}
		}

		result.ResponseText = completionText
		if len(completionText) > 80 {
			result.ResponseText = completionText[:80]
		}

		if latencyMs >= cfg.SlowThresholdMs {
			result.Status = "slow"
		} else {
			result.Status = "ok"
		}
		return result
	}

	// All retries exhausted
	result.Status = "error"
	if strings.Contains(lastErr, "timeout") || strings.Contains(lastErr, "deadline") {
		result.Error = fmt.Sprintf("timeout after %gs (retried %d times)", cfg.TimeoutSeconds, maxRetries)
	} else {
		result.Error = shortError(fmt.Sprintf("%s (retried %d times)", lastErr, maxRetries))
	}
	return result
}

func probeAnthropic(provider Provider, model string, cfg *AppConfig) ProbeResult {
	start := time.Now()
	result := ProbeResult{
		ProviderID:   provider.ID,
		ProviderName: provider.Name,
		ProviderType: provider.Type,
		ProviderIcon: provider.Icon,
		Model:        model,
		CheckedAt:    start.Format("2006-01-02 15:04:05"),
	}

	endpoint := strings.TrimRight(provider.APIEndpoint, "/")
	url := endpoint + "/messages"

	requestBody := map[string]interface{}{
		"model":      model,
		"max_tokens": 16,
		"system":     cfg.ProbeSystemPrompt,
		"messages": []map[string]string{
			{"role": "user", "content": cfg.ProbePrompt},
		},
	}

	bodyBytes, err := json.Marshal(requestBody)
	if err != nil {
		result.Status = "error"
		result.LatencyMs = int(time.Since(start).Milliseconds())
		result.Error = shortError(err.Error())
		return result
	}

	var lastErr string
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Duration(attempt) * 500 * time.Millisecond)
		}

		req, err := http.NewRequest("POST", url, bytes.NewReader(bodyBytes))
		if err != nil {
			result.Status = "error"
			result.LatencyMs = int(time.Since(start).Milliseconds())
			result.Error = shortError(err.Error())
			return result
		}

		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("x-api-key", provider.APIKey)
		req.Header.Set("anthropic-version", "2023-06-01")

		client := &http.Client{
			Timeout: time.Duration(cfg.TimeoutSeconds * float64(time.Second)),
		}

		resp, err := client.Do(req)
		latencyMs := int(time.Since(start).Milliseconds())
		result.LatencyMs = latencyMs

		if err != nil {
			lastErr = err.Error()
			if strings.Contains(err.Error(), "timeout") || strings.Contains(err.Error(), "deadline") ||
				strings.Contains(err.Error(), "connection") || strings.Contains(err.Error(), "temporary") {
				continue
			}
			result.Status = "error"
			result.Error = shortError(lastErr)
			return result
		}

		respBody, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			lastErr = err.Error()
			continue
		}

		if resp.StatusCode >= 400 {
			if resp.StatusCode == 429 || resp.StatusCode >= 500 {
				lastErr = fmt.Sprintf("HTTP %d", resp.StatusCode)
				continue
			}
			result.Status = "error"
			var errResp map[string]interface{}
			if json.Unmarshal(respBody, &errResp) == nil {
				if e, ok := errResp["error"].(map[string]interface{}); ok {
					if msg, ok := e["message"].(string); ok {
						result.Error = shortError(msg)
						return result
					}
				}
			}
			result.Error = fmt.Sprintf("HTTP %d: %s", resp.StatusCode, shortError(string(respBody)))
			return result
		}

		var msgResp map[string]interface{}
		if err := json.Unmarshal(respBody, &msgResp); err == nil {
			if content, ok := msgResp["content"].([]interface{}); ok && len(content) > 0 {
				if block, ok := content[0].(map[string]interface{}); ok {
					if text, ok := block["text"].(string); ok {
						completionText := strings.TrimSpace(text)
						result.ResponseText = completionText
						if len(completionText) > 80 {
							result.ResponseText = completionText[:80]
						}
					}
				}
			}
		}

		if latencyMs >= cfg.SlowThresholdMs {
			result.Status = "slow"
		} else {
			result.Status = "ok"
		}
		return result
	}

	// All retries exhausted
	result.Status = "error"
	if strings.Contains(lastErr, "timeout") || strings.Contains(lastErr, "deadline") {
		result.Error = fmt.Sprintf("timeout after %gs (retried %d times)", cfg.TimeoutSeconds, maxRetries)
	} else {
		result.Error = shortError(fmt.Sprintf("%s (retried %d times)", lastErr, maxRetries))
	}
	return result
}

func probeModel(provider Provider, model string, cfg *AppConfig) ProbeResult {
	switch strings.ToLower(provider.Type) {
	case "anthropic":
		return probeAnthropic(provider, model, cfg)
	default:
		return probeOpenAICompat(provider, model, cfg)
	}
}