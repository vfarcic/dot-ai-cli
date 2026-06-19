package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/vfarcic/dot-ai-cli/internal/config"
)

// Exit code constants matching PRD spec.
const (
	ExitSuccess    = 0
	ExitToolError  = 1
	ExitConnError  = 2
	ExitUsageError = 3
)

// RequestError wraps an HTTP error with an exit code for the CLI.
type RequestError struct {
	Message  string
	ExitCode int
	// Status is the HTTP status code that produced this error (0 for
	// non-HTTP failures such as a connection error). Callers can use it to
	// scope handling, e.g. reframing a request-scoped 4xx.
	Status int
	// ServerMessage is the raw message extracted from the server's error
	// envelope, before it is wrapped into the user-facing Message. Empty when
	// the server returned no parseable message.
	ServerMessage string
}

func (e *RequestError) Error() string {
	return e.Message
}

// Param holds a resolved parameter name, value, and location.
type Param struct {
	Name     string
	Value    string
	Location string // "path", "query", "body"
	// ForceString, when set on a "body" param, forces the value to be encoded
	// as a JSON string even when it happens to be valid JSON on its own (e.g. a
	// numeric branch name "123" or "true"). Known-string fields like the
	// prompts-override repo/path/branch set this so they are never silently
	// sent as a JSON number/bool/null. Without it, body values are parsed as
	// JSON first to support object/array params from dynamic commands.
	ForceString bool
}

// Do executes an HTTP request against the server.
//
// It handles path parameter substitution, query parameters, JSON body
// construction, Bearer auth, timeout, and error classification.
func Do(cfg *config.Config, method, pathTemplate string, params []Param) ([]byte, error) {
	return DoWithHeaders(cfg, method, pathTemplate, params, nil)
}

// DoWithHeaders behaves like Do but also sets the given extra request headers
// (empty-valued entries are skipped). It is used to forward the per-request
// X-Dot-AI-Git-Token credential on prompts-override requests. Header values
// are never logged.
func DoWithHeaders(cfg *config.Config, method, pathTemplate string, params []Param, headers map[string]string) ([]byte, error) {
	resolvedPath := pathTemplate
	queryParams := url.Values{}
	bodyFields := map[string]json.RawMessage{}

	for _, p := range params {
		switch p.Location {
		case "path":
			resolvedPath = strings.ReplaceAll(resolvedPath, "{"+p.Name+"}", url.PathEscape(p.Value))
		case "query":
			if p.Value != "" {
				queryParams.Set(p.Name, p.Value)
			}
		case "body":
			if p.Value != "" {
				if p.ForceString {
					// Known-string field: always encode as a JSON string so a
					// value that is itself valid JSON (e.g. "123") is not sent
					// as a number/bool/null.
					quoted, _ := json.Marshal(p.Value)
					bodyFields[p.Name] = quoted
					break
				}
				// Try to parse as JSON first (for object/array values).
				// If it fails, treat as a plain string.
				var raw json.RawMessage
				if json.Unmarshal([]byte(p.Value), &raw) == nil {
					bodyFields[p.Name] = raw
				} else {
					quoted, _ := json.Marshal(p.Value)
					bodyFields[p.Name] = quoted
				}
			}
		}
	}

	fullURL := strings.TrimRight(cfg.ServerURL, "/") + resolvedPath
	if len(queryParams) > 0 {
		fullURL += "?" + queryParams.Encode()
	}

	var bodyReader io.Reader
	if len(bodyFields) > 0 {
		bodyBytes, err := json.Marshal(bodyFields)
		if err != nil {
			return nil, &RequestError{
				Message:  fmt.Sprintf("failed to build request body: %v", err),
				ExitCode: ExitUsageError,
			}
		}
		bodyReader = bytes.NewReader(bodyBytes)
	} else if method == http.MethodPost || method == http.MethodPut || method == http.MethodPatch {
		bodyReader = bytes.NewReader([]byte("{}"))
	}

	ctx := context.Background()

	req, err := http.NewRequestWithContext(ctx, method, fullURL, bodyReader)
	if err != nil {
		return nil, &RequestError{
			Message:  fmt.Sprintf("failed to create request: %v", err),
			ExitCode: ExitToolError,
		}
	}

	if bodyReader != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if cfg.Token != "" {
		req.Header.Set("Authorization", "Bearer "+cfg.Token)
	}
	for k, v := range headers {
		if v != "" {
			req.Header.Set(k, v)
		}
	}

	httpClient := &http.Client{
		// net/http strips Authorization/Cookie on a cross-host redirect but
		// leaves caller-supplied headers intact, so X-Dot-AI-Git-Token (the
		// per-request git credential) would otherwise be re-sent to whatever
		// host the configured server redirects to. Drop every caller-supplied
		// header when the redirect target host differs from the original, so a
		// credential is never sent to a host the caller did not target.
		// Same-host redirects keep the headers, preserving normal behavior.
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return fmt.Errorf("stopped after 10 redirects")
			}
			if len(via) > 0 && req.URL.Host != via[0].URL.Host {
				for k := range headers {
					req.Header.Del(k)
				}
			}
			return nil
		},
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, &RequestError{
			Message: fmt.Sprintf("Error: cannot connect to server at %s.\n"+
				"Set the server URL with --server-url or DOT_AI_URL.", cfg.ServerURL),
			ExitCode: ExitConnError,
		}
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, &RequestError{
			Message:  fmt.Sprintf("Error: failed to read response: %v", err),
			ExitCode: ExitToolError,
		}
	}

	if resp.StatusCode >= 400 {
		return body, classifyHTTPError(resp.StatusCode, body)
	}

	return body, nil
}

