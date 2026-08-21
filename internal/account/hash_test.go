package account

import "testing"

func TestHashRoundTrip(t *testing.T) {
	h, err := hashPassword(goodPassword, testCost)
	if err != nil {
		t.Fatalf("hashPassword: %v", err)
	}
	if !verifyPassword(h, goodPassword) {
		t.Error("the password does not verify against its own hash")
	}
	if verifyPassword(h, goodPassword+" ") {
		t.Error("a different password verified")
	}
	// Two hashes of the same password differ, because each has its own salt.
	other, err := hashPassword(goodPassword, testCost)
	if err != nil {
		t.Fatal(err)
	}
	if other == h {
		t.Error("two hashes of the same password are identical, so the salt is not random")
	}
}

// A stored hash that has been truncated, emptied or hand-edited into nonsense
// must authenticate nobody. Failing closed matters more here than anywhere else
// in the application.
func TestAnUnreadableHashAuthenticatesNobody(t *testing.T) {
	good, err := hashPassword(goodPassword, testCost)
	if err != nil {
		t.Fatal(err)
	}
	broken := []string{
		"", "$", "$$$", "x", "pbkdf2-sha256", "pbkdf2-sha256$2",
		"pbkdf2-sha256$2$$", "pbkdf2-sha256$0$c2FsdA$a2V5",
		"pbkdf2-sha256$-1$c2FsdA$a2V5", "pbkdf2-sha256$abc$c2FsdA$a2V5",
		"pbkdf2-sha256$2$!!!$a2V5", "pbkdf2-sha256$2$c2FsdA$!!!",
		"scrypt$2$c2FsdA$a2V5",
		// A truncated key must not shorten the comparison along with itself.
		good[:len(good)-4],
		good[:len(good)-40],
		good + "x",
	}
	for _, h := range broken {
		if verifyPassword(h, goodPassword) {
			t.Errorf("%q authenticated", h)
		}
		if verifyPassword(h, "") {
			t.Errorf("%q authenticated an empty password", h)
		}
	}
}

func TestNeedsRehash(t *testing.T) {
	weak, err := hashPassword(goodPassword, 2)
	if err != nil {
		t.Fatal(err)
	}
	if !needsRehash(weak, 100) {
		t.Error("a hash at a lower cost does not want upgrading")
	}
	if needsRehash(weak, 2) {
		t.Error("a hash at the current cost wants upgrading")
	}
	if needsRehash(weak, 1) {
		t.Error("a hash at a higher cost than we now use wants upgrading")
	}
	// Nothing to upgrade about a hash that works for nobody.
	if needsRehash("rubbish", 100) {
		t.Error("an unreadable hash wants upgrading")
	}
}

// verifyPassword takes whatever is in the file, so it takes anything.
//
// The seeds are deterministic on purpose: a fuzz worker is a separate process
// that re-runs this function, so a seed derived from fresh randomness would
// differ between the coordinator and the worker replaying it.
func FuzzVerifyPassword(f *testing.F) {
	f.Add("", "")
	f.Add("pbkdf2-sha256$1$c2FsdHNhbHRzYWx0c2FsdA$"+
		"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA", goodPassword)
	f.Add("pbkdf2-sha256$1$c2FsdA$a2V5", "x")

	f.Fuzz(func(t *testing.T, encoded, password string) {
		got := verifyPassword(encoded, password)

		// The same question twice has to give the same answer, or something is
		// reading uninitialised state.
		if again := verifyPassword(encoded, password); again != got {
			t.Fatalf("verifyPassword(%q, %q) answered %v then %v", encoded, password, got, again)
		}
		if !got {
			return
		}
		// Anything that verifies has to have been a hash of the shape we write.
		// Accepting something else means the parsing let a malformed stored value
		// through into the comparison.
		alg, cost, salt, key, ok := splitHash(encoded)
		if !ok {
			t.Fatalf("%q verified but does not parse", encoded)
		}
		if alg != hashAlgorithm {
			t.Fatalf("%q verified under the algorithm %q", encoded, alg)
		}
		if cost < 1 {
			t.Fatalf("%q verified at cost %d", encoded, cost)
		}
		if len(salt) < 8 {
			t.Fatalf("%q verified with a %d-byte salt", encoded, len(salt))
		}
		if len(key) != keyBytes {
			t.Fatalf("%q verified with a %d-byte key, want %d", encoded, len(key), keyBytes)
		}
	})
}
