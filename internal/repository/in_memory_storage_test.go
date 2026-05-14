package repository

import (
	"errors"
	"testing"

	"github.com/e-l-l-a-r/monitoring/internal/model"
	"github.com/stretchr/testify/assert"
)

var ms *MemStorage = NewMemStorage()

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
			ms.AddData(tt.key, tt.mType, tt.val)
			val, err := ms.GetValue(tt.key, tt.mType)
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
			ms.AddData(tt.key, tt.mType, tt.val)
			val, err := ms.GetValue(tt.key, tt.mType)
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
		{"Add incorrect type", "cnt", model.Gauge, 1.0, "Type mismatch"},
		{"Add wrong type", "bad", "bad", 2.0, "Invalid type"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ms.AddData(tt.key, tt.mType, tt.val)
			assert.EqualError(t, err, tt.expected)
		})
	}

	val, err := ms.GetValue("cnt", model.Counter)
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
				Value: func() *float64 { v := 3.14; return &v }(),
			},
			expected: map[string]model.Metrics{
				"test_gauge": {
					ID:    "test_gauge",
					MType: model.Gauge,
					Value: func() *float64 { v := 3.14; return &v }(),
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
					Value: func() *float64 { v := 1.0; return &v }(),
				},
			},
			metric: model.Metrics{
				ID:    "test_gauge",
				MType: model.Gauge,
				Value: func() *float64 { v := 2.5; return &v }(),
			},
			expected: map[string]model.Metrics{
				"test_gauge": {
					ID:    "test_gauge",
					MType: model.Gauge,
					Value: func() *float64 { v := 2.5; return &v }(),
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
					Delta: func() *int64 { v := int64(10); return &v }(),
				},
			},
			metric: model.Metrics{
				ID:    "test_counter",
				MType: model.Counter,
				Delta: func() *int64 { d := int64(32); return &d }(),
			},
			expected: map[string]model.Metrics{
				"test_counter": {
					ID:    "test_counter",
					MType: model.Counter,
					Delta: func() *int64 { v := int64(42); return &v }(),
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
					Value: func() *float64 { v := 1.0; return &v }(),
				},
			},
			metric: model.Metrics{
				ID:    "test_metric",
				MType: model.Counter,
				Delta: func() *int64 { d := int64(5); return &d }(),
			},
			expected: map[string]model.Metrics{
				"test_metric": {
					ID:    "test_metric",
					MType: model.Gauge,
					Value: func() *float64 { v := 1.0; return &v }(),
				},
			},
			err: errors.New("Type mismatch"),
		},
		{
			name:    "Invalid metric type",
			initial: map[string]model.Metrics{},
			metric: model.Metrics{
				ID:    "invalid",
				MType: "invalid_type",
				Value: func() *float64 { v := 0.0; return &v }(),
			},
			expected: map[string]model.Metrics{},
			err:      errors.New("Invalid type"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			storage := &MemStorage{
				Metrics: tt.initial,
			}

			err := storage.AddMetricData(tt.metric)

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
					Value: func() *float64 { v := 3.14; return &v }(),
				},
			},
			metric: &model.Metrics{
				ID:    "test_gauge",
				MType: model.Gauge,
			},
			expected: &model.Metrics{
				ID:    "test_gauge",
				MType: model.Gauge,
				Value: func() *float64 { v := 3.14; return &v }(),
			},
			err: nil,
		},
		{
			name: "Get existing counter",
			initial: map[string]model.Metrics{
				"test_counter": {
					ID:    "test_counter",
					MType: model.Counter,
					Delta: func() *int64 { v := int64(42); return &v }(),
				},
			},
			metric: &model.Metrics{
				ID:    "test_counter",
				MType: model.Counter,
			},
			expected: &model.Metrics{
				ID:    "test_counter",
				MType: model.Counter,
				Delta: func() *int64 { v := int64(42); return &v }(),
			},
			err: nil,
		},
		{
			name: "Metric not found",
			initial: map[string]model.Metrics{
				"other_metric": {
					ID:    "other_metric",
					MType: model.Gauge,
					Value: func() *float64 { v := 1.0; return &v }(),
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
					Value: func() *float64 { v := 1.0; return &v }(),
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

			err := storage.GetMetricValue(tt.metric)

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
