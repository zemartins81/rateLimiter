package integration

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rateLimiter/cmd/server/config"
	redisStore "rateLimiter/infra/db/redis"
	"rateLimiter/internal/rateLimiter"
	"rateLimiter/pkg/middleware"
)

// setupIntegrationTest configura o ambiente para testes de integração
func setupIntegrationTest(t *testing.T, maxIP, maxToken, blockDurationIP, blockDurationToken int) (*miniredis.Miniredis, http.Handler, *redis.Client) {
	// Iniciar um servidor Redis em memória
	mr, err := miniredis.Run()
	require.NoError(t, err)

	// Configurar cliente Redis
	client := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})

	// Configurar rate limiter
	cfg := &config.LimiterConfig{
		MaxRequestsPerIP:          maxIP,
		MaxRequestsPerToken:       maxToken,
		BlockDurationIPSeconds:    blockDurationIP,
		BlockDurationTokenSeconds: blockDurationToken,
		TokenHeaderName:           "API_KEY",
	}

	// Criar store e rate limiter
	store := redisStore.NewRedisStore(client)
	rl := rateLimiter.NewRateLimiter(cfg, store)

	// Configurar rota de teste
	router := http.NewServeMux()
	router.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprintln(w, "Teste do Rate Limiter")
	})

	// Aplicar middleware de rate limiting
	protectedHandler := middleware.RateLimit(rl)(router)

	return mr, protectedHandler, client
}

// Test_IP_Rate_Limiting testa o rate limiting por IP em nível de integração
func Test_IP_Rate_Limiting(t *testing.T) {
	maxRequests := 5
	blockDuration := 10
	mr, handler, _ := setupIntegrationTest(t, maxRequests, 10, blockDuration, blockDuration)
	defer mr.Close()

	// Criar servidor HTTP de teste
	server := httptest.NewServer(handler)
	defer server.Close()

	// Criar cliente HTTP com IP fixo para teste
	client := &http.Client{}

	// Fazer requisições dentro do limite
	for i := 1; i <= maxRequests; i++ {
		req, err := http.NewRequest("GET", server.URL, nil)
		require.NoError(t, err)

		// Simular um IP específico através do header X-Forwarded-For
		req.Header.Set("X-Forwarded-For", "192.168.1.1")

		resp, err := client.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode, "Requisição %d deveria ser permitida", i)

		// Ler o corpo da resposta para liberar o TCP keep-alive
		_, _ = io.ReadAll(resp.Body)
	}

	// Fazer uma requisição além do limite
	req, err := http.NewRequest("GET", server.URL, nil)
	require.NoError(t, err)
	req.Header.Set("X-Forwarded-For", "192.168.1.1")

	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	// Verificar que foi bloqueada (429 Too Many Requests)
	assert.Equal(t, http.StatusTooManyRequests, resp.StatusCode, "Requisição após o limite deveria ser bloqueada")
}

// Test_Token_Rate_Limiting testa o rate limiting por token em nível de integração
func Test_Token_Rate_Limiting(t *testing.T) {
	maxIPRequests := 5
	maxTokenRequests := 10
	blockDuration := 10
	mr, handler, _ := setupIntegrationTest(t, maxIPRequests, maxTokenRequests, blockDuration, blockDuration)
	defer mr.Close()

	// Criar servidor HTTP de teste
	server := httptest.NewServer(handler)
	defer server.Close()

	// Criar cliente HTTP
	client := &http.Client{}
	token := "test-token-123"

	// Fazer requisições dentro do limite
	for i := 1; i <= maxTokenRequests; i++ {
		req, err := http.NewRequest("GET", server.URL, nil)
		require.NoError(t, err)

		// Adicionar token de API
		req.Header.Set("API_KEY", token)

		resp, err := client.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode, "Requisição %d deveria ser permitida", i)

		// Ler o corpo da resposta para liberar o TCP keep-alive
		_, _ = io.ReadAll(resp.Body)
	}

	// Fazer uma requisição além do limite
	req, err := http.NewRequest("GET", server.URL, nil)
	require.NoError(t, err)
	req.Header.Set("API_KEY", token)

	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	// Verificar que foi bloqueada (429 Too Many Requests)
	assert.Equal(t, http.StatusTooManyRequests, resp.StatusCode, "Requisição após o limite deveria ser bloqueada")
}

