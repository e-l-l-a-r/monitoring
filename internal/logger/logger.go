package logger

import (
	"fmt"
	"io"
	console "log"
	"net/http"
	"time"

	"go.uber.org/zap"
)

type (
	// структура для хранения сведений об ответе
	responseData struct {
		status int
		size   int
	}

	// оберточная структура для http.ResponseWriter
	loggingResponseWriter struct {
		http.ResponseWriter // оригинальный http.ResponseWriter
		responseData        *responseData
	}

	// структура самого логгера
	logger struct {
		zap.Logger
	}
)

// Глобальная переменная для реализации работы логгера - синглтона
var singleLogger *logger

func GetLogger() (*logger, error) {
	if singleLogger == nil {
		return nil, fmt.Errorf("no logger inited")
	}
	return singleLogger, nil
}

func InitLogger(level string) (*logger, error) {
	// преобразуем текстовый уровень логирования в zap.AtomicLevel
	lvl, err := zap.ParseAtomicLevel(level)
	if err != nil {
		return nil, err
	}
	// создаём новую конфигурацию логера
	cfg := zap.NewProductionConfig()
	// устанавливаем уровень
	cfg.Level = lvl
	// создаём логер на основе конфигурации
	log, err := cfg.Build()
	if err != nil {
		return nil, err
	}
	singleLogger = &logger{
		*log,
	}
	return GetLogger()
}

func (r *loggingResponseWriter) Write(b []byte) (int, error) {
	// записываем ответ, используя оригинальный http.ResponseWriter
	size, err := r.ResponseWriter.Write(b)
	r.responseData.size += size // захватываем размер
	return size, err
}

func (r *loggingResponseWriter) WriteHeader(statusCode int) {
	// записываем код статуса, используя оригинальный http.ResponseWriter
	r.ResponseWriter.WriteHeader(statusCode)
	r.responseData.status = statusCode // захватываем код статуса
}

func (l *logger) ServerRequestLogger(h http.HandlerFunc) http.HandlerFunc {
	logFn := func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		responseData := &responseData{
			status: 0,
			size:   0,
		}
		lw := loggingResponseWriter{
			ResponseWriter: w, // встраиваем оригинальный http.ResponseWriter
			responseData:   responseData,
		}
		h.ServeHTTP(&lw, r) // внедряем реализацию http.ResponseWriter

		duration := time.Since(start)

		l.Sugar().Infoln(
			"uri", r.RequestURI,
			"method", r.Method,
			"status", responseData.status, // получаем перехваченный код статуса ответа
			"duration", duration,
			"size", responseData.size, // получаем перехваченный размер ответа
		)
	}
	return http.HandlerFunc(logFn)
}

func (l *logger) DoRequestWithLog(c *http.Client, req *http.Request) (resp *http.Response, err error) {
	resp, err = c.Do(req)
	if err != nil {
		l.WarnMsg(err)
	} else if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		l.InfoMsg("url:", req.URL, "\tstatus code:", resp.StatusCode, "Body", string(bodyBytes))
		err = fmt.Errorf("status code: %d", resp.StatusCode)
		//io.Copy(os.Stdout, resp.Body)
		resp.Body.Close()
	} else {
		l.InfoMsg("Sent: ", req.URL, "Data: ", req.Body)
	}
	return
}

func (l *logger) InfoMsg(args ...interface{}) {
	if l != nil {
		l.Sugar().Infoln(args)
	} else {
		console.Println(args...)
	}
}

func (l *logger) WarnMsg(args ...interface{}) {
	if l != nil {
		l.Sugar().Warnln(args)
	} else {
		console.Println(args...)
	}
}

func ServerRequestLogger(h http.HandlerFunc) http.HandlerFunc {
	if singleLogger != nil {
		return singleLogger.ServerRequestLogger(h)
	}

	console.Println("No logger inited to process request")
	return h
}

func Fatal(args ...interface{}) {
	if singleLogger != nil {
		singleLogger.Sugar().Fatalln(args)
	} else {
		console.Fatal(args...)
	}
}

func Warn(args ...interface{}) {
	if singleLogger != nil {
		singleLogger.Sugar().Warnln(args)
	} else {
		console.Println(args...)
	}
}

func Info(args ...interface{}) {
	if singleLogger != nil {
		singleLogger.Sugar().Infoln(args)
	} else {
		console.Println(args...)
	}
}
