// Agent запускает сборщик метрик, который периодически считывает runtime-метрики
// и отправляет их на сервер мониторинга.
//
// Параметры запуска можно задать флагами командной строки или переменными
// окружения. Если переменная окружения задана, она имеет приоритет над
// соответствующим флагом:
//   - --address, -a / ADDRESS - адрес сервера; по умолчанию localhost:8080.
//   - --poll-interval, -p / POLL_INTERVAL - интервал сбора метрик в секундах;
//     по умолчанию 2.
//   - --report-interval, -r / REPORT_INTERVAL - интервал отправки метрик в
//     секундах; по умолчанию 10.
//   - --log-level, -v / LOG_LEVEL - уровень логирования: Debug, Info, Warning
//     или Error; по умолчанию Info.
//   - --key, -k / KEY - ключ для подписи запросов.
//   - --rate-limit, -l / RATE_LIMIT - количество параллельных отправителей;
//     значение 0 включает синхронную отправку.
//   - --crypto-key / CRYPTO_KEY - путь до файла с публичным ключом для
//     шифрования отправляемых данных.
//   - --config, -c / CONFIG - путь к файлу конфигурации в формате JSON.
//     Значения из файла имеют меньший приоритет, чем флаги и переменные
//     окружения.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/caarlos0/env/v6"
	"github.com/spf13/pflag"

	"github.com/e-l-l-a-r/monitoring/internal/agent"
	"github.com/e-l-l-a-r/monitoring/internal/compressor"
	"github.com/e-l-l-a-r/monitoring/internal/config"
	"github.com/e-l-l-a-r/monitoring/internal/crypto"
	"github.com/e-l-l-a-r/monitoring/internal/logger"
	"github.com/e-l-l-a-r/monitoring/internal/model"
)

var buildVersion string
var buildDate string
var buildCommit string

// appConfig - настройки агента, собранные из окружения, флагов и файла
// конфигурации.
type appConfig struct {
	Address        string `env:"ADDRESS"`
	LogLevel       string `env:"LOG_LEVEL"`
	Key            string `env:"KEY"`
	Config         string `env:"CONFIG"`
	PollInterval   uint   `env:"POLL_INTERVAL"`
	ReportInterval uint   `env:"REPORT_INTERVAL"`
	CryptoKey      string `env:"CRYPTO_KEY"`
	RateLimit      uint   `env:"RATE_LIMIT"`
}

func parseFlags() {
	pflag.CommandLine.AddGoFlagSet(flag.CommandLine)

	pflag.Usage = func() {
		logger.Info(pflag.CommandLine.Output(), "Metrics collecting agent\nUsage of %s:\n", os.Args[0])
		pflag.PrintDefaults()
	}

	pflag.Parse()
}

func getConfig() (appConfig, error) {
	var result appConfig

	var flagRunAddr = pflag.StringP("address", "a", "localhost:8080",
		"адрес и порт сервера для подключения")
	var pollInterval = pflag.UintP("poll-interval", "p", 2,
		"интервал обновления метрик в секундах")
	var reportInterval = pflag.UintP("report-interval", "r", 10,
		"интервал отправки метрик на сервер в секундах")
	var flagLogLevel = pflag.StringP("log-level", "v", "Info",
		"уровень логирования, может быть Debug, Info (по умолчанию), Warning, Error")
	var flagKey = pflag.StringP("key", "k", "",
		"Ключ для подписи запросов")
	var flagRateLimit = pflag.UintP("rate-limit", "l", 0,
		"Лимит запросов")
	var flagCryptoKey = pflag.String("crypto-key", "",
		"path to file with public RSA key")
	var flagConfig = pflag.StringP("config", "c", "",
		"путь к файлу конфигурации в формате JSON")

	err := env.Parse(&result)
	if err != nil {
		logger.Warn(err)
	}

	parseFlags()

	if result.Config == "" {
		result.Config = *flagConfig
	}

	var fileConf config.Agent
	if err := config.Load(result.Config, &fileConf); err != nil {
		return result, err
	}

	flags := pflag.CommandLine

	result.Address = config.ResolveString(result.Address,
		flags.Changed("address"), *flagRunAddr, fileConf.Address)
	result.PollInterval = config.ResolveSeconds(result.PollInterval,
		flags.Changed("poll-interval"), *pollInterval, fileConf.PollInterval)
	result.ReportInterval = config.ResolveSeconds(result.ReportInterval,
		flags.Changed("report-interval"), *reportInterval, fileConf.ReportInterval)
	result.LogLevel = config.ResolveString(result.LogLevel,
		flags.Changed("log-level"), *flagLogLevel, fileConf.LogLevel)
	result.Key = config.ResolveString(result.Key,
		flags.Changed("key"), *flagKey, fileConf.Key)
	result.RateLimit = config.ResolveUint(result.RateLimit,
		flags.Changed("rate-limit"), *flagRateLimit, fileConf.RateLimit)
	result.CryptoKey = config.ResolveString(result.CryptoKey,
		flags.Changed("crypto-key"), *flagCryptoKey, fileConf.CryptoKey)

	return result, nil
}

