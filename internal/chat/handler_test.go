package chat

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Procionegobbo/nte-companion/internal/ratelimit"
)

func fakeOpenRouter(t *testing.T, body string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(body))
	}))
}

func newTestHandler(t *testing.T, upstreamBody string) *Handler {
	t.Helper()
	srv := fakeOpenRouter(t, upstreamBody)
	t.Cleanup(srv.Close)

	client := NewClient("test-key", srv.URL, "test-model", 700, 5*time.Second)
	limiter := ratelimit.New(1000, 1000)
	t.Cleanup(limiter.Close)
	return NewHandler(client, "system prompt di test", limiter)
}

func TestHandlerStreamsDeltas(t *testing.T) {
	upstream := "data: {\"choices\":[{\"delta\":{\"content\":\"Ciao \"}}]}\n\n" +
		"data: {\"choices\":[{\"delta\":{\"content\":\"mondo\"}}]}\n\n" +
		"data: [DONE]\n\n"
	h := newTestHandler(t, upstream)

	req := httptest.NewRequest(http.MethodPost, "/api/chat", strings.NewReader(
		`{"messages":[{"role":"user","content":"ciao"}]}`))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	body := w.Body.String()
	if !strings.Contains(body, `event: delta`) || !strings.Contains(body, `"Ciao "`) {
		t.Errorf("atteso un evento delta con 'Ciao ', got: %s", body)
	}
	if !strings.Contains(body, `event: done`) {
		t.Errorf("atteso un evento done finale, got: %s", body)
	}
}

func TestHandlerRejectsEmptyHistory(t *testing.T) {
	h := newTestHandler(t, "data: [DONE]\n\n")
	req := httptest.NewRequest(http.MethodPost, "/api/chat", strings.NewReader(`{"messages":[]}`))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

func TestHandlerRejectsNonUserLastMessage(t *testing.T) {
	h := newTestHandler(t, "data: [DONE]\n\n")
	req := httptest.NewRequest(http.MethodPost, "/api/chat", strings.NewReader(
		`{"messages":[{"role":"user","content":"ciao"},{"role":"assistant","content":"risposta"}]}`))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

func TestHandlerTruncatesLongHistory(t *testing.T) {
	var messages []ChatMessage
	for i := 0; i < 30; i++ {
		role := "user"
		if i%2 == 1 {
			role = "assistant"
		}
		messages = append(messages, ChatMessage{Role: role, Content: "msg"})
	}
	if len(messages) > 0 {
		messages[len(messages)-1].Role = "user"
	}
	got, err := sanitizeHistory(messages)
	if err != nil {
		t.Fatalf("sanitizeHistory: %v", err)
	}
	if len(got) != maxHistoryMessages {
		t.Fatalf("len(got) = %d, want %d", len(got), maxHistoryMessages)
	}
}

func TestHandlerRateLimited(t *testing.T) {
	srv := fakeOpenRouter(t, "data: [DONE]\n\n")
	defer srv.Close()
	client := NewClient("test-key", srv.URL, "test-model", 700, 5*time.Second)
	limiter := ratelimit.New(1, 1000)
	defer limiter.Close()
	h := NewHandler(client, "system prompt", limiter)

	do := func() *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/api/chat", strings.NewReader(
			`{"messages":[{"role":"user","content":"ciao"}]}`))
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		return w
	}

	if w := do(); w.Code != http.StatusOK {
		t.Fatalf("prima richiesta: status = %d, want 200", w.Code)
	}
	w := do()
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("seconda richiesta: status = %d, want 429", w.Code)
	}
	if w.Header().Get("Retry-After") == "" {
		t.Errorf("atteso header Retry-After")
	}
}
