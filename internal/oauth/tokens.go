package oauth

import "time"

// Tokens is what RunLogin returns and what callers persist on disk.
//
// JSON shape matches fglpkg-cli's credentials.json layout so the two CLIs
// could in principle share a credentials file (we currently do not, but
// keeping the on-disk shape compatible avoids future churn).
type Tokens struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token,omitempty"`
	ExpiresAt    time.Time `json:"expires_at"`
	Scope        string    `json:"scope,omitempty"`
	ClientID     string    `json:"client_id"`
	ClientSecret string    `json:"client_secret,omitempty"`
}

// Expired reports whether t is within skew of its ExpiresAt. A token without
// a parseable ExpiresAt is treated as expired so the caller refreshes.
func Expired(t Tokens, skew time.Duration) bool {
	if t.ExpiresAt.IsZero() {
		return true
	}
	return time.Now().Add(skew).After(t.ExpiresAt)
}
