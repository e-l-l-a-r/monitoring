package agent

import (
	"testing"
)

func BenchmarkDataCollector_UpdMetrics(b *testing.B) {
	dc := NewDataCollector()
	for b.Loop() {
		dc.UpdMetrics()
	}
}
