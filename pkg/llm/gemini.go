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

// GeminiProvider implements the Provider interface for Google Gemini
type GeminiProvider struct {
	config     Config
	httpClient *http.Client
}

// NewGeminiProvider creates a new Gemini provider
func NewGeminiProvider(config Config) *GeminiProvider {
	if config.BaseURL == "" {
		config.BaseURL = "https://generativelanguage.googleapis.com"
	}
	if config.Model == "" {
		config.Model = "gemini-1.5-pro"
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

	return &GeminiProvider{
		config: config,
		httpClient: &http.Client{
			Timeout: time.Duration(config.Timeout) * time.Second,
		},
	}
}

// Gemini API types
type GeminiRequest struct {
	Contents          []GeminiContent `json:"contents"`
	SystemInstruction *GeminiContent  `json:"systemInstruction,omitempty"`
	GenerationConfig  GeminiGenConfig `json:"generationConfig,omitempty"`
	Tools             []GeminiTool    `json:"tools,omitempty"`
}

type GeminiContent struct {
	Parts []GeminiPart `json:"parts"`
	Role  string       `json:"role,omitempty"`
}

type GeminiPart struct {
	Text         string              `json:"text,omitempty"`
	FunctionCall *GeminiFunctionCall `json:"functionCall,omitempty"`
}

type GeminiFunctionCall struct {
	Name string                 `json:"name"`
	Args map[string]interface{} `json:"args"`
}

type GeminiGenConfig struct {
	Temperature     float32 `json:"temperature,omitempty"`
	MaxOutputTokens int     `json:"maxOutputTokens,omitempty"`
}

type GeminiTool struct {
	FunctionDeclarations []GeminiFunctionDecl `json:"functionDeclarations"`
}

type GeminiFunctionDecl struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	Parameters  interface{} `json:"parameters"`
}

type GeminiResponse struct {
	Candidates []GeminiCandidate `json:"candidates"`
	Error      *GeminiError      `json:"error,omitempty"`
}

type GeminiCandidate struct {
	Content       GeminiContent `json:"content"`
	FinishReason  string        `json:"finishReason"`
	SafetyRatings []interface{} `json:"safetyRatings"`
}

type GeminiError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Status  string `json:"status"`
}

// Name returns the provider name
func (p *GeminiProvider) Name() string {
	return string(ProviderGemini)
}

// IsAvailable checks if the provider is configured
func (p *GeminiProvider) IsAvailable(ctx context.Context) bool {
	return p.config.APIKey != ""
}

// GenerateBrowserCode generates Go Rod code for a semantic action
func (p *GeminiProvider) GenerateBrowserCode(ctx context.Context, action models.SemanticAction, pageCtx PageContext) (string, error) {
	if p.config.APIKey == "" {
		return "", fmt.Errorf("Gemini API key not configured")
	}

	prompt := BuildActionPrompt(action, pageCtx, 0, "")

	response, err := p.generateContent(ctx, SystemPromptTemplate, prompt)
	if err != nil {
		return "", fmt.Errorf("gemini generation failed: %w", err)
	}

	return extractCode(response), nil
}

// IdentifyVariableTokens uses Gemini to identify variable tokens
func (p *GeminiProvider) IdentifyVariableTokens(ctx context.Context, actions []models.SemanticAction) ([]models.WorkflowParameter, error) {
	if p.config.APIKey == "" {
		return nil, fmt.Errorf("Gemini API key not configured")
	}

	prompt := BuildVariableTokenPrompt(actions)

	response, err := p.generateContent(ctx, "You are a JSON generator. Output ONLY valid JSON.", prompt)
	if err != nil {
		return nil, fmt.Errorf("gemini generation failed: %w", err)
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
func (p *GeminiProvider) ClassifyValue(ctx context.Context, value string) (string, error) {
	// For now, return "input" or use heuristic to avoid API costs
	return "input", nil
}

// GenerateCompleteWorkflow generates the complete workflow code
func (p *GeminiProvider) GenerateCompleteWorkflow(ctx context.Context, actions []models.SemanticAction, params []models.WorkflowParameter) (string, error) {
	if p.config.APIKey == "" {
		return "", fmt.Errorf("Gemini API key not configured")
	}

	prompt := BuildWorkflowPrompt(actions, params)

	response, err := p.generateContent(ctx, SystemPromptTemplate, prompt)
	if err != nil {
		return "", fmt.Errorf("gemini generation failed: %w", err)
	}

	return extractCode(response), nil
}

// generateContent makes a request to the Gemini API
func (p *GeminiProvider) generateContent(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	reqBody := GeminiRequest{
		SystemInstruction: &GeminiContent{
			Parts: []GeminiPart{{Text: systemPrompt}},
		},
		Contents: []GeminiContent{
			{
				Role:  "user",
				Parts: []GeminiPart{{Text: userPrompt}},
			},
		},
		GenerationConfig: GeminiGenConfig{
			Temperature:     p.config.Temperature,
			MaxOutputTokens: p.config.MaxTokens,
		},
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/v1beta/models/%s:generateContent?key=%s",
		p.config.BaseURL, p.config.Model, p.config.APIKey)

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonBody))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	var geminiResp GeminiResponse
	if err := json.Unmarshal(body, &geminiResp); err != nil {
		return "", fmt.Errorf("failed to decode response: %w", err)
	}

	if geminiResp.Error != nil {
		return "", fmt.Errorf("gemini error: %s", geminiResp.Error.Message)
	}

	if len(geminiResp.Candidates) == 0 {
		return "", fmt.Errorf("no candidates in response")
	}

	// Extract text from parts
	var textContent string
	for _, part := range geminiResp.Candidates[0].Content.Parts {
		if part.Text != "" {
			textContent += part.Text
		}
	}

	if textContent == "" {
		return "", fmt.Errorf("no text content in response")
	}

	return textContent, nil
}
