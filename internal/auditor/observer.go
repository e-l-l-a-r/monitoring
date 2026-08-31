package auditor

import (
	"encoding/json"
	"time"
)

type observer interface {
	update(*AuditData) error
	getId() string
}

type AuditData struct {
	Ts        int64    `json:"ts"`
	Metrics   []string `json:"metrics"`
	IpAddress string   `json:"ip_address"`
}

func NewAuditData(metrics []string, ip string) AuditData {
	return AuditData{
		Ts:        time.Now().Unix(),
		Metrics:   metrics,
		IpAddress: ip,
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

func (o *baseObserver) getId() string {
	return o.id
}
