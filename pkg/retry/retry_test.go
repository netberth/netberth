// Copyright (c) 2025 NetBerth Contributors.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// See the LICENSE file in the project root for full license text.

package retry

import (
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func testCfg() Config {
	return Config{
		BaseDelay:       time.Millisecond,
		MaxDelay:        4 * time.Millisecond,
		Factor:          2.0,
		Jitter:          0,
		MaxAttempts:     4,
		CBThreshold:     3,
		CBHalfOpenAfter: 5 * time.Millisecond,
	}
}

func TestDefaultConfig(t *testing.T) {
	c := DefaultConfig()
	if c.BaseDelay != 500*time.Millisecond || c.MaxDelay != 30*time.Second ||
		c.Factor != 2.0 || c.Jitter != 0.2 || c.MaxAttempts != 5 ||
		c.CBThreshold != 7 || c.CBHalfOpenAfter != 30*time.Second {
		t.Fatalf("unexpected default config: %+v", c)
	}
}

func TestBreakerStateTransitions(t *testing.T) {
	b := NewBreaker(2, 10*time.Millisecond)
	if !b.Allow() {
		t.Fatal("expected closed breaker to allow")
	}
	b.Failure()
	b.Failure()
	if b.Allow() {
		t.Fatal("expected open breaker to reject")
	}
	time.Sleep(15 * time.Millisecond)
	if !b.Allow() {
		t.Fatal("expected half-open breaker to allow a probe")
	}
	b.Success()
	if !b.Allow() {
		t.Fatal("expected breaker to close after success")
	}
}

func TestBreakerResetOnSuccess(t *testing.T) {
	b := NewBreaker(2, time.Hour)
	b.Failure()
	b.Success() // success resets the failure counter
	b.Failure()
	if !b.Allow() {
		t.Fatal("single failure after reset must not open the circuit")
	}
	b.Failure()
	if b.Allow() {
		t.Fatal("expected breaker open after two consecutive failures")
	}
}

func TestDoSuccessFirstAttempt(t *testing.T) {
	cfg := testCfg()
	var calls int32
	res, attempt, err := Do(cfg, nil, "t", func() (string, error) {
		atomic.AddInt32(&calls, 1)
		return "ok", nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if attempt != 1 || res != "ok" {
		t.Fatalf("expected (ok,1), got (%q,%d)", res, attempt)
	}
	if atomic.LoadInt32(&calls) != 1 {
		t.Fatalf("expected 1 call, got %d", calls)
	}
}

func TestDoRetriesThenSuccess(t *testing.T) {
	cfg := testCfg()
	var calls int32
	res, attempt, err := Do(cfg, nil, "t", func() (int, error) {
		n := atomic.AddInt32(&calls, 1)
		if n < 3 {
			return 0, errors.New("transient")
		}
		return 42, nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if attempt != 3 || res != 42 {
		t.Fatalf("expected (42,3), got (%d,%d)", res, attempt)
	}
}

func TestDoExhaustsRetries(t *testing.T) {
	cfg := testCfg()
	wantErr := errors.New("permanent")
	_, attempt, err := Do(cfg, nil, "t", func() (int, error) { return 0, wantErr })
	if err != wantErr {
		t.Fatalf("expected original error, got %v", err)
	}
	if attempt != cfg.MaxAttempts {
		t.Fatalf("expected attempt %d, got %d", cfg.MaxAttempts, attempt)
	}
}

func TestDoCircuitOpen(t *testing.T) {
	b := NewBreaker(1, time.Hour)
	b.Failure() // open
	cfg := testCfg()
	_, attempt, err := Do(cfg, b, "t", func() (int, error) { return 0, nil })
	if err != ErrCircuitOpen {
		t.Fatalf("expected ErrCircuitOpen, got %v", err)
	}
	if attempt != 0 {
		t.Fatalf("expected attempt 0, got %d", attempt)
	}
}

func TestDoMaxAttemptsZero(t *testing.T) {
	cfg := testCfg()
	cfg.MaxAttempts = 0
	_, attempt, err := Do(cfg, nil, "t", func() (int, error) { return 0, nil })
	if err != ErrMaxRetries || attempt != 0 {
		t.Fatalf("expected (ErrMaxRetries,0), got (%v,%d)", err, attempt)
	}
}

func TestDoBreakerSuccessResets(t *testing.T) {
	b := NewBreaker(1, time.Hour)
	cfg := testCfg()
	cfg.MaxAttempts = 2
	if _, _, err := Do(cfg, b, "t", func() (int, error) { return 0, errors.New("boom") }); err == nil {
		t.Fatal("expected error")
	}
	if b.Allow() {
		t.Fatal("expected circuit open after failure")
	}
	if _, _, err := Do(cfg, b, "t", func() (int, error) { return 1, nil }); err != ErrCircuitOpen {
		t.Fatalf("expected circuit open rejection, got %v", err)
	}
}

func TestRetryError(t *testing.T) {
	if RetryError("boom").Error() != "boom" {
		t.Fatal("RetryError.Error mismatch")
	}
	if ErrCircuitOpen.Error() == "" || ErrMaxRetries.Error() == "" {
		t.Fatal("sentinel errors must have messages")
	}
}
