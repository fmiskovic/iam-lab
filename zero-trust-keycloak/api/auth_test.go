package main

import (
	"crypto/rand"
	"crypto/rsa"
	"errors"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const (
	testIssuer = "http://localhost:8080/realms/demo"
	testKID    = "test-key"
)

var testKeys = sync.OnceValue(func() [2]*rsa.PrivateKey {
	var keys [2]*rsa.PrivateKey
	for i := range keys {
		key, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			panic(err)
		}
		keys[i] = key
	}
	return keys
})

func trustedKey() *rsa.PrivateKey   { return testKeys()[0] }
func untrustedKey() *rsa.PrivateKey { return testKeys()[1] }

// testKeyfunc plays the realm's JWKS: it knows exactly one key ID.
func testKeyfunc(token *jwt.Token) (any, error) {
	if kid, _ := token.Header["kid"].(string); kid != testKID {
		return nil, errors.New("key not found")
	}
	return &trustedKey().PublicKey, nil
}

func testConfig(name string) config {
	return config{Name: name, Issuer: testIssuer, AllowedAZP: []string{"order-api"}, Leeway: 5 * time.Second}
}

// accessToken returns claims like Keycloak's exchanged token for audience, with roles of that client.
func accessToken(audience string, roles ...string) accessClaims {
	now := time.Now()
	return accessClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    testIssuer,
			Subject:   "3f1c6e2a-alice",
			Audience:  jwt.ClaimStrings{audience},
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(30 * time.Second)),
		},
		Type:              "Bearer",
		AuthorizedParty:   "order-api",
		PreferredUsername: "alice",
		ResourceAccess:    map[string]clientRoles{audience: {Roles: roles}},
	}
}

func signRS256(t *testing.T, key *rsa.PrivateKey, kid string, claims jwt.Claims) string {
	t.Helper()
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = kid
	signed, err := token.SignedString(key)
	if err != nil {
		t.Fatalf("signing token: %v", err)
	}
	return signed
}

func bearer(t *testing.T, claims accessClaims) string {
	return "Bearer " + signRS256(t, trustedKey(), testKID, claims)
}

func with(claims accessClaims, change func(*accessClaims)) accessClaims {
	changed := claims
	change(&changed)
	return changed
}

