package shanvas

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base32"
	"errors"
	"fmt"
	"io"
)

type TokenHandler struct {
	secret  []byte
	encoder *base32.Encoding
}

func NewTokenHandler(secret string) *TokenHandler {
	encoder := base32.StdEncoding.WithPadding(base32.NoPadding)

	return &TokenHandler{
		secret:  []byte(secret),
		encoder: encoder,
	}
}

// Sourced from:
//
//	https://github.com/gorilla/securecookie/blob/main/securecookie.go#L515
func generateRandomKey(length int) []byte {
	key := make([]byte, length)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return nil
	}
	return key
}

func (t *TokenHandler) encode(data []byte) []byte {
	result := make([]byte, t.encoder.EncodedLen(len(data)))
	t.encoder.Encode(result, data)
	return result
}

func (t *TokenHandler) decode(data []byte) ([]byte, error) {
	result := make([]byte, t.encoder.DecodedLen(len(data)))

	acc, err := t.encoder.Decode(result, data)
	if err != nil {
		return nil, err
	}

	return result[:acc], nil
}

func (t *TokenHandler) Generate() string {
	rand := generateRandomKey(32)
	id := t.encode(rand)

	hash := sha256.New()

	hash.Write(t.secret)
	hash.Write(id)
	mac := t.encode(hash.Sum(nil))

	token := fmt.Appendf(nil, "%s.%s", id, mac)

	return string(t.encode(token))
}

var ErrInvalidToken = errors.New("token is invalid")

func (t *TokenHandler) Verify(token string) (string, error) {
	data, err := t.decode([]byte(token))
	if err != nil {
		return "", err
	}

	parts := bytes.Split(data, []byte("."))
	if len(parts) != 2 {
		return "", ErrInvalidToken
	}

	uid := parts[0]
	mac, err := t.decode(parts[1])
	if err != nil {
		return "", err
	}

	hash := sha256.New()
	hash.Write(t.secret)
	hash.Write(uid)

	macExpected := hash.Sum(nil)

	if !bytes.Equal(mac, macExpected) {
		return "", ErrInvalidToken
	}

	return string(uid), nil
}
