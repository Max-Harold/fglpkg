package oauth

import (
	"testing"
)

func TestVerifierShape(t *testing.T) {
	for i := 0; i < 50; i++ {
		v, err := GenerateVerifier()
		if err != nil {
			t.Fatalf("GenerateVerifier: %v", err)
		}
		if !ValidVerifier(v) {
			t.Fatalf("verifier %q does not match RFC 7636 §4.1 charset", v)
		}
		if len(v) != 43 {
			t.Errorf("verifier length = %d, want 43 (from 32 random bytes)", len(v))
		}
	}
}

// RFC 7636 §4.6 worked example.
func TestChallengeForRFC7636Example(t *testing.T) {
	const (
		verifier = "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
		want     = "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM"
	)
	if got := ChallengeFor(verifier); got != want {
		t.Errorf("ChallengeFor(%q) = %q, want %q", verifier, got, want)
	}
}

func TestStateNonEmpty(t *testing.T) {
	a, _ := GenerateState()
	b, _ := GenerateState()
	if a == "" || b == "" {
		t.Fatal("state should not be empty")
	}
	if a == b {
		t.Fatal("two GenerateState calls returned the same value — bad randomness")
	}
}

func TestValidVerifierBoundaries(t *testing.T) {
	cases := []struct {
		v    string
		want bool
	}{
		{"", false},
		{"short", false},
		{repeat("a", 42), false},  // just under min
		{repeat("a", 43), true},   // min
		{repeat("a", 128), true},  // max
		{repeat("a", 129), false}, // just over max
		{"contains/slash" + repeat("a", 30), false},
	}
	for _, c := range cases {
		if got := ValidVerifier(c.v); got != c.want {
			t.Errorf("ValidVerifier(len=%d) = %v, want %v", len(c.v), got, c.want)
		}
	}
}

func repeat(s string, n int) string {
	out := make([]byte, 0, n*len(s))
	for i := 0; i < n; i++ {
		out = append(out, s...)
	}
	return string(out)
}
