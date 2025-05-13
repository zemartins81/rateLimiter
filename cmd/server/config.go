package server

import (
	"os"

	"github.com/joho/godotenv"
)

func LoadConfig() {
	err := godotenv.Load()
	if err != nill {
		panic("Erro ao ler o arquivo de configurações")
	}

	maxRequestsIPStr := os.Getenv("MAX_REQUESTS_PER_IP")
	if maxRequestsIPStr = "" {

	}
	maxRequestsTokenStr := os.Getenv("MAX_REQUESTS_PER_TOKEN")
	blockDurationIPStr := os.Getenv("BLOCK_DURATION_IP_SECONDS")
	blockDurationTokenStr := os.Getenv("BLOCK_DURATION_TOKEN_SECONDS")
	tokenHeaderName := os.Getenv("TOKEN_HEADER_NAME")
}
