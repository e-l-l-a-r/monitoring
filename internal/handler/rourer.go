package handler

import (
	"errors"
	"html/template"
	"net/http"
	"strconv"

	"github.com/e-l-l-a-r/monitoring/internal/logger"
	"github.com/e-l-l-a-r/monitoring/internal/repository"
	"github.com/go-chi/chi/v5"
)

func listMetrics(storage *repository.MemStorage) http.HandlerFunc {
	return func(resp http.ResponseWriter, req *http.Request) {
		type MetricRow struct {
			Name  string
			Type  string
			Value float64
		}

		const metricListTemplate = `
		<HTML>
		<BODY>
		<H2>Список метрик</H2>
		<table border="1" cellpadding="5" cellspacing="0">
			<tr>
				<th>Имя</th>
				<th>Тип</th>
				<th>Значение</th>
			</tr>
			{{range .}}
			<tr>
				<td>{{.Name}}</td>
				<td>{{.Type}}</td>
				<td>{{printf "%.2f" .Value}}</td>
			</tr>
			{{end}}
		</table>
		</BODY>
		</HTML>
		`

		tmpl, err := template.New("metric-list").Parse(metricListTemplate)
		if err != nil {
			http.Error(resp, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		rows := make([]MetricRow, 0, len(storage.GetValues()))
		for _, metric := range storage.GetValues() {
			rows = append(rows, MetricRow{
				Name:  metric.ID,
				Type:  metric.MType,
				Value: *metric.Value,
			})
		}

		err = tmpl.Execute(resp, rows)
		if err != nil {
			http.Error(resp, "Internal Server Error", http.StatusInternalServerError)
		}
	}
}

func getMetric(storage *repository.MemStorage) http.HandlerFunc {
	return func(resp http.ResponseWriter, req *http.Request) {
		val, err := storage.GetValue(chi.URLParam(req, "name"), chi.URLParam(req, "mtype"))
		if err != nil {
			if _, ok := errors.AsType[*repository.MetricNotFoundError](err); ok {
				http.Error(resp, err.Error(), http.StatusNotFound)
			} else if _, ok := errors.AsType[*repository.TypeMismatchError](err); ok {
				http.Error(resp, err.Error(), http.StatusBadRequest)
			} else {
				http.Error(resp, "Internal Server Error", http.StatusInternalServerError)
			}
			return
		}
		resp.Write([]byte(strconv.FormatFloat(val, 'f', -1, 64)))
	}
}

func incorrectApi(resp http.ResponseWriter, _ *http.Request) {
	http.Error(resp, "Incorrect API", http.StatusBadRequest)
}

func notFound(resp http.ResponseWriter, _ *http.Request) {
	http.Error(resp, "No metric name", http.StatusNotFound)

}
func badRequest(resp http.ResponseWriter, _ *http.Request) {
	http.Error(resp, "No value", http.StatusBadRequest)
}

var (
	routesInstalled bool
	rtr             *chi.Mux
)

func GetRouter() *chi.Mux {
	if routesInstalled {
		return rtr // Return existing router
	}
	routesInstalled = true
	rtr = chi.NewRouter()
	storage := repository.NewMemStorage()

	rtr.Get("/", logger.ServerRequestLogger(listMetrics(storage)))
	rtr.Post("/", logger.ServerRequestLogger(incorrectApi))

	rtr.Post("/update/", logger.ServerRequestLogger(incorrectApi))
	rtr.Post("/update/{mtype}/", logger.ServerRequestLogger(notFound))
	rtr.Post("/update/{mtype}/{name}/", logger.ServerRequestLogger(badRequest))
	rtr.Post("/update/{mtype}/{name}/{val}", logger.ServerRequestLogger(updMetric(storage)))

	rtr.Get("/value/{mtype}/{name}", logger.ServerRequestLogger(getMetric(storage)))

	return rtr
}

func updMetric(storage *repository.MemStorage) http.HandlerFunc {
	return func(resp http.ResponseWriter, req *http.Request) {
		val, err := strconv.ParseFloat(chi.URLParam(req, "val"), 64)
		if err != nil {
			http.Error(resp, "Incorrect value", http.StatusBadRequest)
			return
		}
		err = storage.AddData(chi.URLParam(req, "name"), chi.URLParam(req, "mtype"), val)
		if err != nil {
			http.Error(resp, err.Error(), http.StatusBadRequest)
			return
		}

		resp.Write([]byte(""))
	}

}
