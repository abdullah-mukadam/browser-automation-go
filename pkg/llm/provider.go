package llm

import (
	"context"

	"dev/bravebird/browser-automation-go/pkg/models"
)

// Provider is the interface for LLM providers
type Provider interface {
	// GenerateBrowserCode generates Go Rod code for a semantic action
	GenerateBrowserCode(ctx context.Context, action models.SemanticAction, pageContext PageContext) (string, error)

	// IdentifyVariableTokens analyzes actions to identify variable vs fixed tokens
	IdentifyVariableTokens(ctx context.Context, actions []models.SemanticAction) ([]models.WorkflowParameter, error)

	// GenerateCompleteWorkflow generates the complete workflow code
	GenerateCompleteWorkflow(ctx context.Context, actions []models.SemanticAction, params []models.WorkflowParameter) (string, error)

	// ClassifyValue classifies a value into a semantic category
	ClassifyValue(ctx context.Context, value string) (string, error)

	// Name returns the provider name
	Name() string

	// IsAvailable checks if the provider is configured and available
	IsAvailable(ctx context.Context) bool
}

// PageContext provides context about the current page state
type PageContext struct {
	URL         string            `json:"url"`
	Title       string            `json:"title"`
	VisibleText string            `json:"visible_text,omitempty"`
	Screenshot  []byte            `json:"screenshot,omitempty"`
	Variables   map[string]string `json:"variables,omitempty"`
}

// GenerateRequest represents a request to generate browser code
type GenerateRequest struct {
	Action      models.SemanticAction `json:"action"`
	PageContext PageContext           `json:"page_context"`
	RetryCount  int                   `json:"retry_count"`
	LastError   string                `json:"last_error,omitempty"`
}

// GenerateResponse represents the response from code generation
type GenerateResponse struct {
	Code        string `json:"code"`
	Selector    string `json:"selector,omitempty"`
	Explanation string `json:"explanation,omitempty"`
}

// Config holds LLM provider configuration
type Config struct {
	Provider    string  `json:"provider"`
	Model       string  `json:"model,omitempty"`
	APIKey      string  `json:"-"` // Don't serialize
	BaseURL     string  `json:"base_url,omitempty"`
	Temperature float32 `json:"temperature,omitempty"`
	MaxTokens   int     `json:"max_tokens,omitempty"`
	Timeout     int     `json:"timeout_seconds,omitempty"`
}

// ProviderName represents supported LLM providers
type ProviderName string

const (
	ProviderOllama    ProviderName = "ollama"
	ProviderOpenAI    ProviderName = "openai"
	ProviderAnthropic ProviderName = "anthropic"
	ProviderGemini    ProviderName = "gemini"
)

// DefaultConfigs returns default configurations for each provider
func DefaultConfigs() map[ProviderName]Config {
	return map[ProviderName]Config{
		ProviderOllama: {
			Provider:    string(ProviderOllama),
			Model:       "codellama:13b",
			BaseURL:     "http://localhost:11434",
			Temperature: 0.1,
			MaxTokens:   4096,
			Timeout:     120,
		},
		ProviderOpenAI: {
			Provider:    string(ProviderOpenAI),
			Model:       "gpt-4-turbo-preview",
			BaseURL:     "https://api.openai.com/v1",
			Temperature: 0.1,
			MaxTokens:   4096,
			Timeout:     60,
		},
		ProviderAnthropic: {
			Provider:    string(ProviderAnthropic),
			Model:       "claude-3-sonnet-20240229",
			BaseURL:     "https://api.anthropic.com",
			Temperature: 0.1,
			MaxTokens:   4096,
			Timeout:     60,
		},
		ProviderGemini: {
			Provider:    string(ProviderGemini),
			Model:       "gemini-1.5-pro",
			BaseURL:     "https://generativelanguage.googleapis.com",
			Temperature: 0.1,
			MaxTokens:   4096,
			Timeout:     60,
		},
	}
}

// NewProvider creates a new LLM provider based on configuration
func NewProvider(config Config) (Provider, error) {
	switch ProviderName(config.Provider) {
	case ProviderOllama:
		return NewOllamaProvider(config), nil
	case ProviderOpenAI:
		return NewOpenAIProvider(config), nil
	case ProviderAnthropic:
		return NewAnthropicProvider(config), nil
	case ProviderGemini:
		return NewGeminiProvider(config), nil
	default:
		// Default to Ollama for local development
		return NewOllamaProvider(config), nil
	}
}

// ToolCall represents a function/tool call from the LLM
type ToolCall struct {
	Name      string                 `json:"name"`
	Arguments map[string]interface{} `json:"arguments"`
}

// BrowserTool defines the tool schema for browser automation
type BrowserTool struct {
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Parameters  map[string]Schema `json:"parameters"`
}

// Schema represents a JSON schema for tool parameters
type Schema struct {
	Type        string            `json:"type"`
	Description string            `json:"description,omitempty"`
	Required    bool              `json:"required,omitempty"`
	Properties  map[string]Schema `json:"properties,omitempty"`
	Items       *Schema           `json:"items,omitempty"`
	Enum        []string          `json:"enum,omitempty"`
}

// BrowserTools returns the tool definitions for browser automation
func BrowserTools() []BrowserTool {
	return []BrowserTool{
		{
			Name:        "navigate",
			Description: "Navigate to a URL",
			Parameters: map[string]Schema{
				"url": {Type: "string", Description: "The URL to navigate to", Required: true},
			},
		},
		{
			Name:        "click",
			Description: "Click on an element",
			Parameters: map[string]Schema{
				"selector": {Type: "string", Description: "CSS selector for the element", Required: true},
			},
		},
		{
			Name:        "type_text",
			Description: "Type text into an input field",
			Parameters: map[string]Schema{
				"selector": {Type: "string", Description: "CSS selector for the input", Required: true},
				"text":     {Type: "string", Description: "Text to type", Required: true},
			},
		},
		{
			Name:        "press_key",
			Description: "Press a keyboard key",
			Parameters: map[string]Schema{
				"key": {Type: "string", Description: "Key to press (e.g., Enter, Tab)", Required: true},
			},
		},
		{
			Name:        "wait_for_element",
			Description: "Wait for an element to be visible",
			Parameters: map[string]Schema{
				"selector": {Type: "string", Description: "CSS selector for the element", Required: true},
			},
		},
		{
			Name:        "screenshot",
			Description: "Take a screenshot",
			Parameters: map[string]Schema{
				"filename": {Type: "string", Description: "Filename for the screenshot", Required: true},
			},
		},
	}
}
