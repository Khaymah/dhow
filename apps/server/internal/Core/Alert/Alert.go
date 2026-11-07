package alert

import (
	"net/netip"
	"strings"
	"time"

	i18n "github.com/Khaymah/dhow/apps/server/internal/Core/I18n"
	mailer "github.com/Khaymah/dhow/apps/server/internal/Core/Mail"
)

const (
	KeySubject   = "mail.alert.subject"
	KeyBody      = "mail.alert.body"
	KeyResetBody = "mail.alert.reset.body"
	KeyUnknown   = "mail.alert.unknown"
	KeyChange    = "mail.alert.change."

	ChangeMfaReset = "mfa_reset"

	mailDateLayout = "2006-01-02 15:04"
	ipv4Prefix     = 24
	ipv6Prefix     = 48
)

type Details struct {
	To        string
	Name      string
	Change    string
	At        time.Time
	IP        string
	UserAgent string
	Link      string
}

func Message(locale i18n.Locale, d Details) mailer.Message {
	unknown := i18n.Translate(locale, KeyUnknown)
	change := i18n.Translate(locale, KeyChange+d.Change)
	r := strings.NewReplacer(
		"{name}", d.Name, "{change}", change, "{time}", d.At.UTC().Format(mailDateLayout),
		"{ip}", orUnknown(ApproximateIP(d.IP), unknown), "{browser}", orUnknown(Browser(d.UserAgent), unknown), "{link}", d.Link,
	)
	subject := strings.NewReplacer("{change}", change).Replace(i18n.Translate(locale, KeySubject))
	body := KeyBody
	if d.Change == ChangeMfaReset {
		body = KeyResetBody
	}
	return mailer.Message{To: d.To, Subject: oneLine(subject), Body: r.Replace(i18n.Translate(locale, body))}
}

func orUnknown(v, unknown string) string {
	if v == "" {
		return unknown
	}
	return v
}

func oneLine(v string) string { return strings.NewReplacer("\r", " ", "\n", " ").Replace(v) }

func ApproximateIP(raw string) string {
	addr, err := netip.ParseAddr(raw)
	if err != nil {
		return ""
	}
	addr = addr.Unmap()
	bits := ipv6Prefix
	if addr.Is4() {
		bits = ipv4Prefix
	}
	prefix, err := addr.Prefix(bits)
	if err != nil {
		return ""
	}
	return prefix.String()
}

type marker struct{ token, label string }

func browsers() []marker {
	return []marker{
		{"Edg/", "Edge"},
		{"OPR/", "Opera"},
		{"Firefox/", "Firefox"},
		{"FxiOS/", "Firefox"},
		{"CriOS/", "Chrome"},
		{"Chrome/", "Chrome"},
		{"Safari/", "Safari"},
		{"curl/", "curl"},
		{"dhow/", "Dhow CLI"},
	}
}

func systems() []marker {
	return []marker{
		{"Windows", "Windows"},
		{"Android", "Android"},
		{"iPhone", "iOS"},
		{"iPad", "iPadOS"},
		{"CrOS", "ChromeOS"},
		{"Mac OS X", "macOS"},
		{"Macintosh", "macOS"},
		{"Linux", "Linux"},
	}
}

func first(ua string, ms []marker) string {
	for _, m := range ms {
		if strings.Contains(ua, m.token) {
			return m.label
		}
	}
	return ""
}

func Browser(ua string) string {
	b, sys := first(ua, browsers()), first(ua, systems())
	switch {
	case b != "" && sys != "":
		return b + " · " + sys
	case b != "":
		return b
	default:
		return sys
	}
}
