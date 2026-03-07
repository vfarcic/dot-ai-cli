package auth

import (
	"testing"
)

func TestGenerateCodeVerifier(t *testing.T) {
	v, err := GenerateCodeVerifier()
	if err != nil {
		t.Fatalf("GenerateCodeVerifier: %v", err)
	}
	// base64url of 32 bytes = 43 characters
	if len(v) != 43 {
		t.Errorf("verifier length = %d, want 43", len(v))
	}
	// Should be different each time.
	v2, _ := GenerateCodeVerifier()
	if v == v2 {
		t.Error("two verifiers should not be identical")
	}
}

func TestCodeChallenge(t *testing.T) {
	// Known test vector from RFC 7636 Appendix B.
	verifier := "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
	challenge := CodeChallenge(verifier)
	expected := "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM"
	if challenge != expected {
		t.Errorf("CodeChallenge = %q, want %q", challenge, expected)
	}
}

func TestMaskToken(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"short", "****"},
		{"exactly12ch", "****"},
		{"a-longer-token-value", "a-longer...alue"},
	}
	for _, tt := range tests {
		got := maskToken(tt.input)
		if got != tt.want {
			t.Errorf("maskToken(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
