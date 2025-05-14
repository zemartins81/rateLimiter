package rateLimiter
package rateLimiter

import (
	"context"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
	"github.com/joho/godotenv"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rateLimiter/cmd/server/config"
	redisStore "rateLimiter/infra/db/redis"
)

func init() {
	// Tentar carregar variáveis de ambiente do arquivo .env, ignorando erros
	_ = godotenv.Load()
}

// getEnvInt obtém um valor inteiro de uma variável de ambiente ou retorna um valor padrão
func getEnvInt(key string, defaultValue int) int {
	if value, exists := os.LookupEnv(key); exists {
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		}
	}
	return defaultValue
}

// setupTestRedis configura um servidor Redis em memória para testes
func setupTestRedis(t *testing.T) (*miniredis.Miniredis, *redis.Client) {
	mr, err := miniredis.Run()
	require.NoError(t, err)

	client := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})

	return mr, client
}

// createTestRateLimiter cria um rate limiter com configurações baseadas em variáveis de ambiente ou valores padrão
func createTestRateLimiter(client *redis.Client) *RateLimiter {
	maxIP := getEnvInt("MAX_REQUESTS_PER_IP", 5)
	maxToken := getEnvInt("MAX_REQUESTS_PER_TOKEN", 10)
	blockDurationIP := getEnvInt("BLOCK_DURATION_IP_SECONDS", 60)
	blockDurationToken := getEnvInt("BLOCK_DURATION_TOKEN_SECONDS", 120)
	tokenHeaderName := os.Getenv("TOKEN_HEADER_NAME")
	if tokenHeaderName == "" {
		tokenHeaderName = "API_KEY"
	}

	cfg := &config.LimiterConfig{
		MaxRequestsPerIP:          maxIP,
		MaxRequestsPerToken:       maxToken,
		BlockDurationIPSeconds:    blockDurationIP,
		BlockDurationTokenSeconds: blockDurationToken,
		TokenHeaderName:           tokenHeaderName,
	}

	store := redisStore.NewRedisStore(client)
	return NewRateLimiter(cfg, store)
}

// createTestRateLimiterWithConfig cria um rate limiter com configurações específicas para testes
func createTestRateLimiterWithConfig(client *redis.Client, maxIP, maxToken, blockDurationIP, blockDurationToken int) *RateLimiter {
	cfg := &config.LimiterConfig{
		MaxRequestsPerIP:          maxIP,
		MaxRequestsPerToken:       maxToken,
		BlockDurationIPSeconds:    blockDurationIP,
		BlockDurationTokenSeconds: blockDurationToken,
		TokenHeaderName:           "API_KEY",
	}

	store := redisStore.NewRedisStore(client)
	return NewRateLimiter(cfg, store)
}

// Test_RateLimiter_IP_Limiting verifica se o rate limiting por IP funciona corretamente
func Test_RateLimiter_IP_Limiting(t *testing.T) {
	mr, client := setupTestRedis(t)
	defer mr.Close()
	defer client.Close()

	// Obter configurações do ambiente ou usar valores padrão
	maxIP := getEnvInt("MAX_REQUESTS_PER_IP", 5)
	blockDurationIP := getEnvInt("BLOCK_DURATION_IP_SECONDS", 60)

	// Criar rate limiter com configurações do ambiente
	rl := createTestRateLimiter(client)
	ctx := context.Background()
	testIP := "192.168.1.1"
	isToken := false

	t.Logf("Testando limite por IP: %d requisições, bloqueio de %d segundos", maxIP, blockDurationIP)

	// As requisições até o limite devem ser permitidas
	for i := 0; i < maxIP; i++ {
		allowed, err := rl.Allow(ctx, testIP, isToken)
		assert.NoError(t, err)
		assert.True(t, allowed, "Requisição %d deveria ser permitida", i+1)
	}

	// A próxima requisição deve ser bloqueada
	allowed, err := rl.Allow(ctx, testIP, isToken)
	assert.NoError(t, err)
	assert.False(t, allowed, "A requisição após o limite deveria ser bloqueada")

	// Verificar se há uma chave de bloqueio no Redis
	blockedKey := "blocked_ip_" + testIP
	val, err := client.Get(ctx, blockedKey).Result()
	assert.NoError(t, err)
	assert.Equal(t, "blocked", val)
	
	// Verificar o TTL da chave de bloqueio
	ttl, err := client.TTL(ctx, blockedKey).Result()
	assert.NoError(t, err)
	assert.True(t, ttl > 0 && ttl <= time.Duration(blockDurationIP)*time.Second, 
		"TTL deveria ser aproximadamente %d segundos", blockDurationIP)
}

