package chat

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
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
	httpClient *http.Client
	apiKey     string
	baseURL    string
	model      string
	maxTokens  int
}

func NewClient(apiKey, baseURL, model string, maxTokens int, timeout time.Duration) *Client {
	return &Client{
		httpClient: &http.Client{Timeout: timeout},
		apiKey:     apiKey,
		baseURL:    strings.TrimSuffix(baseURL, "/"),
		model:      model,
		maxTokens:  maxTokens,
	}
}

type chatCompletionChunk struct {
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
		"model":      c.model,
		"stream":     true,
		"max_tokens": c.maxTokens,
		"messages":   messages,
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
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return &UpstreamError{StatusCode: resp.StatusCode, Body: string(body)}
	}

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
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
