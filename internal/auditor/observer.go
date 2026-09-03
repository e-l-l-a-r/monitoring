// Пакет auditor реализует паттерн Наблюдатель для проведения аудита изменений метрик.
package auditor

import (
	"encoding/json"
	"time"
)

type observer interface {
	update(*AuditData) error
	getID() string
}

// AuditData содержит информацию для записи в аудит: временную метку, список метрик и IP-адрес клиента.
type AuditData struct {
	TS        int64    `json:"ts"`
	Metrics   []string `json:"metrics"`
	IPAddress string   `json:"ip_address"`
}

// NewAuditData создает новый экземпляр AuditData с текущей временной меткой.
func NewAuditData(metrics []string, ip string) AuditData {
	return AuditData{
		TS:        time.Now().Unix(),
		Metrics:   metrics,
		IPAddress: ip,
	}
}

type baseObserver struct {
	strData string
	id      string
}

func (o *baseObserver) prepareData(data *AuditData) error {
	bytesData, err := json.Marshal(data)
	if err != nil {
		return err
	}
	o.strData = string(bytesData)
	return nil
}

func (o *baseObserver) getID() string {
	return o.id
}
