// Package auth issues and verifies the single HS256 session JWT used to
// gate Frontend + API access (see docs/adr/0018). No third-party JWT
// library — the claim set is fixed (exp/iat only) and HS256 needs nothing
// beyond stdlib crypto/hmac + crypto/sha256, consistent with this repo's
// dependency-minimalism (no ORM, no DI container).
package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// jwtHeader is the fixed HS256/JWT header, base64url-encoded once at
// package init — every token this package issues uses the same header.
var jwtHeader = base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))

type claims struct {
	IssuedAt int64 `json:"iat"`
	Expires  int64 `json:"exp"`
}

// ErrInvalidToken is returned by VerifyToken for any malformed token, bad
// signature, or expired token — callers only need to distinguish
// "valid" from "not valid", never why.
var ErrInvalidToken = errors.New("invalid token")

// IssueToken returns a compact HS256 JWT valid for ttl, signed with secret.
func IssueToken(secret []byte, ttl time.Duration) (string, error) {
	now := time.Now()
	payload, err := json.Marshal(claims{IssuedAt: now.Unix(), Expires: now.Add(ttl).Unix()})
	if err != nil {
		return "", fmt.Errorf("marshal claims: %w", err)
	}
	signingInput := jwtHeader + "." + base64.RawURLEncoding.EncodeToString(payload)
	return signingInput + "." + sign(secret, signingInput), nil
}

// VerifyToken reports whether token has a valid signature under secret and
// has not expired. Returns ErrInvalidToken for any failure.
func VerifyToken(secret []byte, token string) error {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return ErrInvalidToken
	}
	signingInput := parts[0] + "." + parts[1]
	if !hmac.Equal([]byte(sign(secret, signingInput)), []byte(parts[2])) {
		return ErrInvalidToken
	}
	rawPayload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return ErrInvalidToken
	}
	var decoded claims
	if err := json.Unmarshal(rawPayload, &decoded); err != nil {
		return ErrInvalidToken
	}
	if time.Now().Unix() >= decoded.Expires {
		return ErrInvalidToken
	}
	return nil
}

func sign(secret []byte, signingInput string) string {
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(signingInput))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
