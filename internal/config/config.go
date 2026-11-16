// Package config loads the server's runtime configuration from environment
// variables. There is no config file and no flags: every setting is an env
// var with a sensible default, except the OpenRouter API key, which has
// none on purpose — the server refuses to start without it.
package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

type Config struct {
	Port    string
	BaseURL string // informational only, e.g. for the "powered by" line

	OpenRouterAPIKey  string
	OpenRouterModel   string
	OpenRouterBaseURL string
	MaxTokens         int
	RequestTimeout    time.Duration

	RateLimitPerMinute int
	RateLimitPerDay    int
	TrustProxyHeaders  bool
}

// Load reads and validates the configuration. It fails hard if
// OPENROUTER_API_KEY is missing: a chat endpoint that always 500s because
// of a missing secret is worse than a server that never starts.
func Load() (Config, error) {
	cfg := Config{
		Port:               getEnv("PORT", "8080"),
		OpenRouterAPIKey:   os.Getenv("OPENROUTER_API_KEY"),
		OpenRouterModel:    getEnv("OPENROUTER_MODEL", "google/gemini-2.5-flash-lite"),
		OpenRouterBaseURL:  getEnv("OPENROUTER_BASE_URL", "https://openrouter.ai/api/v1"),
		RateLimitPerMinute: getEnvInt("RATE_LIMIT_PER_MINUTE", 10),
		RateLimitPerDay:    getEnvInt("RATE_LIMIT_PER_DAY", 80),
		MaxTokens:          getEnvInt("MAX_TOKENS", 700),
		TrustProxyHeaders:  getEnvBool("TRUST_PROXY_HEADERS", true),
	}
	cfg.RequestTimeout = time.Duration(getEnvInt("REQUEST_TIMEOUT_SECONDS", 60)) * time.Second

	if cfg.OpenRouterAPIKey == "" {
		return Config{}, fmt.Errorf("config: OPENROUTER_API_KEY non impostata")
	}
	if cfg.RateLimitPerMinute <= 0 {
		return Config{}, fmt.Errorf("config: RATE_LIMIT_PER_MINUTE deve essere positivo")
	}
	if cfg.RateLimitPerDay <= 0 {
		return Config{}, fmt.Errorf("config: RATE_LIMIT_PER_DAY deve essere positivo")
	}
	if cfg.MaxTokens <= 0 {
		return Config{}, fmt.Errorf("config: MAX_TOKENS deve essere positivo")
	}

	return cfg, nil
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}

func getEnvBool(key string, fallback bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return fallback
	}
	return b
}
