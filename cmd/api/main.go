package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gorilla/mux"
	"github.com/rs/cors"
	"go.temporal.io/sdk/client"

	"dev/bravebird/browser-automation-go/pkg/api"
	"dev/bravebird/browser-automation-go/pkg/database"
	"dev/bravebird/browser-automation-go/pkg/llm"
	"dev/bravebird/browser-automation-go/pkg/semantic"
)

func main() {
	log.Println("Starting Browser Automation API Server")

	// Get configuration from environment
	port := getEnvOrDefault("PORT", "8080")
	mysqlDSN := getEnvOrDefault("MYSQL_DSN", "automator:automator@tcp(localhost:3306)/automator?parseTime=true")
	temporalHost := getEnvOrDefault("TEMPORAL_HOST", "localhost:7233")
	ollamaHost := getEnvOrDefault("OLLAMA_HOST", "http://localhost:11434")

	// Initialize database
	db, err := database.New(mysqlDSN)
	if err != nil {
		log.Printf("Warning: Failed to connect to database: %v", err)
		log.Println("Running without database persistence")
		db = nil
	}
	if db != nil {
		defer db.Close()
	}

	// Initialize Temporal client
	temporalClient, err := client.Dial(client.Options{
		HostPort: temporalHost,
	})
	if err != nil {
		log.Fatalf("Failed to create Temporal client: %v", err)
	}
	defer temporalClient.Close()

	// Initialize LLM configs
	llmConfigs := make(map[string]llm.Config)
	llmConfigs["ollama"] = llm.Config{
		Provider: "ollama",
		Model:    getEnvOrDefault("OLLAMA_MODEL", "codellama:13b"),
		BaseURL:  ollamaHost,
	}
	if apiKey := os.Getenv("OPENAI_API_KEY"); apiKey != "" {
		llmConfigs["openai"] = llm.Config{Provider: "openai", APIKey: apiKey}
	}
	if apiKey := os.Getenv("ANTHROPIC_API_KEY"); apiKey != "" {
		llmConfigs["anthropic"] = llm.Config{Provider: "anthropic", APIKey: apiKey}
	}
	if apiKey := os.Getenv("GEMINI_API_KEY"); apiKey != "" {
		llmConfigs["gemini"] = llm.Config{Provider: "gemini", APIKey: apiKey}
	}

	// Initialize embedding service
	embeddingService := semantic.NewEmbeddingService(ollamaHost, "nomic-embed-text")

	// Create API handlers
	handlers := api.NewHandlers(db, temporalClient, llmConfigs, embeddingService)

	// Setup router
	router := mux.NewRouter()

	// Health check
	router.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}).Methods("GET")

	// API routes
	apiRouter := router.PathPrefix("/api").Subrouter()

	// Workflows
	apiRouter.HandleFunc("/workflows", handlers.ListWorkflows).Methods("GET")
	apiRouter.HandleFunc("/workflows", handlers.CreateWorkflow).Methods("POST")
	apiRouter.HandleFunc("/workflows/{id}", handlers.GetWorkflow).Methods("GET")
	apiRouter.HandleFunc("/workflows/{id}", handlers.DeleteWorkflow).Methods("DELETE")
	apiRouter.HandleFunc("/workflows/{id}/generate", handlers.GenerateWorkflow).Methods("POST")
	apiRouter.HandleFunc("/workflows/{id}/actions", handlers.GetWorkflowActions).Methods("GET")

	// Runs
	apiRouter.HandleFunc("/workflows/{id}/run", handlers.ExecuteWorkflow).Methods("POST")
	apiRouter.HandleFunc("/runs", handlers.ListRuns).Methods("GET")
	apiRouter.HandleFunc("/runs/{id}", handlers.GetRun).Methods("GET")
	apiRouter.HandleFunc("/runs/{id}/cancel", handlers.CancelRun).Methods("POST")

	// WebSocket for real-time updates
	apiRouter.HandleFunc("/runs/{id}/stream", handlers.StreamRunUpdates).Methods("GET")

	// LLM providers
	apiRouter.HandleFunc("/llm/providers", handlers.ListLLMProviders).Methods("GET")

	// Screenshots
	apiRouter.HandleFunc("/screenshots/{filename}", handlers.ServeScreenshot).Methods("GET")

	// Setup CORS
	c := cors.New(cors.Options{
		AllowedOrigins:   []string{"*"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"*"},
		AllowCredentials: true,
	})

	handler := c.Handler(router)

	// Create server
	server := &http.Server{
		Addr:         ":" + port,
		Handler:      handler,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Start server in goroutine
	go func() {
		log.Printf("API server listening on port %s", port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server failed: %v", err)
		}
	}()

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down server...")

	// Graceful shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}

	log.Println("Server stopped")
}

func getEnvOrDefault(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}
