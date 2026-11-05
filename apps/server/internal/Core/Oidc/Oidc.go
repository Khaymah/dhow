package oidcauth

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"

	gooidc "github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"

	config "github.com/Khaymah/dhow/apps/server/internal/Config"
)

var (
	errNoIDToken     = errors.New("token response has no id_token")
	errNonceMismatch = errors.New("id_token nonce does not match the flow")
	errAzpMismatch   = errors.New("id_token was issued to another client")
	errBadSubject    = errors.New("id_token subject is empty or too long")
)

type oidcProvider struct {
	cfg       config.OIDCProvider
	client    *http.Client
	now       func() time.Time
	allowHTTP bool

	mu         sync.Mutex
	discovered *gooidc.Provider
	failedAt   time.Time
	failure    error
	inflight   chan struct{}
}

func newOIDC(c config.OIDCProvider, client *http.Client, now func() time.Time, allowHTTP bool) *oidcProvider {
	return &oidcProvider{cfg: c, client: client, now: now, allowHTTP: allowHTTP}
}

func (p *oidcProvider) Info() Info {
	return Info{ID: p.cfg.ID, Name: p.cfg.Name, Kind: p.cfg.Kind}
}

func (p *oidcProvider) Issuer() string { return p.cfg.Issuer }

func (p *oidcProvider) AllowedDomains() []string { return p.cfg.AllowedDomains }

func (p *oidcProvider) oauth(endpoint oauth2.Endpoint, redirect string) *oauth2.Config {
	return &oauth2.Config{
		ClientID: p.cfg.ClientID, ClientSecret: p.cfg.ClientSecret, Endpoint: endpoint, RedirectURL: redirect,
		Scopes: []string{gooidc.ScopeOpenID, "email", "profile"},
	}
}

func (p *oidcProvider) AuthorizeURL(ctx context.Context, f Flow) (string, error) {
	discovered, err := p.provider(ctx)
	if err != nil {
		return "", err
	}
	return p.oauth(discovered.Endpoint(), f.RedirectURL).AuthCodeURL(f.State, oauth2.S256ChallengeOption(f.Verifier), gooidc.Nonce(f.Nonce)), nil
}

type oidcClaims struct {
	Email         string
	EmailVerified bool
	Name          string
	Azp           string
	Login         string
}

func decodeClaims(idt *gooidc.IDToken) (oidcClaims, error) {
	var raw map[string]json.RawMessage
	if err := idt.Claims(&raw); err != nil {
		return oidcClaims{}, err
	}
	text := func(key string) string {
		var v string
		if json.Unmarshal(raw[key], &v) != nil {
			return ""
		}
		return v
	}
	return oidcClaims{Email: text("email"), EmailVerified: truthy(raw["email_verified"]), Name: text("name"), Azp: text("azp"), Login: text("preferred_username")}, nil
}

func (p *oidcProvider) Exchange(ctx context.Context, code string, f Flow) (Identity, error) {
	discovered, err := p.provider(ctx)
	if err != nil {
		return Identity{}, err
	}
	ctx = context.WithValue(ctx, oauth2.HTTPClient, p.client)
	token, err := p.oauth(discovered.Endpoint(), f.RedirectURL).Exchange(ctx, code, oauth2.VerifierOption(f.Verifier))
	if err != nil {
		return Identity{}, rejected("code exchange", err)
	}
	raw, ok := token.Extra("id_token").(string)
	if !ok || raw == "" {
		return Identity{}, rejected("id_token", errNoIDToken)
	}
	verifier := discovered.VerifierContext(gooidc.ClientContext(context.WithoutCancel(ctx), p.client), &gooidc.Config{ClientID: p.cfg.ClientID, Now: p.now})
	idt, err := verifier.Verify(ctx, raw)
	if err != nil {
		return Identity{}, rejected("id_token", err)
	}
	if subtle.ConstantTimeCompare([]byte(idt.Nonce), []byte(f.Nonce)) != 1 {
		return Identity{}, rejected("id_token", errNonceMismatch)
	}
	claims, err := decodeClaims(idt)
	if err != nil {
		return Identity{}, rejected("id_token claims", err)
	}
	if (len(idt.Audience) > 1 || claims.Azp != "") && claims.Azp != p.cfg.ClientID {
		return Identity{}, rejected("id_token", errAzpMismatch)
	}
	if idt.Subject == "" || len(idt.Subject) > MaxSubjectLength {
		return Identity{}, rejected("id_token", errBadSubject)
	}
	out := Identity{Issuer: p.cfg.Issuer, Subject: idt.Subject, Name: claims.Name, Login: CleanLogin(claims.Login)}
	if email := strings.TrimSpace(claims.Email); email != "" {
		out.Emails = []Email{{Address: email, Verified: claims.EmailVerified, Primary: true}}
	}
	return out, nil
}

func truthy(raw json.RawMessage) bool {
	var b bool
	if json.Unmarshal(raw, &b) == nil {
		return b
	}
	var s string
	return json.Unmarshal(raw, &s) == nil && s == "true"
}
