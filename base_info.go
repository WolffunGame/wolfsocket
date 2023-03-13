package wolfsocket

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/redis/go-redis/v9"
	"time"
)

var (
	ctx                 = context.Background()
	ErrBaseInfoNotFound = fmt.Errorf("base info not found ")
)

type BaseInfo interface {
	GetID() string
	GetKey() string
}

type RedisStore struct {
	client redis.UniversalClient
}

func NewRedisStore(client redis.UniversalClient) (*RedisStore, error) {
	_, err := client.Ping(ctx).Result()
	if err != nil {
		return nil, err
	}

	return &RedisStore{client}, nil
}

func (s *RedisStore) SaveInfo(info BaseInfo) error {
	key := fmt.Sprintf("base:%s", info.GetID())
	value, err := json.Marshal(info)
	if err != nil {
		return err
	}

	err = s.client.Set(ctx, key, value, time.Hour).Err()
	if err != nil {
		return err
	}

	return nil
}

func (s *RedisStore) GetInfo(key string, info BaseInfo) error {
	result, err := s.client.Get(ctx, key).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return ErrBaseInfoNotFound
		}
		return err
	}

	err = json.Unmarshal([]byte(result), &info)
	if err != nil {
		return err
	}

	return nil
}

func (s *RedisStore) UpdateInfo(info BaseInfo) error {
	key := info.GetKey()
	value, err := json.Marshal(info)
	if err != nil {
		return err
	}

	err = s.client.Set(ctx, key, value, 0).Err()
	if err != nil {
		return err
	}

	return nil
}

func (s *RedisStore) DeleteInfo(key string) error {
	err := s.client.Del(ctx, key).Err()
	if err != nil {
		return err
	}

	return nil
}
