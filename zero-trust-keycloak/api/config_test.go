package main

import (
	"slices"
	"strings"
	"testing"
	"time"
)

func env(values map[string]string) func(string) string {
	return func(key string) string { return values[key] }
}

func validEnv() map[string]string {
	return map[string]string{
		"API_NAME":    "payment-api",
		"ISSUER":      "http://localhost:8080/realms/demo",
		"JWKS_URL":    "http://keycloak:8080/realms/demo/protocol/openid-connect/certs",
		"ALLOWED_AZP": " order-api, ,checkout-job ",
	}
}

func TestLoadConfig(t *testing.T) {
	cfg, err := loadConfig(env(validEnv()))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Name != paymentAPI || cfg.Addr != defaultAddr || cfg.Leeway != defaultLeeway {
		t.Fatalf("unexpected defaults: %+v", cfg)
	}
	if !slices.Equal(cfg.AllowedAZP, []string{"order-api", "checkout-job"}) {
		t.Fatalf("unexpected allow-list: %q", cfg.AllowedAZP)
	}

	custom := validEnv()
	custom["LEEWAY"] = "30s"
	custom["LISTEN_ADDR"] = ":9090"
	cfg, err = loadConfig(env(custom))
	if err != nil || cfg.Leeway != 30*time.Second || cfg.Addr != ":9090" {
		t.Fatalf("custom values not applied: %+v, %v", cfg, err)
	}
}

func TestLoadConfigRejects(t *testing.T) {
	tests := []struct {
		key, value, wantInError string
	}{
		{"API_NAME", "loyalty-api", "API_NAME"},
		{"ISSUER", "", "ISSUER"},
		{"ISSUER", "localhost:8080/realms/demo", "ISSUER"},
		{"JWKS_URL", "file:///etc/jwks.json", "JWKS_URL"},
		{"LEEWAY", "forever", "LEEWAY"},
		{"LEEWAY", "-1s", "LEEWAY"},
		{"LEEWAY", "1h", "LEEWAY"},
	}
	for _, tt := range tests {
		t.Run(tt.key+"="+tt.value, func(t *testing.T) {
			values := validEnv()
			values[tt.key] = tt.value
			if _, err := loadConfig(env(values)); err == nil || !strings.Contains(err.Error(), tt.wantInError) {
				t.Fatalf("want an error about %s, got %v", tt.wantInError, err)
			}
		})
	}
}
