package context

import (
	"sync"

	"tianxuan/internal/provider"
)

// MessageStore is the abstraction for conversation history storage.
// V4.0: enables swapping the default in-memory store for SQLite or other
// backends without changing FlowLayer logic.
type MessageStore interface {
	Append(msg provider.Message) error
	Range(start, end int) ([]provider.Message, error)
	Len() int
	Truncate(n int) error
	Close() error
}

// MemoryStore is the default in-memory MessageStore (backed by a slice).
// This preserves the existing V3.x behaviour exactly. V4.0c: thread-safe.
type MemoryStore struct {
	mu   sync.Mutex
	msgs []provider.Message
}

// NewMemoryStore creates an empty in-memory store.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{}
}

func (s *MemoryStore) Append(msg provider.Message) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.msgs = append(s.msgs, msg)
	return nil
}

func (s *MemoryStore) Range(start, end int) ([]provider.Message, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if start < 0 {
		start = 0
	}
	if end > len(s.msgs) {
		end = len(s.msgs)
	}
	if start >= end {
		return nil, nil
	}
	return s.msgs[start:end], nil
}

func (s *MemoryStore) Len() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.msgs)
}

func (s *MemoryStore) Truncate(n int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if n < len(s.msgs) {
		s.msgs = s.msgs[:n]
	}
	return nil
}

func (s *MemoryStore) Close() error {
	return nil
}
