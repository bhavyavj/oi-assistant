package storage

import (
	"context"
	"sync"
	"time"

	"github.com/bhavyavj/oi-assistant/internal/models"
)

type cacheItem[T any] struct {
	value      T
	expiration time.Time
}

type InMemoryStore struct {
	mu         sync.RWMutex
	records    map[string]cacheItem[[]models.OptionRecord]
	spotPrices map[string]cacheItem[float64]
	analyses   map[string]cacheItem[*models.AnalyseResponse]
	defaultTTL time.Duration
}

func NewInMemoryStore(defaultTTL time.Duration) *InMemoryStore {
	return &InMemoryStore{
		records:    make(map[string]cacheItem[[]models.OptionRecord]),
		spotPrices: make(map[string]cacheItem[float64]),
		analyses:   make(map[string]cacheItem[*models.AnalyseResponse]),
		defaultTTL: defaultTTL,
	}
}

func (s *InMemoryStore) SaveRecords(ctx context.Context, symbol string, records []models.OptionRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.records[symbol] = cacheItem[[]models.OptionRecord]{
		value:      records,
		expiration: time.Now().Add(s.defaultTTL),
	}
	return nil
}

func (s *InMemoryStore) GetRecords(ctx context.Context, symbol string) ([]models.OptionRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	item, exists := s.records[symbol]
	if !exists || time.Now().After(item.expiration) {
		return nil, nil
	}
	return item.value, nil
}

func (s *InMemoryStore) SaveSpotPrice(ctx context.Context, symbol string, price float64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.spotPrices[symbol] = cacheItem[float64]{
		value:      price,
		expiration: time.Now().Add(s.defaultTTL),
	}
	return nil
}

func (s *InMemoryStore) GetSpotPrice(ctx context.Context, symbol string) (float64, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	item, exists := s.spotPrices[symbol]
	if !exists || time.Now().After(item.expiration) {
		return 0, nil
	}
	return item.value, nil
}

func (s *InMemoryStore) SaveAnalysis(ctx context.Context, symbol string, resp *models.AnalyseResponse) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.analyses[symbol] = cacheItem[*models.AnalyseResponse]{
		value:      resp,
		expiration: time.Now().Add(s.defaultTTL),
	}
	return nil
}

func (s *InMemoryStore) GetAnalysis(ctx context.Context, symbol string) (*models.AnalyseResponse, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	item, exists := s.analyses[symbol]
	if !exists || time.Now().After(item.expiration) {
		return nil, nil
	}
	resp := *item.value
	resp.Cached = true
	return &resp, nil
}

func (s *InMemoryStore) DeleteSignals(ctx context.Context, symbol string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.analyses, symbol)
	return nil
}

func (s *InMemoryStore) Close() error {
	return nil
}
