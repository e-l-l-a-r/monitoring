package repository

import (
	"context"
	"fmt"
	"testing"

	"github.com/e-l-l-a-r/monitoring/internal/model"
)

func BenchmarkMemStorage_AddData(b *testing.B) {
	ms := NewMemStorage(0, "")
	ctx := context.Background()
	i := 0
	for b.Loop() {
		name := fmt.Sprintf("metric_%d", i%1000)
		ms.AddData(ctx, name, model.Gauge, float64(i))
		i++
	}
}

func BenchmarkMemStorage_GetValue(b *testing.B) {
	ms := NewMemStorage(0, "")
	ctx := context.Background()
	for i := range 1000 {
		ms.AddData(ctx, fmt.Sprintf("metric_%d", i), model.Gauge, float64(i))
	}
	i := 0
	for b.Loop() {
		name := fmt.Sprintf("metric_%d", i%1000)
		ms.GetValue(ctx, name, model.Gauge)
		i++
	}
}

func BenchmarkMemStorage_AddMetricData(b *testing.B) {
	ms := NewMemStorage(0, "")
	ctx := context.Background()
	metrics := make([]model.Metrics, 1000)
	for i := range 1000 {
		metrics[i] = model.NewGaugeMetrics(fmt.Sprintf("metric_%d", i), float64(i))
	}
	i := 0
	for b.Loop() {
		ms.AddMetricData(ctx, metrics[i%1000])
		i++
	}
}

func BenchmarkMemStorage_GetMetricValue(b *testing.B) {
	ms := NewMemStorage(0, "")
	ctx := context.Background()
	for i := range 1000 {
		ms.AddData(ctx, fmt.Sprintf("metric_%d", i), model.Gauge, float64(i))
	}
	i := 0
	for b.Loop() {
		m := model.Metrics{ID: fmt.Sprintf("metric_%d", i%1000), MType: model.Gauge}
		ms.GetMetricValue(ctx, &m)
		i++
	}
}
