package unit_test

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	notifications "github.com/Khaymah/dhow/apps/server/internal/Modules/Notifications"
)

const (
	vectorSecret    = notifications.SecretPrefix + "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
	vectorTimestamp = int64(1794045600)
	vectorBody      = `{"Event":"deployment.finished","Status":"failed"}`
)

func TestSignMatchesAnIndependentHMACOfTimestampDotBody(t *testing.T) {
	mac := hmac.New(sha256.New, []byte(vectorSecret))
	mac.Write([]byte(strconv.FormatInt(vectorTimestamp, 10) + "." + vectorBody))
	want := "v1=" + hex.EncodeToString(mac.Sum(nil))
	got := notifications.Sign(vectorSecret, time.Unix(vectorTimestamp, 0), []byte(vectorBody))
	assert.Equal(t, want, got)
}

func TestSignHasAFixedTestVector(t *testing.T) {
	got := notifications.Sign("dhow_whsec_test", time.Unix(1700000000, 0), []byte(`{"A":1}`))
	assert.Equal(t, "v1=63bf61c029c49a6b4f4776e06bfa67277e5f4a9f9517208b3c1ea9e845e45adb", got, "the vector printed in the document")
}

func TestVerifyAcceptsTheSignatureInsideTheWindowOnly(t *testing.T) {
	at := time.Unix(vectorTimestamp, 0)
	sig := notifications.Sign(vectorSecret, at, []byte(vectorBody))
	ts := strconv.FormatInt(vectorTimestamp, 10)
	assert.True(t, notifications.Verify(vectorSecret, sig, ts, []byte(vectorBody), at))
	assert.True(t, notifications.Verify(vectorSecret, sig, ts, []byte(vectorBody), at.Add(notifications.ReplayWindow)))
	assert.False(t, notifications.Verify(vectorSecret, sig, ts, []byte(vectorBody), at.Add(notifications.ReplayWindow+time.Second)), "too old")
	assert.False(t, notifications.Verify(vectorSecret, sig, ts, []byte(vectorBody), at.Add(-notifications.ReplayWindow-time.Second)), "from the future")
	assert.False(t, notifications.Verify(vectorSecret, sig, ts, []byte(vectorBody+" "), at), "a changed body")
	assert.False(t, notifications.Verify("dhow_whsec_other", sig, ts, []byte(vectorBody), at), "another secret")
	assert.False(t, notifications.Verify(vectorSecret, sig, "x", []byte(vectorBody), at), "a bad timestamp")
	assert.False(t, notifications.Verify(vectorSecret, "v1=00", ts, []byte(vectorBody), at), "a bad signature")
	assert.False(t, notifications.Verify(vectorSecret, sig, strconv.FormatInt(vectorTimestamp+1, 10), []byte(vectorBody), at), "a changed timestamp")
}

func TestNewSecretIsRandomAndWellFormed(t *testing.T) {
	a, err := notifications.NewSecret()
	require.NoError(t, err)
	b, err := notifications.NewSecret()
	require.NoError(t, err)
	assert.NotEqual(t, a, b)
	assert.True(t, notifications.ValidSecret(a))
	assert.False(t, notifications.ValidSecret("whsec_x"))
	assert.False(t, notifications.ValidSecret(""))
	assert.Len(t, a, len(notifications.SecretPrefix)+43)
}