// DoJSON sends method to path with the given pre-marshaled JSON body and extra
// request headers (empty-valued entries are skipped), using Bearer auth and the
// same error classification as Do/DoWithHeaders. It exists for endpoints whose
// body is a nested JSON document the flat Param-body model cannot express — e.g.
// the prompts-source ingestion upload, whose body is {source, contentHash,
// files:[{path,content,mode}]}. The same cross-host-redirect header-stripping
// policy applies so any caller-supplied header is never re-sent to a redirect
// target on a different host.
func DoJSON(cfg *config.Config, method, path string, body []byte, headers map[string]string) ([]byte, error) {
	fullURL := strings.TrimRight(cfg.ServerURL, "/") + path

	req, err := http.NewRequestWithContext(context.Background(), method, fullURL, bytes.NewReader(body))
	if err != nil {
		return nil, &RequestError{
			Message:  fmt.Sprintf("failed to create request: %v", err),
			ExitCode: ExitToolError,
		}
	}
	req.Header.Set("Content-Type", "application/json")
	if cfg.Token != "" {
		req.Header.Set("Authorization", "Bearer "+cfg.Token)
	}
	for k, v := range headers {
		if v != "" {
			req.Header.Set(k, v)
		}
	}

	httpClient := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return fmt.Errorf("stopped after 10 redirects")
			}
			if len(via) > 0 && req.URL.Host != via[0].URL.Host {
				for k := range headers {
					req.Header.Del(k)
				}
			}
			return nil
		},
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, &RequestError{
			Message: fmt.Sprintf("Error: cannot connect to server at %s.\n"+
				"Set the server URL with --server-url or DOT_AI_URL.", cfg.ServerURL),
			ExitCode: ExitConnError,
		}
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, &RequestError{
			Message:  fmt.Sprintf("Error: failed to read response: %v", err),
			ExitCode: ExitToolError,
		}
	}

	if resp.StatusCode >= 400 {
		return respBody, classifyHTTPError(resp.StatusCode, respBody)
	}
	return respBody, nil
}

