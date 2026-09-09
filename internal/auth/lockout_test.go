package auth

import (
	"testing"
	"time"
)

func TestLockoutLocksAfterMaxFailures(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	clock := func() time.Time { return now }
	l := NewLockout(clock)

	for i := 0; i < MaxLoginFailures-1; i++ {
		l.RecordFailure("Alice", "")
		if locked, _ := l.Locked("alice"); locked {
			t.Fatalf("should not be locked after %d failures", i+1)
		}
	}
	l.RecordFailure("alice", "")
	locked, until := l.Locked("ALICE")
	if !locked {
		t.Fatalf("expected lock after %d failures", MaxLoginFailures)
	}
	if !until.Equal(now.Add(LockoutDuration)) {
		t.Fatalf("unexpected lock expiry: %v", until)
	}
}

func TestLockoutExpires(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	clock := func() time.Time { return now }
	l := NewLockout(clock)
	for i := 0; i < MaxLoginFailures; i++ {
		l.RecordFailure("bob", "")
	}
	if locked, _ := l.Locked("bob"); !locked {
		t.Fatalf("expected locked")
	}
	now = now.Add(LockoutDuration + time.Second)
	if locked, _ := l.Locked("bob"); locked {
		t.Fatalf("expected lock to have expired")
	}
	// after expiry, failure count should have reset
	l.RecordFailure("bob", "")
	if locked, _ := l.Locked("bob"); locked {
		t.Fatalf("single failure after expiry should not relock")
	}
}

func TestLockoutReset(t *testing.T) {
	l := NewLockout(nil)
	for i := 0; i < MaxLoginFailures-1; i++ {
		l.RecordFailure("carol", "")
	}
	l.Reset("carol")
	l.RecordFailure("carol", "")
	if locked, _ := l.Locked("carol"); locked {
		t.Fatalf("expected reset to clear prior failures")
	}
}

func TestLockoutIndependentUsers(t *testing.T) {
	l := NewLockout(nil)
	for i := 0; i < MaxLoginFailures; i++ {
		l.RecordFailure("dave", "")
	}
	if locked, _ := l.Locked("dave"); !locked {
		t.Fatalf("dave should be locked")
	}
	if locked, _ := l.Locked("erin"); locked {
		t.Fatalf("erin should not be affected by dave's failures")
	}
}
