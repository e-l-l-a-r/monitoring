package main

import (
	"flag"
	"net/http"
	"os"

	"github.com/caarlos0/env/v6"
	"github.com/e-l-l-a-r/monitoring/internal/handler"
	"github.com/e-l-l-a-r/monitoring/internal/logger"
	"github.com/spf13/pflag"
)

type Config struct {
	Address  string `env:"ADDRESS"`
	LogLevel string `env:"LOG_LEVEL"`
}

func parseFlags() {
	pflag.CommandLine.AddGoFlagSet(flag.CommandLine)

	pflag.Usage = func() {
		logger.Info(pflag.CommandLine.Output(), "Metrics collecting server\nUsage of %s:\n", os.Args[0])
		pflag.PrintDefaults()
	}

	pflag.Parse()
}

func get_config() (result Config) {
	var flagRunAddr *string = pflag.StringP("address", "a", "localhost:8080",
		"address and port to run server")
	var flagLogLevel *string = pflag.StringP("log-level", "l", "Info",
		"log level, may be Debug, Info (default), Warning, Error")

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

	return
}

func main() {
	if err := run(); err != nil {
		logger.Fatal(err)
	}
}

func run() error {
	conf := get_config()
	log, err := logger.InitLogger(conf.LogLevel)

	if err != nil {
		return err
	}

	log.InfoMsg("Running server on ", conf.Address)
	router := handler.GetRouter()
	err = http.ListenAndServe(conf.Address, router)
	if err != nil {
		return err
	}
	return nil
}
