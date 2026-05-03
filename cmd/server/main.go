package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/e-l-l-a-r/monitoring/internal/handler"
	"github.com/spf13/pflag"
)

type config struct {
	address string
}

func parseFlags() {
	pflag.CommandLine.AddGoFlagSet(flag.CommandLine)

	pflag.Usage = func() {
		fmt.Fprintf(pflag.CommandLine.Output(), "Metrics collecting server\nUsage of %s:\n", os.Args[0])
		pflag.PrintDefaults()
	}

	pflag.Parse()
}

func get_config() (result config) {
	var flagRunAddr *string = pflag.StringP("address", "a", "localhost:8080",
		"address and port to run server")

	parseFlags()

	result = config{
		*flagRunAddr,
	}

	return
}

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	conf := get_config()
	log.Println("Running server on ", conf.address)
	router := handler.GetRouter()
	err := http.ListenAndServe(conf.address, router)
	if err != nil {
		return err
	}
	return nil
}
