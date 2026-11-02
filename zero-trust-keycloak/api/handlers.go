package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
)

// protectedHandler runs only after the token was accepted and the role check passed.
type protectedHandler func(w http.ResponseWriter, r *http.Request, claims accessClaims)

func newHandler(cfg config, v *verifier, orders *orderBook) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	switch cfg.Name {
	case inventoryAPI:
		mux.Handle("GET /stock/{sku}", v.protect("stock:read", getStock))
	case paymentAPI:
		mux.Handle("POST /payments", v.protect("payment:create", createPayment(orders)))
	}
	return withRequestLog(cfg.Name, mux)
}

// protect accepts a request only with a valid token for this API that carries the role.
// Every route to the operation goes through it; there is no "internal" bypass.
func (v *verifier) protect(role string, next protectedHandler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims, err := v.verify(r.Header.Get("Authorization"))
		if err == nil {
			err = v.requireRole(claims, role)
		}
		if err != nil {
			writeAuthError(w, v.audience, err)
			return
		}
		next(w, r, claims)
	})
}

var stockLevels = map[string]int{"SKU-1001": 7, "SKU-1002": 0}

func getStock(w http.ResponseWriter, r *http.Request, _ accessClaims) {
	sku := r.PathValue("sku")
	available, ok := stockLevels[sku]
	if !ok {
		writeError(w, http.StatusNotFound, "not_found", "unknown SKU "+sku)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"sku": sku, "available": available})
}

// createPayment checks what the token can't: who owns the order and whether it's still open.
func createPayment(orders *orderBook) protectedHandler {
	return func(w http.ResponseWriter, r *http.Request, claims accessClaims) {
		var req struct {
			OrderID string `json:"order_id"`
		}
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<10))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&req); err != nil || req.OrderID == "" {
			writeError(w, http.StatusBadRequest, "invalid_request", `body must be {"order_id": "<id>"}`)
			return
		}

		paid, err := orders.pay(req.OrderID, claims.PreferredUsername)
		switch {
		case err == nil:
			writeJSON(w, http.StatusCreated, paid)
		case errors.Is(err, errOrderNotFound):
			writeError(w, http.StatusNotFound, "not_found", fmt.Sprintf("order %s: %v", req.OrderID, err))
		case errors.Is(err, errNotOrderOwner):
			writeError(w, http.StatusForbidden, "forbidden", fmt.Sprintf("order %s: %v", req.OrderID, err))
		case errors.Is(err, errOrderPaid):
			writeError(w, http.StatusConflict, "conflict", fmt.Sprintf("order %s: %v", req.OrderID, err))
		default:
			slog.Error("payment failed", "order_id", req.OrderID, "error", err)
			writeError(w, http.StatusInternalServerError, "server_error", "payment failed")
		}
	}
}

type errorBody struct {
	Error       string `json:"error,omitempty"`
	Description string `json:"error_description"`
}

// writeAuthError answers with an RFC 6750 WWW-Authenticate challenge.
func writeAuthError(w http.ResponseWriter, realm string, err *authError) {
	challenge := fmt.Sprintf(`Bearer realm="%s"`, realm)
	if err.code != "" {
		challenge += fmt.Sprintf(`, error="%s", error_description="%s"`, err.code, headerSafe(err.description))
	}
	w.Header().Set("WWW-Authenticate", challenge)
	writeError(w, err.status, err.code, err.description)
}

// headerSafe keeps a description within RFC 6750's character set: printable ASCII without " and \.
func headerSafe(s string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r == '"' || r == '\\':
			return '\''
		case r < 0x20 || r > 0x7e:
			return -1
		default:
			return r
		}
	}, s)
}

func writeError(w http.ResponseWriter, status int, code, description string) {
	setReason(w, description)
	writeJSON(w, status, errorBody{Error: code, Description: description})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		slog.Error("writing response failed", "error", err)
	}
}

// recorder captures what the request log needs from a response.
type recorder struct {
	http.ResponseWriter
	status int
	reason string
}

func (r *recorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

func setReason(w http.ResponseWriter, reason string) {
	if rec, ok := w.(*recorder); ok {
		rec.reason = reason
	}
}

var safeRequestID = regexp.MustCompile(`^[A-Za-z0-9-]{1,64}$`)

// withRequestLog logs one line per request with a correlation ID. It never logs the
// Authorization header: a bearer token in a log file is a credential in a log file.
func withRequestLog(api string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Request-ID")
		if !safeRequestID.MatchString(id) {
			id = newRequestID()
		}
		w.Header().Set("X-Request-ID", id)

		rec := &recorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		if r.URL.Path == "/healthz" {
			return
		}
		slog.Info("request", "api", api, "request_id", id, "method", r.Method, "path", r.URL.Path,
			"status", rec.status, "reason", rec.reason)
	})
}

func newRequestID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "unknown"
	}
	return hex.EncodeToString(b)
}
