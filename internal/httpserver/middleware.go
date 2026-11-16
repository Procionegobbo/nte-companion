package httpserver

import (
	"log"
	"net/http"
	"time"

	"github.com/Procionegobbo/nte-companion/internal/clientip"
)

// withClientIP resolves the caller's IP once per request and stores it in
// the context, so any handler downstream (notably the chat rate limiter)
// can read it without knowing about X-Forwarded-For at all.
func withClientIP(trustProxyHeaders bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := clientip.Extract(r, trustProxyHeaders)
			next.ServeHTTP(w, r.WithContext(clientip.WithIP(r.Context(), ip)))
		})
	}
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

// Flush lets the chat handler's SSE streaming keep working through this
// middleware: without it, wrapping the ResponseWriter would hide the
// underlying http.Flusher.
func (s *statusRecorder) Flush() {
	if f, ok := s.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func withLogging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		start := time.Now()
		next.ServeHTTP(rec, r)
		log.Printf("%s %s %d %s %s", r.Method, r.URL.Path, rec.status, time.Since(start), clientip.FromContext(r.Context()))
	})
}

func withRecover(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				log.Printf("panic: %v", rec)
				http.Error(w, "internal server error", http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}
