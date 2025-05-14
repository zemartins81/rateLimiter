package main
package main

import (
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
	
	"github.com/joho/godotenv"
)

// Opções de configuração da ferramenta de benchmark
type benchmarkOptions struct {
	url           string
	numRequests   int
	concurrency   int
	useToken      bool
	tokenValue    string
	tokenHeader   string
	printProgress bool
}

// Resultados do benchmark
type benchmarkResults struct {
	totalRequests      int
	successRequests    int32
	ratelimitedReqs    int32
	otherErrors        int32
	totalDuration      time.Duration
	minResponseTime    time.Duration
	maxResponseTime    time.Duration
	avgResponseTime    time.Duration
	requestsPerSecond  float64
}

func init() {
	// Carregar variáveis de ambiente do arquivo .env, ignorando erros
	_ = godotenv.Load()
}

// getEnvString obtém um valor de string de uma variável de ambiente ou retorna um valor padrão
func getEnvString(key, defaultValue string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return defaultValue
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

func main() {
	// Analisar as opções da linha de comando
	opts := parseOptions()
	fmt.Printf("Iniciando benchmark para %s\n", opts.url)
	fmt.Printf("Enviando %d requisições com %d conexões concorrentes\n", opts.numRequests, opts.concurrency)

	if opts.useToken {
		fmt.Printf("Usando token '%s' no header '%s'\n", opts.tokenValue, opts.tokenHeader)
	} else {
		fmt.Println("Testando sem token (limitação por IP)")
	}

	// Executar o benchmark
	results := runBenchmark(opts)

	// Imprimir resultados
	printResults(results)
}

func parseOptions() *benchmarkOptions {
	// Valores padrão do ambiente ou hardcoded
	defaultURL := getEnvString("API_URL", "http://localhost:8080/")
	defaultTokenHeader := getEnvString("TOKEN_HEADER_NAME", "API_KEY")
	defaultTokenValue := getEnvString("TEST_TOKEN", "meu-token-123")
	defaultNumRequests := getEnvInt("BENCHMARK_NUM_REQUESTS", 100)
	defaultConcurrency := getEnvInt("BENCHMARK_CONCURRENCY", 10)
	
	// Configuração via linha de comando (sobrescreve variáveis de ambiente)
	url := flag.String("url", defaultURL, "URL para testar")
	numRequests := flag.Int("n", defaultNumRequests, "Número total de requisições")
	concurrency := flag.Int("c", defaultConcurrency, "Número de requisições concorrentes")
	useToken := flag.Bool("token", false, "Usar token para autenticação")
	tokenValue := flag.String("token-value", defaultTokenValue, "Valor do token a ser usado")
	tokenHeader := flag.String("token-header", defaultTokenHeader, "Nome do header de token")
	printProgress := flag.Bool("progress", true, "Mostrar progresso durante o teste")

	flag.Parse()

	// Exibir informações sobre configuração do ambiente
	if _, exists := os.LookupEnv("MAX_REQUESTS_PER_IP"); exists {
		fmt.Printf("Limite por IP configurado no ambiente: %d\n", 
			getEnvInt("MAX_REQUESTS_PER_IP", 5))
	}
	
	if _, exists := os.LookupEnv("MAX_REQUESTS_PER_TOKEN"); exists {
		fmt.Printf("Limite por token configurado no ambiente: %d\n", 
			getEnvInt("MAX_REQUESTS_PER_TOKEN", 10))
	}

	return &benchmarkOptions{
		url:           *url,
		numRequests:   *numRequests,
		concurrency:   *concurrency,
		useToken:      *useToken,
		tokenValue:    *tokenValue,
		tokenHeader:   *tokenHeader,
		printProgress: *printProgress,
	}
}

func runBenchmark(opts *benchmarkOptions) *benchmarkResults {
	results := &benchmarkResults{
		totalRequests:   opts.numRequests,
		minResponseTime: time.Hour, // Valor inicial alto para ser substituído
	}

	// Criar um WaitGroup para controlar as goroutines
	var wg sync.WaitGroup
	
	// Canal para limitar a concorrência
	semaphore := make(chan struct{}, opts.concurrency)
	
	// Canal para registrar tempos de resposta
	responseTimes := make(chan time.Duration, opts.numRequests)
	
	startTime := time.Now()
	
	// Iniciar as requisições
	for i := 0; i < opts.numRequests; i++ {
		wg.Add(1)
		semaphore <- struct{}{} // Adquirir um slot do semáforo
		
		go func(reqNum int) {
			defer wg.Done()
			defer func() { <-semaphore }() // Liberar o slot do semáforo
			
			// Fazer a requisição HTTP
			reqStart := time.Now()
			statusCode, err := makeRequest(opts)
			reqDuration := time.Since(reqStart)
			
			// Enviar o tempo de resposta para o canal
			responseTimes <- reqDuration
			
			// Contabilizar resultado
			if err != nil {
				atomic.AddInt32(&results.otherErrors, 1)
				if opts.printProgress {
					fmt.Printf("Erro na requisição %d: %v\n", reqNum, err)
				}
			} else if statusCode == http.StatusTooManyRequests {
				atomic.AddInt32(&results.ratelimitedReqs, 1)
				if opts.printProgress {
					fmt.Printf("Requisição %d: bloqueada pelo rate limiter (429)\n", reqNum)
				}
			} else if statusCode == http.StatusOK {
				atomic.AddInt32(&results.successRequests, 1)
				if opts.printProgress && reqNum%10 == 0 {
					fmt.Printf("Requisição %d: sucesso (%d ms)\n", reqNum, reqDuration.Milliseconds())
				}
			} else {
				atomic.AddInt32(&results.otherErrors, 1)
				if opts.printProgress {
					fmt.Printf("Requisição %d: código inesperado %d\n", reqNum, statusCode)
				}
			}
		}(i + 1)
	}
	
	// Esperar todas as goroutines terminarem
	wg.Wait()
	close(responseTimes)
	
	// Calcular a duração total
	results.totalDuration = time.Since(startTime)
	
	// Calcular estatísticas de tempo de resposta
	var totalResponseTime time.Duration
	count := 0
	
	for rt := range responseTimes {
		if rt < results.minResponseTime {
			results.minResponseTime = rt
		}
		if rt > results.maxResponseTime {
			results.maxResponseTime = rt
		}
		totalResponseTime += rt
		count++
	}
	
	if count > 0 {
		results.avgResponseTime = totalResponseTime / time.Duration(count)
		results.requestsPerSecond = float64(opts.numRequests) / results.totalDuration.Seconds()
	}
	
	return results
}

func makeRequest(opts *benchmarkOptions) (int, error) {
	client := &http.Client{
		Timeout: 10 * time.Second,
	}
	
	req, err := http.NewRequest("GET", opts.url, nil)
	if err != nil {
		return 0, err
	}
	
	// Adicionar token, se necessário
	if opts.useToken {
		req.Header.Set(opts.tokenHeader, opts.tokenValue)
	}
	
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	
	// Ler e descartar o corpo da resposta para liberar conexões
	_, err = io.Copy(io.Discard, resp.Body)
	if err != nil {
		return 0, err
	}
	
	return resp.StatusCode, nil
}

func printResults(results *benchmarkResults) {
	fmt.Println("\n=== RESULTADOS DO BENCHMARK ===")
	fmt.Printf("Tempo total: %.2f segundos\n", results.totalDuration.Seconds())
	fmt.Printf("Requisições totais: %d\n", results.totalRequests)
	fmt.Printf("Requisições com sucesso: %d (%.1f%%)\n", 
		results.successRequests, float64(results.successRequests)*100/float64(results.totalRequests))
	fmt.Printf("Requisições bloqueadas: %d (%.1f%%)\n", 
		results.ratelimitedReqs, float64(results.ratelimitedReqs)*100/float64(results.totalRequests))
	fmt.Printf("Erros: %d (%.1f%%)\n", 
		results.otherErrors, float64(results.otherErrors)*100/float64(results.totalRequests))
	fmt.Printf("Requisições por segundo: %.2f\n", results.requestsPerSecond)
	fmt.Printf("Tempo mínimo de resposta: %s\n", results.minResponseTime)
	fmt.Printf("Tempo máximo de resposta: %s\n", results.maxResponseTime)
	fmt.Printf("Tempo médio de resposta: %s\n", results.avgResponseTime)
	
	// Análise dos resultados do rate limiter
	if results.ratelimitedReqs > 0 {
		fmt.Println("\nANÁLISE DO RATE LIMITER:")
		fmt.Println("O rate limiter está funcionando corretamente e bloqueou algumas requisições.")
		
		// Comparar os resultados com os limites configurados
		maxRequestsPerIP := getEnvInt("MAX_REQUESTS_PER_IP", 5)
		maxRequestsPerToken := getEnvInt("MAX_REQUESTS_PER_TOKEN", 10)
		
		if results.successRequests > 0 {
			limite := maxRequestsPerIP
			if maxRequestsPerToken > 0 {
				limite = maxRequestsPerToken
			}
			
			fmt.Printf("Expectativa: No máximo %d requisições deveriam ser permitidas antes do bloqueio.\n", limite)
		}
		
		if float64(results.ratelimitedReqs)/float64(results.totalRequests) > 0.5 {
			fmt.Println("AVISO: Mais de 50% das requisições foram bloqueadas!")
			fmt.Println("Isso pode indicar que o limite configurado é muito baixo para a carga de teste.")
		}
	} else {
		fmt.Println("\nAVISO: Nenhuma requisição foi bloqueada pelo rate limiter!")
		fmt.Println("Isso pode indicar um problema na configuração ou o limite é muito alto.")
		fmt.Println("Tente aumentar o número de requisições ou diminuir o limite configurado.")
	}
}
import (
	"flag"
	"fmt"
	"io"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

// Opções de configuração da ferramenta de benchmark
type benchmarkOptions struct {
	url           string
	numRequests   int
	concurrency   int
	useToken      bool
	tokenValue    string
	tokenHeader   string
	printProgress bool
}

// Resultados do benchmark
type benchmarkResults struct {
	totalRequests      int
	successRequests    int32
	ratelimitedReqs    int32
	otherErrors        int32
	totalDuration      time.Duration
	minResponseTime    time.Duration
	maxResponseTime    time.Duration
	avgResponseTime    time.Duration
	requestsPerSecond  float64
}

func main() {
	// Analisar as opções da linha de comando
	opts := parseOptions()
	fmt.Printf("Iniciando benchmark para %s\n", opts.url)
	fmt.Printf("Enviando %d requisições com %d conexões concorrentes\n", opts.numRequests, opts.concurrency)

	if opts.useToken {
		fmt.Printf("Usando token '%s' no header '%s'\n", opts.tokenValue, opts.tokenHeader)
	} else {
		fmt.Println("Testando sem token (limitação por IP)")
	}

	// Executar o benchmark
	results := runBenchmark(opts)

	// Imprimir resultados
	printResults(results)
}

func parseOptions() *benchmarkOptions {
	url := flag.String("url", "http://localhost:8080/", "URL para testar")
	numRequests := flag.Int("n", 100, "Número total de requisições")
	concurrency := flag.Int("c", 10, "Número de requisições concorrentes")
	useToken := flag.Bool("token", false, "Usar token para autenticação")
	tokenValue := flag.String("token-value", "meu-token-123", "Valor do token a ser usado")
	tokenHeader := flag.String("token-header", "API_KEY", "Nome do header de token")
	printProgress := flag.Bool("progress", true, "Mostrar progresso durante o teste")

	flag.Parse()

	return &benchmarkOptions{
		url:           *url,
		numRequests:   *numRequests,
		concurrency:   *concurrency,
		useToken:      *useToken,
		tokenValue:    *tokenValue,
		tokenHeader:   *tokenHeader,
		printProgress: *printProgress,
	}
}

func runBenchmark(opts *benchmarkOptions) *benchmarkResults {
	results := &benchmarkResults{
		totalRequests:   opts.numRequests,
		minResponseTime: time.Hour, // Valor inicial alto para ser substituído
	}

	// Criar um WaitGroup para controlar as goroutines
	var wg sync.WaitGroup
	
	// Canal para limitar a concorrência
	semaphore := make(chan struct{}, opts.concurrency)
	
	// Canal para registrar tempos de resposta
	responseTimes := make(chan time.Duration, opts.numRequests)
	
	startTime := time.Now()
	
	// Iniciar as requisições
	for i := 0; i < opts.numRequests; i++ {
		wg.Add(1)
		semaphore <- struct{}{} // Adquirir um slot do semáforo
		
		go func(reqNum int) {
			defer wg.Done()
			defer func() { <-semaphore }() // Liberar o slot do semáforo
			
			// Fazer a requisição HTTP
			reqStart := time.Now()
			statusCode, err := makeRequest(opts)
			reqDuration := time.Since(reqStart)
			
			// Enviar o tempo de resposta para o canal
			responseTimes <- reqDuration
			
			// Contabilizar resultado
			if err != nil {
				atomic.AddInt32(&results.otherErrors, 1)
				if opts.printProgress {
					fmt.Printf("Erro na requisição %d: %v\n", reqNum, err)
				}
			} else if statusCode == http.StatusTooManyRequests {
				atomic.AddInt32(&results.ratelimitedReqs, 1)
				if opts.printProgress {
					fmt.Printf("Requisição %d: bloqueada pelo rate limiter (429)\n", reqNum)
				}
			} else if statusCode == http.StatusOK {
				atomic.AddInt32(&results.successRequests, 1)
				if opts.printProgress && reqNum%10 == 0 {
					fmt.Printf("Requisição %d: sucesso (%d ms)\n", reqNum, reqDuration.Milliseconds())
				}
			} else {
				atomic.AddInt32(&results.otherErrors, 1)
				if opts.printProgress {
					fmt.Printf("Requisição %d: código inesperado %d\n", reqNum, statusCode)
				}
			}
		}(i + 1)
	}
	
	// Esperar todas as goroutines terminarem
	wg.Wait()
	close(responseTimes)
	
	// Calcular a duração total
	results.totalDuration = time.Since(startTime)
	
	// Calcular estatísticas de tempo de resposta
	var totalResponseTime time.Duration
	count := 0
	
	for rt := range responseTimes {
		if rt < results.minResponseTime {
			results.minResponseTime = rt
		}
		if rt > results.maxResponseTime {
			results.maxResponseTime = rt
		}
		totalResponseTime += rt
		count++
	}
	
	if count > 0 {
		results.avgResponseTime = totalResponseTime / time.Duration(count)
		results.requestsPerSecond = float64(opts.numRequests) / results.totalDuration.Seconds()
	}
	
	return results
}

func makeRequest(opts *benchmarkOptions) (int, error) {
	client := &http.Client{
		Timeout: 10 * time.Second,
	}
	
	req, err := http.NewRequest("GET", opts.url, nil)
	if err != nil {
		return 0, err
	}
	
	// Adicionar token, se necessário
	if opts.useToken {
		req.Header.Set(opts.tokenHeader, opts.tokenValue)
	}
	
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	
	// Ler e descartar o corpo da resposta para liberar conexões
	_, err = io.Copy(io.Discard, resp.Body)
	if err != nil {
		return 0, err
	}
	
	return resp.StatusCode, nil
}

func printResults(results *benchmarkResults) {
	fmt.Println("\n=== RESULTADOS DO BENCHMARK ===")
	fmt.Printf("Tempo total: %.2f segundos\n", results.totalDuration.Seconds())
	fmt.Printf("Requisições totais: %d\n", results.totalRequests)
	fmt.Printf("Requisições com sucesso: %d (%.1f%%)\n", 
		results.successRequests, float64(results.successRequests)*100/float64(results.totalRequests))
	fmt.Printf("Requisições bloqueadas: %d (%.1f%%)\n", 
		results.ratelimitedReqs, float64(results.ratelimitedReqs)*100/float64(results.totalRequests))
	fmt.Printf("Erros: %d (%.1f%%)\n", 
		results.otherErrors, float64(results.otherErrors)*100/float64(results.totalRequests))
	fmt.Printf("Requisições por segundo: %.2f\n", results.requestsPerSecond)
	fmt.Printf("Tempo mínimo de resposta: %s\n", results.minResponseTime)
	fmt.Printf("Tempo máximo de resposta: %s\n", results.maxResponseTime)
	fmt.Printf("Tempo médio de resposta: %s\n", results.avgResponseTime)
	
	// Análise dos resultados do rate limiter
	if results.ratelimitedReqs > 0 {
		fmt.Println("\nANÁLISE DO RATE LIMITER:")
		fmt.Println("O rate limiter está funcionando corretamente e bloqueou algumas requisições.")
		
		if float64(results.ratelimitedReqs)/float64(results.totalRequests) > 0.5 {
			fmt.Println("AVISO: Mais de 50% das requisições foram bloqueadas!")
			fmt.Println("Isso pode indicar que o limite configurado é muito baixo para a carga de teste.")
		}
	} else {
		fmt.Println("\nAVISO: Nenhuma requisição foi bloqueada pelo rate limiter!")
		fmt.Println("Isso pode indicar um problema na configuração ou o limite é muito alto.")
		fmt.Println("Tente aumentar o número de requisições ou diminuir o limite configurado.")
	}
}
