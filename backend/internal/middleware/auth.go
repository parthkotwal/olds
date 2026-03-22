// Package middleware provides Gin middleware for the Olds backend.
package middleware

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// UserIDKey is the key used to store the verified user ID in the Gin context.
// Handlers retrieve it with: userID, _ := c.Get(middleware.UserIDKey)
const UserIDKey = "userID"

// Auth returns a Gin middleware that validates Supabase-issued JWTs.
//
// How Supabase auth works with our Go backend:
//   - The frontend (React + @supabase/supabase-js) handles the OAuth flow.
//     After login, Supabase gives the browser a signed JWT.
//   - The frontend includes that token in the Authorization header:
//     "Authorization: Bearer <jwt>"
//   - This middleware verifies the token's signature using the shared JWT secret,
//     then extracts the "sub" claim (the user's UUID) and stores it in the context.
//   - Handlers can then read the user ID without worrying about auth logic.
//
// Supabase JWTs use HS256 (HMAC-SHA256). We verify them here without any
// external JWT library — the standard library provides all we need.
func Auth(jwtSecret string) gin.HandlerFunc {
	return func(c *gin.Context) {
		token := extractBearerToken(c)
		if token == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": "missing or malformed Authorization header",
			})
			return
		}

		claims, err := verifyJWT(token, jwtSecret)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": "invalid token: " + err.Error(),
			})
			return
		}

		// "sub" is the Supabase user UUID — stable across sessions and devices.
		// It is the canonical user identity in our system.
		sub, ok := claims["sub"].(string)
		if !ok || sub == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": "token missing sub claim",
			})
			return
		}

		// Store the user ID in the request context so downstream handlers
		// can access it without re-parsing the token.
		c.Set(UserIDKey, sub)
		c.Next()
	}
}

// extractBearerToken reads the raw JWT from the Authorization header.
// Returns "" if the header is absent, empty, or not a Bearer token.
func extractBearerToken(c *gin.Context) string {
	header := c.GetHeader("Authorization")
	if !strings.HasPrefix(header, "Bearer ") {
		return ""
	}
	return strings.TrimPrefix(header, "Bearer ")
}

// verifyJWT validates a HS256 JWT against the provided HMAC secret and returns its claims.
//
// HS256 verification steps:
//  1. Split the token into header, payload, and signature (dot-separated).
//  2. Check the header's "alg" field — we only accept HS256.
//  3. Recompute HMAC-SHA256(header + "." + payload, secret).
//  4. Compare with the decoded signature using constant-time comparison
//     (hmac.Equal prevents timing attacks).
//  5. Decode the base64url payload and unmarshal the JSON claims.
//  6. Check the "exp" claim to reject expired tokens.
//
// All inputs are base64url-encoded (RFC 4648 §5) without padding.
func verifyJWT(tokenString, secret string) (map[string]interface{}, error) {
	parts := strings.Split(tokenString, ".")
	if len(parts) != 3 {
		return nil, fmt.Errorf("malformed JWT: expected 3 parts, got %d", len(parts))
	}

	// ── 1. Decode and check the header ────────────────────────────────────────
	headerJSON, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, fmt.Errorf("decode JWT header: %w", err)
	}
	var header map[string]interface{}
	if err := json.Unmarshal(headerJSON, &header); err != nil {
		return nil, fmt.Errorf("parse JWT header: %w", err)
	}
	if alg, _ := header["alg"].(string); alg != "HS256" {
		return nil, fmt.Errorf("unsupported JWT algorithm: %q (expected HS256)", alg)
	}

	// ── 2. Verify the signature ────────────────────────────────────────────────
	// The signed input is always: base64url(header) + "." + base64url(payload).
	// This is the JWT spec (RFC 7519 §7.2).
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(parts[0] + "." + parts[1]))
	expected := mac.Sum(nil)

	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return nil, fmt.Errorf("decode JWT signature: %w", err)
	}

	// hmac.Equal uses constant-time comparison to prevent timing side-channel
	// attacks. A naive == comparison would leak information about how many bytes
	// matched before the mismatch — an attacker could exploit this to forge tokens.
	if !hmac.Equal(expected, sig) {
		return nil, fmt.Errorf("signature verification failed")
	}

	// ── 3. Decode the payload claims ──────────────────────────────────────────
	payloadJSON, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("decode JWT payload: %w", err)
	}
	var claims map[string]interface{}
	if err := json.Unmarshal(payloadJSON, &claims); err != nil {
		return nil, fmt.Errorf("parse JWT claims: %w", err)
	}

	// ── 4. Check token expiry ─────────────────────────────────────────────────
	// "exp" is a Unix timestamp (seconds since epoch). json.Unmarshal decodes
	// JSON numbers into float64 by default when the target is interface{}.
	if exp, ok := claims["exp"].(float64); ok {
		if time.Now().Unix() > int64(exp) {
			return nil, fmt.Errorf("token has expired")
		}
	}

	return claims, nil
}
