package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"dev/bravebird/browser-automation-go/pkg/models"
)

// AnthropicProvider implements the Provider interface for Anthropic Claude
type AnthropicProvider struct {
	config     Config
	httpClient *http.Client
}

// NewAnthropicProvider creates a new Anthropic provider
func NewAnthropicProvider(config Config) *AnthropicProvider {
	if config.BaseURL == "" {
		config.BaseURL = "https://api.anthropic.com"
	}
	if config.Model == "" {
		config.Model = "claude-3-sonnet-20240229"
	}
	if config.Timeout == 0 {
		config.Timeout = 60
	}
	if config.Temperature == 0 {
		config.Temperature = 0.1
	}
	if config.MaxTokens == 0 {
		config.MaxTokens = 4096
	}

	return &AnthropicProvider{
		config: config,
		httpClient: &http.Client{
			Timeout: time.Duration(config.Timeout) * time.Second,
		},
	}
}

// Anthropic API types
type AnthropicRequest struct {
	Model       string             `json:"model"`
	MaxTokens   int                `json:"max_tokens"`
	System      string             `json:"system,omitempty"`
	Messages    []AnthropicMessage `json:"messages"`
	Temperature float32            `json:"temperature,omitempty"`
	Tools       []AnthropicTool    `json:"tools,omitempty"`
}

type AnthropicMessage struct {
	Role    string             `json:"role"`
	Content []AnthropicContent `json:"content"`
}

type AnthropicContent struct {
	Type  string          `json:"type"`
	Text  string          `json:"text,omitempty"`
	ID    string          `json:"id,omitempty"`
	Name  string          `json:"name,omitempty"`
	Input json.RawMessage `json:"input,omitempty"`
}

type AnthropicTool struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	InputSchema interface{} `json:"input_schema"`
}

type AnthropicResponse struct {
	ID         string             `json:"id"`
	Type       string             `json:"type"`
	Role       string             `json:"role"`
	Content    []AnthropicContent `json:"content"`
	Model      string             `json:"model"`
	StopReason string             `json:"stop_reason"`
	Error      *AnthropicError    `json:"error,omitempty"`
}

type AnthropicError struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

// Name returns the provider name
func (p *AnthropicProvider) Name() string {
	return string(ProviderAnthropic)
}

// IsAvailable checks if the provider is configured
func (p *AnthropicProvider) IsAvailable(ctx context.Context) bool {
	return p.config.APIKey != ""
}

// GenerateBrowserCode generates Go Rod code for a semantic action
func (p *AnthropicProvider) GenerateBrowserCode(ctx context.Context, action models.SemanticAction, pageCtx PageContext) (string, error) {
	if p.config.APIKey == "" {
		return "", fmt.Errorf("Anthropic API key not configured")
	}

	prompt := BuildActionPrompt(action, pageCtx, 0, "")

	response, err := p.createMessage(ctx, SystemPromptTemplate, prompt)
	if err != nil {
		return "", fmt.Errorf("anthropic generation failed: %w", err)
	}

	return extractCode(response), nil
}

// IdentifyVariableTokens uses Claude to identify variable tokens
func (p *AnthropicProvider) IdentifyVariableTokens(ctx context.Context, actions []models.SemanticAction) ([]models.WorkflowParameter, error) {
	if p.config.APIKey == "" {
		return nil, fmt.Errorf("Anthropic API key not configured")
	}

	prompt := BuildVariableTokenPrompt(actions)

	response, err := p.createMessage(ctx, "You are a JSON generator. Output ONLY valid JSON.", prompt)
	if err != nil {
		return nil, fmt.Errorf("anthropic generation failed: %w", err)
	}

	var result struct {
		Parameters []models.WorkflowParameter `json:"parameters"`
	}

	jsonStr := extractJSON(response)
	if err := json.Unmarshal([]byte(jsonStr), &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return result.Parameters, nil
}

// ClassifyValue classifies a value into a semantic category
func (p *AnthropicProvider) ClassifyValue(ctx context.Context, value string) (string, error) {
	// For now, return "input" or use heuristic to avoid API costs
	return "input", nil
}

// GenerateCompleteWorkflow generates the complete workflow code
func (p *AnthropicProvider) GenerateCompleteWorkflow(ctx context.Context, actions []models.SemanticAction, params []models.WorkflowParameter) (string, error) {
	if p.config.APIKey == "" {
		return "", fmt.Errorf("Anthropic API key not configured")
	}

	prompt := BuildWorkflowPrompt(actions, params)

	response, err := p.createMessage(ctx, SystemPromptTemplate, prompt)
	if err != nil {
		return "", fmt.Errorf("anthropic generation failed: %w", err)
	}

	return extractCode(response), nil
}

// createMessage makes a request to the Anthropic messages API
func (p *AnthropicProvider) createMessage(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	reqBody := AnthropicRequest{
		Model:       p.config.Model,
		MaxTokens:   p.config.MaxTokens,
		System:      systemPrompt,
		Temperature: p.config.Temperature,
		Messages: []AnthropicMessage{
			{
				Role: "user",
				Content: []AnthropicContent{
					{Type: "text", Text: userPrompt},
				},
			},
		},
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/v1/messages", p.config.BaseURL)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonBody))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", p.config.APIKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	var anthropicResp AnthropicResponse
	if err := json.Unmarshal(body, &anthropicResp); err != nil {
		return "", fmt.Errorf("failed to decode response: %w", err)
	}

	if anthropicResp.Error != nil {
		return "", fmt.Errorf("anthropic error: %s", anthropicResp.Error.Message)
	}

	// Extract text from content blocks
	var textContent string
	for _, content := range anthropicResp.Content {
		if content.Type == "text" {
			textContent += content.Text
		}
	}

	if textContent == "" {
		return "", fmt.Errorf("no text content in response")
	}

	return textContent, nil
}
