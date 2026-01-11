package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"dev/bravebird/browser-automation-go/pkg/models"
)

// OllamaProvider implements the Provider interface for Ollama
type OllamaProvider struct {
	config     Config
	httpClient *http.Client
}

// NewOllamaProvider creates a new Ollama provider
func NewOllamaProvider(config Config) *OllamaProvider {
	if config.BaseURL == "" {
		config.BaseURL = "http://localhost:11434"
	}
	if config.Model == "" {
		config.Model = "codellama:13b"
	}
	if config.Timeout == 0 {
		config.Timeout = 120
	}
	if config.Temperature == 0 {
		config.Temperature = 0.1
	}

	return &OllamaProvider{
		config: config,
		httpClient: &http.Client{
			Timeout: time.Duration(config.Timeout) * time.Second,
		},
	}
}

// OllamaRequest represents a request to the Ollama API
type OllamaRequest struct {
	Model    string                   `json:"model"`
	Prompt   string                   `json:"prompt,omitempty"`
	System   string                   `json:"system,omitempty"`
	Messages []OllamaMessage          `json:"messages,omitempty"`
	Stream   bool                     `json:"stream"`
	Options  OllamaOptions            `json:"options,omitempty"`
	Format   string                   `json:"format,omitempty"`
	Tools    []map[string]interface{} `json:"tools,omitempty"`
}

// OllamaMessage represents a chat message
type OllamaMessage struct {
	Role      string           `json:"role"`
	Content   string           `json:"content"`
	ToolCalls []OllamaToolCall `json:"tool_calls,omitempty"`
}

// OllamaToolCall represents a tool call
type OllamaToolCall struct {
	Function OllamaFunction `json:"function"`
}

// OllamaFunction represents a function call
type OllamaFunction struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

// OllamaOptions represents generation options
type OllamaOptions struct {
	Temperature float32 `json:"temperature,omitempty"`
	NumPredict  int     `json:"num_predict,omitempty"`
}

// OllamaResponse represents a response from the Ollama API
type OllamaResponse struct {
	Model    string        `json:"model"`
	Response string        `json:"response,omitempty"`
	Message  OllamaMessage `json:"message,omitempty"`
	Done     bool          `json:"done"`
	Error    string        `json:"error,omitempty"`
}

// Name returns the provider name
func (p *OllamaProvider) Name() string {
	return string(ProviderOllama)
}

// IsAvailable checks if Ollama is running and the model is available
func (p *OllamaProvider) IsAvailable(ctx context.Context) bool {
	url := fmt.Sprintf("%s/api/tags", p.config.BaseURL)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return false
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	return resp.StatusCode == http.StatusOK
}

// GenerateBrowserCode generates Go Rod code for a semantic action
func (p *OllamaProvider) GenerateBrowserCode(ctx context.Context, action models.SemanticAction, pageCtx PageContext) (string, error) {
	prompt := BuildActionPrompt(action, pageCtx, 0, "")

	response, err := p.generate(ctx, SystemPromptTemplate, prompt)
	if err != nil {
		return "", fmt.Errorf("ollama generation failed: %w", err)
	}

	// Clean up the response - extract just the code
	code := extractCode(response)
	return code, nil
}

// IdentifyVariableTokens uses Ollama to identify variable tokens
func (p *OllamaProvider) IdentifyVariableTokens(ctx context.Context, actions []models.SemanticAction) ([]models.WorkflowParameter, error) {
	prompt := BuildVariableTokenPrompt(actions)

	response, err := p.generate(ctx, "You are a JSON generator. Output ONLY valid JSON, no explanations.", prompt)
	if err != nil {
		return nil, fmt.Errorf("ollama generation failed: %w", err)
	}

	// Parse the JSON response
	var result struct {
		Parameters []models.WorkflowParameter `json:"parameters"`
	}

	// Try to extract JSON from the response
	jsonStr := extractJSON(response)
	if err := json.Unmarshal([]byte(jsonStr), &result); err != nil {
		return nil, fmt.Errorf("failed to parse variable tokens: %w", err)
	}

	return result.Parameters, nil
}

// GenerateCompleteWorkflow generates the complete workflow code
func (p *OllamaProvider) GenerateCompleteWorkflow(ctx context.Context, actions []models.SemanticAction, params []models.WorkflowParameter) (string, error) {
	prompt := BuildWorkflowPrompt(actions, params)

	response, err := p.generate(ctx, SystemPromptTemplate, prompt)
	if err != nil {
		return "", fmt.Errorf("ollama generation failed: %w", err)
	}

	return extractCode(response), nil
}