// Test_RateLimiter_Token_Limiting verifica se o rate limiting por token funciona corretamente
func Test_RateLimiter_Token_Limiting(t *testing.T) {
	mr, client := setupTestRedis(t)
	defer mr.Close()
	defer client.Close()

	// Obter configurações do ambiente ou usar valores padrão
	maxToken := getEnvInt("MAX_REQUESTS_PER_TOKEN", 10)
	blockDurationToken := getEnvInt("BLOCK_DURATION_TOKEN_SECONDS", 120)

	// Criar rate limiter com configurações do ambiente
	rl := createTestRateLimiter(client)
	ctx := context.Background()
	testToken := "abc123"
	isToken := true

	t.Logf("Testando limite por token: %d requisições, bloqueio de %d segundos", maxToken, blockDurationToken)

	// As requisições até o limite devem ser permitidas
	for i := 0; i < maxToken; i++ {
		allowed, err := rl.Allow(ctx, testToken, isToken)
		assert.NoError(t, err)
		assert.True(t, allowed, "Requisição %d deveria ser permitida", i+1)
	}

	// A requisição após o limite deve ser bloqueada
	allowed, err := rl.Allow(ctx, testToken, isToken)
	assert.NoError(t, err)
	assert.False(t, allowed, "A requisição após o limite deveria ser bloqueada")

	// Verificar se há uma chave de bloqueio no Redis
	blockedKey := "blocked_token_" + testToken
	val, err := client.Get(ctx, blockedKey).Result()
	assert.NoError(t, err)
	assert.Equal(t, "blocked", val)
	
	// Verificar o TTL da chave de bloqueio para token
	ttl, err := client.TTL(ctx, blockedKey).Result()
	assert.NoError(t, err)
	assert.True(t, ttl > 0 && ttl <= time.Duration(blockDurationToken)*time.Second, 
		"TTL deveria ser aproximadamente %d segundos", blockDurationToken)
}

// Test_RateLimiter_Block_Duration verifica se o rate limiter respeita o tempo de bloqueio
func Test_RateLimiter_Block_Duration(t *testing.T) {
	mr, client := setupTestRedis(t)
	defer mr.Close()
	defer client.Close()

	// Criar rate limiter com bloqueio de 5 segundos para teste
	blockDuration := 5
	rl := createTestRateLimiterWithConfig(client, 3, 3, blockDuration, blockDuration)
	ctx := context.Background()
	testIP := "192.168.1.2"
	isToken := false

	// Exceder o limite para provocar bloqueio
	for i := 0; i < 4; i++ {
		rl.Allow(ctx, testIP, isToken)
	}

	// Verificar se está bloqueado
	allowed, err := rl.Allow(ctx, testIP, isToken)
	assert.NoError(t, err)
	assert.False(t, allowed, "Requisição deveria estar bloqueada")

	// Avançar o tempo do Redis em 3 segundos (ainda bloqueado)
	mr.FastForward(3 * time.Second)
	
	allowed, err = rl.Allow(ctx, testIP, isToken)
	assert.NoError(t, err)
	assert.False(t, allowed, "Requisição ainda deveria estar bloqueada após 3s")

	// Avançar mais 3 segundos (passaram-se 6s no total, bloqueio deve ter expirado)
	mr.FastForward(3 * time.Second)
	
	allowed, err = rl.Allow(ctx, testIP, isToken)
	assert.NoError(t, err)
	assert.True(t, allowed, "Requisição deveria ser permitida após o período de bloqueio")
}