// Test_Block_Duration_Expiration testa se o bloqueio expira corretamente após o tempo definido
func Test_Block_Duration_Expiration(t *testing.T) {
	maxRequests := 5
	blockDuration := 3 // 3 segundos para facilitar o teste
	mr, handler, _ := setupIntegrationTest(t, maxRequests, maxRequests, blockDuration, blockDuration)
	defer mr.Close()

	// Criar servidor HTTP de teste
	server := httptest.NewServer(handler)
	defer server.Close()

	// Criar cliente HTTP
	client := &http.Client{}
	testIP := "192.168.1.5"

	// Exceder o limite de requisições
	for i := 0; i <= maxRequests; i++ {
		req, err := http.NewRequest("GET", server.URL, nil)
		require.NoError(t, err)
		req.Header.Set("X-Forwarded-For", testIP)

		resp, err := client.Do(req)
		require.NoError(t, err)
		_, _ = io.ReadAll(resp.Body)
		resp.Body.Close()

		// A última requisição deve ser bloqueada
		if i == maxRequests {
			assert.Equal(t, http.StatusTooManyRequests, resp.StatusCode, "Requisição após o limite deveria ser bloqueada")
		}
	}

	// Tentar novamente, ainda deve estar bloqueado
	req, err := http.NewRequest("GET", server.URL, nil)
	require.NoError(t, err)
	req.Header.Set("X-Forwarded-For", testIP)

	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusTooManyRequests, resp.StatusCode, "Requisição ainda deveria estar bloqueada")

	// Avançar o tempo do Redis além do período de bloqueio
	mr.FastForward(time.Duration(blockDuration+1) * time.Second)

	// Tentar novamente, agora deveria funcionar
	req, err = http.NewRequest("GET", server.URL, nil)
	require.NoError(t, err)
	req.Header.Set("X-Forwarded-For", testIP)

	resp, err = client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode, "Requisição deveria ser permitida após o tempo de bloqueio")
}

// Test_Concurrent_Requests testa o comportamento com múltiplas requisições concorrentes
func Test_Concurrent_Requests(t *testing.T) {
	maxRequests := 10
	blockDuration := 5
	mr, handler, _ := setupIntegrationTest(t, maxRequests, maxRequests, blockDuration, blockDuration)
	defer mr.Close()

	// Criar servidor HTTP de teste
	server := httptest.NewServer(handler)
	defer server.Close()

	// Criar cliente HTTP
	client := &http.Client{}
	testIP := "192.168.1.10"

	// Realizar 20 requisições concorrentes (com limite de 10)
	totalRequests := 20
	var wg sync.WaitGroup
	responses := make([]int, totalRequests)

	for i := 0; i < totalRequests; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()

			req, err := http.NewRequest("GET", server.URL, nil)
			if err != nil {
				t.Logf("Erro ao criar requisição: %v", err)
				return
			}
			req.Header.Set("X-Forwarded-For", testIP)

			resp, err := client.Do(req)
			if err != nil {
				t.Logf("Erro ao fazer requisição: %v", err)
				return
			}
			defer resp.Body.Close()

			responses[idx] = resp.StatusCode
			_, _ = io.ReadAll(resp.Body)
		}(i)
	}

	wg.Wait()

	// Contar respostas 200 OK e 429 Too Many Requests
	okCount := 0
	tooManyCount := 0

	for _, status := range responses {
		if status == http.StatusOK {
			okCount++
		} else if status == http.StatusTooManyRequests {
			tooManyCount++
		}
	}

	// Verificar que temos até maxRequests respostas OK
	assert.LessOrEqual(t, okCount, maxRequests, "Número de respostas OK não deve exceder o limite configurado")

	// Verificar que temos pelo menos algumas respostas bloqueadas
	assert.GreaterOrEqual(t, tooManyCount, totalRequests-maxRequests, "Devemos ter requisições bloqueadas")

	// Verificar que todas as requisições foram processadas
	assert.Equal(t, totalRequests, okCount+tooManyCount, "Todas as requisições devem retornar 200 ou 429")
}
