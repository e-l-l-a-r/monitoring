package repository

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/e-l-l-a-r/monitoring/internal/model"
)

var ms = NewMemStorage(0, "test_metrics.json")

func TestMemStorage_GoodGaugeCases(t *testing.T) {

	tests := []struct {
		name     string
		key      string
		mType    string
		val      float64
		expected float64
	}{
		{"Init gauge", "test", model.Gauge, 1.0, 1.0},
		{"Change gauge", "test", model.Gauge, 2.0, 2.0},
		{"Change to negative", "test", model.Gauge, -1.0, -1.0},
		{"Init other gauge", "alt_test", model.Gauge, 1.5, 1.5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.TODO())
			defer cancel()
			ms.AddData(ctx, tt.key, tt.mType, tt.val)
			val, err := ms.GetValue(ctx, tt.key, tt.mType)
			assert.NoError(t, err)
			assert.Equal(t, tt.expected, val)
		})
	}
}

func TestMemStorage_GoodCounterCases(t *testing.T) {

	tests := []struct {
		name     string
		key      string
		mType    string
		val      int64
		expected float64
	}{
		{"Init counter", "cnt", model.Counter, 1, 1},
		{"Update counter", "cnt", model.Counter, 2, 3},
		{"Update with negative", "cnt", model.Counter, -1, 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.TODO())
			defer cancel()
			ms.AddData(ctx, tt.key, tt.mType, tt.val)
			val, err := ms.GetValue(ctx, tt.key, tt.mType)
			assert.NoError(t, err)
			assert.Equal(t, tt.expected, val)
		})
	}
}

func TestMemStorage_BadCases(t *testing.T) {
	tests := []struct {
		name     string
		key      string
		mType    string
		val      float64
		expected string
	}{
		{"Add incorrect type", "cnt", model.Gauge, 1.0, "type mismatch"},
		{"Add wrong type", "bad", "bad", 2.0, "invalid type"},
	}
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ms.AddData(ctx, tt.key, tt.mType, tt.val)
			assert.EqualError(t, err, tt.expected)
		})
	}

	val, err := ms.GetValue(ctx, "cnt", model.Counter)
	assert.NoError(t, err)
	assert.Equal(t, 2.0, val)
}
func TestAddMetricData(t *testing.T) {
	tests := []struct {
		name     string
		initial  map[string]model.Metrics
		metric   model.Metrics
		expected map[string]model.Metrics
		err      error
	}{
		{
			name:    "Add new gauge metric",
			initial: map[string]model.Metrics{},
			metric: model.Metrics{
				ID:    "test_gauge",
				MType: model.Gauge,
				Value: func() *float64 { return new(3.14) }(),
			},
			expected: map[string]model.Metrics{
				"test_gauge": {
					ID:    "test_gauge",
					MType: model.Gauge,
					Value: func() *float64 { return new(3.14) }(),
				},
			},
			err: nil,
		},
		{
			name: "Update existing gauge",
			initial: map[string]model.Metrics{
				"test_gauge": {
					ID:    "test_gauge",
					MType: model.Gauge,
					Value: func() *float64 { return new(1.0) }(),
				},
			},
			metric: model.Metrics{
				ID:    "test_gauge",
				MType: model.Gauge,
				Value: func() *float64 { return new(2.5) }(),
			},
			expected: map[string]model.Metrics{
				"test_gauge": {
					ID:    "test_gauge",
					MType: model.Gauge,
					Value: func() *float64 { return new(2.5) }(),
				},
			},
			err: nil,
		},
		{
			name: "Update existing counter",
			initial: map[string]model.Metrics{
				"test_counter": {
					ID:    "test_counter",
					MType: model.Counter,
					Delta: func() *int64 { return new(int64(10)) }(),
				},
			},
			metric: model.Metrics{
				ID:    "test_counter",
				MType: model.Counter,
				Delta: func() *int64 { return new(int64(32)) }(),
			},
			expected: map[string]model.Metrics{
				"test_counter": {
					ID:    "test_counter",
					MType: model.Counter,
					Delta: func() *int64 { return new(int64(42)) }(),
				},
			},
			err: nil,
		},
		{
			name: "Type mismatch",
			initial: map[string]model.Metrics{
				"test_metric": {
					ID:    "test_metric",
					MType: model.Gauge,
					Value: func() *float64 { return new(1.0) }(),
				},
			},
			metric: model.Metrics{
				ID:    "test_metric",
				MType: model.Counter,
				Delta: func() *int64 { return new(int64(5)) }(),
			},
			expected: map[string]model.Metrics{
				"test_metric": {
					ID:    "test_metric",
					MType: model.Gauge,
					Value: func() *float64 { return new(1.0) }(),
				},
			},
			err: errors.New("type mismatch"),
		},
		{
			name:    "Invalid metric type",
			initial: map[string]model.Metrics{},
			metric: model.Metrics{
				ID:    "invalid",
				MType: "invalid_type",
				Value: func() *float64 { return new(0.0) }(),
			},
			expected: map[string]model.Metrics{},
			err:      errors.New("invalid type"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			storage := &MemStorage{
				Metrics: tt.initial,
			}
			ctx, cancel := context.WithCancel(context.TODO())
			defer cancel()

			err := storage.AddMetricData(ctx, tt.metric)

			if tt.err != nil {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.err.Error())
			} else {
				assert.NoError(t, err)
			}

			assert.Equal(t, tt.expected, storage.Metrics)
		})
	}
}

