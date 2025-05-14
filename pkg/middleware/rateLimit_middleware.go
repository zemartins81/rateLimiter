package middleware

import (
	"context"
	"log"
	"net"
	"net/http"
	"rateLimiter/internal/rateLimiter"
)

// RateLimit é o middleware que aplica o rate limiting.
func RateLimit(rl rateLimiter.RateLimiterInterface) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := context.Background()
			var identifier string
			var isToken bool

			// Tenta obter o token do header
			token := r.Header.Get(rl.GetConfig().TokenHeaderName)

			if token != "" {
				identifier = token
				isToken = true

			} else {
				// Se não houver token, usa o IP
				clientIP, _, err := net.SplitHostPort(r.RemoteAddr)
				if err != nil {
					log.Printf("Erro ao obter o IP do cliente: %v", err)
					http.Error(w, "Erro interno do servidor", http.StatusInternalServerError)
					return
				}

				identifier = clientIP
				isToken = false
			}

			allowed, err := rl.Allow(ctx, identifier, isToken)
			if err != nil {
				log.Printf("Erro ao verificar o rate limit para %s (token: %t): %v", identifier, isToken, err)
				http.Error(w, "Erro interno do servidor", http.StatusInternalServerError)
				return
			}

			if !allowed {
				w.Header().Set("Content-Type", "text/plain; charset=utf-8")
				w.WriteHeader(http.StatusTooManyRequests) // Código HTTP 429
				_, _ = w.Write([]byte("you have reached the maximum number of requests or actions allowed within a certain time frame"))
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
