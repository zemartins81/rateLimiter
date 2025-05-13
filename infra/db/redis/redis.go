package redis

import (
"context"
"errors"
"fmt"
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

// Increment incrementa o contador para uma chave e define um tempo de expiração (janela).
func (rs *RedisStore) Increment(ctx context.Context, key string, window time.Duration) (int64, error) {
// Usamos um script Lua para garantir atomicidade ao incrementar e definir o TTL na primeira vez.
// Ou apenas incrementar se a chave já existir.
// O script retorna o novo valor do contador.
script := `
local current_val = redis.call("INCR", KEYS[1])
if tonumber(current_val) == 1 then
  redis.call("EXPIRE", KEYS[1], ARGV[1])
end
return current_val
`
// window.Seconds() é float64, precisamos de int para o script
result, err := rs.client.Eval(ctx, script, []string{key}, int(window.Seconds())).Result()
if err != nil {
return 0, fmt.Errorf("erro ao executar script Lua INCR com EXPIRE: %w", err)
}

count, ok := result.(int64)
if !ok {
return 0, fmt.Errorf("resultado inesperado do script Lua: %v", result)
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
