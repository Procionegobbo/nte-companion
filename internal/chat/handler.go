package chat

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"

	"github.com/Procionegobbo/nte-companion/internal/clientip"
	"github.com/Procionegobbo/nte-companion/internal/ratelimit"
)

const (
	maxHistoryMessages = 20
	maxMessageChars    = 4000
)

// Handler serves POST /api/chat.
type Handler struct {
	client       *Client
	systemPrompt string
	limiter      *ratelimit.Limiter
}

func NewHandler(client *Client, systemPrompt string, limiter *ratelimit.Limiter) *Handler {
	return &Handler{client: client, systemPrompt: systemPrompt, limiter: limiter}
}

type chatRequest struct {
	Messages []ChatMessage `json:"messages"`
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeJSONError(w, http.StatusMethodNotAllowed, "method_not_allowed", "usa POST")
		return
	}

	ip := clientip.FromContext(r.Context())
	if ok, retryAfter := h.limiter.Allow(ip); !ok {
		seconds := int(retryAfter.Seconds())
		if seconds < 1 {
			seconds = 1
		}
		w.Header().Set("Retry-After", strconv.Itoa(seconds))
		writeJSONErrorWithField(w, http.StatusTooManyRequests, "rate_limited", "troppe richieste, riprova tra poco", "retry_after_seconds", seconds)
		return
	}

	var req chatRequest
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)) // 1MB hard cap on the whole body
	if err := dec.Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid_request", "corpo della richiesta non valido")
		return
	}
	history, err := sanitizeHistory(req.Messages)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeJSONError(w, http.StatusInternalServerError, "internal_error", "streaming non disponibile")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // disable buffering on proxies that honor it

	ctx := r.Context()
	err = h.client.StreamChat(ctx, h.systemPrompt, history, func(delta string) error {
		return writeSSE(w, flusher, "delta", map[string]string{"text": delta})
	})

	if err != nil {
		if errors.Is(err, context.Canceled) {
			return // the client went away; nothing left to tell it
		}
		log.Printf("chat: errore upstream: %v", err)
		_ = writeSSE(w, flusher, "error", map[string]string{
			"message": "Il servizio AI non è al momento disponibile. Riprova più tardi.",
		})
		return
	}

	_ = writeSSE(w, flusher, "done", map[string]string{})
}

// sanitizeHistory validates the client-supplied conversation and trims it
// to the last maxHistoryMessages entries: without this cap, a client could
// resend an ever-growing history and inflate both our cost and OpenRouter's.
func sanitizeHistory(messages []ChatMessage) ([]ChatMessage, error) {
	if len(messages) == 0 {
		return nil, errInvalid("la conversazione è vuota")
	}
	for _, m := range messages {
		if m.Role != "user" && m.Role != "assistant" {
			return nil, errInvalid("ruolo messaggio non consentito")
		}
		if m.Content == "" {
			return nil, errInvalid("un messaggio non può essere vuoto")
		}
		if len(m.Content) > maxMessageChars {
			return nil, errInvalid("messaggio troppo lungo")
		}
	}
	if messages[len(messages)-1].Role != "user" {
		return nil, errInvalid("l'ultimo messaggio deve essere dell'utente")
	}

	if len(messages) > maxHistoryMessages {
		messages = messages[len(messages)-maxHistoryMessages:]
	}
	return messages, nil
}

type validationError struct{ msg string }

func (e *validationError) Error() string { return e.msg }
func errInvalid(msg string) error        { return &validationError{msg} }

func writeSSE(w http.ResponseWriter, f http.Flusher, event string, data any) error {
	payload, err := json.Marshal(data)
	if err != nil {
		return err
	}
	if _, err := w.Write([]byte("event: " + event + "\ndata: " + string(payload) + "\n\n")); err != nil {
		return err
	}
	f.Flush()
	return nil
}

func writeJSONError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": code, "message": message})
}

func writeJSONErrorWithField(w http.ResponseWriter, status int, code, message, field string, value int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": code, "message": message, field: value})
}
