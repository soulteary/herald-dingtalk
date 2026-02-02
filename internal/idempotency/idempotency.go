package idempotency

import (
	"sync"
	"time"
)

type entry struct {
	ok        bool
	messageID string
	expiresAt time.Time
}

// Store is an in-memory idempotency store. Same key within TTL returns cached response.
type Store struct {
	mu     sync.RWMutex
	m      map[string]entry
	ttlSec int
}

// NewStore creates a store with the given TTL in seconds.
func NewStore(ttlSec int) *Store {
	s := &Store{m: make(map[string]entry), ttlSec: ttlSec}
	if s.ttlSec <= 0 {
		s.ttlSec = 300
	}
	return s
}

type cached struct {
	OK        bool
	MessageID string
}

// Get returns cached result for key if not expired. ok=false means miss.
func (s *Store) Get(key string) (cached, bool) {
	s.mu.RLock()
	e, ok := s.m[key]
	s.mu.RUnlock()
	if !ok || time.Now().After(e.expiresAt) {
		return cached{}, false
	}
	return cached{OK: e.ok, MessageID: e.messageID}, true
}

// Set stores the result for key with TTL.
func (s *Store) Set(key string, ok bool, messageID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.m[key] = entry{
		ok:        ok,
		messageID: messageID,
		expiresAt: time.Now().Add(time.Duration(s.ttlSec) * time.Second),
	}
}
