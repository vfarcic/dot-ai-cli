package client

import "testing"

// TestRedactCredentials covers the credential-scrubbing regex used to keep an
// embedded git credential from leaking into a server-supplied message. The
// regression cases (PRD #16, Finding 2) are the percent-encoded forms and the
// username-only PAT URL, which a mandatory ":password@" group would let slip
// through.
func TestRedactCredentials(t *testing.T) {
	const redacted = "https://***:***@host/repo"
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"standard user:pass", "https://user:pass@host/repo", redacted},
		{"encoded chars, literal colon", "https://user%40name:pass%3Aword@host/repo", redacted},
		// Colon between user and pass is itself percent-encoded (%3A), so there
		// is no literal ":" — the old regex missed this and leaked the secret.
		{"encoded colon, no literal colon", "https://TOKEN%3ASECRET@host/repo", redacted},
		// Username-only PAT URL (no colon at all) — the documented residual.
		{"username-only PAT", "https://TOKEN@host/repo", redacted},
		// Idempotent: re-redacting an already-scrubbed string is a no-op.
		{"already redacted", redacted, redacted},
		// Embedded in free text, like a server error message.
		{"in free text", "clone failed for https://x:S3CR3T@github.com/orgA/skills: auth", "clone failed for https://***:***@github.com/orgA/skills: auth"},
		// Must NOT over-redact: an @ in a path/query is not userinfo.
		{"at-sign in query, no creds", "https://host/path?email=user@example.com", "https://host/path?email=user@example.com"},
		{"plain url, no creds", "https://api.example.com/users", "https://api.example.com/users"},
		{"no url at all", "just a plain message", "just a plain message"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := RedactCredentials(tc.in); got != tc.want {
				t.Errorf("RedactCredentials(%q) = %q, want %q", tc.in, got, tc.want)
			}
			// Idempotency: a second pass must not change the result.
			if got := RedactCredentials(RedactCredentials(tc.in)); got != tc.want {
				t.Errorf("RedactCredentials not idempotent for %q: %q", tc.in, got)
			}
		})
	}
}
