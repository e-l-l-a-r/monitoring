package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/caarlos0/env/v6"
	"github.com/e-l-l-a-r/monitoring/internal/agent"
	"github.com/e-l-l-a-r/monitoring/internal/compressor"
	"github.com/e-l-l-a-r/monitoring/internal/logger"
	"github.com/e-l-l-a-r/monitoring/internal/model"
	"github.com/spf13/pflag"
)

type config struct {
	Address        string `env:"ADDRESS"`
	PollInterval   uint   `env:"POLL_INTERVAL"`
	ReportInterval uint   `env:"REPORT_INTERVAL"`
	LogLevel       string `env:"LOG_LEVEL"`
}

func parseFlags() {
	pflag.CommandLine.AddGoFlagSet(flag.CommandLine)

	pflag.Usage = func() {
		logger.Info(pflag.CommandLine.Output(), "Metrics collecting agent\nUsage of %s:\n", os.Args[0])
		pflag.PrintDefaults()
	}

	pflag.Parse()
}

func getConfig() (result config) {
	var flagRunAddr = pflag.StringP("address", "a", "localhost:8080",
		"address and port of server to connect")
	var pollInterval = pflag.UintP("poll-interval", "p", 2,
		"number of seconds to update metrics")
	var reportInterval = pflag.UintP("report-interval", "r", 10,
		"number of seconds to send metrics to server")
	var flagLogLevel = pflag.StringP("log-level", "l", "Info",
		"log level, may be Debug, Info (default), Warning, Error")

	err := env.Parse(&result)
	if err != nil {
		logger.Warn(err)
	}

	parseFlags()

	if result.Address == "" {
		result.Address = *flagRunAddr
	}

	if result.PollInterval == 0 {
		result.PollInterval = *pollInterval
	}

	if result.ReportInterval == 0 {
		result.ReportInterval = *reportInterval
	}

	if result.LogLevel == "" {
		result.LogLevel = *flagLogLevel
	}

	return
}

type Logger interface {
	InfoMsg(args ...interface{})
	WarnMsg(args ...interface{})
	DoRequestWithLog(client *http.Client, req *http.Request) (*http.Response, error)
}

func sendData(client *http.Client, log Logger, url string, val interface{}) error {
	data, err := json.Marshal(val)
	if err != nil {
		panic(err)
	}
	isCompressed := true
	reader, err := compressor.NewGzippedReader(data)
	if err != nil {
		log.WarnMsg("error while compressing data: ", err, ". Send uncompressed.")
		isCompressed = false
	}
	request, err := http.NewRequest(http.MethodPost, url, reader)
	if err != nil {
		log.WarnMsg(err)
	}
	request.Header.Set("Content-Type", "application/json")
	if isCompressed {
		request.Header.Set("Content-Encoding", "gzip")
	}

	_, err = log.DoRequestWithLog(client, request)
	return err
}

func main() {
	var counter uint // счетчик не может быть меньше нуля

	conf := getConfig()
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

	for {
		mon.UpdMetrics()
		// отправляем данные только по достижении счетчиком заданного значения
		if counter*conf.PollInterval >= conf.ReportInterval {
			vals := mon.GetValues()
			log.InfoMsg("Sending data to server")
			url := fmt.Sprintf("http://%s/updates/", conf.Address)
			values := make([]model.Metrics, 0, len(vals))
			for _, v := range vals {
				values = append(values, v.Metrics)
			}
			err := sendData(&client, log, url, values)
			if err == nil {
				for key, _ := range vals {
					mon.OnSuccessSent(key)
				}
			} else {
				for key, val := range vals {
					url := fmt.Sprintf("http://%s/update/", conf.Address)
					err := sendData(&client, log, url, val.Metrics)
					if err == nil {
						mon.OnSuccessSent(key)
					}
				}
			}
			log.InfoMsg("All sent")
			counter = 0
		}
		time.Sleep(time.Duration(conf.PollInterval) * time.Second)
		counter += 1
	}
}
