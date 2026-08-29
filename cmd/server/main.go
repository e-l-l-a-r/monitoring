package main

import (
	"context"
	"flag"
	"net/http"
	"os"
	"time"

	"github.com/caarlos0/env/v6"
	"github.com/e-l-l-a-r/monitoring/internal/auditor"
	"github.com/e-l-l-a-r/monitoring/internal/compressor"
	"github.com/e-l-l-a-r/monitoring/internal/crypto"
	"github.com/e-l-l-a-r/monitoring/internal/handler"
	"github.com/e-l-l-a-r/monitoring/internal/logger"
	"github.com/e-l-l-a-r/monitoring/internal/repository"
	"github.com/spf13/pflag"
)

type Config struct {
	Address         string `env:"ADDRESS"`
	LogLevel        string `env:"LOG_LEVEL"`
	StoreInterval   uint   `env:"STORE_INTERVAL"`
	FileStoragePath string `env:"FILE_STORAGE_PATH"`
	Restore         bool   `env:"RESTORE"`
	DbConnSrting    string `env:"DATABASE_DSN"`
	Key             string `env:"KEY"`
	AuditFile       string `env:"AUDIT_FILE"`
	AuditUrl        string `env:"AUDIT_URL"`
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
	var flagDbConnSrting = pflag.StringP("db-conn-string", "d", "",
		"database connection string")
	var flagKey = pflag.StringP("key", "k", "",
		"Key for signing the requests")
	var flagAuditFile = pflag.StringP("audit-file", "c", "",
		"file to store requests log")
	var flagAuditUrl = pflag.StringP("audit-url", "u", "",
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
	if result.Restore == false {
		result.Restore = *flagRestore
	}
	if result.DbConnSrting == "" {
		result.DbConnSrting = *flagDbConnSrting
	}
	if result.AuditFile == "" {
		result.AuditFile = *flagAuditFile
	}
	if result.AuditUrl == "" {
		result.AuditUrl = *flagAuditUrl
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
	if conf.DbConnSrting != "" {
		log.InfoMsg("Use DataBase storage")
		result, err := logger.ExecuteWithRetry(func(args ...interface{}) (interface{}, error) {
			return repository.NewSqlStorage(conf.DbConnSrting)
		})
		if err != nil {
			return err
		}
		storage = result.(*repository.SqlStorage)
		defer storage.(*repository.SqlStorage).Close()
		storage.(*repository.SqlStorage).DoMigrate()
		storage.(*repository.SqlStorage).Restore(ctx)
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
			for {
				select {
				case <-syncTicker.C:
					if err := storage.SyncIfNeed(ctx); err != nil {
						log.WarnMsg("Error during periodic sync:", err)
					}
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
	if conf.AuditUrl != "" {
		audit.Register(auditor.NewUrlAuditor(conf.AuditUrl))
	}

	router := handler.GetRouter(storage, audit)

	err = http.ListenAndServe(conf.Address, crypto.SignHandle(compressor.GzipHandle(router)))
	if err != nil {
		return err
	}
	return nil
}
