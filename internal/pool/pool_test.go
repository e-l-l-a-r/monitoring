package pool

import (
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/e-l-l-a-r/monitoring/internal/model"
)

// buffer — вспомогательный тип с методом Reset() для проверки работы пула.
type buffer struct {
	data   []byte
	resets int
}

// Reset сбрасывает состояние buffer и считает количество вызовов.
func (b *buffer) Reset() {
	b.data = b.data[:0]
	b.resets++
}

func TestPoolGetEmpty(t *testing.T) {
	p := New[*buffer]()

	obj, ok := p.Get()

	assert.False(t, ok)
	assert.Nil(t, obj)
}

func TestPoolPutGet(t *testing.T) {
	tests := []struct {
		name string
		data []byte
	}{
		{name: "пустой буфер", data: nil},
		{name: "заполненный буфер", data: []byte("metrics")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := New[*buffer]()
			src := &buffer{data: tt.data}

			p.Put(src)
			obj, ok := p.Get()

			require.True(t, ok)
			assert.Same(t, src, obj, "пул должен вернуть тот же объект")
			assert.Empty(t, obj.data, "Get() обязан сбросить состояние объекта")
			assert.Equal(t, 1, obj.resets, "Reset() вызывается ровно один раз при выдаче")
			assert.Equal(t, cap(tt.data), cap(obj.data), "ёмкость буфера сохраняется")
		})
	}
}

func TestPoolGetDrainsPool(t *testing.T) {
	p := New[*buffer]()
	p.Put(&buffer{})

	_, ok := p.Get()
	require.True(t, ok)

	_, ok = p.Get()
	assert.False(t, ok, "повторный Get() на опустевшем пуле возвращает false")
}

func TestPoolReturnsAllStoredObjects(t *testing.T) {
	p := New[*buffer]()
	stored := []*buffer{{}, {}, {}}
	for _, obj := range stored {
		p.Put(obj)
	}

	got := make(map[*buffer]bool, len(stored))
	for range stored {
		obj, ok := p.Get()
		require.True(t, ok)
		got[obj] = true
	}

	for _, obj := range stored {
		assert.True(t, got[obj], "каждый помещённый объект должен быть выдан пулом")
	}

	_, ok := p.Get()
	assert.False(t, ok)
}

func TestPoolWithGeneratedResetType(t *testing.T) {
	p := New[*model.Metrics]()
	delta := int64(42)
	p.Put(&model.Metrics{ID: "PollCount", MType: "counter", Delta: &delta, Hash: "hash"})

	obj, ok := p.Get()

	require.True(t, ok)
	assert.Empty(t, obj.ID)
	assert.Empty(t, obj.MType)
	assert.Empty(t, obj.Hash)
	require.NotNil(t, obj.Delta)
	assert.Equal(t, int64(0), *obj.Delta)
}

func TestPoolConcurrentAccess(t *testing.T) {
	const (
		workers = 16
		rounds  = 100
	)

	p := New[*buffer]()
	for i := 0; i < workers; i++ {
		p.Put(&buffer{data: make([]byte, 0, 8)})
	}

	var got atomic.Int64

	var wg sync.WaitGroup
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < rounds; j++ {
				obj, ok := p.Get()
				if !ok {
					continue
				}
				got.Add(1)
				obj.data = append(obj.data, 'x')
				p.Put(obj)
			}
		}()
	}
	wg.Wait()

	assert.Positive(t, got.Load(), "горутины должны были получить объекты из пула")

	total := 0
	for {
		if _, ok := p.Get(); !ok {
			break
		}
		total++
	}
	assert.Equal(t, workers, total, "количество объектов в пуле не должно измениться")
}
