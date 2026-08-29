package auditor

import (
	"context"
	"net/http"
)

type Publisher interface {
	Register(observer)
	Deregister(observer)
	Notify(*auditData)
}

type auditor struct {
	observers map[string]observer
}

func (a *auditor) Register(o observer) {
	if a.observers == nil {
		a.observers = make(map[string]observer)
	}
	a.observers[o.getId()] = o
}

func (a *auditor) Deregister(o observer) {
	delete(a.observers, o.getId())
}

func (a *auditor) Notify(data *auditData) {
	for _, observer := range a.observers {
		observer.update(data)
	}
}

func NewAuditor() *auditor {
	return &auditor{}
}

type auditorKey struct{}

func WithPublisher(p Publisher) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := context.WithValue(r.Context(), auditorKey{}, p)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func FromContext(ctx context.Context) (auditor, bool) {
	a, ok := ctx.Value(auditorKey{}).(*auditor)
	return *a, ok
}
