package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// --- OpenAI format test server ---

func newOpenAIModelsTestServer(models []string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/models" {
			http.NotFound(w, r)
			return
		}
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		auth := r.Header.Get("Authorization")
		if auth != "Bearer test-key" {
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(map[string]string{"error": "unauthorized"})
			return
		}

		data := make([]map[string]string, len(models))
		for i, m := range models {
			data[i] = map[string]string{"id": m, "object": "model"}
		}
		json.NewEncoder(w).Encode(map[string]interface{}{"data": data})
	}))
}

// --- Anthropic format test server ---

func newAnthropicModelsTestServer(models []string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/models" {
			http.NotFound(w, r)
			return
		}
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		key := r.Header.Get("x-api-key")
		if key != "sk-ant-test" {
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(map[string]string{"error": "unauthorized"})
			return
		}

		data := make([]map[string]string, len(models))
		for i, m := range models {
			data[i] = map[string]string{"type": "model", "name": m}
		}
		json.NewEncoder(w).Encode(map[string]interface{}{"data": data})
	}))
}

// --- Error test servers ---

func newRateLimitTestServer() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		json.NewEncoder(w).Encode(map[string]string{"error": "rate limit exceeded"})
	}))
}

func newNotFoundTestServer() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "not found"})
	}))
}

