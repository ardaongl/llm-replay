package arena

import (
	"sync"
	"time"
)

const (
	defaultStoreLimit = 100
	defaultStoreTTL   = 15 * time.Minute
)

type Store struct {
	mu    sync.Mutex
	items map[string]Comparison
	order []string
	limit int
	ttl   time.Duration
	now   func() time.Time
}

func NewStore() *Store {
	return &Store{items: make(map[string]Comparison), limit: defaultStoreLimit, ttl: defaultStoreTTL, now: time.Now}
}

func (s *Store) Put(value Comparison) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.evictExpired()
	if _, exists := s.items[value.ID]; !exists {
		s.order = append(s.order, value.ID)
	}
	s.items[value.ID] = value
	for len(s.order) > s.limit {
		delete(s.items, s.order[0])
		s.order = s.order[1:]
	}
}

func (s *Store) Get(id string) (Comparison, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.evictExpired()
	value, ok := s.items[id]
	return value, ok
}

func (s *Store) evictExpired() {
	cutoff := s.now().Add(-s.ttl)
	kept := s.order[:0]
	for _, id := range s.order {
		value, exists := s.items[id]
		if !exists {
			continue
		}
		if value.CreatedAt.Before(cutoff) {
			delete(s.items, id)
			continue
		}
		kept = append(kept, id)
	}
	s.order = kept
}
