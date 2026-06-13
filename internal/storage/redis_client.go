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

func (s *RedisStore) Close() error { return s.client.Close() }

func (s *RedisStore) SaveRecords(ctx context.Context, symbol string, records []models.OptionRecord) error {
	data, err := json.Marshal(records)
	if err != nil {
		return err
	}
	return s.client.Set(ctx, "oi:records:"+symbol, data, s.ttl).Err()
}

func (s *RedisStore) GetRecords(ctx context.Context, symbol string) ([]models.OptionRecord, error) {
	data, err := s.client.Get(ctx, "oi:records:"+symbol).Bytes()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var records []models.OptionRecord
	return records, json.Unmarshal(data, &records)
}

func (s *RedisStore) SaveSpotPrice(ctx context.Context, symbol string, price float64) error {
	return s.client.Set(ctx, "oi:spot:"+symbol, price, s.ttl).Err()
}

func (s *RedisStore) GetSpotPrice(ctx context.Context, symbol string) (float64, error) {
	val, err := s.client.Get(ctx, "oi:spot:"+symbol).Float64()
	if err == redis.Nil {
		return 0, nil
	}
	return val, err
}

func (s *RedisStore) SaveAnalysis(ctx context.Context, symbol string, resp *models.AnalyseResponse) error {
	data, err := json.Marshal(resp)
	if err != nil {
		return err
	}
	return s.client.Set(ctx, "oi:analysis:"+symbol, data, s.ttl).Err()
}

func (s *RedisStore) GetAnalysis(ctx context.Context, symbol string) (*models.AnalyseResponse, error) {
	data, err := s.client.Get(ctx, "oi:analysis:"+symbol).Bytes()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var resp models.AnalyseResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, err
	}
	resp.Cached = true
	return &resp, nil
}

func (s *RedisStore) DeleteSignals(ctx context.Context, symbol string) error {
	return s.client.Del(ctx, "oi:analysis:"+symbol).Err()
}