func TestVerify(t *testing.T) {
	v := newVerifier(testConfig(inventoryAPI), testKeyfunc)
	valid := accessToken(inventoryAPI, "stock:read")
	past := time.Now().Add(-time.Minute)

	hs256 := jwt.NewWithClaims(jwt.SigningMethodHS256, valid)
	hs256.Header["kid"] = testKID
	hs256Signed, err := hs256.SignedString([]byte("a-shared-secret-an-attacker-guessed"))
	if err != nil {
		t.Fatal(err)
	}
	unsigned, err := jwt.NewWithClaims(jwt.SigningMethodNone, valid).SignedString(jwt.UnsafeAllowNoneSignatureType)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name          string
		authorization string
		wantStatus    int // 0 means accepted
		wantInError   string
	}{
		{"valid access token", bearer(t, valid), 0, ""},
		{"scheme is case-insensitive", "bearer " + signRS256(t, trustedKey(), testKID, valid), 0, ""},
		{"aud as array with this API", bearer(t, with(valid, func(c *accessClaims) {
			c.Audience = jwt.ClaimStrings{paymentAPI, inventoryAPI}
		})), 0, ""},
		{"expired, but within leeway", bearer(t, with(valid, func(c *accessClaims) {
			c.ExpiresAt = jwt.NewNumericDate(time.Now().Add(-2 * time.Second))
		})), 0, ""},

		{"no header", "", http.StatusUnauthorized, "no bearer token"},
		{"basic auth", "Basic YWxpY2U6YWxpY2U=", http.StatusUnauthorized, "no bearer token"},
		{"empty bearer", "Bearer   ", http.StatusUnauthorized, "no bearer token"},
		{"malformed", "Bearer not.a.jwt", http.StatusUnauthorized, "token is malformed"},
		{"wrong audience", bearer(t, with(valid, func(c *accessClaims) {
			c.Audience = jwt.ClaimStrings{paymentAPI}
		})), http.StatusUnauthorized, "token has invalid audience"},
		{"no audience", bearer(t, with(valid, func(c *accessClaims) {
			c.Audience = nil
		})), http.StatusUnauthorized, "aud claim is required"},
		{"untrusted issuer", bearer(t, with(valid, func(c *accessClaims) {
			c.Issuer = "http://localhost:8080/realms/partner"
		})), http.StatusUnauthorized, "token has invalid issuer"},
		{"expired beyond leeway", bearer(t, with(valid, func(c *accessClaims) {
			c.ExpiresAt = jwt.NewNumericDate(past)
		})), http.StatusUnauthorized, "token is expired"},
		{"no expiry", bearer(t, with(valid, func(c *accessClaims) {
			c.ExpiresAt = nil
		})), http.StatusUnauthorized, "exp claim is required"},
		{"not valid yet", bearer(t, with(valid, func(c *accessClaims) {
			c.NotBefore = jwt.NewNumericDate(time.Now().Add(time.Minute))
		})), http.StatusUnauthorized, "token is not valid yet"},
		{"issued in the future", bearer(t, with(valid, func(c *accessClaims) {
			c.IssuedAt = jwt.NewNumericDate(time.Now().Add(time.Minute))
		})), http.StatusUnauthorized, "token used before issued"},
		{"key the API doesn't trust", "Bearer " + signRS256(t, untrustedKey(), "other-realm-key", valid),
			http.StatusUnauthorized, "key not found"},
		{"forged with the trusted key ID", "Bearer " + signRS256(t, untrustedKey(), testKID, valid),
			http.StatusUnauthorized, "signature is invalid"},
		{"HS256 instead of RS256", "Bearer " + hs256Signed, http.StatusUnauthorized, "signing method HS256 is invalid"},
		{"alg none", "Bearer " + unsigned, http.StatusUnauthorized, "signing method none is invalid"},
		{"ID token", bearer(t, with(valid, func(c *accessClaims) {
			c.Type = "ID"
		})), http.StatusUnauthorized, `not an access token (typ "ID")`},
		{"requested by another client", bearer(t, with(valid, func(c *accessClaims) {
			c.AuthorizedParty = "partner-app"
		})), http.StatusUnauthorized, `requested by "partner-app"`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			claims, authErr := v.verify(tt.authorization)
			if tt.wantStatus == 0 {
				if authErr != nil {
					t.Fatalf("want accepted, got %d: %s", authErr.status, authErr.description)
				}
				if claims.PreferredUsername != "alice" {
					t.Fatalf("claims not decoded: %+v", claims)
				}
				return
			}
			if authErr == nil {
				t.Fatalf("want %d, token was accepted", tt.wantStatus)
			}
			if authErr.status != tt.wantStatus || !strings.Contains(authErr.description, tt.wantInError) {
				t.Fatalf("want %d containing %q, got %d: %s", tt.wantStatus, tt.wantInError, authErr.status, authErr.description)
			}
		})
	}
}

func TestVerifyReportsEveryFailedCheckOnOneLine(t *testing.T) {
	v := newVerifier(testConfig(inventoryAPI), testKeyfunc)
	token := with(accessToken(paymentAPI), func(c *accessClaims) {
		c.ExpiresAt = jwt.NewNumericDate(time.Now().Add(-time.Minute))
	})

	_, err := v.verify(bearer(t, token))
	if err == nil {
		t.Fatal("want rejected")
	}
	for _, want := range []string{"token is expired", "token has invalid audience"} {
		if !strings.Contains(err.description, want) {
			t.Errorf("want %q in %q", want, err.description)
		}
	}
	if strings.Contains(err.description, "\n") {
		t.Errorf("description spans lines: %q", err.description)
	}
}

func TestVerifyWithoutAZPAllowList(t *testing.T) {
	cfg := testConfig(inventoryAPI)
	cfg.AllowedAZP = nil
	v := newVerifier(cfg, testKeyfunc)

	token := with(accessToken(inventoryAPI, "stock:read"), func(c *accessClaims) { c.AuthorizedParty = "any-client" })
	if _, err := v.verify(bearer(t, token)); err != nil {
		t.Fatalf("want accepted without an azp allow-list, got %s", err.description)
	}
}

func TestRequireRole(t *testing.T) {
	v := newVerifier(testConfig(paymentAPI), testKeyfunc)

	tests := []struct {
		name   string
		claims accessClaims
		want   bool
	}{
		{"role of this API", accessToken(paymentAPI, "payment:create"), true},
		{"no roles", accessToken(paymentAPI), false},
		{"other role of this API", accessToken(paymentAPI, "payment:refund"), false},
		{"same role name, other client", accessToken(inventoryAPI, "payment:create"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := v.requireRole(tt.claims, "payment:create")
			if got := err == nil; got != tt.want {
				t.Fatalf("allowed = %v, want %v (%v)", got, tt.want, err)
			}
			if err != nil && (err.status != http.StatusForbidden || err.code != "insufficient_scope") {
				t.Fatalf("want 403 insufficient_scope, got %d %s", err.status, err.code)
			}
		})
	}
}