// Test_RateLimiter_DifferentIdentifiers verifica se o rate limiter trata diferentes IPs/tokens separadamente
func Test_RateLimiter_DifferentIdentifiers(t *testing.T) {
	mr, client := setupTestRedis(t)
	defer mr.Close()
	defer client.Close()

	// Obter configurações do ambiente ou usar valores padrão
	maxIP := getEnvInt("MAX_REQUESTS_PER_IP", 3)
	maxToken := getEnvInt("MAX_REQUESTS_PER_TOKEN", 3)
	
	// Criar rate limiter com configurações do ambiente
	rl := createTestRateLimiter(client)
	ctx := context.Background()
	
	// Testar com dois IPs diferentes
	ip1 := "192.168.1.10"
	ip2 := "192.168.1.11"
	isToken := false

	t.Logf("Testando independência entre identificadores com limites: IP=%d, Token=%d", maxIP, maxToken)

	// Exceder o limite para o primeiro IP
	for i := 0; i < maxIP+1; i++ {
		allowed, _ := rl.Allow(ctx, ip1, isToken)
		if i == maxIP {
			assert.False(t, allowed, "Requisição após limite para IP1 deveria ser bloqueada")
		}
	}
	
	// Verificar se IP1 está bloqueado
	allowed, err := rl.Allow(ctx, ip1, isToken)
	assert.NoError(t, err)
	assert.False(t, allowed, "IP1 deveria estar bloqueado")
	
	// Verificar se IP2 ainda está permitido
	allowed, err = rl.Allow(ctx, ip2, isToken)
	assert.NoError(t, err)
	assert.True(t, allowed, "IP2 deveria estar permitido mesmo com IP1 bloqueado")
	
	// O mesmo deve acontecer para tokens
	token1 := "token1"
	token2 := "token2"
	isToken = true
	
	// Exceder o limite para o primeiro token
	for i := 0; i < maxToken+1; i++ {
		allowed, _ := rl.Allow(ctx, token1, isToken)
		if i == maxToken {
			assert.False(t, allowed, "Requisição após limite para Token1 deveria ser bloqueada")
		}
	}
	
	// Verificar se token1 está bloqueado
	allowed, err = rl.Allow(ctx, token1, isToken)
	assert.NoError(t, err)
	assert.False(t, allowed, "Token1 deveria estar bloqueado")
	
	// Verificar se token2 ainda está permitido
	allowed, err = rl.Allow(ctx, token2, isToken)
	assert.NoError(t, err)
	assert.True(t, allowed, "Token2 deveria estar permitido mesmo com Token1 bloqueado")
}

// Test_RateLimiter_Config_Override verifica se o rate limiter usa corretamente as configurações
// definidas nas variáveis de ambiente
func Test_RateLimiter_Config_Override(t *testing.T) {
	// Salvar variáveis de ambiente atuais
	oldMaxIP := os.Getenv("MAX_REQUESTS_PER_IP")
	oldMaxToken := os.Getenv("MAX_REQUESTS_PER_TOKEN")
	
	// Restaurar variáveis de ambiente ao final do teste
	defer func() {
		if oldMaxIP != "" {
			os.Setenv("MAX_REQUESTS_PER_IP", oldMaxIP)
		} else {
			os.Unsetenv("MAX_REQUESTS_PER_IP")
		}
		if oldMaxToken != "" {
			os.Setenv("MAX_REQUESTS_PER_TOKEN", oldMaxToken)
		} else {
			os.Unsetenv("MAX_REQUESTS_PER_TOKEN")
		}
	}()
	
	// Definir valores personalizados para este teste
	os.Setenv("MAX_REQUESTS_PER_IP", "2")
	os.Setenv("MAX_REQUESTS_PER_TOKEN", "3")
	
	mr, client := setupTestRedis(t)
	defer mr.Close()
	defer client.Close()
	
	// Criar rate limiter que usa as variáveis de ambiente modificadas
	rl := createTestRateLimiter(client)
	ctx := context.Background()
	
	// Testar limite de IP com o novo valor (2)
	testIP := "192.168.1.20"
	isToken := false
	
	// Duas requisições devem ser permitidas
	for i := 0; i < 2; i++ {
		allowed, err := rl.Allow(ctx, testIP, isToken)
		assert.NoError(t, err)
		assert.True(t, allowed, "Requisição %d deveria ser permitida com novo limite", i+1)
	}
	
	// A terceira requisição deve ser bloqueada (novo limite é 2)
	allowed, err := rl.Allow(ctx, testIP, isToken)
	assert.NoError(t, err)
	assert.False(t, allowed, "Terceira requisição deveria ser bloqueada com novo limite de IP=2")
	
	// Testar limite de token com o novo valor (3)
	testToken := "token-test-custom"
	isToken = true
	
	// Três requisições devem ser permitidas
	for i := 0; i < 3; i++ {
		allowed, err := rl.Allow(ctx, testToken, isToken)
		assert.NoError(t, err)
		assert.True(t, allowed, "Requisição %d deveria ser permitida com novo limite de token", i+1)
	}
	
	// A quarta requisição deve ser bloqueada (novo limite é 3)
	allowed, err = rl.Allow(ctx, testToken, isToken)
	assert.NoError(t, err)
	assert.False(t, allowed, "Quarta requisição deveria ser bloqueada com novo limite de token=3")
}

