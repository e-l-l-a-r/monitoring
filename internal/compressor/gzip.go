package compressor

import (
	"bytes"
	"compress/gzip"
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

func (w gzipWriter) Write(b []byte) (int, error) {
	// w.Writer будет отвечать за gzip-сжатие, поэтому пишем в него
	sz, err := w.Writer.Write(b)
	w.Info.compressedSize += sz
	w.Info.originalSize += len(b)
	return sz, err
}

func GzipHandle(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
	reader, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return bytes.NewReader(data), err
	}
	return reader, nil
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
