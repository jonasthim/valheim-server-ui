package auth

import (
	"strings"
	"sync"
	"time"
)

// Lockout policy (ARCHITECTURE.md §13): 5 failed attempts for one username
// lock that username for 5 minutes. A second, looser counter per client IP
// (MaxLoginFailuresPerIP within LockoutDuration) stops one source from
// hammering many usernames, and a bounded map with periodic sweeping keeps an
// unauthenticated client from growing the table without limit.
const (
	MaxLoginFailures      = 5
	MaxLoginFailuresPerIP = 30
	LockoutDuration       = 5 * time.Minute
	maxLockoutEntries     = 10_000
)

// Clock abstracts time.Now for deterministic tests.
type Clock func() time.Time

type lockoutState struct {
	failures    int
	lastFailure time.Time
	lockedUntil time.Time
}

// Lockout tracks failed local-login attempts per username and per client IP,
// in memory. It is safe for concurrent use.
type Lockout struct {
	mu       sync.Mutex
	now      Clock
	attempts map[string]*lockoutState
}

// NewLockout constructs a Lockout. A nil clock uses time.Now.
func NewLockout(clock Clock) *Lockout {
	if clock == nil {
		clock = time.Now
	}
	return &Lockout{now: clock, attempts: make(map[string]*lockoutState)}
}

func userKey(username string) string { return "u:" + strings.ToLower(username) }
func ipKey(ip string) string         { return "ip:" + ip }

// Locked reports whether username is currently locked out, and until when.
// An expired lock is cleared as a side effect.
func (l *Lockout) Locked(username string) (bool, time.Time) {
	return l.locked(userKey(username))
}

// LockedIP reports whether the client address is currently locked out.
func (l *Lockout) LockedIP(ip string) (bool, time.Time) {
	if ip == "" {
		return false, time.Time{}
	}
	return l.locked(ipKey(ip))
}

func (l *Lockout) locked(key string) (bool, time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()
	st, ok := l.attempts[key]
	if !ok || st.lockedUntil.IsZero() {
		return false, time.Time{}
	}
	now := l.now()
	if now.Before(st.lockedUntil) {
		return true, st.lockedUntil
	}
	delete(l.attempts, key)
	return false, time.Time{}
}

// RecordFailure increments the failure count for username (locking it once
// MaxLoginFailures is reached) and, when ip is non-empty, for the client
// address (locking it at MaxLoginFailuresPerIP).
func (l *Lockout) RecordFailure(username, ip string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	l.sweepLocked(now)
	l.recordLocked(userKey(username), MaxLoginFailures, now)
	if ip != "" {
		l.recordLocked(ipKey(ip), MaxLoginFailuresPerIP, now)
	}
}

func (l *Lockout) recordLocked(key string, limit int, now time.Time) {
	st, ok := l.attempts[key]
	if !ok {
		if len(l.attempts) >= maxLockoutEntries {
			l.evictOldestLocked()
		}
		st = &lockoutState{}
		l.attempts[key] = st
	}
	st.failures++
	st.lastFailure = now
	if st.failures >= limit {
		st.lockedUntil = now.Add(LockoutDuration)
	}
}

// sweepLocked drops entries whose failures and lock are both older than
// LockoutDuration, so a burst of random usernames does not pin memory.
func (l *Lockout) sweepLocked(now time.Time) {
	for k, st := range l.attempts {
		if now.Sub(st.lastFailure) > LockoutDuration && !now.Before(st.lockedUntil) {
			delete(l.attempts, k)
		}
	}
}

func (l *Lockout) evictOldestLocked() {
	var oldestKey string
	var oldest time.Time
	for k, st := range l.attempts {
		if oldestKey == "" || st.lastFailure.Before(oldest) {
			oldestKey, oldest = k, st.lastFailure
		}
	}
	if oldestKey != "" {
		delete(l.attempts, oldestKey)
	}
}

// Reset clears all failure state for username (called on successful login
// and by the break-glass password reset).
func (l *Lockout) Reset(username string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.attempts, userKey(username))
}
