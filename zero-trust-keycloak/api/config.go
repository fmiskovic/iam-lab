package main

import (
	"fmt"
	"net/url"
	"strings"
	"time"
)

const (
	inventoryAPI = "inventory-api"
	paymentAPI   = "payment-api"

	defaultAddr   = ":8080"
	defaultLeeway = 5 * time.Second
	maxLeeway     = 5 * time.Minute
)

// config is read from the environment once at startup.
type config struct {
	// Name is inventory-api or payment-api. It is also the only audience the API accepts.
	Name string
	Addr string
	// Issuer is compared with the token's iss claim. It is configured, never taken from the token.
	Issuer string
	// JWKSURL is where the realm's public signing keys are fetched. In Docker this is an
	// in-network address, while the issuer is the URL clients used to get their tokens.
	JWKSURL string
	// AllowedAZP lists the clients whose tokens are accepted (azp claim). Empty allows any client.
	AllowedAZP []string
	// Leeway is the clock-skew allowance for exp, nbf and iat.
	Leeway time.Duration
}

func loadConfig(getenv func(string) string) (config, error) {
	name := getenv("API_NAME")
	if name != inventoryAPI && name != paymentAPI {
		return config{}, fmt.Errorf("API_NAME must be %s or %s, got %q", inventoryAPI, paymentAPI, name)
	}

	issuer, err := requireURL(getenv, "ISSUER")
	if err != nil {
		return config{}, err
	}
	jwksURL, err := requireURL(getenv, "JWKS_URL")
	if err != nil {
		return config{}, err
	}

	leeway := defaultLeeway
	if raw := getenv("LEEWAY"); raw != "" {
		parsed, err := time.ParseDuration(raw)
		if err != nil || parsed < 0 || parsed > maxLeeway {
			return config{}, fmt.Errorf("LEEWAY must be a duration between 0s and %s, got %q", maxLeeway, raw)
		}
		leeway = parsed
	}

	addr := getenv("LISTEN_ADDR")
	if addr == "" {
		addr = defaultAddr
	}

	return config{
		Name:       name,
		Addr:       addr,
		Issuer:     issuer,
		JWKSURL:    jwksURL,
		AllowedAZP: splitList(getenv("ALLOWED_AZP")),
		Leeway:     leeway,
	}, nil
}

func requireURL(getenv func(string) string, key string) (string, error) {
	raw := getenv(key)
	u, err := url.Parse(raw)
	if raw == "" || err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "", fmt.Errorf("%s must be an absolute http(s) URL, got %q", key, raw)
	}
	return raw, nil
}

func splitList(raw string) []string {
	items := []string{}
	for item := range strings.SplitSeq(raw, ",") {
		if trimmed := strings.TrimSpace(item); trimmed != "" {
			items = append(items, trimmed)
		}
	}
	return items
}
