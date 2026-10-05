package auth

import (
	"strings"
	"testing"
)

// cheap keeps the tests fast; production uses DefaultArgon2.
var cheap = Argon2Params{Memory: 1024, Iterations: 1, Parallelism: 1, SaltLen: 16, KeyLen: 32}

func TestHashAndVerifyPassword(t *testing.T) {
	hash, err := HashPassword("correct horse battery", cheap)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(hash, "$argon2id$v=19$m=1024,t=1,p=1$") {
		t.Fatalf("unexpected hash format %q", hash)
	}
	other, _ := HashPassword("correct horse battery", cheap)
	if other == hash {
		t.Fatal("two hashes of the same password are identical: salt is not random")
	}

	tests := []struct {
		name     string
		password string
		encoded  string
		want     bool
		wantErr  bool
	}{
		{"correct", "correct horse battery", hash, true, false},
		{"wrong", "correct horse battery!", hash, false, false},
		{"empty", "", hash, false, false},
		{"default params still verify", "pw", mustHash(t, "pw", DefaultArgon2), true, false},
		{"not argon2id", "pw", "$argon2i$v=19$m=1024,t=1,p=1$c2FsdA$aGFzaA", false, true},
		{"too few parts", "pw", "$argon2id$v=19$m=1024", false, true},
		{"bad version", "pw", "$argon2id$v=1$m=1024,t=1,p=1$c2FsdA$aGFzaA", false, true},
		{"bad base64", "pw", "$argon2id$v=19$m=1024,t=1,p=1$!!$aGFzaA", false, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := VerifyPassword(tc.password, tc.encoded)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tc.wantErr)
			}
			if got != tc.want {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func mustHash(t *testing.T, pw string, p Argon2Params) string {
	t.Helper()
	h, err := HashPassword(pw, p)
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func TestValidation(t *testing.T) {
	emails := []struct {
		in, want string
		ok       bool
	}{
		{"Player@Example.com", "player@example.com", true},
		{"  a@b.co ", "a@b.co", true},
		{"no-at-sign", "", false},
		{"Name <a@b.co>", "", false},
		{"", "", false},
	}
	for _, tc := range emails {
		got, err := normalizeEmail(tc.in)
		if (err == nil) != tc.ok || got != tc.want {
			t.Errorf("normalizeEmail(%q) = %q, %v", tc.in, got, err)
		}
	}

	usernames := map[string]bool{
		"odin": true, "Loki_42": true, "ab": false, "has space": false,
		"emoji🙂": false, "a_very_long_username_x": false,
	}
	for in, ok := range usernames {
		if err := validateUsername(in); (err == nil) != ok {
			t.Errorf("validateUsername(%q) = %v, want ok=%v", in, err, ok)
		}
	}

	passwords := map[string]bool{
		"short": false, "ten chars!": true, strings.Repeat("x", 128): true, strings.Repeat("x", 129): false,
		"ééééééééé": false, "éééééééééé": true, // counted in characters, not bytes
	}
	for in, ok := range passwords {
		if err := validatePassword(in); (err == nil) != ok {
			t.Errorf("validatePassword(%q) = %v, want ok=%v", in, err, ok)
		}
	}
}

func TestUsernameBase(t *testing.T) {
	tests := []struct{ name, email, want string }{
		{"Freya Odinsdottir", "f@x.io", "FreyaOdinsdotti"},
		{"李", "thor.son@x.io", "thorson"},
		{"", "ab@x.io", "player"},
	}
	for _, tc := range tests {
		if got := usernameBase(tc.name, tc.email); got != tc.want {
			t.Errorf("usernameBase(%q, %q) = %q, want %q", tc.name, tc.email, got, tc.want)
		}
	}
}
