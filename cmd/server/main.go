package main

import (
	goflag "flag"
	"fmt"
	"net/http"
	"os"

	"github.com/e-l-l-a-r/monitoring/internal/handler"
	flag "github.com/spf13/pflag"
)

var flagRunAddr *string = flag.StringP("address", "a", "localhost:8080", "address and port to run server")

func parseFlags() {
	flag.CommandLine.AddGoFlagSet(goflag.CommandLine)

	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Metrics collecting server\nUsage of %s:\n", os.Args[0])
		flag.PrintDefaults()
	}

	flag.Parse()
}

func main() {
	if err := run(); err != nil {
		panic(err)
	}
}

func run() error {
	parseFlags()
	fmt.Println("Running server on", *flagRunAddr)
	router := handler.GetRouter()
	err := http.ListenAndServe(*flagRunAddr, router)
	if err != nil {
		panic(err)
	}
	return nil
}
