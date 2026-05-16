package main

import "testing"

func setupConfigTestDB(t *testing.T) {
	t.Helper()
	ResetRateLimiter()
	if db != nil {
		CloseDB()
	}
	configMu.Lock()
	appConfig = nil
	configMu.Unlock()
	if err := InitDB(t.TempDir()); err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	t.Cleanup(func() {
		CloseDB()
		configMu.Lock()
		appConfig = nil
		configMu.Unlock()
	})
}

func TestAddProviderReplacesExistingCacheEntry(t *testing.T) {
	setupConfigTestDB(t)

	first := Provider{ID: "openai", Name: "OpenAI", Type: "openai", APIEndpoint: "https://api.openai.com/v1", APIKey: "first", Models: []string{"gpt-4o"}}
	second := Provider{ID: "openai", Name: "OpenAI Updated", Type: "openai", APIEndpoint: "https://api.openai.com/v1", APIKey: "second", Models: []string{"gpt-4o-mini"}}

	if err := AddProvider(first); err != nil {
		t.Fatalf("AddProvider first: %v", err)
	}
	if err := AddProvider(second); err != nil {
		t.Fatalf("AddProvider second: %v", err)
	}

	cfg := GetConfig()
	if got := len(cfg.Providers); got != 1 {
		t.Fatalf("expected one cached provider, got %d", got)
	}
	if got := cfg.Providers[0].Name; got != second.Name {
		t.Fatalf("expected cached provider name %q, got %q", second.Name, got)
	}
	if got := providerRowCount(t); got != 1 {
		t.Fatalf("expected one provider row, got %d", got)
	}
}

func TestProviderCRUDPreservesAndClearsAPIKey(t *testing.T) {
	setupConfigTestDB(t)

	provider := Provider{ID: "anthropic", Name: "Anthropic", Type: "anthropic", APIEndpoint: "https://api.anthropic.com", APIKey: "secret", Models: []string{"claude-sonnet-4-6"}}
	if err := AddProvider(provider); err != nil {
		t.Fatalf("AddProvider: %v", err)
	}

	updated := Provider{ID: "ignored", Name: "Anthropic Updated", Type: "anthropic", APIEndpoint: "https://api.anthropic.com", APIKey: maskedAPIKey, Models: []string{"claude-opus-4-6"}}
	if err := UpdateProvider("anthropic", updated); err != nil {
		t.Fatalf("UpdateProvider masked: %v", err)
	}

	cfg := GetConfig()
	if got := len(cfg.Providers); got != 1 {
		t.Fatalf("expected one provider after update, got %d", got)
	}
	if got := cfg.Providers[0].APIKey; got != "secret" {
		t.Fatalf("expected masked update to preserve api key, got %q", got)
	}
	if got := cfg.Providers[0].Models[0]; got != "claude-opus-4-6" {
		t.Fatalf("expected updated model, got %q", got)
	}

	updated.APIKey = ""
	if err := UpdateProvider("anthropic", updated); err != nil {
		t.Fatalf("UpdateProvider clear key: %v", err)
	}
	if got := GetConfig().Providers[0].APIKey; got != "" {
		t.Fatalf("expected empty api key after clear, got %q", got)
	}

	if err := RemoveProvider("anthropic"); err != nil {
		t.Fatalf("RemoveProvider: %v", err)
	}
	if got := len(GetConfig().Providers); got != 0 {
		t.Fatalf("expected no cached providers after remove, got %d", got)
	}
	if got := providerRowCount(t); got != 0 {
		t.Fatalf("expected no provider rows after remove, got %d", got)
	}
}

func TestAdminPasswordIsHashedAndVerified(t *testing.T) {
	setupConfigTestDB(t)

	if IsSetupComplete() {
		t.Fatal("expected setup to be incomplete")
	}
	if err := SaveAdminPassword("password123"); err != nil {
		t.Fatalf("SaveAdminPassword: %v", err)
	}
	if !IsSetupComplete() {
		t.Fatal("expected setup to be complete")
	}
	if !VerifyAdminPassword("password123") {
		t.Fatal("expected password to verify")
	}
	if VerifyAdminPassword("wrong") {
		t.Fatal("expected wrong password to fail")
	}
	var stored string
	if err := db.QueryRow("SELECT value FROM config WHERE key = ?", adminPasswordKey).Scan(&stored); err != nil {
		t.Fatalf("query password hash: %v", err)
	}
	if stored == "password123" {
		t.Fatal("password stored in plaintext")
	}
}

func TestSessionLifecycle(t *testing.T) {
	setupConfigTestDB(t)

	token, err := CreateSession()
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	if !ValidateSession(token) {
		t.Fatal("expected session to validate")
	}
	ClearSession(token)
	if ValidateSession(token) {
		t.Fatal("expected cleared session to fail")
	}
}

func providerRowCount(t *testing.T) int {
	t.Helper()
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM providers").Scan(&count); err != nil {
		t.Fatalf("count providers: %v", err)
	}
	return count
}
