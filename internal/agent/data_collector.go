package agent

import (
	"math/rand"
	"runtime"

	"github.com/e-l-l-a-r/monitoring/internal/model"
)

type metric struct {
	model.Metrics
	getter func() float64
}

func NewMetric(name string, mtype string, getter func() float64) *metric {
	return &metric{
		Metrics: model.NewMetrics(name, mtype, 0),
		getter:  getter,
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
			"Alloc": *NewMetric("Alloc", model.Gauge,
				func() float64 { return float64(memStats.Alloc) }),
			"BuckHashSys": *NewMetric("BuckHashSys", model.Gauge,
				func() float64 { return float64(memStats.BuckHashSys) }),
			"Frees": *NewMetric("Frees", model.Gauge,
				func() float64 { return float64(memStats.Frees) }),
			"GCCPUFraction": *NewMetric("GCCPUFraction", model.Gauge,
				func() float64 { return float64(memStats.GCCPUFraction) }),
			"GCSys": *NewMetric("GCSys", model.Gauge,
				func() float64 { return float64(memStats.GCSys) }),
			"HeapAlloc": *NewMetric("HeapAlloc", model.Gauge,
				func() float64 { return float64(memStats.HeapAlloc) }),
			"HeapIdle": *NewMetric("HeapIdle", model.Gauge,
				func() float64 { return float64(memStats.HeapIdle) }),
			"HeapInuse": *NewMetric("HeapInuse", model.Gauge,
				func() float64 { return float64(memStats.HeapInuse) }),
			"HeapObjects": *NewMetric("HeapObjects", model.Gauge,
				func() float64 { return float64(memStats.HeapObjects) }),
			"HeapReleased": *NewMetric("HeapReleased", model.Gauge,
				func() float64 { return float64(memStats.HeapReleased) }),
			"HeapSys": *NewMetric("HeapSys", model.Gauge,
				func() float64 { return float64(memStats.HeapSys) }),
			"LastGC": *NewMetric("LastGC", model.Gauge,
				func() float64 { return float64(memStats.LastGC) }),
			"Lookups": *NewMetric("Lookups", model.Gauge,
				func() float64 { return float64(memStats.Lookups) }),
			"MCacheInuse": *NewMetric("MCacheInuse", model.Gauge,
				func() float64 { return float64(memStats.MCacheInuse) }),
			"MCacheSys": *NewMetric("MCacheSys", model.Gauge,
				func() float64 { return float64(memStats.MCacheSys) }),
			"MSpanInuse": *NewMetric("MSpanInuse", model.Gauge,
				func() float64 { return float64(memStats.MSpanInuse) }),
			"MSpanSys": *NewMetric("MSpanSys", model.Gauge,
				func() float64 { return float64(memStats.MSpanSys) }),
			"Mallocs": *NewMetric("Mallocs", model.Gauge,
				func() float64 { return float64(memStats.Mallocs) }),
			"NextGC": *NewMetric("NextGC", model.Gauge,
				func() float64 { return float64(memStats.NextGC) }),
			"NumForcedGC": *NewMetric("NumForcedGC", model.Gauge,
				func() float64 { return float64(memStats.NumForcedGC) }),
			"NumGC": *NewMetric("NumGC", model.Gauge,
				func() float64 { return float64(memStats.NumGC) }),
			"OtherSys": *NewMetric("OtherSys", model.Gauge,
				func() float64 { return float64(memStats.OtherSys) }),
			"PauseTotalNs": *NewMetric("PauseTotalNs", model.Gauge,
				func() float64 { return float64(memStats.PauseTotalNs) }),
			"StackInuse": *NewMetric("StackInuse", model.Gauge,
				func() float64 { return float64(memStats.StackInuse) }),
			"StackSys": *NewMetric("StackSys", model.Gauge,
				func() float64 { return float64(memStats.StackSys) }),
			"Sys": *NewMetric("Sys", model.Gauge,
				func() float64 { return float64(memStats.Sys) }),
			"TotalAlloc": *NewMetric("TotalAlloc", model.Gauge,
				func() float64 { return float64(memStats.TotalAlloc) }),
			"PollCount": *NewMetric("PollCount", model.Counter,
				func() float64 { return 1.0 }),
			"RandomValue": *NewMetric("RandomValue", model.Gauge,
				func() float64 { return rand.Float64() }),
		},
	}
}

func (dc *DataCollector) UpdMetrics() {
	runtime.ReadMemStats(dc.memStt)
	for _, metric := range dc.metrics {
		//В соответствии с типом либо переписываем, либо инкрементируем значение
		switch metric.MType {

		case "gauge":
			*metric.Value = metric.getter()

		case "counter":
			*metric.Value += metric.getter()

		}
	}
}

func (dc *DataCollector) GetValues() map[string]metric {
	return dc.metrics
}
func (dc *DataCollector) OnSuccessSent(name string) {
	val, ok := dc.metrics[name]
	if ok && val.MType == "counter" {
		*val.Value = 0
	}

}