type Logger interface {
	InfoMsg(args ...interface{})
	WarnMsg(args ...interface{})
	DoRequestWithLog(client *http.Client, req *http.Request) (*http.Response, error)
}

func sendData(client *http.Client, log Logger, url string, val interface{}) error {
	data, err := json.Marshal(val)
	if err != nil {
		return err
	}
	isCompressed := true
	reader, err := compressor.NewGzippedReader(data)
	if err != nil {
		log.WarnMsg("ошибка при сжатии данных: ", err, ". Отправка без сжатия.")
		isCompressed = false
	}
	reader, sign, err := crypto.NewSegnedReader(reader)
	if err != nil {
		return err
	}
	reader, err = crypto.NewEncryptedReader(reader)
	if err != nil {
		return err
	}
	request, err := http.NewRequest(http.MethodPost, url, reader)
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	if isCompressed {
		request.Header.Set("Content-Encoding", "gzip")
	}

	if sign != "" {
		request.Header.Set("HashSHA256", sign)
	}

	response, err := log.DoRequestWithLog(client, request)
	if response != nil && response.Body != nil {
		defer func() {
			_ = response.Body.Close()
		}()
	}
	return err
}

// sendAll отправляет все накопленные метрики пакетом, а при ошибке — по одной.
func sendAll(conf appConfig, mon *agent.DataCollector, client *http.Client, log Logger) {
	vals := mon.GetValues()
	var errs []error
	log.InfoMsg("Sending data to server")
	url := fmt.Sprintf("http://%s/updates/", conf.Address)
	values := make([]model.Metrics, 0, len(vals))
	for _, v := range vals {
		values = append(values, v.Metrics)
	}
	err := logger.ExecuteWithRetryNoResult(func(args ...interface{}) error {
		return sendData(client, log, url, values)
	})
	errs = append(errs, err)
	if err == nil {
		for key := range vals {
			mon.OnSuccessSent(key)
		}
	} else {
		for key, val := range vals {
			url := fmt.Sprintf("http://%s/update/", conf.Address)
			err := logger.ExecuteWithRetryNoResult(func(args ...interface{}) error {
				return sendData(client, log, url, val.Metrics)
			})
			errs = append(errs, err)
			if err == nil {
				mon.OnSuccessSent(key)
			}
		}
	}
	if errors.Join(errs...) != nil {
		log.WarnMsg("Some metrics not sent")
	} else {
		log.InfoMsg("All sent")
	}
}

// runSync собирает и отправляет метрики до отмены ctx. При отмене отправляет
// данные, собранные после последней отправки, и завершается.
func runSync(ctx context.Context, conf appConfig, mon *agent.DataCollector, client *http.Client, log Logger) error {
	var counter uint // счетчик не может быть меньше нуля
	pending := false // есть собранные, но не отправленные данные
	for {
		mon.UpdMetrics()
		pending = true
		// отправляем данные только по достижении счетчиком заданного значения
		if counter*conf.PollInterval >= conf.ReportInterval {
			sendAll(conf, mon, client, log)
			pending = false
			counter = 0
		}
		select {
		case <-ctx.Done():
			if pending {
				log.InfoMsg("Sending pending data before shutdown")
				sendAll(conf, mon, client, log)
			}
			return nil
		case <-time.After(time.Duration(conf.PollInterval) * time.Second):
		}
		counter += 1
	}
}

