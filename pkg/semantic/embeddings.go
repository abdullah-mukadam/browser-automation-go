package semantic

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

// EmbeddingService generates embeddings using Ollama
type EmbeddingService struct {
	ollamaHost string
	model      string
	httpClient *http.Client
}

// NewEmbeddingService creates a new embedding service
func NewEmbeddingService(ollamaHost, model string) *EmbeddingService {
	if model == "" {
		model = "nomic-embed-text" // Good embedding model for Ollama
	}
	if ollamaHost == "" {
		ollamaHost = "http://localhost:11434"
	}
	return &EmbeddingService{
		ollamaHost: ollamaHost,
		model:      model,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// OllamaEmbeddingRequest represents the request to Ollama embedding API
type OllamaEmbeddingRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
}

// OllamaEmbeddingResponse represents the response from Ollama embedding API
type OllamaEmbeddingResponse struct {
	Embedding []float32 `json:"embedding"`
}

// GenerateEmbedding creates an embedding for a single text
func (e *EmbeddingService) GenerateEmbedding(ctx context.Context, text string) ([]float32, error) {
	if text == "" {
		return nil, fmt.Errorf("empty text")
	}

	reqBody := OllamaEmbeddingRequest{
		Model:  e.model,
		Prompt: text,
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/api/embeddings", e.ollamaHost)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := e.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to call Ollama: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("Ollama returned status %d: %s", resp.StatusCode, string(body))
	}

	var embResp OllamaEmbeddingResponse
	if err := json.NewDecoder(resp.Body).Decode(&embResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return embResp.Embedding, nil
}

// GenerateActionEmbedding creates an embedding for a semantic action
func (e *EmbeddingService) GenerateActionEmbedding(ctx context.Context, action models.SemanticAction) ([]float32, error) {
	// Create a text representation of the action
	text := e.actionToText(action)
	return e.GenerateEmbedding(ctx, text)
}

// actionToText converts a semantic action to a text representation for embedding
func (e *EmbeddingService) actionToText(action models.SemanticAction) string {
	var parts []string

	// Action type
	parts = append(parts, fmt.Sprintf("Action: %s", action.ActionType))

	// Target information
	if action.Target.Tag != "" {
		parts = append(parts, fmt.Sprintf("Element: %s", action.Target.Tag))
	}
	if action.Target.Text != "" {
		parts = append(parts, fmt.Sprintf("Text: %s", truncateForEmbedding(action.Target.Text, 100)))
	}
	if action.Target.Selector != "" {
		parts = append(parts, fmt.Sprintf("Selector: %s", action.Target.Selector))
	}

	// Value for input actions
	if action.Value != "" {
		parts = append(parts, fmt.Sprintf("Value: %s", action.Value))
	}

	// Key attributes
	for key, value := range action.Target.Attributes {
		if key == "aria-label" || key == "placeholder" || key == "name" || key == "role" {
			if strVal, ok := value.(string); ok && strVal != "" {
				parts = append(parts, fmt.Sprintf("%s: %s", key, strVal))
			}
		}
	}

	return joinParts(parts)
}

// EmbedActions generates embeddings for a list of actions
func (e *EmbeddingService) EmbedActions(ctx context.Context, actions []models.SemanticAction) ([]models.SemanticAction, error) {
	for i := range actions {
		embedding, err := e.GenerateActionEmbedding(ctx, actions[i])
		if err != nil {
			// Log but don't fail - embeddings are optional
			fmt.Printf("Warning: failed to generate embedding for action %d: %v\n", actions[i].SequenceID, err)
			continue
		}
		actions[i].Embeddings = embedding
	}
	return actions, nil
}

// CosineSimilarity calculates the cosine similarity between two embedding vectors
func CosineSimilarity(a, b []float32) float32 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}

	var dotProduct, normA, normB float32
	for i := range a {
		dotProduct += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}

	if normA == 0 || normB == 0 {
		return 0
	}

	return dotProduct / (sqrt(normA) * sqrt(normB))
}

