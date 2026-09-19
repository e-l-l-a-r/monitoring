package auditor

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type slowObserver struct {
	baseObserver
	started chan struct{}
	release chan struct{}
	once    sync.Once
}

func (s *slowObserver) update(_ *AuditData) error {
	s.once.Do(func() {
		close(s.started)
	})
	<-s.release
	return nil
}

type mockObserver struct {
	baseObserver
	received chan *AuditData
}

func (m *mockObserver) update(data *AuditData) error {
	copied := &AuditData{
		TS:        data.TS,
		IPAddress: data.IPAddress,
		Metrics:   append([]string(nil), data.Metrics...),
	}
	m.received <- copied
	return nil
}

func TestAuditor(t *testing.T) {
	a := NewAuditor()
	defer a.Close()

	obs := &mockObserver{
		baseObserver: baseObserver{id: "mock"},
		received:     make(chan *AuditData, 1),
	}

	t.Run("Register", func(t *testing.T) {
		a.Register(obs)
		assert.Contains(t, a.observers, "mock")
	})

	t.Run("Notify", func(t *testing.T) {
		data := NewAuditData([]string{"m1"}, "127.0.0.1")
		err := a.Notify(&data)
		require.NoError(t, err)

		select {
		case received := <-obs.received:
			assert.Equal(t, data.Metrics, received.Metrics)
		case <-time.After(time.Second):
			t.Fatal("observer did not receive audit data")
		}
	})

	t.Run("Deregister", func(t *testing.T) {
		a.Deregister(obs)
		assert.NotContains(t, a.observers, "mock")
	})
}

func TestAuditorNotifyDoesNotBlockOnSlowObserver(t *testing.T) {
	a := NewAuditor()
	defer a.Close()

	slow := &slowObserver{
		baseObserver: baseObserver{id: "slow"},
		started:      make(chan struct{}),
		release:      make(chan struct{}),
	}
	defer close(slow.release)
	fast := &mockObserver{
		baseObserver: baseObserver{id: "fast"},
		received:     make(chan *AuditData, 1),
	}

	a.Register(slow)
	a.Register(fast)

	data := NewAuditData([]string{"m1"}, "127.0.0.1")
	require.NoError(t, a.Notify(&data))

	select {
	case <-slow.started:
	case <-time.After(time.Second):
		t.Fatal("slow observer did not start")
	}

	require.NoError(t, a.Notify(&data))

	select {
	case received := <-fast.received:
		assert.Equal(t, data.Metrics, received.Metrics)
	case <-time.After(time.Second):
		t.Fatal("fast observer was blocked by slow observer")
	}
}

func TestAuditorReturnsEventsToPool(t *testing.T) {
	a := NewAuditor()
	defer a.Close()

	obs := &mockObserver{
		baseObserver: baseObserver{id: "mock"},
		received:     make(chan *AuditData, 1),
	}
	a.Register(obs)

	data := NewAuditData([]string{"m1"}, "127.0.0.1")
	require.NoError(t, a.Notify(&data))

	select {
	case <-obs.received:
	case <-time.After(time.Second):
		t.Fatal("observer did not receive audit data")
	}

	var event *AuditData
	require.Eventually(t, func() bool {
		var ok bool
		event, ok = a.events.Get()
		return ok
	}, time.Second, 10*time.Millisecond, "обработанное событие должно вернуться в пул")

	assert.Empty(t, event.Metrics, "Get() сбрасывает состояние события")
	assert.Empty(t, event.IPAddress)
	assert.Zero(t, event.TS)
	assert.Positive(t, cap(event.Metrics), "backing-массив имён метрик сохраняется для переиспользования")
}

func TestAuditorGivesEachObserverOwnEvent(t *testing.T) {
	a := NewAuditor()
	defer a.Close()

	first := &mockObserver{
		baseObserver: baseObserver{id: "first"},
		received:     make(chan *AuditData, 1),
	}
	second := &mockObserver{
		baseObserver: baseObserver{id: "second"},
		received:     make(chan *AuditData, 1),
	}
	a.Register(first)
	a.Register(second)

	data := NewAuditData([]string{"m1"}, "127.0.0.1")
	require.NoError(t, a.Notify(&data))

	for _, obs := range []*mockObserver{first, second} {
		select {
		case received := <-obs.received:
			assert.Equal(t, data.Metrics, received.Metrics)
			assert.Equal(t, data.IPAddress, received.IPAddress)
			assert.Equal(t, data.TS, received.TS)
		case <-time.After(time.Second):
			t.Fatalf("observer %s did not receive audit data", obs.getID())
		}
	}
}

