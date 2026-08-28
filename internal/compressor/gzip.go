package compressor

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/e-l-l-a-r/monitoring/internal/logger"
)

type compressionInfo struct {
	compressedSize int
	originalSize   int
}

type gzipWriter struct {
	http.ResponseWriter
	Writer io.Writer
	Info   *compressionInfo
}

type gzipReadCloser struct {
	*gzip.Reader
	originalBody io.ReadCloser
}

func (w gzipWriter) Write(b []byte) (int, error) {
	// w.Writer будет отвечать за gzip-сжатие, поэтому пишем в него
	sz, err := w.Writer.Write(b)
	w.Info.compressedSize += sz
	w.Info.originalSize += len(b)
	return sz, err
}

func (g gzipReadCloser) Close() error {
	// Закрываем gzip reader и возвращаем его в пул
	err1 := g.Reader.Close()
	// Закрываем оригинальное тело запроса
	err2 := g.originalBody.Close()

	if err1 != nil {
		return err1
	}
	return err2
}

func GzipHandle(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.EqualFold(r.Header.Get("Content-Encoding"), "gzip") {
			dataReader, err := gzip.NewReader(r.Body)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}

			// Оборачиваем тело запроса
			r.Body = gzipReadCloser{originalBody: r.Body, Reader: dataReader}

			// Удаляем заголовки, чтобы хендлер ниже по цепочке не запутался
			r.Header.Del("Content-Encoding")
			r.Header.Del("Content-Length")
		}

		// проверяем, что клиент поддерживает gzip-сжатие
		if !strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
			// если gzip не поддерживается, передаём управление
			// дальше без изменений
			logger.Warn("GZIP is NOT supported by client")
			next.ServeHTTP(w, r)
			return
		}

		// создаём gzip.Writer поверх текущего w
		gz, err := gzip.NewWriterLevel(w, gzip.BestSpeed)
		if err != nil {
			io.WriteString(w, err.Error())
			logger.Warn("Error while creating GZIP writer")
			return
		}
		defer gz.Close()

		logger.Info("Set Content-Encoding to gzip")
		w.Header().Set("Content-Encoding", "gzip")

		info := &compressionInfo{
			originalSize:   0,
			compressedSize: 0,
		}
		gzWr := gzipWriter{ResponseWriter: w, Writer: gz, Info: info}
		next.ServeHTTP(gzWr, r)
		logger.Info("Send GZIPped response with size:", gzWr.Info.compressedSize)
	})
}

func NewGzippedReader(data []byte) (io.Reader, error) {
	var buf bytes.Buffer

	gz, err := gzip.NewWriterLevel(&buf, gzip.DefaultCompression)
	if err != nil {
		return nil, fmt.Errorf("create gzip writer: %w", err)
	}

	if _, err := gz.Write(data); err != nil {
		gz.Close()
		return nil, fmt.Errorf("write gzip data: %w", err)
	}

	if err := gz.Close(); err != nil {
		return nil, fmt.Errorf("close gzip writer: %w", err)
	}

	return &buf, nil
}

func RequesrReader(req *http.Request) (io.Reader, error) {
	if req.Header.Get("Content-Encoding") == "gzip" {
		gz, err := gzip.NewReader(req.Body)
		if err != nil {
			logger.Info("cannot unzip request body: ", err)
			return nil, err
		}
		return gz, nil
	}
	return req.Body, nil
}
