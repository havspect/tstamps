package api

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	loginLimit   = 10
	loginWindow  = time.Minute
	maxLimiterIP = 1024 // tracked addresses before expired buckets are pruned
)

// loginLimiter counts login attempts per IP in a fixed window. The zero value is ready.
type loginLimiter struct {
	mu      sync.Mutex
	buckets map[string]loginBucket
}

type loginBucket struct {
	start time.Time
	count int
}

// allow spends one attempt for ip and reports whether it was within the limit.
func (l *loginLimiter) allow(ip string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.buckets == nil {
		l.buckets = make(map[string]loginBucket)
	}
	b := l.buckets[ip]
	if now.Sub(b.start) >= loginWindow {
		b = loginBucket{start: now}
	}
	b.count++
	l.buckets[ip] = b
	if len(l.buckets) > maxLimiterIP {
		for k, v := range l.buckets {
			if now.Sub(v.start) >= loginWindow {
				delete(l.buckets, k)
			}
		}
	}
	return b.count <= loginLimit
}

// clientIP is the client address. Behind the deployed proxy the peer is
// localhost, so the last X-Forwarded-For hop (the one Caddy appended) wins;
// a remote peer cannot set that header for itself.
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	if !proxyPeer(r) {
		return host
	}
	forwarded := r.Header.Get("X-Forwarded-For")
	last := strings.TrimSpace(forwarded[strings.LastIndex(forwarded, ",")+1:])
	if net.ParseIP(last) != nil {
		return last
	}
	return host
}
