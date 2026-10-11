package main

import (
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/e-l-l-a-r/monitoring/internal/logger"
)

func ipNet(t *testing.T, cidr string) net.Addr {
	t.Helper()
	ip, n, err := net.ParseCIDR(cidr)
	require.NoError(t, err)
	n.IP = ip
	return n
}

func TestHostIP(t *testing.T) {
	tests := []struct {
		name    string
		want    string
		addrs   []string
		wantErr bool
	}{
		{name: "пропускает loopback", addrs: []string{"127.0.0.1/8", "192.168.1.10/24"}, want: "192.168.1.10"},
		{name: "только loopback", addrs: []string{"127.0.0.1/8"}, wantErr: true},
		{name: "нет адресов", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			addrs := make([]net.Addr, 0, len(tt.addrs))
			for _, a := range tt.addrs {
				addrs = append(addrs, ipNet(t, a))
			}
			got, err := hostIP(addrs)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestSendDataSetsRealIP(t *testing.T) {
	tests := []struct {
		name   string
		realIP string
	}{
		{name: "адрес задан", realIP: "192.168.1.10"},
		{name: "адрес не определён - заголовка нет", realIP: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got []string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				got = r.Header.Values(realIPHeader)
				w.WriteHeader(http.StatusOK)
			}))
			defer srv.Close()

			log, err := logger.New("Error")
			require.NoError(t, err)
			s := &sender{client: srv.Client(), log: log, realIP: tt.realIP}

			require.NoError(t, s.sendData(srv.URL+"/update/", map[string]string{"id": "x"}))
			if tt.realIP == "" {
				assert.Empty(t, got)
			} else {
				assert.Equal(t, []string{tt.realIP}, got)
			}
		})
	}
}
