// Server запускает HTTP-сервер мониторинга, который принимает метрики от
// агентов, хранит их в памяти, файле или базе данных и обслуживает API
// получения текущих значений.
//
// Параметры запуска можно задать флагами командной строки или переменными
// окружения. Если переменная окружения задана, она имеет приоритет над
// соответствующим флагом:
//   - --address, -a / ADDRESS - адрес и порт HTTP-сервера; по умолчанию
//     localhost:8080.
//   - --log-level, -l / LOG_LEVEL - уровень логирования: Debug, Info, Warning
//     или Error; по умолчанию Info.
//   - --store-interval, -i / STORE_INTERVAL - интервал сохранения метрик в файл
//     в секундах; значение 0 включает синхронную запись; по умолчанию 300.
//   - --file-storage-path, -f / FILE_STORAGE_PATH - путь к файлу хранения
//     метрик; по умолчанию metrics.json.
//   - --restore, -r / RESTORE - восстанавливать метрики из файла при старте.
//   - --db-conn-string, -d / DATABASE_DSN - строка подключения к базе данных;
//     если задана, используется SQL-хранилище.
//   - --key, -k / KEY - ключ для проверки подписи запросов.
//   - --audit-file / AUDIT_FILE - файл для записи аудита запросов.
//   - --audit-url, -u / AUDIT_URL - URL для отправки аудита запросов.
//   - --crypto-key / CRYPTO_KEY - путь до файла с приватным ключом для
//     расшифровки входящих запросов.
//   - --trusted-subnet, -t / TRUSTED_SUBNET - доверенная подсеть в нотации
//     CIDR. Если задана, запросы на обновление метрик принимаются только с
//     IP-адресов этой подсети (заголовок X-Real-IP), остальные получают
//     403 Forbidden. Пустое значение снимает ограничение.
//   - --config, -c / CONFIG - путь к файлу конфигурации в формате JSON.
//     Значения из файла имеют меньший приоритет, чем флаги и переменные
//     окружения.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net"
	"net/http"
	_ "net/http/pprof" // подключаем пакет pprof
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/caarlos0/env/v6"
	"github.com/spf13/pflag"
	"go.uber.org/fx"

	"github.com/e-l-l-a-r/monitoring/internal/auditor"
	"github.com/e-l-l-a-r/monitoring/internal/compressor"
	"github.com/e-l-l-a-r/monitoring/internal/config"
	"github.com/e-l-l-a-r/monitoring/internal/crypto"
	"github.com/e-l-l-a-r/monitoring/internal/handler"
	"github.com/e-l-l-a-r/monitoring/internal/logger"
	"github.com/e-l-l-a-r/monitoring/internal/repository"
)

const (
	// serverShutdownTimeout - время на корректное завершение HTTP-сервера.
	serverShutdownTimeout = 5 * time.Second
	// appStopTimeout - общее время на остановку всех компонентов приложения.
	appStopTimeout = 30 * time.Second
)

// envConfig - настройки, прочитанные из переменных окружения. Указатели
// позволяют отличить незаданную переменную (nil) от заданной нулевым значением
// (0 или false).
type envConfig struct {
	StoreInterval   *uint  `env:"STORE_INTERVAL"`
	Restore         *bool  `env:"RESTORE"`
	Address         string `env:"ADDRESS"`
	LogLevel        string `env:"LOG_LEVEL"`
	FileStoragePath string `env:"FILE_STORAGE_PATH"`
	DBConnString    string `env:"DATABASE_DSN"`
	Key             string `env:"KEY"`
	CryptoKey       string `env:"CRYPTO_KEY"`
	AuditFile       string `env:"AUDIT_FILE"`
	AuditURL        string `env:"AUDIT_URL"`
	TrustedSubnet   string `env:"TRUSTED_SUBNET"`
	Config          string `env:"CONFIG"`
}

// Config - итоговые настройки сервера после разрешения приоритета окружения,
// флагов и файла конфигурации.
type Config struct {
	Address         string
	LogLevel        string
	FileStoragePath string
	DBConnString    string
	Key             string
	CryptoKey       string
	AuditFile       string
	AuditURL        string
	TrustedSubnet   string
	Config          string
	StoreInterval   uint
	Restore         bool
}

var buildVersion string
var buildDate string
var buildCommit string

