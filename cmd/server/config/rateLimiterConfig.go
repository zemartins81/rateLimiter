package server

import (
	"os"

	"github.com/joho/godotenv"
)

type RateLimiterConfig struct {
	MaxRequestsPerIP          int
	MaxRequestsPerToken       int
	BlockDurationIPSeconds    int
	BlockDurationTokenSeconds int
	TokenHeaderName           string
}

func LoadConfigRateLimiter() (config *RateLimiterConfig) {
	err := godotenv.Load()
	if err != nil {
		panic("Erro ao ler o arquivo de configurações")
	}

	maxRequestsIPStr := os.Getenv("MAX_REQUESTS_PER_IP")
	if maxRequestsIPStr == "" {
		maxRequestsIPStr = "5"
	}

	maxRequestsTokenStr := os.Getenv("MAX_REQUESTS_PER_TOKEN")
	if maxRequestsTokenStr == "" {
		maxRequestsTokenStr = "10"
	}

	blockDurationIPStr := os.Getenv("BLOCK_DURATION_IP_SECONDS")
	if blockDurationIPStr == "" {
		blockDurationIPStr = "300"
	}

	blockDurationTokenStr := os.Getenv("BLOCK_DURATION_TOKEN_SECONDS")
	if blockDurationTokenStr == "" {
		blockDurationTokenStr = "300"
	}

	tokenHeaderName := os.Getenv("TOKEN_HEADER_NAME")
	if tokenHeaderName == "" {
		tokenHeaderName = "API_KEY"
	}
}
