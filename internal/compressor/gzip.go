package compressor

import (
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
		gz_wr := gzipWriter{ResponseWriter: w, Writer: gz, Info: info}
		next.ServeHTTP(gz_wr, r)
		logger.Info("Send GZIPped response with size:", gz_wr.Info.compressedSize)
	})
}
