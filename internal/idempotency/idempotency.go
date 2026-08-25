package idempotency

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/soulteary/provider-kit"
)

const (
	defaultTTLSeconds = 300
	defaultMaxEntries = 10000
)

var (
	ErrConflict = errors.New("idempotency key reused with a different request")
	ErrAborted  = errors.New("idempotent operation aborted")
)

type Outcome string

const (
	OutcomeExecuted Outcome = "executed"
	OutcomeCached   Outcome = "cached"
	OutcomeShared   Outcome = "shared"
)

// Result preserves the complete HTTP response produced by a send attempt.
type Result struct {
	StatusCode int
	Response   provider.HTTPSendResponse
}

type entry struct {
	fingerprint string
	result      Result
	expiresAt   time.Time
}

type call struct {
	fingerprint string
	done        chan struct{}
	result      Result
	err         error
}

// Store is an in-memory idempotency store with in-flight request coalescing.
// Successful results are cached. Concurrent callers also share the leader's
// failure result, but failures are not retained for later retries.
type Store struct {
	mu         sync.Mutex
	entries    map[string]entry
	inFlight   map[string]*call
	ttl        time.Duration
	maxEntries int
	now        func() time.Time
}

// NewStore creates a bounded store with the given TTL in seconds.
func NewStore(ttlSec int) *Store {
	return newStore(ttlSec, defaultMaxEntries, time.Now)
}

func newStore(ttlSec, maxEntries int, now func() time.Time) *Store {
	if ttlSec <= 0 {
		ttlSec = defaultTTLSeconds
	}
	if maxEntries <= 0 {
		maxEntries = defaultMaxEntries
	}
	if now == nil {
		now = time.Now
	}
	return &Store{
		entries:    make(map[string]entry),
		inFlight:   make(map[string]*call),
		ttl:        time.Duration(ttlSec) * time.Second,
		maxEntries: maxEntries,
		now:        now,
	}
}

// Do returns a cached result, joins an identical in-flight request, or executes
// fn as the leader. Reusing a key with a different fingerprint returns
// ErrConflict. Only successful responses are retained after fn completes.
func (s *Store) Do(ctx context.Context, key, fingerprint string, fn func() Result) (Result, Outcome, error) {
	if err := ctx.Err(); err != nil {
		return Result{}, "", err
	}
	if key == "" {
		return fn(), OutcomeExecuted, nil
	}

	s.mu.Lock()
	now := s.now()
	s.pruneExpiredLocked(now)
	if cached, ok := s.entries[key]; ok {
		if cached.fingerprint != fingerprint {
			s.mu.Unlock()
			return Result{}, "", ErrConflict
		}
		result := cached.result
		s.mu.Unlock()
		return result, OutcomeCached, nil
	}
	if active, ok := s.inFlight[key]; ok {
		if active.fingerprint != fingerprint {
			s.mu.Unlock()
			return Result{}, "", ErrConflict
		}
		done := active.done
		s.mu.Unlock()
		select {
		case <-done:
			return active.result, OutcomeShared, active.err
		case <-ctx.Done():
			return Result{}, "", ctx.Err()
		}
	}

	active := &call{fingerprint: fingerprint, done: make(chan struct{})}
	s.inFlight[key] = active
	s.mu.Unlock()

	completed := false
	defer func() {
		if !completed {
			s.finish(key, active, Result{}, false, ErrAborted)
		}
	}()

	result := fn()
	s.finish(key, active, result, result.Response.OK, nil)
	completed = true
	return result, OutcomeExecuted, nil
}

func (s *Store) finish(key string, active *call, result Result, cache bool, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	active.result = result
	active.err = err
	delete(s.inFlight, key)
	if cache {
		s.evictForInsertLocked()
		s.entries[key] = entry{
			fingerprint: active.fingerprint,
			result:      result,
			expiresAt:   s.now().Add(s.ttl),
		}
	}
	close(active.done)
}

func (s *Store) pruneExpiredLocked(now time.Time) {
	for key, cached := range s.entries {
		if !now.Before(cached.expiresAt) {
			delete(s.entries, key)
		}
	}
}

func (s *Store) evictForInsertLocked() {
	if len(s.entries) < s.maxEntries {
		return
	}
	var oldestKey string
	var oldestExpiry time.Time
	for key, cached := range s.entries {
		if oldestKey == "" || cached.expiresAt.Before(oldestExpiry) {
			oldestKey = key
			oldestExpiry = cached.expiresAt
		}
	}
	if oldestKey != "" {
		delete(s.entries, oldestKey)
	}
}
