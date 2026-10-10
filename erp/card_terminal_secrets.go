package erp

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"io"
	"os"
	"strings"

	"github.com/sirinibin/startpos/backend/env"
)

// Card terminal credentials (API keys, secrets, passwords) are sealed with
// AES-256-GCM before they are written to MongoDB and are never sent back to a
// browser: reads only say whether a secret is set and show its last 4
// characters. The key is CARD_TERMINAL_SECRET_KEY when set, else derived from
// the access-token secret, so a database copy alone never reveals them.

const sealedPrefix = "enc1:"

var secretKeyFn = func() []byte {
	k := strings.TrimSpace(os.Getenv("CARD_TERMINAL_SECRET_KEY"))
	if k == "" {
		k = "starterp-card-terminals|" + env.GetJWTAccessSecret()
	}
	sum := sha256.Sum256([]byte(k))
	return sum[:]
}

func sealSecret(plain string) (string, error) {
	if plain == "" {
		return "", nil
	}
	block, err := aes.NewCipher(secretKeyFn())
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	out := gcm.Seal(nonce, nonce, []byte(plain), nil)
	return sealedPrefix + base64.RawStdEncoding.EncodeToString(out), nil
}

func openSecret(sealed string) (string, error) {
	if sealed == "" {
		return "", nil
	}
	if !strings.HasPrefix(sealed, sealedPrefix) {
		return "", errors.New("not a sealed secret")
	}
	raw, err := base64.RawStdEncoding.DecodeString(strings.TrimPrefix(sealed, sealedPrefix))
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(secretKeyFn())
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(raw) < gcm.NonceSize() {
		return "", errors.New("sealed secret too short")
	}
	plain, err := gcm.Open(nil, raw[:gcm.NonceSize()], raw[gcm.NonceSize():], nil)
	if err != nil {
		return "", err
	}
	return string(plain), nil
}

// secretHint is what a read shows for a secret: "" when unset, else "••••1234".
func secretHint(plain string) string {
	if plain == "" {
		return ""
	}
	r := []rune(plain)
	if len(r) <= 4 {
		return "••••"
	}
	return "••••" + string(r[len(r)-4:])
}
