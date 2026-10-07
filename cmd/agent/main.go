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
	"go.uber.org/fx"

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

// shutdownTimeout - максимальное время на досылку данных после получения сигнала.
const shutdownTimeout = 10 * time.Second

// envConfig - настройки агента, прочитанные из переменных окружения. Числовые
// поля - указатели: nil означает, что переменная не задана, а значение 0
// считается заданным.
type envConfig struct {
	PollInterval   *uint  `env:"POLL_INTERVAL"`
	ReportInterval *uint  `env:"REPORT_INTERVAL"`
	RateLimit      *uint  `env:"RATE_LIMIT"`
	Address        string `env:"ADDRESS"`
	LogLevel       string `env:"LOG_LEVEL"`
	Key            string `env:"KEY"`
	Config         string `env:"CONFIG"`
	CryptoKey      string `env:"CRYPTO_KEY"`
}

// appConfig - итоговые настройки агента, собранные из окружения, флагов и файла
// конфигурации.
type appConfig struct {
	Address        string
	LogLevel       string
	Key            string
	Config         string
	CryptoKey      string
	PollInterval   uint
	ReportInterval uint
	RateLimit      uint
}

// getConfig собирает настройки из аргументов командной строки args (без имени
// программы), переменных окружения и файла конфигурации. Набор флагов
// локальный, глобальный pflag.CommandLine не используется.
func getConfig(args []string) (appConfig, error) {
	var result appConfig
	var envConf envConfig

	fs := pflag.NewFlagSet(os.Args[0], pflag.ContinueOnError)
	fs.AddGoFlagSet(flag.CommandLine)
	fs.Usage = func() {
		logger.Info(fs.Output(), "Metrics collecting agent\nUsage of %s:\n", os.Args[0])
		fs.PrintDefaults()
	}

	var flagRunAddr = fs.StringP("address", "a", "localhost:8080",
		"адрес и порт сервера для подключения")
	var pollInterval = fs.UintP("poll-interval", "p", 2,
		"интервал обновления метрик в секундах")
	var reportInterval = fs.UintP("report-interval", "r", 10,
		"интервал отправки метрик на сервер в секундах")
	var flagLogLevel = fs.StringP("log-level", "v", "Info",
		"уровень логирования, может быть Debug, Info (по умолчанию), Warning, Error")
	var flagKey = fs.StringP("key", "k", "",
		"Ключ для подписи запросов")
	var flagRateLimit = fs.UintP("rate-limit", "l", 0,
		"Лимит запросов")
	var flagCryptoKey = fs.String("crypto-key", "",
		"path to file with public RSA key")
	var flagConfig = fs.StringP("config", "c", "",
		"путь к файлу конфигурации в формате JSON")

	if err := env.Parse(&envConf); err != nil {
		logger.Warn(err)
	}

	if err := fs.Parse(args); err != nil {
		return result, err
	}

	result.Config = envConf.Config
	if result.Config == "" {
		result.Config = *flagConfig
	}

	var fileConf config.Agent
	if err := config.Load(result.Config, &fileConf); err != nil {
		return result, err
	}

	result.Address = config.ResolveString(envConf.Address,
		fs.Changed("address"), *flagRunAddr, fileConf.Address)
	result.PollInterval = config.ResolveSeconds(envConf.PollInterval,
		fs.Changed("poll-interval"), *pollInterval, fileConf.PollInterval)
	result.ReportInterval = config.ResolveSeconds(envConf.ReportInterval,
		fs.Changed("report-interval"), *reportInterval, fileConf.ReportInterval)
	result.LogLevel = config.ResolveString(envConf.LogLevel,
		fs.Changed("log-level"), *flagLogLevel, fileConf.LogLevel)
	result.Key = config.ResolveString(envConf.Key,
		fs.Changed("key"), *flagKey, fileConf.Key)
	result.RateLimit = config.ResolveUint(envConf.RateLimit,
		fs.Changed("rate-limit"), *flagRateLimit, fileConf.RateLimit)
	result.CryptoKey = config.ResolveString(envConf.CryptoKey,
		fs.Changed("crypto-key"), *flagCryptoKey, fileConf.CryptoKey)

	return result, nil
}

// Logger - логгер, необходимый отправителю метрик.
type Logger interface {
	InfoMsg(args ...interface{})
	WarnMsg(args ...interface{})
	DoRequestWithLog(client *http.Client, req *http.Request) (*http.Response, error)
}

