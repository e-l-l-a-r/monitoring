package main

import (
	goflag "flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/e-l-l-a-r/monitoring/internal/agent"
	flag "github.com/spf13/pflag"
)

var flagRunAddr *string = flag.StringP("address", "a", "localhost:8080",
	"address and port to run server")
var pollInterval *int8 = flag.Int8P("poll-interval", "p", 2,
	"number of seconds to update metrics")
var reportInterval *int8 = flag.Int8P("report-interval", "r", 10,
	"number of seconds to send metrics to server")

func parseFlags() {
	flag.CommandLine.AddGoFlagSet(goflag.CommandLine)

	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Metrics collecting agent\nUsage of %s:\n", os.Args[0])
		flag.PrintDefaults()
	}

	flag.Parse()
}

func main() {
	var counter int8

	parseFlags()

	fmt.Println("Connect to server ", *flagRunAddr)
	fmt.Println("pollInterval: ", *pollInterval)
	fmt.Println("reportInterval: ", *reportInterval)

	mon := agent.NewDataCollector()
	client := http.Client{
		Timeout: time.Second * 1, // интервал ожидания: 1 секунда
	}

	for {
		mon.UpdMetrics()
		// отправляем данные только по достижении счетчиком заданного значения
		if counter >= *reportInterval {
			for key, val := range mon.GetValues() {
				url := fmt.Sprintf("http://%s/update/%s/%s/%f", *flagRunAddr, val.MType, key, val.Val)
				request, err := http.NewRequest(http.MethodPost, url, nil)
				if err != nil {
					panic(err)
				}
				request.Header.Set("Content-Type", "text/plain")

				response, err := client.Do(request)
				if err != nil {
					panic(err)
				}
				if response.StatusCode != http.StatusOK {
					fmt.Printf("url: %s\n\tstatus code: %d\t", url, response.StatusCode)
					io.Copy(os.Stdout, response.Body)
					response.Body.Close()
				}
			}
			fmt.Println("All sent")
			counter = 0
		}
		time.Sleep(time.Duration(*pollInterval) * time.Second)
		counter += *pollInterval
	}
}
