package context

import (
	"encoding/json"
	"os"
	"sync"

	"tianxuan/internal/provider"
)

// FileStore persists messages to a JSONL file for cross-session survival.
// Each message is one JSON line. Less efficient than SQLite but requires
// zero external dependencies.
//
// V4.0: enable via FlowConfig.StoreType = "file".
type FileStore struct {
	mu   sync.Mutex
	path string
	msgs []provider.Message
	f    *os.File
}

// NewFileStore opens or creates a JSONL message store at the given path.
func NewFileStore(path string) (*FileStore, error) {
	s := &FileStore{path: path}
	if path != "" {
		var err error
		s.f, err = os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_RDWR, 0644)
		if err != nil {
			return nil, err
		}
		// Load existing messages
		s.load()
	}
	return s, nil
}

func (s *FileStore) Append(msg provider.Message) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.msgs = append(s.msgs, msg)
	if s.f != nil {
		data, _ := json.Marshal(msg)
		data = append(data, '\n')
		s.f.Write(data)
	}
	return nil
}

func (s *FileStore) Range(start, end int) ([]provider.Message, error) {
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

func (s *FileStore) Len() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.msgs)
}
func (s *FileStore) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.f != nil {
		return s.f.Close()
	}
	return nil
}

func (s *FileStore) Truncate(n int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if n < len(s.msgs) {
		s.msgs = s.msgs[:n]
	}
	// V4.0c fix: also truncate the backing file to prevent stale data on reload
	if s.f != nil {
		// Re-open for truncation (os.Truncate on append-mode file is safe)
		if err := os.Truncate(s.path, 0); err != nil {
			return err
		}
		// Re-write remaining messages
		for _, m := range s.msgs {
			data, _ := json.Marshal(m)
			data = append(data, '\n')
			s.f.Write(data)
		}
	}
	return nil
}

func (s *FileStore) load() {
	if s.f == nil {
		return
	}
	dec := json.NewDecoder(s.f)
	for {
		var msg provider.Message
		if err := dec.Decode(&msg); err != nil {
			break
		}
		s.msgs = append(s.msgs, msg)
	}
}
