// Package middleware provides Gin middleware for the Olds backend.
package middleware

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
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
//   - This middleware verifies the token's signature, then extracts the "sub"
//     claim (the user's UUID) and stores it in the context.
//   - Handlers can then read the user ID without worrying about auth logic.
//
// Supabase originally issued HS256 tokens (symmetric HMAC with a shared
// secret). Newer Supabase projects issue ES256 tokens (asymmetric ECDSA with
// a P-256 key pair). This middleware supports both: it branches on the "alg"
// field in the JWT header. During a key rotation, both old (HS256) and new
// (ES256) tokens may be live simultaneously.
//
// ecKeys is the list of ECDSA public keys fetched from Supabase's JWKS
// endpoint at startup. jwtSecret is the legacy HS256 shared secret; it may
// be empty if the project has fully rotated to ES256.
func Auth(jwtSecret string, ecKeys []*ecdsa.PublicKey) gin.HandlerFunc {
	return func(c *gin.Context) {
		token := extractBearerToken(c)
		if token == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": "missing or malformed Authorization header",
			})
			return
		}

		claims, err := verifyJWT(token, jwtSecret, ecKeys)
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

// verifyJWT validates a JWT and returns its claims. It supports both HS256
// (legacy Supabase tokens, verified with an HMAC secret) and ES256 (current
// Supabase tokens, verified with an ECDSA P-256 public key).
//
// The algorithm is read from the JWT header's "alg" field and must match one
// of the supported algorithms — we never fall back silently.
func verifyJWT(tokenString, secret string, ecKeys []*ecdsa.PublicKey) (map[string]interface{}, error) {
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
	alg, _ := header["alg"].(string)

	// ── 2. Verify the signature based on the algorithm ────────────────────────
	// The signed input is always: base64url(header) + "." + base64url(payload).
	signingInput := parts[0] + "." + parts[1]

	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return nil, fmt.Errorf("decode JWT signature: %w", err)
	}

	switch alg {
	case "HS256":
		if err := verifyHS256(signingInput, sig, secret); err != nil {
			return nil, err
		}
	case "ES256":
		if err := verifyES256(signingInput, sig, ecKeys); err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("unsupported JWT algorithm: %q", alg)
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

// verifyHS256 checks a JWT signature using HMAC-SHA256.
//
// HMAC is symmetric — the same secret is used to both sign and verify. This
// is the legacy algorithm used by older Supabase projects.
//
// hmac.Equal uses constant-time comparison to prevent timing side-channel
// attacks. A naive == comparison would leak information about how many bytes
// matched before the mismatch — an attacker could exploit this to forge tokens.
func verifyHS256(signingInput string, sig []byte, secret string) error {
	if secret == "" {
		return fmt.Errorf("HS256 token received but SUPABASE_JWT_SECRET is not configured")
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(signingInput))
	expected := mac.Sum(nil)
	if !hmac.Equal(expected, sig) {
		return fmt.Errorf("signature verification failed")
	}
	return nil
}

// verifyES256 checks a JWT signature using ECDSA with P-256 and SHA-256.
//
// ES256 is asymmetric: Supabase signs tokens with its private key; we verify
// with the corresponding public key fetched from the JWKS endpoint. We never
// see the private key.
//
// JWT encodes the ES256 signature as raw R‖S (two concatenated 32-byte
// big-endian integers), not ASN.1 DER. We split the 64 bytes and pass them
// to ecdsa.Verify as *big.Int values.
//
// If multiple EC keys are provided (e.g. during a key rotation), the token is
// accepted if any key verifies it successfully.
func verifyES256(signingInput string, sig []byte, keys []*ecdsa.PublicKey) error {
	if len(keys) == 0 {
		return fmt.Errorf("ES256 token received but no ECDSA public keys are configured")
	}
	// ES256 signatures are always exactly 64 bytes: 32 for R, 32 for S.
	if len(sig) != 64 {
		return fmt.Errorf("ES256 signature must be 64 bytes, got %d", len(sig))
	}

	// Hash the signing input — ECDSA operates on the hash, not the raw bytes.
	digest := sha256.Sum256([]byte(signingInput))

	r := new(big.Int).SetBytes(sig[:32])
	s := new(big.Int).SetBytes(sig[32:])

	for _, key := range keys {
		if key.Curve == elliptic.P256() && ecdsa.Verify(key, digest[:], r, s) {
			return nil
		}
	}
	return fmt.Errorf("signature verification failed")
}

// jwk is the minimal subset of a JSON Web Key we need to reconstruct an
// ECDSA public key. We only care about EC keys on the P-256 curve (crv=P-256),
// which is what Supabase uses for ES256 tokens.
type jwk struct {
	Kty string `json:"kty"` // key type — must be "EC"
	Crv string `json:"crv"` // curve — must be "P-256"
	X   string `json:"x"`   // base64url-encoded X coordinate
	Y   string `json:"y"`   // base64url-encoded Y coordinate
}

// FetchJWKS retrieves the ECDSA public keys from Supabase's JWKS endpoint
// and returns them as a slice of *ecdsa.PublicKey. The endpoint is at:
//
//	{supabaseURL}/auth/v1/.well-known/jwks.json
//
// This should be called once at server startup and the result cached for the
// lifetime of the process. The JWKS contains all currently valid signing keys,
// including any keys being rotated out that are still needed to verify
// in-flight tokens.
func FetchJWKS(supabaseURL string) ([]*ecdsa.PublicKey, error) {
	url := strings.TrimRight(supabaseURL, "/") + "/auth/v1/.well-known/jwks.json"
	resp, err := http.Get(url) //nolint:noctx // startup call, no deadline needed
	if err != nil {
		return nil, fmt.Errorf("fetch JWKS from %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("JWKS endpoint returned HTTP %d", resp.StatusCode)
	}

	var body struct {
		Keys []jwk `json:"keys"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("decode JWKS response: %w", err)
	}

	var keys []*ecdsa.PublicKey
	for _, k := range body.Keys {
		if k.Kty != "EC" || k.Crv != "P-256" {
			continue // skip non-EC or non-P-256 keys
		}
		xBytes, err := base64.RawURLEncoding.DecodeString(k.X)
		if err != nil {
			return nil, fmt.Errorf("decode JWK X coordinate: %w", err)
		}
		yBytes, err := base64.RawURLEncoding.DecodeString(k.Y)
		if err != nil {
			return nil, fmt.Errorf("decode JWK Y coordinate: %w", err)
		}
		keys = append(keys, &ecdsa.PublicKey{
			Curve: elliptic.P256(),
			X:     new(big.Int).SetBytes(xBytes),
			Y:     new(big.Int).SetBytes(yBytes),
		})
	}

	return keys, nil
}