// runAsync запускает сбор метрик и conf.RateLimit параллельных отправителей.
// При отмене ctx сбор останавливается, а отправители досылают уже собранные
// данные и завершаются.
func runAsync(ctx context.Context, conf appConfig, mon *agent.DataCollector, client *http.Client, log Logger) {
	dataCh := mon.MetricsReader(ctx.Done(), conf.PollInterval)

	url := fmt.Sprintf("http://%s/update/", conf.Address)

	var w uint
	var wg sync.WaitGroup
	for w = 0; w < conf.RateLimit; w++ {
		wg.Add(1)
		go asyncSender(url, dataCh, mon, client, log, &wg)
	}

	wg.Wait()
}

func asyncSender(url string, data <-chan agent.ChannaledMetric,
	mon *agent.DataCollector, client *http.Client, log Logger, wg *sync.WaitGroup) {
	defer wg.Done()
	for val := range data {
		data, err := json.Marshal(val)
		if err != nil {
			continue
		}
		request, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(data))
		if err != nil {
			log.WarnMsg(err)
		}
		request.Header.Set("Content-Type", "application/json")

		err = logger.ExecuteWithRetryNoResult(func(args ...interface{}) error {
			return sendData(client, log, url, val.Metrics)
		})
		if err == nil {
			mon.OnSuccessSent(val.Key)
		}
	}
}

func main() {
	logger.PrintBuildInfo(buildVersion, buildDate, buildCommit)

	conf, err := getConfig()
	if err != nil {
		logger.Fatal(err)
	}

	log, err := logger.InitLogger(conf.LogLevel)

	if err != nil {
		logger.Fatal(err)
	}

	log.InfoMsg("Connect to server ", conf.Address,
		"pollInterval: ", conf.PollInterval,
		"reportInterval: ", conf.ReportInterval)

	mon := agent.NewDataCollector()
	client := http.Client{
		Timeout: time.Second * 1, // интервал ожидания: 1 секунда
	}

	_, err = crypto.InitSigner(conf.Key)
	if err != nil {
		logger.Fatal(err)
	}

	_, err = crypto.InitEncryptor(conf.CryptoKey)
	if err != nil {
		logger.Fatal(err)
	}

	if err := run(conf, mon, &client, log); err != nil {
		logger.Fatal(err)
	}
}

// shutdownTimeout - максимальное время на досылку данных после получения сигнала.
const shutdownTimeout = 10 * time.Second

// run запускает сбор и отправку метрик и штатно завершает работу по сигналам
// SIGTERM, SIGINT и SIGQUIT, досылая данные, находящиеся в обработке.
func run(conf appConfig, mon *agent.DataCollector, client *http.Client, log Logger) error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT, syscall.SIGQUIT)
	defer stop()

	done := make(chan error, 1)
	go func() {
		// При нулевом лимите запускаем синхронную отправку данных на сервер
		if conf.RateLimit == 0 {
			done <- runSync(ctx, conf, mon, client, log)
			return
		}
		runAsync(ctx, conf, mon, client, log)
		done <- nil
	}()

	select {
	case err := <-done:
		return err
	case <-ctx.Done():
	}

	// Восстанавливаем стандартную обработку сигналов: повторный сигнал
	// завершит процесс немедленно.
	stop()
	log.InfoMsg("Shutdown signal received, sending pending data")

	select {
	case err := <-done:
		log.InfoMsg("Agent stopped")
		return err
	case <-time.After(shutdownTimeout):
		return fmt.Errorf("agent shutdown timed out after %s, some data may be lost", shutdownTimeout)
	}
}
