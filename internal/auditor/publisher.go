package auditor

import (
	"context"
	"net/http"
)

// Publisher определяет интерфейс для управления наблюдателями и уведомления их о событиях аудита.
type Publisher interface {
	// Register регистрирует нового наблюдателя.
	Register(observer)
	// Deregister удаляет наблюдателя из списка.
	Deregister(observer)
	// Notify уведомляет всех зарегистрированных наблюдателей о событии.
	Notify(*AuditData)
}

type auditor struct {
	observers map[string]observer
}

func (a *auditor) Register(o observer) {
	if a.observers == nil {
		a.observers = make(map[string]observer)
	}
	a.observers[o.getID()] = o
}

func (a *auditor) Deregister(o observer) {
	delete(a.observers, o.getID())
}

func (a *auditor) Notify(data *AuditData) {
	for _, observer := range a.observers {
		observer.update(data)
	}
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
