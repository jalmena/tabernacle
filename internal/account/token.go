package account

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"strconv"
	"strings"
	"time"
)

// ErrBadToken reports a session token that is not one we issued, or is one we
// issued and no longer honour.
//
// It says nothing about which, on purpose: a caller's only sensible response to
// any of it is to treat the request as anonymous.
var ErrBadToken = errors.New("account: the session token is not valid")

// tokenVersion prefixes every payload so that a change of shape cannot be read
// as the old shape.
const tokenVersion = "v1"

var tokenEncoding = base64.RawURLEncoding

// Claims is what a session token asserts.
type Claims struct {
	ID      string
	Epoch   int
	Expires time.Time
}

// Mint signs a session token for a user.
//
// The token carries the user's immutable id rather than their name, so renaming
// an account does not log it out, and their epoch, which is what makes a
// password change take effect on every device at once. It carries no role:
// authority is read from the file on each request, so demoting somebody is
// immediate and a stolen token cannot assert a privilege it never had.
func Mint(secret []byte, u User, expires time.Time) string {
	payload := tokenVersion + "|" + u.ID + "|" + strconv.Itoa(u.Epoch) + "|" +
		strconv.FormatInt(expires.Unix(), 10)
	body := tokenEncoding.EncodeToString([]byte(payload))
	return body + "." + tokenEncoding.EncodeToString(sign(secret, body))
}

// Parse checks a token's signature and expiry and returns what it claims.
//
// The signature is verified before any field is read, so a forged payload never
// reaches the parsing code.
func Parse(secret []byte, token string, now time.Time) (Claims, error) {
	c, err := claims(secret, token)
	if err != nil {
		return Claims{}, err
	}
	if !now.Before(c.Expires) {
		return Claims{}, ErrBadToken
	}
	return c, nil
}

// claims verifies and decodes without judging the expiry, so that the caller
// deciding whether to renew a cookie and the caller deciding whether to trust it
// read the same fields the same way.
func claims(secret []byte, token string) (Claims, error) {
	body, tag, found := strings.Cut(token, ".")
	if !found {
		return Claims{}, ErrBadToken
	}
	given, ok := decodeExact(tag)
	if !ok || !hmac.Equal(given, sign(secret, body)) {
		return Claims{}, ErrBadToken
	}

	raw, ok := decodeExact(body)
	if !ok {
		return Claims{}, ErrBadToken
	}
	parts := strings.Split(string(raw), "|")
	if len(parts) != 4 || parts[0] != tokenVersion || parts[1] == "" {
		return Claims{}, ErrBadToken
	}
	epoch, err := strconv.Atoi(parts[2])
	if err != nil {
		return Claims{}, ErrBadToken
	}
	unix, err := strconv.ParseInt(parts[3], 10, 64)
	if err != nil {
		return Claims{}, ErrBadToken
	}
	return Claims{ID: parts[1], Epoch: epoch, Expires: time.Unix(unix, 0)}, nil
}

// decodeExact decodes base64 and insists the input was the canonical spelling of
// what came out.
//
// Go's decoder ignores newlines and, outside strict mode, tolerates unused bits
// in the final character, so several strings decode to one value. None of that is
// dangerous here — the signature is over the bytes — but there is no reason for a
// session to have more than one spelling, and a token that is not exactly what
// Mint produces is not a token we issued.
func decodeExact(s string) ([]byte, bool) {
	raw, err := tokenEncoding.DecodeString(s)
	if err != nil || tokenEncoding.EncodeToString(raw) != s {
		return nil, false
	}
	return raw, true
}

func sign(secret []byte, body string) []byte {
	m := hmac.New(sha256.New, secret)
	m.Write([]byte(body))
	return m.Sum(nil)
}
