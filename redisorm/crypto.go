package redisorm

import (
	cryptoRand "crypto/rand"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"strings"
)

const fieldEncPrefix = "encf:v1:gcm:"

func randBytes(n int) ([]byte, error) { b := make([]byte, n); _, err := cryptoRand.Read(b); return b, err }

// HMAC برای ایندکس رمز‌شده (برابری دترمینیستیک). در محیط واقعی بهتره per-tenant salt/pepper داشته باشی.
func macString(kek []byte, s string) string {
	h := hmac.New(sha256.New, kek)
	h.Write([]byte(s))
	return base64.StdEncoding.EncodeToString(h.Sum(nil))
}

func aesGCMEncrypt(key, plain []byte) (string, error) {
	bc, err := aes.NewCipher(key); if err != nil { return "", err }
	gcm, err := cipher.NewGCM(bc); if err != nil { return "", err }
	nonce, err := randBytes(gcm.NonceSize()); if err != nil { return "", err }
	ct := gcm.Seal(nil, nonce, plain, nil)
	out := append(nonce, ct...)
	return fieldEncPrefix + base64.StdEncoding.EncodeToString(out), nil
}

func aesGCMDecrypt(key []byte, enc string) ([]byte, error) {
	if !strings.HasPrefix(enc, fieldEncPrefix) { return nil, errors.New("invalid ciphertext prefix") }
	raw, err := base64.StdEncoding.DecodeString(enc[len(fieldEncPrefix):]); if err != nil { return nil, err }
	bc, err := aes.NewCipher(key); if err != nil { return nil, err }
	gcm, err := cipher.NewGCM(bc); if err != nil { return nil, err }
	ns := gcm.NonceSize()
	if len(raw) < ns { return nil, errors.New("ciphertext too short") }
	nonce, ct := raw[:ns], raw[ns:]
	return gcm.Open(nil, nonce, ct, nil)
}

func wrapDEK(kek, dek []byte) string                       { s, _ := aesGCMEncrypt(kek, dek); return s }
func unwrapDEK(kek []byte, wrapped string) ([]byte, error) { return aesGCMDecrypt(kek, wrapped) }
