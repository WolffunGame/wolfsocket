package wolfsocket

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/redis/go-redis/v9"
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
	value, err := json.Marshal(info)
	if err != nil {
		return err
	}

	key := info.GetKey()

	pipe := s.client.Pipeline()

	pipe.HSet(ctx, key, "value", value)
	//pipe.Expire(ctx, key, time.Hour)

	_, err = pipe.Exec(ctx)
	if err != nil {
		return err
	}

	return nil
}

func (s *RedisStore) GetInfo(key string, info BaseInfo) error {
	result, err := s.client.HGet(ctx, key, "value").Result()
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

func (s *RedisStore) DeleteInfo(key string) error {
	pipe := s.client.Pipeline()

	pipe.Del(ctx, key)

	_, err := pipe.Exec(ctx)
	if err != nil {
		return err
	}

	return nil
}
