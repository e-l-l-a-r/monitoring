package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMemStorage_AddData(t *testing.T) {
	ms := NewMemStorage()
	ms.AddData("test", Gauge, 1.0)
	assert.Equal(t, *ms.Metrics["test"].Value, 1.0)
	ms.AddData("test", Gauge, 2.0)
	assert.Equal(t, *ms.Metrics["test"].Value, 2.0)
	ms.AddData("test", Gauge, -1.0)
	assert.Equal(t, *ms.Metrics["test"].Value, -1.0)
	ms.AddData("alt_test", Gauge, 1.5)
	assert.Equal(t, *ms.Metrics["alt_test"].Value, 1.5)
	assert.Equal(t, *ms.Metrics["test"].Value, -1.0)

	ms.AddData("cnt", Counter, 1.5)
	assert.Equal(t, *ms.Metrics["cnt"].Value, 1.5)
	ms.AddData("cnt", Counter, 1.5)
	assert.Equal(t, *ms.Metrics["cnt"].Value, 3.0)
	ms.AddData("cnt", Counter, -1.0)
	assert.Equal(t, *ms.Metrics["cnt"].Value, 2.0)

	err := ms.AddData("cnt", Gauge, 1.5)
	assert.EqualError(t, err, "Type mismatch")
	assert.Equal(t, *ms.Metrics["cnt"].Value, 2.0)

	err = ms.AddData("bad", "bad", 1.5)
	assert.EqualError(t, err, "Invalid type")
}
