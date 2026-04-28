package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/e-l-l-a-r/monitoring/internal/agent"
)

const (
	pollInterval   = 2
	reportInterval = 10
	reportURL      = "http://localhost:8080/update"
)

func main() {
	var counter int8

	mon := agent.NewDataCollector()
	client := http.Client{
		Timeout: time.Second * 1, // интервал ожидания: 1 секунда
	}

	for {
		mon.UpdMetrics()
		// отправляем данные только по достижении счетчиком заданного значения
		if counter >= reportInterval {
			for key, val := range mon.GetValues() {
				url := fmt.Sprintf("%s/%s/%s/%f", reportURL, val.MType, key, val.Val)
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
		time.Sleep(pollInterval * time.Second)
		counter += pollInterval
	}
}
