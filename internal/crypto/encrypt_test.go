package crypto

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeKeyPair генерирует пару ключей RSA и пишет их во временные PEM-файлы.
func writeKeyPair(t *testing.T) (pubPath, privPath string) {
	t.Helper()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	dir := t.TempDir()

	pubBytes, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	require.NoError(t, err)
	pubPath = filepath.Join(dir, "public.pem")
	require.NoError(t, os.WriteFile(pubPath,
		pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubBytes}), 0o600))

	privBytes, err := x509.MarshalPKCS8PrivateKey(key)
	require.NoError(t, err)
	privPath = filepath.Join(dir, "private.pem")
	require.NoError(t, os.WriteFile(privPath,
		pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privBytes}), 0o600))

	return pubPath, privPath
}

func TestNewKeys(t *testing.T) {
	pubPath, privPath := writeKeyPair(t)

	garbageDir := t.TempDir()
	garbagePath := filepath.Join(garbageDir, "garbage.pem")
	require.NoError(t, os.WriteFile(garbagePath, []byte("not a pem at all"), 0o600))

	missingPath := filepath.Join(garbageDir, "missing.pem")

	tests := []struct {
		name     string
		path     string
		wantErr  bool
		isInited bool
	}{
		{name: "valid public key", path: pubPath, wantErr: false, isInited: true},
		{name: "empty path disables encryption", path: "", wantErr: false, isInited: false},
		{name: "missing file", path: missingPath, wantErr: true},
		{name: "garbage instead of pem", path: garbagePath, wantErr: true},
	}

	for _, tt := range tests {
		t.Run("encryptor/"+tt.name, func(t *testing.T) {
			enc, err := NewEncryptor(tt.path)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.isInited, enc.isInited)
		})
	}

	privTests := []struct {
		name     string
		path     string
		wantErr  bool
		isInited bool
	}{
		{name: "valid private key", path: privPath, wantErr: false, isInited: true},
		{name: "empty path disables decryption", path: "", wantErr: false, isInited: false},
		{name: "missing file", path: missingPath, wantErr: true},
		{name: "garbage instead of pem", path: garbagePath, wantErr: true},
		{name: "public key as private", path: pubPath, wantErr: true},
	}

	for _, tt := range privTests {
		t.Run("decryptor/"+tt.name, func(t *testing.T) {
			dec, err := NewDecryptor(tt.path)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.isInited, dec.isInited)
		})
	}
}

func TestEncryptDecryptRoundTrip(t *testing.T) {
	pubPath, privPath := writeKeyPair(t)

	big := make([]byte, 5000)
	_, err := rand.Read(big)
	require.NoError(t, err)

	tests := []struct {
		name string
		data []byte
	}{
		{name: "one byte", data: []byte("x")},
		{name: "small payload", data: []byte(`{"id":"Alloc","type":"gauge","value":3.14}`)},
		{name: "exactly one block", data: bytes.Repeat([]byte("a"), 190)},
		{name: "one byte over block", data: bytes.Repeat([]byte("b"), 191)},
		{name: "multi block random", data: big},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			enc, err := NewEncryptor(pubPath)
			require.NoError(t, err)
			dec, err := NewDecryptor(privPath)
			require.NoError(t, err)

			reader, err := enc.NewReader(bytes.NewReader(tt.data))
			require.NoError(t, err)

			encrypted, err := io.ReadAll(reader)
			require.NoError(t, err)

			// шифротекст состоит из целых блоков размера ключа и не равен открытому тексту
			assert.Zero(t, len(encrypted)%enc.key.Size())
			assert.NotEqual(t, tt.data, encrypted)

			plain, err := dec.decrypt(encrypted)
			require.NoError(t, err)
			assert.Equal(t, tt.data, plain)
		})
	}
}

