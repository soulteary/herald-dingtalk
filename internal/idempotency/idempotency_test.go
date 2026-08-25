package idempotency

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/soulteary/provider-kit"
)

func successResult(messageID string) Result {
	return Result{
		StatusCode: 200,
		Response: provider.HTTPSendResponse{
			OK: true, MessageID: messageID, Provider: "dingtalk",
		},
	}
}

func failureResult() Result {
	return Result{
		StatusCode: 502,
		Response: provider.HTTPSendResponse{
			OK: false, ErrorCode: "send_failed", ErrorMessage: "upstream failed",
		},
	}
}

func TestStoreCachesSuccessfulResult(t *testing.T) {
	store := NewStore(300)
	var calls int
	first, outcome, err := store.Do(context.Background(), "key", "fingerprint", func() Result {
		calls++
		return successResult("message-1")
	})
	if err != nil || outcome != OutcomeExecuted || first.Response.MessageID != "message-1" {
		t.Fatalf("first Do = %#v, %q, %v", first, outcome, err)
	}
	second, outcome, err := store.Do(context.Background(), "key", "fingerprint", func() Result {
		calls++
		return successResult("message-2")
	})
	if err != nil || outcome != OutcomeCached || second.Response.MessageID != "message-1" {
		t.Fatalf("second Do = %#v, %q, %v", second, outcome, err)
	}
	if calls != 1 {
		t.Fatalf("calls = %d, want 1", calls)
	}
}

func TestStoreDoesNotCacheFailure(t *testing.T) {
	store := NewStore(300)
	var calls int
	for i := 0; i < 2; i++ {
		result, outcome, err := store.Do(context.Background(), "key", "fingerprint", func() Result {
			calls++
			return failureResult()
		})
		if err != nil || outcome != OutcomeExecuted || result.Response.ErrorCode != "send_failed" {
			t.Fatalf("Do = %#v, %q, %v", result, outcome, err)
		}
	}
	if calls != 2 {
		t.Fatalf("calls = %d, want 2", calls)
	}
}

func TestStoreRejectsFingerprintConflict(t *testing.T) {
	store := NewStore(300)
	_, _, err := store.Do(context.Background(), "key", "first", func() Result {
		return successResult("message-1")
	})
	if err != nil {
		t.Fatalf("first Do: %v", err)
	}
	_, _, err = store.Do(context.Background(), "key", "second", func() Result {
		return successResult("message-2")
	})
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("err = %v, want ErrConflict", err)
	}
}

func TestStoreCoalescesConcurrentRequests(t *testing.T) {
	store := NewStore(300)
	started := make(chan struct{})
	release := make(chan struct{})
	var calls atomic.Int32
	var wg sync.WaitGroup
	results := make(chan Outcome, 8)

	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, outcome, err := store.Do(context.Background(), "key", "fingerprint", func() Result {
				if calls.Add(1) == 1 {
					close(started)
				}
				<-release
				return successResult("message-1")
			})
			if err != nil {
				t.Errorf("Do: %v", err)
				return
			}
			results <- outcome
		}()
	}
	<-started
	close(release)
	wg.Wait()
	close(results)

	if calls.Load() != 1 {
		t.Fatalf("calls = %d, want 1", calls.Load())
	}
	var executed, shared, cached int
	for outcome := range results {
		switch outcome {
		case OutcomeExecuted:
			executed++
		case OutcomeShared:
			shared++
		case OutcomeCached:
			cached++
		}
	}
	if executed != 1 || shared+cached != 7 {
		t.Fatalf("outcomes: executed=%d shared=%d cached=%d", executed, shared, cached)
	}
}

func TestStoreWaiterHonorsContextCancellation(t *testing.T) {
	store := NewStore(300)
	started := make(chan struct{})
	release := make(chan struct{})
	go func() {
		_, _, _ = store.Do(context.Background(), "key", "fingerprint", func() Result {
			close(started)
			<-release
			return successResult("message-1")
		})
	}()
	<-started

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, _, err := store.Do(ctx, "key", "fingerprint", func() Result {
		t.Fatal("waiter must not execute")
		return Result{}
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	close(release)
}

func TestStoreExpiresAndEvictsEntries(t *testing.T) {
	now := time.Unix(100, 0)
	clock := func() time.Time { return now }
	store := newStore(1, 1, clock)
	_, _, _ = store.Do(context.Background(), "first", "one", func() Result {
		return successResult("message-1")
	})
	_, _, _ = store.Do(context.Background(), "second", "two", func() Result {
		return successResult("message-2")
	})
	if len(store.entries) != 1 {
		t.Fatalf("entries = %d, want 1", len(store.entries))
	}

	now = now.Add(2 * time.Second)
	var calls int
	result, outcome, err := store.Do(context.Background(), "second", "two", func() Result {
		calls++
		return successResult("message-3")
	})
	if err != nil || outcome != OutcomeExecuted || result.Response.MessageID != "message-3" || calls != 1 {
		t.Fatalf("expired Do = %#v, %q, %v, calls=%d", result, outcome, err, calls)
	}
}

func TestStoreUsesSafeDefaults(t *testing.T) {
	store := newStore(0, 0, nil)
	if store.ttl != defaultTTLSeconds*time.Second {
		t.Fatalf("ttl = %s, want %s", store.ttl, defaultTTLSeconds*time.Second)
	}
	if store.maxEntries != defaultMaxEntries {
		t.Fatalf("maxEntries = %d, want %d", store.maxEntries, defaultMaxEntries)
	}
	if store.now == nil {
		t.Fatal("default clock must be configured")
	}
}

func TestStoreEmptyKeyBypassesCache(t *testing.T) {
	store := NewStore(300)
	var calls int
	for i := 0; i < 2; i++ {
		result, outcome, err := store.Do(context.Background(), "", "fingerprint", func() Result {
			calls++
			return successResult("message")
		})
		if err != nil || outcome != OutcomeExecuted || !result.Response.OK {
			t.Fatalf("Do = %#v, %q, %v", result, outcome, err)
		}
	}
	if calls != 2 {
		t.Fatalf("calls = %d, want 2", calls)
	}
}

func TestStoreRejectsCanceledContextBeforeExecution(t *testing.T) {
	store := NewStore(300)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	executed := false
	_, _, err := store.Do(ctx, "key", "fingerprint", func() Result {
		executed = true
		return successResult("message")
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if executed {
		t.Fatal("operation executed after its context was canceled")
	}
}
