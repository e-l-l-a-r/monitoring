package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewMetrics(t *testing.T) {
	t.Run("counter", func(t *testing.T) {
		m := NewCounterMetrics("c1", 10)
		assert.Equal(t, "c1", m.ID)
		assert.Equal(t, Counter, m.MType)
		assert.Equal(t, int64(10), *m.Delta)
		assert.Nil(t, m.Value)
	})

	t.Run("gauge", func(t *testing.T) {
		m := NewGaugeMetrics("g1", 10.5)
		assert.Equal(t, "g1", m.ID)
		assert.Equal(t, Gauge, m.MType)
		assert.Equal(t, 10.5, *m.Value)
		assert.Nil(t, m.Delta)
	})

	t.Run("generic constructor", func(t *testing.T) {
		m1 := NewMetrics("c2", Counter)
		assert.Equal(t, Counter, m1.MType)
		assert.Equal(t, int64(0), *m1.Delta)

		m2 := NewMetrics("g2", Gauge)
		assert.Equal(t, Gauge, m2.MType)
		assert.Equal(t, 0.0, *m2.Value)

		m3 := NewMetrics("u1", "unknown")
		assert.Equal(t, "unknown", m3.MType)
		assert.Nil(t, m3.Delta)
		assert.Nil(t, m3.Value)
	})
}