func TestGetMetricValue(t *testing.T) {
	tests := []struct {
		name     string
		initial  map[string]model.Metrics
		metric   *model.Metrics
		expected *model.Metrics
		err      error
	}{
		{
			name: "Get existing gauge",
			initial: map[string]model.Metrics{
				"test_gauge": {
					ID:    "test_gauge",
					MType: model.Gauge,
					Value: func() *float64 { return new(3.14) }(),
				},
			},
			metric: &model.Metrics{
				ID:    "test_gauge",
				MType: model.Gauge,
			},
			expected: &model.Metrics{
				ID:    "test_gauge",
				MType: model.Gauge,
				Value: func() *float64 { return new(3.14) }(),
			},
			err: nil,
		},
		{
			name: "Get existing counter",
			initial: map[string]model.Metrics{
				"test_counter": {
					ID:    "test_counter",
					MType: model.Counter,
					Delta: func() *int64 { return new(int64(42)) }(),
				},
			},
			metric: &model.Metrics{
				ID:    "test_counter",
				MType: model.Counter,
			},
			expected: &model.Metrics{
				ID:    "test_counter",
				MType: model.Counter,
				Delta: func() *int64 { return new(int64(42)) }(),
			},
			err: nil,
		},
		{
			name: "Metric not found",
			initial: map[string]model.Metrics{
				"other_metric": {
					ID:    "other_metric",
					MType: model.Gauge,
					Value: func() *float64 { return new(1.0) }(),
				},
			},
			metric: &model.Metrics{
				ID:    "missing_metric",
				MType: model.Gauge,
			},
			expected: nil,
			err:      &MetricNotFoundError{"missing_metric"},
		},
		{
			name: "Type mismatch",
			initial: map[string]model.Metrics{
				"test_metric": {
					ID:    "test_metric",
					MType: model.Gauge,
					Value: func() *float64 { return new(1.0) }(),
				},
			},
			metric: &model.Metrics{
				ID:    "test_metric",
				MType: model.Counter,
			},
			expected: nil,
			err:      &TypeMismatchError{"test_metric"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			storage := &MemStorage{
				Metrics: tt.initial,
			}
			ctx, cancel := context.WithCancel(context.TODO())
			defer cancel()

			err := storage.GetMetricValue(ctx, tt.metric)

			if tt.err != nil {
				assert.Error(t, err)
				assert.Equal(t, tt.err, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, tt.metric)
			}
		})
	}
}