// FindSimilarActions finds actions similar to a query action
func (e *EmbeddingService) FindSimilarActions(
	ctx context.Context,
	queryAction models.SemanticAction,
	candidateActions []models.SemanticAction,
	threshold float32,
) ([]models.SemanticAction, error) {
	// Generate embedding for query if not present
	queryEmbedding := queryAction.Embeddings
	if len(queryEmbedding) == 0 {
		var err error
		queryEmbedding, err = e.GenerateActionEmbedding(ctx, queryAction)
		if err != nil {
			return nil, fmt.Errorf("failed to embed query action: %w", err)
		}
	}

	var similar []models.SemanticAction
	for _, candidate := range candidateActions {
		if len(candidate.Embeddings) == 0 {
			continue
		}

		similarity := CosineSimilarity(queryEmbedding, candidate.Embeddings)
		if similarity >= threshold {
			similar = append(similar, candidate)
		}
	}

	return similar, nil
}

// truncateForEmbedding truncates text to a reasonable length for embedding
func truncateForEmbedding(text string, maxLen int) string {
	if len(text) <= maxLen {
		return text
	}
	return text[:maxLen]
}

// joinParts joins text parts with spaces
func joinParts(parts []string) string {
	result := ""
	for i, part := range parts {
		if i > 0 {
			result += " | "
		}
		result += part
	}
	return result
}

// sqrt calculates square root for float32
func sqrt(x float32) float32 {
	// Use Newton's method
	if x <= 0 {
		return 0
	}
	z := x
	for i := 0; i < 10; i++ {
		z -= (z*z - x) / (2 * z)
	}
	return z
}

// IsAvailable checks if the Ollama service is available
func (e *EmbeddingService) IsAvailable(ctx context.Context) bool {
	url := fmt.Sprintf("%s/api/tags", e.ollamaHost)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return false
	}

	resp, err := e.httpClient.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	return resp.StatusCode == http.StatusOK
}

