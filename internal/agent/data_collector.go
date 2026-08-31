package agent

import (
	"fmt"
	"math/rand"
	"runtime"
	"time"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/mem"

	"github.com/e-l-l-a-r/monitoring/internal/logger"
	"github.com/e-l-l-a-r/monitoring/internal/model"
)

type metric struct {
	model.Metrics
	getter func() float64
	tick   func() int64
}

type ChannaledMetric struct {
	Key string
	metric
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
	memUsg  *mem.VirtualMemoryStat
	cpuUsg  *[]float64
	metrics map[string]metric
}

func NewDataCollector() *DataCollector {
	memStats := new(runtime.MemStats)
	memUsage, _ := mem.VirtualMemory()
	cpuUsage, _ := cpu.Percent(0, true)

	metrics := map[string]metric{
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
		"TotalMemory":   *NewGauge("TotalMemory", func() float64 { return float64(memUsage.Total) }),
		"FreeMemory":    *NewGauge("FreeMemory", func() float64 { return float64(memUsage.Free) }),
	}

	for n, _ := range cpuUsage {
		name := fmt.Sprintf("CPUutilization%d", (n + 1))
		metrics[name] = *NewGauge(fmt.Sprintf("CPU%d Usage", n+1), func() float64 { return cpuUsage[n] })
	}
	return &DataCollector{
		memStt:  memStats,
		memUsg:  memUsage,
		cpuUsg:  &cpuUsage,
		metrics: metrics,
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

func (dc *DataCollector) MetricsReader(doneCh chan struct{}, delay uint) chan ChannaledMetric {
	ch := make(chan ChannaledMetric, len(dc.metrics))
	sync := make(chan struct{})

	// Горутина обновляет данные раз в нужный интервал времени и запускает генерацию данных
	go func() {
		for {
			select {
			case <-doneCh:
				close(sync)
				return
			default:
				logger.Info("Updating metrics")
				runtime.ReadMemStats(dc.memStt)
				dc.memUsg, _ = mem.VirtualMemory()
				cpu_vals, _ := cpu.Percent(0, true)
				for n, val := range cpu_vals {
					(*dc.cpuUsg)[n] = val
				}
				for range dc.metrics {
					sync <- struct{}{}
				}
			}
			time.Sleep(time.Second * time.Duration(delay))
		}
	}()

	for key, metric := range dc.metrics {
		metric := ChannaledMetric{
			key, metric,
		}
		go func() {
			logger.Info("Starting collector goroutine for metric " + key)
			for {
				_, ok := <-sync
				if !ok {
					return
				}
				logger.Info("Collecting metric " + key)
				switch metric.MType {
				case model.Gauge:
					*metric.Value = metric.getter()
				case model.Counter:
					*metric.Delta += metric.tick()
				}
				ch <- metric
				time.Sleep(time.Millisecond * 100)
			}
		}()
	}

	return ch
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
