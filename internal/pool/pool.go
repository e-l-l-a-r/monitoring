// Package pool предоставляет типобезопасный пул переиспользуемых объектов,
// каждый из которых умеет сбрасывать своё состояние методом Reset().
package pool

import "sync"

// Resetter описывает объект, способный сбросить своё состояние к начальным значениям.
type Resetter interface {
	Reset()
}

// Pool — пул объектов одного конкретного типа T.
type Pool[T Resetter] struct {
	items []T
	mu    sync.Mutex
}

// New создаёт пустой пул объектов типа T и возвращает указатель на него.
func New[T Resetter]() *Pool[T] {
	return &Pool[T]{}
}

// Get извлекает объект из пула, сбрасывает его состояние вызовом Reset() и возвращает вызывающему.
// Второе возвращаемое значение равно false, если пул пуст; в этом случае первым значением
// возвращается нулевое значение типа T.
func (p *Pool[T]) Get() (T, bool) {
	p.mu.Lock()

	if len(p.items) == 0 {
		p.mu.Unlock()

		var zero T
		return zero, false
	}

	last := len(p.items) - 1
	obj := p.items[last]

	var zero T
	p.items[last] = zero
	p.items = p.items[:last]

	p.mu.Unlock()

	obj.Reset()
	return obj, true
}

// Put помещает объект в пул для последующего переиспользования.
func (p *Pool[T]) Put(obj T) {
	p.mu.Lock()
	p.items = append(p.items, obj)
	p.mu.Unlock()
}
