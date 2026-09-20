package crypto

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"os"
)

// encryptor шифрует данные открытым ключом RSA.
type encryptor struct {
	key      *rsa.PublicKey
	isInited bool
}

// decryptor расшифровывает данные закрытым ключом RSA.
type decryptor struct {
	key      *rsa.PrivateKey
	isInited bool
}

// Глобальные переменные для реализации работы синглтонов
var (
	singleEncryptor *encryptor
	singleDecryptor *decryptor
)

// GetEncryptor возвращает текущий экземпляр шифровальщика.
func GetEncryptor() (*encryptor, error) {
	if singleEncryptor == nil {
		return nil, fmt.Errorf("no encryptor inited")
	}
	return singleEncryptor, nil
}

// GetDecryptor возвращает текущий экземпляр расшифровщика.
func GetDecryptor() (*decryptor, error) {
	if singleDecryptor == nil {
		return nil, fmt.Errorf("no decryptor inited")
	}
	return singleDecryptor, nil
}

// InitEncryptor инициализирует глобальный шифровальщик открытым ключом из файла.
// Пустой путь означает, что шифрование выключено.
func InitEncryptor(path string) (*encryptor, error) {
	singleEncryptor = &encryptor{}

	if path == "" {
		return GetEncryptor()
	}

	key, err := readPublicKey(path)
	if err != nil {
		return nil, err
	}

	singleEncryptor.key = key
	singleEncryptor.isInited = true

	return GetEncryptor()
}

// InitDecryptor инициализирует глобальный расшифровщик закрытым ключом из файла.
// Пустой путь означает, что расшифровка выключена.
func InitDecryptor(path string) (*decryptor, error) {
	singleDecryptor = &decryptor{}

	if path == "" {
		return GetDecryptor()
	}

	key, err := readPrivateKey(path)
	if err != nil {
		return nil, err
	}

	singleDecryptor.key = key
	singleDecryptor.isInited = true

	return GetDecryptor()
}

// readPEMBlock читает файл и декодирует из него первый PEM-блок.
func readPEMBlock(path string) (*pem.Block, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read key file %s: %w", path, err)
	}

	block, _ := pem.Decode(data)
	if block == nil {
		return nil, fmt.Errorf("decode pem block from %s: no pem data found", path)
	}

	return block, nil
}

// readPublicKey загружает открытый ключ RSA в формате PKIX или PKCS#1.
func readPublicKey(path string) (*rsa.PublicKey, error) {
	block, err := readPEMBlock(path)
	if err != nil {
		return nil, err
	}

	if key, err := x509.ParsePKIXPublicKey(block.Bytes); err == nil {
		rsaKey, ok := key.(*rsa.PublicKey)
		if !ok {
			return nil, fmt.Errorf("parse public key %s: not an rsa key", path)
		}
		return rsaKey, nil
	}

	key, err := x509.ParsePKCS1PublicKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse public key %s: %w", path, err)
	}

	return key, nil
}

// readPrivateKey загружает закрытый ключ RSA в формате PKCS#8 или PKCS#1.
func readPrivateKey(path string) (*rsa.PrivateKey, error) {
	block, err := readPEMBlock(path)
	if err != nil {
		return nil, err
	}

	if key, err := x509.ParsePKCS8PrivateKey(block.Bytes); err == nil {
		rsaKey, ok := key.(*rsa.PrivateKey)
		if !ok {
			return nil, fmt.Errorf("parse private key %s: not an rsa key", path)
		}
		return rsaKey, nil
	}

	key, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse private key %s: %w", path, err)
	}

	return key, nil
}

// encrypt шифрует данные поблочно алгоритмом RSA-OAEP.
func (e *encryptor) encrypt(data []byte) ([]byte, error) {
	size := e.key.Size()
	maxChunk := size - 2*sha256.Size - 2
	if maxChunk <= 0 {
		return nil, fmt.Errorf("encrypt data: rsa key is too short: %d bytes", size)
	}

	blocks := (len(data) + maxChunk - 1) / maxChunk
	result := make([]byte, 0, blocks*size)

	for start := 0; start < len(data); start += maxChunk {
		end := start + maxChunk
		if end > len(data) {
			end = len(data)
		}

		block, err := rsa.EncryptOAEP(sha256.New(), rand.Reader, e.key, data[start:end], nil)
		if err != nil {
			return nil, fmt.Errorf("encrypt data: %w", err)
		}

		result = append(result, block...)
	}

	return result, nil
}

// decrypt расшифровывает поблочно зашифрованные алгоритмом RSA-OAEP данные.
func (d *decryptor) decrypt(data []byte) ([]byte, error) {
	size := d.key.Size()
	if len(data) == 0 || len(data)%size != 0 {
		return nil, fmt.Errorf("decrypt data: body length %d is not a multiple of block size %d",
			len(data), size)
	}

	result := make([]byte, 0, len(data))

	for start := 0; start < len(data); start += size {
		block, err := rsa.DecryptOAEP(sha256.New(), rand.Reader, d.key, data[start:start+size], nil)
		if err != nil {
			return nil, fmt.Errorf("decrypt data: %w", err)
		}

		result = append(result, block...)
	}

	return result, nil
}

// NewEncryptedReader шифрует данные из Reader и возвращает новый Reader с зашифрованным текстом.
// Если шифрование выключено, исходный Reader возвращается без изменений.
func NewEncryptedReader(r io.Reader) (io.Reader, error) {
	enc, err := GetEncryptor()
	if err != nil {
		return nil, fmt.Errorf("get encryptor error: %w", err)
	}

	if !enc.isInited {
		return r, nil
	}

	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("read data to encrypt: %w", err)
	}

	encrypted, err := enc.encrypt(data)
	if err != nil {
		return nil, err
	}

	return bytes.NewReader(encrypted), nil
}

// DecryptHandle — middleware для расшифровки тела входящих запросов.
// Если расшифровка выключена, запрос передается дальше без изменений.
func DecryptHandle(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		dec, err := GetDecryptor()
		if err != nil || !dec.isInited {
			// если нет ключа расшифровки, передаём управление
			// дальше без изменений
			next.ServeHTTP(w, r)
			return
		}

		data, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "Cannot read body", http.StatusBadRequest)
			return
		}
		r.Body.Close()

		plain, err := dec.decrypt(data)
		if err != nil {
			http.Error(w, "Cannot decrypt body", http.StatusBadRequest)
			return
		}

		r.Body = io.NopCloser(bytes.NewReader(plain))
		r.ContentLength = int64(len(plain))
		r.Header.Del("Content-Length")

		next.ServeHTTP(w, r)
	})
}
