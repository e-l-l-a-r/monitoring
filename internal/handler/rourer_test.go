package handler

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func performRequest(r http.Handler, method, url string, body io.Reader) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, url, body)
	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, req)
	return resp
}

func TestUpdateHandler_MethodNotAllowed(t *testing.T) {
	handler := NewRouter().GetMux()

	resp := performRequest(handler, http.MethodGet, "/update/counter/test_metric/123", nil)

	assert.Equal(t, http.StatusMethodNotAllowed, resp.Code)
	assert.Equal(t, "Only POST requests are allowed!\n", resp.Body.String())
}

func TestUpdateHandler_InvalidPath(t *testing.T) {
	handler := NewRouter().GetMux()

	tests := []struct {
		name       string
		path       string
		expected   int
		errMessage string
	}{
		{"Empty path", "/update/", http.StatusBadRequest, "Incorrect API"},
		{"Missing metric name", "/update/counter/", http.StatusNotFound, "No metric name"},
		{"Missing value", "/update/counter/test_metric/", http.StatusBadRequest, "No value"},
		{"Invalid value", "/update/counter/test_metric/abc", http.StatusBadRequest, "Incorrect value"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := performRequest(handler, http.MethodPost, tt.path, nil)

			assert.Equal(t, tt.expected, resp.Code)
			assert.Equal(t, tt.errMessage+"\n", resp.Body.String())
		})
	}
}

func TestUpdateHandler_ValidRequest(t *testing.T) {
	handler := NewRouter().GetMux()
	tests := []struct {
		name   string
		path   string
		mType  string
		mName  string
		mValue float64
	}{
		{"Counter metric", "/update/counter/test_counter/123.45", "counter", "test_counter", 123.45},
		{"Gauge metric", "/update/gauge/test_gauge/67.89", "gauge", "test_gauge", 67.89},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := performRequest(handler, http.MethodPost, tt.path, nil)

			assert.Equal(t, http.StatusOK, resp.Code)

			// Проверяем, что метрика была добавлена
			metric, ok := storage.Metrics[tt.mName]

			assert.True(t, ok, "Метрика '%s' не найдена в хранилище", tt.mName)
			assert.Equal(t, tt.mType, metric.MType, "Неверный тип метрики")
			assert.Equal(t, tt.mValue, *metric.Value)

		})
	}
}
