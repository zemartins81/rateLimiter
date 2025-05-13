package domain

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"time"
	
	"github.com/joho/godotenv"
	"rateLimiter/infra/db"
)

// LimiterConfig armazena as configurações do rate limiter.
type LimiterConfig struct {
	MaxRequestsPerIP          int
	MaxRequestsPerToken       int
	BlockDurationIPSeconds    int
	BlockDurationTokenSeconds int
	TokenHeaderName           string
}

// RateLimiter é a estrutura principal do rate limiter.
type RateLimiter struct {
	config *LimiterConfig
	store  db.Store
}

// NewRateLimiter cria uma nova instância do RateLimiter.
func NewRateLimiter(config *LimiterConfig, store db.Store) *RateLimiter {
	return &RateLimiter{
		config: config,
		store:  store,
	}
}

// GetConfig retorna a configuração do rate limiter.
func (rl *RateLimiter) GetConfig() *LimiterConfig {
	return rl.config
}

// LoadConfigFromEnv carrega as configurações das variáveis de ambiente ou de um arquivo .env.
func LoadConfigFromEnv() (*LimiterConfig, error) {
	err := godotenv.Load()
	if err != nil {
		return nil, fmt.Errorf("erro ao carregar arquivo .env: %w", err)
	}
	
	maxRequestsIPStr := os.Getenv("MAX_REQUESTS_PER_IP")
	if maxRequestsIPStr == "" {
		maxRequestsIPStr = "5" // Valor padrão
	}
	maxRequestsIP, err := strconv.Atoi(maxRequestsIPStr)
	if err != nil {
		return nil, fmt.Errorf("erro ao converter MAX_REQUESTS_PER_IP: %w", err)
	}
	
	maxRequestsTokenStr := os.Getenv("MAX_REQUESTS_PER_TOKEN")
	if maxRequestsTokenStr == "" {
		maxRequestsTokenStr = "10" // Valor padrão
	}
	maxRequestsToken, err := strconv.Atoi(maxRequestsTokenStr)
	if err != nil {
		return nil, fmt.Errorf("erro ao converter MAX_REQUESTS_PER_TOKEN: %w", err)
	}
	
	blockDurationIPStr := os.Getenv("BLOCK_DURATION_IP_SECONDS")
	if blockDurationIPStr == "" {
		blockDurationIPStr = "300" // Valor padrão (5 minutos)
	}
	blockDurationIP, err := strconv.Atoi(blockDurationIPStr)
	if err != nil {
		return nil, fmt.Errorf("erro ao converter BLOCK_DURATION_IP_SECONDS: %w", err)
	}
	
	blockDurationTokenStr := os.Getenv("BLOCK_DURATION_TOKEN_SECONDS")
	if blockDurationTokenStr == "" {
		blockDurationTokenStr = "300" // Valor padrão (5 minutos)
	}
	blockDurationToken, err := strconv.Atoi(blockDurationTokenStr)
	if err != nil {
		return nil, fmt.Errorf("erro ao converter BLOCK_DURATION_TOKEN_SECONDS: %w", err)
	}
	
	tokenHeaderName := os.Getenv("TOKEN_HEADER_NAME")
	if tokenHeaderName == "" {
		tokenHeaderName = "API_KEY"
	}
	
	return &LimiterConfig{
		MaxRequestsPerIP:          maxRequestsIP,
		MaxRequestsPerToken:       maxRequestsToken,
		BlockDurationIPSeconds:    blockDurationIP,
		BlockDurationTokenSeconds: blockDurationToken,
		TokenHeaderName:           tokenHeaderName,
	}, nil
}

// Allow verifica se uma requisição deve ser permitida.
func (rl *RateLimiter) Allow(ctx context.Context, identifier string, isToken bool) (bool, error) {
	var maxRequests int
	var blockDuration time.Duration
	var keyPrefix string
	
	if isToken {
		maxRequests = rl.config.MaxRequestsPerToken
		blockDuration = time.Duration(rl.config.BlockDurationTokenSeconds) * time.Second
		keyPrefix = "token_"
	} else {
		maxRequests = rl.config.MaxRequestsPerIP
		blockDuration = time.Duration(rl.config.BlockDurationIPSeconds) * time.Second
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
