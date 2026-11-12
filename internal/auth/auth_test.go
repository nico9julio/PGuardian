package auth

import (
	"errors"
	"testing"
)

func TestHashAndAuthenticateWithRateLimit(t *testing.T) {
	hash, err := HashPassword("987654")
	if err != nil {
		t.Fatalf("HashPassword error: %v", err)
	}

	mgr := NewManager()
	tok, err := mgr.Authenticate("987654", hash)
	if err != nil || tok == "" {
		t.Fatalf("expected valid token, got err=%v", err)
	}
	if !mgr.ValidateToken(tok) {
		t.Fatalf("expected token to be valid")
	}

	// Trigger rate limiter after 5 bad attempts
	for i := 0; i < 5; i++ {
		_, err = mgr.Authenticate("0000", hash)
		if !errors.Is(err, ErrInvalidCredentials) {
			t.Fatalf("attempt %d expected ErrInvalidCredentials, got %v", i, err)
		}
	}
	_, err = mgr.Authenticate("987654", hash)
	if !errors.Is(err, ErrRateLimited) {
		t.Fatalf("expected ErrRateLimited after 5 failures, got %v", err)
	}
}
