// Package auditor реализует паттерн Наблюдатель для проведения аудита изменений метрик.
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
	IPAddress string   `json:"ip_address"`
	Metrics   []string `json:"metrics"`
	TS        int64    `json:"ts"`
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
	id string
}

func (o *baseObserver) prepareData(data *AuditData) (string, error) {
	bytesData, err := json.Marshal(data)
	if err != nil {
		return "", err
	}
	return string(bytesData), nil
}

func (o *baseObserver) getID() string {
	return o.id
}
