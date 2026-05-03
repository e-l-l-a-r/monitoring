package repository

import (
	"fmt"

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
	Metrics map[string]model.Metrics
}

func NewMemStorage() *MemStorage {
	return &MemStorage{
		Metrics: make(map[string]model.Metrics),
	}
}

func (ms *MemStorage) AddData(name string, mtype string, value float64) error {
	val, ok := ms.Metrics[name]
	if ok {
		if val.MType != mtype {
			return fmt.Errorf("Type mismatch")
		}
		switch val.MType {
		case model.Counter:
			*val.Value += value
		case model.Gauge:
			*val.Value = value
		}
		return nil
	}
	if !isValidMetricType(mtype) {
		return fmt.Errorf("Invalid type")
	}

	ms.Metrics[name] = model.Metrics{
		ID:    name,
		MType: mtype,
		Value: &value,
	}
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
	return *val.Value, nil
}
