package telemetrybackend

import (
	"net"
	"net/http"
	"net/netip"
	"strings"
	"sync"
	"time"
)

type rateBucket struct {
	tokens   float64
	lastSeen time.Time
}

type ipRateLimiter struct {
	mu         sync.Mutex
	buckets    map[string]rateBucket
	perSecond  float64
	burst      float64
	maxBuckets int
	now        func() time.Time
}

func newIPRateLimiter(perMinute, burst int) *ipRateLimiter {
	return &ipRateLimiter{
		buckets:    make(map[string]rateBucket),
		perSecond:  float64(perMinute) / 60,
		burst:      float64(burst),
		maxBuckets: 10_000,
		now:        time.Now,
	}
}

func (l *ipRateLimiter) Allow(key string) bool {
	now := l.now()
	l.mu.Lock()
	defer l.mu.Unlock()

	bucket, ok := l.buckets[key]
	if !ok {
		l.cleanup(now)
		if len(l.buckets) >= l.maxBuckets {
			return false
		}
		bucket = rateBucket{tokens: l.burst, lastSeen: now}
	} else {
		elapsed := now.Sub(bucket.lastSeen).Seconds()
		bucket.tokens = min(l.burst, bucket.tokens+elapsed*l.perSecond)
		bucket.lastSeen = now
	}
	if bucket.tokens < 1 {
		l.buckets[key] = bucket
		l.cleanup(now)
		return false
	}
	bucket.tokens--
	l.buckets[key] = bucket
	l.cleanup(now)
	return true
}

func (l *ipRateLimiter) cleanup(now time.Time) {
	if len(l.buckets) < l.maxBuckets {
		return
	}
	for key, bucket := range l.buckets {
		if now.Sub(bucket.lastSeen) > 10*time.Minute {
			delete(l.buckets, key)
		}
	}
}

func clientIP(request *http.Request, trusted []netip.Prefix) string {
	remote := parseRemoteIP(request.RemoteAddr)
	if isTrustedProxy(remote, trusted) {
		for _, item := range strings.Split(request.Header.Get("X-Forwarded-For"), ",") {
			if addr, err := netip.ParseAddr(strings.TrimSpace(item)); err == nil {
				return addr.Unmap().String()
			}
		}
	}
	if remote.IsValid() {
		return remote.Unmap().String()
	}
	return "unknown"
}

func parseRemoteIP(remoteAddr string) netip.Addr {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		host = remoteAddr
	}
	addr, _ := netip.ParseAddr(strings.Trim(host, "[]"))
	return addr
}

func isTrustedProxy(addr netip.Addr, trusted []netip.Prefix) bool {
	if !addr.IsValid() {
		return false
	}
	for _, prefix := range trusted {
		if prefix.Contains(addr) {
			return true
		}
	}
	return false
}
