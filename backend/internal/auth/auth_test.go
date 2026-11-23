package auth

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestIssueToken_VerifyToken_RoundTrips(t *testing.T) {
	secret := []byte("secret-a")
	token, err := IssueToken(secret, time.Hour)
	if err != nil {
		t.Fatalf("IssueToken: %v", err)
	}
	if err := VerifyToken(secret, token); err != nil {
		t.Errorf("VerifyToken() = %v, want nil", err)
	}
}

func TestVerifyToken_WrongSecretFails(t *testing.T) {
	token, err := IssueToken([]byte("secret-a"), time.Hour)
	if err != nil {
		t.Fatalf("IssueToken: %v", err)
	}
	if err := VerifyToken([]byte("secret-b"), token); !errors.Is(err, ErrInvalidToken) {
		t.Errorf("VerifyToken() = %v, want ErrInvalidToken", err)
	}
}

func TestVerifyToken_ExpiredTokenFails(t *testing.T) {
	secret := []byte("secret-a")
	token, err := IssueToken(secret, -1*time.Second)
	if err != nil {
		t.Fatalf("IssueToken: %v", err)
	}
	if err := VerifyToken(secret, token); !errors.Is(err, ErrInvalidToken) {
		t.Errorf("VerifyToken() = %v, want ErrInvalidToken", err)
	}
}

func TestVerifyToken_MalformedTokenFails(t *testing.T) {
	if err := VerifyToken([]byte("secret-a"), "not-a-jwt"); !errors.Is(err, ErrInvalidToken) {
		t.Errorf("VerifyToken() = %v, want ErrInvalidToken", err)
	}
}

func TestVerifyToken_TamperedPayloadFails(t *testing.T) {
	secret := []byte("secret-a")
	token, err := IssueToken(secret, time.Hour)
	if err != nil {
		t.Fatalf("IssueToken: %v", err)
	}
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		t.Fatalf("token has %d parts, want 3", len(parts))
	}
	payload := []byte(parts[1])
	// Flip one character (preserve base64url alphabet validity) so the
	// segment still decodes but the signature no longer matches.
	flipIndex := len(payload) / 2
	if payload[flipIndex] == 'A' {
		payload[flipIndex] = 'B'
	} else {
		payload[flipIndex] = 'A'
	}
	tampered := parts[0] + "." + string(payload) + "." + parts[2]
	if err := VerifyToken(secret, tampered); !errors.Is(err, ErrInvalidToken) {
		t.Errorf("VerifyToken() = %v, want ErrInvalidToken", err)
	}
}
