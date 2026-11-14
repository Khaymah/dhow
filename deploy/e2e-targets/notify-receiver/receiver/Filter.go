package receiver

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

const pathCount = "/_count"

type filter struct {
	service   string
	contains  []string
	absent    []string
	key       string
	signed    bool
	noMention bool
}

func filterOf(q url.Values) (filter, bool) {
	f := filter{
		service: q.Get("service"), contains: q["contains"], absent: q["absent"], key: q.Get("key"),
		signed: q.Get("signed") == "1", noMention: q.Get("mentions") == "none",
	}
	return f, f.service == "" || knownService(f.service)
}

func (f filter) matches(m Message) bool {
	if f.service != "" && m.Service != f.service {
		return false
	}
	if f.key != "" && m.Idempotency != f.key {
		return false
	}
	if f.signed && !m.Signed {
		return false
	}
	if f.noMention && (!m.HasMentions || len(m.MentionsParse) != 0) {
		return false
	}
	for _, c := range f.contains {
		if !strings.Contains(m.Text, c) {
			return false
		}
	}
	for _, a := range f.absent {
		if strings.Contains(m.Text, a) {
			return false
		}
	}
	return true
}

func (r *Receiver) matching(f filter) []Message {
	var out []Message
	for _, m := range r.Messages() {
		if f.matches(m) {
			out = append(out, m)
		}
	}
	return out
}

func (r *Receiver) count(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	f, ok := filterOf(req.URL.Query())
	if !ok {
		http.Error(w, "unknown service", http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "text/plain")
	_, _ = w.Write([]byte(strconv.Itoa(len(r.matching(f))) + "\n"))
}
