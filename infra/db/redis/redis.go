package redis

import (
	"encoding/gob"
	"time"

	"github.com/go-redis/redis/v8"
)

// RedisStore implementa a interface Store usando Redis.
type RedisStore struct {
	client *redis.Client
}

// NewRedisStore cria uma nova instância de RedisStore.
func NewRedisStore(client *redis.Client) *RedisStore {
	return &RedisStore{client: client}
}
