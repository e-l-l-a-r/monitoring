package main

import (
	"flag"
	"net/http"
	"os"
	"time"

	"github.com/caarlos0/env/v6"
	"github.com/e-l-l-a-r/monitoring/internal/compressor"
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

	if err != nil {
		return err
	}

	log.InfoMsg("Running server on ", conf.Address)
	log.InfoMsg("Sync data to ", conf.FileStoragePath, " every ", conf.StoreInterval, " seconds.")
	log.InfoMsg("Restore on startup: ", conf.Restore)
	log.InfoMsg("==================================")
	storage := repository.NewMemStorage(conf.StoreInterval, conf.FileStoragePath)
	if conf.Restore {
		err := storage.RestoreFromFile()
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
				if err := storage.SyncIfNeed(); err != nil {
					log.WarnMsg("Error during periodic sync:", err)
				}
			}
		}
	}()

	router := handler.GetRouter(storage)
	err = http.ListenAndServe(conf.Address, compressor.GzipHandle(router))
	if err != nil {
		return err
	}
	return nil
}
