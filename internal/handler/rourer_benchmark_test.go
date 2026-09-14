package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/e-l-l-a-r/monitoring/internal/auditor"
	"github.com/e-l-l-a-r/monitoring/internal/repository"
)

func BenchmarkRouter_Update(b *testing.B) {
	ms := repository.NewMemStorage(0, "")
	router := GetRouter(ms, auditor.NewAuditor())

	for b.Loop() {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/update/gauge/test_metric/1.23", nil)
		router.ServeHTTP(w, req)
	}
}

func BenchmarkRouter_Value(b *testing.B) {
	ms := repository.NewMemStorage(0, "")
	router := GetRouter(ms, auditor.NewAuditor())
	// Seed some data
	{
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/update/gauge/test_metric/1.23", nil)
		router.ServeHTTP(w, req)
	}

	for b.Loop() {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/value/gauge/test_metric", nil)
		router.ServeHTTP(w, req)
	}
}