// PullModel pulls an embedding model if not already available
func (e *EmbeddingService) PullModel(ctx context.Context) error {
	reqBody := map[string]string{
		"name": e.model,
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/api/pull", e.ollamaHost)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonBody))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	// Use a longer timeout for model pulling
	client := &http.Client{Timeout: 10 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to pull model: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to pull model, status %d: %s", resp.StatusCode, string(body))
	}

	// Consume the streaming response
	io.Copy(io.Discard, resp.Body)

	return nil
}

// ValueCategory represents a semantic category for input values
type ValueCategory struct {
	Name        string   // e.g., "searchQuery", "email", "password"
	DisplayName string   // e.g., "Search Query", "Email", "Password"
	Examples    []string // Example values for this category
	Embedding   []float32
}

// predefined categories for value classification
var valueCategories = []ValueCategory{
	{Name: "searchQuery", DisplayName: "Search Query", Examples: []string{"search for products", "find restaurants nearby", "weather today", "how to cook pasta"}},
	{Name: "email", DisplayName: "Email", Examples: []string{"john@example.com", "user@gmail.com", "contact@company.org"}},
	{Name: "password", DisplayName: "Password", Examples: []string{"secretPassword123", "MyP@ssw0rd!", "abc123secure"}},
	{Name: "username", DisplayName: "Username", Examples: []string{"johndoe", "user123", "admin_user", "test.user"}},
	{Name: "fullName", DisplayName: "Full Name", Examples: []string{"John Doe", "Jane Smith", "Robert Johnson"}},
	{Name: "firstName", DisplayName: "First Name", Examples: []string{"John", "Jane", "Robert", "Alice"}},
	{Name: "lastName", DisplayName: "Last Name", Examples: []string{"Doe", "Smith", "Johnson", "Williams"}},
	{Name: "phoneNumber", DisplayName: "Phone Number", Examples: []string{"+1-555-123-4567", "555-1234", "9876543210"}},
	{Name: "address", DisplayName: "Address", Examples: []string{"123 Main St, New York", "456 Oak Ave", "Suite 200, Building A"}},
	{Name: "url", DisplayName: "URL", Examples: []string{"https://example.com", "http://google.com", "www.github.com"}},
	{Name: "creditCard", DisplayName: "Credit Card", Examples: []string{"4111111111111111", "5500000000000004"}},
	{Name: "date", DisplayName: "Date", Examples: []string{"2024-01-15", "January 15, 2024", "01/15/2024"}},
	{Name: "comment", DisplayName: "Comment", Examples: []string{"This is a great product!", "I have a question about...", "Please help me with..."}},
	{Name: "message", DisplayName: "Message", Examples: []string{"Hello, how are you?", "Thank you for your help", "I would like to inquire about..."}},
	{Name: "quantity", DisplayName: "Quantity", Examples: []string{"1", "5", "100", "2 items"}},
	{Name: "price", DisplayName: "Price", Examples: []string{"$99.99", "100.00", "€50"}},
}

// ClassifyValueType uses embeddings to classify an input value into a semantic category
func (e *EmbeddingService) ClassifyValueType(ctx context.Context, value string) (string, error) {
	if value == "" {
		return "input", nil
	}

	// Quick heuristic checks first (faster than embeddings)
	if isEmail(value) {
		return "email", nil
	}
	if isURL(value) {
		return "url", nil
	}
	if isPhoneNumber(value) {
		return "phoneNumber", nil
	}
	if isNumeric(value) {
		return "quantity", nil
	}

	// Generate embedding for the input value
	valueEmbedding, err := e.GenerateEmbedding(ctx, value)
	if err != nil {
		// Fallback to heuristic if embedding fails
		return classifyByHeuristic(value), nil
	}

	// Find the most similar category
	bestMatch := "input"
	bestScore := float32(0.0)

	for _, category := range valueCategories {
		// Generate embedding for category examples (cached in production)
		exampleText := joinParts(category.Examples)
		categoryEmbedding, err := e.GenerateEmbedding(ctx, exampleText)
		if err != nil {
			continue
		}

		similarity := CosineSimilarity(valueEmbedding, categoryEmbedding)
		if similarity > bestScore {
			bestScore = similarity
			bestMatch = category.Name
		}
	}

	// Only return the match if confidence is high enough
	if bestScore < 0.5 {
		return classifyByHeuristic(value), nil
	}

	return bestMatch, nil
}

// Helper functions for quick heuristic classification
func isEmail(s string) bool {
	return len(s) > 5 && 
		len(s) < 100 && 
		containsAt(s) && 
		containsDot(s) && 
		!containsSpace(s)
}

func isURL(s string) bool {
	return len(s) > 7 && 
		(hasPrefix(s, "http://") || hasPrefix(s, "https://") || hasPrefix(s, "www."))
}

func isPhoneNumber(s string) bool {
	digits := 0
	for _, c := range s {
		if c >= '0' && c <= '9' {
			digits++
		}
	}
	return digits >= 7 && digits <= 15 && len(s) <= 20
}

func isNumeric(s string) bool {
	if len(s) == 0 {
		return false
	}
	for _, c := range s {
		if (c < '0' || c > '9') && c != '.' && c != ',' {
			return false
		}
	}
	return true
}

func containsAt(s string) bool {
	for _, c := range s {
		if c == '@' {
			return true
		}
	}
	return false
}

func containsDot(s string) bool {
	for _, c := range s {
		if c == '.' {
			return true
		}
	}
	return false
}

func containsSpace(s string) bool {
	for _, c := range s {
		if c == ' ' {
			return true
		}
	}
	return false
}

func hasPrefix(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}

func classifyByHeuristic(value string) string {
	// Word count heuristics
	words := 0
	inWord := false
	for _, c := range value {
		if c == ' ' || c == '\t' || c == '\n' {
			inWord = false
		} else if !inWord {
			inWord = true
			words++
		}
	}

	// Long text with multiple words = likely a message or search query
	if words >= 3 {
		if len(value) > 100 {
			return "message"
		}
		return "searchQuery"
	}

	// Short single word
	if words == 1 {
		if len(value) <= 20 {
			return "username"
		}
	}

	return "input"
}

