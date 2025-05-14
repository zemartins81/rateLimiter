package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"rateLimiter/cmd/server/config"
	redisStore "rateLimiter/infra/db/redis"
	"rateLimiter/internal/rateLimiter"
)

// Mock do RateLimiter para testes unitários
type mockRateLimiter struct {
	mock.Mock
}

func (m *mockRateLimiter) Allow(ctx context.Context, identifier string, isToken bool) (bool, error) {
	args := m.Called(ctx, identifier, isToken)
	return args.Bool(0), args.Error(1)
}

func (m *mockRateLimiter) GetConfig() *config.LimiterConfig {
	args := m.Called()
	return args.Get(0).(*config.LimiterConfig)
}

// Test_RateLimit_Middleware_Token testa o middleware com token
func Test_RateLimit_Middleware_Token(t *testing.T) {
	// Criar mock do RateLimiter
	mockRL := new(mockRateLimiter)

	// Configurar o mock para retornar uma configuração
	mockRL.On("GetConfig").Return(&config.LimiterConfig{
		TokenHeaderName: "API_KEY",
	})

	// Configurar o mock para permitir a requisição
	mockRL.On("Allow", mock.Anything, "test-token", true).Return(true, nil)

	// Criar servidor HTTP e handler de teste
	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	middleware := RateLimit(mockRL)(nextHandler)

	// Criar requisição com token
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("API_KEY", "test-token")
	rec := httptest.NewRecorder()

	// Executar o middleware
	middleware.ServeHTTP(rec, req)

	// Verificar resultado
	assert.Equal(t, http.StatusOK, rec.Code)
	mockRL.AssertExpectations(t)
}

// Test_RateLimit_Middleware_IP testa o middleware com IP
func Test_RateLimit_Middleware_IP(t *testing.T) {
	// Criar mock do RateLimiter
	mockRL := new(mockRateLimiter)

	// Configurar o mock para retornar uma configuração
	mockRL.On("GetConfig").Return(&config.LimiterConfig{
		TokenHeaderName: "API_KEY",
	})

	// Configurar o mock para permitir a requisição
	mockRL.On("Allow", mock.Anything, "192.0.2.1", false).Return(true, nil)

	// Criar servidor HTTP e handler de teste
	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	middleware := RateLimit(mockRL)(nextHandler)

	// Criar requisição com IP
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "192.0.2.1:12345"
	rec := httptest.NewRecorder()

	// Executar o middleware
	middleware.ServeHTTP(rec, req)

	// Verificar resultado
	assert.Equal(t, http.StatusOK, rec.Code)
	mockRL.AssertExpectations(t)
}

// Test_RateLimit_Middleware_Blocked testa o middleware quando o rate limiter bloqueia a requisição
func Test_RateLimit_Middleware_Blocked(t *testing.T) {
	// Criar mock do RateLimiter
	mockRL := new(mockRateLimiter)

	// Configurar o mock para retornar uma configuração
	mockRL.On("GetConfig").Return(&config.LimiterConfig{
		TokenHeaderName: "API_KEY",
	})

	// Configurar o mock para bloquear a requisição
	mockRL.On("Allow", mock.Anything, "192.0.2.2", false).Return(false, nil)

	// Criar servidor HTTP e handler de teste
	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	middleware := RateLimit(mockRL)(nextHandler)

	// Criar requisição com IP
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "192.0.2.2:12345"
	rec := httptest.NewRecorder()

	// Executar o middleware
	middleware.ServeHTTP(rec, req)

	// Verificar que a resposta é 429 Too Many Requests
	assert.Equal(t, http.StatusTooManyRequests, rec.Code)
	mockRL.AssertExpectations(t)
}

// Mock para o Redis Store para teste de integração
type redisStoreMock struct {
	client *redis.Client
}

func (rs *redisStoreMock) Increment(ctx context.Context, key string, window time.Duration) (int64, error) {
	pipe := rs.client.Pipeline()
	incr := pipe.Incr(ctx, key)
	pipe.Expire(ctx, key, window)
	_, err := pipe.Exec(ctx)
	if err != nil {
		return 0, err
	}
	return incr.Val(), nil
}

func (rs *redisStoreMock) IsBlocked(ctx context.Context, key string) (bool, error) {
	val, err := rs.client.Get(ctx, key).Result()
	if err == redis.Nil {
		return false, nil
	} else if err != nil {
		return false, err
	}
	return val == "blocked", nil
}

func (rs *redisStoreMock) Block(ctx context.Context, key string, duration time.Duration) error {
	return rs.client.Set(ctx, key, "blocked", duration).Err()
}

func (rs *redisStoreMock) Reset(ctx context.Context, key string) error {
	return rs.client.Del(ctx, key).Err()
}

func (rs *redisStoreMock) Close() error {
	return rs.client.Close()
}

