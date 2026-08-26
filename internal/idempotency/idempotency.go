package idempotency

import (
	"container/list"
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
	order       *list.Element
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
	order      *list.List
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
		order:      list.New(),
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

	go s.execute(key, active, fn)
	return waitForCall(ctx, active, OutcomeExecuted)
}

func (s *Store) execute(key string, active *call, fn func() Result) {
	completed := false
	defer func() {
		if !completed {
			_ = recover()
			s.finish(key, active, Result{}, false, ErrAborted)
		}
	}()
	result := fn()
	s.finish(key, active, result, result.Response.OK, nil)
	completed = true
}

func waitForCall(ctx context.Context, active *call, outcome Outcome) (Result, Outcome, error) {
	select {
	case <-active.done:
		return active.result, outcome, active.err
	case <-ctx.Done():
		return Result{}, "", ctx.Err()
	}
}

func (s *Store) finish(key string, active *call, result Result, cache bool, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	active.result = result
	active.err = err
	delete(s.inFlight, key)
	if cache {
		s.evictForInsertLocked()
		order := s.order.PushBack(key)
		s.entries[key] = entry{
			fingerprint: active.fingerprint,
			result:      result,
			expiresAt:   s.now().Add(s.ttl),
			order:       order,
		}
	}
	close(active.done)
}

func (s *Store) pruneExpiredLocked(now time.Time) {
	for front := s.order.Front(); front != nil; front = s.order.Front() {
		key := front.Value.(string)
		cached, ok := s.entries[key]
		if !ok {
			s.order.Remove(front)
			continue
		}
		if now.Before(cached.expiresAt) {
			return
		}
		delete(s.entries, key)
		s.order.Remove(front)
	}
}

func (s *Store) evictForInsertLocked() {
	if len(s.entries) < s.maxEntries {
		return
	}
	if oldest := s.order.Front(); oldest != nil {
		delete(s.entries, oldest.Value.(string))
		s.order.Remove(oldest)
	}
}
