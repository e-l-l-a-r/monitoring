package handler

import (
	"encoding/json"
	"errors"
	"html/template"
	"net/http"
	"strconv"

	"github.com/e-l-l-a-r/monitoring/internal/compressor"
	"github.com/e-l-l-a-r/monitoring/internal/logger"
	"github.com/e-l-l-a-r/monitoring/internal/model"
	"github.com/e-l-l-a-r/monitoring/internal/repository"
	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"
)

type Storage interface {
	AddData(name string, mtype string, value interface{}) error
	AddMetricData(metrics model.Metrics) error
	GetValues() map[string]model.Metrics
	GetValue(name string, mtype string) (float64, error)
	GetMetricValue(metric *model.Metrics) error
	SyncIfNeed() error
}

func listMetrics(storage Storage) http.HandlerFunc {
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
			if metric.MType == model.Counter {
				rows = append(rows, MetricRow{
					Name:  metric.ID,
					Type:  metric.MType,
					Value: float64(*metric.Delta),
				})
				continue
			}
			rows = append(rows, MetricRow{
				Name:  metric.ID,
				Type:  metric.MType,
				Value: *metric.Value,
			})
		}

		resp.Header().Set("Content-Type", "text/html")

		err = tmpl.Execute(resp, rows)
		if err != nil {
			http.Error(resp, "Internal Server Error", http.StatusInternalServerError)
		}
	}
}

func getMetric(storage Storage) http.HandlerFunc {
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

func GetRouter(storage Storage) *chi.Mux {
	if routesInstalled {
		return rtr // Return existing router
	}
	routesInstalled = true
	rtr = chi.NewRouter()

	rtr.Get("/", logger.ServerRequestLogger(listMetrics(storage)))
	rtr.Post("/", logger.ServerRequestLogger(incorrectApi))

	rtr.Post("/update/", logger.ServerRequestLogger(updJsonMetric(storage)))
	//rtr.Post("/update/", logger.ServerRequestLogger(incorrectApi))
	rtr.Post("/update/{mtype}/", logger.ServerRequestLogger(notFound))
	rtr.Post("/update/{mtype}/{name}/", logger.ServerRequestLogger(badRequest))
	rtr.Post("/update/{mtype}/{name}/{val}", logger.ServerRequestLogger(updMetric(storage)))

	rtr.Get("/value/{mtype}/{name}", logger.ServerRequestLogger(getMetric(storage)))
	rtr.Post("/value/", logger.ServerRequestLogger(getJsonMetric(storage)))

	rtr.Get("/ping", logger.ServerRequestLogger(pingDb(storage)))

	return rtr
}

func updMetric(storage Storage) http.HandlerFunc {
	return func(resp http.ResponseWriter, req *http.Request) {
		val, err := strconv.ParseFloat(chi.URLParam(req, "val"), 64)
		if err != nil {
			http.Error(resp, "Incorrect value", http.StatusBadRequest)
			return
		}
		switch chi.URLParam(req, "mtype") {
		case model.Gauge:
			err = storage.AddData(chi.URLParam(req, "name"), chi.URLParam(req, "mtype"), val)
		case model.Counter:
			err = storage.AddData(chi.URLParam(req, "name"), chi.URLParam(req, "mtype"), int64(val))
		default:
			http.Error(resp, "Incorrect type", http.StatusBadRequest)
		}
		if err != nil {
			logger.Warn("Update error: ", err.Error())
			http.Error(resp, err.Error(), http.StatusBadRequest)
			return
		}

		storage.SyncIfNeed()

		resp.Write([]byte(""))
	}

}

func updJsonMetric(storage Storage) http.HandlerFunc {
	return func(resp http.ResponseWriter, req *http.Request) {

		var metric model.Metrics

		dataReader, err := compressor.RequesrReader(req)
		if err != nil {
			http.Error(resp, err.Error(), http.StatusBadRequest)
			return
		}
		dec := json.NewDecoder(dataReader)

		if err := dec.Decode(&metric); err != nil {
			http.Error(resp, "Incorrect value", http.StatusBadRequest)
			logger.Info("cannot decode request JSON body", zap.Error(err))
			return
		}

		str, _ := json.Marshal(metric)
		logger.Info("Update metric: ", string(str))

		err = storage.AddMetricData(metric)
		if err != nil {
			http.Error(resp, err.Error(), http.StatusBadRequest)
			return
		}

		storage.SyncIfNeed()

		resp.Write([]byte(""))
	}
}

func getJsonMetric(storage Storage) http.HandlerFunc {
	return func(resp http.ResponseWriter, req *http.Request) {

		var metric model.Metrics
		dec := json.NewDecoder(req.Body)
		resp.Header().Set("Content-Type", "application/json")

		if err := dec.Decode(&metric); err != nil {
			http.Error(resp, "Incorrect value", http.StatusBadRequest)
			logger.Info("cannot decode request JSON body", zap.Error(err))
			return
		}

		storage.GetMetricValue(&metric)

		// сериализуем ответ сервера
		enc := json.NewEncoder(resp)
		if err := enc.Encode(metric); err != nil {
			logger.Info("error encoding response", zap.Error(err))
			return
		}
	}
}

func pingDb(storage Storage) http.HandlerFunc {
	return func(resp http.ResponseWriter, req *http.Request) {
		switch s := storage.(type) {
		case *repository.SqlStorage:
			// Здесь s имеет тип *repository.SqlStorage
			err := s.Ping()
			if err != nil {
				http.Error(resp, err.Error(), http.StatusInternalServerError)
				logger.Warn("Ping error: ", err.Error())
				return
			}
			resp.Write([]byte(""))
		default:
			http.Error(resp, "Incorrect storage type", http.StatusInternalServerError)
			logger.Warn("Incorrect storage type")
		}
	}
}
