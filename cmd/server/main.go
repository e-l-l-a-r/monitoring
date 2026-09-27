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
//   - --config, -c / CONFIG - путь к файлу конфигурации в формате JSON.
//     Значения из файла имеют меньший приоритет, чем флаги и переменные
//     окружения.
package main

import (
	"context"
	"errors"
	"flag"
	"net/http"
	_ "net/http/pprof" // подключаем пакет pprof
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/caarlos0/env/v6"
	"github.com/spf13/pflag"

	"github.com/e-l-l-a-r/monitoring/internal/auditor"
	"github.com/e-l-l-a-r/monitoring/internal/compressor"
	"github.com/e-l-l-a-r/monitoring/internal/config"
	"github.com/e-l-l-a-r/monitoring/internal/crypto"
	"github.com/e-l-l-a-r/monitoring/internal/handler"
	"github.com/e-l-l-a-r/monitoring/internal/logger"
	"github.com/e-l-l-a-r/monitoring/internal/repository"
)

type Config struct {
	Address         string `env:"ADDRESS"`
	LogLevel        string `env:"LOG_LEVEL"`
	FileStoragePath string `env:"FILE_STORAGE_PATH"`
	DBConnString    string `env:"DATABASE_DSN"`
	Key             string `env:"KEY"`
	CryptoKey       string `env:"CRYPTO_KEY"`
	AuditFile       string `env:"AUDIT_FILE"`
	AuditURL        string `env:"AUDIT_URL"`
	Config          string `env:"CONFIG"`
	StoreInterval   uint   `env:"STORE_INTERVAL"`
	Restore         bool   `env:"RESTORE"`
}

var buildVersion string
var buildDate string
var buildCommit string

func parseFlags() {
	pflag.CommandLine.AddGoFlagSet(flag.CommandLine)

	pflag.Usage = func() {
		logger.Info(pflag.CommandLine.Output(), "Metrics collecting server\nUsage of %s:\n", os.Args[0])
		pflag.PrintDefaults()
	}

	pflag.Parse()
}

func getConfig() (Config, error) {
	var result Config

	var flagRunAddr = pflag.StringP("address", "a", "localhost:8080",
		"адрес и порт для запуска сервера")
	var flagLogLevel = pflag.StringP("log-level", "l", "Info",
		"уровень логирования, может быть Debug, Info (по умолчанию), Warning, Error")
	var flagStoreInterval = pflag.UintP("store-interval", "i", 300,
		"количество секунд для сохранения метрик в файл, нулевое значение для синхронной записи")
	var flagFileStoragePath = pflag.StringP("file-storage-path", "f", "metrics.json",
		"файл для хранения метрик")
	var flagRestore = pflag.BoolP("restore", "r", false,
		"восстанавливать метрики из файла")
	var flagDBConnString = pflag.StringP("db-conn-string", "d", "",
		"строка подключения к базе данных")
	var flagKey = pflag.StringP("key", "k", "",
		"Ключ для подписи запросов")
	var flagAuditFile = pflag.String("audit-file", "",
		"файл для хранения лога запросов")
	var flagAuditURL = pflag.StringP("audit-url", "u", "",
		"URL для отправки лога запросов")
	var flagCryptoKey = pflag.String("crypto-key", "",
		"path to file with private RSA key")
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

	var fileConf config.Server
	if err := config.Load(result.Config, &fileConf); err != nil {
		return result, err
	}

	flags := pflag.CommandLine

	result.Address = config.ResolveString(result.Address,
		flags.Changed("address"), *flagRunAddr, fileConf.Address)
	result.LogLevel = config.ResolveString(result.LogLevel,
		flags.Changed("log-level"), *flagLogLevel, fileConf.LogLevel)
	result.StoreInterval = config.ResolveSeconds(result.StoreInterval,
		flags.Changed("store-interval"), *flagStoreInterval, fileConf.StoreInterval)
	result.FileStoragePath = config.ResolveString(result.FileStoragePath,
		flags.Changed("file-storage-path"), *flagFileStoragePath, fileConf.StoreFile)
	result.Restore = config.ResolveBool(result.Restore,
		flags.Changed("restore"), *flagRestore, fileConf.Restore)
	result.DBConnString = config.ResolveString(result.DBConnString,
		flags.Changed("db-conn-string"), *flagDBConnString, fileConf.DatabaseDSN)
	result.AuditFile = config.ResolveString(result.AuditFile,
		flags.Changed("audit-file"), *flagAuditFile, fileConf.AuditFile)
	result.AuditURL = config.ResolveString(result.AuditURL,
		flags.Changed("audit-url"), *flagAuditURL, fileConf.AuditURL)
	result.Key = config.ResolveString(result.Key,
		flags.Changed("key"), *flagKey, fileConf.Key)
	result.CryptoKey = config.ResolveString(result.CryptoKey,
		flags.Changed("crypto-key"), *flagCryptoKey, fileConf.CryptoKey)

	return result, nil
}

