package config

import (
	"fmt"
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

// LimiterConfig armazena as configurações do rate limiter.
type LimiterConfig struct {
	MaxRequestsPerIP          int
	MaxRequestsPerToken       int
	BlockDurationIPSeconds    int
	BlockDurationTokenSeconds int
	TokenHeaderName           string
}

func LoadConfigRateLimiter() (*LimiterConfig, error) {
	// Carrega o arquivo .env, mas não falha se ele não existir
	_ = godotenv.Load()

	maxRequestsIPStr := os.Getenv("MAX_REQUESTS_PER_IP")
	if maxRequestsIPStr == "" {
		fmt.Println("Aviso: MAX_REQUESTS_PER_IP não definido, usando valor padrão (5)")
		maxRequestsIPStr = "5"
	}
	maxRequestsIP, err := strconv.Atoi(maxRequestsIPStr)
	if err != nil {
		return nil, fmt.Errorf("erro ao converter MAX_REQUESTS_PER_IP: %w", err)
	}

	maxRequestsTokenStr := os.Getenv("MAX_REQUESTS_PER_TOKEN")
	if maxRequestsTokenStr == "" {
		fmt.Println("Aviso: MAX_REQUESTS_PER_TOKEN não definido, usando valor padrão (10)")
		maxRequestsTokenStr = "10"
	}
	maxRequestsToken, err := strconv.Atoi(maxRequestsTokenStr)
	if err != nil {
		return nil, fmt.Errorf("erro ao converter MAX_REQUESTS_PER_TOKEN: %w", err)
	}

	blockDurationIPStr := os.Getenv("BLOCK_DURATION_IP_SECONDS")
	if blockDurationIPStr == "" {
		fmt.Println("Aviso: BLOCK_DURATION_IP_SECONDS não definido, usando valor padrão (300)")
		blockDurationIPStr = "300"
	}
	blockDurationIP, err := strconv.Atoi(blockDurationIPStr)
	if err != nil {
		return nil, fmt.Errorf("erro ao converter BLOCK_DURATION_IP_SECONDS: %w", err)
	}

	blockDurationTokenStr := os.Getenv("BLOCK_DURATION_TOKEN_SECONDS")
	if blockDurationTokenStr == "" {
		fmt.Println("Aviso: BLOCK_DURATION_TOKEN_SECONDS não definido, usando valor padrão (300)")
		blockDurationTokenStr = "300"
	}
	blockDurationToken, err := strconv.Atoi(blockDurationTokenStr)
	if err != nil {
		return nil, fmt.Errorf("erro ao converter BLOCK_DURATION_TOKEN_SECONDS: %w", err)
	}

	tokenHeaderName := os.Getenv("TOKEN_HEADER_NAME")
	if tokenHeaderName == "" {
		fmt.Println("Aviso: TOKEN_HEADER_NAME não definido, usando valor padrão (API_KEY)")
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