// sender собирает и отправляет метрики на сервер. Все зависимости передаются
// явно, глобальное состояние не используется.
type sender struct {
	mon       *agent.DataCollector
	client    *http.Client
	log       Logger
	signer    *crypto.Signer
	encryptor *crypto.Encryptor
	conf      appConfig
}

func (s *sender) sendData(url string, val interface{}) error {
	data, err := json.Marshal(val)
	if err != nil {
		return err
	}
	isCompressed := true
	reader, err := compressor.NewGzippedReader(data)
	if err != nil {
		s.log.WarnMsg("ошибка при сжатии данных: ", err, ". Отправка без сжатия.")
		isCompressed = false
	}
	reader, sign, err := s.signer.NewReader(reader)
	if err != nil {
		return err
	}
	reader, err = s.encryptor.NewReader(reader)
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

	response, err := s.log.DoRequestWithLog(s.client, request)
	if response != nil && response.Body != nil {
		defer func() {
			_ = response.Body.Close()
		}()
	}
	return err
}

// sendAll отправляет все накопленные метрики пакетом, а при ошибке — по одной.
func (s *sender) sendAll() {
	vals := s.mon.GetValues()
	var errs []error
	s.log.InfoMsg("Sending data to server")
	url := fmt.Sprintf("http://%s/updates/", s.conf.Address)
	values := make([]model.Metrics, 0, len(vals))
	for _, v := range vals {
		values = append(values, v.Metrics)
	}
	err := logger.ExecuteWithRetryNoResult(func(args ...interface{}) error {
		return s.sendData(url, values)
	})
	errs = append(errs, err)
	if err == nil {
		for key := range vals {
			s.mon.OnSuccessSent(key)
		}
	} else {
		for key, val := range vals {
			url := fmt.Sprintf("http://%s/update/", s.conf.Address)
			err := logger.ExecuteWithRetryNoResult(func(args ...interface{}) error {
				return s.sendData(url, val.Metrics)
			})
			errs = append(errs, err)
			if err == nil {
				s.mon.OnSuccessSent(key)
			}
		}
	}
	if errors.Join(errs...) != nil {
		s.log.WarnMsg("Some metrics not sent")
	} else {
		s.log.InfoMsg("All sent")
	}
}

// runSync собирает и отправляет метрики до отмены ctx. При отмене отправляет
// данные, собранные после последней отправки, и завершается.
func (s *sender) runSync(ctx context.Context) error {
	var counter uint // счетчик не может быть меньше нуля
	pending := false // есть собранные, но не отправленные данные
	for {
		s.mon.UpdMetrics()
		pending = true
		// отправляем данные только по достижении счетчиком заданного значения
		if counter*s.conf.PollInterval >= s.conf.ReportInterval {
			s.sendAll()
			pending = false
			counter = 0
		}
		select {
		case <-ctx.Done():
			if pending {
				s.log.InfoMsg("Sending pending data before shutdown")
				s.sendAll()
			}
			return nil
		case <-time.After(time.Duration(s.conf.PollInterval) * time.Second):
		}
		counter += 1
	}
}

// runAsync запускает сбор метрик и conf.RateLimit параллельных отправителей.
// При отмене ctx сбор останавливается, а отправители досылают уже собранные
// данные и завершаются.
func (s *sender) runAsync(ctx context.Context) {
	dataCh := s.mon.MetricsReader(ctx.Done(), s.conf.PollInterval)

	url := fmt.Sprintf("http://%s/update/", s.conf.Address)

	var w uint
	var wg sync.WaitGroup
	for w = 0; w < s.conf.RateLimit; w++ {
		wg.Add(1)
		go s.asyncSender(url, dataCh, &wg)
	}

	wg.Wait()
}

func (s *sender) asyncSender(url string, data <-chan agent.ChannaledMetric, wg *sync.WaitGroup) {
	defer wg.Done()
	for val := range data {
		data, err := json.Marshal(val)
		if err != nil {
			continue
		}
		request, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(data))
		if err != nil {
			s.log.WarnMsg(err)
		}
		request.Header.Set("Content-Type", "application/json")

		err = logger.ExecuteWithRetryNoResult(func(args ...interface{}) error {
			return s.sendData(url, val.Metrics)
		})
		if err == nil {
			s.mon.OnSuccessSent(val.Key)
		}
	}
}