func newMalformedTestServer() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"data": [{"object": "list"}]}`))
	}))
}

func newEmptyTestServer() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{"data": []interface{}{}})
	}))
}

// --- Tests ---

func TestOpenAIModelsFetcher_ParsesCorrectly(t *testing.T) {
	expectedModels := []string{"gpt-4o", "gpt-4o-mini", "gpt-4-turbo", "claude-sonnet-4-6"}
	srv := newOpenAIModelsTestServer(expectedModels)
	defer srv.Close()

	provider := Provider{
		ID:          "test-openai",
		Name:        "Test OpenAI",
		Type:        "openai",
		APIEndpoint: srv.URL,
		APIKey:      "test-key",
	}

	models, err := FetchModels(provider)
	if err != nil {
		t.Fatalf("FetchModels failed: %v", err)
	}

	if len(models) != len(expectedModels) {
		t.Fatalf("expected %d models, got %d: %v", len(expectedModels), len(models), models)
	}
	for i, m := range models {
		if m != expectedModels[i] {
			t.Fatalf("model[%d]: expected %q, got %q", i, expectedModels[i], m)
		}
	}
}

func TestAnthropicModelsFetcher_ParsesCorrectly(t *testing.T) {
	expectedModels := []string{"claude-sonnet-4-6", "claude-opus-4-6", "claude-haiku-3-5"}
	srv := newAnthropicModelsTestServer(expectedModels)
	defer srv.Close()

	provider := Provider{
		ID:          "test-anthropic",
		Name:        "Test Anthropic",
		Type:        "anthropic",
		APIEndpoint: srv.URL,
		APIKey:      "sk-ant-test",
	}

	models, err := FetchModels(provider)
	if err != nil {
		t.Fatalf("FetchModels failed: %v", err)
	}

	if len(models) != len(expectedModels) {
		t.Fatalf("expected %d models, got %d: %v", len(expectedModels), len(models), models)
	}
	for i, m := range models {
		if m != expectedModels[i] {
			t.Fatalf("model[%d]: expected %q, got %q", i, expectedModels[i], m)
		}
	}
}

func TestOpenAIModelsFetcher_HandlesAuthFailure(t *testing.T) {
	srv := newOpenAIModelsTestServer([]string{"gpt-4o"})
	defer srv.Close()

	provider := Provider{
		ID:          "test-bad-auth",
		Name:        "Bad Auth",
		Type:        "openai",
		APIEndpoint: srv.URL,
		APIKey:      "wrong-key",
	}

	_, err := FetchModels(provider)
	if err == nil {
		t.Fatal("expected error for bad auth, got nil")
	}
	if !contains(err.Error(), "401") && !contains(err.Error(), "unauthorized") && !contains(err.Error(), "authentication") {
		t.Fatalf("expected auth error, got: %v", err)
	}
}

func TestAnthropicModelsFetcher_HandlesAuthFailure(t *testing.T) {
	srv := newAnthropicModelsTestServer([]string{"claude-sonnet-4-6"})
	defer srv.Close()

	provider := Provider{
		ID:          "test-bad-auth",
		Name:        "Bad Auth",
		Type:        "anthropic",
		APIEndpoint: srv.URL,
		APIKey:      "wrong-key",
	}

	_, err := FetchModels(provider)
	if err == nil {
		t.Fatal("expected error for bad auth, got nil")
	}
}

func TestModelsFetcher_HandlesRateLimit(t *testing.T) {
	srv := newRateLimitTestServer()
	defer srv.Close()

	provider := Provider{
		ID:          "test-rate-limited",
		Name:        "Rate Limited",
		Type:        "openai",
		APIEndpoint: srv.URL,
		APIKey:      "test-key",
	}

	_, err := FetchModels(provider)
	if err == nil {
		t.Fatal("expected error for rate limit, got nil")
	}
	if !contains(err.Error(), "429") && !contains(err.Error(), "rate limited") {
		t.Fatalf("expected rate limit error, got: %v", err)
	}
}

func TestModelsFetcher_HandlesNotFound(t *testing.T) {
	srv := newNotFoundTestServer()
	defer srv.Close()

	provider := Provider{
		ID:          "test-not-found",
		Name:        "Not Found",
		Type:        "openai",
		APIEndpoint: srv.URL,
		APIKey:      "test-key",
	}

	_, err := FetchModels(provider)
	if err == nil {
		t.Fatal("expected error for 404, got nil")
	}
	if !contains(err.Error(), "404") && !contains(err.Error(), "not found") {
		t.Fatalf("expected 404 error, got: %v", err)
	}
}

func TestModelsFetcher_HandlesMalformedResponse(t *testing.T) {
	srv := newMalformedTestServer()
	defer srv.Close()

	provider := Provider{
		ID:          "test-malformed",
		Name:        "Malformed",
		Type:        "openai",
		APIEndpoint: srv.URL,
		APIKey:      "test-key",
	}

	_, err := FetchModels(provider)
	if err == nil {
		t.Fatal("expected error for malformed response (missing object field), got nil")
	}
}

func TestModelsFetcher_HandlesEmptyList(t *testing.T) {
	srv := newEmptyTestServer()
	defer srv.Close()

	provider := Provider{
		ID:          "test-empty",
		Name:        "Empty",
		Type:        "openai",
		APIEndpoint: srv.URL,
		APIKey:      "test-key",
	}

	_, err := FetchModels(provider)
	if err == nil {
		t.Fatal("expected error for empty model list, got nil")
	}
}

func TestOpenAIModelsFetcher_FiltersByObjectField(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"data": []map[string]string{
				{"id": "gpt-4o", "object": "model"},
				{"id": "gpt-4o-mini", "object": "model"},
				{"id": "whisper-1", "object": "model"},
				{"id": "text-embedding-ada-002", "object": "model"},
				{"id": "dall-e-3", "object": "model"},
			},
		})
	}))
	defer srv.Close()

	provider := Provider{
		ID:          "test-filter",
		Name:        "Filter Test",
		Type:        "openai",
		APIEndpoint: srv.URL,
		APIKey:      "test-key",
	}

	models, err := FetchModels(provider)
	if err != nil {
		t.Fatalf("FetchModels failed: %v", err)
	}

	if len(models) != 5 {
		t.Fatalf("expected 5 models, got %d: %v", len(models), models)
	}
}

func TestFetchModels_IntegrationWithAPIEndpoint(t *testing.T) {
	setupConfigTestDB(t)

	expected := []string{"gpt-4o", "gpt-4o-mini"}
	srv := newOpenAIModelsTestServer(expected)
	defer srv.Close()

	if err := SaveAdminPassword("test1234"); err != nil {
		t.Fatalf("SaveAdminPassword: %v", err)
	}

	err := AddProvider(Provider{
		ID:          "integration-test",
		Name:        "Integration Test",
		Type:        "openai",
		APIEndpoint: srv.URL,
		APIKey:      "test-key",
		Models:      []string{},
	})
	if err != nil {
		t.Fatalf("AddProvider: %v", err)
	}

	cfg := GetConfig()
	var found bool
	for _, p := range cfg.Providers {
		if p.ID == "integration-test" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("provider not found in config")
	}
}

func TestHandleFetchModels_APIEndpoint(t *testing.T) {
	setupConfigTestDB(t)

	expected := []string{"gpt-4o", "gpt-4o-mini"}
	srv := newOpenAIModelsTestServer(expected)
	defer srv.Close()

	if err := SaveAdminPassword("test1234"); err != nil {
		t.Fatalf("SaveAdminPassword: %v", err)
	}

	if err := AddProvider(Provider{
		ID:          "fetch-test",
		Name:        "Fetch Test",
		Type:        "openai",
		APIEndpoint: srv.URL,
		APIKey:      "test-key",
		Models:      []string{},
	}); err != nil {
		t.Fatalf("AddProvider: %v", err)
	}

	mux := newTestMux("test-token")
	sessionToken, err := CreateSession()
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	body := bytes.NewBufferString(`{"provider_id":"fetch-test"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/admin/providers/fetch-models", body)
	req.AddCookie(&http.Cookie{Name: "amm_session", Value: sessionToken})
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp struct {
		Status string   `json:"status"`
		Models []string `json:"models"`
		Count  int      `json:"count"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp.Status != "ok" {
		t.Fatalf("expected status ok, got %q", resp.Status)
	}
	if resp.Count != 2 {
		t.Fatalf("expected 2 models, got %d: %v", resp.Count, resp.Models)
	}
}

func TestHandleFetchModels_RejectsMissingProvider(t *testing.T) {
	setupConfigTestDB(t)
	if err := SaveAdminPassword("test1234"); err != nil {
		t.Fatalf("SaveAdminPassword: %v", err)
	}

	mux := newTestMux("test-token")
	sessionToken, err := CreateSession()
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	body := bytes.NewBufferString(`{"provider_id":"nonexistent"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/admin/providers/fetch-models", body)
	req.AddCookie(&http.Cookie{Name: "amm_session", Value: sessionToken})
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestHandleFetchModels_RequiresAuth(t *testing.T) {
	setupConfigTestDB(t)
	mux := newTestMux("secret")
	body := bytes.NewBufferString(`{"provider_id":"test"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/admin/providers/fetch-models", body)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rr.Code)
	}
}

func TestGetModelListFetcher_ReturnsCorrectFetcher(t *testing.T) {
	openAI := getModelListFetcher("openai")
	if _, ok := openAI.(*openAIModelsFetcher); !ok {
		t.Fatalf("expected *openAIModelsFetcher for openai, got %T", openAI)
	}

	anthropic := getModelListFetcher("anthropic")
	if _, ok := anthropic.(*anthropicModelsFetcher); !ok {
		t.Fatalf("expected *anthropicModelsFetcher for anthropic, got %T", anthropic)
	}

	deepseek := getModelListFetcher("deepseek")
	if _, ok := deepseek.(*openAIModelsFetcher); !ok {
		t.Fatalf("expected *openAIModelsFetcher for deepseek, got %T", deepseek)
	}

	empty := getModelListFetcher("")
	if _, ok := empty.(*openAIModelsFetcher); !ok {
		t.Fatalf("expected *openAIModelsFetcher for empty type, got %T", empty)
	}
}

func TestAutoFillModels_UpdatesProviderInConfig(t *testing.T) {
	setupConfigTestDB(t)

	expected := []string{"gpt-4o", "gpt-4o-mini", "gpt-4-turbo"}
	srv := newOpenAIModelsTestServer(expected)
	defer srv.Close()

	if err := AddProvider(Provider{
		ID:          "autofill-test",
		Name:        "AutoFill Test",
		Type:        "openai",
		APIEndpoint: srv.URL,
		APIKey:      "test-key",
		Models:      []string{},
	}); err != nil {
		t.Fatalf("AddProvider: %v", err)
	}

	models, err := AutoFillModels(Provider{
		ID:          "autofill-test",
		Name:        "AutoFill Test",
		Type:        "openai",
		APIEndpoint: srv.URL,
		APIKey:      "test-key",
	})
	if err != nil {
		t.Fatalf("AutoFillModels: %v", err)
	}

	if len(models) != 3 {
		t.Fatalf("expected 3 models, got %d: %v", len(models), models)
	}

	cfg := GetConfig()
	var found bool
	for _, p := range cfg.Providers {
		if p.ID == "autofill-test" {
			found = true
			if len(p.Models) != 3 {
				t.Fatalf("expected provider to have 3 models, got %d: %v", len(p.Models), p.Models)
			}
			break
		}
	}
	if !found {
		t.Fatal("provider not found in config")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && containsStr(s, substr)
}

func containsStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestFetchNonStandardProvider(t *testing.T) {
	expected := []string{"deepseek-chat", "deepseek-reasoner"}
	srv := newOpenAIModelsTestServer(expected)
	defer srv.Close()

	provider := Provider{
		ID:          "test-deepseek",
		Name:        "Test DeepSeek",
		Type:        "deepseek",
		APIEndpoint: srv.URL,
		APIKey:      "test-key",
	}

	models, err := FetchModels(provider)
	if err != nil {
		t.Fatalf("FetchModels for deepseek failed: %v", err)
	}

	if len(models) != 2 {
		t.Fatalf("expected 2 models, got %d: %v", len(models), models)
	}
}

func TestAnthropicModelsFetcher_HandlesNonModelEntries(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"data": []map[string]string{
				{"type": "model", "name": "claude-sonnet-4-6"},
				{"type": "model", "name": "claude-opus-4-6"},
				{"type": "model", "name": "claude-haiku-3-5"},
			},
		})
	}))
	defer srv.Close()

	provider := Provider{
		ID:          "test-anthropic-filter",
		Name:        "Anthropic Filter",
		Type:        "anthropic",
		APIEndpoint: srv.URL,
		APIKey:      "sk-ant-test",
	}

	models, err := FetchModels(provider)
	if err != nil {
		t.Fatalf("FetchModels failed: %v", err)
	}

	if len(models) != 3 {
		t.Fatalf("expected 3 models, got %d: %v", len(models), models)
	}
}

func TestModelsFetcher_TrailingSlashEndpoint(t *testing.T) {
	expected := []string{"gpt-4o"}
	srv := newOpenAIModelsTestServer(expected)
	defer srv.Close()

	provider := Provider{
		ID:          "test-trailing",
		Name:        "Trailing Slash",
		Type:        "openai",
		APIEndpoint: srv.URL + "/",
		APIKey:      "test-key",
	}

	models, err := FetchModels(provider)
	if err != nil {
		t.Fatalf("FetchModels with trailing slash failed: %v", err)
	}
	if len(models) != 1 {
		t.Fatalf("expected 1 model, got %d", len(models))
	}
}
