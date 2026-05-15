package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func newTestMux(token string) *http.ServeMux {
	mux := http.NewServeMux()
	registerAPIRoutes(mux, token)
	return mux
}

func TestAdminAuthRequiresConfiguredToken(t *testing.T) {
	mux := newTestMux("")
	req := httptest.NewRequest(http.MethodGet, "/api/admin/config", nil)
	rr := httptest.NewRecorder()

	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected status %d, got %d", http.StatusUnauthorized, rr.Code)
	}
}

func TestAdminAuthRejectsMissingAndInvalidToken(t *testing.T) {
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
