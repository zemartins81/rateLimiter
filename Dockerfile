FROM golang:1.24-alpine3.21

# Configurar o diretório de trabalho
WORKDIR /app

# Copiar os arquivos do projeto
COPY . .

# Instalar dependências e compilar o aplicativo
RUN go mod tidy
RUN go build -o main ./cmd/server

EXPOSE 8080

# Executar o aplicativo
CMD ["./main"]