package crypto

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"

	"github.com/e-l-l-a-r/monitoring/internal/logger"
)

type (
	signer struct {
		key       string
		is_inited bool
	}

	SignedWriter struct {
		http.ResponseWriter
		hash []byte
	}
)

// Глобальная переменная для реализации работы синглтона
var singleSigner *signer

func GetSigner() (*signer, error) {
	if singleSigner == nil {
		return nil, fmt.Errorf("no signer inited")
	}
	return singleSigner, nil
}

func (s *signer) SignBytes(data []byte, init []byte) []byte {
	if !s.is_inited {
		return nil
	}
	h := hmac.New(sha256.New, []byte(s.key))
	if _, err := h.Write(data); err != nil {
		return nil
	}
	return h.Sum(init)
}

func (s *signer) SignData(data []byte) string {
	if !s.is_inited {
		return ""
	}
	h := hmac.New(sha256.New, []byte(s.key))
	if _, err := h.Write(data); err != nil {
		return ""
	}
	return hex.EncodeToString(h.Sum(nil))
}

func InitSigner(key string) (*signer, error) {
	singleSigner = &signer{
		key:       key,
		is_inited: (key != ""),
	}

	return GetSigner()
}

func NewSignedWriter(w http.ResponseWriter) *SignedWriter {
	s := new(SignedWriter)
	s.ResponseWriter = w
	s.hash = nil
	return s
}

func (s *SignedWriter) Write(p []byte) (int, error) {
	sign, _ := GetSigner()
	s.hash = sign.SignBytes(p, s.hash)
	return s.ResponseWriter.Write(p)
}

func SignHandle(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		signer, _ := GetSigner()
		if !signer.is_inited {
			// еслинет ключа шифрования ничего не днлаем, передаём управление
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
		r.Body.Close()
		sign := signer.SignData(data)
		if sign != key {
			// если ключ не совпадает,отбрасываем запрос с ошибкой
			logger.Warn("Bad sign: ", sign, " != ", key)
			http.Error(w, "Bad sign", http.StatusBadRequest)
			return
		}

		// создаём Writer поверх текущего w
		sw := NewSignedWriter(w)

		r.Body = io.NopCloser(bytes.NewBuffer(data))

		next.ServeHTTP(sw, r)

		w.Header().Set("HashSHA256", string(sw.hash))
	})
}

func NewSegnedReader(r io.Reader) (io.Reader, string, error) {
	sig, err := GetSigner()
	if err != nil {
		return nil, "", fmt.Errorf("get sgner error: %w", err)
	}

	data, _ := io.ReadAll(r)

	h := sig.SignData(data)

	return bytes.NewBuffer(data), h, nil
}
