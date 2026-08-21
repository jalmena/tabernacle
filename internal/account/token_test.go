package account

import (
	"errors"
	"testing"
	"time"
)

var (
	testSecret  = []byte("0123456789abcdef0123456789abcdef")
	otherSecret = []byte("fedcba9876543210fedcba9876543210")
)

func TestTokenRoundTrip(t *testing.T) {
	u := User{ID: "abcd1234", Epoch: 3}
	now := time.Unix(1_700_000_000, 0)
	tok := Mint(testSecret, u, now.Add(time.Hour))

	c, err := Parse(testSecret, tok, now)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if c.ID != u.ID || c.Epoch != u.Epoch {
		t.Errorf("claims = %+v, want id %q epoch %d", c, u.ID, u.Epoch)
	}
	if !c.Expires.Equal(now.Add(time.Hour)) {
		t.Errorf("expiry = %v", c.Expires)
	}
}

// Everything about a token comes from outside, so every way of breaking one has
// to end in the same refusal.
func TestABrokenTokenIsRefused(t *testing.T) {
	u := User{ID: "abcd1234", Epoch: 1}
	now := time.Unix(1_700_000_000, 0)
	tok := Mint(testSecret, u, now.Add(time.Hour))

	tests := map[string]string{
		"empty":            "",
		"no separator":     "abcdef",
		"only a separator": ".",
		"signature gone":   tok[:len(tok)-1],
		"signature junk":   tok[:len(tok)-6] + "AAAAA",
		"payload changed":  "x" + tok[1:],
		"body only":        tok[:len(tok)-45],
		"not base64":       "!!!.!!!",
	}
	for name, bad := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := Parse(testSecret, bad, now); !errors.Is(err, ErrBadToken) {
				t.Errorf("Parse(%q) = %v, want ErrBadToken", bad, err)
			}
		})
	}

	t.Run("another secret", func(t *testing.T) {
		if _, err := Parse(otherSecret, tok, now); !errors.Is(err, ErrBadToken) {
			t.Error("a token signed with another key was accepted")
		}
	})
	t.Run("expired", func(t *testing.T) {
		if _, err := Parse(testSecret, tok, now.Add(2*time.Hour)); !errors.Is(err, ErrBadToken) {
			t.Error("an expired token was accepted")
		}
	})
	t.Run("expiring exactly now", func(t *testing.T) {
		edge := Mint(testSecret, u, now)
		if _, err := Parse(testSecret, edge, now); !errors.Is(err, ErrBadToken) {
			t.Error("a token expiring at this instant was accepted")
		}
	})
}

// A token carries no role, so it cannot assert one. Authority is read from the
// file on every request, which is what makes a demotion immediate.
func TestATokenCarriesNoAuthority(t *testing.T) {
	tok := Mint(testSecret, User{ID: "abcd1234", Role: RoleAdmin, Epoch: 1}, time.Now().Add(time.Hour))
	for _, word := range []string{"admin", string(RoleAdmin), "role"} {
		if contains(tok, word) {
			t.Errorf("the token mentions %q", word)
		}
	}
}

func contains(haystack, needle string) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}

// A session cookie is untrusted input on every request.
func FuzzParseToken(f *testing.F) {
	f.Add(Mint(testSecret, User{ID: "abcd1234", Epoch: 1}, time.Unix(2_000_000_000, 0)))
	f.Add("")
	f.Add(".")
	f.Add("v1|x|1|1.AAAA")

	now := time.Unix(1_700_000_000, 0)
	f.Fuzz(func(t *testing.T, token string) {
		c, err := Parse(testSecret, token, now)
		if err != nil {
			return
		}
		// Anything accepted must be internally sensible, or a later caller will
		// trust a field this one waved through.
		if c.ID == "" {
			t.Fatalf("%q was accepted with no id", token)
		}
		if !c.Expires.After(now) {
			t.Fatalf("%q was accepted although it expires at %v", token, c.Expires)
		}
		// And it must be exactly what we would have minted.
		if again := Mint(testSecret, User{ID: c.ID, Epoch: c.Epoch}, c.Expires); again != token {
			t.Fatalf("%q was accepted but is not what Mint produces (%q)", token, again)
		}
	})
}
