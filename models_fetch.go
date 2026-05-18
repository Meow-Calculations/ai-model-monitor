package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

type ModelListFetcher interface {
	ModelsURL(baseEndpoint string) string
	ParseModelsResponse(body []byte) ([]string, error)
	SetHeaders(req *http.Request, provider Provider)
}

type openAIModelsFetcher struct{}

type openAIModelsResponse struct {
	Data []openAIModelEntry `json:"data"`
}

type openAIModelEntry struct {
	ID     string `json:"id"`
	Object string `json:"object"`
}

func (f *openAIModelsFetcher) ModelsURL(baseEndpoint string) string {
	base := strings.TrimRight(baseEndpoint, "/")
	return base + "/models"
}

func (f *openAIModelsFetcher) ParseModelsResponse(body []byte) ([]string, error) {
	var resp openAIModelsResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parse openai models: %w", err)
	}
	if len(resp.Data) == 0 {
		return nil, fmt.Errorf("openai models response has empty data")
	}
	var models []string
	for _, entry := range resp.Data {
		if entry.Object == "model" || entry.Object == "" {
			models = append(models, entry.ID)
		}
	}
	if len(models) == 0 {
		return nil, fmt.Errorf("openai models response: no entries with object=model")
	}
	return models, nil
}

func (f *openAIModelsFetcher) SetHeaders(req *http.Request, provider Provider) {
	req.Header.Set("Content-Type", "application/json")
	if provider.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+provider.APIKey)
	}
}

type anthropicModelsFetcher struct{}

type anthropicModelsResponse struct {
	Data []anthropicModelEntry `json:"data"`
}

type anthropicModelEntry struct {
	Type string `json:"type"`
	Name string `json:"name"`
}

func (f *anthropicModelsFetcher) ModelsURL(baseEndpoint string) string {
	base := strings.TrimRight(baseEndpoint, "/")
	return base + "/models"
}

func (f *anthropicModelsFetcher) ParseModelsResponse(body []byte) ([]string, error) {
	var resp anthropicModelsResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parse anthropic models: %w", err)
	}
	if len(resp.Data) == 0 {
		return nil, fmt.Errorf("anthropic models response has empty data")
	}
	var models []string
	for _, entry := range resp.Data {
		if entry.Type == "model" && entry.Name != "" {
			models = append(models, entry.Name)
		}
	}
	if len(models) == 0 {
		return nil, fmt.Errorf("anthropic models response: no entries with type=model")
	}
	return models, nil
}

func (f *anthropicModelsFetcher) SetHeaders(req *http.Request, provider Provider) {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", provider.APIKey)
	req.Header.Set("anthropic-version", "2023-06-01")
}

func getModelListFetcher(providerType string) ModelListFetcher {
	switch strings.ToLower(providerType) {
	case "anthropic":
		return &anthropicModelsFetcher{}
	default:
		return &openAIModelsFetcher{}
	}
}

func FetchModels(provider Provider) ([]string, error) {
	fetcher := getModelListFetcher(provider.Type)
	url := fetcher.ModelsURL(provider.APIEndpoint)

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	fetcher.SetHeaders(req, provider)

	client := &http.Client{
		Timeout: 15 * time.Second,
		Transport: &http.Transport{
			MaxIdleConns:       4,
			IdleConnTimeout:    30 * time.Second,
			DisableCompression: false,
		},
	}
	defer client.CloseIdleConnections()

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode == http.StatusTooManyRequests {
		return nil, fmt.Errorf("rate limited (HTTP 429)")
	}
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return nil, fmt.Errorf("authentication failed (HTTP %d)", resp.StatusCode)
	}
	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("models endpoint not found (HTTP 404)")
	}
	if resp.StatusCode >= 400 {
		var errBody map[string]interface{}
		msg := fmt.Sprintf("HTTP %d", resp.StatusCode)
		if json.Unmarshal(body, &errBody) == nil {
			if e, ok := errBody["error"].(map[string]interface{}); ok {
				if m, ok := e["message"].(string); ok {
					msg = m
				}
			}
		}
		return nil, fmt.Errorf("API error: %s", msg)
	}

	models, err := fetcher.ParseModelsResponse(body)
	if err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	return models, nil
}

func AutoFillModels(provider Provider) ([]string, error) {
	models, err := FetchModels(provider)
	if err != nil {
		return nil, err
	}

	existing := GetConfig()
	for _, p := range existing.Providers {
		if p.ID == provider.ID {
			updated := p
			updated.Models = models
			if err := UpdateProvider(p.ID, updated); err != nil {
				return models, fmt.Errorf("update provider: %w", err)
			}
			return models, nil
		}
	}

	return models, fmt.Errorf("provider %s not found in config", provider.ID)
}

func AutoFillAllEmptyProviders() {
	cfg := GetConfig()
	for _, p := range cfg.Providers {
		if len(p.Models) > 0 {
			continue
		}
		models, err := FetchModels(p)
		if err != nil {
			log.Printf("warn: auto-fill models for %s (%s): %v", p.Name, p.Type, err)
			continue
		}
		updated := p
		updated.Models = models
		if err := UpdateProvider(p.ID, updated); err != nil {
			log.Printf("warn: auto-fill save for %s: %v", p.Name, err)
			continue
		}
		log.Printf("auto-filled %d models for %s (%s)", len(models), p.Name, p.Type)
	}
}
