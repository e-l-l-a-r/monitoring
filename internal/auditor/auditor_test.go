package auditor

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockObserver struct {
	baseObserver
	received *AuditData
}

func (m *mockObserver) update(data *AuditData) error {
	m.received = data
	return nil
}

func TestAuditor(t *testing.T) {
	a := NewAuditor()
	obs := &mockObserver{baseObserver: baseObserver{id: "mock"}}

	t.Run("Register", func(t *testing.T) {
		a.Register(obs)
		assert.Contains(t, a.observers, "mock")
	})

	t.Run("Notify", func(t *testing.T) {
		data := NewAuditData([]string{"m1"}, "127.0.0.1")
		a.Notify(&data)
		assert.NotNil(t, obs.received)
		assert.Equal(t, data.Metrics, obs.received.Metrics)
	})

	t.Run("Deregister", func(t *testing.T) {
		a.Deregister(obs)
		assert.NotContains(t, a.observers, "mock")
	})
}

func TestWithPublisher(t *testing.T) {
	a := NewAuditor()
	mw := WithPublisher(a)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pub, ok := FromContext(r.Context())
		assert.True(t, ok)
		assert.Equal(t, a, pub)
	})

	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	mw(handler).ServeHTTP(rec, req)
}

func TestFileAuditor(t *testing.T) {
	tmpFile := "test_audit.txt"
	defer os.Remove(tmpFile)

	fa := NewFileAuditor(tmpFile)
	assert.Equal(t, "File:"+tmpFile, fa.getID())

	data := NewAuditData([]string{"test_metric"}, "1.1.1.1")
	err := fa.update(&data)
	require.NoError(t, err)

	content, err := os.ReadFile(tmpFile)
	require.NoError(t, err)
	assert.Contains(t, string(content), "test_metric")
	assert.Contains(t, string(content), "1.1.1.1")
}

func TestURLAuditor(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if strings.Contains(string(body), "test_metric") {
			w.WriteHeader(http.StatusOK)
		} else {
			w.WriteHeader(http.StatusBadRequest)
		}
	}))
	defer ts.Close()

	ua := NewURLAuditor(ts.URL)
	assert.Equal(t, "Url:"+ts.URL, ua.getID())

	data := &AuditData{TS: 123, Metrics: []string{"test_metric"}, IPAddress: "1.1.1.1"}
	ua.baseObserver.prepareData(data)
	err := ua.update(data)
	assert.NoError(t, err)
}

func TestBaseObserver_PrepareData(t *testing.T) {
	bo := &baseObserver{}
	data := &AuditData{TS: 123, Metrics: []string{"m1"}, IPAddress: "127.0.0.1"}
	err := bo.prepareData(data)
	assert.NoError(t, err)
	assert.JSONEq(t, `{"ts":123, "metrics":["m1"], "ip_address":"127.0.0.1"}`, bo.strData)
}