// Test_RateLimiter_TokenHeader verifica se o nome do header do token é reconhecido corretamente
func Test_RateLimiter_TokenHeader(t *testing.T) {
	// Salvar valor atual da variável de ambiente
	oldHeaderName := os.Getenv("TOKEN_HEADER_NAME")
	
	// Restaurar ao final do teste
	defer func() {
		if oldHeaderName != "" {
			os.Setenv("TOKEN_HEADER_NAME", oldHeaderName)
		} else {
			os.Unsetenv("TOKEN_HEADER_NAME")
		}
	}()
	
	// Definir um nome de header personalizado
	customHeader := "X-CUSTOM-API-KEY"
	os.Setenv("TOKEN_HEADER_NAME", customHeader)
	
	mr, client := setupTestRedis(t)
	defer mr.Close()
	defer client.Close()
	
	// Criar rate limiter com o novo header
	rl := createTestRateLimiter(client)
	
	// Verificar se a configuração foi aplicada
	config := rl.GetConfig()
	assert.Equal(t, customHeader, config.TokenHeaderName, 
		"O nome do header de token deveria refletir a variável de ambiente")
}

// Test_RateLimiter_Error_Handling verifica se o rate limiter lida corretamente com erros do Redis
func Test_RateLimiter_Error_Handling(t *testing.T) {
	mr, client := setupTestRedis(t)
	rl := createTestRateLimiter(client)
	ctx := context.Background()
	
	// Simular um erro fechando o servidor Redis
	mr.Close()
	
	// Tentar executar uma operação após o Redis ser fechado
	_, err := rl.Allow(ctx, "192.168.1.100", false)
	assert.Error(t, err, "Deveria retornar um erro quando o Redis está indisponível")
	
	// A mensagem de erro deve ser clara
	if err != nil {
		assert.Contains(t, err.Error(), "erro ao", 
			"A mensagem de erro deve explicar qual operação falhou")
	}
}
import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rateLimiter/cmd/server/config"
	redisStore "rateLimiter/infra/db/redis"
)

// setupTestRedis configura um servidor Redis em memória para testes
func setupTestRedis(t *testing.T) (*miniredis.Miniredis, *redis.Client) {
	mr, err := miniredis.Run()
	require.NoError(t, err)

	client := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})

	return mr, client
}

// createTestRateLimiter cria um rate limiter com configurações de teste
func createTestRateLimiter(client *redis.Client, maxIP, maxToken, blockDurationIP, blockDurationToken int) *RateLimiter {
	cfg := &config.LimiterConfig{
		MaxRequestsPerIP:          maxIP,
		MaxRequestsPerToken:       maxToken,
		BlockDurationIPSeconds:    blockDurationIP,
		BlockDurationTokenSeconds: blockDurationToken,
		TokenHeaderName:           "API_KEY",
	}

	store := redisStore.NewRedisStore(client)
	return NewRateLimiter(cfg, store)
}

// Test_RateLimiter_IP_Limiting verifica se o rate limiting por IP funciona corretamente
func Test_RateLimiter_IP_Limiting(t *testing.T) {
	mr, client := setupTestRedis(t)
	defer mr.Close()
	defer client.Close()

	// Criar rate limiter com máximo de 5 requisições por IP
	rl := createTestRateLimiter(client, 5, 10, 60, 60)
	ctx := context.Background()
	testIP := "192.168.1.1"
	isToken := false

	// As primeiras 5 requisições devem ser permitidas
	for i := 0; i < 5; i++ {
		allowed, err := rl.Allow(ctx, testIP, isToken)
		assert.NoError(t, err)
		assert.True(t, allowed, "Requisição %d deveria ser permitida", i+1)
	}

	// A sexta requisição deve ser bloqueada
	allowed, err := rl.Allow(ctx, testIP, isToken)
	assert.NoError(t, err)
	assert.False(t, allowed, "A sexta requisição deveria ser bloqueada")

	// Verificar se há uma chave de bloqueio no Redis
	blockedKey := "blocked_ip_" + testIP
	val, err := client.Get(ctx, blockedKey).Result()
	assert.NoError(t, err)
	assert.Equal(t, "blocked", val)
	
	// Verificar o TTL da chave de bloqueio
	ttl, err := client.TTL(ctx, blockedKey).Result()
	assert.NoError(t, err)
	assert.True(t, ttl > 0 && ttl <= 60*time.Second, "TTL deveria ser aproximadamente 60 segundos")
}

