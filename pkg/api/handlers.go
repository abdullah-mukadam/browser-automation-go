package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	"go.temporal.io/sdk/client"

	"dev/bravebird/browser-automation-go/pkg/database"
	"dev/bravebird/browser-automation-go/pkg/ingestion"
	"dev/bravebird/browser-automation-go/pkg/llm"
	"dev/bravebird/browser-automation-go/pkg/models"
	"dev/bravebird/browser-automation-go/pkg/semantic"
)

const TaskQueue = "browser-automation"

// Handlers contains API handlers
type Handlers struct {
	db               *database.DB
	temporalClient   client.Client
	llmConfigs       map[string]llm.Config
	runtimeAPIKeys   map[string]string // API keys submitted via UI
	embeddingService *semantic.EmbeddingService
	upgrader         websocket.Upgrader
}

// NewHandlers creates new API handlers
func NewHandlers(
	db *database.DB,
	temporalClient client.Client,
	llmConfigs map[string]llm.Config,
	embeddingService *semantic.EmbeddingService,
) *Handlers {
	return &Handlers{
		db:               db,
		temporalClient:   temporalClient,
		llmConfigs:       llmConfigs,
		runtimeAPIKeys:   make(map[string]string),
		embeddingService: embeddingService,
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool { return true },
		},
	}
}

// ==================== Workflow Handlers ====================

// ListWorkflows lists all workflow definitions
func (h *Handlers) ListWorkflows(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	if h.db == nil {
		http.Error(w, "Database not available", http.StatusServiceUnavailable)
		return
	}

	workflows, err := h.db.ListWorkflowDefinitions(ctx)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	respondJSON(w, workflows)
}

