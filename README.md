# Rate Limiter para APIs em Go

Este é um sistema de limitação de taxa (rate limiter) para APIs web, implementado em Go. O sistema limita o número de requisições por IP e/ou token de API em um determinado período de tempo.

## Características

- Limitação por endereço IP
- Limitação por token de API
- Configuração flexível de limites e períodos de bloqueio
- Armazenamento de contadores em Redis
- Suporte para bloqueio temporário após exceder o limite

## Como baixar o repositório

Para obter uma cópia local do projeto, clone o repositório usando o seguinte comando:

```bash
git clone https://github.com/zemartins81/rateLimiter.git
cd rateLimiter
```

Copie o arquivo .env.example e renomeie como .env

```bash
cp .env.example .env
```

Ajuste as variáveis de ambiente conforme desejar.

Para executar o projeto:

```bash
docker-compose up -d
```

Para executar os testes, primeiramente precisamos tornar o arquivo test_ratelimiter.sh executável:
```bash
sudo chmod +x ./tests/functional/test_ratelimiter.sh
```

Em seguida, executar os testes:
```bash
./tests/functional/test_ratelimiter.sh
```

Interromper a execução dos containers:
```bash
docker-compose down
```