// Test_RateLimit_Integration testa o middleware integrado com um rate limiter real
func Test_RateLimit_Integration(t *testing.T) {
	// Configurar Redis para teste
	mr, err := miniredis.Run()
	require.NoError(t, err)
	defer mr.Close()

	client := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})
	defer client.Close()

	// Configurar rate limiter
	cfg := &config.LimiterConfig{
		MaxRequestsPerIP:          3,
		MaxRequestsPerToken:       5,
		BlockDurationIPSeconds:    10,
		BlockDurationTokenSeconds: 10,
		TokenHeaderName:           "API_KEY",
	}

	// Criar store e rate limiter real
	store := redisStore.NewRedisStore(client)
	rl := rateLimiter.NewRateLimiter(cfg, store)

	// Criar servidor HTTP e handler de teste
	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	middleware := RateLimit(rl)(nextHandler)

	// Teste com IP: 3 requisições permitidas, a 4ª bloqueada
	for i := 0; i < 3; i++ {
		req := httptest.NewRequest("GET", "/", nil)
		req.RemoteAddr = "192.0.2.3:12345"
		rec := httptest.NewRecorder()

		middleware.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusOK, rec.Code, "Requisição %d deveria ser permitida", i+1)
	}

	// 4ª requisição deve ser bloqueada
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "192.0.2.3:12345"
	rec := httptest.NewRecorder()

	middleware.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusTooManyRequests, rec.Code, "Requisição após o limite deveria ser bloqueada")

	// Teste com token: 5 requisições permitidas, a 6ª bloqueada
	for i := 0; i < 5; i++ {
		req := httptest.NewRequest("GET", "/", nil)
		req.Header.Set("API_KEY", "test-token-abc")
		rec := httptest.NewRecorder()

		middleware.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusOK, rec.Code, "Requisição %d deveria ser permitida", i+1)
	}

	// 6ª requisição deve ser bloqueada
	req = httptest.NewRequest("GET", "/", nil)
	req.Header.Set("API_KEY", "test-token-abc")
	rec = httptest.NewRecorder()

	middleware.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusTooManyRequests, rec.Code, "Requisição após o limite deveria ser bloqueada")

	// Avançar o tempo do Redis além do período de bloqueio
	mr.FastForward(15 * time.Second)

	// Testar novamente com o mesmo token após a expiração do bloqueio
	req = httptest.NewRequest("GET", "/", nil)
	req.Header.Set("API_KEY", "test-token-abc")
	rec = httptest.NewRecorder()

	middleware.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code, "Requisição após o período de bloqueio deveria ser permitida")
}

// Test_RateLimit_DifferentIPsAndTokens testa se o rate limiter trata diferentes IPs e tokens de forma independente
func Test_RateLimit_DifferentIPsAndTokens(t *testing.T) {
	// Configurar Redis para teste
	mr, err := miniredis.Run()
	require.NoError(t, err)
	defer mr.Close()

	client := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})
	defer client.Close()

	// Configurar rate limiter
	cfg := &config.LimiterConfig{
		MaxRequestsPerIP:          3,
		MaxRequestsPerToken:       3,
		BlockDurationIPSeconds:    10,
		BlockDurationTokenSeconds: 10,
		TokenHeaderName:           "API_KEY",
	}

	// Criar store e rate limiter real
	store := redisStore.NewRedisStore(client)
	rl := rateLimiter.NewRateLimiter(cfg, store)

	// Criar servidor HTTP e handler de teste
	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	middleware := RateLimit(rl)(nextHandler)

	// Teste com dois IPs diferentes
	ip1 := "192.0.2.10:12345"
	ip2 := "192.0.2.11:12345"

	// Exceder o limite para o primeiro IP
	for i := 0; i < 4; i++ {
		req := httptest.NewRequest("GET", "/", nil)
		req.RemoteAddr = ip1
		rec := httptest.NewRecorder()

		middleware.ServeHTTP(rec, req)

		// A última requisição deve ser bloqueada
		if i == 3 {
			assert.Equal(t, http.StatusTooManyRequests, rec.Code, "Requisição após o limite para IP1 deveria ser bloqueada")
		}
	}

	// O segundo IP ainda deve funcionar normalmente
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = ip2
	rec := httptest.NewRecorder()

	middleware.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code, "Requisição do IP2 deveria ser permitida mesmo com IP1 bloqueado")

	// Teste com dois tokens diferentes
	token1 := "token-test-1"
	token2 := "token-test-2"

	// Exceder o limite para o primeiro token
	for i := 0; i < 4; i++ {
		req := httptest.NewRequest("GET", "/", nil)
		req.Header.Set("API_KEY", token1)
		rec := httptest.NewRecorder()

		middleware.ServeHTTP(rec, req)

		// A última requisição deve ser bloqueada
		if i == 3 {
			assert.Equal(t, http.StatusTooManyRequests, rec.Code, "Requisição após o limite para Token1 deveria ser bloqueada")
		}
	}

	// O segundo token ainda deve funcionar normalmente
	req = httptest.NewRequest("GET", "/", nil)
	req.Header.Set("API_KEY", token2)
	rec = httptest.NewRecorder()

	middleware.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code, "Requisição do Token2 deveria ser permitida mesmo com Token1 bloqueado")
}
