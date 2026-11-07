package notifications

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"time"

	config "github.com/Khaymah/dhow/apps/server/internal/Config"
)

const (
	HeaderEvent     = "Dhow-Event"
	HeaderDelivery  = "Dhow-Delivery"
	HeaderTimestamp = "Dhow-Timestamp"
	HeaderSignature = "Dhow-Signature"
	HeaderIdempKey  = "Idempotency-Key"
	SignatureScheme = "v1="
	SecretPrefix    = "dhow_whsec_" //nolint:gosec // A public prefix, not a credential.
	ReplayWindow    = 5 * time.Minute
)

func NewSecret() (string, error) {
	raw := make([]byte, config.NotifySecretBytes)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("read random bytes for a webhook secret: %w", err)
	}
	return SecretPrefix + base64.RawURLEncoding.EncodeToString(raw), nil
}

func ValidSecret(secret string) bool {
	body, ok := strings.CutPrefix(secret, SecretPrefix)
	if !ok {
		return false
	}
	raw, err := base64.RawURLEncoding.DecodeString(body)
	return err == nil && len(raw) == config.NotifySecretBytes
}

func Sign(secret string, at time.Time, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(strconv.FormatInt(at.Unix(), 10)))
	mac.Write([]byte("."))
	mac.Write(body)
	return SignatureScheme + hex.EncodeToString(mac.Sum(nil))
}

func Verify(secret, header, timestamp string, body []byte, now time.Time) bool {
	seconds, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil {
		return false
	}
	at := time.Unix(seconds, 0)
	if d := now.Sub(at); d > ReplayWindow || d < -ReplayWindow {
		return false
	}
	want := Sign(secret, at, body)
	return hmac.Equal([]byte(want), []byte(header))
}