func TestDecryptBadInput(t *testing.T) {
	_, privPath := writeKeyPair(t)
	dec, err := NewDecryptor(privPath)
	require.NoError(t, err)

	tests := []struct {
		name string
		data []byte
	}{
		{name: "empty body", data: []byte{}},
		{name: "not multiple of block size", data: []byte("plain text body")},
		{name: "correct size but garbage", data: bytes.Repeat([]byte("z"), dec.key.Size())},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := dec.decrypt(tt.data)
			require.Error(t, err)
		})
	}
}

func TestNilEncryptorDecryptor(t *testing.T) {
	data := []byte("plain")

	var enc *Encryptor
	reader, err := enc.NewReader(bytes.NewReader(data))
	require.NoError(t, err)
	got, err := io.ReadAll(reader)
	require.NoError(t, err)
	assert.Equal(t, data, got)

	var dec *Decryptor
	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called = true })
	dec.Handle(next).ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(data)))
	assert.True(t, called)
}

func TestEncryptorNewReaderDisabled(t *testing.T) {
	enc, err := NewEncryptor("")
	require.NoError(t, err)

	data := []byte("some plain data")
	reader, err := enc.NewReader(bytes.NewReader(data))
	require.NoError(t, err)

	got, err := io.ReadAll(reader)
	require.NoError(t, err)
	assert.Equal(t, data, got)
}

func TestDecryptorHandle(t *testing.T) {
	pubPath, privPath := writeKeyPair(t)

	payload := bytes.Repeat([]byte("payload-"), 700) // заведомо больше одного блока

	t.Run("disabled passthrough", func(t *testing.T) {
		dec, err := NewDecryptor("")
		require.NoError(t, err)

		called := false
		var got []byte
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			called = true
			got, _ = io.ReadAll(r.Body)
			w.WriteHeader(http.StatusOK)
		})

		req := httptest.NewRequest(http.MethodPost, "/updates/", bytes.NewReader(payload))
		rec := httptest.NewRecorder()
		dec.Handle(next).ServeHTTP(rec, req)

		assert.True(t, called)
		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Equal(t, payload, got)
	})

	t.Run("strict mode rejects plain body", func(t *testing.T) {
		dec, err := NewDecryptor(privPath)
		require.NoError(t, err)

		called := false
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			called = true
		})

		req := httptest.NewRequest(http.MethodPost, "/updates/", bytes.NewReader(payload))
		rec := httptest.NewRecorder()
		dec.Handle(next).ServeHTTP(rec, req)

		assert.False(t, called, "next must not be called on decryption failure")
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("strict mode rejects corrupted body", func(t *testing.T) {
		enc, err := NewEncryptor(pubPath)
		require.NoError(t, err)
		dec, err := NewDecryptor(privPath)
		require.NoError(t, err)

		reader, err := enc.NewReader(bytes.NewReader(payload))
		require.NoError(t, err)
		encrypted, err := io.ReadAll(reader)
		require.NoError(t, err)

		// портим один байт внутри первого блока
		corrupted := bytes.Clone(encrypted)
		corrupted[10] ^= 0xFF

		called := false
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			called = true
		})

		req := httptest.NewRequest(http.MethodPost, "/updates/", bytes.NewReader(corrupted))
		rec := httptest.NewRecorder()
		dec.Handle(next).ServeHTTP(rec, req)

		assert.False(t, called)
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("end to end encrypted request", func(t *testing.T) {
		enc, err := NewEncryptor(pubPath)
		require.NoError(t, err)
		dec, err := NewDecryptor(privPath)
		require.NoError(t, err)

		reader, err := enc.NewReader(bytes.NewReader(payload))
		require.NoError(t, err)

		called := false
		var got []byte
		var contentLength int64
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			called = true
			contentLength = r.ContentLength
			got, _ = io.ReadAll(r.Body)
			w.WriteHeader(http.StatusOK)
		})

		req := httptest.NewRequest(http.MethodPost, "/updates/", reader)
		rec := httptest.NewRecorder()
		dec.Handle(next).ServeHTTP(rec, req)

		require.True(t, called)
		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Equal(t, payload, got)
		assert.Equal(t, int64(len(payload)), contentLength)
	})
}
