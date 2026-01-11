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

// OpenAIProvider implements the Provider interface for OpenAI
type OpenAIProvider struct {
	config     Config
	httpClient *http.Client
}

// NewOpenAIProvider creates a new OpenAI provider
func NewOpenAIProvider(config Config) *OpenAIProvider {
	if config.BaseURL == "" {
		config.BaseURL = "https://api.openai.com/v1"
	}
	if config.Model == "" {
		config.Model = "gpt-4-turbo-preview"
	}
	if config.Timeout == 0 {
		config.Timeout = 60
	}
	if config.Temperature == 0 {
		config.Temperature = 0.1
	}

	return &OpenAIProvider{
		config: config,
		httpClient: &http.Client{
			Timeout: time.Duration(config.Timeout) * time.Second,
		},
	}
}

// OpenAI API types
type OpenAIRequest struct {
	Model       string          `json:"model"`
	Messages    []OpenAIMessage `json:"messages"`
	Temperature float32         `json:"temperature,omitempty"`
	MaxTokens   int             `json:"max_tokens,omitempty"`
	Tools       []OpenAITool    `json:"tools,omitempty"`
}

type OpenAIMessage struct {
	Role       string           `json:"role"`
	Content    string           `json:"content,omitempty"`
	ToolCalls  []OpenAIToolCall `json:"tool_calls,omitempty"`
	ToolCallID string           `json:"tool_call_id,omitempty"`
}

type OpenAITool struct {
	Type     string            `json:"type"`
	Function OpenAIFunctionDef `json:"function"`
}

type OpenAIFunctionDef struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	Parameters  interface{} `json:"parameters"`
}

type OpenAIToolCall struct {
	ID       string             `json:"id"`
	Type     string             `json:"type"`
	Function OpenAIFunctionCall `json:"function"`
}

type OpenAIFunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type OpenAIResponse struct {
	ID      string         `json:"id"`
	Object  string         `json:"object"`
	Choices []OpenAIChoice `json:"choices"`
	Error   *OpenAIError   `json:"error,omitempty"`
}

type OpenAIChoice struct {
	Index        int           `json:"index"`
	Message      OpenAIMessage `json:"message"`
	FinishReason string        `json:"finish_reason"`
}

type OpenAIError struct {
	Message string `json:"message"`
	Type    string `json:"type"`
	Code    string `json:"code"`
}

// Name returns the provider name
func (p *OpenAIProvider) Name() string {
	return string(ProviderOpenAI)
}

// IsAvailable checks if the provider is configured
func (p *OpenAIProvider) IsAvailable(ctx context.Context) bool {
	return p.config.APIKey != ""
}

// GenerateBrowserCode generates Go Rod code for a semantic action
func (p *OpenAIProvider) GenerateBrowserCode(ctx context.Context, action models.SemanticAction, pageCtx PageContext) (string, error) {
	if p.config.APIKey == "" {
		return "", fmt.Errorf("OpenAI API key not configured")
	}

	prompt := BuildActionPrompt(action, pageCtx, 0, "")

	response, err := p.chatCompletion(ctx, []OpenAIMessage{
		{Role: "system", Content: SystemPromptTemplate},
		{Role: "user", Content: prompt},
	})
	if err != nil {
		return "", fmt.Errorf("openai generation failed: %w", err)
	}

	return extractCode(response), nil
}

// IdentifyVariableTokens uses OpenAI to identify variable tokens
func (p *OpenAIProvider) IdentifyVariableTokens(ctx context.Context, actions []models.SemanticAction) ([]models.WorkflowParameter, error) {
	if p.config.APIKey == "" {
		return nil, fmt.Errorf("OpenAI API key not configured")
	}

	prompt := BuildVariableTokenPrompt(actions)

	response, err := p.chatCompletion(ctx, []OpenAIMessage{
		{Role: "system", Content: "You are a JSON generator. Output ONLY valid JSON."},
		{Role: "user", Content: prompt},
	})
	if err != nil {
		return nil, fmt.Errorf("openai generation failed: %w", err)
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

// GenerateCompleteWorkflow generates the complete workflow code
func (p *OpenAIProvider) GenerateCompleteWorkflow(ctx context.Context, actions []models.SemanticAction, params []models.WorkflowParameter) (string, error) {
	if p.config.APIKey == "" {
		return "", fmt.Errorf("OpenAI API key not configured")
	}

	prompt := BuildWorkflowPrompt(actions, params)

	response, err := p.chatCompletion(ctx, []OpenAIMessage{
		{Role: "system", Content: SystemPromptTemplate},
		{Role: "user", Content: prompt},
	})
	if err != nil {
		return "", fmt.Errorf("openai generation failed: %w", err)
	}

	return extractCode(response), nil
}

// ClassifyValue classifies a value into a semantic category
func (p *OpenAIProvider) ClassifyValue(ctx context.Context, value string) (string, error) {
	// For now, return "input" or use heuristic to avoid API costs
	return "input", nil
}

// chatCompletion makes a request to the OpenAI chat API
func (p *OpenAIProvider) chatCompletion(ctx context.Context, messages []OpenAIMessage) (string, error) {
	reqBody := OpenAIRequest{
		Model:       p.config.Model,
		Messages:    messages,
		Temperature: p.config.Temperature,
		MaxTokens:   p.config.MaxTokens,
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/chat/completions", p.config.BaseURL)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonBody))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", p.config.APIKey))

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	var openaiResp OpenAIResponse
	if err := json.Unmarshal(body, &openaiResp); err != nil {
		return "", fmt.Errorf("failed to decode response: %w", err)
	}

	if openaiResp.Error != nil {
		return "", fmt.Errorf("openai error: %s", openaiResp.Error.Message)
	}

	if len(openaiResp.Choices) == 0 {
		return "", fmt.Errorf("no response from OpenAI")
	}

	return openaiResp.Choices[0].Message.Content, nil
}
