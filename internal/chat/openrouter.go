package chat

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

// ChatMessage is one turn of the conversation, in the OpenAI-compatible
// shape OpenRouter expects.
type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// UpstreamError wraps a non-200 response from OpenRouter.
type UpstreamError struct {
	StatusCode int
	Body       string
}

func (e *UpstreamError) Error() string {
	return fmt.Sprintf("openrouter: risposta %d: %s", e.StatusCode, e.Body)
}

// Client is a minimal, dependency-free client for OpenRouter's streaming
// chat completions endpoint. It exists instead of a generic SDK because the
// only thing this server needs — POST a message list, read an SSE stream of
// content deltas — is a handful of lines with net/http and bufio.Scanner.
type Client struct {
	httpClient  *http.Client
	apiKey      string
	baseURL     string
	models      []string // [0] is the preferred model; the rest are cost-ordered fallbacks
	maxTokens   int
	maxPriceIn  float64
	maxPriceOut float64
}

// NewClient builds a client that tries primaryModel first and falls back,
// in order, to fallbackModels if OpenRouter reports the current one
// rate-limited or down (see StreamChat). maxPriceIn/Out ($ per million
// tokens) are a hard ceiling passed to OpenRouter on every request, so a
// misconfigured fallback list can never silently get routed to an
// expensive provider.
func NewClient(apiKey, baseURL, primaryModel string, fallbackModels []string, maxTokens int, maxPriceIn, maxPriceOut float64, timeout time.Duration) *Client {
	models := append([]string{primaryModel}, fallbackModels...)
	return &Client{
		httpClient:  &http.Client{Timeout: timeout},
		apiKey:      apiKey,
		baseURL:     strings.TrimSuffix(baseURL, "/"),
		models:      models,
		maxTokens:   maxTokens,
		maxPriceIn:  maxPriceIn,
		maxPriceOut: maxPriceOut,
	}
}

type chatCompletionChunk struct {
	Model   string `json:"model"`
	Choices []struct {
		Delta struct {
			Content string `json:"content"`
		} `json:"delta"`
	} `json:"choices"`
}

// StreamChat sends systemPrompt + history to OpenRouter and invokes onDelta
// for every piece of assistant text as it arrives. It returns once the
// upstream stream ends (or ctx is cancelled, or onDelta returns an error).
func (c *Client) StreamChat(ctx context.Context, systemPrompt string, history []ChatMessage, onDelta func(string) error) error {
	messages := make([]ChatMessage, 0, len(history)+1)
	messages = append(messages, ChatMessage{Role: "system", Content: systemPrompt})
	messages = append(messages, history...)

	payload, err := json.Marshal(map[string]any{
		// A list rather than a single "model": if the preferred model's
		// provider is rate-limited or down, OpenRouter automatically tries
		// the next one in order instead of failing the request outright.
		"models":     c.models,
		"stream":     true,
		"max_tokens": c.maxTokens,
		"messages":   messages,
		"provider": map[string]any{
			// Prefer the cheapest provider for whichever model ends up
			// being tried, and keep automatic failover to other providers
			// of that same model on (it's the default, stated for clarity).
			"sort":            "price",
			"allow_fallbacks": true,
			"max_price": map[string]float64{
				"prompt":     c.maxPriceIn,
				"completion": c.maxPriceOut,
			},
		},
	})
	if err != nil {
		return fmt.Errorf("openrouter: encode request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/chat/completions", bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("openrouter: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Accept", "text/event-stream")
	// Optional but recommended by OpenRouter for attributing traffic to this app.
	req.Header.Set("HTTP-Referer", "https://github.com/Procionegobbo/nte-companion")
	req.Header.Set("X-Title", "Not the End - Companion")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("openrouter: request fallita: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return &UpstreamError{StatusCode: resp.StatusCode, Body: string(body)}
	}

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	loggedModel := false
	for scanner.Scan() {
		line := scanner.Text()
		data, ok := strings.CutPrefix(line, "data: ")
		if !ok {
			continue // blank lines and SSE comments (": keep-alive") land here
		}
		if data == "[DONE]" {
			return nil
		}

		var chunk chatCompletionChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			continue // ignore anything we don't recognize rather than aborting the stream
		}
		if !loggedModel && chunk.Model != "" {
			// Only worth a log line when the fallback chain actually kicked
			// in: that's the one thing an operator can't see from the
			// request itself.
			if chunk.Model != c.models[0] {
				log.Printf("chat: risposto da %q (fallback, preferito era %q)", chunk.Model, c.models[0])
			}
			loggedModel = true
		}
		for _, choice := range chunk.Choices {
			if choice.Delta.Content == "" {
				continue
			}
			if err := onDelta(choice.Delta.Content); err != nil {
				return err
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("openrouter: lettura stream: %w", err)
	}
	return nil
}
