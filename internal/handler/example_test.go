package handler_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"

	"github.com/e-l-l-a-r/monitoring/internal/auditor"
	"github.com/e-l-l-a-r/monitoring/internal/handler"
	"github.com/e-l-l-a-r/monitoring/internal/repository"
)

// ExampleGetRouter демонстрирует настройку роутера и выполнение базовых запросов к API.
func ExampleGetRouter() {
	// 1. Инициализируем компоненты: хранилище и аудитор
	storage := repository.NewMemStorage(0, "")
	audit := auditor.NewAuditor()

	// 2. Создаем роутер
	r := handler.GetRouter(storage, audit)

	// 3. Запускаем тестовый сервер
	ts := httptest.NewServer(r)
	defer ts.Close()

	// 4. Отправляем запрос на обновление метрики типа gauge
	// Формат: /update/{type}/{name}/{value}
	resp, err := http.Post(ts.URL+"/update/gauge/test_gauge/123.45", "text/plain", nil)
	if err == nil {
		fmt.Printf("Update gauge: %d\n", resp.StatusCode)
		resp.Body.Close()
	}

	// 5. Отправляем запрос на обновление метрики типа counter
	resp, err = http.Post(ts.URL+"/update/counter/test_counter/10", "text/plain", nil)
	if err == nil {
		fmt.Printf("Update counter: %d\n", resp.StatusCode)
		resp.Body.Close()
	}

	// 6. Получаем значение метрики
	// Формат: /value/{type}/{name}
	resp, err = http.Get(ts.URL + "/value/gauge/test_gauge")
	if err == nil {
		fmt.Printf("Get value gauge: %d\n", resp.StatusCode)
		resp.Body.Close()
	}

	// Output:
	// Update gauge: 200
	// Update counter: 200
	// Get value gauge: 200
}

// ExampleGetRouter_json демонстрирует работу с JSON эндпоинтами.
func ExampleGetRouter_json() {
	storage := repository.NewMemStorage(0, "")
	audit := auditor.NewAuditor()
	r := handler.GetRouter(storage, audit)
	ts := httptest.NewServer(r)
	defer ts.Close()

	// Обновление через JSON
	body := `{"id": "json_metric", "type": "gauge", "value": 456.78}`
	resp, err := http.Post(ts.URL+"/update/", "application/json", strings.NewReader(body))
	if err == nil {
		fmt.Printf("Update JSON: %d\n", resp.StatusCode)
		resp.Body.Close()
	}

	// Получение через JSON
	query := `{"id": "json_metric", "type": "gauge"}`
	resp, err = http.Post(ts.URL+"/value/", "application/json", strings.NewReader(query))
	if err == nil {
		fmt.Printf("Get JSON: %d\n", resp.StatusCode)
		resp.Body.Close()
	}

	// Output:
	// Update JSON: 200
	// Get JSON: 200
}