// parseFlags разбирает аргументы командной строки в локальном наборе флагов.
func parseFlags(fs *pflag.FlagSet, args []string) error {
	fs.AddGoFlagSet(flag.CommandLine)

	fs.Usage = func() {
		_, _ = fmt.Fprintf(fs.Output(), "Metrics collecting server\nUsage of %s:\n", fs.Name())
		fs.PrintDefaults()
	}

	return fs.Parse(args)
}

// getConfig собирает настройки из окружения, аргументов args и файла
// конфигурации. Приоритет: окружение, флаг, файл, значение флага по умолчанию.
func getConfig(args []string) (Config, error) {
	var envConf envConfig

	fs := pflag.NewFlagSet("server", pflag.ContinueOnError)

	flagRunAddr := fs.StringP("address", "a", "localhost:8080",
		"адрес и порт для запуска сервера")
	flagLogLevel := fs.StringP("log-level", "l", "Info",
		"уровень логирования, может быть Debug, Info (по умолчанию), Warning, Error")
	flagStoreInterval := fs.UintP("store-interval", "i", 300,
		"количество секунд для сохранения метрик в файл, нулевое значение для синхронной записи")
	flagFileStoragePath := fs.StringP("file-storage-path", "f", "metrics.json",
		"файл для хранения метрик")
	flagRestore := fs.BoolP("restore", "r", false,
		"восстанавливать метрики из файла")
	flagDBConnString := fs.StringP("db-conn-string", "d", "",
		"строка подключения к базе данных")
	flagKey := fs.StringP("key", "k", "",
		"Ключ для подписи запросов")
	flagAuditFile := fs.String("audit-file", "",
		"файл для хранения лога запросов")
	flagAuditURL := fs.StringP("audit-url", "u", "",
		"URL для отправки лога запросов")
	flagCryptoKey := fs.String("crypto-key", "",
		"path to file with private RSA key")
	flagTrustedSubnet := fs.StringP("trusted-subnet", "t", "",
		"доверенная подсеть агентов в нотации CIDR")
	flagConfig := fs.StringP("config", "c", "",
		"путь к файлу конфигурации в формате JSON")

	if err := env.Parse(&envConf); err != nil {
		logger.Warn(err)
	}

	if err := parseFlags(fs, args); err != nil {
		return Config{}, err
	}

	configPath := envConf.Config
	if configPath == "" {
		configPath = *flagConfig
	}

	var fileConf config.Server
	if err := config.Load(configPath, &fileConf); err != nil {
		return Config{}, err
	}

	return Config{
		Config: configPath,
		Address: config.ResolveString(envConf.Address,
			fs.Changed("address"), *flagRunAddr, fileConf.Address),
		LogLevel: config.ResolveString(envConf.LogLevel,
			fs.Changed("log-level"), *flagLogLevel, fileConf.LogLevel),
		StoreInterval: config.ResolveSeconds(envConf.StoreInterval,
			fs.Changed("store-interval"), *flagStoreInterval, fileConf.StoreInterval),
		FileStoragePath: config.ResolveString(envConf.FileStoragePath,
			fs.Changed("file-storage-path"), *flagFileStoragePath, fileConf.StoreFile),
		Restore: config.ResolveBool(envConf.Restore,
			fs.Changed("restore"), *flagRestore, fileConf.Restore),
		DBConnString: config.ResolveString(envConf.DBConnString,
			fs.Changed("db-conn-string"), *flagDBConnString, fileConf.DatabaseDSN),
		AuditFile: config.ResolveString(envConf.AuditFile,
			fs.Changed("audit-file"), *flagAuditFile, fileConf.AuditFile),
		AuditURL: config.ResolveString(envConf.AuditURL,
			fs.Changed("audit-url"), *flagAuditURL, fileConf.AuditURL),
		Key: config.ResolveString(envConf.Key,
			fs.Changed("key"), *flagKey, fileConf.Key),
		CryptoKey: config.ResolveString(envConf.CryptoKey,
			fs.Changed("crypto-key"), *flagCryptoKey, fileConf.CryptoKey),
		TrustedSubnet: config.ResolveString(envConf.TrustedSubnet,
			fs.Changed("trusted-subnet"), *flagTrustedSubnet, fileConf.TrustedSubnet),
	}, nil
}

// serverErrors - канал, по которому HTTP-сервер сообщает о завершении
// ListenAndServe с ошибкой, отличной от http.ErrServerClosed.
type serverErrors chan error

