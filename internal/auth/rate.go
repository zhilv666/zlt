package auth

import (
	"sync"
	"time"
)

// loginLimiter caps failed login attempts per source IP. The plan calls for
// "每分钟 5 次失败尝试" per IP. Successful logins are not counted; only
// failures are, so a legitimate user who fat-fingers the key a couple of
// times is not penalised by their own later success.
type loginLimiter struct {
	mu      sync.Mutex
	window  time.Duration
	maxFail int
	hits    map[string][]time.Time
	now     func() time.Time
}

func newLoginLimiter(now func() time.Time) *loginLimiter {
	return &loginLimiter{
		window:  time.Minute,
		maxFail: 5,
		hits:    make(map[string][]time.Time),
		now:     now,
	}
}

// recordFailure appends a failed attempt for ip and returns whether the IP is
// now over the limit (caller should respond 429).
func (l *loginLimiter) recordFailure(ip string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now()
	cutoff := now.Add(-l.window)
	hits := l.hits[ip]

	// Drop entries outside the rolling window.
	kept := hits[:0]
	for _, t := range hits {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	kept = append(kept, now)
	l.hits[ip] = kept
	return len(kept) > l.maxFail
}

// reset clears failures for an IP after a successful login so a user who
// recovered is not left on the edge of the limit.
func (l *loginLimiter) reset(ip string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.hits, ip)
}

// sweep drops idle IP buckets so the map does not grow without bound.
func (l *loginLimiter) sweep() {
	l.mu.Lock()
	defer l.mu.Unlock()
	cutoff := l.now().Add(-l.window)
	for ip, hits := range l.hits {
		kept := hits[:0]
		for _, t := range hits {
			if t.After(cutoff) {
				kept = append(kept, t)
			}
		}
		if len(kept) == 0 {
			delete(l.hits, ip)
		} else {
			l.hits[ip] = kept
		}
	}
}
