package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
	"github.com/mistweaverco/withsecrets/apps/server/storage"
)

type oauthProvider struct {
	Name         string
	AuthURL      string
	TokenURL     string
	UserURL      string
	EmailURL     string
	JWKSURL      string
	Issuer       string
	ClientID     string
	ClientSecret string
	Scopes       string
}

func (s *Server) providers() map[string]oauthProvider {
	out := map[string]oauthProvider{}
	if s.cfg.GitHubID != "" && s.cfg.GitHubSecret != "" {
		out["github"] = oauthProvider{
			Name: "github", AuthURL: "https://github.com/login/oauth/authorize",
			TokenURL: "https://github.com/login/oauth/access_token",
			UserURL:  "https://api.github.com/user", EmailURL: "https://api.github.com/user/emails",
			Issuer: "https://github.com", ClientID: s.cfg.GitHubID, ClientSecret: s.cfg.GitHubSecret,
			Scopes: "read:user user:email",
		}
	}
	if s.cfg.GoogleID != "" && s.cfg.GoogleSecret != "" {
		out["google"] = oauthProvider{
			Name: "google", AuthURL: "https://accounts.google.com/o/oauth2/v2/auth",
			TokenURL: "https://oauth2.googleapis.com/token",
			JWKSURL:  "https://www.googleapis.com/oauth2/v3/certs",
			Issuer:   "https://accounts.google.com", ClientID: s.cfg.GoogleID, ClientSecret: s.cfg.GoogleSecret,
			Scopes: "openid email profile",
		}
	}
	if s.cfg.MicrosoftID != "" && s.cfg.MicrosoftSecret != "" && s.cfg.MicrosoftTenant != "" && s.cfg.MicrosoftTenant != "common" {
		base := "https://login.microsoftonline.com/" + url.PathEscape(s.cfg.MicrosoftTenant)
		out["microsoft"] = oauthProvider{
			Name: "microsoft", AuthURL: base + "/oauth2/v2.0/authorize",
			TokenURL: base + "/oauth2/v2.0/token",
			JWKSURL:  base + "/discovery/v2.0/keys",
			Issuer:   "https://login.microsoftonline.com/" + s.cfg.MicrosoftTenant + "/v2.0",
			ClientID: s.cfg.MicrosoftID, ClientSecret: s.cfg.MicrosoftSecret,
			Scopes: "openid email profile",
		}
	}
	return out
}

// SetProvider replaces a provider definition. Tests point AuthURL and TokenURL at a local IdP.
func (s *Server) SetProvider(p oauthProvider) { s.oauth[p.Name] = p }

func (s *Server) exchangeProviderCode(r *http.Request, name, code string, tx storage.Transaction) (identityResult, error) {
	return s.completeOAuth(r, name, code, tx)
}

func (s *Server) completeOAuth(r *http.Request, name, code string, tx storage.Transaction) (identityResult, error) {
	p, ok := s.oauth[name]
	if !ok || code == "" {
		return identityResult{}, fmt.Errorf("provider")
	}
	form := url.Values{}
	form.Set("client_id", p.ClientID)
	form.Set("client_secret", p.ClientSecret)
	form.Set("code", code)
	form.Set("grant_type", "authorization_code")
	form.Set("redirect_uri", strings.TrimRight(s.cfg.PublicBaseURL, "/")+"/auth/callback/"+name)
	if tx.ProviderVerifier != "" {
		form.Set("code_verifier", tx.ProviderVerifier)
	}
	req, err := http.NewRequestWithContext(r.Context(), http.MethodPost, p.TokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return identityResult{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "withsecrets")
	resp, err := s.client.Do(req)
	if err != nil {
		return identityResult{}, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	var tok struct {
		AccessToken      string `json:"access_token"`
		IDToken          string `json:"id_token"`
		Error            string `json:"error"`
		ErrorDescription string `json:"error_description"`
	}
	if err := json.Unmarshal(raw, &tok); err != nil {
		return identityResult{}, fmt.Errorf("token status %d", resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK || tok.Error != "" {
		return identityResult{}, fmt.Errorf("token status %d: %s", resp.StatusCode, tok.Error)
	}
	if name == "github" {
		return s.githubIdentity(r, p, tok.AccessToken)
	}
	return s.oidcIdentity(r, p, tok.IDToken, tx.Nonce)
}

func (s *Server) githubIdentity(r *http.Request, p oauthProvider, access string) (identityResult, error) {
	if access == "" {
		return identityResult{}, fmt.Errorf("github token")
	}
	var user struct {
		ID   int64  `json:"id"`
		Name string `json:"name"`
	}
	if err := s.getJSON(r, p.UserURL, access, &user); err != nil {
		return identityResult{}, err
	}
	var emails []struct {
		Email    string `json:"email"`
		Primary  bool   `json:"primary"`
		Verified bool   `json:"verified"`
	}
	if err := s.getJSON(r, p.EmailURL, access, &emails); err != nil {
		return identityResult{}, err
	}
	var email string
	for _, e := range emails {
		if e.Primary && e.Verified {
			email = e.Email
		}
	}
	if email == "" {
		return identityResult{}, fmt.Errorf("unverified email")
	}
	return identityResult{Issuer: p.Issuer, Subject: strconv.FormatInt(user.ID, 10), Email: email, Name: user.Name, EmailVerified: true}, nil
}

func (s *Server) getJSON(r *http.Request, endpoint, access string, dst any) error {
	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+access)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "withsecrets")
	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("user status %d", resp.StatusCode)
	}
	return json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(dst)
}

func (s *Server) oidcIdentity(r *http.Request, p oauthProvider, raw, nonce string) (identityResult, error) {
	if raw == "" {
		return identityResult{}, fmt.Errorf("id token")
	}
	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, p.JWKSURL, nil)
	if err != nil {
		return identityResult{}, err
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return identityResult{}, err
	}
	defer resp.Body.Close()
	var set jose.JSONWebKeySet
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&set); err != nil {
		return identityResult{}, err
	}
	tok, err := jwt.ParseSigned(raw, []jose.SignatureAlgorithm{jose.RS256})
	if err != nil {
		return identityResult{}, err
	}
	var claims jwt.Claims
	var extra struct {
		Email         string `json:"email"`
		EmailVerified any    `json:"email_verified"`
		Nonce         string `json:"nonce"`
	}
	if err := tok.Claims(&set, &claims, &extra); err != nil {
		return identityResult{}, err
	}
	if err := claims.Validate(jwt.Expected{Issuer: p.Issuer, AnyAudience: jwt.Audience{p.ClientID}, Time: time.Now()}); err != nil {
		return identityResult{}, err
	}
	if nonce != "" && extra.Nonce != nonce {
		return identityResult{}, fmt.Errorf("nonce")
	}
	verified := false
	switch v := extra.EmailVerified.(type) {
	case bool:
		verified = v
	case string:
		verified = v == "true"
	}
	if !verified || extra.Email == "" || claims.Subject == "" {
		return identityResult{}, fmt.Errorf("unverified email")
	}
	return identityResult{Issuer: p.Issuer, Subject: claims.Subject, Email: extra.Email, Name: extra.Email, EmailVerified: true}, nil
}
