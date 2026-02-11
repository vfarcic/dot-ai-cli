package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
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
}

func (e *RequestError) Error() string {
	return e.Message
}

// Param holds a resolved parameter name, value, and location.
type Param struct {
	Name     string
	Value    string
	Location string // "path", "query", "body"
}

// Do executes an HTTP request against the server.
//
// It handles path parameter substitution, query parameters, JSON body
// construction, Bearer auth, timeout, and error classification.
func Do(cfg *config.Config, method, pathTemplate string, params []Param) ([]byte, error) {
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

	resp, err := http.DefaultClient.Do(req)
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

// classifyHTTPError maps HTTP status codes to user-friendly errors.
func classifyHTTPError(status int, body []byte) *RequestError {
	var errResp struct {
		Error   string `json:"error"`
		Message string `json:"message"`
	}
	msg := ""
	if json.Unmarshal(body, &errResp) == nil {
		if errResp.Error != "" {
			msg = errResp.Error
		} else if errResp.Message != "" {
			msg = errResp.Message
		}
	}

	switch {
	case status == 401:
		return &RequestError{
			Message:  "Error: authentication failed (401). Check your --token or DOT_AI_AUTH_TOKEN.",
			ExitCode: ExitToolError,
		}
	case status == 404:
		if msg != "" {
			return &RequestError{
				Message:  fmt.Sprintf("Error: not found (404): %s", msg),
				ExitCode: ExitToolError,
			}
		}
		return &RequestError{
			Message:  "Error: resource not found (404).",
			ExitCode: ExitToolError,
		}
	case status >= 500:
		if msg != "" {
			return &RequestError{
				Message:  fmt.Sprintf("Error: server error (%d): %s", status, msg),
				ExitCode: ExitToolError,
			}
		}
		return &RequestError{
			Message:  fmt.Sprintf("Error: server error (%d). The server encountered an internal error.", status),
			ExitCode: ExitToolError,
		}
	default:
		if msg != "" {
			return &RequestError{
				Message:  fmt.Sprintf("Error: request failed (%d): %s", status, msg),
				ExitCode: ExitToolError,
			}
		}
		return &RequestError{
			Message:  fmt.Sprintf("Error: request failed (%d).", status),
			ExitCode: ExitToolError,
		}
	}
}
