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
	path string) (*http.Response, string) {
	req, err := http.NewRequest(method, ts.URL+path, nil)
	require.NoError(t, err)

	resp, err := ts.Client().Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	return resp, string(respBody)
}

func TestUpdateHandler_InvalidPath(t *testing.T) {
	ts := httptest.NewServer(GetRouter(repository.NewMemStorage(300, "router_testmetrics.json"), auditor.NewAuditor()))
	defer ts.Close()

	tests := []struct {
		name       string
		path       string
		expected   int
		errMessage string
	}{
		//{"Empty path", "/update/", http.StatusBadRequest, "Incorrect API"},
		{"Missing metric name", "/update/counter/", http.StatusNotFound, "No metric name"},
		{"Missing value", "/update/counter/test_metric/", http.StatusBadRequest, "No value"},
		{"Invalid value", "/update/counter/test_metric/abc", http.StatusBadRequest, "Incorrect value"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, get := testRequest(t, ts, http.MethodPost, tt.path)

			assert.Equal(t, tt.expected, resp.StatusCode)
			assert.Equal(t, tt.errMessage+"\n", get)

		})
	}
}

func TestUpdateHandler_ValidRequest(t *testing.T) {
	ts := httptest.NewServer(GetRouter(repository.NewMemStorage(300, "router_testmetrics.json"), auditor.NewAuditor()))
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
		{"Counter metric", http.MethodPost,
			"/update/counter/test_counter/123", "", "counter", "test_counter", 123},
		{"Gauge metric", http.MethodPost,
			"/update/gauge/test_gauge/67.89", "", "gauge", "test_gauge", 67.89},
		{"Get counter metric", http.MethodGet,
			"/value/counter/test_counter", "123", "counter", "test_counter", 123},
		{"Get gauge metric", http.MethodGet,
			"/value/gauge/test_gauge", "67.89", "gauge", "test_gauge", 67.89},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, get := testRequest(t, ts, tt.method, tt.path)

			assert.Equal(t, http.StatusOK, resp.StatusCode)
			assert.Equal(t, tt.result, get)
		})
	}
}
