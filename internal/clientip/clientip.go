// Package clientip carries the caller's IP address through the request
// context. It's split out from internal/httpserver (which sets it) so that
// internal/chat (which reads it, for rate limiting) doesn't need to import
// the HTTP routing package just for this one value.
package clientip

import (
	"context"
	"net"
	"net/http"
	"strings"
)

type ctxKey struct{}

// Extract returns the caller's IP: the first address in X-Forwarded-For
// when trustProxyHeaders is true (the deploy target, Coolify/Traefik, is
// always the sole entrypoint in front of this service), falling back to
// RemoteAddr otherwise — which is also what a bare `go run` locally sees.
func Extract(r *http.Request, trustProxyHeaders bool) string {
	if trustProxyHeaders {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			first, _, _ := strings.Cut(xff, ",")
			return strings.TrimSpace(first)
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// WithIP returns a new context carrying ip.
func WithIP(ctx context.Context, ip string) context.Context {
	return context.WithValue(ctx, ctxKey{}, ip)
}

// FromContext returns the IP stored by WithIP, or "" if none was set.
func FromContext(ctx context.Context) string {
	ip, _ := ctx.Value(ctxKey{}).(string)
	return ip
}
