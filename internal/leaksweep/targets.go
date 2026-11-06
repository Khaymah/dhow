package leaksweep

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/Khaymah/dhow/internal/jobspec"
)

const (
	PendingHostKeyRow  = "pg_dump/DeployTargetHosts.sql"
	pemMarker          = "PRIVATE KEY-----"
	pemFence           = "-----"
	minAuthorizedParts = 2
)

func LooksLikePrivateKey(value []byte) bool { return bytes.Contains(value, []byte(pemMarker)) }

func PrivateKey(name string, pemText []byte) ([]Needle, error) {
	if out, err := PEMKey(name, pemText); err == nil {
		return append(out, bodyNeedle(name, pemText)...), nil
	}
	whole, err := Encoded(name+" (PEM)", pemText)
	if err != nil {
		return nil, err
	}
	return append([]Needle{whole}, bodyNeedle(name, pemText)...), nil
}

func bodyNeedle(name string, pemText []byte) []Needle {
	var body strings.Builder
	for _, line := range strings.Split(string(pemText), "\n") {
		line = strings.TrimSpace(line)
		if line != "" && !strings.HasPrefix(line, pemFence) {
			body.WriteString(line)
		}
	}
	n, err := Encoded(name+" (PEM body on one line)", []byte(body.String()))
	if err != nil {
		return nil
	}
	return []Needle{n}
}

func PendingHostKey(host string, port int, authorizedKey string) ([]Needle, error) {
	fields := strings.Fields(authorizedKey)
	if len(fields) < minAuthorizedParts {
		return nil, fmt.Errorf("%w: the pending host key of %s", ErrShortValue, host)
	}
	label := fmt.Sprintf("pending host key of %s:%d", host, port)
	blob, err := Encoded(label, []byte(fields[1]))
	if err != nil {
		return nil, err
	}
	line, err := Encoded(label+" as a known_hosts line", []byte(jobspec.KnownHostsLine(host, port, fields[0]+" "+fields[1])))
	if err != nil {
		return nil, err
	}
	blob.Allowed, line.Allowed = []string{PendingHostKeyRow}, []string{PendingHostKeyRow}
	return []Needle{blob, line}, nil
}
