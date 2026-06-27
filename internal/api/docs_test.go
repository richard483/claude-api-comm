package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDocsPageServed(t *testing.T) {
	r := NewRouter(&fakeSvc{})
	req := httptest.NewRequest(http.MethodGet, "/docs", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Errorf("Content-Type = %q, want text/html", ct)
	}
	body := w.Body.String()
	if !strings.Contains(strings.ToLower(body), "swagger") {
		t.Error("docs page does not reference swagger ui")
	}
	if !strings.Contains(body, "openapi.yaml") {
		t.Error("docs page does not point at openapi.yaml")
	}
}

func TestOpenAPISpecServed(t *testing.T) {
	r := NewRouter(&fakeSvc{})
	req := httptest.NewRequest(http.MethodGet, "/openapi.yaml", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/yaml" {
		t.Errorf("Content-Type = %q, want application/yaml", ct)
	}
	if w.Body.Len() == 0 {
		t.Fatal("empty spec body")
	}
}

// TestEmbeddedSpecLooksValid sanity-checks the embedded OpenAPI document without pulling in a
// YAML dependency: it must declare OpenAPI 3, have a paths section, and document the core routes.
func TestEmbeddedSpecLooksValid(t *testing.T) {
	spec := string(openapiSpec)
	mustContain := []string{
		"openapi: 3.",
		"paths:",
		"/sessions",
		"/sessions/{id}/messages",
		"/sessions/{id}/turns/{turnID}/stream",
		"/sessions/{id}/summarize",
		"/health/ready",
	}
	for _, s := range mustContain {
		if !strings.Contains(spec, s) {
			t.Errorf("embedded openapi.yaml missing %q", s)
		}
	}
}
