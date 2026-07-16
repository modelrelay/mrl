package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// runWebLogin performs a browser/loopback OAuth login (RFC 8252): it starts a
// local server on an ephemeral loopback port, uses it as the OAuth return_to,
// opens the browser to the provider, and captures the account token the
// /auth/oauth/callback posts back. No password, no manual token copying.
func runWebLogin(cfg runtimeConfig, provider string) error {
	if provider == "" {
		provider = "github"
	}

	// Local loopback server that receives the posted tokens.
	var listenConfig net.ListenConfig
	listener, err := listenConfig.Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		return fmt.Errorf("start local callback server: %w", err)
	}
	defer func() { _ = listener.Close() }()
	tcpAddress, ok := listener.Addr().(*net.TCPAddr)
	if !ok {
		return fmt.Errorf("local callback listener returned address type %T", listener.Addr())
	}
	port := tcpAddress.Port
	redirectURL := fmt.Sprintf("http://127.0.0.1:%d/callback", port)

	// Binds this flow to our callback (the API echoes it back as handoff_nonce).
	nonce, err := randomHex(16)
	if err != nil {
		return err
	}

	type loginResult struct {
		access  string
		refresh string
		err     error
	}
	resultCh := make(chan loginResult, 1)

	mux := http.NewServeMux()
	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		if parseErr := r.ParseForm(); parseErr != nil {
			webLoginFailPage(w, "could not parse the sign-in callback")
			resultCh <- loginResult{err: fmt.Errorf("parse callback: %w", parseErr)}
			return
		}
		if r.PostFormValue("handoff_nonce") != nonce {
			webLoginFailPage(w, "state mismatch")
			resultCh <- loginResult{err: errors.New("callback nonce mismatch (possible CSRF) — aborted")}
			return
		}
		access := r.PostFormValue("access_token")
		if access == "" {
			webLoginFailPage(w, "no token was returned")
			resultCh <- loginResult{err: errors.New("callback contained no access token")}
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = io.WriteString(w, webLoginSuccessHTML)
		resultCh <- loginResult{access: access, refresh: r.PostFormValue("refresh_token")}
	})

	srv := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go func() { _ = srv.Serve(listener) }()
	defer func() {
		shutCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutCtx)
	}()

	// Ask the API for the provider authorization URL.
	startPath := fmt.Sprintf("/auth/oauth/start?provider=%s&return_to=%s&handoff_nonce=%s",
		url.QueryEscape(provider), url.QueryEscape(redirectURL), url.QueryEscape(nonce))
	startCtx, cancel := contextWithTimeout(cfg.Timeout)
	defer cancel()
	var startResp struct {
		RedirectURL string `json:"redirect_url"`
	}
	if startErr := doJSON(startCtx, cfg, authModeNone, http.MethodPost, startPath, nil, &startResp); startErr != nil {
		return fmt.Errorf("oauth start: %w", startErr)
	}
	if strings.TrimSpace(startResp.RedirectURL) == "" {
		return errors.New("oauth start returned no redirect url")
	}

	// Hand off to the browser.
	fmt.Fprintf(os.Stderr, "Opening your browser to sign in with %s...\n", provider)
	fmt.Fprintf(os.Stderr, "If it doesn't open, visit:\n  %s\n", startResp.RedirectURL)
	_ = openBrowser(startResp.RedirectURL)

	select {
	case res := <-resultCh:
		if res.err != nil {
			return res.err
		}
		if persistErr := persistAccountToken(cfg.Profile, res.access, res.refresh); persistErr != nil {
			return persistErr
		}
		fmt.Printf("logged in via %s; account token saved to profile %s\n", provider, cfg.Profile)
		return nil
	case <-time.After(3 * time.Minute):
		return errors.New("timed out waiting for browser sign-in")
	}
}

const webLoginSuccessHTML = `<!doctype html><html><head><meta charset="utf-8"><title>Signed in</title></head>
<body style="font-family:-apple-system,system-ui,sans-serif;text-align:center;padding:3rem">
<h2>Signed in to ModelRelay</h2>
<p>You can close this tab and return to the terminal.</p>
</body></html>`

// webLoginFailPage renders a static failure page (msg is always a constant —
// never user input — so it is safe to embed directly).
func webLoginFailPage(w http.ResponseWriter, msg string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusBadRequest)
	_, _ = io.WriteString(w, "<!doctype html><html><body style=\"font-family:sans-serif;text-align:center;padding:3rem\"><h2>Sign-in failed</h2><p>"+msg+"</p></body></html>")
}

func randomHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate nonce: %w", err)
	}
	return hex.EncodeToString(b), nil
}

func openBrowser(rawURL string) error {
	var name string
	var args []string
	switch runtime.GOOS {
	case "darwin":
		name, args = "open", []string{rawURL}
	case "windows":
		name, args = "rundll32", []string{"url.dll,FileProtocolHandler", rawURL}
	default:
		name, args = "xdg-open", []string{rawURL}
	}
	return exec.CommandContext(context.Background(), name, args...).Start() //nolint:gosec // command is an OS constant; the URL is the explicit browser target
}
