package redis

import (
	"errors"
	"fmt"
	"github.com/go-redis/redis/v8"
	"golang.org/x/net/context"
	"time"
)

// RedisStore implementa a interface Store usando Redis.
type RedisStore struct {
	client *redis.Client
}

// NewRedisStore cria uma nova instância de RedisStore.
func NewRedisStore(client *redis.Client) *RedisStore {
	return &RedisStore{client: client}
}

// Increment usa uma transação Redis para incrementar e possivelmente definir TTL
func (rs *RedisStore) Increment(ctx context.Context, key string, window time.Duration) (int64, error) {
	// Primeiro verificamos se a chave já existe
	exists, err := rs.client.Exists(ctx, key).Result()
	if err != nil {
		return 0, fmt.Errorf("erro ao verificar existência da chave: %w", err)
	}

	// Se a chave não existir, podemos usar um método mais simples
	if exists == 0 {
		pipe := rs.client.Pipeline()
		incr := pipe.Incr(ctx, key)
		pipe.Expire(ctx, key, window)
		_, err := pipe.Exec(ctx)
		if err != nil {
			return 0, fmt.Errorf("erro ao executar pipeline para nova chave: %w", err)
		}
		return incr.Val(), nil
	}

	// Caso a chave já exista, apenas incrementamos
	count, err := rs.client.Incr(ctx, key).Result()
	if err != nil {
		return 0, fmt.Errorf("erro ao incrementar contador: %w", err)
	}

	return count, nil
}

// IsBlocked verifica se uma chave está marcada como bloqueada.
func (rs *RedisStore) IsBlocked(ctx context.Context, key string) (bool, error) {
	val, err := rs.client.Get(ctx, key).Result()
	if err == redis.Nil {
		return false, nil // Chave não existe, não está bloqueada
	} else if err != nil {
		return false, fmt.Errorf("erro ao verificar chave de bloqueio no Redis: %w", err)
	}
	return val == "blocked", nil // Se a chave existir e o valor for "blocked"
}

// Block marca uma chave como bloqueada por uma determinada duração.
func (rs *RedisStore) Block(ctx context.Context, key string, duration time.Duration) error {
	err := rs.client.Set(ctx, key, "blocked", duration).Err()
	if err != nil {
		return fmt.Errorf("erro ao definir chave de bloqueio no Redis: %w", err)
	}
	return nil
}

// Reset remove uma chave do Redis (usado para limpar contadores após bloqueio, por exemplo).
func (rs *RedisStore) Reset(ctx context.Context, key string) error {
	err := rs.client.Del(ctx, key).Err()
	if err != nil && !errors.Is(err, redis.Nil) { // Ignora erro se a chave não existir
		return fmt.Errorf("erro ao deletar chave no Redis: %w", err)
	}
	return nil
}

// Close fecha a conexão com o Redis.
func (rs *RedisStore) Close() error {
	return rs.client.Close()
}
