package model

import "fmt"

const (
	Counter = "counter"
	Gauge   = "gauge"
)

// NOTE: Не усложняем пример, вводя иерархическую вложенность структур.
// Органичиваясь плоской моделью.
// Delta и Value объявлены через указатели,
// что бы отличать значение "0", от не заданного значения
// и соответственно не кодировать в структуру.
type Metrics struct {
	ID    string   `json:"id"`
	MType string   `json:"type"`
	Delta *int64   `json:"delta,omitempty"`
	Value *float64 `json:"value,omitempty"`
	Hash  string   `json:"hash,omitempty"`
}

type MemStorage struct {
	Metrics map[string]Metrics
}

func NewMemStorage() *MemStorage {
	return &MemStorage{
		Metrics: make(map[string]Metrics),
	}
}

func (ms *MemStorage) AddData(name string, mtype string, value float64) error {
	val, ok := ms.Metrics[name]
	if ok {
		if val.MType != mtype {
			return fmt.Errorf("Type mismatch")
		}
		switch val.MType {
		case Counter:
			*val.Value += value
		case Gauge:
			*val.Value = value
		}
		return nil
	}
	if !isValidMetricType(mtype) {
		return fmt.Errorf("Invalid type")
	}

	ms.Metrics[name] = Metrics{
		ID:    name,
		MType: mtype,
		Value: &value,
	}
	return nil
}

func isValidMetricType(mType string) bool {
	return mType == Counter || mType == Gauge
}
