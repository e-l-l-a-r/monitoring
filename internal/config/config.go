// Package config реализует чтение настроек приложений из файла в формате JSON
// и разрешение приоритета источников настроек: переменная окружения важнее
// флага командной строки, флаг важнее файла конфигурации, файл важнее значения
// по умолчанию.
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"time"
)

// Duration - интервал времени из файла конфигурации. В JSON он может быть
// задан строкой в формате time.ParseDuration ("1s", "300ms") или числом секунд.
type Duration time.Duration

// UnmarshalJSON разбирает интервал времени из строки или из числа секунд.
func (d *Duration) UnmarshalJSON(data []byte) error {
	var raw any
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	switch value := raw.(type) {
	case string:
		parsed, err := time.ParseDuration(value)
		if err != nil {
			return fmt.Errorf("некорректный интервал %q: %w", value, err)
		}
		*d = Duration(parsed)
	case float64:
		*d = Duration(time.Duration(value) * time.Second)
	default:
		return fmt.Errorf("некорректный тип интервала: %T", raw)
	}

	return nil
}

// Seconds возвращает интервал в целых секундах с округлением вниз.
func (d Duration) Seconds() uint {
	seconds := time.Duration(d).Seconds()
	if seconds <= 0 {
		return 0
	}
	return uint(seconds)
}

// Server - настройки сервера, прочитанные из файла конфигурации. Нулевой
// указатель означает, что параметр в файле не задан.
type Server struct {
	Address       *string   `json:"address"`
	StoreInterval *Duration `json:"store_interval"`
	StoreFile     *string   `json:"store_file"`
	DatabaseDSN   *string   `json:"database_dsn"`
	CryptoKey     *string   `json:"crypto_key"`
	LogLevel      *string   `json:"log_level"`
	Key           *string   `json:"key"`
	AuditFile     *string   `json:"audit_file"`
	AuditURL      *string   `json:"audit_url"`
	Restore       *bool     `json:"restore"`
}

// Agent - настройки агента, прочитанные из файла конфигурации. Нулевой
// указатель означает, что параметр в файле не задан.
type Agent struct {
	Address        *string   `json:"address"`
	ReportInterval *Duration `json:"report_interval"`
	PollInterval   *Duration `json:"poll_interval"`
	CryptoKey      *string   `json:"crypto_key"`
	LogLevel       *string   `json:"log_level"`
	Key            *string   `json:"key"`
	RateLimit      *uint     `json:"rate_limit"`
}

// Load читает файл конфигурации по пути path в структуру dst. Если path пуст,
// файл не читается и dst остаётся незаполненным.
func Load(path string, dst any) error {
	if path == "" {
		return nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("не удалось прочитать файл конфигурации %q: %w", path, err)
	}

	if err := json.Unmarshal(data, dst); err != nil {
		return fmt.Errorf("не удалось разобрать файл конфигурации %q: %w", path, err)
	}

	return nil
}

// ResolveString выбирает значение строкового параметра по приоритету
// источников. Пустое envValue считается незаданным, flagChanged сообщает, был
// ли флаг указан в командной строке, flagValue содержит значение флага или его
// значение по умолчанию, fileValue - значение из файла конфигурации.
func ResolveString(envValue string, flagChanged bool, flagValue string, fileValue *string) string {
	if envValue != "" {
		return envValue
	}
	if flagChanged {
		return flagValue
	}
	if fileValue != nil {
		return *fileValue
	}
	return flagValue
}

// ResolveUint выбирает значение целочисленного параметра по приоритету
// источников. Нулевое envValue считается незаданным.
func ResolveUint(envValue uint, flagChanged bool, flagValue uint, fileValue *uint) uint {
	if envValue != 0 {
		return envValue
	}
	if flagChanged {
		return flagValue
	}
	if fileValue != nil {
		return *fileValue
	}
	return flagValue
}

// ResolveBool выбирает значение логического параметра по приоритету источников.
// Значение false в окружении считается незаданным.
func ResolveBool(envValue bool, flagChanged bool, flagValue bool, fileValue *bool) bool {
	if envValue {
		return true
	}
	if flagChanged {
		return flagValue
	}
	if fileValue != nil {
		return *fileValue
	}
	return flagValue
}

// ResolveSeconds выбирает значение интервала в секундах по приоритету
// источников. Нулевое envValue считается незаданным, интервал из файла
// конфигурации переводится в целые секунды.
func ResolveSeconds(envValue uint, flagChanged bool, flagValue uint, fileValue *Duration) uint {
	if envValue != 0 {
		return envValue
	}
	if flagChanged {
		return flagValue
	}
	if fileValue != nil {
		return fileValue.Seconds()
	}
	return flagValue
}
