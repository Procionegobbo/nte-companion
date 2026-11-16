// Package config loads the server's runtime configuration from environment
// variables. There is no config file and no flags: every setting is an env
// var with a sensible default, except the OpenRouter API key, which has
// none on purpose — the server refuses to start without it.
package config

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"
)

// defaultFallbackModels are tried in order, after the primary model, when
// it's unavailable or rate-limited. Picked from a real pricing comparison
// (see the architecture notes): both are the cheapest option within their
// own provider family and support a large enough context window for the
// full regolamento, so cost stays low even when OpenRouter has to fail
// over — and failing over across providers (Google, Anthropic, OpenAI)
// means one provider's outage can't take the chatbot down entirely.
//
// OpenRouter caps the request's "models" array at 3 entries total, so this
// list can hold at most maxFallbackModels (2): primary + 2 fallbacks = 3.
const (
	defaultFallbackModels = "anthropic/claude-haiku-5.5,openai/gpt-4o-mini"
	maxFallbackModels     = 2
)

type Config struct {
	Port    string
	BaseURL string // informational only, e.g. for the "powered by" line

	OpenRouterAPIKey      string
	OpenRouterModel       string
	OpenRouterFallbacks   []string
	OpenRouterBaseURL     string
	OpenRouterMaxPriceIn  float64 // $/M input tokens, safety ceiling across every model/provider tried
	OpenRouterMaxPriceOut float64 // $/M output tokens, same purpose
	MaxTokens             int
	RequestTimeout        time.Duration

	RateLimitPerMinute int
	RateLimitPerDay    int
	TrustProxyHeaders  bool
}

// Load reads and validates the configuration. It fails hard if
// OPENROUTER_API_KEY is missing: a chat endpoint that always 500s because
// of a missing secret is worse than a server that never starts.
func Load() (Config, error) {
	cfg := Config{
		Port:                  getEnv("PORT", "8080"),
		OpenRouterAPIKey:      os.Getenv("OPENROUTER_API_KEY"),
		OpenRouterModel:       getEnv("OPENROUTER_MODEL", "google/gemini-2.5-flash-lite"),
		OpenRouterFallbacks:   splitCSV(getEnv("OPENROUTER_FALLBACK_MODELS", defaultFallbackModels)),
		OpenRouterBaseURL:     getEnv("OPENROUTER_BASE_URL", "https://openrouter.ai/api/v1"),
		OpenRouterMaxPriceIn:  getEnvFloat("OPENROUTER_MAX_PRICE_PROMPT", 0.50),
		OpenRouterMaxPriceOut: getEnvFloat("OPENROUTER_MAX_PRICE_COMPLETION", 2.00),
		RateLimitPerMinute:    getEnvInt("RATE_LIMIT_PER_MINUTE", 10),
		RateLimitPerDay:       getEnvInt("RATE_LIMIT_PER_DAY", 80),
		MaxTokens:             getEnvInt("MAX_TOKENS", 700),
		TrustProxyHeaders:     getEnvBool("TRUST_PROXY_HEADERS", true),
	}
	cfg.RequestTimeout = time.Duration(getEnvInt("REQUEST_TIMEOUT_SECONDS", 60)) * time.Second

	if len(cfg.OpenRouterFallbacks) > maxFallbackModels {
		log.Printf("config: OPENROUTER_FALLBACK_MODELS ha %d modelli, OpenRouter accetta al massimo %d in totale (primario + fallback): tengo solo i primi %d",
			len(cfg.OpenRouterFallbacks), maxFallbackModels+1, maxFallbackModels)
		cfg.OpenRouterFallbacks = cfg.OpenRouterFallbacks[:maxFallbackModels]
	}

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

func getEnvFloat(key string, fallback float64) float64 {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return fallback
	}
	return f
}

// splitCSV turns "a, b ,c" into ["a","b","c"], dropping empty entries so an
// explicitly empty env var means "no fallbacks" rather than one blank model.
func splitCSV(s string) []string {
	var out []string
	for _, part := range strings.Split(s, ",") {
		if v := strings.TrimSpace(part); v != "" {
			out = append(out, v)
		}
	}
	return out
}