// Test_RateLimiter_Token_Limiting verifica se o rate limiting por token funciona corretamente
func Test_RateLimiter_Token_Limiting(t *testing.T) {
	mr, client := setupTestRedis(t)
	defer mr.Close()
	defer client.Close()

	// Criar rate limiter com máximo de 10 requisições por token
	rl := createTestRateLimiter(client, 5, 10, 60, 120)
	ctx := context.Background()
	testToken := "abc123"
	isToken := true

	// As primeiras 10 requisições devem ser permitidas
	for i := 0; i < 10; i++ {
		allowed, err := rl.Allow(ctx, testToken, isToken)
		assert.NoError(t, err)
		assert.True(t, allowed, "Requisição %d deveria ser permitida", i+1)
	}

	// A 11ª requisição deve ser bloqueada
	allowed, err := rl.Allow(ctx, testToken, isToken)
	assert.NoError(t, err)
	assert.False(t, allowed, "A 11ª requisição deveria ser bloqueada")

	// Verificar se há uma chave de bloqueio no Redis
	blockedKey := "blocked_token_" + testToken
	val, err := client.Get(ctx, blockedKey).Result()
	assert.NoError(t, err)
	assert.Equal(t, "blocked", val)
	
	// Verificar o TTL da chave de bloqueio para token (deve ser 120s)
	ttl, err := client.TTL(ctx, blockedKey).Result()
	assert.NoError(t, err)
	assert.True(t, ttl > 0 && ttl <= 120*time.Second, "TTL deveria ser aproximadamente 120 segundos")
}

// Test_RateLimiter_Block_Duration verifica se o rate limiter respeita o tempo de bloqueio
func Test_RateLimiter_Block_Duration(t *testing.T) {
	mr, client := setupTestRedis(t)
	defer mr.Close()
	defer client.Close()

	// Criar rate limiter com bloqueio de 5 segundos para teste
	blockDuration := 5
	rl := createTestRateLimiter(client, 3, 3, blockDuration, blockDuration)
	ctx := context.Background()
	testIP := "192.168.1.2"
	isToken := false

	// Exceder o limite para provocar bloqueio
	for i := 0; i < 4; i++ {
		rl.Allow(ctx, testIP, isToken)
	}

	// Verificar se está bloqueado
	allowed, err := rl.Allow(ctx, testIP, isToken)
	assert.NoError(t, err)
	assert.False(t, allowed, "Requisição deveria estar bloqueada")

	// Avançar o tempo do Redis em 3 segundos (ainda bloqueado)
	mr.FastForward(3 * time.Second)
	
	allowed, err = rl.Allow(ctx, testIP, isToken)
	assert.NoError(t, err)
	assert.False(t, allowed, "Requisição ainda deveria estar bloqueada após 3s")

	// Avançar mais 3 segundos (passaram-se 6s no total, bloqueio deve ter expirado)
	mr.FastForward(3 * time.Second)
	
	allowed, err = rl.Allow(ctx, testIP, isToken)
	assert.NoError(t, err)
	assert.True(t, allowed, "Requisição deveria ser permitida após o período de bloqueio")
}

// Test_RateLimiter_DifferentIdentifiers verifica se o rate limiter trata diferentes IPs/tokens separadamente
func Test_RateLimiter_DifferentIdentifiers(t *testing.T) {
	mr, client := setupTestRedis(t)
	defer mr.Close()
	defer client.Close()

	rl := createTestRateLimiter(client, 3, 3, 60, 60)
	ctx := context.Background()
	
	// Testar com dois IPs diferentes
	ip1 := "192.168.1.10"
	ip2 := "192.168.1.11"
	isToken := false

	// Exceder o limite para o primeiro IP
	for i := 0; i < 4; i++ {
		rl.Allow(ctx, ip1, isToken)
	}
	
	// Verificar se IP1 está bloqueado
	allowed, err := rl.Allow(ctx, ip1, isToken)
	assert.NoError(t, err)
	assert.False(t, allowed, "IP1 deveria estar bloqueado")
	
	// Verificar se IP2 ainda está permitido
	allowed, err = rl.Allow(ctx, ip2, isToken)
	assert.NoError(t, err)
	assert.True(t, allowed, "IP2 deveria estar permitido")
	
	// O mesmo deve acontecer para tokens
	token1 := "token1"
	token2 := "token2"
	isToken = true
	
	// Exceder o limite para o primeiro token
	for i := 0; i < 4; i++ {
		rl.Allow(ctx, token1, isToken)
	}
	
	// Verificar se token1 está bloqueado
	allowed, err = rl.Allow(ctx, token1, isToken)
	assert.NoError(t, err)
	assert.False(t, allowed, "Token1 deveria estar bloqueado")
	
	// Verificar se token2 ainda está permitido
	allowed, err = rl.Allow(ctx, token2, isToken)
	assert.NoError(t, err)
	assert.True(t, allowed, "Token2 deveria estar permitido")
}
