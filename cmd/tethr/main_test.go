package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHealth(t *testing.T) {
	tests := []struct {
		method string
		want   int
	}{
		{http.MethodGet, http.StatusOK},
		{http.MethodHead, http.StatusOK},
		{http.MethodPost, http.StatusMethodNotAllowed},
	}
	for _, tt := range tests {
		t.Run(tt.method, func(t *testing.T) {
			rec := httptest.NewRecorder()
			routes(http.NotFoundHandler()).ServeHTTP(rec, httptest.NewRequest(tt.method, "/healthz", nil))
			if rec.Code != tt.want {
				t.Errorf("%s /healthz = %d, want %d", tt.method, rec.Code, tt.want)
			}
		})
	}
}

func TestUnknownPath(t *testing.T) {
	rec := httptest.NewRecorder()
	routes(http.NotFoundHandler()).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/nope", nil))
	if rec.Code != http.StatusNotFound {
		t.Errorf("GET /nope = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestRoot(t *testing.T) {
	rec := httptest.NewRecorder()
	routes(http.NotFoundHandler()).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET / = %d, want %d", rec.Code, http.StatusOK)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	var got struct{ Name, Version string }
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Name != "tethr" || got.Version != version {
		t.Errorf("GET / = %+v, want name tethr and version %q", got, version)
	}
}

func TestDocs(t *testing.T) {
	tests := []struct{ path, contentType, contains string }{
		{"/openapi.yaml", "application/yaml; charset=utf-8", "openapi: 3.0.3"},
		{"/docs", "text/html; charset=utf-8", "/openapi.yaml"},
	}
	for _, tt := range tests {
		rec := httptest.NewRecorder()
		routes(http.NotFoundHandler()).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, tt.path, nil))
		if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != tt.contentType || !strings.Contains(rec.Body.String(), tt.contains) {
			t.Errorf("GET %s = %d %q", tt.path, rec.Code, rec.Header().Get("Content-Type"))
		}
	}
}