func main() {
	logger.PrintBuildInfo(buildVersion, buildDate, buildCommit)
	if err := run(); err != nil {
		logger.Fatal(err)
	}
}

func run() error {
	conf, err := getConfig()
	if err != nil {
		return err
	}

	log, err := logger.InitLogger(conf.LogLevel)
	ctx := context.Background()

	if err != nil {
		return err
	}

	log.InfoMsg("Running server on ", conf.Address)
	log.InfoMsg("Sync data to ", conf.FileStoragePath, " every ", conf.StoreInterval, " seconds.")
	log.InfoMsg("Restore on startup: ", conf.Restore)
	log.InfoMsg("==================================")
	var storage handler.Storage
	if conf.DBConnString != "" {
		log.InfoMsg("Use DataBase storage")
		res, e := logger.ExecuteWithRetry(func(args ...interface{}) (interface{}, error) {
			return repository.NewSQLStorage(conf.DBConnString)
		})
		if e != nil {
			return e
		}
		storage = res.(*repository.SQLStorage)
		defer storage.(*repository.SQLStorage).Close()
		if e := storage.(*repository.SQLStorage).DoMigrate(); e != nil {
			return e
		}
		if e := storage.(*repository.SQLStorage).Restore(ctx); e != nil {
			return e
		}
	} else {
		log.InfoMsg("Use Memory storage")
		storage = repository.NewMemStorage(conf.StoreInterval, conf.FileStoragePath)
		if conf.Restore {
			e := storage.(*repository.MemStorage).RestoreFromFile(ctx)
			if e != nil {
				log.WarnMsg("Error restoring metrics from file", e)
			}
		}

		syncTicker := time.NewTicker(time.Duration(conf.StoreInterval) * time.Second)
		defer syncTicker.Stop()

		go func() {
			for range syncTicker.C {
				if e := storage.SyncIfNeed(ctx); e != nil {
					log.WarnMsg("Error during periodic sync:", e)
				}
			}
		}()
	}

	if _, err := crypto.InitSigner(conf.Key); err != nil {
		logger.Fatal(err)
	}

	if _, err := crypto.InitDecryptor(conf.CryptoKey); err != nil {
		return err
	}

	audit := auditor.NewAuditor()

	if conf.AuditFile != "" {
		fileAudit, err := auditor.NewFileAuditor(conf.AuditFile)
		if err != nil {
			return err
		}
		defer func() {
			if err := fileAudit.Close(); err != nil {
				log.WarnMsg("Error closing audit file:", err)
			}
		}()
		audit.Register(fileAudit)
	}
	if conf.AuditURL != "" {
		audit.Register(auditor.NewURLAuditor(conf.AuditURL))
	}
	defer audit.Close()

	router := handler.GetRouter(storage, audit)
	server := &http.Server{
		Addr:    conf.Address,
		Handler: crypto.DecryptHandle(crypto.SignHandle(compressor.GzipHandle(router))),
	}

	serverErr := make(chan error, 1)
	go func() {
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
			return
		}
		serverErr <- nil
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(stop)

	select {
	case err := <-serverErr:
		return err
	case <-stop:
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return server.Shutdown(shutdownCtx)
	}
}
