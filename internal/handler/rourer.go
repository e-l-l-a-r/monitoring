package handler

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/e-l-l-a-r/monitoring/internal/model"
	"github.com/go-chi/chi/v5"
)

var (
	storage *model.MemStorage
)

func init() {
	storage = model.NewMemStorage()
}

func listMetrics(resp http.ResponseWriter, req *http.Request) {
	resp.Write([]byte(fmt.Sprint("<HTML><BODY><H2>Список метрик</H2><table>")))
	resp.Write([]byte(fmt.Sprint("<tr><td>Имя</td><td>Тип</td><td>Значение</td></tr>")))
	for name, metric := range storage.GetValues() {
		resp.Write([]byte(fmt.Sprintf("<tr><td>%s</td><td>%s</td><td>%.2f</td></tr>",
			name, metric.MType, *metric.Value)))
	}
	resp.Write([]byte(fmt.Sprint("</table></BODY></HTML>")))
}

func formatFloat(f float64) string {
	// Сначала форматируем до 2 знаков после запятой
	s := fmt.Sprintf("%.3f", f)

	// Убираем лишние нули и точку, если она в конце
	s = strings.TrimRight(s, "0")
	s = strings.TrimRight(s, ".")
	return s
}

func getMetric(resp http.ResponseWriter, req *http.Request) {
	val, err := storage.GetValue(chi.URLParam(req, "name"), chi.URLParam(req, "mtype"))
	if err != nil {
		switch val {
		case 0:
			http.Error(resp, err.Error(), http.StatusBadRequest)
		case -1:
			http.Error(resp, err.Error(), http.StatusNotFound)
		}

		return
	}
	resp.Write([]byte(formatFloat(val)))
}

func incorrectApi(resp http.ResponseWriter, req *http.Request) {
	http.Error(resp, "Incorrect API", http.StatusBadRequest)
}

func notFound(resp http.ResponseWriter, req *http.Request) {
	http.Error(resp, "No metric name", http.StatusNotFound)

}
func badRequest(resp http.ResponseWriter, req *http.Request) {
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

	rtr.Route("/", func(r chi.Router) {
		r.Get("/", listMetrics)
		r.Post("/", incorrectApi)
		r.Route("/update", func(r chi.Router) {
			r.Post("/", incorrectApi)
			r.Route("/{mtype}", func(r chi.Router) {
				r.Post("/", notFound)
				r.Route("/{name}", func(r chi.Router) {
					r.Post("/", badRequest)
					r.Route("/{val}", func(r chi.Router) {
						r.Post("/", updMetric)
					})
				})
			})
		})
		r.Route("/value", func(r chi.Router) {
			r.Route("/{mtype}", func(r chi.Router) {
				r.Route("/{name}", func(r chi.Router) {
					r.Get("/", getMetric)
				})
			})
		})
	})

	return rtr
}

func updMetric(resp http.ResponseWriter, req *http.Request) {
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
