package account

import (
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// DefaultCost is the PBKDF2 iteration count for a new password.
//
// Six hundred thousand is the current OWASP figure for PBKDF2-HMAC-SHA256, and
// it costs the better part of a second on a Raspberry Pi. That is fine for a
// login and is why the web layer rate-limits attempts: an unauthenticated
// request that can be made to do this much work is a lever whatever its
// password-guessing value.
//
// It is a parameter rather than a constant in the hashing code because it is
// stored in every hash, so raising it later is one number and no migration.
const DefaultCost = 600_000

// MinPasswordRunes is the only rule about passwords.
//
// Length is the property that matters; composition rules mostly teach people to
// write Password1! and reuse it. Eight is the floor rather than a
// recommendation: the cost of a guess here is a PBKDF2 derivation at
// DefaultCost, and the login handler rations attempts, so the arithmetic that
// makes a short password dangerous elsewhere does not hold.
//
// It is exported so the sentence the reader sees can name it instead of
// repeating it in nineteen languages, where the number and the rule would
// eventually disagree.
const MinPasswordRunes = 8

const (
	hashAlgorithm = "pbkdf2-sha256"
	saltBytes     = 16
	keyBytes      = 32
)

var b64 = base64.RawStdEncoding

// hashPassword derives a storable hash, self-describing so that the parameters
// can change without anyone having to remember what they used to be.
func hashPassword(password string, cost int) (string, error) {
	if cost < 1 {
		return "", errors.New("account: the hashing cost must be positive")
	}
	salt := make([]byte, saltBytes)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("account: read random salt: %w", err)
	}
	key, err := pbkdf2.Key(sha256.New, password, salt, cost, keyBytes)
	if err != nil {
		return "", fmt.Errorf("account: derive key: %w", err)
	}
	return hashAlgorithm + "$" + strconv.Itoa(cost) + "$" + b64.EncodeToString(salt) + "$" + b64.EncodeToString(key), nil
}

// verifyPassword reports whether the password produces this hash.
//
// A hash it cannot parse fails closed. A stored value that has been truncated or
// hand-edited into nonsense must never authenticate anybody, and returning an
// error here would tempt a caller into treating "broken" as "unknown".
func verifyPassword(encoded, password string) bool {
	alg, cost, salt, want, ok := splitHash(encoded)
	if !ok || alg != hashAlgorithm {
		return false
	}
	got, err := pbkdf2.Key(sha256.New, password, salt, cost, keyBytes)
	if err != nil {
		return false
	}
	return subtle.ConstantTimeCompare(got, want) == 1
}

// needsRehash reports whether a stored hash was made to weaker parameters than
// the ones in force now.
func needsRehash(encoded string, cost int) bool {
	alg, have, _, _, ok := splitHash(encoded)
	if !ok {
		return false // unparseable: it authenticates nobody, so nothing to upgrade
	}
	return alg != hashAlgorithm || have < cost
}

// burn spends what a verification spends, for a name that does not exist.
//
// Deriving a key with the same function and the same iteration count is the same
// work, which is the point: the time a login takes must not report whether the
// name was real. Comparing against a decoy hash would only work if the decoy were
// built at the same cost, and building one at startup would cost a second of
// every boot for nothing.
func burn(cost int) {
	_, _ = pbkdf2.Key(sha256.New, "", burnSalt, cost, keyBytes)
}

var burnSalt = func() []byte {
	b := make([]byte, saltBytes)
	if _, err := rand.Read(b); err != nil {
		panic("account: no randomness at startup: " + err.Error())
	}
	return b
}()

func splitHash(encoded string) (alg string, cost int, salt, key []byte, ok bool) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 4 {
		return "", 0, nil, nil, false
	}
	cost, err := strconv.Atoi(parts[1])
	if err != nil || cost < 1 {
		return "", 0, nil, nil, false
	}
	// A salt has to be long enough to be one.
	if salt, err = b64.DecodeString(parts[2]); err != nil || len(salt) < 8 {
		return "", 0, nil, nil, false
	}
	// The key has to be exactly the right length. Deriving to whatever length
	// happens to be stored would let a truncated file shorten the comparison, and
	// a hash cut down to one byte would then be guessable in two hundred and
	// fifty-six attempts.
	if key, err = b64.DecodeString(parts[3]); err != nil || len(key) != keyBytes {
		return "", 0, nil, nil, false
	}
	return parts[0], cost, salt, key, true
}
