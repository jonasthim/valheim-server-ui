package auth

import "testing"

func TestHashAndVerifyPassword(t *testing.T) {
	hash, err := HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if !VerifyPassword(hash, "correct horse battery staple") {
		t.Fatalf("expected password to verify")
	}
	if VerifyPassword(hash, "wrong password") {
		t.Fatalf("expected wrong password to fail")
	}
}

func TestHashPasswordFormat(t *testing.T) {
	hash, err := HashPassword("some-password-1234")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	want := "$argon2id$v=19$m=65536,t=3,p=4$"
	if len(hash) < len(want) || hash[:len(want)] != want {
		t.Fatalf("unexpected hash prefix: %s", hash)
	}
}

func TestHashPasswordUniqueSalt(t *testing.T) {
	h1, err := HashPassword("same-password")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	h2, err := HashPassword("same-password")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if h1 == h2 {
		t.Fatalf("expected distinct salts to produce distinct hashes")
	}
	if !VerifyPassword(h1, "same-password") || !VerifyPassword(h2, "same-password") {
		t.Fatalf("both hashes should verify")
	}
}

func TestVerifyPasswordMalformedHash(t *testing.T) {
	cases := []string{
		"",
		"not-a-hash",
		"$argon2id$v=19$m=65536,t=3,p=4$badbase64$$",
		"$bcrypt$v=19$m=65536,t=3,p=4$c2FsdA$aGFzaA",
		"$argon2id$v=19$m=notanumber,t=3,p=4$c2FsdA$aGFzaA",
	}
	for _, c := range cases {
		if VerifyPassword(c, "anything") {
			t.Fatalf("expected malformed hash %q to fail verification", c)
		}
	}
}
