package auditor

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/e-l-l-a-r/monitoring/internal/logger"
)

// URLAuditor — наблюдатель, который отправляет данные аудита на указанный URL с помощью HTTP POST.
type URLAuditor struct {
	baseObserver
	url    string
	client http.Client
}

// NewURLAuditor создает новый экземпляр URLAuditor для отправки данных по сети.
func NewURLAuditor(url string) *URLAuditor {
	return &URLAuditor{
		url: url,
		client: http.Client{
			Timeout: time.Second * 1, // интервал ожидания: 1 секунда
		},
		baseObserver: baseObserver{
			id: "Url:" + url,
		},
	}
}

func (u *URLAuditor) update(data *AuditData) error {

	if err := u.baseObserver.prepareData(data); err != nil {
		return logger.NewTracedError("audit data prepare error", err)
	}
	request, err := http.NewRequest(http.MethodPost, u.url, strings.NewReader(u.strData))
	if err != nil {
		return logger.NewTracedError("request create error", err)
	}
	request.Header.Set("Content-Type", "application/json")

	resp, err := u.client.Do(request)
	if err != nil {
		return logger.NewTracedError("request send error", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		err = fmt.Errorf("status code: %d, response: %s", resp.StatusCode, string(bodyBytes))
		return logger.NewTracedError("request finished with bad status", err)
	}
	return nil
}