// provideLogger создаёт логгер по итоговой конфигурации.
//
// Вызывается именно InitLogger, а не New: пакетные функции logger.Info,
// logger.Warn и logger.Fatal, которыми пользуются другие пакеты, работают через
// внутренний singleLogger, и его нужно заполнить.
func provideLogger(conf Config) (*logger.Logger, error) {
	return logger.InitLogger(conf.LogLevel)
}

// provideSigner создаёт подписчик запросов; пустой ключ отключает подпись.
func provideSigner(conf Config) *crypto.Signer {
	return crypto.NewSigner(conf.Key)
}

// provideDecryptor создаёт расшифровщик запросов; пустой путь отключает его.
func provideDecryptor(conf Config) (*crypto.Decryptor, error) {
	return crypto.NewDecryptor(conf.CryptoKey)
}

// provideStorage создаёт хранилище метрик (SQL или в памяти) и регистрирует
// его запуск и остановку в lifecycle. Хуки хранилища регистрируются раньше
// хуков сервера, поэтому при остановке данные сохраняются уже после остановки
// сервера.
func provideStorage(lc fx.Lifecycle, conf Config, log *logger.Logger) (handler.Storage, error) {
	if conf.DBConnString != "" {
		return provideSQLStorage(lc, conf, log)
	}
	return provideMemStorage(lc, conf, log), nil
}

func provideSQLStorage(lc fx.Lifecycle, conf Config, log *logger.Logger) (handler.Storage, error) {
	log.InfoMsg("Use DataBase storage")

	res, err := logger.ExecuteWithRetry(func(args ...interface{}) (interface{}, error) {
		return repository.NewSQLStorage(conf.DBConnString)
	})
	if err != nil {
		return nil, err
	}
	storage, ok := res.(*repository.SQLStorage)
	if !ok {
		return nil, fmt.Errorf("unexpected storage type %T", res)
	}

	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			if err := storage.DoMigrate(); err != nil {
				return err
			}
			return storage.Restore(ctx)
		},
		OnStop: func(context.Context) error {
			storage.Close()
			return nil
		},
	})

	return storage, nil
}

func provideMemStorage(lc fx.Lifecycle, conf Config, log *logger.Logger) handler.Storage {
	log.InfoMsg("Use Memory storage")

	storage := repository.NewMemStorage(conf.StoreInterval, conf.FileStoragePath)
	syncDone := make(chan struct{})
	var syncWG sync.WaitGroup

	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			if conf.Restore {
				if err := storage.RestoreFromFile(ctx); err != nil {
					log.WarnMsg("Error restoring metrics from file", err)
				}
			}

			// При нулевом интервале запись синхронная (в хендлерах), тикер не нужен.
			if conf.StoreInterval == 0 {
				return nil
			}

			syncTicker := time.NewTicker(time.Duration(conf.StoreInterval) * time.Second)
			syncWG.Add(1)
			go func() {
				defer syncWG.Done()
				defer syncTicker.Stop()
				for {
					select {
					case <-syncDone:
						return
					case <-syncTicker.C:
						if err := storage.SyncIfNeed(context.Background()); err != nil {
							log.WarnMsg("Error during periodic sync:", err)
						}
					}
				}
			}()
			return nil
		},
		OnStop: func(ctx context.Context) error {
			close(syncDone)
			syncWG.Wait()
			log.InfoMsg("Saving metrics to ", conf.FileStoragePath)
			return storage.Flush(ctx)
		},
	})

	return storage
}

// provideAuditor создаёт аудитор с наблюдателями из конфигурации. При
// остановке сначала закрывается аудитор (дожидается обработки очередей), затем
// файл аудита.
func provideAuditor(lc fx.Lifecycle, conf Config, log *logger.Logger) (auditor.Publisher, error) {
	audit := auditor.NewAuditor()

	if conf.AuditFile != "" {
		fileAudit, err := auditor.NewFileAuditor(conf.AuditFile)
		if err != nil {
			return nil, err
		}
		lc.Append(fx.Hook{OnStop: func(context.Context) error {
			if err := fileAudit.Close(); err != nil {
				log.WarnMsg("Error closing audit file:", err)
			}
			return nil
		}})
		audit.Register(fileAudit)
	}
	if conf.AuditURL != "" {
		audit.Register(auditor.NewURLAuditor(conf.AuditURL))
	}

	// Хуки OnStop выполняются в обратном порядке: Close аудитора - до закрытия файла.
	lc.Append(fx.Hook{OnStop: func(context.Context) error {
		audit.Close()
		return nil
	}})

	return audit, nil
}

