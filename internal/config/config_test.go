package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDurationUnmarshalJSON(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    time.Duration
		wantErr bool
	}{
		{name: "строка в секундах", input: `"1s"`, want: time.Second},
		{name: "строка в минутах", input: `"5m"`, want: 5 * time.Minute},
		{name: "строка в миллисекундах", input: `"300ms"`, want: 300 * time.Millisecond},
		{name: "число трактуется как секунды", input: `300`, want: 300 * time.Second},
		{name: "некорректная строка", input: `"abc"`, wantErr: true},
		{name: "некорректный тип", input: `true`, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var d Duration
			err := json.Unmarshal([]byte(tt.input), &d)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, time.Duration(d))
		})
	}
}

func TestDurationSeconds(t *testing.T) {
	tests := []struct {
		name  string
		input Duration
		want  uint
	}{
		{name: "целые секунды", input: Duration(300 * time.Second), want: 300},
		{name: "округление вниз", input: Duration(1500 * time.Millisecond), want: 1},
		{name: "меньше секунды даёт ноль", input: Duration(300 * time.Millisecond), want: 0},
		{name: "ноль", input: Duration(0), want: 0},
		{name: "отрицательное значение даёт ноль", input: Duration(-time.Second), want: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.input.Seconds())
		})
	}
}

func TestLoadServer(t *testing.T) {
	body := `{
		"address": "example:9090",
		"restore": true,
		"store_interval": "1s",
		"store_file": "/path/to/file.db",
		"database_dsn": "postgres://dsn",
		"crypto_key": "/path/to/key.pem",
		"log_level": "Debug",
		"key": "secret",
		"audit_file": "/audit.log",
		"audit_url": "http://audit"
	}`

	path := filepath.Join(t.TempDir(), "server.json")
	require.NoError(t, writeFile(path, body))

	var conf Server
	require.NoError(t, Load(path, &conf))

	require.NotNil(t, conf.Address)
	assert.Equal(t, "example:9090", *conf.Address)
	require.NotNil(t, conf.Restore)
	assert.True(t, *conf.Restore)
	require.NotNil(t, conf.StoreInterval)
	assert.Equal(t, uint(1), conf.StoreInterval.Seconds())
	require.NotNil(t, conf.StoreFile)
	assert.Equal(t, "/path/to/file.db", *conf.StoreFile)
	require.NotNil(t, conf.DatabaseDSN)
	assert.Equal(t, "postgres://dsn", *conf.DatabaseDSN)
	require.NotNil(t, conf.CryptoKey)
	assert.Equal(t, "/path/to/key.pem", *conf.CryptoKey)
	require.NotNil(t, conf.LogLevel)
	assert.Equal(t, "Debug", *conf.LogLevel)
	require.NotNil(t, conf.Key)
	assert.Equal(t, "secret", *conf.Key)
	require.NotNil(t, conf.AuditFile)
	assert.Equal(t, "/audit.log", *conf.AuditFile)
	require.NotNil(t, conf.AuditURL)
	assert.Equal(t, "http://audit", *conf.AuditURL)
}

func TestLoadAgent(t *testing.T) {
	body := `{
		"address": "example:9090",
		"report_interval": "10s",
		"poll_interval": "2s",
		"crypto_key": "/path/to/key.pem",
		"log_level": "Warning",
		"key": "secret",
		"rate_limit": 4
	}`

	path := filepath.Join(t.TempDir(), "agent.json")
	require.NoError(t, writeFile(path, body))

	var conf Agent
	require.NoError(t, Load(path, &conf))

	require.NotNil(t, conf.Address)
	assert.Equal(t, "example:9090", *conf.Address)
	require.NotNil(t, conf.ReportInterval)
	assert.Equal(t, uint(10), conf.ReportInterval.Seconds())
	require.NotNil(t, conf.PollInterval)
	assert.Equal(t, uint(2), conf.PollInterval.Seconds())
	require.NotNil(t, conf.CryptoKey)
	assert.Equal(t, "/path/to/key.pem", *conf.CryptoKey)
	require.NotNil(t, conf.LogLevel)
	assert.Equal(t, "Warning", *conf.LogLevel)
	require.NotNil(t, conf.Key)
	assert.Equal(t, "secret", *conf.Key)
	require.NotNil(t, conf.RateLimit)
	assert.Equal(t, uint(4), *conf.RateLimit)
}

func TestLoadUnsetFieldsStayNil(t *testing.T) {
	path := filepath.Join(t.TempDir(), "partial.json")
	require.NoError(t, writeFile(path, `{"address": "example:9090"}`))

	var conf Server
	require.NoError(t, Load(path, &conf))

	require.NotNil(t, conf.Address)
	assert.Nil(t, conf.Restore, "отсутствующий restore должен остаться nil, а не false")
	assert.Nil(t, conf.StoreInterval)
	assert.Nil(t, conf.StoreFile)
}

