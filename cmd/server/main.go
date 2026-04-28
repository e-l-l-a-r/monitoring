package main

import (
	"net/http"

	"github.com/e-l-l-a-r/monitoring/internal/handler"
)

func main() {
	if err := run(); err != nil {
		panic(err)
	}
}

func run() error {
	router := handler.GetRouter()
	err := http.ListenAndServe(`:8080`, router)
	if err != nil {
		panic(err)
	}
	return nil
}
