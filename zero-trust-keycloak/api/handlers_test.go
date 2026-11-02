package main

import (
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func serve(t *testing.T, name, method, path, authorization, body string, orders *orderBook) *httptest.ResponseRecorder {
	t.Helper()
	handler := newHandler(testConfig(name), newVerifier(testConfig(name), testKeyfunc), orders)
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if authorization != "" {
		req.Header.Set("Authorization", authorization)
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func decode(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("response is not JSON: %q", rec.Body.String())
	}
	return body
}

func TestInventory(t *testing.T) {
	tests := []struct {
		name          string
		path          string
		authorization string
		wantStatus    int
		wantInBody    string
	}{
		{"stock with stock:read", "/stock/SKU-1001", bearer(t, accessToken(inventoryAPI, "stock:read")), http.StatusOK, `"available":7`},
		{"unknown SKU", "/stock/SKU-9999", bearer(t, accessToken(inventoryAPI, "stock:read")), http.StatusNotFound, "unknown SKU"},
		{"no token", "/stock/SKU-1001", "", http.StatusUnauthorized, "no bearer token"},
		{"token for payment-api", "/stock/SKU-1001", bearer(t, accessToken(paymentAPI, "payment:create")), http.StatusUnauthorized, "invalid audience"},
		{"inventory token without the role", "/stock/SKU-1001", bearer(t, accessToken(inventoryAPI)), http.StatusForbidden, "lacks the inventory-api role stock:read"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := serve(t, inventoryAPI, http.MethodGet, tt.path, tt.authorization, "", newOrderBook())
			if rec.Code != tt.wantStatus || !strings.Contains(rec.Body.String(), tt.wantInBody) {
				t.Fatalf("want %d with %q, got %d: %s", tt.wantStatus, tt.wantInBody, rec.Code, rec.Body.String())
			}
		})
	}
}

func TestInventoryHasNoPaymentRoute(t *testing.T) {
	rec := serve(t, inventoryAPI, http.MethodPost, "/payments", bearer(t, accessToken(inventoryAPI, "stock:read")), `{"order_id":"42"}`, newOrderBook())
	if rec.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d", rec.Code)
	}
}

func TestPayments(t *testing.T) {
	alice := bearer(t, accessToken(paymentAPI, "payment:create"))
	bobWithoutRole := bearer(t, with(accessToken(paymentAPI), func(c *accessClaims) { c.PreferredUsername = "bob" }))

	tests := []struct {
		name          string
		authorization string
		body          string
		wantStatus    int
		wantInBody    string
	}{
		{"alice pays her order", alice, `{"order_id":"42"}`, http.StatusCreated, `"amount":"129.90"`},
		{"alice pays it again", alice, `{"order_id":"42"}`, http.StatusConflict, "already paid"},
		{"alice pays bob's order", alice, `{"order_id":"43"}`, http.StatusForbidden, "belongs to another user"},
		{"bob without payment:create", bobWithoutRole, `{"order_id":"43"}`, http.StatusForbidden, "lacks the payment-api role payment:create"},
		{"unknown order", alice, `{"order_id":"99"}`, http.StatusNotFound, "order not found"},
		{"amount in the body is rejected", alice, `{"order_id":"43","amount":"0.01"}`, http.StatusBadRequest, "body must be"},
		{"empty body", alice, ``, http.StatusBadRequest, "body must be"},
		{"inventory token", bearer(t, accessToken(inventoryAPI, "stock:read")), `{"order_id":"42"}`, http.StatusUnauthorized, "invalid audience"},
	}

	orders := newOrderBook() // shared: the second payment must see the first
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := serve(t, paymentAPI, http.MethodPost, "/payments", tt.authorization, tt.body, orders)
			if rec.Code != tt.wantStatus || !strings.Contains(rec.Body.String(), tt.wantInBody) {
				t.Fatalf("want %d with %q, got %d: %s", tt.wantStatus, tt.wantInBody, rec.Code, rec.Body.String())
			}
		})
	}
}

func TestAuthErrorsFollowRFC6750(t *testing.T) {
	noToken := serve(t, paymentAPI, http.MethodPost, "/payments", "", `{"order_id":"42"}`, newOrderBook())
	if got := noToken.Header().Get("WWW-Authenticate"); got != `Bearer realm="payment-api"` {
		t.Fatalf("no credentials must not carry an error code, got %q", got)
	}
	if _, hasCode := decode(t, noToken)["error"]; hasCode {
		t.Fatalf("no credentials must not carry an error code: %s", noToken.Body.String())
	}

	idToken := bearer(t, with(accessToken(paymentAPI, "payment:create"), func(c *accessClaims) { c.Type = "ID" }))
	rejected := serve(t, paymentAPI, http.MethodPost, "/payments", idToken, `{"order_id":"42"}`, newOrderBook())
	challenge := rejected.Header().Get("WWW-Authenticate")
	if !strings.Contains(challenge, `error="invalid_token"`) || !strings.Contains(challenge, `error_description="not an access token (typ 'ID')"`) {
		t.Fatalf("unexpected challenge: %q", challenge)
	}
	if body := decode(t, rejected); body["error"] != "invalid_token" {
		t.Fatalf("unexpected body: %v", body)
	}

	noRole := serve(t, paymentAPI, http.MethodPost, "/payments", bearer(t, accessToken(paymentAPI)), `{"order_id":"42"}`, newOrderBook())
	if !strings.Contains(noRole.Header().Get("WWW-Authenticate"), `error="insufficient_scope"`) {
		t.Fatalf("want insufficient_scope, got %q", noRole.Header().Get("WWW-Authenticate"))
	}
}

func TestHeaderSafe(t *testing.T) {
	if got := headerSafe("typ \"ID\" \\ \n é"); got != `typ 'ID' '  ` {
		t.Fatalf("got %q", got)
	}
}

func TestRequestID(t *testing.T) {
	handler := newHandler(testConfig(inventoryAPI), newVerifier(testConfig(inventoryAPI), testKeyfunc), newOrderBook())

	keep := httptest.NewRequest(http.MethodGet, "/stock/SKU-1001", nil)
	keep.Header.Set("X-Request-ID", "trace-123")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, keep)
	if got := rec.Header().Get("X-Request-ID"); got != "trace-123" {
		t.Fatalf("want the caller's request ID, got %q", got)
	}

	replace := httptest.NewRequest(http.MethodGet, "/stock/SKU-1001", nil)
	replace.Header.Set("X-Request-ID", "bad id\nwith newline")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, replace)
	if got := rec.Header().Get("X-Request-ID"); !safeRequestID.MatchString(got) || got == "bad id\nwith newline" {
		t.Fatalf("want a generated request ID, got %q", got)
	}
}

func TestHealthcheck(t *testing.T) {
	handler := newHandler(testConfig(inventoryAPI), newVerifier(testConfig(inventoryAPI), testKeyfunc), newOrderBook())
	server := httptest.NewServer(handler)
	defer server.Close()

	_, port, err := net.SplitHostPort(strings.TrimPrefix(server.URL, "http://"))
	if err != nil {
		t.Fatal(err)
	}
	if got := healthcheck(":" + port); got != 0 {
		t.Fatalf("healthy server: want 0, got %d", got)
	}
	if got := healthcheck("not-an-address"); got != 1 {
		t.Fatalf("bad address: want 1, got %d", got)
	}
	server.Close()
	if got := healthcheck(":" + port); got != 1 {
		t.Fatalf("stopped server: want 1, got %d", got)
	}
}
