// Package crypto предоставляет инструменты для подписи данных с использованием HMAC-SHA256
// и для шифрования данных RSA-OAEP.
package crypto

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"

	"github.com/e-l-l-a-r/monitoring/internal/logger"
)

// SignedWriter — обертка для http.ResponseWriter, выполняющая подпись передаваемых данных.
type SignedWriter struct {
	http.ResponseWriter
	signer *Signer
	hash   []byte
}

// Signer подписывает данные HMAC-SHA256 с секретным ключом. Пустой ключ означает,
// что подпись выключена. Нулевой *Signer также считается выключенным.
type Signer struct {
	key      string
	isInited bool
}

// NewSigner создаёт подписчик с секретным ключом. Пустой key выключает подпись.
func NewSigner(key string) *Signer {
	return &Signer{
		key:      key,
		isInited: key != "",
	}
}

// enabled сообщает, включена ли подпись.
func (s *Signer) enabled() bool {
	return s != nil && s.isInited
}

// SignBytes вычисляет HMAC данных и дописывает его к init. Если подпись выключена, возвращает nil.
func (s *Signer) SignBytes(data []byte, init []byte) []byte {
	if !s.enabled() {
		return nil
	}
	h := hmac.New(sha256.New, []byte(s.key))
	if _, err := h.Write(data); err != nil {
		return nil
	}
	return h.Sum(init)
}

// SignData возвращает HMAC данных в виде hex-строки. Если подпись выключена, возвращает пустую строку.
func (s *Signer) SignData(data []byte) string {
	if !s.enabled() {
		return ""
	}
	h := hmac.New(sha256.New, []byte(s.key))
	if _, err := h.Write(data); err != nil {
		return ""
	}
	return hex.EncodeToString(h.Sum(nil))
}

// NewSignedWriter создает новый SignedWriter для автоматической подписи HTTP-ответов signer-ом s.
func NewSignedWriter(w http.ResponseWriter, s *Signer) *SignedWriter {
	sw := new(SignedWriter)
	sw.ResponseWriter = w
	sw.signer = s
	sw.hash = nil
	return sw
}

// Write записывает данные в ответ и обновляет подпись.
func (s *SignedWriter) Write(p []byte) (int, error) {
	s.hash = s.signer.SignBytes(p, s.hash)
	return s.ResponseWriter.Write(p)
}

// Handle — middleware для проверки подписи входящих запросов и подписи исходящих ответов.
// Если подпись выключена, запрос передаётся дальше без изменений.
func (s *Signer) Handle(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !s.enabled() {
			// если нет ключа шифрования ничего не делаем, передаём управление
			// дальше без изменений
			logger.Warn("No crypto key specified")
			next.ServeHTTP(w, r)
			return
		}

		key := r.Header.Get("HashSHA256")
		logger.Info("Request signed with key: ", key)
		if key == "" {
			//// если ключ не передан,отбрасываем запрос с ошибкой
			//http.Error(w, "No sign", http.StatusBadRequest)
			next.ServeHTTP(w, r)
			return
		}

		data, _ := io.ReadAll(r.Body)
		_ = r.Body.Close()
		sign := s.SignData(data)
		if sign != key {
			// если ключ не совпадает,отбрасываем запрос с ошибкой
			logger.Warn("Bad sign: ", sign, " != ", key)
			http.Error(w, "Bad sign", http.StatusBadRequest)
			return
		}

		// создаём Writer поверх текущего w
		sw := NewSignedWriter(w, s)

		r.Body = io.NopCloser(bytes.NewBuffer(data))

		next.ServeHTTP(sw, r)

		w.Header().Set("HashSHA256", string(sw.hash))
	})
}

// NewReader вычисляет подпись для данных из Reader и возвращает новый Reader с теми же данными.
// Если подпись выключена, подпись пустая.
func (s *Signer) NewReader(r io.Reader) (io.Reader, string, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, "", err
	}

	return bytes.NewBuffer(data), s.SignData(data), nil
}
