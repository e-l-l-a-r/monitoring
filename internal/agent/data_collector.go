package agent

import (
	"math/rand"
	"runtime"
)

type metric struct {
	name   string
	getter func() float64
	Val    float64
	MType  string
}

type DataCollector struct {
	memStt  *runtime.MemStats
	metrics []metric
}

func NewDataCollector() *DataCollector {
	memStats := new(runtime.MemStats)
	return &DataCollector{
		memStt: memStats,
		metrics: []metric{
			{
				name:   "Alloc",
				getter: func() float64 { return float64(memStats.Alloc) },
				MType:  "gauge",
			},
			{
				name:   "BuckHashSys",
				getter: func() float64 { return float64(memStats.BuckHashSys) },
				MType:  "gauge",
			},
			{
				name:   "Frees",
				getter: func() float64 { return float64(memStats.Frees) },
				MType:  "gauge",
			}, {
				name:   "GCCPUFraction",
				getter: func() float64 { return float64(memStats.GCCPUFraction) },
				MType:  "gauge",
			}, {
				name:   "GCSys",
				getter: func() float64 { return float64(memStats.GCSys) },
				MType:  "gauge",
			}, {
				name:   "HeapAlloc",
				getter: func() float64 { return float64(memStats.HeapAlloc) },
				MType:  "gauge",
			}, {
				name:   "HeapIdle",
				getter: func() float64 { return float64(memStats.HeapIdle) },
				MType:  "gauge",
			}, {
				name:   "HeapInuse",
				getter: func() float64 { return float64(memStats.HeapInuse) },
				MType:  "gauge",
			}, {
				name:   "HeapObjects",
				getter: func() float64 { return float64(memStats.HeapObjects) },
				MType:  "gauge",
			}, {
				name:   "HeapReleased",
				getter: func() float64 { return float64(memStats.HeapReleased) },
				MType:  "gauge",
			}, {
				name:   "HeapSys",
				getter: func() float64 { return float64(memStats.HeapSys) },
				MType:  "gauge",
			}, {
				name:   "LastGC",
				getter: func() float64 { return float64(memStats.LastGC) },
				MType:  "gauge",
			}, {
				name:   "Lookups",
				getter: func() float64 { return float64(memStats.Lookups) },
				MType:  "gauge",
			}, {
				name:   "MCacheInuse",
				getter: func() float64 { return float64(memStats.MCacheInuse) },
				MType:  "gauge",
			}, {
				name:   "MCacheSys",
				getter: func() float64 { return float64(memStats.MCacheSys) },
				MType:  "gauge",
			}, {
				name:   "MSpanInuse",
				getter: func() float64 { return float64(memStats.MSpanInuse) },
				MType:  "gauge",
			}, {
				name:   "MSpanSys",
				getter: func() float64 { return float64(memStats.MSpanSys) },
				MType:  "gauge",
			}, {
				name:   "Mallocs",
				getter: func() float64 { return float64(memStats.Mallocs) },
				MType:  "gauge",
			}, {
				name:   "NextGC",
				getter: func() float64 { return float64(memStats.NextGC) },
				MType:  "gauge",
			}, {
				name:   "NumForcedGC",
				getter: func() float64 { return float64(memStats.NumForcedGC) },
				MType:  "gauge",
			}, {
				name:   "NumGC",
				getter: func() float64 { return float64(memStats.NumGC) },
				MType:  "gauge",
			}, {
				name:   "OtherSys",
				getter: func() float64 { return float64(memStats.OtherSys) },
				MType:  "gauge",
			}, {
				name:   "PauseTotalNs",
				getter: func() float64 { return float64(memStats.PauseTotalNs) },
				MType:  "gauge",
			}, {
				name:   "StackInuse",
				getter: func() float64 { return float64(memStats.StackInuse) },
				MType:  "gauge",
			}, {
				name:   "StackSys",
				getter: func() float64 { return float64(memStats.StackSys) },
				MType:  "gauge",
			}, {
				name:   "Sys",
				getter: func() float64 { return float64(memStats.Sys) },
				MType:  "gauge",
			}, {
				name:   "TotalAlloc",
				getter: func() float64 { return float64(memStats.TotalAlloc) },
				MType:  "gauge",
			}, {
				name:   "PollCount",
				getter: func() float64 { return 1.0 },
				MType:  "counter",
			}, {
				name:   "RandomValue",
				getter: func() float64 { return rand.Float64() },
				MType:  "gauge",
			},
		},
	}
}

func (dc *DataCollector) UpdMetrics() {
	runtime.ReadMemStats(dc.memStt)
	for i := range dc.metrics {
		metric := &dc.metrics[i]
		//В соответствии с типом либо переписываем, либо инкрементируем значение
		switch metric.MType {

		case "gauge":
			metric.Val = metric.getter()

		case "counter":
			metric.Val += metric.getter()

		}
	}
}

func (dc *DataCollector) GetValues() (result map[string]metric) {
	result = make(map[string]metric)
	for i, metric := range dc.metrics {
		result[metric.name] = metric

		// На скервере значение счетчика приплюсовывается к предыдущему,
		//поэтому храним только сумму за прошедшее с прошлой отправки время
		if metric.MType == "counter" {
			data := &dc.metrics[i]
			data.Val = 0
		}
	}

	return
}
