package oauth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Defaults used by the flow.
const (
	defaultScope = "registry:read"
	clientName   = "fglpkg CLI"
	clientURI    = "https://github.com/4js-mikefolcher/fglpkg"
)

// LoginConfig overrides defaults — mostly useful in tests.
type LoginConfig struct {
	// Scope to request; defaults to "registry:read".
	Scope string
	// BrowserOpen is called with the authorise URL. Default opens the
	// system browser; tests can override to hit /callback directly.
	BrowserOpen func(url string) error
	// HTTPClient is used for /register and /token requests.
	HTTPClient *http.Client
}

// RunLogin opens the browser, runs the auth-code + PKCE flow against base
// (a registry URL with no trailing slash), and returns the resulting
// tokens. ctx is honoured at the loopback-callback wait — cancelling it
// shuts the local server down.
func RunLogin(ctx context.Context, base string, cfg LoginConfig) (Tokens, error) {
	base = strings.TrimRight(base, "/")
	scope := cfg.Scope
	if scope == "" {
		scope = defaultScope
	}
	open := cfg.BrowserOpen
	if open == nil {
		open = openInBrowser
	}
	client := cfg.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}

	srv, err := startCallbackServer()
	if err != nil {
		return Tokens{}, err
	}
	defer srv.close()

	dcr, err := registerClient(ctx, client, base, srv.redirectURI)
	if err != nil {
		return Tokens{}, err
	}

	verifier, err := GenerateVerifier()
	if err != nil {
		return Tokens{}, err
	}
	state, err := GenerateState()
	if err != nil {
		return Tokens{}, err
	}
	challenge := ChallengeFor(verifier)

	authURL := buildAuthURL(base, dcr.ClientID, srv.redirectURI, scope, state, challenge)
	if err := open(authURL); err != nil {
		return Tokens{}, fmt.Errorf("open browser: %w", err)
	}

	code, gotState, err := srv.awaitCallback(ctx)
	if err != nil {
		return Tokens{}, err
	}
	if gotState != state {
		return Tokens{}, fmt.Errorf("OAuth state mismatch — possible CSRF; aborting")
	}

	tr, err := exchangeCode(ctx, client, base, exchangeArgs{
		ClientID:     dcr.ClientID,
		ClientSecret: dcr.ClientSecret,
		Code:         code,
		RedirectURI:  srv.redirectURI,
		Verifier:     verifier,
	})
	if err != nil {
		return Tokens{}, err
	}
	return tokensFromResponse(tr, scope, dcr.ClientID, dcr.ClientSecret), nil
}

// Refresh trades a refresh_token for a fresh access token. The refresh_token
// itself may rotate; callers should persist whatever comes back.
func Refresh(ctx context.Context, base string, prev Tokens) (Tokens, error) {
	if prev.RefreshToken == "" {
		return Tokens{}, fmt.Errorf("no refresh_token on record — log in again")
	}
	base = strings.TrimRight(base, "/")

	body := url.Values{}
	body.Set("grant_type", "refresh_token")
	body.Set("refresh_token", prev.RefreshToken)
	body.Set("client_id", prev.ClientID)
	if prev.ClientSecret != "" {
		body.Set("client_secret", prev.ClientSecret)
	}

	tr, err := postForm(ctx, http.DefaultClient, base+"/token", body)
	if err != nil {
		return Tokens{}, fmt.Errorf("refresh: %w", err)
	}
	return tokensFromResponse(tr, prev.Scope, prev.ClientID, prev.ClientSecret), nil
}

// ─── internals ──────────────────────────────────────────────────────────────

type dcrResponse struct {
	ClientID     string   `json:"client_id"`
	ClientSecret string   `json:"client_secret,omitempty"`
	RedirectURIs []string `json:"redirect_uris,omitempty"`
}

type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token,omitempty"`
	ExpiresIn    int    `json:"expires_in,omitempty"`
	TokenType    string `json:"token_type,omitempty"`
	Scope        string `json:"scope,omitempty"`
}

type exchangeArgs struct {
	ClientID, ClientSecret, Code, RedirectURI, Verifier string
}

func registerClient(ctx context.Context, c *http.Client, base, redirectURI string) (dcrResponse, error) {
	payload := map[string]any{
		"client_name":                clientName,
		"client_uri":                 clientURI,
		"redirect_uris":              []string{redirectURI},
		"token_endpoint_auth_method": "none",
		"grant_types":                []string{"authorization_code", "refresh_token"},
		"response_types":             []string{"code"},
		"scope":                      defaultScope,
	}
	buf, _ := json.Marshal(payload)
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, base+"/register", strings.NewReader(string(buf)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	resp, err := c.Do(req)
	if err != nil {
		return dcrResponse{}, fmt.Errorf("dynamic client registration: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return dcrResponse{}, fmt.Errorf("dynamic client registration: HTTP %d — %s", resp.StatusCode, truncate(string(body), 240))
	}
	var d dcrResponse
	if err := json.Unmarshal(body, &d); err != nil {
		return dcrResponse{}, fmt.Errorf("dynamic client registration: invalid JSON: %w", err)
	}
	if d.ClientID == "" {
		return dcrResponse{}, fmt.Errorf("dynamic client registration: response missing client_id")
	}
	return d, nil
}

func buildAuthURL(base, clientID, redirectURI, scope, state, challenge string) string {
	u, _ := url.Parse(base + "/authorize")
	q := u.Query()
	q.Set("response_type", "code")
	q.Set("client_id", clientID)
	q.Set("redirect_uri", redirectURI)
	q.Set("scope", scope)
	q.Set("state", state)
	q.Set("code_challenge", challenge)
	q.Set("code_challenge_method", "S256")
	u.RawQuery = q.Encode()
	return u.String()
}

func exchangeCode(ctx context.Context, c *http.Client, base string, a exchangeArgs) (tokenResponse, error) {
	body := url.Values{}
	body.Set("grant_type", "authorization_code")
	body.Set("code", a.Code)
	body.Set("redirect_uri", a.RedirectURI)
	body.Set("client_id", a.ClientID)
	body.Set("code_verifier", a.Verifier)
	if a.ClientSecret != "" {
		body.Set("client_secret", a.ClientSecret)
	}
	return postForm(ctx, c, base+"/token", body)
}

func postForm(ctx context.Context, c *http.Client, u string, body url.Values) (tokenResponse, error) {
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, u, strings.NewReader(body.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	resp, err := c.Do(req)
	if err != nil {
		return tokenResponse{}, err
	}
	defer resp.Body.Close()
	buf, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return tokenResponse{}, fmt.Errorf("HTTP %d — %s", resp.StatusCode, truncate(string(buf), 240))
	}
	var tr tokenResponse
	if err := json.Unmarshal(buf, &tr); err != nil {
		return tokenResponse{}, fmt.Errorf("invalid token JSON: %w", err)
	}
	if tr.AccessToken == "" {
		return tokenResponse{}, fmt.Errorf("token response missing access_token")
	}
	return tr, nil
}

func tokensFromResponse(tr tokenResponse, fallbackScope, clientID, clientSecret string) Tokens {
	expiresIn := tr.ExpiresIn
	if expiresIn <= 0 {
		expiresIn = 3600
	}
	scope := tr.Scope
	if scope == "" {
		scope = fallbackScope
	}
	return Tokens{
		AccessToken:  tr.AccessToken,
		RefreshToken: tr.RefreshToken,
		ExpiresAt:    time.Now().Add(time.Duration(expiresIn) * time.Second),
		Scope:        scope,
		ClientID:     clientID,
		ClientSecret: clientSecret,
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
