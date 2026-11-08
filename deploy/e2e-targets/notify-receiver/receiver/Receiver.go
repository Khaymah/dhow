package receiver

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
)

const (
	ServiceSlack    = "slack"
	ServiceDiscord  = "discord"
	ServiceTelegram = "telegram"

	pathSlack    = "/services/"
	pathDiscord  = "/api/webhooks/"
	pathTelegram = "/bot"
	pathMessages = "/_messages"
	pathScript   = "/_script"
	pathHealth   = "/_health"

	bodyMax = 1 << 20
)

type Message struct {
	Service       string
	Text          string
	ChatID        string
	ParseMode     string
	MentionsParse []string
	HasMentions   bool
}

type Receiver struct {
	mu       sync.Mutex
	messages []Message
	script   map[string][]int
	attempts map[string]int
}

func New() *Receiver {
	return &Receiver{script: map[string][]int{}, attempts: map[string]int{}}
}

func (r *Receiver) Messages() []Message {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]Message(nil), r.messages...)
}

func (r *Receiver) Attempts(service string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.attempts[service]
}

func (r *Receiver) Script(service string, statuses ...int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.script[service] = append([]int(nil), statuses...)
}

func (r *Receiver) Reset() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.messages, r.script, r.attempts = nil, map[string][]int{}, map[string]int{}
}

func (r *Receiver) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc(pathSlack, r.serve(ServiceSlack, slackAPI{}))
	mux.HandleFunc(pathDiscord, r.serve(ServiceDiscord, discordAPI{}))
	telegram := r.serve(ServiceTelegram, telegramAPI{})
	mux.HandleFunc("/", func(w http.ResponseWriter, req *http.Request) {
		if strings.HasPrefix(req.URL.Path, pathTelegram) {
			telegram(w, req)
			return
		}
		http.NotFound(w, req)
	})
	mux.HandleFunc(pathMessages, r.control)
	mux.HandleFunc(pathScript, r.setScript)
	mux.HandleFunc(pathHealth, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	return mux
}

type api interface {
	Accept(req *http.Request, body []byte) (Message, reply)
}

type reply struct {
	Status int
	Body   string
	Type   string
}

func (r *Receiver) serve(service string, a api) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		body, err := io.ReadAll(http.MaxBytesReader(w, req.Body, bodyMax))
		if err != nil {
			http.Error(w, "body too large", http.StatusRequestEntityTooLarge)
			return
		}
		r.mu.Lock()
		r.attempts[service]++
		scripted, ok := r.popScript(service)
		r.mu.Unlock()
		if ok {
			write(w, failure(service, scripted))
			return
		}
		msg, res := a.Accept(req, body)
		if res.Status/100 == 2 {
			msg.Service = service
			r.mu.Lock()
			r.messages = append(r.messages, msg)
			r.mu.Unlock()
		}
		write(w, res)
	}
}

func (r *Receiver) popScript(service string) (int, bool) {
	queue := r.script[service]
	if len(queue) == 0 {
		return 0, false
	}
	r.script[service] = queue[1:]
	return queue[0], true
}

func write(w http.ResponseWriter, res reply) {
	if res.Type != "" {
		w.Header().Set("Content-Type", res.Type)
	}
	w.WriteHeader(res.Status)
	_, _ = io.WriteString(w, res.Body)
}

func failure(service string, status int) reply {
	switch service {
	case ServiceSlack:
		return reply{Status: status, Body: "server_error", Type: "text/plain"}
	case ServiceDiscord:
		return reply{Status: status, Body: `{"message":"scripted failure","code":0}`, Type: "application/json"}
	default:
		return reply{
			Status: status, Type: "application/json",
			Body: `{"ok":false,"error_code":` + strconv.Itoa(status) + `,"description":"scripted failure"}`,
		}
	}
}

type view struct {
	Service       string
	Text          string
	ChatId        string
	ParseMode     string
	MentionsParse []string
	HasMentions   bool
}

func (r *Receiver) control(w http.ResponseWriter, req *http.Request) {
	switch req.Method {
	case http.MethodGet:
		out := make([]view, 0)
		for _, m := range r.Messages() {
			out = append(out, view{
				Service: m.Service, Text: m.Text, ChatId: m.ChatID, ParseMode: m.ParseMode, MentionsParse: m.MentionsParse, HasMentions: m.HasMentions,
			})
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(out)
	case http.MethodDelete:
		r.Reset()
		w.WriteHeader(http.StatusNoContent)
	default:
		w.Header().Set("Allow", "GET, DELETE")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (r *Receiver) setScript(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	service := req.URL.Query().Get("service")
	if service != ServiceSlack && service != ServiceDiscord && service != ServiceTelegram {
		http.Error(w, "unknown service", http.StatusBadRequest)
		return
	}
	var statuses []int
	for _, part := range strings.Split(req.URL.Query().Get("status"), ",") {
		if part == "" {
			continue
		}
		n, err := strconv.Atoi(part)
		if err != nil || n < 100 || n > 599 {
			http.Error(w, "bad status", http.StatusBadRequest)
			return
		}
		statuses = append(statuses, n)
	}
	r.Script(service, statuses...)
	w.WriteHeader(http.StatusNoContent)
}
