package storage

import (
	"context"
	"sync"
	"time"
)

// MemoryStorage guarda tudo num map dentro do processo. Serve pros testes e pra
// mostrar que dá pra trocar a estratégia sem mexer no limiter, mas não funciona
// com mais de uma instância da aplicação, por isso o padrão é o Redis.
type MemoryStorage struct {
	mu       sync.Mutex
	counters map[string]counter
	blocks   map[string]time.Time
}

type counter struct {
	value     int64
	expiresAt time.Time
}

func NewMemoryStorage() *MemoryStorage {
	return &MemoryStorage{
		counters: map[string]counter{},
		blocks:   map[string]time.Time{},
	}
}

func (s *MemoryStorage) Increment(ctx context.Context, key string, window time.Duration) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	c, ok := s.counters[key]
	if !ok || now.After(c.expiresAt) {
		c = counter{expiresAt: now.Add(window)}
	}
	c.value++
	s.counters[key] = c
	return c.value, nil
}

func (s *MemoryStorage) Block(ctx context.Context, key string, duration time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.blocks[key] = time.Now().Add(duration)
	return nil
}

func (s *MemoryStorage) IsBlocked(ctx context.Context, key string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	until, ok := s.blocks[key]
	if !ok {
		return false, nil
	}
	if time.Now().After(until) {
		delete(s.blocks, key)
		return false, nil
	}
	return true, nil
}
