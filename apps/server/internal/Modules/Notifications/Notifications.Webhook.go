package notifications

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"strconv"

	config "github.com/Khaymah/dhow/apps/server/internal/Config"
	outbox "github.com/Khaymah/dhow/apps/server/internal/Core/Outbox"
	"github.com/Khaymah/dhow/internal/netguard"
)

const (
	CodeBlocked        = "blocked"
	CodeRedirect       = "redirect"
	CodeTimeout        = "timeout"
	CodeConnect        = "connect"
	CodeHTTPStatus     = "http_status"
	CodeConfig         = "config"
	CodeChannelGone    = "channel_removed"
	CodeChannelOff     = "channel_disabled"
	CodeMailOff        = "mail_disabled"
	CodeMailSend       = "mail_send"
	CodeNoRecipient    = "no_recipient"
	userAgent          = "Dhow-Notifications/1"
	contentTypeJSON    = "application/json"
	statusRequestTime  = http.StatusRequestTimeout
	statusTooEarly     = 425
	statusTooManyReqs  = http.StatusTooManyRequests
	statusClassSuccess = 2
)

type webhookDeliverer struct{ s *Service }

func (s *Service) httpClient() *http.Client {
	s.clientOnce.Do(func() { s.client = s.newHTTPClient() })
	return s.client
}

func (s *Service) newHTTPClient() *http.Client {
	d := s.dialer()
	if s.dial != nil {
		d.Dial = s.dial
	} else {
		connect := &net.Dialer{Timeout: config.NotifyConnectTimeout, Control: d.Control}
		d.Dial = connect.DialContext
	}
	return d.HTTPClient(netguard.ClientOptions{TLS: s.tls})
}

func transportCode(err error) (string, bool) {
	switch {
	case errors.Is(err, netguard.ErrBlocked), errors.Is(err, netguard.ErrInsecure), errors.Is(err, netguard.ErrNoAddress):
		return CodeBlocked, true
	case errors.Is(err, netguard.ErrRedirect):
		return CodeRedirect, true
	case errors.Is(err, context.DeadlineExceeded):
		return CodeTimeout, false
	default:
		var timeout interface{ Timeout() bool }
		if errors.As(err, &timeout) && timeout.Timeout() {
			return CodeTimeout, false
		}
		return CodeConnect, false
	}
}

func statusResult(status int) outbox.Result {
	if status/100 == statusClassSuccess {
		return outbox.Result{Status: status}
	}
	permanent := status >= 400 && status < 500 && status != statusRequestTime && status != statusTooEarly && status != statusTooManyReqs
	return outbox.Result{Code: CodeHTTPStatus, Status: status, Permanent: permanent}
}

func (d webhookDeliverer) Deliver(ctx context.Context, item outbox.Item) outbox.Result {
	s := d.s
	ch, res, ok := s.liveChannel(ctx, item)
	if !ok {
		return res
	}
	cfg, err := s.openConfig(ctx, ch)
	if err != nil {
		s.log.WarnContext(ctx, "notification channel settings cannot be read", "channel_id", ch.Id.String())
		return outbox.Result{Code: CodeConfig, Permanent: true}
	}
	req, err := s.signedRequest(ctx, cfg, item)
	if err != nil {
		return outbox.Result{Code: CodeBlocked, Permanent: true}
	}
	resp, err := s.httpClient().Do(req)
	if err != nil {
		code, permanent := transportCode(err)
		return outbox.Result{Code: code, Permanent: permanent}
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, config.NotifyResponseReadMax))
	return statusResult(resp.StatusCode)
}

func (s *Service) signedRequest(ctx context.Context, cfg channelConfig, item outbox.Item) (*http.Request, error) {
	target, err := ParseTarget(cfg.URL, false)
	if err != nil {
		return nil, err
	}
	at := s.now()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target.URL.String(), bytes.NewReader(item.Payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", contentTypeJSON)
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set(HeaderEvent, item.EventName)
	req.Header.Set(HeaderDelivery, item.ID.String())
	req.Header.Set(HeaderIdempKey, item.EventID.String())
	req.Header.Set(HeaderTimestamp, strconv.FormatInt(at.Unix(), 10))
	req.Header.Set(HeaderSignature, Sign(cfg.Secret, at, item.Payload))
	return req, nil
}