func TestAuditorReusesPooledEvent(t *testing.T) {
	a := NewAuditor()
	defer a.Close()

	obs := &mockObserver{
		baseObserver: baseObserver{id: "mock"},
		received:     make(chan *AuditData, 1),
	}
	a.Register(obs)

	pooled := &AuditData{Metrics: make([]string, 0, 8)}
	a.events.Put(pooled)

	data := NewAuditData([]string{"m1", "m2"}, "127.0.0.1")
	require.NoError(t, a.Notify(&data))

	select {
	case received := <-obs.received:
		assert.Equal(t, data.Metrics, received.Metrics)
	case <-time.After(time.Second):
		t.Fatal("observer did not receive audit data")
	}

	var event *AuditData
	require.Eventually(t, func() bool {
		var ok bool
		event, ok = a.events.Get()
		return ok
	}, time.Second, 10*time.Millisecond)

	assert.Same(t, pooled, event, "Notify должен был взять готовый объект из пула, а не аллоцировать новый")
	assert.Equal(t, 8, cap(event.Metrics), "емкость слайса имён переиспользована без аллокации")
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
	tmpFile := filepath.Join(t.TempDir(), "test_audit.txt")

	fa, err := NewFileAuditor(tmpFile)
	require.NoError(t, err)
	defer func() {
		assert.NoError(t, fa.Close())
	}()
	assert.Equal(t, "File:"+tmpFile, fa.getID())

	data := NewAuditData([]string{"test_metric"}, "1.1.1.1")
	err = fa.update(&data)
	require.NoError(t, err)

	content, err := os.ReadFile(tmpFile)
	require.NoError(t, err)
	assert.Contains(t, string(content), "test_metric")
	assert.Contains(t, string(content), "1.1.1.1")
}

func TestFileAuditorConcurrentUpdate(t *testing.T) {
	tmpFile := filepath.Join(t.TempDir(), "test_audit.txt")

	fa, err := NewFileAuditor(tmpFile)
	require.NoError(t, err)
	defer func() {
		assert.NoError(t, fa.Close())
	}()

	const updatesCount = 20
	var wg sync.WaitGroup
	wg.Add(updatesCount)

	for i := 0; i < updatesCount; i++ {
		go func() {
			defer wg.Done()

			data := NewAuditData([]string{"test_metric"}, "1.1.1.1")
			assert.NoError(t, fa.update(&data))
		}()
	}
	wg.Wait()

	content, err := os.ReadFile(tmpFile)
	require.NoError(t, err)
	assert.Len(t, strings.Split(strings.TrimSpace(string(content)), "\n"), updatesCount)
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
	err := ua.update(data)
	assert.NoError(t, err)
}

func TestURLAuditorRetriesRetriableStatus(t *testing.T) {
	attempts := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		body, _ := io.ReadAll(r.Body)
		assert.Contains(t, string(body), "test_metric")

		if attempts == 1 {
			http.Error(w, "temporary error", http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	ua := NewURLAuditor(ts.URL)
	data := &AuditData{TS: 123, Metrics: []string{"test_metric"}, IPAddress: "1.1.1.1"}

	err := ua.update(data)
	require.NoError(t, err)
	assert.Equal(t, 2, attempts)
}

func TestBaseObserver_PrepareData(t *testing.T) {
	bo := &baseObserver{}
	data := &AuditData{TS: 123, Metrics: []string{"m1"}, IPAddress: "127.0.0.1"}
	strData, err := bo.prepareData(data)
	assert.NoError(t, err)
	assert.JSONEq(t, `{"ts":123, "metrics":["m1"], "ip_address":"127.0.0.1"}`, strData)
}
