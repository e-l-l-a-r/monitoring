package crypto

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSigner(t *testing.T) {
	key := "secret"
	s := NewSigner(key)
	assert.True(t, s.isInited)

	data := []byte("hello world")

	t.Run("SignBytes", func(t *testing.T) {
		hash := s.SignBytes(data, nil)
		assert.NotEmpty(t, hash)

		// Повторная подпись тех же данных должна давать тот же хеш, если init равен nil
		hash2 := s.SignBytes(data, nil)
		assert.Equal(t, hash, hash2)

		// Инкрементальная подпись
		part1 := []byte("hello ")
		part2 := []byte("world")
		h1 := s.SignBytes(part1, nil)
		h2 := s.SignBytes(part2, h1)

		// Обычно HMAC работает с Sum(init) не так, но посмотрим, что делает реализация.
		// Она вызывает h.Sum(init), что добавляет HMAC в конец init.
		assert.Equal(t, append(h1, s.SignBytes(part2, nil)...), h2)
	})

	t.Run("SignData", func(t *testing.T) {
		hashStr := s.SignData(data)
		assert.NotEmpty(t, hashStr)
	})

	t.Run("Not Inited", func(t *testing.T) {
		s2 := NewSigner("")
		assert.False(t, s2.isInited)
		assert.Nil(t, s2.SignBytes(data, nil))
		assert.Empty(t, s2.SignData(data))
	})

	t.Run("nil signer", func(t *testing.T) {
		var s2 *Signer
		assert.Nil(t, s2.SignBytes(data, nil))
		assert.Empty(t, s2.SignData(data))
	})
}

func TestSignerHandle(t *testing.T) {
	key := "secret"
	signer := NewSigner(key)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		w.Write(body)
	})

	signedHandler := signer.Handle(handler)

	t.Run("валидная подпись", func(t *testing.T) {
		data := []byte("signed data")
		signature := signer.SignData(data)

		req := httptest.NewRequest("POST", "/", bytes.NewBuffer(data))
		req.Header.Set("HashSHA256", signature)
		rec := httptest.NewRecorder()

		signedHandler.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Equal(t, string(data), rec.Body.String())
		assert.NotEmpty(t, rec.Header().Get("HashSHA256"))
	})

	t.Run("невалидная подпись", func(t *testing.T) {
		data := []byte("signed data")
		req := httptest.NewRequest("POST", "/", bytes.NewBuffer(data))
		req.Header.Set("HashSHA256", "wrong")
		rec := httptest.NewRecorder()

		signedHandler.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("отсутствует заголовок подписи", func(t *testing.T) {
		data := []byte("data")
		req := httptest.NewRequest("POST", "/", bytes.NewBuffer(data))
		rec := httptest.NewRecorder()

		signedHandler.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Equal(t, "data", rec.Body.String())
	})

	t.Run("отсутствует ключ подписи", func(t *testing.T) {
		data := []byte("data")
		req := httptest.NewRequest("POST", "/", bytes.NewBuffer(data))
		rec := httptest.NewRecorder()

		NewSigner("").Handle(handler).ServeHTTP(rec, req)
		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Equal(t, "data", rec.Body.String())
	})

	t.Run("nil signer пропускает запрос", func(t *testing.T) {
		var nilSigner *Signer
		req := httptest.NewRequest("POST", "/", bytes.NewBufferString("data"))
		rec := httptest.NewRecorder()

		nilSigner.Handle(handler).ServeHTTP(rec, req)
		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Equal(t, "data", rec.Body.String())
	})
}

func TestSignerNewReader(t *testing.T) {
	data := []byte("data to read")

	reader, hash, err := NewSigner("secret").NewReader(bytes.NewBuffer(data))
	require.NoError(t, err)
	assert.NotEmpty(t, hash)

	readData, _ := io.ReadAll(reader)
	assert.Equal(t, data, readData)
}
