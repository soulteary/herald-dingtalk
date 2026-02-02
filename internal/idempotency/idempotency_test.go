package idempotency

import (
	"testing"
	"time"
)

func TestNewStore(t *testing.T) {
	s := NewStore(60)
	if s == nil {
		t.Fatal("NewStore(60) returned nil")
	}
	s0 := NewStore(0)
	if s0 == nil {
		t.Fatal("NewStore(0) returned nil")
	}
}

func TestStore_SetAndGetHit(t *testing.T) {
	s := NewStore(300)
	key := "idem-key-1"
	s.Set(key, true, "msg-123")
	c, hit := s.Get(key)
	if !hit {
		t.Fatal("expected hit after Set")
	}
	if !c.OK || c.MessageID != "msg-123" {
		t.Errorf("got OK=%v MessageID=%q, want OK=true MessageID=msg-123", c.OK, c.MessageID)
	}
}

func TestStore_GetMissNoSet(t *testing.T) {
	s := NewStore(300)
	_, hit := s.Get("nonexistent")
	if hit {
		t.Fatal("expected miss for key never set")
	}
}

func TestStore_GetMissAfterExpiry(t *testing.T) {
	s := NewStore(1) // 1 second TTL
	key := "expire-key"
	s.Set(key, true, "msg-456")
	if _, hit := s.Get(key); !hit {
		t.Fatal("expected hit immediately after Set")
	}
	time.Sleep(2 * time.Second)
	_, hit := s.Get(key)
	if hit {
		t.Fatal("expected miss after TTL expiry")
	}
}

func TestStore_SetFailureThenGet(t *testing.T) {
	s := NewStore(300)
	key := "fail-key"
	s.Set(key, false, "")
	c, hit := s.Get(key)
	if !hit {
		t.Fatal("expected hit for cached failure")
	}
	if c.OK || c.MessageID != "" {
		t.Errorf("got OK=%v MessageID=%q, want OK=false MessageID=", c.OK, c.MessageID)
	}
}
