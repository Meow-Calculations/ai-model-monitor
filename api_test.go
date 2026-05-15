package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
)

func newTestMux(token string) *http.ServeMux {
	mux := http.NewServeMux()
	registerAPIRoutes(mux, token)
	return mux
}

func TestAdminAuthRequiresSetupFirst(t *testing.T) {
	setupConfigTestDB(t)
	mux := newTestMux("secret")
	req := httptest.NewRequest(http.MethodGet, "/api/admin/config", nil)
	rr := httptest.NewRecorder()

	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected status %d, got %d", http.StatusUnauthorized, rr.Code)
	}
}

func TestAdminAuthRejectsMissingAndInvalidToken(t *testing.T) {
	setupConfigTestDB(t)
	if err := SaveAdminPassword("password123"); err != nil {
		t.Fatalf("SaveAdminPassword: %v", err)
	}
	mux := newTestMux("secret")

	for _, tc := range []struct {
		name string
		req  *http.Request
	}{
		{name: "missing", req: httptest.NewRequest(http.MethodGet, "/api/admin/config", nil)},
		{name: "invalid bearer", req: func() *http.Request {
			req := httptest.NewRequest(http.MethodGet, "/api/admin/config", nil)
			req.Header.Set("Authorization", "Bearer wrong")
			return req
		}()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rr := httptest.NewRecorder()
			mux.ServeHTTP(rr, tc.req)
			if rr.Code != http.StatusUnauthorized {
				t.Fatalf("expected status %d, got %d", http.StatusUnauthorized, rr.Code)
			}
		})
	}
}

func TestAdminAuthAcceptsBearerAndAPIKey(t *testing.T) {
	setupConfigTestDB(t)
	if err := SaveAdminPassword("password123"); err != nil {
		t.Fatalf("SaveAdminPassword: %v", err)
	}
	mux := newTestMux("secret")

	for _, tc := range []struct {
		name   string
		header string
		value  string
	}{
		{name: "bearer", header: "Authorization", value: "Bearer secret"},
		{name: "api key", header: "X-API-Key", value: "secret"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/admin/status", nil)
			req.Header.Set(tc.header, tc.value)
			rr := httptest.NewRecorder()

			mux.ServeHTTP(rr, req)

			if rr.Code != http.StatusOK {
				t.Fatalf("expected status %d, got %d", http.StatusOK, rr.Code)
			}
		})
	}
}

func TestLegacyManagementRoutesRequireAdminAuth(t *testing.T) {
	setupConfigTestDB(t)
	if err := SaveAdminPassword("password123"); err != nil {
		t.Fatalf("SaveAdminPassword: %v", err)
	}
	mux := newTestMux("secret")

	for _, path := range []string{"/api/config", "/api/providers", "/api/probe"} {
		t.Run(path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, path, nil)
			if path == "/api/probe" {
				req = httptest.NewRequest(http.MethodPost, path, nil)
			}
			rr := httptest.NewRecorder()

			mux.ServeHTTP(rr, req)

			if rr.Code != http.StatusUnauthorized {
				t.Fatalf("expected status %d, got %d", http.StatusUnauthorized, rr.Code)
			}
		})
	}
}

func TestPublicStatusRemainsCompatible(t *testing.T) {
	mux := newTestMux("secret")
	req := httptest.NewRequest(http.MethodGet, "/api/status", nil)
	rr := httptest.NewRecorder()

	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rr.Code)
	}
}

func TestPublicHistoryRemainsCompatible(t *testing.T) {
	mux := newTestMux("secret")
	req := httptest.NewRequest(http.MethodGet, "/api/history?key=invalid", nil)
	rr := httptest.NewRecorder()

	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rr.Code)
	}
}

func TestSetupAndLoginFlow(t *testing.T) {
	setupConfigTestDB(t)
	mux := newTestMux("secret")

	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/setup-status", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("setup status: expected %d, got %d", http.StatusOK, rr.Code)
	}
	if !bytes.Contains(rr.Body.Bytes(), []byte("true")) {
		t.Fatalf("expected setup to be required, got %s", rr.Body.String())
	}

	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/api/setup", bytes.NewBufferString(`{"password":"password123"}`)))
	if rr.Code != http.StatusOK {
		t.Fatalf("setup: expected %d, got %d: %s", http.StatusOK, rr.Code, rr.Body.String())
	}
	cookie := rr.Result().Cookies()[0]
	if cookie.Name != "amm_session" || cookie.Value == "" {
		t.Fatalf("expected session cookie, got %#v", cookie)
	}

	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/api/setup", bytes.NewBufferString(`{"password":"password123"}`)))
	if rr.Code != http.StatusConflict {
		t.Fatalf("repeat setup: expected %d, got %d", http.StatusConflict, rr.Code)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/admin/status", nil)
	req.AddCookie(cookie)
	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("session auth: expected %d, got %d", http.StatusOK, rr.Code)
	}

	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/api/login", bytes.NewBufferString(`{"password":"wrong"}`)))
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("bad login: expected %d, got %d", http.StatusUnauthorized, rr.Code)
	}

	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/api/login", bytes.NewBufferString(`{"password":"password123"}`)))
	if rr.Code != http.StatusOK {
		t.Fatalf("login: expected %d, got %d", http.StatusOK, rr.Code)
	}
}
