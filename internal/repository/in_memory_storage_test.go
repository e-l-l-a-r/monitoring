package repository

import (
	"testing"

	"github.com/e-l-l-a-r/monitoring/internal/model"
	"github.com/stretchr/testify/assert"
)

var ms *MemStorage = NewMemStorage()

func TestMemStorage_GoodCases(t *testing.T) {

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
		{"Init counter", "cnt", model.Counter, 1.5, 1.5},
		{"Update counter", "cnt", model.Counter, 1.5, 3.0},
		{"Update with negative", "cnt", model.Counter, -1.0, 2.0},
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