// generate makes a request to the Ollama API
func (p *OllamaProvider) generate(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	reqBody := OllamaRequest{
		Model:  p.config.Model,
		Stream: false,
		Messages: []OllamaMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt},
		},
		Options: OllamaOptions{
			Temperature: p.config.Temperature,
			NumPredict:  p.config.MaxTokens,
		},
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/api/chat", p.config.BaseURL)
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

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("ollama returned status %d: %s", resp.StatusCode, string(body))
	}

	var ollamaResp OllamaResponse
	if err := json.NewDecoder(resp.Body).Decode(&ollamaResp); err != nil {
		return "", fmt.Errorf("failed to decode response: %w", err)
	}

	if ollamaResp.Error != "" {
		return "", fmt.Errorf("ollama error: %s", ollamaResp.Error)
	}

	return ollamaResp.Message.Content, nil
}

// PullModel pulls a model from Ollama
func (p *OllamaProvider) PullModel(ctx context.Context, model string) error {
	if model == "" {
		model = p.config.Model
	}

	reqBody := map[string]string{"name": model}
	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/api/pull", p.config.BaseURL)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonBody))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	// Use longer timeout for pulling
	client := &http.Client{Timeout: 30 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("pull request failed: %w", err)
	}
	defer resp.Body.Close()

	// Read and discard streaming response
	io.Copy(io.Discard, resp.Body)

	return nil
}

// ClassifyValue classifies a value into a semantic category using a small LLM
func (p *OllamaProvider) ClassifyValue(ctx context.Context, value string) (string, error) {
	// Use checks for obvious cases to save LLM calls
	if len(value) < 2 {
		return "input", nil
	}

	systemPrompt := "You are a semantic classifier. You will be given a text value. You must output a single, short, camelCase string that describes the semantic type of this value. Examples: 'user@example.com' -> 'email', '123 Main St' -> 'address', 'search term' -> 'searchQuery'. Output ONLY the class name, nothing else."
	userPrompt := fmt.Sprintf("Classify this value: \"%s\"", value)

	// Use a small model for classification if available, otherwise use configured model
	// Ideally we'd use something like "llama3.2" or "phi3:mini" here
	model := p.config.Model

	reqBody := OllamaRequest{
		Model:  model,
		Stream: false,
		Messages: []OllamaMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt},
		},
		Options: OllamaOptions{
			Temperature: 0.1,
			NumPredict:  10, // We only need a short class name
		},
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return "input", fmt.Errorf("failed to marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/api/chat", p.config.BaseURL)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonBody))
	if err != nil {
		return "input", fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return "input", fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "input", fmt.Errorf("ollama status %d", resp.StatusCode)
	}

	var ollamaResp OllamaResponse
	if err := json.NewDecoder(resp.Body).Decode(&ollamaResp); err != nil {
		return "input", fmt.Errorf("decode failed: %w", err)
	}

	// Clean up the response
	className := strings.TrimSpace(ollamaResp.Message.Content)
	className = strings.Trim(className, "\"'`")
	className = strings.Split(className, "\n")[0] // Take first line only
	className = strings.Split(className, " ")[0]  // Take first word only

	if className == "" {
		return "input", nil
	}

	return className, nil
}

// extractCode extracts Go code from an LLM response
func extractCode(response string) string {
	// Try to find code blocks
	if strings.Contains(response, "```go") {
		start := strings.Index(response, "```go")
		end := strings.Index(response[start+5:], "```")
		if end > 0 {
			return strings.TrimSpace(response[start+5 : start+5+end])
		}
	}

	if strings.Contains(response, "```") {
		start := strings.Index(response, "```")
		end := strings.Index(response[start+3:], "```")
		if end > 0 {
			code := response[start+3 : start+3+end]
			// Remove language identifier if present
			if strings.HasPrefix(code, "go\n") {
				code = code[3:]
			}
			return strings.TrimSpace(code)
		}
	}

	// No code blocks, return trimmed response
	return strings.TrimSpace(response)
}

// extractJSON extracts JSON from an LLM response
func extractJSON(response string) string {
	// Try to find JSON blocks
	if strings.Contains(response, "```json") {
		start := strings.Index(response, "```json")
		end := strings.Index(response[start+7:], "```")
		if end > 0 {
			return strings.TrimSpace(response[start+7 : start+7+end])
		}
	}

	if strings.Contains(response, "```") {
		start := strings.Index(response, "```")
		end := strings.Index(response[start+3:], "```")
		if end > 0 {
			return strings.TrimSpace(response[start+3 : start+3+end])
		}
	}

	// Try to find raw JSON
	start := strings.Index(response, "{")
	end := strings.LastIndex(response, "}")
	if start >= 0 && end > start {
		return response[start : end+1]
	}

	return response
}
