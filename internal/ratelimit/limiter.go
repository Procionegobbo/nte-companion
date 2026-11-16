// Package ratelimit implements a per-IP request limiter kept entirely in
// memory: there is no database, so an abusive IP's counters simply reset if
// the process restarts. That's an accepted tradeoff for this project.
package ratelimit

import (
	"sync"
	"time"

	"golang.org/x/time/rate"
)

type entry struct {
	minute     *rate.Limiter
	dayCount   int
	dayResetAt time.Time
	lastSeen   time.Time
}

// Limiter enforces two combined limits per IP: a burst-friendly per-minute
// rate, and a rolling 24h cap that bounds a single IP's worst-case daily
// cost even if it stays just under the per-minute limit all day.
type Limiter struct {
	mu        sync.Mutex
	entries   map[string]*entry
	perMinute int
	perDay    int
	stop      chan struct{}
}

func New(perMinute, perDay int) *Limiter {
	l := &Limiter{
		entries:   make(map[string]*entry),
		perMinute: perMinute,
		perDay:    perDay,
		stop:      make(chan struct{}),
	}
	go l.cleanupLoop()
	return l
}

// Close stops the background cleanup goroutine. Mainly useful for tests.
func (l *Limiter) Close() {
	close(l.stop)
}

// Allow reports whether ip may make a request now. When it returns false,
// retryAfter is how long the caller should wait before trying again.
func (l *Limiter) Allow(ip string) (ok bool, retryAfter time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	e, found := l.entries[ip]
	if !found {
		e = &entry{
			minute:     rate.NewLimiter(rate.Limit(float64(l.perMinute)/60.0), l.perMinute),
			dayResetAt: now.Add(24 * time.Hour),
		}
		l.entries[ip] = e
	}
	e.lastSeen = now

	if now.After(e.dayResetAt) {
		e.dayCount = 0
		e.dayResetAt = now.Add(24 * time.Hour)
	}
	if e.dayCount >= l.perDay {
		return false, time.Until(e.dayResetAt)
	}
	if !e.minute.Allow() {
		return false, time.Second // the per-minute bucket refills continuously; a short backoff is enough
	}

	e.dayCount++
	return true, 0
}

// cleanupLoop periodically forgets IPs that haven't been seen in a while,
// so a long-running process (and a service with no auth, scanned by
// whatever passes by) doesn't grow its IP map without bound.
func (l *Limiter) cleanupLoop() {
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-l.stop:
			return
		case now := <-ticker.C:
			l.mu.Lock()
			for ip, e := range l.entries {
				if now.Sub(e.lastSeen) > 24*time.Hour {
					delete(l.entries, ip)
				}
			}
			l.mu.Unlock()
		}
	}
}
