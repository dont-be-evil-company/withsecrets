package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestEmbeddedPortalFiles(t *testing.T) {
	h := portalStatic()
	for _, path := range []string{"/", "/account", "/server", "/auth/login"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		body := rec.Body.String()
		if rec.Code != http.StatusOK || !strings.Contains(body, "/_app/immutable/") || !strings.Contains(body, "withsecrets") {
			t.Fatalf("%s %d", path, rec.Code)
		}
	}
}
