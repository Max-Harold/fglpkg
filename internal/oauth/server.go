package oauth

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"
)

// callbackServer is the small HTTP server we bind on 127.0.0.1:<random> for
// the duration of the login flow. It accepts a single GET /callback then
// signals the waiter via the result channel.
type callbackServer struct {
	server      *http.Server
	listener    net.Listener
	redirectURI string
	result      chan callbackResult
}

type callbackResult struct {
	code  string
	state string
	err   error
}

func startCallbackServer() (*callbackServer, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("bind loopback: %w", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port

	cs := &callbackServer{
		listener:    ln,
		redirectURI: fmt.Sprintf("http://127.0.0.1:%d/callback", port),
		result:      make(chan callbackResult, 1),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/callback", cs.handleCallback)
	cs.server = &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	go func() { _ = cs.server.Serve(ln) }()

	return cs, nil
}

func (cs *callbackServer) handleCallback(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	code := q.Get("code")
	state := q.Get("state")
	errParam := q.Get("error")

	if errParam != "" {
		writeErrorPage(w, errParam, q.Get("error_description"))
		cs.result <- callbackResult{err: fmt.Errorf("authorisation failed: %s — %s", errParam, q.Get("error_description"))}
		return
	}
	if code == "" {
		writeErrorPage(w, "missing_code", "Authorisation server did not return a code.")
		cs.result <- callbackResult{err: fmt.Errorf("authorisation server returned no code")}
		return
	}
	writeSuccessPage(w)
	cs.result <- callbackResult{code: code, state: state}
}

// awaitCallback blocks until the browser hits /callback or ctx is cancelled.
func (cs *callbackServer) awaitCallback(ctx context.Context) (code, state string, err error) {
	select {
	case r := <-cs.result:
		return r.code, r.state, r.err
	case <-ctx.Done():
		return "", "", ctx.Err()
	}
}

func (cs *callbackServer) close() {
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	_ = cs.server.Shutdown(shutdownCtx)
}

func writeSuccessPage(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(`<!doctype html><meta charset="utf-8"><title>fglpkg signed in</title>
<style>body{font-family:system-ui,sans-serif;max-width:32rem;margin:4rem auto;color:#222}</style>
<h1>You're signed in.</h1>
<p>You can close this tab and return to your terminal.</p>`))
}

func writeErrorPage(w http.ResponseWriter, code, desc string) {
	w.WriteHeader(http.StatusBadRequest)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = fmt.Fprintf(w, `<!doctype html><meta charset="utf-8"><title>fglpkg sign-in failed</title>
<style>body{font-family:system-ui,sans-serif;max-width:32rem;margin:4rem auto;color:#222}</style>
<h1>Sign-in failed.</h1>
<p><code>%s</code>: %s</p>
<p>You can close this tab and re-run <code>fglpkg login</code>.</p>`, code, desc)
}
