package rateLimiter

import (
	"context"
	"fmt"
	"time"

	"rateLimiter/cmd/server/config"
	"rateLimiter/infra/db"
)

// RateLimiter é a estrutura principal do rate limiter.
type RateLimiter struct {
	limiterConfig *config.LimiterConfig
	store         db.Store
}

// NewRateLimiter cria uma nova instância do RateLimiter.
func NewRateLimiter(config *config.LimiterConfig, store db.Store) *RateLimiter {
	return &RateLimiter{
		limiterConfig: config,
		store:         store,
	}
}

// GetConfig retorna a configuração do rate limiter.
func (rl *RateLimiter) GetConfig() *config.LimiterConfig {
	limiterConfig, err := config.LoadConfigRateLimiter()
	if err != nil {
		panic("Erro ao ler as configurações do rate limiter: " + err.Error())
	}
	return limiterConfig
}

// Allow verifica se uma requisição deve ser permitida.
func (rl *RateLimiter) Allow(ctx context.Context, identifier string, isToken bool) (bool, error) {
	var maxRequests int
	var blockDuration time.Duration
	var keyPrefix string

	if isToken {
		maxRequests = rl.limiterConfig.MaxRequestsPerToken
		blockDuration = time.Duration(rl.limiterConfig.BlockDurationTokenSeconds) * time.Second
		keyPrefix = "token_"
	} else {
		maxRequests = rl.limiterConfig.MaxRequestsPerIP
		blockDuration = time.Duration(rl.limiterConfig.BlockDurationIPSeconds) * time.Second
		keyPrefix = "ip_"
	}

	key := keyPrefix + identifier
	blockedKey := "blocked_" + key

	// Verifica se está bloqueado
	isBlocked, err := rl.store.IsBlocked(ctx, blockedKey)
	if err != nil {
		return false, fmt.Errorf("erro ao verificar se está bloqueado: %w", err)
	}
	if isBlocked {
		return false, nil // Bloqueado
	}

	count, err := rl.store.Increment(ctx, key, time.Second) // Janela de 1 segundo
	if err != nil {
		return false, fmt.Errorf("erro ao incrementar contador: %w", err)
	}

	if count > int64(maxRequests) {
		err = rl.store.Block(ctx, blockedKey, blockDuration)
		if err != nil {
			return false, fmt.Errorf("erro ao bloquear: %w", err)
		}
		// Limpa o contador de requisições após bloquear para evitar que continue incrementando desnecessariamente
		_ = rl.store.Reset(ctx, key)
		return false, nil // Limite excedido
	}

	return true, nil // Permitido
}
