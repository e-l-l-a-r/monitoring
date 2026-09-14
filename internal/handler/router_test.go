// Package handler содержит тесты для HTTP-обработчиков.
package handler

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/e-l-l-a-r/monitoring/internal/auditor"
	"github.com/e-l-l-a-r/monitoring/internal/repository"
)

func testRequest(t *testing.T, ts *httptest.Server, method,
	path string) (int, string) {
	req, err := http.NewRequest(method, ts.URL+path, nil)
	require.NoError(t, err)

	resp, err := ts.Client().Do(req)
	require.NoError(t, err)
	defer func() {
		_ = resp.Body.Close()
	}()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	return resp.StatusCode, string(respBody)
}

func TestUpdateHandler_InvalidPath(t *testing.T) {
	audit := auditor.NewAuditor()
	defer audit.Close()

	ts := httptest.NewServer(GetRouter(repository.NewMemStorage(300, "router_testmetrics.json"), audit))
	defer ts.Close()

	tests := []struct {
		name       string
		path       string
		errMessage string
		expected   int
	}{
		//{"Empty path", "/update/", http.StatusBadRequest, "Incorrect API"},
		{"Отсутствует имя метрики", "/update/counter/", "No metric name", http.StatusNotFound},
		{"Отсутствует значение", "/update/counter/test_metric/", "No value", http.StatusBadRequest},
		{"Некорректное значение", "/update/counter/test_metric/abc", "Incorrect value", http.StatusBadRequest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			statusCode, get := testRequest(t, ts, http.MethodPost, tt.path)

			assert.Equal(t, tt.expected, statusCode)
			assert.Equal(t, tt.errMessage+"\n", get)

		})
	}
}

func TestUpdateHandler_ValidRequest(t *testing.T) {
	audit := auditor.NewAuditor()
	defer audit.Close()

	ts := httptest.NewServer(GetRouter(repository.NewMemStorage(300, "router_testmetrics.json"), audit))
	defer ts.Close()
	tests := []struct {
		name   string
		method string
		path   string
		result string
		mType  string
		mName  string
		mValue float64
	}{
		{"Метрика Counter", http.MethodPost,
			"/update/counter/test_counter/123", "", "counter", "test_counter", 123},
		{"Метрика Gauge", http.MethodPost,
			"/update/gauge/test_gauge/67.89", "", "gauge", "test_gauge", 67.89},
		{"Получение метрики counter", http.MethodGet,
			"/value/counter/test_counter", "123", "counter", "test_counter", 123},
		{"Получение метрики gauge", http.MethodGet,
			"/value/gauge/test_gauge", "67.89", "gauge", "test_gauge", 67.89},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			statusCode, get := testRequest(t, ts, tt.method, tt.path)

			assert.Equal(t, http.StatusOK, statusCode)
			assert.Equal(t, tt.result, get)
		})
	}
}

func TestRequestIP(t *testing.T) {
	tests := []struct {
		name       string
		remoteAddr string
		want       string
	}{
		{name: "ipv4 с портом", remoteAddr: "192.168.0.42:1234", want: "192.168.0.42"},
		{name: "ipv6 с портом", remoteAddr: "[2001:db8::1]:1234", want: "2001:db8::1"},
		{name: "без порта", remoteAddr: "192.168.0.42", want: "192.168.0.42"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.RemoteAddr = tt.remoteAddr

			assert.Equal(t, tt.want, requestIP(req))
		})
	}
}
