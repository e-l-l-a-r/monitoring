// Пакет compressor предоставляет инструменты для сжатия и распаковки HTTP-трафика с использованием gzip.
package compressor

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"

	"github.com/e-l-l-a-r/monitoring/internal/logger"
)

var writerPool = sync.Pool{
	New: func() any {
		w, _ := gzip.NewWriterLevel(io.Discard, gzip.BestSpeed)
		return w
	},
}

var readerPool = sync.Pool{
	New: func() any {
		return new(gzip.Reader)
	},
}

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
	readerPool.Put(g.Reader)
	// Закрываем оригинальное тело запроса
	err2 := g.originalBody.Close()

	if err1 != nil {
		return err1
	}
	return err2
}

// GzipHandle — middleware для обработки сжатого входящего трафика и автоматического сжатия ответов.
func GzipHandle(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.EqualFold(r.Header.Get("Content-Encoding"), "gzip") {
			dataReader := readerPool.Get().(*gzip.Reader)
			err := dataReader.Reset(r.Body)
			if err != nil {
				readerPool.Put(dataReader)
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
		gz := writerPool.Get().(*gzip.Writer)
		gz.Reset(w)
		defer func() {
			gz.Close()
			writerPool.Put(gz)
		}()

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

// NewGzippedReader создает io.Reader, который возвращает сжатые данные.
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

// RequestReader возвращает io.Reader для тела запроса, автоматически распаковывая его, если оно сжато.
func RequestReader(req *http.Request) (io.Reader, error) {
	if req.Header.Get("Content-Encoding") == "gzip" {
		dataReader := readerPool.Get().(*gzip.Reader)
		err := dataReader.Reset(req.Body)
		if err != nil {
			readerPool.Put(dataReader)
			logger.Info("cannot unzip request body: ", err)
			return nil, err
		}
		req.Body = gzipReadCloser{originalBody: req.Body, Reader: dataReader}
		req.Header.Del("Content-Encoding")
		req.Header.Del("Content-Length")
	}
	return req.Body, nil
}
