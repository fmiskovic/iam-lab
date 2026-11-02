package main

import (
	"fmt"
	"net/http"
	"slices"
	"strings"

	"github.com/golang-jwt/jwt/v5"
)

// accessClaims are the parts of a Keycloak access token the APIs read.
type accessClaims struct {
	jwt.RegisteredClaims
	Type              string                 `json:"typ"`
	AuthorizedParty   string                 `json:"azp"`
	PreferredUsername string                 `json:"preferred_username"`
	ResourceAccess    map[string]clientRoles `json:"resource_access"`
}

type clientRoles struct {
	Roles []string `json:"roles"`
}

func (c accessClaims) hasClientRole(client, role string) bool {
	return slices.Contains(c.ResourceAccess[client].Roles, role)
}

// authError is a rejected request: 401 for a missing or unacceptable token,
// 403 for an acceptable token that doesn't allow the operation.
type authError struct {
	status int
	// code is the RFC 6750 error code. It is empty when the request had no credentials.
	code        string
	description string
}

func (e *authError) Error() string { return e.description }

func invalidToken(description string) *authError {
	return &authError{status: http.StatusUnauthorized, code: "invalid_token", description: description}
}

func forbidden(description string) *authError {
	return &authError{status: http.StatusForbidden, code: "insufficient_scope", description: description}
}

// verifier decides whether a bearer token is acceptable at this API.
type verifier struct {
	audience   string
	allowedAZP []string
	keyfunc    jwt.Keyfunc
	parser     *jwt.Parser
}

func newVerifier(cfg config, keyfunc jwt.Keyfunc) *verifier {
	return &verifier{
		audience:   cfg.Name,
		allowedAZP: cfg.AllowedAZP,
		keyfunc:    keyfunc,
		parser: jwt.NewParser(
			jwt.WithValidMethods([]string{"RS256"}), // the realm's key algorithm, nothing else
			jwt.WithIssuer(cfg.Issuer),              // from configuration, never from the token
			jwt.WithAudience(cfg.Name),              // this API; a missing aud fails too
			jwt.WithExpirationRequired(),
			jwt.WithIssuedAt(),
			jwt.WithLeeway(cfg.Leeway),
		),
	}
}

// verify checks the bearer token in an Authorization header value and returns its claims.
// Lab: the descriptions name the failed check so you can see it. Production APIs often
// return less detail to the caller and log the rest.
func (v *verifier) verify(authorization string) (accessClaims, *authError) {
	scheme, raw, found := strings.Cut(authorization, " ")
	raw = strings.TrimSpace(raw)
	if !found || !strings.EqualFold(scheme, "Bearer") || raw == "" {
		return accessClaims{}, &authError{status: http.StatusUnauthorized, description: "no bearer token"}
	}

	var claims accessClaims
	if _, err := v.parser.ParseWithClaims(raw, &claims, v.keyfunc); err != nil {
		// golang-jwt reports every failed check, joined by newlines; keep them on one line.
		return accessClaims{}, invalidToken(strings.ReplaceAll(err.Error(), "\n", "; "))
	}
	// Keycloak sets typ "Bearer" in access tokens and "ID" in ID tokens.
	if claims.Type != "Bearer" {
		return accessClaims{}, invalidToken(fmt.Sprintf("not an access token (typ %q)", claims.Type))
	}
	if len(v.allowedAZP) > 0 && !slices.Contains(v.allowedAZP, claims.AuthorizedParty) {
		return accessClaims{}, invalidToken(fmt.Sprintf("token was requested by %q, which may not call this API", claims.AuthorizedParty))
	}
	return claims, nil
}

// requireRole checks a client role of this API, from resource_access.<audience>.roles.
func (v *verifier) requireRole(claims accessClaims, role string) *authError {
	if claims.hasClientRole(v.audience, role) {
		return nil
	}
	return forbidden(fmt.Sprintf("token lacks the %s role %s", v.audience, role))
}