// provideTrustedSubnet создаёт фильтр доверенной подсети; пустая подсеть
// отключает проверку, некорректная прерывает запуск.
func provideTrustedSubnet(conf Config) (*handler.TrustedSubnet, error) {
	return handler.NewTrustedSubnet(conf.TrustedSubnet)
}

// provideRouter строит роутер API поверх хранилища и аудитора.
func provideRouter(storage handler.Storage, audit auditor.Publisher, trusted *handler.TrustedSubnet) http.Handler {
	return handler.GetRouter(storage, audit, trusted)
}

// provideServer создаёт HTTP-сервер с цепочкой middleware. Порт занимается
// синхронно в OnStart, поэтому ошибка привязки адреса прерывает запуск. Ошибка
// Serve после старта передаётся в errs.
func provideServer(lc fx.Lifecycle, conf Config, log *logger.Logger, router http.Handler,
	signer *crypto.Signer, decryptor *crypto.Decryptor, errs serverErrors) *http.Server {
	server := &http.Server{
		Addr:    conf.Address,
		Handler: decryptor.Handle(signer.Handle(compressor.GzipHandle(router))),
	}

	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			var lcfg net.ListenConfig
			ln, err := lcfg.Listen(ctx, "tcp", server.Addr)
			if err != nil {
				return err
			}

			log.InfoMsg("Running server on ", conf.Address)
			log.InfoMsg("Sync data to ", conf.FileStoragePath, " every ", conf.StoreInterval, " seconds.")
			log.InfoMsg("Restore on startup: ", conf.Restore)
			log.InfoMsg("==================================")

			go func() {
				if err := server.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
					errs <- err
				}
			}()
			return nil
		},
		OnStop: func(ctx context.Context) error {
			log.InfoMsg("Shutdown signal received, stopping server")
			shutdownCtx, cancel := context.WithTimeout(ctx, serverShutdownTimeout)
			defer cancel()
			return server.Shutdown(shutdownCtx)
		},
	})

	return server
}

// newApp собирает граф зависимостей приложения.
func newApp(args []string, errs *serverErrors) *fx.App {
	return fx.New(
		fx.NopLogger,
		fx.StopTimeout(appStopTimeout),
		fx.Provide(
			func() (Config, error) { return getConfig(args) },
			func() serverErrors { return make(serverErrors, 1) },
			provideLogger,
			provideSigner,
			provideDecryptor,
			provideStorage,
			provideAuditor,
			provideTrustedSubnet,
			provideRouter,
			provideServer,
		),
		fx.Populate(errs),
		fx.Invoke(func(*http.Server) {}),
	)
}

func main() {
	logger.PrintBuildInfo(buildVersion, buildDate, buildCommit)
	if err := run(os.Args[1:]); err != nil {
		logger.Fatal(err)
	}
}

// run запускает приложение и ждёт сигнала завершения или ошибки сервера.
// Остановка выполняется в обратном порядке запуска: сначала HTTP-сервер, затем
// сохранение данных и закрытие аудитора.
func run(args []string) error {
	// Подписываемся на сигналы завершения до запуска компонентов,
	// чтобы не потерять сигнал, пришедший во время старта.
	sigCtx, stopSignals := signal.NotifyContext(context.Background(),
		syscall.SIGTERM, syscall.SIGINT, syscall.SIGQUIT)
	defer stopSignals()

	var errs serverErrors
	app := newApp(args, &errs)
	if err := app.Err(); err != nil {
		if errors.Is(err, pflag.ErrHelp) {
			return nil
		}
		return err
	}

	if err := app.Start(sigCtx); err != nil {
		return err
	}

	var runErr error
	select {
	case runErr = <-errs:
	case <-sigCtx.Done():
	}

	// Сохраняем данные даже если сервер завершился с ошибкой.
	stopCtx, cancel := context.WithTimeout(context.Background(), appStopTimeout)
	defer cancel()
	if err := app.Stop(stopCtx); err != nil {
		runErr = errors.Join(runErr, err)
	}
	logger.Info("Server stopped")
	return runErr
}