// CreateWorkflow creates a new workflow from uploaded events file
func (h *Handlers) CreateWorkflow(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Parse multipart form
	if err := r.ParseMultipartForm(100 << 20); err != nil { // 100MB max
		http.Error(w, "Failed to parse form: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Get file
	file, header, err := r.FormFile("events_file")
	if err != nil {
		http.Error(w, "Missing events_file", http.StatusBadRequest)
		return
	}
	defer file.Close()

	// Read file content
	content, err := io.ReadAll(file)
	if err != nil {
		http.Error(w, "Failed to read file", http.StatusInternalServerError)
		return
	}

	// Parse events
	var parser *ingestion.HybridParser

	// Check file extension for proto binary
	ext := filepath.Ext(header.Filename)
	if strings.ToLower(ext) == ".bin" {
		p := ingestion.NewProtoParser()
		if err := p.Parse(content); err != nil {
			http.Error(w, "Failed to parse proto events: "+err.Error(), http.StatusBadRequest)
			return
		}
		parser = p.HybridParser
	} else {
		// Default to JSON
		p := ingestion.NewHybridParser()
		if err := p.Parse(content); err != nil {
			http.Error(w, "Failed to parse events: "+err.Error(), http.StatusBadRequest)
			return
		}
		parser = p
	}

	// Parse tolerance
	toleranceStr := r.FormValue("tolerance")
	tolerance := semantic.ToleranceMedium // Default
	switch strings.ToLower(toleranceStr) {
	case "low":
		tolerance = semantic.ToleranceLow
	case "high":
		tolerance = semantic.ToleranceHigh
	}

	// Extract semantic actions
	extractor := semantic.NewExtractor(parser, tolerance)
	actions := extractor.ExtractActions()

	// Identify variable tokens using semantic classification
	var classifier semantic.ValueClassifier
	if provider := r.FormValue("llm_provider"); provider != "" {
		config := llm.Config{
			Provider: provider,
		}
		if p, err := llm.NewProvider(config); err == nil {
			classifier = p
		}
	}

	params := extractor.IdentifyVariableTokens(r.Context(), actions, classifier)

	// Save file to disk
	uploadsDir := "/tmp/uploads"
	os.MkdirAll(uploadsDir, 0755)
	filePath := filepath.Join(uploadsDir, fmt.Sprintf("%s_%s", uuid.New().String(), header.Filename))
	if err := os.WriteFile(filePath, content, 0644); err != nil {
		http.Error(w, "Failed to save file", http.StatusInternalServerError)
		return
	}

	// Create workflow definition
	actionsJSON, _ := json.Marshal(actions)
	paramsJSON, _ := json.Marshal(params)

	workflow := &models.WorkflowDefinition{
		ID:              uuid.New().String(),
		Name:            r.FormValue("name"),
		EventsFilePath:  filePath,
		StartURL:        parser.GetStartURL(),
		SemanticContext: string(actionsJSON),
		ParametersJSON:  string(paramsJSON),
	}

	if workflow.Name == "" {
		workflow.Name = header.Filename
	}

	if h.db != nil {
		if err := h.db.CreateWorkflowDefinition(ctx, workflow); err != nil {
			http.Error(w, "Failed to create workflow: "+err.Error(), http.StatusInternalServerError)
			return
		}

		// Store semantic actions
		for i := range actions {
			actions[i].ID = uuid.New().String()
			actions[i].WorkflowID = workflow.ID
		}
		if err := h.db.CreateSemanticActions(ctx, workflow.ID, actions); err != nil {
			http.Error(w, "Failed to store actions: "+err.Error(), http.StatusInternalServerError)
			return
		}
	}

	// Return workflow with parsed data
	workflow.Actions = actions
	workflow.Parameters = params

	respondJSON(w, workflow)
}

// GetWorkflow retrieves a workflow definition
func (h *Handlers) GetWorkflow(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	vars := mux.Vars(r)
	id := vars["id"]

	if h.db == nil {
		http.Error(w, "Database not available", http.StatusServiceUnavailable)
		return
	}

	workflow, err := h.db.GetWorkflowDefinition(ctx, id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if workflow == nil {
		http.Error(w, "Workflow not found", http.StatusNotFound)
		return
	}

	// Load actions
	actions, _ := h.db.GetSemanticActions(ctx, id)
	workflow.Actions = actions

	// Parse parameters
	if workflow.ParametersJSON != "" {
		json.Unmarshal([]byte(workflow.ParametersJSON), &workflow.Parameters)
	}

	respondJSON(w, workflow)
}

// DeleteWorkflow deletes a workflow definition
func (h *Handlers) DeleteWorkflow(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	vars := mux.Vars(r)
	id := vars["id"]

	if h.db == nil {
		http.Error(w, "Database not available", http.StatusServiceUnavailable)
		return
	}

	if err := h.db.DeleteWorkflowDefinition(ctx, id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// GenerateWorkflow generates the Temporal workflow code
func (h *Handlers) GenerateWorkflow(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	vars := mux.Vars(r)
	id := vars["id"]

	if h.db == nil {
		http.Error(w, "Database not available", http.StatusServiceUnavailable)
		return
	}

	workflow, err := h.db.GetWorkflowDefinition(ctx, id)
	if err != nil || workflow == nil {
		http.Error(w, "Workflow not found", http.StatusNotFound)
		return
	}

	// Get actions
	actions, _ := h.db.GetSemanticActions(ctx, id)

	// Parse params
	var params []models.WorkflowParameter
	json.Unmarshal([]byte(workflow.ParametersJSON), &params)

	// Get LLM provider preference from request
	var req struct {
		LLMProvider string `json:"llm_provider"`
	}
	json.NewDecoder(r.Body).Decode(&req)

	providerName := req.LLMProvider
	if providerName == "" {
		providerName = "ollama"
	}

	// Generate workflow code using LLM
	config, ok := h.llmConfigs[providerName]
	if !ok {
		config = h.llmConfigs["ollama"]
	}

	provider, _ := llm.NewProvider(config)
	code, err := provider.GenerateCompleteWorkflow(ctx, actions, params)
	if err != nil {
		http.Error(w, "Failed to generate workflow: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Update workflow as generated
	workflow.IsWorkflowGenerated = true
	h.db.UpdateWorkflowDefinition(ctx, workflow)

	respondJSON(w, map[string]interface{}{
		"workflow_id": id,
		"code":        code,
		"generated":   true,
	})
}

// GetWorkflowActions returns the semantic actions for a workflow
func (h *Handlers) GetWorkflowActions(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	vars := mux.Vars(r)
	id := vars["id"]

	if h.db == nil {
		http.Error(w, "Database not available", http.StatusServiceUnavailable)
		return
	}

	actions, err := h.db.GetSemanticActions(ctx, id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	respondJSON(w, actions)
}

// ==================== Run Handlers ====================

// ExecuteWorkflow executes a workflow
func (h *Handlers) ExecuteWorkflow(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	vars := mux.Vars(r)
	workflowID := vars["id"]

	var req models.ExecuteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	req.WorkflowID = workflowID

	// Get workflow
	if h.db == nil {
		http.Error(w, "Database not available", http.StatusServiceUnavailable)
		return
	}

	workflow, err := h.db.GetWorkflowDefinition(ctx, workflowID)
	if err != nil || workflow == nil {
		http.Error(w, "Workflow not found", http.StatusNotFound)
		return
	}

	actions, _ := h.db.GetSemanticActions(ctx, workflowID)

	// Filter actions to remove noise (focus/blur and low-rank clicks)
	filteredActions := make([]models.SemanticAction, 0, len(actions))
	for _, action := range actions {
		// Filter out Focus/Blur actions as they are unreliable/noisy
		if action.ActionType == models.ActionFocus || action.ActionType == models.ActionBlur {
			continue
		}
		// Filter out low-importance clicks (unless rank is not set)
		if action.ActionType == models.ActionClick && action.InteractionRank == models.RankLow {
			continue
		}
		filteredActions = append(filteredActions, action)
	}
	actions = filteredActions

	// Create run record
	runID := uuid.New().String()
	paramsJSON, _ := json.Marshal(req.Parameters)

	run := &models.WorkflowRun{
		ID:             runID,
		WorkflowID:     workflowID,
		Status:         models.StatusPending,
		ParametersJSON: string(paramsJSON),
	}

	if err := h.db.CreateWorkflowRun(ctx, run); err != nil {
		http.Error(w, "Failed to create run: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Start Temporal workflow
	// Get API key from runtime keys if available
	llmAPIKey := ""
	if key, ok := h.runtimeAPIKeys[req.LLMProvider]; ok {
		llmAPIKey = key
	} else if config, ok := h.llmConfigs[req.LLMProvider]; ok {
		llmAPIKey = config.APIKey
	}

	// Parse workflow parameter definitions
	var paramsDef []models.WorkflowParameter
	if workflow.ParametersJSON != "" {
		// Ignore error if parameters are malformed, treat as empty
		_ = json.Unmarshal([]byte(workflow.ParametersJSON), &paramsDef)
	}

	input := models.WorkflowInput{
		WorkflowID:    workflowID,
		RunID:         runID,
		Parameters:    req.Parameters,
		Params:        paramsDef,
		Actions:       actions,
		LLMProvider:   req.LLMProvider,
		LLMAPIKey:     llmAPIKey,
		Headless:      req.Headless,
		Timeout:       300,
		RetryAttempts: 3,
	}

	workflowOptions := client.StartWorkflowOptions{
		ID:        fmt.Sprintf("browser-automation-%s", runID),
		TaskQueue: TaskQueue,
	}

	we, err := h.temporalClient.ExecuteWorkflow(ctx, workflowOptions, "BrowserAutomationWorkflow", input)
	if err != nil {
		h.db.UpdateWorkflowRunStatus(ctx, runID, models.StatusFailed, err.Error())
		http.Error(w, "Failed to start workflow: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Update run with Temporal IDs and mark as running
	h.db.UpdateWorkflowRunStarted(ctx, runID, we.GetID(), we.GetRunID())

	respondJSON(w, map[string]interface{}{
		"run_id":               runID,
		"temporal_workflow_id": we.GetID(),
		"temporal_run_id":      we.GetRunID(),
		"status":               "running",
	})
}

// ListRuns lists workflow runs
func (h *Handlers) ListRuns(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	workflowID := r.URL.Query().Get("workflow_id")

	if h.db == nil {
		http.Error(w, "Database not available", http.StatusServiceUnavailable)
		return
	}

	var runs []models.WorkflowRun
	var err error

	if workflowID != "" {
		runs, err = h.db.ListWorkflowRuns(ctx, workflowID)
	} else {
		// List all recent runs
		runs, err = h.db.ListAllWorkflowRuns(ctx, 50)
	}

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	respondJSON(w, runs)
}

// GetRun retrieves a workflow run
func (h *Handlers) GetRun(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	vars := mux.Vars(r)
	id := vars["id"]

	if h.db == nil {
		http.Error(w, "Database not available", http.StatusServiceUnavailable)
		return
	}

	run, err := h.db.GetWorkflowRun(ctx, id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if run == nil {
		http.Error(w, "Run not found", http.StatusNotFound)
		return
	}

	// Get action results
	results, _ := h.db.GetActionResults(ctx, id)
	run.ActionResults = results

	respondJSON(w, run)
}

// CancelRun cancels a running workflow
func (h *Handlers) CancelRun(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	vars := mux.Vars(r)
	id := vars["id"]

	if h.db == nil {
		http.Error(w, "Database not available", http.StatusServiceUnavailable)
		return
	}

	run, err := h.db.GetWorkflowRun(ctx, id)
	if err != nil || run == nil {
		http.Error(w, "Run not found", http.StatusNotFound)
		return
	}

	// Cancel Temporal workflow
	if run.TemporalWorkflowID != "" {
		err = h.temporalClient.CancelWorkflow(ctx, run.TemporalWorkflowID, run.TemporalRunID)
		if err != nil {
			http.Error(w, "Failed to cancel workflow: "+err.Error(), http.StatusInternalServerError)
			return
		}
	}

	h.db.UpdateWorkflowRunStatus(ctx, id, models.StatusCanceled, "Cancelled by user")

	respondJSON(w, map[string]string{"status": "canceled"})
}

// StreamRunUpdates streams run updates via WebSocket
func (h *Handlers) StreamRunUpdates(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	runID := vars["id"]

	conn, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	ctx := r.Context()

	// Poll for updates
	ticker := time.NewTicker(500 * time.Millisecond) // Faster polling for better UX
	defer ticker.Stop()

	lastStatus := ""
	lastActionCount := 0

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			var status models.RunStatus
			var actionResults []models.ActionResult

			// Try to query Temporal workflow directly for real-time progress
			if h.temporalClient != nil {
				// Query workflow for progress using the correct workflow ID format
				temporalWorkflowID := fmt.Sprintf("browser-automation-%s", runID)
				queryResp, err := h.temporalClient.QueryWorkflow(ctx, temporalWorkflowID, "", "getProgress")
				if err == nil {
					var result models.WorkflowResult
					if queryResp.Get(&result) == nil {
						status = result.Status
						actionResults = result.ActionResults
					}
				}
			}

			// Fall back to DB if Temporal query didn't work
			if status == "" && h.db != nil {
				run, err := h.db.GetWorkflowRun(ctx, runID)
				if err != nil || run == nil {
					continue
				}
				status = run.Status
				results, _ := h.db.GetActionResults(ctx, runID)
				actionResults = results
			}

			// Send update if status or results changed
			if string(status) != lastStatus || len(actionResults) != lastActionCount {
				msg := models.WSMessage{
					Type: "run_update",
					Payload: map[string]interface{}{
						"run_id":         runID,
						"status":         status,
						"action_results": actionResults,
					},
				}
				conn.WriteJSON(msg)

				lastStatus = string(status)
				lastActionCount = len(actionResults)

				// Close if completed
				if status == models.StatusSuccess || status == models.StatusFailed || status == models.StatusCanceled {
					// Update database with final status
					if h.db != nil {
						errorMsg := ""
						if status == models.StatusFailed {
							for _, ar := range actionResults {
								if ar.ErrorMessage != "" {
									errorMsg = ar.ErrorMessage
									break
								}
							}
						}
						h.db.UpdateWorkflowRunStatus(ctx, runID, status, errorMsg)
					}
					return
				}
			}
		}
	}
}

// ==================== LLM Handlers ====================

// ListLLMProviders lists all available LLM providers with their status
func (h *Handlers) ListLLMProviders(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Define all supported providers
	allProviders := []struct {
		name        string
		displayName string
		requiresKey bool
	}{
		{"ollama", "Ollama", false},
		{"openai", "OpenAI", true},
		{"anthropic", "Anthropic", true},
		{"gemini", "Gemini", true},
	}

	providers := make([]map[string]interface{}, 0)

	for _, p := range allProviders {
		// Check if we have a config (from env) or runtime key
		config, hasEnvConfig := h.llmConfigs[p.name]
		runtimeKey, hasRuntimeKey := h.runtimeAPIKeys[p.name]

		hasKey := false
		available := false

		if p.requiresKey {
			// Check if API key is available from env or runtime
			hasKey = (hasEnvConfig && config.APIKey != "") || hasRuntimeKey

			if hasKey {
				// Create config with runtime key if needed
				testConfig := config
				if hasRuntimeKey {
					testConfig = llm.DefaultConfigs()[llm.ProviderName(p.name)]
					testConfig.APIKey = runtimeKey
				}
				provider, _ := llm.NewProvider(testConfig)
				available = provider != nil && provider.IsAvailable(ctx)
			}
		} else {
			// Ollama doesn't require a key
			hasKey = true
			if hasEnvConfig {
				provider, _ := llm.NewProvider(config)
				available = provider != nil && provider.IsAvailable(ctx)
			}
		}

		providers = append(providers, map[string]interface{}{
			"name":      p.name,
			"display":   p.displayName,
			"has_key":   hasKey,
			"available": available,
		})
	}

	respondJSON(w, providers)
}

// SetAPIKey sets an API key for a provider
func (h *Handlers) SetAPIKey(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	providerName := vars["name"]

	// Validate provider name
	validProviders := map[string]bool{"openai": true, "anthropic": true, "gemini": true}
	if !validProviders[providerName] {
		http.Error(w, "Invalid provider name", http.StatusBadRequest)
		return
	}

	var req struct {
		APIKey string `json:"api_key"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.APIKey == "" {
		http.Error(w, "API key is required", http.StatusBadRequest)
		return
	}

	// Store the runtime API key
	h.runtimeAPIKeys[providerName] = req.APIKey

	// Also update llmConfigs so it's used in workflow execution
	config := llm.DefaultConfigs()[llm.ProviderName(providerName)]
	config.APIKey = req.APIKey
	h.llmConfigs[providerName] = config

	// Verify the key works
	ctx := r.Context()
	provider, _ := llm.NewProvider(config)
	available := provider != nil && provider.IsAvailable(ctx)

	respondJSON(w, map[string]interface{}{
		"success":   true,
		"provider":  providerName,
		"available": available,
	})
}

// DeleteAPIKey removes an API key for a provider
func (h *Handlers) DeleteAPIKey(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	providerName := vars["name"]

	delete(h.runtimeAPIKeys, providerName)
	delete(h.llmConfigs, providerName)

	respondJSON(w, map[string]interface{}{
		"success":  true,
		"provider": providerName,
	})
}

// ==================== Screenshot Handlers ====================

// ServeScreenshot serves a screenshot file
func (h *Handlers) ServeScreenshot(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	filename := vars["filename"]

	// Security: Only allow files from the screenshots directory
	screenshotDir := os.Getenv("SCREENSHOT_DIR")
	if screenshotDir == "" {
		screenshotDir = "/tmp/screenshots"
	}

	filePath := filepath.Join(screenshotDir, filepath.Base(filename))

	// Check file exists
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		http.Error(w, "Screenshot not found", http.StatusNotFound)
		return
	}

	// Serve the file
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	http.ServeFile(w, r, filePath)
}

// ==================== Helpers ====================

func respondJSON(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}
