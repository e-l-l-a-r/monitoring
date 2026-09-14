// Пакет logger предоставляет функционал для логирования HTTP-запросов и системных событий.
package logger

import (
	"fmt"
	"io"
	console "log"
	"net/http"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
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

// GetLogger возвращает текущий экземпляр логгера. Возвращает ошибку, если логгер не инициализирован.
func GetLogger() (*logger, error) {
	if singleLogger == nil {
		return nil, fmt.Errorf("no logger inited")
	}
	return singleLogger, nil
}

// InitLogger инициализирует глобальный логгер с заданным уровнем логирования (например, "info", "debug").
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
	cfg.Encoding = "console"
	cfg.EncoderConfig.ConsoleSeparator = "\t| "
	// формат времени
	cfg.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder // или RFC3339TimeEncoder, EpochTimeEncoder
	// формат уровня логирования
	cfg.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	// настраиваем формат caller
	cfg.EncoderConfig.EncodeCaller = zapcore.ShortCallerEncoder
	// создаём логер на основе конфигурации
	log, err := cfg.Build(zap.AddCallerSkip(1))
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

// ServerRequestLogger — middleware для логирования входящих HTTP-запросов.
// Логирует URI, метод, статус ответа, длительность и размер.
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

// DoRequestWithLog выполняет HTTP-запрос и логирует результат.
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

// InfoMsg логирует сообщение на уровне Info.
func (l *logger) InfoMsg(args ...interface{}) {
	if l != nil {
		l.Sugar().Infoln(args)
	} else {
		console.Println(args...)
	}
}

// WarnMsg логирует сообщение на уровне Warn.
func (l *logger) WarnMsg(args ...interface{}) {
	if l != nil {
		l.Sugar().Warnln(args)
	} else {
		console.Println(args...)
	}
}

// ServerRequestLogger — глобальная обертка для ServerRequestLogger.
func ServerRequestLogger(h http.HandlerFunc) http.HandlerFunc {
	if singleLogger != nil {
		return singleLogger.ServerRequestLogger(h)
	}

	console.Println("No logger inited to process request")
	return h
}

// Fatal логирует сообщение и завершает выполнение программы.
func Fatal(args ...interface{}) {
	if singleLogger != nil {
		singleLogger.Sugar().Fatalln(args)
	} else {
		console.Fatal(args...)
	}
}

// Warn логирует сообщение на уровне Warn.
func Warn(args ...interface{}) {
	if singleLogger != nil {
		singleLogger.Sugar().Warnln(args)
	} else {
		console.Println(args...)
	}
}

// Info логирует сообщение на уровне Info.
func Info(args ...interface{}) {
	if singleLogger != nil {
		singleLogger.Sugar().Infoln(args)
	} else {
		console.Println(args...)
	}
}
