package cache

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

type Store interface {
	Get(ctx context.Context, key string) ([]byte, bool, error)
	Set(ctx context.Context, key string, value []byte, ttl time.Duration) error
	Delete(ctx context.Context, keys ...string) error
	DeleteByPrefix(ctx context.Context, prefix string) error
	RememberJSON(ctx context.Context, key string, ttl time.Duration, fn func() (interface{}, error)) ([]byte, error)
}

type redisStore struct {
	client *redis.Client
}

type noopStore struct{}

func NewStore(redisURL string) Store {
	if strings.TrimSpace(redisURL) == "" {
		return noopStore{}
	}
	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		return noopStore{}
	}
	return &redisStore{client: redis.NewClient(opts)}
}

func (s *redisStore) Get(ctx context.Context, key string) ([]byte, bool, error) {
	val, err := s.client.Get(ctx, key).Bytes()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, false, nil
		}
		return nil, false, err
	}
	return val, true, nil
}

func (s *redisStore) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	return s.client.Set(ctx, key, value, ttl).Err()
}

func (s *redisStore) Delete(ctx context.Context, keys ...string) error {
	if len(keys) == 0 {
		return nil
	}
	return s.client.Del(ctx, keys...).Err()
}

func (s *redisStore) DeleteByPrefix(ctx context.Context, prefix string) error {
	iter := s.client.Scan(ctx, 0, prefix+"*", 200).Iterator()
	keys := make([]string, 0, 200)
	for iter.Next(ctx) {
		keys = append(keys, iter.Val())
		if len(keys) >= 200 {
			if err := s.client.Del(ctx, keys...).Err(); err != nil {
				return err
			}
			keys = keys[:0]
		}
	}
	if len(keys) > 0 {
		if err := s.client.Del(ctx, keys...).Err(); err != nil {
			return err
		}
	}
	return iter.Err()
}

func (s *redisStore) RememberJSON(ctx context.Context, key string, ttl time.Duration, fn func() (interface{}, error)) ([]byte, error) {
	if cached, ok, err := s.Get(ctx, key); err == nil && ok {
		return cached, nil
	}
	v, err := fn()
	if err != nil {
		return nil, err
	}
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	_ = s.Set(ctx, key, b, ttl)
	return b, nil
}

func (noopStore) Get(context.Context, string) ([]byte, bool, error) { return nil, false, nil }
func (noopStore) Set(context.Context, string, []byte, time.Duration) error {
	return nil
}
func (noopStore) Delete(context.Context, ...string) error { return nil }
func (noopStore) DeleteByPrefix(context.Context, string) error {
	return nil
}
func (n noopStore) RememberJSON(ctx context.Context, key string, ttl time.Duration, fn func() (interface{}, error)) ([]byte, error) {
	v, err := fn()
	if err != nil {
		return nil, err
	}
	return json.Marshal(v)
}
