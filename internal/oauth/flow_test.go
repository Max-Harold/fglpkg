package oauth

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

// fakeRegistry stands in for /register, /authorize, and /token. /authorize
// responds by 302-redirecting to the loopback redirect_uri with a code &
// state — that's how a real provider would behave after the user clicks
// "Allow". For test purposes the redirect step is enough.
func fakeRegistry(t *testing.T, opts fakeRegistryOpts) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()

	mux.HandleFunc("/register", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method", http.StatusMethodNotAllowed)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"client_id":     "test-client-id",
			"client_secret": opts.ClientSecret, // empty if test wants a public client
		})
	})

	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		switch r.Form.Get("grant_type") {
		case "authorization_code":
			if r.Form.Get("code") != "test-code" || r.Form.Get("code_verifier") == "" {
				http.Error(w, "bad code/verifier", http.StatusBadRequest)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"access_token":  "access-1",
				"refresh_token": "refresh-1",
				"expires_in":    3600,
				"scope":         "registry:read",
			})
		case "refresh_token":
			if r.Form.Get("refresh_token") != "refresh-1" {
				http.Error(w, "bad refresh", http.StatusUnauthorized)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"access_token":  "access-2",
				"refresh_token": "refresh-2",
				"expires_in":    3600,
			})
		default:
			http.Error(w, "unknown grant_type", http.StatusBadRequest)
		}
	})

	return httptest.NewServer(mux)
}

type fakeRegistryOpts struct {
	ClientSecret string
}

// browserHitter, given the authorise URL, hits the loopback redirect_uri
// directly with a synthesised code+state. That mimics what a real browser
// would do after the user clicked "Allow".
func browserHitter(t *testing.T) func(string) error {
	t.Helper()
	return func(authURL string) error {
		u, err := url.Parse(authURL)
		if err != nil {
			return err
		}
		q := u.Query()
		redirect, _ := url.Parse(q.Get("redirect_uri"))
		rq := redirect.Query()
		rq.Set("code", "test-code")
		rq.Set("state", q.Get("state"))
		redirect.RawQuery = rq.Encode()

		go func() {
			// Tiny delay so RunLogin reaches awaitCallback first.
			time.Sleep(20 * time.Millisecond)
			_, _ = http.Get(redirect.String())
		}()
		return nil
	}
}

func TestRunLoginHappyPath(t *testing.T) {
	srv := fakeRegistry(t, fakeRegistryOpts{})
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	got, err := RunLogin(ctx, srv.URL, LoginConfig{
		BrowserOpen: browserHitter(t),
	})
	if err != nil {
		t.Fatalf("RunLogin: %v", err)
	}
	if got.AccessToken != "access-1" {
		t.Errorf("AccessToken = %q, want access-1", got.AccessToken)
	}
	if got.RefreshToken != "refresh-1" {
		t.Errorf("RefreshToken = %q, want refresh-1", got.RefreshToken)
	}
	if got.ClientID != "test-client-id" {
		t.Errorf("ClientID = %q, want test-client-id", got.ClientID)
	}
	if got.ExpiresAt.Before(time.Now()) {
		t.Errorf("ExpiresAt = %v should be in the future", got.ExpiresAt)
	}
}

func TestRefreshRotates(t *testing.T) {
	srv := fakeRegistry(t, fakeRegistryOpts{})
	defer srv.Close()
	ctx := context.Background()

	prev := Tokens{
		AccessToken:  "access-1",
		RefreshToken: "refresh-1",
		ExpiresAt:    time.Now().Add(-time.Minute), // expired
		Scope:        "registry:read",
		ClientID:     "test-client-id",
	}
	got, err := Refresh(ctx, srv.URL, prev)
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if got.AccessToken != "access-2" || got.RefreshToken != "refresh-2" {
		t.Errorf("post-refresh tokens = %+v, want access-2/refresh-2", got)
	}
}

func TestRunLoginStateMismatchAborts(t *testing.T) {
	srv := fakeRegistry(t, fakeRegistryOpts{})
	defer srv.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	open := func(authURL string) error {
		u, _ := url.Parse(authURL)
		q := u.Query()
		redirect, _ := url.Parse(q.Get("redirect_uri"))
		rq := redirect.Query()
		rq.Set("code", "test-code")
		rq.Set("state", "WRONG-STATE")
		redirect.RawQuery = rq.Encode()
		go func() {
			time.Sleep(20 * time.Millisecond)
			_, _ = http.Get(redirect.String())
		}()
		return nil
	}
	_, err := RunLogin(ctx, srv.URL, LoginConfig{BrowserOpen: open})
	if err == nil || !strings.Contains(err.Error(), "state mismatch") {
		t.Fatalf("want state mismatch error, got %v", err)
	}
}

func TestRegisterClientErrorSurfacesBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"error":"invalid_client_metadata"}`)
	}))
	defer srv.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	_, err := RunLogin(ctx, srv.URL, LoginConfig{
		BrowserOpen: func(string) error { return nil },
	})
	if err == nil || !strings.Contains(err.Error(), "invalid_client_metadata") {
		t.Fatalf("want body in error, got %v", err)
	}
}
