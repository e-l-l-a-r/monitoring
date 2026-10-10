package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/e-l-l-a-r/monitoring/internal/auditor"
	"github.com/e-l-l-a-r/monitoring/internal/repository"
)

func TestNewTrustedSubnet(t *testing.T) {
	tests := []struct {
		name    string
		cidr    string
		wantErr bool
	}{
		{name: "пустая подсеть", cidr: ""},
		{name: "IPv4", cidr: "192.168.1.0/24"},
		{name: "IPv6", cidr: "fd00::/8"},
		{name: "адрес вместо подсети", cidr: "192.168.1.1", wantErr: true},
		{name: "мусор", cidr: "not-a-cidr", wantErr: true},
		{name: "неверная маска", cidr: "10.0.0.0/33", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewTrustedSubnet(tt.cidr)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestTrustedSubnetHandle(t *testing.T) {
	tests := []struct {
		name     string
		cidr     string
		realIP   string
		noHeader bool
		want     int
	}{
		{name: "пустая подсеть, без заголовка", cidr: "", noHeader: true, want: http.StatusOK},
		{name: "пустая подсеть, чужой IP", cidr: "", realIP: "8.8.8.8", want: http.StatusOK},
		{name: "IP в подсети", cidr: "192.168.1.0/24", realIP: "192.168.1.15", want: http.StatusOK},
		{name: "подсеть задана адресом хоста", cidr: "192.168.1.7/24", realIP: "192.168.1.200", want: http.StatusOK},
		{name: "IP вне подсети", cidr: "192.168.1.0/24", realIP: "192.168.2.15", want: http.StatusForbidden},
		{name: "нет заголовка", cidr: "192.168.1.0/24", noHeader: true, want: http.StatusForbidden},
		{name: "некорректный IP", cidr: "192.168.1.0/24", realIP: "abc", want: http.StatusForbidden},
		{name: "IPv4-mapped IPv6 в подсети", cidr: "10.0.0.0/8", realIP: "::ffff:10.1.2.3", want: http.StatusOK},
		{name: "IPv6 в подсети", cidr: "fd00::/8", realIP: "fd12::1", want: http.StatusOK},
		{name: "IPv6 вне подсети", cidr: "fd00::/8", realIP: "2001:db8::1", want: http.StatusForbidden},
	}

	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ts, err := NewTrustedSubnet(tt.cidr)
			require.NoError(t, err)

			req := httptest.NewRequest(http.MethodPost, "/update/", nil)
			if !tt.noHeader {
				req.Header.Set(RealIPHeader, tt.realIP)
			}
			rec := httptest.NewRecorder()
			ts.Handle(next).ServeHTTP(rec, req)
			assert.Equal(t, tt.want, rec.Code)
		})
	}
}

func TestNilTrustedSubnetAllowsAll(t *testing.T) {
	var ts *TrustedSubnet
	assert.True(t, ts.Contains("1.2.3.4"))
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	rec := httptest.NewRecorder()
	ts.Handle(next).ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/", nil))
	assert.Equal(t, http.StatusOK, rec.Code)
}

// TestRouterTrustedSubnet проверяет, что ограничение применяется только к
// эндпоинтам приёма метрик, а чтение остаётся доступным.
func TestRouterTrustedSubnet(t *testing.T) {
	trusted, err := NewTrustedSubnet("10.0.0.0/24")
	require.NoError(t, err)

	audit := auditor.NewAuditor()
	defer audit.Close()
	router := GetRouter(repository.NewMemStorage(300, t.TempDir()+"/metrics.json"), audit, trusted)

	tests := []struct {
		name   string
		method string
		path   string
		body   string
		realIP string
		want   int
	}{
		{name: "update из подсети", method: http.MethodPost, path: "/update/gauge/g1/1.5", realIP: "10.0.0.5", want: http.StatusOK},
		{name: "update вне подсети", method: http.MethodPost, path: "/update/gauge/g1/1.5", realIP: "10.0.1.5", want: http.StatusForbidden},
		{name: "JSON update вне подсети", method: http.MethodPost, path: "/update/",
			body: `{"id":"g1","type":"gauge","value":1}`, realIP: "10.0.1.5", want: http.StatusForbidden},
		{name: "batch updates без заголовка", method: http.MethodPost, path: "/updates/",
			body: `[{"id":"g1","type":"gauge","value":1}]`, want: http.StatusForbidden},
		{name: "batch updates из подсети", method: http.MethodPost, path: "/updates/",
			body: `[{"id":"g1","type":"gauge","value":1}]`, realIP: "10.0.0.5", want: http.StatusOK},
		{name: "чтение значения вне подсети", method: http.MethodGet, path: "/value/gauge/g1", realIP: "10.0.1.5", want: http.StatusOK},
		{name: "список метрик без заголовка", method: http.MethodGet, path: "/", want: http.StatusOK},
	}

	// Подзапросы выполняются последовательно: чтение опирается на ранее записанную метрику.
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, strings.NewReader(tt.body))
			if tt.body != "" {
				req.Header.Set("Content-Type", "application/json")
			}
			if tt.realIP != "" {
				req.Header.Set(RealIPHeader, tt.realIP)
			}
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)
			assert.Equal(t, tt.want, rec.Code)
		})
	}
}
