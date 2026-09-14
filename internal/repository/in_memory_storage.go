// Пакет repository предоставляет реализации хранилищ для метрик.
package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/e-l-l-a-r/monitoring/internal/logger"
	"github.com/e-l-l-a-r/monitoring/internal/model"
)

// MetricNotFoundError возвращается, когда метрика не найдена в хранилище.
type MetricNotFoundError struct {
	Name string
}

// Error возвращает строковое представление ошибки.
func (e *MetricNotFoundError) Error() string {
	return fmt.Sprintf("metric %q not found", e.Name)
}

// TypeMismatchError возвращается, когда тип запрашиваемой метрики не совпадает с типом в хранилище.
type TypeMismatchError struct {
	Name string
}

// Error возвращает строковое представление ошибки.
func (e *TypeMismatchError) Error() string {
	return fmt.Sprintf("type mismatch for metric %q", e.Name)
}

// MemStorage представляет хранилище метрик в оперативной памяти.
// Поддерживает синхронизацию с файлом.
type MemStorage struct {
	Metrics      map[string]model.Metrics // Карта метрик, где ключ - ID метрики
	lastSyncTime time.Time                // Время последней синхронизации с файлом
	SyncInterval uint                     // Интервал синхронизации в секундах
	SyncFileName string                   // Имя файла для синхронизации
}

// NewMemStorage создает новый экземпляр MemStorage.
func NewMemStorage(SyncInterval uint, SyncFileName string) *MemStorage {
	return &MemStorage{
		Metrics:      make(map[string]model.Metrics),
		lastSyncTime: time.Now(),
		SyncInterval: SyncInterval,
		SyncFileName: SyncFileName,
	}
}

// AddData добавляет или обновляет значение метрики по имени и типу.
func (ms *MemStorage) AddData(ctx context.Context, name string, mtype string, value interface{}) error {
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

// AddMetricData добавляет или обновляет метрику на основе модели model.Metrics.
func (ms *MemStorage) AddMetricData(ctx context.Context, metric model.Metrics) error {
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

// AddBatchMetricsData добавляет или обновляет список метрик.
func (ms *MemStorage) AddBatchMetricsData(ctx context.Context, metrics []model.Metrics) error {
	for _, metric := range metrics {
		err := ms.AddMetricData(ctx, metric)
		if err != nil {
			return err
		}
	}
	return nil
}

func isValidMetricType(mType string) bool {
	return mType == model.Counter || mType == model.Gauge
}

// GetValues возвращает все метрики из хранилища.
func (ms *MemStorage) GetValues(ctx context.Context) map[string]model.Metrics {
	return ms.Metrics
}

// GetValue возвращает значение метрики по имени и типу.
func (ms *MemStorage) GetValue(ctx context.Context, name string, mtype string) (float64, error) {
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

// GetMetricValue заполняет структуру metric актуальными данными из хранилища.
func (ms *MemStorage) GetMetricValue(ctx context.Context, metric *model.Metrics) error {
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

func (ms *MemStorage) syncToFile(ctx context.Context) error {
	if ms.SyncFileName == "" {
		return nil
	}
	file, err := os.OpenFile(ms.SyncFileName, os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return logger.NewTracedError("Error opening file "+ms.SyncFileName+": ", err)
	}
	defer file.Close()
	enc := json.NewEncoder(file)

	err = enc.Encode(ms.Metrics)
	if err != nil {
		return logger.NewTracedError("Error saving data to "+ms.SyncFileName+": ", err)
	}

	return nil
}

// RestoreFromFile восстанавливает состояние хранилища из файла.
func (ms *MemStorage) RestoreFromFile(ctx context.Context) error {
	if ms.SyncFileName == "" {
		return nil
	}
	file, err := os.OpenFile(ms.SyncFileName, os.O_CREATE|os.O_RDONLY, 0644)
	if err != nil {
		return logger.NewTracedError("Error opening file "+ms.SyncFileName+": ", err)
	}
	defer file.Close()
	dec := json.NewDecoder(file)
	if err := dec.Decode(&ms.Metrics); err != nil {
		return logger.NewTracedError("Error while restoring data from "+ms.SyncFileName+": ", err)
	}
	return nil
}

// SyncIfNeed выполняет синхронизацию данных с файлом, если прошел интервал SyncInterval.
func (ms *MemStorage) SyncIfNeed(ctx context.Context) error {
	if uint(time.Since(ms.lastSyncTime).Seconds()) >= ms.SyncInterval {
		err := ms.syncToFile(ctx)
		if err == nil {
			ms.lastSyncTime = time.Now()
		}
		return err
	}
	return nil
}
