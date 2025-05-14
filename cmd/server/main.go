package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-redis/redis/v8"

	"rateLimiter/cmd/server/config"
	redisStore "rateLimiter/infra/db/redis"
	"rateLimiter/internal/rateLimiter"
	"rateLimiter/pkg/middleware"
)

func main() {
	// Carregar configuração
	configRateLimiter, err := config.LoadConfigRateLimiter()
	if err != nil {
		log.Fatalf("Erro ao carregar configuração: %v", err)
	}

	// Configurar cliente Redis
	redisAddr := os.Getenv("REDIS_ADDR")
	if redisAddr == "" {
		redisAddr = "localhost:6379" // Valor padrão se não estiver nas variáveis de ambiente
	}

	rdb := redis.NewClient(&redis.Options{
		Addr: redisAddr,
	})

	// Verificar conexão com o Redis
	ctxRedis, cancelRedis := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelRedis()
	if err := rdb.Ping(ctxRedis).Err(); err != nil {
		log.Fatalf("Não foi possível conectar ao Redis em %s: %v", redisAddr, err)
	}
	log.Println("Conectado ao Redis com sucesso!")

	// Criar store e rate limiter
	store := redisStore.NewRedisStore(rdb)
	rl := rateLimiter.NewRateLimiter(configRateLimiter, store)

	// Configurar servidor HTTP
	router := http.NewServeMux()
	router.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprintln(w, "Olá! Este é um endpoint de teste do Rate Limiter.")
	})

	// Aplicar o middleware de rate limiting
	protectedHandler := middleware.RateLimit(rl)(router)

	serverPort := os.Getenv("SERVER_PORT")
	if serverPort == "" {
		serverPort = "8080"
	}

	srv := &http.Server{
		Addr:         ":" + serverPort,
		Handler:      protectedHandler,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// Goroutine para escutar por sinais de shutdown
	go func() {
		quit := make(chan os.Signal, 1)
		signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
		<-quit
		log.Println("Servidor recebendo sinal de desligamento...")

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		if err := srv.Shutdown(ctx); err != nil {
			log.Fatalf("Erro no desligamento gracioso do servidor: %v", err)
		}
		log.Println("Servidor desligado graciosamente.")
		store.Close() // Fechar conexão com Redis
		log.Println("Conexão com Redis fechada.")
	}()

	log.Printf("Servidor escutando na porta %s...", serverPort)
	if err := srv.ListenAndServe(); err != http.ErrServerClosed {
		log.Fatalf("Erro ao iniciar servidor HTTP: %v", err)
	}

	log.Println("Servidor parou.")
}
