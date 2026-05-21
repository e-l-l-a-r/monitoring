package agent

import (
	"math/rand"
	"runtime"

	"github.com/e-l-l-a-r/monitoring/internal/model"
)

type metric struct {
	model.Metrics
	getter func() float64
	tick   func() int64
}

func NewGauge(name string, getter func() float64) *metric {
	return &metric{
		Metrics: model.NewGaugeMetrics(name, 0),
		getter:  getter,
	}
}

func NewCounter(name string, tick func() int64) *metric {
	return &metric{
		Metrics: model.NewCounterMetrics(name, 0),
		tick:    tick,
	}
}

type DataCollector struct {
	memStt  *runtime.MemStats
	metrics map[string]metric
}

func NewDataCollector() *DataCollector {
	memStats := new(runtime.MemStats)
	return &DataCollector{
		memStt: memStats,
		metrics: map[string]metric{
			"Alloc":         *NewGauge("Alloc", func() float64 { return float64(memStats.Alloc) }),
			"BuckHashSys":   *NewGauge("BuckHashSys", func() float64 { return float64(memStats.BuckHashSys) }),
			"Frees":         *NewGauge("Frees", func() float64 { return float64(memStats.Frees) }),
			"GCCPUFraction": *NewGauge("GCCPUFraction", func() float64 { return float64(memStats.GCCPUFraction) }),
			"GCSys":         *NewGauge("GCSys", func() float64 { return float64(memStats.GCSys) }),
			"HeapAlloc":     *NewGauge("HeapAlloc", func() float64 { return float64(memStats.HeapAlloc) }),
			"HeapIdle":      *NewGauge("HeapIdle", func() float64 { return float64(memStats.HeapIdle) }),
			"HeapInuse":     *NewGauge("HeapInuse", func() float64 { return float64(memStats.HeapInuse) }),
			"HeapObjects":   *NewGauge("HeapObjects", func() float64 { return float64(memStats.HeapObjects) }),
			"HeapReleased":  *NewGauge("HeapReleased", func() float64 { return float64(memStats.HeapReleased) }),
			"HeapSys":       *NewGauge("HeapSys", func() float64 { return float64(memStats.HeapSys) }),
			"LastGC":        *NewGauge("LastGC", func() float64 { return float64(memStats.LastGC) }),
			"Lookups":       *NewGauge("Lookups", func() float64 { return float64(memStats.Lookups) }),
			"MCacheInuse":   *NewGauge("MCacheInuse", func() float64 { return float64(memStats.MCacheInuse) }),
			"MCacheSys":     *NewGauge("MCacheSys", func() float64 { return float64(memStats.MCacheSys) }),
			"MSpanInuse":    *NewGauge("MSpanInuse", func() float64 { return float64(memStats.MSpanInuse) }),
			"MSpanSys":      *NewGauge("MSpanSys", func() float64 { return float64(memStats.MSpanSys) }),
			"Mallocs":       *NewGauge("Mallocs", func() float64 { return float64(memStats.Mallocs) }),
			"NextGC":        *NewGauge("NextGC", func() float64 { return float64(memStats.NextGC) }),
			"NumForcedGC":   *NewGauge("NumForcedGC", func() float64 { return float64(memStats.NumForcedGC) }),
			"NumGC":         *NewGauge("NumGC", func() float64 { return float64(memStats.NumGC) }),
			"OtherSys":      *NewGauge("OtherSys", func() float64 { return float64(memStats.OtherSys) }),
			"PauseTotalNs":  *NewGauge("PauseTotalNs", func() float64 { return float64(memStats.PauseTotalNs) }),
			"StackInuse":    *NewGauge("StackInuse", func() float64 { return float64(memStats.StackInuse) }),
			"StackSys":      *NewGauge("StackSys", func() float64 { return float64(memStats.StackSys) }),
			"Sys":           *NewGauge("Sys", func() float64 { return float64(memStats.Sys) }),
			"TotalAlloc":    *NewGauge("TotalAlloc", func() float64 { return float64(memStats.TotalAlloc) }),
			"PollCount":     *NewCounter("PollCount", func() int64 { return 1.0 }),
			"RandomValue":   *NewGauge("RandomValue", func() float64 { return rand.Float64() }),
		},
	}
}

func (dc *DataCollector) UpdMetrics() {
	runtime.ReadMemStats(dc.memStt)
	for _, metric := range dc.metrics {
		//В соответствии с типом либо переписываем, либо инкрементируем значение
		switch metric.MType {

		case model.Gauge:
			*metric.Value = metric.getter()

		case model.Counter:
			*metric.Delta += metric.tick()

		}
	}
}

func (dc *DataCollector) GetValues() map[string]metric {
	return dc.metrics
}
func (dc *DataCollector) OnSuccessSent(name string) {
	val, ok := dc.metrics[name]
	if ok && val.MType == model.Counter {
		*val.Delta = 0
	}

}
