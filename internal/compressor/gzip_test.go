package compressor

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGzipHandle(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		assert.NoError(t, err)
		w.Write(body)
	})

	gzipHandler := GzipHandle(handler)

	t.Run("no gzip", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/", bytes.NewBufferString("hello"))
		rec := httptest.NewRecorder()
		gzipHandler.ServeHTTP(rec, req)

		assert.Equal(t, "hello", rec.Body.String())
		assert.Empty(t, rec.Header().Get("Content-Encoding"))
	})

	t.Run("gzip response", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/", nil)
		req.Header.Set("Accept-Encoding", "gzip")
		rec := httptest.NewRecorder()
		gzipHandler.ServeHTTP(rec, req)

		assert.Equal(t, "gzip", rec.Header().Get("Content-Encoding"))

		zr, err := gzip.NewReader(rec.Body)
		require.NoError(t, err)
		defer zr.Close()

		content, err := io.ReadAll(zr)
		assert.NoError(t, err)
		assert.Equal(t, "", string(content))
	})

	t.Run("gzip request", func(t *testing.T) {
		var buf bytes.Buffer
		zw := gzip.NewWriter(&buf)
		_, err := zw.Write([]byte("compressed content"))
		require.NoError(t, err)
		zw.Close()

		req := httptest.NewRequest("POST", "/", &buf)
		req.Header.Set("Content-Encoding", "gzip")
		rec := httptest.NewRecorder()
		gzipHandler.ServeHTTP(rec, req)

		assert.Equal(t, "compressed content", rec.Body.String())
	})

	t.Run("gzip request and response", func(t *testing.T) {
		var buf bytes.Buffer
		zw := gzip.NewWriter(&buf)
		_, err := zw.Write([]byte("both directions"))
		require.NoError(t, err)
		zw.Close()

		req := httptest.NewRequest("POST", "/", &buf)
		req.Header.Set("Content-Encoding", "gzip")
		req.Header.Set("Accept-Encoding", "gzip")
		rec := httptest.NewRecorder()
		gzipHandler.ServeHTTP(rec, req)

		assert.Equal(t, "gzip", rec.Header().Get("Content-Encoding"))

		zr, err := gzip.NewReader(rec.Body)
		require.NoError(t, err)
		defer zr.Close()

		content, err := io.ReadAll(zr)
		assert.NoError(t, err)
		assert.Equal(t, "both directions", string(content))
	})
}

func TestNewGzippedReader(t *testing.T) {
	data := []byte("test data")
	reader, err := NewGzippedReader(data)
	require.NoError(t, err)

	zr, err := gzip.NewReader(reader)
	require.NoError(t, err)
	defer zr.Close()

	content, err := io.ReadAll(zr)
	assert.NoError(t, err)
	assert.Equal(t, data, content)
}

func TestRequestReader(t *testing.T) {
	t.Run("uncompressed", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/", bytes.NewBufferString("hello"))
		reader, err := RequestReader(req)
		assert.NoError(t, err)

		content, _ := io.ReadAll(reader)
		assert.Equal(t, "hello", string(content))
	})

	t.Run("compressed", func(t *testing.T) {
		var buf bytes.Buffer
		zw := gzip.NewWriter(&buf)
		zw.Write([]byte("compressed"))
		zw.Close()

		req := httptest.NewRequest("POST", "/", &buf)
		req.Header.Set("Content-Encoding", "gzip")

		reader, err := RequestReader(req)
		assert.NoError(t, err)

		content, _ := io.ReadAll(reader)
		assert.Equal(t, "compressed", string(content))
	})

	t.Run("invalid gzip", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/", bytes.NewBufferString("not gzip"))
		req.Header.Set("Content-Encoding", "gzip")

		_, err := RequestReader(req)
		assert.Error(t, err)
	})
}
