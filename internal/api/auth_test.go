package api

import (
	"testing"
	"time"
)

func TestLoginLockout(t *testing.T) {
	// Isolate the shared limiter for this test.
	loginLimiter.mu.Lock()
	loginLimiter.byIP = map[string]*attemptState{}
	loginLimiter.mu.Unlock()

	ip := "10.0.0.5"
	now := time.Now()

	// First maxLoginFailures-1 failures must NOT lock.
	for i := 0; i < maxLoginFailures-1; i++ {
		if d := recordLoginFailure(ip, now); d != 0 {
			t.Fatalf("failure %d should not lock yet", i+1)
		}
		if locked, _ := loginLocked(ip, now); locked {
			t.Fatalf("should not be locked after %d failures", i+1)
		}
	}
	// The threshold failure locks out.
	if d := recordLoginFailure(ip, now); d != lockoutDuration {
		t.Fatalf("expected lockout %s, got %s", lockoutDuration, d)
	}
	if locked, remain := loginLocked(ip, now); !locked || remain <= 0 {
		t.Fatalf("expected to be locked with remaining time, got locked=%v remain=%s", locked, remain)
	}
	// A different IP is unaffected.
	if locked, _ := loginLocked("10.0.0.6", now); locked {
		t.Fatal("a different IP must not be locked")
	}
	// Lock expires after the duration.
	if locked, _ := loginLocked(ip, now.Add(lockoutDuration+time.Second)); locked {
		t.Fatal("lock should expire")
	}
	// A successful login clears the IP's state.
	recordLoginFailure(ip, now)
	clearLoginFailures(ip)
	if locked, _ := loginLocked(ip, now); locked {
		t.Fatal("clearLoginFailures should reset state")
	}
}
