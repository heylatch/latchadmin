// Package secret encrypts the values LatchAdmin keeps on disk that must not be readable from a copy of its data
// folder: the Latch account key, environment values marked secret (like LATCH_LICENSE) and 2FA secrets.
//
// Values are sealed with AES-256-GCM under a random key stored next to the data. On Windows that key file is itself
// protected with DPAPI, so it only opens for the Windows account that created it. On Linux the key file is 0600.
package secret

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

var key []byte

const prefix = "sealed:v1:"

// Init loads or creates the key in dataDir.
func Init(dataDir string) error {
	p := filepath.Join(dataDir, "secret.key")
	raw, err := os.ReadFile(p)
	if errors.Is(err, os.ErrNotExist) {
		k := make([]byte, 32)
		if _, err := rand.Read(k); err != nil {
			return err
		}
		stored, err := protect(k)
		if err != nil {
			return err
		}
		if err := os.WriteFile(p, stored, 0o600); err != nil {
			return err
		}
		key = k
		return nil
	}
	if err != nil {
		return err
	}
	k, err := unprotect(raw)
	if err != nil {
		return errors.New("secret.key can't be opened by this user (was the data folder copied from another account?): " + err.Error())
	}
	if len(k) != 32 {
		return errors.New("secret.key is damaged")
	}
	key = k
	return nil
}

// Seal encrypts s. An empty string stays empty.
func Seal(s string) (string, error) {
	if s == "" {
		return "", nil
	}
	gcm, err := aead()
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	out := gcm.Seal(nonce, nonce, []byte(s), []byte(prefix))
	return prefix + base64.RawStdEncoding.EncodeToString(out), nil
}

// Open decrypts a value made by Seal.
func Open(s string) (string, error) {
	if s == "" {
		return "", nil
	}
	if !strings.HasPrefix(s, prefix) {
		return "", errors.New("value is not sealed")
	}
	raw, err := base64.RawStdEncoding.DecodeString(strings.TrimPrefix(s, prefix))
	if err != nil {
		return "", err
	}
	gcm, err := aead()
	if err != nil {
		return "", err
	}
	if len(raw) < gcm.NonceSize() {
		return "", errors.New("sealed value is too short")
	}
	plain, err := gcm.Open(nil, raw[:gcm.NonceSize()], raw[gcm.NonceSize():], []byte(prefix))
	if err != nil {
		return "", errors.New("sealed value can't be opened")
	}
	return string(plain), nil
}

// MustOpen returns "" when a value can't be opened.
func MustOpen(s string) string {
	v, _ := Open(s)
	return v
}

func aead() (cipher.AEAD, error) {
	if key == nil {
		return nil, errors.New("secret store not initialised")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}
