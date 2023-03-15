package wolfsocket

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/redis/go-redis/v9"
	"reflect"
	"strconv"
)

var (
	ErrBaseInfoNotFound = fmt.Errorf("base info not found ")
)

type ErrKeyHasBeenModified struct {
	key string
}

func (e ErrKeyHasBeenModified) Error() string {
	return fmt.Sprintf("key %s has been modified by another process ", e.key)
}

type BaseInfo interface {
	GetID() string
	GetKey() string
	GetVersion() int
	SetVersion(int)
}
type RedisStore struct {
	client redis.UniversalClient
}

func NewRedisStore(ctx context.Context, client redis.UniversalClient) (*RedisStore, error) {
	_, err := client.Ping(ctx).Result()
	if err != nil {
		return nil, err
	}

	return &RedisStore{client}, nil
}

func (s *RedisStore) SaveInfo(ctx context.Context, info BaseInfo) error {
	// Watch the key for changes
	err := s.client.Watch(ctx, func(tx *redis.Tx) error {
		// Get the current version of the key
		currentVersion, err := tx.HGet(ctx, info.GetKey(), "version").Int()
		if err != nil && !errors.Is(err, redis.Nil) {
			return err
		}

		// If the key doesn't exist, set the version to 0
		if errors.Is(err, redis.Nil) {
			currentVersion = 0
		}

		// Check if the version matches
		if currentVersion != info.GetVersion() {
			return ErrKeyHasBeenModified{info.GetKey()}
		}

		// Increment the version number and update the key
		newVersion := currentVersion + 1
		info.SetVersion(newVersion)

		// Convert the base info to a map
		infoMap := toMap(info)

		// Use a pipeline to execute the commands atomically
		pipe := tx.Pipeline()
		// Set the values using HMSet
		pipe.HMSet(ctx, info.GetKey(), infoMap)
		_, err = pipe.Exec(ctx)
		if err != nil {
			return err
		}

		return nil
	}, info.GetKey())

	if err != nil {
		return err
	}
	return nil
}

// Helper function to convert a BaseInfo object to a map
func toMap(info BaseInfo) map[string]interface{} {
	infoValue := reflect.ValueOf(info)
	infoType := infoValue.Type()

	if infoValue.Kind() == reflect.Ptr {
		infoValue = infoValue.Elem()
		infoType = infoValue.Type()
	}

	// Create a map to hold the values
	infoMap := make(map[string]interface{})

	// Loop over the fields in the struct and add them to the map
	for i := 0; i < infoValue.NumField(); i++ {
		field := infoValue.Field(i)
		tag := infoType.Field(i).Tag.Get("redis")
		if len(tag) > 0 && tag != "-" {
			infoMap[tag] = field.Interface()
		}
	}

	return infoMap
}
func (s *RedisStore) GetInfo(ctx context.Context, key string, info BaseInfo) error {
	// Retrieve the values of the base info fields and the version
	infoMap, err := s.client.HGetAll(ctx, key).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return ErrBaseInfoNotFound
		}
		return err
	}

	// Convert the map to a JSON-encoded string
	infoJSON, err := json.Marshal(infoMap)
	if err != nil {
		return err
	}

	// Decode the JSON-encoded string into the base info object
	err = json.Unmarshal(infoJSON, &info)
	if err != nil {
		return err
	}

	// Set the version of the base info object
	if versionStr, exists := infoMap["version"]; exists {
		version, err := strconv.Atoi(versionStr)
		if err != nil {
			return err
		}
		info.SetVersion(version)
	}

	return nil
}

func (s *RedisStore) DeleteInfo(ctx context.Context, key string) error {
	pipe := s.client.Pipeline()

	pipe.Del(ctx, key)

	_, err := pipe.Exec(ctx)
	if err != nil {
		return err
	}

	return nil
}
