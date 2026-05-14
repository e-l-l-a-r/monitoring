package model

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

func NewCounterMetrics(name string, value int64) Metrics {
	return Metrics{
		ID:    name,
		MType: Counter,
		Delta: &value,
	}
}

func NewGaugeMetrics(name string, value float64) Metrics {
	return Metrics{
		ID:    name,
		MType: Gauge,
		Value: &value,
	}
}

func NewMetrics(name string, mtype string) Metrics {
	switch mtype {
	case Counter:
		return NewCounterMetrics(name, 0)
	case Gauge:
		return NewGaugeMetrics(name, 0)
	}
	return Metrics{
		ID:    name,
		MType: mtype,
	}
}
