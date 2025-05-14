# Rate Limiter para APIs em Go

Este é um sistema de limitação de taxa (rate limiter) para APIs web, implementado em Go. O sistema limita o número de requisições por IP e/ou token de API em um determinado período de tempo.

## Características

- Limitação por endereço IP
- Limitação por token de API
- Configuração flexível de limites e períodos de bloqueio
- Armazenamento de contadores em Redis
- Suporte para bloqueio temporário após exceder o limite

## Configuração

As configurações do rate limiter podem ser definidas através de variáveis de ambiente ou de um arquivo `.env`:
