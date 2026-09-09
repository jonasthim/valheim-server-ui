package auth

import (
	"strings"
	"sync"
	"time"
)

// MaxLoginFailures and LockoutDuration implement the login lockout policy in
// ARCHITECTURE.md §13: 5 failed attempts per username locks it for 5 minutes.
const (
	MaxLoginFailures = 5
	LockoutDuration  = 5 * time.Minute
)

// Clock abstracts time.Now for deterministic tests.
type Clock func() time.Time

type lockoutState struct {
	failures    int
	lockedUntil time.Time
}

// Lockout tracks failed local-login attempts per username, in-memory. It is
// safe for concurrent use.
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

// Locked reports whether username is currently locked out, and until when.
// An expired lock is cleared as a side effect.
func (l *Lockout) Locked(username string) (bool, time.Time) {
	key := strings.ToLower(username)
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

// RecordFailure increments the failure count for username, locking it once
// MaxLoginFailures is reached.
func (l *Lockout) RecordFailure(username string) {
	key := strings.ToLower(username)
	l.mu.Lock()
	defer l.mu.Unlock()
	st, ok := l.attempts[key]
	if !ok {
		st = &lockoutState{}
		l.attempts[key] = st
	}
	st.failures++
	if st.failures >= MaxLoginFailures {
		st.lockedUntil = l.now().Add(LockoutDuration)
	}
}

// Reset clears all failure state for username (called on successful login).
func (l *Lockout) Reset(username string) {
	key := strings.ToLower(username)
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.attempts, key)
}
