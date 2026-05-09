package agent

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDataCollector_UpdMetrics(t *testing.T) {
	tests := []struct {
		name string
		val  float64
	}{
		{"first", 1.0},
		{"second", 2.0},
		{"third", 3.0},
		{"fourth", 4.0},
		{"fifth", 1.0},
		{"after_send", 2.0},
	}
	rnd := 2.0
	dc := NewDataCollector()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dc.UpdMetrics()
			for _, metric := range dc.metrics {
				switch metric.ID {
				case "PollCount":
					assert.Equal(t, tt.val, *metric.Value)
					if tt.val == 4.0 {
						vals := dc.GetValues()
						assert.Equal(t, tt.val, *vals["PollCount"].Value)
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
