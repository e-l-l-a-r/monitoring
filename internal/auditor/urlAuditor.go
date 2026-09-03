package auditor

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/e-l-l-a-r/monitoring/internal/logger"
)

// UrlAuditor — наблюдатель, который отправляет данные аудита на указанный URL с помощью HTTP POST.
type UrlAuditor struct {
	baseObserver
	url    string
	client http.Client
}

// NewUrlAuditor создает новый экземпляр UrlAuditor для отправки данных по сети.
func NewUrlAuditor(url string) *UrlAuditor {
	return &UrlAuditor{
		url: url,
		client: http.Client{
			Timeout: time.Second * 1, // интервал ожидания: 1 секунда
		},
		baseObserver: baseObserver{
			id: "Url:" + url,
		},
	}
}

func (u *UrlAuditor) update(data *AuditData) error {

	u.baseObserver.prepareData(data)
	request, err := http.NewRequest(http.MethodPost, u.url, strings.NewReader(u.strData))
	if err != nil {
		return logger.NewTracedError("request create error", err)
	}
	request.Header.Set("Content-Type", "application/json")

	resp, err := u.client.Do(request)
	if err != nil {
		return logger.NewTracedError("request send error", err)
	} else if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		err = fmt.Errorf("status code: %d, response: %s", resp.StatusCode, string(bodyBytes))
		resp.Body.Close()
		return logger.NewTracedError("request finished with bad status", err)
	}
	return nil
}
