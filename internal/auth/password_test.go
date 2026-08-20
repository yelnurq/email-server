package auth

import "testing"

func TestHashAndVerifyPassword(t *testing.T) {
	hash, err := HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	if err := VerifyPassword("correct horse battery staple", hash); err != nil {
		t.Fatalf("verify correct password: %v", err)
	}
	if err := VerifyPassword("wrong password", hash); err != ErrPasswordMismatch {
		t.Fatalf("wrong password: got %v, want ErrPasswordMismatch", err)
	}
}

func TestVerifyPasswordRejectsMalformed(t *testing.T) {
	for _, encoded := range []string{
		"", "plaintext", "$bcrypt$whatever", "$argon2id$v=19$m=65536,t=3,p=4$!!!$###",
	} {
		if err := VerifyPassword("x", encoded); err == nil {
			t.Fatalf("malformed hash %q accepted", encoded)
		}
	}
}

func TestDummyHashIsParseable(t *testing.T) {
	// The timing-equalization hash must parse (and fail only on comparison).
	if err := VerifyPassword("anything", dummyHash); err != ErrPasswordMismatch {
		t.Fatalf("dummyHash: got %v, want ErrPasswordMismatch", err)
	}
}
