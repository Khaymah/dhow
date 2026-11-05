package oidcauth

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"golang.org/x/oauth2"

	config "github.com/Khaymah/dhow/apps/server/internal/Config"
)

const (
	gitHubAPI        = "https://api.github.com"
	gitHubEnterprise = "/api/v3"
	gitHubMediaType  = "application/vnd.github+json"
	gitHubAPIVersion = "2022-11-28"
	gitHubMaxEmails  = 100
)

var errBadGitHubUser = errors.New("github user has no numeric id")

type gitHubProvider struct {
	cfg    config.OIDCProvider
	client *http.Client
	api    string
}

func newGitHub(c config.OIDCProvider, client *http.Client) *gitHubProvider {
	api := c.Issuer + gitHubEnterprise
	if c.Issuer == config.OIDCDefaultGitHubIssuer {
		api = gitHubAPI
	}
	return &gitHubProvider{cfg: c, client: client, api: api}
}

func (p *gitHubProvider) Info() Info {
	return Info{ID: p.cfg.ID, Name: p.cfg.Name, Kind: p.cfg.Kind}
}

func (p *gitHubProvider) Issuer() string { return p.cfg.Issuer }

func (p *gitHubProvider) AllowedDomains() []string { return p.cfg.AllowedDomains }

func (p *gitHubProvider) oauth(redirect string) *oauth2.Config {
	return &oauth2.Config{
		ClientID: p.cfg.ClientID, ClientSecret: p.cfg.ClientSecret, RedirectURL: redirect,
		Endpoint: oauth2.Endpoint{
			AuthURL: p.cfg.Issuer + "/login/oauth/authorize", TokenURL: p.cfg.Issuer + "/login/oauth/access_token",
			AuthStyle: oauth2.AuthStyleInParams,
		},
		Scopes: []string{"read:user", "user:email"},
	}
}

func (p *gitHubProvider) AuthorizeURL(_ context.Context, f Flow) (string, error) {
	return p.oauth(f.RedirectURL).AuthCodeURL(f.State, oauth2.S256ChallengeOption(f.Verifier), oauth2.SetAuthURLParam("allow_signup", "false")), nil
}

type gitHubUser struct {
	ID    int64
	Login string
	Name  string
}

type gitHubEmail struct {
	Email    string
	Primary  bool
	Verified bool
}

func (p *gitHubProvider) Exchange(ctx context.Context, code string, f Flow) (Identity, error) {
	ctx = context.WithValue(ctx, oauth2.HTTPClient, p.client)
	token, err := p.oauth(f.RedirectURL).Exchange(ctx, code, oauth2.VerifierOption(f.Verifier))
	if err != nil {
		return Identity{}, rejected("code exchange", err)
	}
	var user gitHubUser
	if err := p.get(ctx, token.AccessToken, "/user", &user); err != nil {
		return Identity{}, rejected("user", err)
	}
	if user.ID <= 0 {
		return Identity{}, rejected("user", errBadGitHubUser)
	}
	name := strings.TrimSpace(user.Name)
	if name == "" {
		name = user.Login
	}
	out := Identity{Issuer: p.cfg.Issuer, Subject: strconv.FormatInt(user.ID, 10), Name: name, Login: CleanLogin(user.Login)}
	var emails []gitHubEmail
	if err := p.get(ctx, token.AccessToken, "/user/emails", &emails); err == nil {
		for i, e := range emails {
			if i >= gitHubMaxEmails {
				break
			}
			if addr := strings.TrimSpace(e.Email); addr != "" {
				out.Emails = append(out.Emails, Email{Address: addr, Verified: e.Verified, Primary: e.Primary})
			}
		}
	}
	return out, nil
}

func (p *gitHubProvider) get(ctx context.Context, accessToken, path string, into any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.api+path, http.NoBody)
	if err != nil {
		return err
	}
	if accessToken != "" {
		req.Header.Set("Authorization", "Bearer "+accessToken)
	}
	req.Header.Set("Accept", gitHubMediaType)
	req.Header.Set("X-GitHub-Api-Version", gitHubAPIVersion)
	res, err := p.client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = res.Body.Close() }()
	body, err := readJSON(res)
	if err != nil {
		return err
	}
	return json.Unmarshal(body, into)
}
