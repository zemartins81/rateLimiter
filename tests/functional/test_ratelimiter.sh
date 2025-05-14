#!/bin/bash

# Script para testar o rate limiter em um ambiente real
# Este script demonstra como o rate limiter funciona com diferentes tipos de requisições

# Carregar variáveis do arquivo .env (se existir)
if [ -f .env ]; then
  echo "Carregando configurações do arquivo .env..."
  # Método 1: Usando export e grep para remover comentários
  export $(grep -v '^#' .env | xargs)
fi

# Configurações com valores padrão que podem ser sobrescritas pelas variáveis de ambiente
API_URL="${API_URL:-http://localhost:8080}"
TEST_TOKEN="${TEST_TOKEN:-meu-token-123}"
MAX_REQUESTS_PER_IP="${MAX_REQUESTS_PER_IP:-5}"
MAX_REQUESTS_PER_TOKEN="${MAX_REQUESTS_PER_TOKEN:-10}"
TOKEN_HEADER_NAME="${TOKEN_HEADER_NAME:-API_KEY}"
BLOCK_DURATION_IP_SECONDS="${BLOCK_DURATION_IP_SECONDS:-60}"
BLOCK_DURATION_TOKEN_SECONDS="${BLOCK_DURATION_TOKEN_SECONDS:-120}"

# Cores para saída
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m' # Sem cor

echo -e "${YELLOW}==========================================${NC}"
echo -e "${YELLOW}   TESTES DO RATE LIMITER               ${NC}"
echo -e "${YELLOW}==========================================${NC}"
echo -e "${YELLOW}Configuração Carregada:${NC}"
echo -e "URL da API: ${API_URL}"
echo -e "Limite por IP: ${MAX_REQUESTS_PER_IP} requisições"
echo -e "Limite por Token: ${MAX_REQUESTS_PER_TOKEN} requisições"
echo -e "Nome do Header de Token: ${TOKEN_HEADER_NAME}"
echo -e "Tempo de Bloqueio por IP: ${BLOCK_DURATION_IP_SECONDS} segundos"
echo -e "Tempo de Bloqueio por Token: ${BLOCK_DURATION_TOKEN_SECONDS} segundos"

# Função para fazer uma requisição e verificar o código de status
make_request() {
    local url=$1
    local headers=$2
    local expected_status=$3
    local description=$4

    # Mensagem inicial
    echo -ne "${YELLOW}Teste: ${description}...${NC}"

    # Fazer a requisição
    response=$(curl -s -o /dev/null -w "%{http_code}" -H "$headers" "$url")

    # Verificar resultado
    if [ "$response" -eq "$expected_status" ]; then
        echo -e "${GREEN} OK! ${NC}(Status: $response)"
        return 0
    else
        echo -e "${RED} FALHA! ${NC}(Esperado: $expected_status, Recebido: $response)"
        return 1
    fi
}

echo -e "\n${YELLOW}=== TESTE 1: Limitação por IP ===${NC}"
echo "Fazendo ${MAX_REQUESTS_PER_IP} requisições sem token (limitado por IP)..."

# Fazer requisições até o limite configurado para IP
for i in $(seq 1 $MAX_REQUESTS_PER_IP); do
    make_request "$API_URL/" "" 200 "Requisição $i de IP"
done

# A próxima requisição deve ser bloqueada
make_request "$API_URL/" "" 429 "Requisição $(($MAX_REQUESTS_PER_IP + 1)) de IP (deve ser bloqueada)"

echo -e "\n${YELLOW}=== TESTE 2: Limitação por Token ===${NC}"
echo "Fazendo ${MAX_REQUESTS_PER_TOKEN} requisições com token (limitado por token)..."

# Fazer requisições até o limite configurado para token
for i in $(seq 1 $MAX_REQUESTS_PER_TOKEN); do
    make_request "$API_URL/" "${TOKEN_HEADER_NAME}: $TEST_TOKEN" 200 "Requisição $i com token"
done

# A próxima requisição deve ser bloqueada
make_request "$API_URL/" "${TOKEN_HEADER_NAME}: $TEST_TOKEN" 429 "Requisição $(($MAX_REQUESTS_PER_TOKEN + 1)) com token (deve ser bloqueada)"

echo -e "\n${YELLOW}=== TESTE 3: Independência entre IPs ===${NC}"
echo "Teste com diferentes IPs (simulados através de X-Forwarded-For)..."

# Bloquear o primeiro IP
for i in $(seq 1 $(($MAX_REQUESTS_PER_IP + 1))); do
    make_request "$API_URL/" "X-Forwarded-For: 10.0.0.1" $([[ $i -le $MAX_REQUESTS_PER_IP ]] && echo "200" || echo "429") "Requisição $i do IP 10.0.0.1"
done

# O segundo IP ainda deve funcionar
make_request "$API_URL/" "X-Forwarded-For: 10.0.0.2" 200 "Requisição do IP 10.0.0.2 (deve funcionar)"

echo -e "\n${YELLOW}=== TESTE 4: Independência entre Tokens ===${NC}"
echo "Teste com diferentes tokens..."

# Bloquear o primeiro token
for i in $(seq 1 $(($MAX_REQUESTS_PER_TOKEN + 1))); do
    make_request "$API_URL/" "${TOKEN_HEADER_NAME}: token1" $([[ $i -le $MAX_REQUESTS_PER_TOKEN ]] && echo "200" || echo "429") "Requisição $i do Token1"
done

# O segundo token ainda deve funcionar
make_request "$API_URL/" "${TOKEN_HEADER_NAME}: token2" 200 "Requisição do Token2 (deve funcionar)"

echo -e "\n${YELLOW}=== TESTE 5: Tempo de Bloqueio ===${NC}"
echo "Este teste verificaria o tempo de expiração do bloqueio."
echo "Em um ambiente real, o IP ficaria bloqueado por ${BLOCK_DURATION_IP_SECONDS} segundos."
echo "O token ficaria bloqueado por ${BLOCK_DURATION_TOKEN_SECONDS} segundos."
echo "Em um teste automatizado, podemos usar o miniredis para simular o avanço do tempo."

echo -e "\n${YELLOW}=== TESTE 6: Alta Carga ===${NC}"
if command -v wrk &>/dev/null; then
    echo "Executando teste de carga com wrk..."
    wrk -t4 -c50 -d10s "$API_URL/"
    echo -e "\nAo executar o teste de carga, muitas requisições devem receber código 429 após atingir o limite."
else
    echo "A ferramenta 'wrk' não está instalada. Não é possível executar o teste de carga."
    echo "Instale-a com: sudo apt-get install wrk (Ubuntu/Debian) ou brew install wrk (macOS)"
fi

echo -e "\n${YELLOW}==========================================${NC}"
echo -e "${YELLOW}   TESTES CONCLUÍDOS                    ${NC}"
echo -e "${YELLOW}==========================================${NC}"

echo -e "\nNOTA: Em um ambiente real, o bloqueio persiste pelo tempo configurado."
echo "Para verificar os bloqueios no Redis, execute:"
echo "docker exec -it redis redis-cli keys '*'"