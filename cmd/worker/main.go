package main

import (
	"log"
	"os"

	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/worker"

	"dev/bravebird/browser-automation-go/pkg/llm"
	"dev/bravebird/browser-automation-go/pkg/temporal/activities"
	"dev/bravebird/browser-automation-go/pkg/temporal/workflows"
)

const TaskQueue = "browser-automation"

func main() {
	// Get Temporal host from environment
	temporalHost := os.Getenv("TEMPORAL_HOST")
	if temporalHost == "" {
		temporalHost = "localhost:7233"
	}

	// Create Temporal client
	c, err := client.Dial(client.Options{
		HostPort: temporalHost,
	})
	if err != nil {
		log.Fatalf("Failed to create Temporal client: %v", err)
	}
	defer c.Close()

	// Create LLM configurations
	llmConfigs := make(map[string]llm.Config)

	// Ollama config (default)
	ollamaHost := os.Getenv("OLLAMA_HOST")
	if ollamaHost == "" {
		ollamaHost = "http://localhost:11434"
	}
	llmConfigs["ollama"] = llm.Config{
		Provider:    "ollama",
		Model:       getEnvOrDefault("OLLAMA_MODEL", "codellama:13b"),
		BaseURL:     ollamaHost,
		Temperature: 0.1,
		MaxTokens:   4096,
		Timeout:     120,
	}

	// OpenAI config
	if apiKey := os.Getenv("OPENAI_API_KEY"); apiKey != "" {
		llmConfigs["openai"] = llm.Config{
			Provider:    "openai",
			Model:       getEnvOrDefault("OPENAI_MODEL", "gpt-4-turbo-preview"),
			APIKey:      apiKey,
			Temperature: 0.1,
			MaxTokens:   4096,
			Timeout:     60,
		}
	}

	// Anthropic config
	if apiKey := os.Getenv("ANTHROPIC_API_KEY"); apiKey != "" {
		llmConfigs["anthropic"] = llm.Config{
			Provider:    "anthropic",
			Model:       getEnvOrDefault("ANTHROPIC_MODEL", "claude-3-sonnet-20240229"),
			APIKey:      apiKey,
			Temperature: 0.1,
			MaxTokens:   4096,
			Timeout:     60,
		}
	}

	// Gemini config
	if apiKey := os.Getenv("GEMINI_API_KEY"); apiKey != "" {
		llmConfigs["gemini"] = llm.Config{
			Provider:    "gemini",
			Model:       getEnvOrDefault("GEMINI_MODEL", "gemini-2.0-flash"),
			APIKey:      apiKey,
			Temperature: 0.1,
			MaxTokens:   4096,
			Timeout:     60,
		}
	}

	// Screenshot directory
	screenshotDir := getEnvOrDefault("SCREENSHOT_DIR", "/tmp/screenshots")

	// Create activities
	acts := activities.NewActivities(llmConfigs, screenshotDir)

	// Create worker
	w := worker.New(c, TaskQueue, worker.Options{
		MaxConcurrentActivityExecutionSize:     5,
		MaxConcurrentWorkflowTaskExecutionSize: 10,
	})

	// Register workflows
	w.RegisterWorkflow(workflows.BrowserAutomationWorkflow)
	w.RegisterWorkflow(workflows.ParallelBrowserAutomationWorkflow)

	// Register activities
	w.RegisterActivity(acts.InitializeBrowserActivity)
	w.RegisterActivity(acts.CloseBrowserActivity)
	w.RegisterActivity(acts.PreGenerateCodeActivity)
	w.RegisterActivity(acts.ExecuteBrowserActionActivity)
	w.RegisterActivity(acts.TakeScreenshotActivity)

	log.Printf("Starting Temporal worker on task queue: %s", TaskQueue)
	log.Printf("Temporal host: %s", temporalHost)
	log.Printf("Available LLM providers: %v", getProviderNames(llmConfigs))

	// Start worker
	err = w.Run(worker.InterruptCh())
	if err != nil {
		log.Fatalf("Worker failed: %v", err)
	}
}

func getEnvOrDefault(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}

func getProviderNames(configs map[string]llm.Config) []string {
	names := make([]string, 0, len(configs))
	for name := range configs {
		names = append(names, name)
	}
	return names
}