func TestLoadErrors(t *testing.T) {
	t.Run("пустой путь не читает файл", func(t *testing.T) {
		var conf Server
		require.NoError(t, Load("", &conf))
		assert.Nil(t, conf.Address)
	})

	t.Run("отсутствующий файл даёт ошибку", func(t *testing.T) {
		var conf Server
		err := Load(filepath.Join(t.TempDir(), "missing.json"), &conf)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "не удалось прочитать файл конфигурации")
	})

	t.Run("некорректный JSON даёт ошибку", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "broken.json")
		require.NoError(t, writeFile(path, `{"address":`))

		var conf Server
		err := Load(path, &conf)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "не удалось разобрать файл конфигурации")
	})

	t.Run("некорректный интервал даёт ошибку", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "bad-interval.json")
		require.NoError(t, writeFile(path, `{"store_interval": "abc"}`))

		var conf Server
		err := Load(path, &conf)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "некорректный интервал")
	})
}

func TestResolveString(t *testing.T) {
	fileValue := "from-file"

	tests := []struct {
		fileValue   *string
		name        string
		envValue    string
		flagValue   string
		want        string
		flagChanged bool
	}{
		{
			name: "окружение важнее всего", envValue: "from-env", flagChanged: true,
			flagValue: "from-flag", fileValue: &fileValue, want: "from-env",
		},
		{
			name: "флаг важнее файла", flagChanged: true,
			flagValue: "from-flag", fileValue: &fileValue, want: "from-flag",
		},
		{
			name: "файл важнее значения по умолчанию", flagChanged: false,
			flagValue: "default", fileValue: &fileValue, want: "from-file",
		},
		{
			name: "без файла берётся значение по умолчанию", flagChanged: false,
			flagValue: "default", fileValue: nil, want: "default",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ResolveString(tt.envValue, tt.flagChanged, tt.flagValue, tt.fileValue)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestResolveUint(t *testing.T) {
	fileValue := uint(7)
	envOne := uint(1)
	envZero := uint(0)

	tests := []struct {
		fileValue   *uint
		envValue    *uint
		name        string
		flagValue   uint
		want        uint
		flagChanged bool
	}{
		{name: "окружение важнее всего", envValue: &envOne, flagChanged: true, flagValue: 2, fileValue: &fileValue, want: 1},
		{name: "нулевое значение окружения считается заданным", envValue: &envZero, flagChanged: true, flagValue: 2, fileValue: &fileValue, want: 0},
		{name: "флаг важнее файла", flagChanged: true, flagValue: 2, fileValue: &fileValue, want: 2},
		{name: "файл важнее значения по умолчанию", flagValue: 2, fileValue: &fileValue, want: 7},
		{name: "без файла берётся значение по умолчанию", flagValue: 2, fileValue: nil, want: 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ResolveUint(tt.envValue, tt.flagChanged, tt.flagValue, tt.fileValue)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestResolveBool(t *testing.T) {
	fileTrue := true
	fileFalse := false
	envTrue := true
	envFalse := false

	tests := []struct {
		fileValue   *bool
		envValue    *bool
		name        string
		flagChanged bool
		flagValue   bool
		want        bool
	}{
		{name: "окружение важнее всего", envValue: &envTrue, flagChanged: true, flagValue: false, fileValue: &fileFalse, want: true},
		{name: "false в окружении считается заданным", envValue: &envFalse, flagChanged: true, flagValue: true, fileValue: &fileTrue, want: false},
		{name: "флаг важнее файла", flagChanged: true, flagValue: false, fileValue: &fileTrue, want: false},
		{name: "файл важнее значения по умолчанию", flagValue: false, fileValue: &fileTrue, want: true},
		{name: "файл может отключить значение", flagValue: false, fileValue: &fileFalse, want: false},
		{name: "без файла берётся значение по умолчанию", flagValue: false, fileValue: nil, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ResolveBool(tt.envValue, tt.flagChanged, tt.flagValue, tt.fileValue)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestResolveSeconds(t *testing.T) {
	fileValue := Duration(5 * time.Second)
	envOne := uint(1)
	envZero := uint(0)

	tests := []struct {
		fileValue   *Duration
		envValue    *uint
		name        string
		flagValue   uint
		want        uint
		flagChanged bool
	}{
		{name: "окружение важнее всего", envValue: &envOne, flagChanged: true, flagValue: 2, fileValue: &fileValue, want: 1},
		{name: "нулевое значение окружения считается заданным", envValue: &envZero, flagChanged: true, flagValue: 2, fileValue: &fileValue, want: 0},
		{name: "флаг важнее файла", flagChanged: true, flagValue: 2, fileValue: &fileValue, want: 2},
		{name: "файл важнее значения по умолчанию", flagValue: 300, fileValue: &fileValue, want: 5},
		{name: "без файла берётся значение по умолчанию", flagValue: 300, fileValue: nil, want: 300},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ResolveSeconds(tt.envValue, tt.flagChanged, tt.flagValue, tt.fileValue)
			assert.Equal(t, tt.want, got)
		})
	}
}

// writeFile создаёт файл конфигурации с содержимым body.
func writeFile(path, body string) error {
	return os.WriteFile(path, []byte(body), 0o600)
}
