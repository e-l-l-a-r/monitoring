package auditor

import (
	"context"
	"errors"
	"net/http"
	"sync"

	"github.com/e-l-l-a-r/monitoring/internal/logger"
)

const observerQueueSize = 100

var ErrAuditorClosed = errors.New("auditor is closed")

// Publisher определяет интерфейс для управления наблюдателями и уведомления их о событиях аудита.
type Publisher interface {
	// Register регистрирует нового наблюдателя.
	Register(observer)
	// Deregister удаляет наблюдателя из списка.
	Deregister(observer)
	// Notify уведомляет всех зарегистрированных наблюдателей о событии.
	Notify(*AuditData) error
}

type observerWorker struct {
	observer observer
	tasks    chan AuditData
}

type auditor struct {
	observers map[string]*observerWorker
	mtx       sync.Mutex
	wg        sync.WaitGroup
	closed    bool
}

func (a *auditor) Register(o observer) {
	a.mtx.Lock()
	defer a.mtx.Unlock()

	if a.closed {
		return
	}
	if a.observers == nil {
		a.observers = make(map[string]*observerWorker)
	}

	id := o.getID()
	if existing, ok := a.observers[id]; ok {
		close(existing.tasks)
	}

	worker := &observerWorker{
		observer: o,
		tasks:    make(chan AuditData, observerQueueSize),
	}
	a.observers[id] = worker

	a.wg.Add(1)
	go a.runObserver(worker)
}

func (a *auditor) Deregister(o observer) {
	a.mtx.Lock()
	defer a.mtx.Unlock()

	if worker, ok := a.observers[o.getID()]; ok {
		close(worker.tasks)
		delete(a.observers, o.getID())
	}
}

func (a *auditor) Notify(data *AuditData) error {
	a.mtx.Lock()
	defer a.mtx.Unlock()

	if a.closed {
		return ErrAuditorClosed
	}

	for _, worker := range a.observers {
		select {
		case worker.tasks <- *data:
		default:
			logger.Warn("Audit observer queue is full, dropping event: ", worker.observer.getID())
		}
	}
	return nil
}

func (a *auditor) runObserver(worker *observerWorker) {
	defer a.wg.Done()

	for data := range worker.tasks {
		if err := worker.observer.update(&data); err != nil {
			logger.Warn("Audit observer error: ", err.Error())
		}
	}
}

func (a *auditor) Close() {
	a.mtx.Lock()
	if a.closed {
		a.mtx.Unlock()
		return
	}
	a.closed = true
	for _, worker := range a.observers {
		close(worker.tasks)
	}
	a.observers = nil
	a.mtx.Unlock()

	a.wg.Wait()
}

// NewAuditor создает новый экземпляр аудитора, реализующего интерфейс Publisher.
func NewAuditor() *auditor {
	return &auditor{}
}

type auditorKey struct{}

// WithPublisher — middleware для добавления объекта Publisher в контекст HTTP-запроса.
func WithPublisher(p Publisher) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := context.WithValue(r.Context(), auditorKey{}, p)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// FromContext извлекает аудитора из контекста. Возвращает аудитор и флаг успеха.
func FromContext(ctx context.Context) (*auditor, bool) {
	a, ok := ctx.Value(auditorKey{}).(*auditor)
	return a, ok
}
