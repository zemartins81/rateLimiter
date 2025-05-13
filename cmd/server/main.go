package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/go-redis/redis/v8"
	"golang.org/x/net/context"
)

type RateLimiter struct {
	client  *redis.Client
	limit   int
	window  time.Duration
	context context.Context
}

func NewRatelimiter(
	client *redis.Client,
	limit int,
	window time.Duration,
) (rateLimiter *RateLimiter) {
	return &RateLimiter{
		client:  client,
		limit:   limit,
		window:  window,
		context: context.Background(),
	}

}

func (rl *RateLimiter) Allow(key string) bool {
	pipe := rl.client.TxPipeline()

	incr := pipe.Incr(rl.context, key)
	pipe.Expire(rl.context, key, rl.window)

	cmder, err := pipe.Exec(rl.context)
	if err != nil {
		panic(err)
	}

	return incr.Val() <= int64(rl.limit)

}

func rateLimiterMd(rl *RateLimiter, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		clientIP, _, _ := net.SplitHostPort(r.RemoteAddr)
		if !rl.Allow(clientIP) {
			http.Error(w, "Too many requests", http.StatusTooManyRequests)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func main() {

	client := redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
	})
	defer client.Close()

	rateLimiter := NewRatelimiter(client, 10, 1*time.Minute)

	router := http.NewServeMux()
	router.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "Hello World!")
	})

	handler := rateLimiterMd(rateLimiter, router)

	http.ListenAndServe(":8080", handler)

}
