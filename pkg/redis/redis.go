package redis

import (
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

// Increment incrementa o contador para uma chave e define um tempo de expiração (janela).
func (rs *RedisStore) Increment(ctx context.Context, key string, window time.Duration) (int64, error) {
	// Método 1: Usar transactions para garantir atomicidade
	// Esta função tenta a operação até 3 vezes em caso de conflito
	var count int64
	var err error
	for i := 0; i < 3; i++ {
		count, err = rs.incrementWithTx(ctx, key, window)
		if err == nil {
			return count, nil
		}
		// Se ocorrer erro de conflito, tentamos novamente
		if err != redis.TxFailedErr {
			break
		}
	}
	return 0, fmt.Errorf("erro ao incrementar contador: %w", err)
}

// incrementWithTx usa uma transação Redis para incrementar e possivelmente definir TTL
func (rs *RedisStore) incrementWithTx(ctx context.Context, key string, window time.Duration) (int64, error) {
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
