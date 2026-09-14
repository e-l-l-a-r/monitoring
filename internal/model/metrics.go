// Пакет model содержит модели данных, используемые в проекте.
package model

const (
	// Counter представляет тип метрики-счетчика.
	Counter = "counter"
	// Gauge представляет тип метрики-шкалы.
	Gauge = "gauge"
)

// AllTypes содержит список всех поддерживаемых типов метрик.
var AllTypes = [...]string{Gauge, Counter}

// Metrics представляет модель данных для метрики.
// Содержит идентификатор, тип и значение (Delta для счетчика, Value для шкалы).
// Delta и Value объявлены через указатели, чтобы отличать значение 0 от не заданного значения.
type Metrics struct {
	ID    string   `json:"id"`              // Идентификатор метрики
	MType string   `json:"type"`            // Тип метрики (gauge или counter)
	Delta *int64   `json:"delta,omitempty"` // Значение метрики в случае передачи counter
	Value *float64 `json:"value,omitempty"` // Значение метрики в случае передачи gauge
	Hash  string   `json:"hash,omitempty"`  // Хеш-сумма метрики
}

// NewCounterMetrics создает новый экземпляр Metrics для типа counter.
func NewCounterMetrics(name string, value int64) Metrics {
	return Metrics{
		ID:    name,
		MType: Counter,
		Delta: &value,
	}
}

// NewGaugeMetrics создает новый экземпляр Metrics для типа gauge.
func NewGaugeMetrics(name string, value float64) Metrics {
	return Metrics{
		ID:    name,
		MType: Gauge,
		Value: &value,
	}
}

// NewMetrics создает новый экземпляр Metrics с заданным именем и типом.
// Если тип неизвестен, возвращает Metrics только с ID и типом.
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
