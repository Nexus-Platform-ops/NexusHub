// Package auth issues and verifies the Nexus JWT tokens. Access tokens are
// short-lived (audience "access"); refresh tokens are longer-lived and carry
// audience "refresh" so they can never be replayed against API endpoints.
package auth

import (
	"errors"
	"strconv"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// AccessTTL and RefreshTTL are the token lifetimes used across the platform.
const (
	AccessTTL  = 24 * time.Hour
	RefreshTTL = 30 * 24 * time.Hour
)

// Claims is the single claim set used for both token types.
type Claims struct {
	jwt.RegisteredClaims
	Typ string `json:"typ"`
}

// JWT signs and verifies tokens with one HMAC key.
type JWT struct {
	key []byte
}

// New builds a signer from the configured secret.
func New(secret string) *JWT {
	return &JWT{key: []byte(secret)}
}

// Sign mints a token for userID with the given type ("access"/"refresh").
func (j *JWT) Sign(userID int64, typ string, ttl time.Duration) (string, error) {
	now := time.Now()
	claims := Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    "nexus",
			Subject:   strconv.FormatInt(userID, 10),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
		},
		Typ: typ,
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(j.key)
}

// Verify parses a token, enforces HMAC signing and the expected type.
func (j *JWT) Verify(tokenStr, wantTyp string) (*Claims, error) {
	tok, err := jwt.ParseWithClaims(
		tokenStr,
		&Claims{},
		func(t *jwt.Token) (any, error) {
			if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, errors.New("unexpected signing method")
			}
			return j.key, nil
		},
		jwt.WithValidMethods([]string{"HS256"}),
		jwt.WithIssuer("nexus"),
	)
	if err != nil || tok == nil || !tok.Valid {
		return nil, errors.New("invalid token")
	}
	claims, ok := tok.Claims.(*Claims)
	if !ok || claims.Typ != wantTyp {
		return nil, errors.New("invalid token type")
	}
	return claims, nil
}

// UserID extracts the numeric subject from verified claims.
func UserID(c *Claims) (int64, error) {
	return strconv.ParseInt(c.Subject, 10, 64)
}
