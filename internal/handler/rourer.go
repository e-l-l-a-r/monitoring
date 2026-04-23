package handler

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/e-l-l-a-r/monitoring/internal/model"
)

type router struct {
	mux *http.ServeMux
}

var (
	storage *model.MemStorage
)

func init() {
	storage = model.NewMemStorage()
}

func NewRouter() *router {
	rtr := &router{
		mux: http.NewServeMux(),
	}

	rtr.mux.HandleFunc(`/`, http.NotFound)
	rtr.mux.HandleFunc(`/update/`, updateHandler)
	return rtr
}

func (r router) GetMux() *http.ServeMux {
	return r.mux
}

func updateHandler(resp http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		http.Error(resp, "Only POST requests are allowed!", http.StatusMethodNotAllowed)
		return
	}
	path, _ := strings.CutPrefix(req.URL.Path, req.Pattern)
	path_parts := strings.Split(path, "/")
	switch {
	case len(path_parts) == 0 || path_parts[0] == "":
		{
			http.Error(resp, "Incorrect API", http.StatusBadRequest)
			return
		}
	case len(path_parts) == 1 || path_parts[1] == "":
		{
			http.Error(resp, "No metric name", http.StatusNotFound)
			return
		}
	case len(path_parts) == 2 || path_parts[2] == "":
		{
			http.Error(resp, "No value", http.StatusBadRequest)
			return
		}
	}

	val, err := strconv.ParseFloat(path_parts[2], 64)
	if err != nil {
		http.Error(resp, "Incorrect value", http.StatusBadRequest)
		return
	}
	err = storage.AddData(path_parts[1], path_parts[0], val)
	if err != nil {
		http.Error(resp, err.Error(), http.StatusBadRequest)
		return
	}

	resp.Write([]byte(""))
}
