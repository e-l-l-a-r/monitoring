package repository

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/e-l-l-a-r/monitoring/internal/logger"
	"github.com/e-l-l-a-r/monitoring/internal/model"
)

type MetricNotFoundError struct {
	Name string
}

func (e *MetricNotFoundError) Error() string {
	return fmt.Sprintf("metric %q not found", e.Name)
}

type TypeMismatchError struct {
	Name string
}

func (e *TypeMismatchError) Error() string {
	return fmt.Sprintf("type mismatch for metric %q", e.Name)
}

type MemStorage struct {
	Metrics      map[string]model.Metrics
	lastSyncTime time.Time
	SyncInterval uint
	SyncFileName string
}

func NewMemStorage(SyncInterval uint, SyncFileName string) *MemStorage {
	return &MemStorage{
		Metrics:      make(map[string]model.Metrics),
		lastSyncTime: time.Now(),
		SyncInterval: SyncInterval,
		SyncFileName: SyncFileName,
	}
}

func (ms *MemStorage) AddData(name string, mtype string, value interface{}) error {
	val, ok := ms.Metrics[name]
	if ok {
		if val.MType != mtype {
			return fmt.Errorf("type mismatch")
		}
		switch val.MType {
		case model.Counter:
			counterValue, ok := value.(int64)
			if !ok {
				return fmt.Errorf("invalid value type for Counter: expected int64")
			}
			*val.Delta += counterValue
		case model.Gauge:
			gaugeValue, ok := value.(float64)
			if !ok {
				return fmt.Errorf("invalid value type for Gauge: expected float64")
			}
			*val.Value = gaugeValue
		}
		return nil
	}

	if !isValidMetricType(mtype) {
		return fmt.Errorf("invalid type")
	}

	switch newVal := value.(type) {
	case int64:
		if mtype == model.Counter {
			ms.Metrics[name] = model.NewCounterMetrics(name, newVal)
		} else {
			return fmt.Errorf("invalid value type for Counter: expected int64")
		}
	case float64:
		if mtype == model.Gauge {
			ms.Metrics[name] = model.NewGaugeMetrics(name, newVal)
		} else {
			return fmt.Errorf("invalid value type for Gauge: expected float64")
		}
	default:
		return fmt.Errorf("unsupported value type: expected int64 or float64")
	}

	return nil
}

func (ms *MemStorage) AddMetricData(metric model.Metrics) error {
	val, ok := ms.Metrics[metric.ID]
	if ok {
		if val.MType != metric.MType {
			return fmt.Errorf("type mismatch")
		}
		before, _ := json.Marshal(val)
		switch val.MType {
		case model.Counter:
			*val.Delta += *metric.Delta
		case model.Gauge:
			*val.Value = *metric.Value
		}
		after, _ := json.Marshal(val)
		logger.Info("Metric changed: ", string(before), " -> ", string(after))
		return nil
	}
	if !isValidMetricType(metric.MType) {
		return fmt.Errorf("invalid type")
	}

	ms.Metrics[metric.ID] = metric
	str, _ := json.Marshal(metric)
	logger.Info("Add metric: ", string(str))
	return nil
}

func isValidMetricType(mType string) bool {
	return mType == model.Counter || mType == model.Gauge
}

func (ms *MemStorage) GetValues() map[string]model.Metrics {
	return ms.Metrics
}

func (ms *MemStorage) GetValue(name string, mtype string) (float64, error) {
	val, ok := ms.Metrics[name]
	if !ok {
		return 0, &MetricNotFoundError{name}
	}
	if val.MType != mtype {
		return 0, &TypeMismatchError{name}
	}
	switch mtype {
	case model.Gauge:
		return *val.Value, nil
	case model.Counter:
		return float64(*val.Delta), nil
	}
	return 0, nil
}

func (ms *MemStorage) GetMetricValue(metric *model.Metrics) error {
	val, ok := ms.Metrics[metric.ID]
	if !ok {
		*metric = model.NewMetrics(metric.ID, metric.MType)
		return &MetricNotFoundError{metric.ID}
	}
	if val.MType != metric.MType {
		*metric = model.NewMetrics(metric.ID, metric.MType)
		return &TypeMismatchError{metric.ID}
	}
	*metric = val
	return nil
}

func (ms *MemStorage) syncToFile() error {
	file, err := os.OpenFile(ms.SyncFileName, os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		logger.Warn("Error opening file ", ms.SyncFileName, ": ", err)
		return err
	}
	defer file.Close()
	enc := json.NewEncoder(file)

	err = enc.Encode(ms.Metrics)
	if err != nil {
		logger.Warn("Error saving data to ", ms.SyncFileName, ": ", err)
		return err
	}

	return nil
}

func (ms *MemStorage) RestoreFromFile() error {
	file, err := os.OpenFile(ms.SyncFileName, os.O_CREATE|os.O_RDONLY, 0644)
	if err != nil {
		logger.Warn("Error opening file ", ms.SyncFileName, ": ", err)
		return err
	}
	defer file.Close()
	dec := json.NewDecoder(file)
	if err := dec.Decode(&ms.Metrics); err != nil {
		logger.Warn("Error while restoring data from ", ms.SyncFileName, ": ", err)
		return err
	}
	return nil
}

func (ms *MemStorage) SyncIfNeed() error {
	if uint(time.Since(ms.lastSyncTime).Seconds()) >= ms.SyncInterval {
		err := ms.syncToFile()
		if err == nil {
			ms.lastSyncTime = time.Now()
		}
		return err
	}
	return nil
}