// run запускает сбор и отправку метрик до отмены ctx.
func (s *sender) run(ctx context.Context) error {
	// При нулевом лимите запускаем синхронную отправку данных на сервер
	if s.conf.RateLimit == 0 {
		return s.runSync(ctx)
	}
	s.runAsync(ctx)
	return nil
}

// newLogger создаёт логгер по настройкам агента. Используется InitLogger, а не
// New: пакетные обёртки logger.Info/Warn/Fatal, которые вызываются из
// internal/agent, internal/compressor и др., опираются на глобальный логгер.
func newLogger(conf appConfig) (*logger.Logger, error) {
	return logger.InitLogger(conf.LogLevel)
}

func newSigner(conf appConfig) *crypto.Signer {
	return crypto.NewSigner(conf.Key)
}

func newEncryptor(conf appConfig) (*crypto.Encryptor, error) {
	return crypto.NewEncryptor(conf.CryptoKey)
}

func newHTTPClient() *http.Client {
	return &http.Client{
		Timeout: time.Second * 1, // интервал ожидания: 1 секунда
	}
}

func newSender(conf appConfig, log *logger.Logger, mon *agent.DataCollector, client *http.Client,
	signer *crypto.Signer, encryptor *crypto.Encryptor) *sender {
	return &sender{
		mon:       mon,
		client:    client,
		log:       log,
		signer:    signer,
		encryptor: encryptor,
		conf:      conf,
	}
}

// registerLifecycle подключает запуск и остановку цикла отправки метрик к
// жизненному циклу приложения. Цикл стартует в OnStart, а OnStop отменяет его
// контекст и ждёт досылки накопленных данных не дольше shutdownTimeout.
// Если цикл завершился сам, приложение останавливается через fx.Shutdowner.
func registerLifecycle(lc fx.Lifecycle, sd fx.Shutdowner, s *sender, log *logger.Logger) {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)

	lc.Append(fx.Hook{
		OnStart: func(context.Context) error {
			log.InfoMsg("Connect to server ", s.conf.Address,
				"pollInterval: ", s.conf.PollInterval,
				"reportInterval: ", s.conf.ReportInterval)
			go func() {
				err := s.run(ctx)
				done <- err
				if ctx.Err() == nil {
					_ = sd.Shutdown()
				}
			}()
			return nil
		},
		OnStop: func(context.Context) error {
			cancel()
			select {
			case err := <-done:
				log.InfoMsg("Agent stopped")
				return err
			case <-time.After(shutdownTimeout):
				return fmt.Errorf("agent shutdown timed out after %s, some data may be lost", shutdownTimeout)
			}
		},
	})
}

// newApp собирает приложение агента с зависимостями из контейнера fx.
func newApp(args []string) *fx.App {
	return fx.New(
		fx.NopLogger,
		fx.Provide(
			func() (appConfig, error) { return getConfig(args) },
			newLogger,
			newSigner,
			newEncryptor,
			agent.NewDataCollector,
			newHTTPClient,
			newSender,
		),
		fx.Invoke(registerLifecycle),
	)
}

func main() {
	logger.PrintBuildInfo(buildVersion, buildDate, buildCommit)

	if err := run(os.Args[1:]); err != nil {
		if errors.Is(err, pflag.ErrHelp) {
			return
		}
		logger.Fatal(err)
	}
}

// run запускает агента и штатно завершает работу по сигналам SIGTERM, SIGINT и
// SIGQUIT, досылая данные, находящиеся в обработке.
func run(args []string) error {
	sigCtx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT, syscall.SIGQUIT)
	defer stop()

	app := newApp(args)

	startCtx, cancelStart := context.WithTimeout(context.Background(), app.StartTimeout())
	err := app.Start(startCtx)
	cancelStart()
	if err != nil {
		return err
	}

	select {
	case <-app.Done():
	case <-sigCtx.Done():
		// Восстанавливаем стандартную обработку сигналов: повторный сигнал
		// завершит процесс немедленно.
		stop()
		logger.Info("Shutdown signal received, sending pending data")
	}

	stopCtx, cancelStop := context.WithTimeout(context.Background(), shutdownTimeout+time.Second)
	defer cancelStop()
	return app.Stop(stopCtx)
}
