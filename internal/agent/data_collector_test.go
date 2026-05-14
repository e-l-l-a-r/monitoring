package agent

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDataCollector_UpdMetrics(t *testing.T) {
	tests := []struct {
		name string
		val  int64
	}{
		{"first", 1},
		{"second", 2},
		{"third", 3},
		{"fourth", 4},
		{"fifth", 1},
		{"after_send", 2},
	}
	rnd := 2.0
	dc := NewDataCollector()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dc.UpdMetrics()
			for _, metric := range dc.metrics {
				switch metric.ID {
				case "PollCount":
					assert.Equal(t, tt.val, *metric.Delta)
					if tt.val == 4.0 {
						vals := dc.GetValues()
						assert.Equal(t, tt.val, *vals["PollCount"].Delta)
						dc.OnSuccessSent("PollCount")
					}
				case "RandomValue":
					assert.NotEqual(t, rnd, *metric.Value)
					rnd = *metric.Value
				}
			}
		})
	}
}
