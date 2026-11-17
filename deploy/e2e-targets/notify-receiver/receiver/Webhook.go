package receiver

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"time"
)

const (
	HostWebhook = "webhook.notify.test"

	headerEvent     = "Dhow-Event"
	headerDelivery  = "Dhow-Delivery"
	headerTimestamp = "Dhow-Timestamp"
	headerSignature = "Dhow-Signature"
	headerIdemp     = "Idempotency-Key"
	signatureScheme = "v1="
	replayWindow    = 5 * time.Minute
	secretMax       = 256
	holdMax         = time.Minute
)

func knownService(service string) bool {
	switch service {
	case ServiceSlack, ServiceDiscord, ServiceTelegram, ServiceWebhook:
		return true
	default:
		return false
	}
}

type webhookAPI struct{ secret func() string }

func (a webhookAPI) Accept(req *http.Request, body []byte) (Message, reply) {
	msg := Message{
		Text: string(body), Event: req.Header.Get(headerEvent), Delivery: req.Header.Get(headerDelivery),
		Idempotency: req.Header.Get(headerIdemp),
	}
	msg.Signed = validSignature(a.secret(), req.Header.Get(headerTimestamp), req.Header.Get(headerSignature), body, time.Now())
	return msg, reply{Status: http.StatusOK, Body: "ok", Type: "text/plain"}
}

func validSignature(secret, timestamp, header string, body []byte, now time.Time) bool {
	seconds, err := strconv.ParseInt(timestamp, 10, 64)
	if secret == "" || err != nil {
		return false
	}
	if d := now.Sub(time.Unix(seconds, 0)); d > replayWindow || d < -replayWindow {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(timestamp + "."))
	mac.Write(body)
	return hmac.Equal([]byte(signatureScheme+hex.EncodeToString(mac.Sum(nil))), []byte(header))
}

func (r *Receiver) setSecret(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	raw, err := io.ReadAll(http.MaxBytesReader(w, req.Body, secretMax))
	if err != nil {
		http.Error(w, "body too large", http.StatusRequestEntityTooLarge)
		return
	}
	r.Secret(string(raw))
	w.WriteHeader(http.StatusNoContent)
}

func (r *Receiver) setHold(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	service := req.URL.Query().Get("service")
	ms, err := strconv.Atoi(req.URL.Query().Get("ms"))
	hold := time.Duration(ms) * time.Millisecond
	if !knownService(service) || err != nil || hold < 0 || hold > holdMax {
		http.Error(w, "bad hold", http.StatusBadRequest)
		return
	}
	r.Hold(service, hold)
	w.WriteHeader(http.StatusNoContent)
}

type statsView struct {
	Attempts map[string]int
	Accepted map[string]int
}

func (r *Receiver) stats(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	out := statsView{Attempts: map[string]int{}, Accepted: map[string]int{}}
	for _, service := range []string{ServiceSlack, ServiceDiscord, ServiceTelegram, ServiceWebhook} {
		out.Attempts[service] = r.Attempts(service)
	}
	for _, m := range r.Messages() {
		out.Accepted[m.Service]++
	}
	w.Header().Set("Content-Type", jsonType)
	_ = json.NewEncoder(w).Encode(out)
}
