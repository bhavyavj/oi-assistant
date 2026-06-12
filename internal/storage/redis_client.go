package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/bhavyavj/oi-assistant/internal/models"
)

type RedisStore struct {
	client *redis.Client
	ttl    time.Duration
}

func NewRedisStore(addr, password string, db int, ttl time.Duration) (*RedisStore, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: password,
		DB:       db,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("redis ping: %w", err)
	}

	return &RedisStore{client: client, ttl: ttl}, nil
}

func (s *RedisStore) Close() error {
	return s.client.Close()
}

func (s *RedisStore) SaveRecords(ctx context.Context, symbol string, records []models.OptionRecord) error {
	data, err := json.Marshal(records)
	if err != nil {
		return fmt.Errorf("marshal records: %w", err)
	}
	return s.client.Set(ctx, recordsKey(symbol), data, s.ttl).Err()
}

func (s *RedisStore) GetRecords(ctx context.Context, symbol string) ([]models.OptionRecord, error) {
	data, err := s.client.Get(ctx, recordsKey(symbol)).Bytes()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("redis get: %w", err)
	}
	var records []models.OptionRecord
	if err := json.Unmarshal(data, &records); err != nil {
		return nil, fmt.Errorf("unmarshal records: %w", err)
	}
	return records, nil
}

func (s *RedisStore) SaveSignals(ctx context.Context, symbol string, signals []models.TradeSignal) error {
	data, err := json.Marshal(signals)
	if err != nil {
		return fmt.Errorf("marshal signals: %w", err)
	}
	return s.client.Set(ctx, signalsKey(symbol), data, s.ttl).Err()
}

func (s *RedisStore) GetSignals(ctx context.Context, symbol string) ([]models.TradeSignal, error) {
	data, err := s.client.Get(ctx, signalsKey(symbol)).Bytes()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("redis get signals: %w", err)
	}
	var signals []models.TradeSignal
	if err := json.Unmarshal(data, &signals); err != nil {
		return nil, fmt.Errorf("unmarshal signals: %w", err)
	}
	return signals, nil
}

func recordsKey(symbol string) string { return "oi:records:" + symbol }
func signalsKey(symbol string) string { return "oi:signals:" + symbol }
