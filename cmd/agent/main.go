package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/caarlos0/env/v6"
	"github.com/e-l-l-a-r/monitoring/internal/agent"
	"github.com/spf13/pflag"
)

type config struct {
	Address        string `env:"ADDRESS"`
	PollInterval   uint   `env:"POLL_INTERVAL"`
	ReportInterval uint   `env:"REPORT_INTERVAL"`
}

func parseFlags() {
	pflag.CommandLine.AddGoFlagSet(flag.CommandLine)

	pflag.Usage = func() {
		fmt.Fprintf(pflag.CommandLine.Output(), "Metrics collecting agent\nUsage of %s:\n", os.Args[0])
		pflag.PrintDefaults()
	}

	pflag.Parse()
}

func get_config() (result config) {
	var flagRunAddr *string = pflag.StringP("address", "a", "localhost:8080",
		"address and port of server to connect")
	var pollInterval *uint = pflag.UintP("poll-interval", "p", 2,
		"number of seconds to update metrics")
	var reportInterval *uint = pflag.UintP("report-interval", "r", 10,
		"number of seconds to send metrics to server")

	err := env.Parse(&result)
	if err != nil {
		log.Println(err)
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

	return
}

func main() {
	var counter uint // счетчик не может быть меньше нуля

	conf := get_config()

	log.Println("Connect to server ", conf.Address)
	log.Println("pollInterval: ", conf.PollInterval)
	log.Println("reportInterval: ", conf.ReportInterval)

	mon := agent.NewDataCollector()
	client := http.Client{
		Timeout: time.Second * 1, // интервал ожидания: 1 секунда
	}

	for {
		mon.UpdMetrics()
		// отправляем данные только по достижении счетчиком заданного значения
		if counter*conf.PollInterval >= conf.ReportInterval {
			for key, val := range mon.GetValues() {
				url := fmt.Sprintf("http://%s/update/%s/%s/%f", conf.Address, val.MType, key, *val.Value)
				request, err := http.NewRequest(http.MethodPost, url, nil)
				if err != nil {
					log.Println(err)
				}
				request.Header.Set("Content-Type", "text/plain")

				response, err := client.Do(request)
				if err != nil {
					log.Println(err)
				} else if response.StatusCode != http.StatusOK {
					log.Printf("url: %s\n\tstatus code: %d\t", url, response.StatusCode)
					io.Copy(os.Stdout, response.Body)
					response.Body.Close()
				} else {
					log.Println("Sent: ", url)
					mon.OnSuccessSent(key)
				}
			}
			fmt.Println("All sent")
			counter = 0
		}
		time.Sleep(time.Duration(conf.PollInterval) * time.Second)
		counter += 1
	}
}
