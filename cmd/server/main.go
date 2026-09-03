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
//   - --audit-file, -c / AUDIT_FILE - файл для записи аудита запросов.
//   - --audit-url, -u / AUDIT_URL - URL для отправки аудита запросов.
package main

import (
	"context"
	"flag"
	"net/http"
	_ "net/http/pprof" // подключаем пакет pprof
	"os"
	"time"

	"github.com/caarlos0/env/v6"
	"github.com/spf13/pflag"

	"github.com/e-l-l-a-r/monitoring/internal/auditor"
	"github.com/e-l-l-a-r/monitoring/internal/compressor"
	"github.com/e-l-l-a-r/monitoring/internal/crypto"
	"github.com/e-l-l-a-r/monitoring/internal/handler"
	"github.com/e-l-l-a-r/monitoring/internal/logger"
	"github.com/e-l-l-a-r/monitoring/internal/repository"
)

type Config struct {
	Address         string `env:"ADDRESS"`
	LogLevel        string `env:"LOG_LEVEL"`
	StoreInterval   uint   `env:"STORE_INTERVAL"`
	FileStoragePath string `env:"FILE_STORAGE_PATH"`
	Restore         bool   `env:"RESTORE"`
	DBConnString    string `env:"DATABASE_DSN"`
	Key             string `env:"KEY"`
	AuditFile       string `env:"AUDIT_FILE"`
	AuditURL        string `env:"AUDIT_URL"`
}

func parseFlags() {
	pflag.CommandLine.AddGoFlagSet(flag.CommandLine)

	pflag.Usage = func() {
		logger.Info(pflag.CommandLine.Output(), "Metrics collecting server\nUsage of %s:\n", os.Args[0])
		pflag.PrintDefaults()
	}

	pflag.Parse()
}

func getConfig() (result Config) {
	var flagRunAddr = pflag.StringP("address", "a", "localhost:8080",
		"address and port to run server")
	var flagLogLevel = pflag.StringP("log-level", "l", "Info",
		"log level, may be Debug, Info (default), Warning, Error")
	var flagStoreInterval = pflag.UintP("store-interval", "i", 300,
		"number of seconds to store metrics to file, zero value for sync write")
	var flagFileStoragePath = pflag.StringP("file-storage-path", "f", "metrics.json",
		"file to store metrics")
	var flagRestore = pflag.BoolP("restore", "r", false,
		"restore metrics from file")
	var flagDBConnString = pflag.StringP("db-conn-string", "d", "",
		"database connection string")
	var flagKey = pflag.StringP("key", "k", "",
		"Key for signing the requests")
	var flagAuditFile = pflag.StringP("audit-file", "c", "",
		"file to store requests log")
	var flagAuditURL = pflag.StringP("audit-url", "u", "",
		"url to send requests log")

	err := env.Parse(&result)

	if err != nil {
		logger.Warn(err)
	}

	parseFlags()

	if result.Address == "" {
		result.Address = *flagRunAddr
	}
	if result.LogLevel == "" {
		result.LogLevel = *flagLogLevel
	}
	if result.StoreInterval == 0 {
		result.StoreInterval = *flagStoreInterval
	}
	if result.FileStoragePath == "" {
		result.FileStoragePath = *flagFileStoragePath
	}
	if !result.Restore {
		result.Restore = *flagRestore
	}
	if result.DBConnString == "" {
		result.DBConnString = *flagDBConnString
	}
	if result.AuditFile == "" {
		result.AuditFile = *flagAuditFile
	}
	if result.AuditURL == "" {
		result.AuditURL = *flagAuditURL
	}

	if result.Key == "" {
		result.Key = *flagKey
	}

	return
}

func main() {
	if err := run(); err != nil {
		logger.Fatal(err)
	}
}

func run() error {
	conf := getConfig()
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
		result, err := logger.ExecuteWithRetry(func(args ...interface{}) (interface{}, error) {
			return repository.NewSQLStorage(conf.DBConnString)
		})
		if err != nil {
			return err
		}
		storage = result.(*repository.SQLStorage)
		defer storage.(*repository.SQLStorage).Close()
		if err := storage.(*repository.SQLStorage).DoMigrate(); err != nil {
			return err
		}
		if err := storage.(*repository.SQLStorage).Restore(ctx); err != nil {
			return err
		}
	} else {
		log.InfoMsg("Use Memory storage")
		storage = repository.NewMemStorage(conf.StoreInterval, conf.FileStoragePath)
		if conf.Restore {
			err := storage.(*repository.MemStorage).RestoreFromFile(ctx)
			if err != nil {
				log.WarnMsg("Error restoring metrics from file", err)
			}
		}

		syncTicker := time.NewTicker(time.Duration(conf.StoreInterval) * time.Second)
		defer syncTicker.Stop()

		go func() {
			for range syncTicker.C {
				if err := storage.SyncIfNeed(ctx); err != nil {
					log.WarnMsg("Error during periodic sync:", err)
				}
			}
		}()
	}

	_, err = crypto.InitSigner(conf.Key)
	if err != nil {
		logger.Fatal(err)
	}

	audit := auditor.NewAuditor()

	if conf.AuditFile != "" {
		audit.Register(auditor.NewFileAuditor(conf.AuditFile))
	}
	if conf.AuditURL != "" {
		audit.Register(auditor.NewURLAuditor(conf.AuditURL))
	}

	router := handler.GetRouter(storage, audit)

	err = http.ListenAndServe(conf.Address, crypto.SignHandle(compressor.GzipHandle(router)))
	if err != nil {
		return err
	}
	return nil
}
