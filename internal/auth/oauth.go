package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net"
	"net/http"
	"net/url"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// openBrowserFunc can be overridden in tests.
var openBrowserFunc = openBrowser

// httpClientFunc can be overridden in tests.
var httpClientFunc = http.DefaultClient.Do

// registrationResponse holds the dynamic client registration result.
type registrationResponse struct {
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
}

// tokenResponse holds the token endpoint result.
type tokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int    `json:"expires_in"`
}

// GenerateCodeVerifier creates a random PKCE code verifier (43-128 chars, base64url).
func GenerateCodeVerifier() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generating code verifier: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// CodeChallenge computes the S256 code challenge from a code verifier.
func CodeChallenge(verifier string) string {
	h := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(h[:])
}

// RegisterClient performs dynamic client registration (RFC 7591).
func RegisterClient(serverURL, redirectURI string) (*registrationResponse, error) {
	regURL := strings.TrimRight(serverURL, "/") + "/register"

	body, err := json.Marshal(map[string]any{
		"redirect_uris":              []string{redirectURI},
		"grant_types":                []string{"authorization_code"},
		"response_types":             []string{"code"},
		"token_endpoint_auth_method": "client_secret_post",
	})
	if err != nil {
		return nil, fmt.Errorf("building registration request: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, regURL, strings.NewReader(string(body)))
	if err != nil {
		return nil, fmt.Errorf("creating registration request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClientFunc(req)
	if err != nil {
		return nil, fmt.Errorf("client registration failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading registration response: %w", err)
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("client registration failed (%d): %s", resp.StatusCode, string(respBody))
	}

	var reg registrationResponse
	if err := json.Unmarshal(respBody, &reg); err != nil {
		return nil, fmt.Errorf("parsing registration response: %w", err)
	}
	return &reg, nil
}

// ExchangeCode exchanges an authorization code for an access token.
func ExchangeCode(serverURL, code, redirectURI, codeVerifier, clientID, clientSecret string) (*tokenResponse, error) {
	tokenURL := strings.TrimRight(serverURL, "/") + "/token"

	data := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {redirectURI},
		"code_verifier": {codeVerifier},
		"client_id":     {clientID},
		"client_secret": {clientSecret},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("creating token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := httpClientFunc(req)
	if err != nil {
		return nil, fmt.Errorf("token exchange failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading token response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("token exchange failed (%d): %s", resp.StatusCode, string(respBody))
	}

	var tok tokenResponse
	if err := json.Unmarshal(respBody, &tok); err != nil {
		return nil, fmt.Errorf("parsing token response: %w", err)
	}
	return &tok, nil
}

// Login performs the full OAuth Authorization Code flow with PKCE.
// It registers a dynamic client, starts a local callback server, opens the
// browser, waits for the callback, exchanges the code, and stores credentials.
func Login(serverURL string, noBrowser bool) error {
	// Start local callback server on random port.
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return fmt.Errorf("starting callback server: %w", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	redirectURI := fmt.Sprintf("http://localhost:%d/callback", port)

	// Register dynamic client.
	reg, err := RegisterClient(serverURL, redirectURI)
	if err != nil {
		listener.Close()
		return err
	}

	// Generate PKCE values.
	verifier, err := GenerateCodeVerifier()
	if err != nil {
		listener.Close()
		return err
	}
	challenge := CodeChallenge(verifier)

	// Channel to receive the authorization code from the callback.
	codeCh := make(chan string, 1)
	errCh := make(chan error, 1)

	mux := http.NewServeMux()
	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		code := r.URL.Query().Get("code")
		if code == "" {
			errMsg := r.URL.Query().Get("error")
			if errMsg == "" {
				errMsg = "no authorization code received"
			}
			errCh <- fmt.Errorf("authorization failed: %s", errMsg)
			fmt.Fprintf(w, "<html><body><h1>Authentication failed</h1><p>%s</p><p>You can close this window.</p></body></html>", html.EscapeString(errMsg))
			return
		}
		codeCh <- code
		fmt.Fprint(w, "<html><body><h1>Authentication successful</h1><p>You can close this window.</p></body></html>")
	})

	srv := &http.Server{Handler: mux}
	go func() {
		if err := srv.Serve(listener); err != nil && err != http.ErrServerClosed {
			errCh <- fmt.Errorf("callback server error: %w", err)
		}
	}()

	// Build authorization URL and open browser.
	authURL := fmt.Sprintf("%s/authorize?response_type=code&client_id=%s&redirect_uri=%s&code_challenge=%s&code_challenge_method=S256",
		strings.TrimRight(serverURL, "/"),
		url.QueryEscape(reg.ClientID),
		url.QueryEscape(redirectURI),
		url.QueryEscape(challenge),
	)

	if noBrowser {
		fmt.Printf("Open this URL in your browser:\n%s\n", authURL)
	} else {
		fmt.Println("Opening browser for authentication...")
		if err := openBrowserFunc(authURL); err != nil {
			fmt.Printf("Could not open browser. Please visit:\n%s\n", authURL)
		}
	}

	// Wait for callback or timeout.
	var code string
	select {
	case code = <-codeCh:
	case err := <-errCh:
		srv.Shutdown(context.Background())
		return err
	case <-time.After(5 * time.Minute):
		srv.Shutdown(context.Background())
		return fmt.Errorf("authentication timed out after 5 minutes")
	}

	// Shut down callback server.
	srv.Shutdown(context.Background())

	// Exchange code for token.
	tok, err := ExchangeCode(serverURL, code, redirectURI, verifier, reg.ClientID, reg.ClientSecret)
	if err != nil {
		return err
	}

	// Compute expiry time.
	expiresAt := ""
	if tok.ExpiresIn > 0 {
		expiresAt = time.Now().Add(time.Duration(tok.ExpiresIn) * time.Second).UTC().Format(time.RFC3339)
	}

	// Store credentials.
	creds, err := LoadCredentials()
	if err != nil {
		return fmt.Errorf("loading credentials: %w", err)
	}
	creds.AccessToken = tok.AccessToken
	creds.TokenType = tok.TokenType
	creds.ExpiresAt = expiresAt
	creds.ClientID = reg.ClientID
	creds.ClientSecret = reg.ClientSecret
	if err := creds.Save(); err != nil {
		return fmt.Errorf("saving credentials: %w", err)
	}

	fmt.Println("Authentication successful.")
	return nil
}

// Logout clears OAuth session fields from credentials.json.
func Logout() error {
	creds, err := LoadCredentials()
	if err != nil {
		return fmt.Errorf("loading credentials: %w", err)
	}
	creds.ClearOAuth()
	if err := creds.Save(); err != nil {
		return fmt.Errorf("saving credentials: %w", err)
	}
	return nil
}

// StatusInfo holds information about the current authentication state.
type StatusInfo struct {
	Mode      string // "oauth", "static-token", "none"
	Token     string // masked token for display
	ExpiresAt string // RFC 3339 timestamp (OAuth only)
	Expired   bool   // whether the OAuth token is expired
}

// Status returns information about the current authentication state.
// It follows the same token-selection precedence as config.Resolve():
// auth_token (static) > access_token (OAuth, only if not expired).
func Status() (*StatusInfo, error) {
	creds, err := LoadCredentials()
	if err != nil {
		return nil, fmt.Errorf("loading credentials: %w", err)
	}

	info := &StatusInfo{Mode: "none"}

	// Static token takes precedence, matching config.Resolve() behavior.
	if creds.AuthToken != "" {
		info.Mode = "static-token"
		info.Token = maskToken(creds.AuthToken)
		return info, nil
	}

	if creds.AccessToken != "" {
		info.Mode = "oauth"
		info.Token = maskToken(creds.AccessToken)
		info.ExpiresAt = creds.ExpiresAt
		if creds.ExpiresAt != "" {
			t, err := time.Parse(time.RFC3339, creds.ExpiresAt)
			if err == nil {
				info.Expired = time.Now().After(t)
			} else {
				info.Expired = true
			}
		} else {
			// No expiry means unusable OAuth token.
			info.Expired = true
		}
		return info, nil
	}

	return info, nil
}

// maskToken shows only the first 8 and last 4 characters.
func maskToken(token string) string {
	if len(token) <= 12 {
		return "****"
	}
	return token[:8] + "..." + token[len(token)-4:]
}

// openBrowser opens the given URL in the default browser.
func openBrowser(rawURL string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", rawURL)
	case "linux":
		cmd = exec.Command("xdg-open", rawURL)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", rawURL)
	default:
		return fmt.Errorf("unsupported platform %s", runtime.GOOS)
	}
	return cmd.Start()
}
