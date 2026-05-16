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

var probeHTTPClient = &http.Client{
	Timeout: 30 * time.Second,
	Transport: &http.Transport{
		MaxIdleConns:        25,
		MaxIdleConnsPerHost: 10,
		IdleConnTimeout:     90 * time.Second,
		DisableCompression:  false,
	},
}

type probeAPIHandler struct {
	urlSuffix     string
	buildBody     func(model string, cfg *AppConfig) map[string]interface{}
	setHeaders    func(req *http.Request, provider Provider)
	parseResponse func(body []byte) string
}

func getProbeHandler(providerType string) probeAPIHandler {
	switch strings.ToLower(providerType) {
	case "anthropic":
		return probeAPIHandler{
			urlSuffix: "/messages",
			buildBody: func(model string, cfg *AppConfig) map[string]interface{} {
				return map[string]interface{}{
					"model":      model,
					"max_tokens": 16,
					"system":     cfg.ProbeSystemPrompt,
					"messages": []map[string]string{
						{"role": "user", "content": cfg.ProbePrompt},
					},
				}
			},
			setHeaders: func(req *http.Request, provider Provider) {
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("x-api-key", provider.APIKey)
				req.Header.Set("anthropic-version", "2023-06-01")
			},
			parseResponse: func(body []byte) string {
				var resp map[string]interface{}
				if err := json.Unmarshal(body, &resp); err != nil {
					return ""
				}
				content, ok := resp["content"].([]interface{})
				if !ok || len(content) == 0 {
					return ""
				}
				block, ok := content[0].(map[string]interface{})
				if !ok {
					return ""
				}
				text, _ := block["text"].(string)
				return strings.TrimSpace(text)
			},
		}
	default:
		return probeAPIHandler{
			urlSuffix: "/chat/completions",
			buildBody: func(model string, cfg *AppConfig) map[string]interface{} {
				return map[string]interface{}{
					"model": model,
					"messages": []map[string]string{
						{"role": "system", "content": cfg.ProbeSystemPrompt},
						{"role": "user", "content": cfg.ProbePrompt},
					},
					"max_tokens":  16,
					"temperature": 0,
				}
			},
			setHeaders: func(req *http.Request, provider Provider) {
				req.Header.Set("Content-Type", "application/json")
				if provider.APIKey != "" {
					req.Header.Set("Authorization", "Bearer "+provider.APIKey)
				}
			},
			parseResponse: func(body []byte) string {
				var resp map[string]interface{}
				if err := json.Unmarshal(body, &resp); err != nil {
					return ""
				}
				choices, ok := resp["choices"].([]interface{})
				if !ok || len(choices) == 0 {
					return ""
				}
				choice, ok := choices[0].(map[string]interface{})
				if !ok {
					return ""
				}
				msg, ok := choice["message"].(map[string]interface{})
				if !ok {
					return ""
				}
				content, _ := msg["content"].(string)
				return strings.TrimSpace(content)
			},
		}
	}
}

func probeModel(provider Provider, model string, cfg *AppConfig) ProbeResult {
	start := time.Now()
	result := ProbeResult{
		ProviderID:   provider.ID,
		ProviderName: provider.Name,
		ProviderType: provider.Type,
		ProviderIcon: provider.Icon,
		Model:        model,
		CheckedAt:    start.Format("2006-01-02 15:04:05"),
	}

	handler := getProbeHandler(provider.Type)
	endpoint := strings.TrimRight(provider.APIEndpoint, "/")
	url := endpoint + handler.urlSuffix

	requestBody := handler.buildBody(model, cfg)
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
			time.Sleep(time.Duration(attempt) * 500 * time.Millisecond)
		}

		req, err := http.NewRequest("POST", url, bytes.NewReader(bodyBytes))
		if err != nil {
			result.Status = "error"
			result.LatencyMs = int(time.Since(start).Milliseconds())
			result.Error = fmt.Sprintf("create request error: %s", err.Error())
			return result
		}

		handler.setHeaders(req, provider)

		probeHTTPClient.Timeout = time.Duration(cfg.TimeoutSeconds * float64(time.Second))
		resp, err := probeHTTPClient.Do(req)
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

		completionText := handler.parseResponse(respBody)

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

	result.Status = "error"
	if strings.Contains(lastErr, "timeout") || strings.Contains(lastErr, "deadline") {
		result.Error = fmt.Sprintf("timeout after %gs (retried %d times)", cfg.TimeoutSeconds, maxRetries)
	} else {
		result.Error = shortError(fmt.Sprintf("%s (retried %d times)", lastErr, maxRetries))
	}
	return result
}
