FROM golang:1.24-alpine3.21

# Configurar o diretório de trabalho
WORKDIR /app

# Copiar os arquivos do projeto
COPY . .

EXPOSE 8080

CMD []