// classifyHTTPError maps HTTP status codes to user-friendly errors.
func classifyHTTPError(status int, body []byte) *RequestError {
	msg := parseServerMessage(body)

	switch {
	case status == 401:
		return &RequestError{
			Message:       "authentication failed. Run 'dot-ai auth login', use --token flag, or set DOT_AI_AUTH_TOKEN env. var.",
			ExitCode:      ExitToolError,
			Status:        status,
			ServerMessage: msg,
		}
	case status == 404:
		if msg != "" {
			return &RequestError{
				Message:       fmt.Sprintf("Error: not found (404): %s", msg),
				ExitCode:      ExitToolError,
				Status:        status,
				ServerMessage: msg,
			}
		}
		return &RequestError{
			Message:  "Error: resource not found (404).",
			ExitCode: ExitToolError,
			Status:   status,
		}
	case status >= 500:
		if msg != "" {
			return &RequestError{
				Message:       fmt.Sprintf("Error: server error (%d): %s", status, msg),
				ExitCode:      ExitToolError,
				Status:        status,
				ServerMessage: msg,
			}
		}
		return &RequestError{
			Message:  fmt.Sprintf("Error: server error (%d). The server encountered an internal error.", status),
			ExitCode: ExitToolError,
			Status:   status,
		}
	default:
		if msg != "" {
			return &RequestError{
				Message:       fmt.Sprintf("Error: request failed (%d): %s", status, msg),
				ExitCode:      ExitToolError,
				Status:        status,
				ServerMessage: msg,
			}
		}
		return &RequestError{
			Message:  fmt.Sprintf("Error: request failed (%d).", status),
			ExitCode: ExitToolError,
			Status:   status,
		}
	}
}

// parseServerMessage extracts a human-readable message from the server's error
// envelope. It accepts both the legacy flat shape ({"error":"...", or
// "message":"..."}) and the structured envelope ({"error":{"code","message"}})
// the dot-ai server returns for validation failures. Any embedded credential is
// scrubbed before the message is returned (defense-in-depth — the server is
// expected to scrub, but the CLI must never echo a credential it received).
func parseServerMessage(body []byte) string {
	var env struct {
		Error   json.RawMessage `json:"error"`
		Message string          `json:"message"`
	}
	if json.Unmarshal(body, &env) != nil {
		return ""
	}
	if len(env.Error) > 0 {
		// error as a nested object {"code","message"}.
		var obj struct {
			Message string `json:"message"`
		}
		if json.Unmarshal(env.Error, &obj) == nil && obj.Message != "" {
			return RedactCredentials(obj.Message)
		}
		// error as a plain string.
		var s string
		if json.Unmarshal(env.Error, &s) == nil && s != "" {
			return RedactCredentials(s)
		}
	}
	return RedactCredentials(env.Message)
}

// credInURLRe matches userinfo embedded in a URL anywhere inside a larger
// string, so credentials can be scrubbed even when a whole-URL parse is not
// possible (e.g. a URL embedded in a server-supplied message). The userinfo
// segment is one-or-more chars that are not `/`, `@`, or whitespace, which
// already admits percent-encoded octets (`%40`, `%3A`, ...). The password
// group (`:...`) is OPTIONAL so the regex also covers username-only PAT URLs
// (`https://TOKEN@host`) and creds whose separating colon is itself percent-
// encoded (`https://user%3Apass@host`) — both of which a mandatory `:pass`
// group would let slip through unredacted.
var credInURLRe = regexp.MustCompile(`://[^/@\s]+(?::[^/@\s]*)?@`)

// RedactCredentials strips userinfo (user:password@, or a bare token@) from any
// URL embedded in s, replacing it with ://***:***@. Unlike a whole-string URL
// redaction, it works on free-text messages that merely contain a credentialed
// URL. It is a no-op for strings without an embedded credential and is
// idempotent (re-redacting an already-redacted string yields the same string).
func RedactCredentials(s string) string {
	return credInURLRe.ReplaceAllString(s, "://***:***@")
}
