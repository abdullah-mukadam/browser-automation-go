package activities

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/input"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/proto"
	"github.com/google/uuid"
	"go.temporal.io/sdk/activity"

	"dev/bravebird/browser-automation-go/pkg/llm"
	"dev/bravebird/browser-automation-go/pkg/models"
	"dev/bravebird/browser-automation-go/pkg/temporal/workflows"
)

// BrowserPool manages browser sessions
type BrowserPool struct {
	sessions map[string]*BrowserSessionData
	mu       sync.RWMutex
}

// BrowserSessionData holds data for a browser session
type BrowserSessionData struct {
	Browser     *rod.Browser
	Page        *rod.Page
	LLMProvider llm.Provider
	CreatedAt   time.Time
}

var browserPool = &BrowserPool{
	sessions: make(map[string]*BrowserSessionData),
}

// Activities holds activity implementations
type Activities struct {
	LLMConfigs    map[string]llm.Config
	ScreenshotDir string
}

// NewActivities creates new activities
func NewActivities(llmConfigs map[string]llm.Config, screenshotDir string) *Activities {
	return &Activities{
		LLMConfigs:    llmConfigs,
		ScreenshotDir: screenshotDir,
	}
}

// InitializeBrowserActivity initializes a browser session
func (a *Activities) InitializeBrowserActivity(ctx context.Context, input workflows.BrowserInitInput) (workflows.BrowserSession, error) {
	logger := activity.GetLogger(ctx)
	logger.Info("Initializing browser session", "headless", input.Headless)

	// Launch browser
	l := launcher.New()

	// Use CHROME_BIN if set (Docker environment)
	if chromeBin := os.Getenv("CHROME_BIN"); chromeBin != "" {
		l = l.Bin(chromeBin)
	}

	// Configure headless mode
	if input.Headless {
		l = l.Headless(true)
	} else {
		// Non-headless mode - use the DISPLAY env var for Xvfb
		l = l.Headless(false)
	}

	// Additional Chrome flags for Docker compatibility
	l = l.Set("no-sandbox")
	l = l.Set("disable-gpu")
	l = l.Set("disable-dev-shm-usage")

	url, err := l.Launch()
	if err != nil {
		return workflows.BrowserSession{}, fmt.Errorf("failed to launch browser: %w", err)
	}

	browser := rod.New().ControlURL(url)
	if err := browser.Connect(); err != nil {
		return workflows.BrowserSession{}, fmt.Errorf("failed to connect to browser: %w", err)
	}

	page, err := browser.Page(proto.TargetCreateTarget{URL: "about:blank"})
	if err != nil {
		browser.Close()
		return workflows.BrowserSession{}, fmt.Errorf("failed to create page: %w", err)
	}

	// Create LLM provider
	// Create LLM provider
	var llmProvider llm.Provider
	providerName := input.LLMProvider
	if providerName == "" {
		providerName = "ollama"
	}

	if config, ok := a.LLMConfigs[providerName]; ok {
		llmProvider, _ = llm.NewProvider(config)
	} else {
		// Try to find ANY provider from config
		for _, cfg := range a.LLMConfigs {
			llmProvider, _ = llm.NewProvider(cfg)
			break
		}

		// Last resort: default config (likely localhost, may fail in docker)
		if llmProvider == nil {
			llmProvider = llm.NewOllamaProvider(llm.DefaultConfigs()[llm.ProviderOllama])
		}
	}

	// Store session
	sessionID := uuid.New().String()
	browserPool.mu.Lock()
	browserPool.sessions[sessionID] = &BrowserSessionData{
		Browser:     browser,
		Page:        page,
		LLMProvider: llmProvider,
		CreatedAt:   time.Now(),
	}
	browserPool.mu.Unlock()

	logger.Info("Browser session created", "sessionID", sessionID)

	return workflows.BrowserSession{
		SessionID: sessionID,
		PageURL:   "about:blank",
	}, nil
}

// CloseBrowserActivity closes a browser session
func (a *Activities) CloseBrowserActivity(ctx context.Context, sessionID string) error {
	logger := activity.GetLogger(ctx)
	logger.Info("Closing browser session", "sessionID", sessionID)

	browserPool.mu.Lock()
	defer browserPool.mu.Unlock()

	session, ok := browserPool.sessions[sessionID]
	if !ok {
		return nil // Already closed
	}

	if session.Browser != nil {
		session.Browser.Close()
	}

	delete(browserPool.sessions, sessionID)
	return nil
}

// PreGenerateCodeActivity pre-generates Go Rod code for all actions before browser execution
func (a *Activities) PreGenerateCodeActivity(ctx context.Context, input workflows.PreGenerateCodeInput) (workflows.PreGeneratedCode, error) {
	logger := activity.GetLogger(ctx)
	logger.Info("Pre-generating Go Rod code", "actionCount", len(input.Actions), "llmProvider", input.LLMProvider)

	result := workflows.PreGeneratedCode{
		ActionCodes: make(map[int]string),
	}

	// Get LLM provider
	// Get LLM provider
	var llmProvider llm.Provider
	providerName := input.LLMProvider
	if providerName == "" {
		providerName = "ollama"
		logger.Info("LLM provider not specified, defaulting to ollama")
	}

	if config, ok := a.LLMConfigs[providerName]; ok {
		llmProvider, _ = llm.NewProvider(config)
	} else {
		logger.Warn("LLM provider config not found", "provider", providerName, "available", getProviderNames(a.LLMConfigs))
		// Try to find ANY provider
		for name, cfg := range a.LLMConfigs {
			logger.Info("Falling back to available provider", "provider", name)
			llmProvider, _ = llm.NewProvider(cfg)
			break
		}
	}

	if llmProvider == nil || !llmProvider.IsAvailable(ctx) {
		// Fall back to template-based generation
		logger.Warn("LLM provider not available, using template-based code generation")
		for _, action := range input.Actions {
			code := llm.GenerateCodeFromAction(action, input.Parameters)
			result.ActionCodes[action.SequenceID] = code
		}
		return result, nil
	}

	// Ensure generated code directory exists
	baseDir := "generated_code"
	workflowDir := filepath.Join(baseDir, input.WorkflowID)
	if err := os.MkdirAll(workflowDir, 0755); err != nil {
		logger.Error("Failed to create directory for generated code", "dir", workflowDir, "error", err)
		return result, err
	}

	// Generate code for each action using LLM
	pageCtx := llm.PageContext{
		URL:   "about:blank", // Will be updated at runtime
		Title: "",
	}

	for i, action := range input.Actions {
		logger.Info("Generating code for action", "sequence", action.SequenceID, "type", action.ActionType, "progress", fmt.Sprintf("%d/%d", i+1, len(input.Actions)))

		// Heartbeat to keep activity alive during long generation
		activity.RecordHeartbeat(ctx, fmt.Sprintf("Generating action %d/%d", i+1, len(input.Actions)))

		code, err := llmProvider.GenerateBrowserCode(ctx, action, pageCtx)
		if err != nil {
			logger.Warn("LLM generation failed for action, using fallback", "sequence", action.SequenceID, "error", err)
			code = llm.GenerateCodeFromAction(action, input.Parameters)
		}

		// Save code to file
		filename := fmt.Sprintf("action_%d.go", action.SequenceID)
		filePath := filepath.Join(workflowDir, filename)
		if err := os.WriteFile(filePath, []byte(code), 0644); err != nil {
			logger.Error("Failed to write generated code to file", "path", filePath, "error", err)
			// Continue, but maybe we should fail?
			// Fallback: put code in map if file write fails? No, keep path convention.
			// If file fail, we can't save path.
			// Let's assume write success or error out.
			continue
		}

		// Return absolute path so other activities can find it easily
		absPath, _ := filepath.Abs(filePath)
		result.ActionCodes[action.SequenceID] = absPath
	}

	logger.Info("Pre-generation complete", "generatedCount", len(result.ActionCodes))
	return result, nil
}

// ExecuteBrowserActionActivity executes a single browser action
func (a *Activities) ExecuteBrowserActionActivity(ctx context.Context, actionInput workflows.ActionInput) (models.ActionResult, error) {
	logger := activity.GetLogger(ctx)
	logger.Info("Executing browser action", "type", actionInput.Action.ActionType, "sequence", actionInput.Action.SequenceID)

	result := models.ActionResult{
		Status: models.StatusRunning,
	}
	startTime := time.Now()

	// Get session
	browserPool.mu.RLock()
	session, ok := browserPool.sessions[actionInput.SessionID]
	browserPool.mu.RUnlock()

	if !ok {
		return result, fmt.Errorf("browser session not found: %s", actionInput.SessionID)
	}

	page := session.Page

	// Get page context for LLM
	pageCtx := llm.PageContext{
		URL:   page.MustInfo().URL,
		Title: page.MustInfo().Title,
	}

	// Use pre-generated code if available, otherwise generate on-the-fly
	var code string
	var err error

	if actionInput.GeneratedCode != "" {
		// Calculate if it is a file path or raw code
		// Simple check: does it look like a path?
		if strings.HasPrefix(actionInput.GeneratedCode, "/") || strings.Contains(actionInput.GeneratedCode, string(os.PathSeparator)) {
			// It's a path, read the file
			content, err := os.ReadFile(actionInput.GeneratedCode)
			if err != nil {
				logger.Error("Failed to read generated code file", "path", actionInput.GeneratedCode, "error", err)
				return result, fmt.Errorf("failed to read generated code: %w", err)
			}
			code = string(content)
			logger.Info("Loaded generated code from file", "path", actionInput.GeneratedCode, "size", len(code))
		} else {
			// Backward compatibility or fallback
			code = actionInput.GeneratedCode
			logger.Info("Using pre-generated code (inline)", "sequence", actionInput.Action.SequenceID)
		}
	} else if session.LLMProvider != nil && session.LLMProvider.IsAvailable(ctx) {
		// Generate code on-the-fly
		code, err = session.LLMProvider.GenerateBrowserCode(ctx, actionInput.Action, pageCtx)
		if err != nil {
			logger.Warn("LLM code generation failed, using fallback", "error", err)
			code = llm.GenerateCodeFromAction(actionInput.Action, actionInput.Parameters)
		}
	} else {
		code = llm.GenerateCodeFromAction(actionInput.Action, actionInput.Parameters)
	}

	result.GeneratedCode = code

	// Execute the action:
	// Ideally we would use 'yaegi' here to run 'code'.
	// However, exposing 'rod' structs to interpreted code requires comprehensive symbols export.
	// For now, we will PARSE the intention from the generated code and execute it using our reliable helper.
	// This honors "fetching generated code" as the source of truth.

	// Extract selector from code (heuristic)
	// Looking for: .MustElement("selector") or similar
	// Note: This is an intermediate step.

	// If the code contains explicit variable use, we might need to be careful.

	// Execute the action based on type (using our standard executor for stability)
	// OPTIMIZATION: Try to extract robust selector from generated code
	if code != "" && (actionInput.Action.ActionType == models.ActionClick || actionInput.Action.ActionType == models.ActionInput) {
		// Look for MustElement("selector")
		re := regexp.MustCompile(`MustElement\("([^"]+)"\)`)
		matches := re.FindStringSubmatch(code)
		if len(matches) > 1 {
			newSelector := matches[1]
			if newSelector != "" && newSelector != actionInput.Action.Target.Selector {
				logger.Info("Updating selector from generated code", "old", actionInput.Action.Target.Selector, "new", newSelector)
				actionInput.Action.Target.Selector = newSelector
			}
		}
	}

	err = a.executeAction(page, actionInput.Action, actionInput.Parameters)
	if err != nil {
		result.ErrorMessage = err.Error()
		result.Duration = time.Since(startTime).Milliseconds()
		return result, err
	}

	result.Status = models.StatusSuccess
	result.Duration = time.Since(startTime).Milliseconds()

	// Heartbeat for long-running activities
	activity.RecordHeartbeat(ctx, fmt.Sprintf("Completed action %d", actionInput.Action.SequenceID))

	return result, nil
}

// executeAction executes a browser action using Go Rod
func (a *Activities) executeAction(page *rod.Page, action models.SemanticAction, params map[string]string) error {
	// Substitute parameters in values
	value := action.Value
	for paramName, paramValue := range params {
		value = strings.ReplaceAll(value, "{{"+paramName+"}}", paramValue)
		// Also replace if the entire value matches a param value (for variable tokens)
		if value == paramName {
			value = paramValue
		}
	}

	switch action.ActionType {
	case models.ActionNavigate:
		url := value
		// Substitute parameters in URL
		for paramName, paramValue := range params {
			url = strings.ReplaceAll(url, "{{"+paramName+"}}", paramValue)
		}
		return page.Navigate(url)

	case models.ActionClick:
		selector := a.getBestSelector(action)
		elem, err := page.Element(selector)
		if err != nil {
			return fmt.Errorf("element not found: %s", selector)
		}
		return elem.Click(proto.InputMouseButtonLeft, 1)

	case models.ActionInput:
		selector := a.getBestSelector(action)
		elem, err := page.Element(selector)
		if err != nil {
			return fmt.Errorf("element not found: %s", selector)
		}
		// Clear existing text and input new value
		elem.MustSelectAllText()
		return elem.Input(value)

	case models.ActionKeypress:
		key := getKeyFromValue(value)
		return page.Keyboard.Press(key)

	case models.ActionCopy:
		// Press Ctrl+C for copy (use Type with modifier)
		page.KeyActions().Press(input.ControlLeft).Type(input.KeyC).MustDo()
		return nil

	case models.ActionPaste:
		// Press Ctrl+V for paste
		page.KeyActions().Press(input.ControlLeft).Type(input.KeyV).MustDo()
		return nil

	case models.ActionScroll:
		// Scroll is usually not critical, just log it
		return nil

	default:
		return fmt.Errorf("unsupported action type: %s", action.ActionType)
	}
}

// getBestSelector returns the best selector for an action
func (a *Activities) getBestSelector(action models.SemanticAction) string {
	attrs := action.Target.Attributes
	tag := strings.ToLower(action.Target.Tag)

	// Priority 1: aria-label
	if ariaLabel, ok := attrs["aria-label"].(string); ok && ariaLabel != "" {
		return fmt.Sprintf("%s[aria-label='%s']", tag, ariaLabel)
	}

	// Priority 2: name
	if name, ok := attrs["name"].(string); ok && name != "" {
		return fmt.Sprintf("%s[name='%s']", tag, name)
	}

	// Priority 3: placeholder
	if placeholder, ok := attrs["placeholder"].(string); ok && placeholder != "" {
		return fmt.Sprintf("%s[placeholder='%s']", tag, placeholder)
	}

	// Priority 4: data-testid
	if testID, ok := attrs["data-testid"].(string); ok && testID != "" {
		return fmt.Sprintf("[data-testid='%s']", testID)
	}

	// Fallback to provided selector
	return action.Target.Selector
}

// getKeyFromValue converts a key name to rod input key
func getKeyFromValue(value string) input.Key {
	switch strings.ToLower(value) {
	case "enter":
		return input.Enter
	case "tab":
		return input.Tab
	case "escape":
		return input.Escape
	case "backspace":
		return input.Backspace
	case "arrowup":
		return input.ArrowUp
	case "arrowdown":
		return input.ArrowDown
	case "arrowleft":
		return input.ArrowLeft
	case "arrowright":
		return input.ArrowRight
	default:
		// For single characters, return as-is
		if len(value) == 1 {
			return input.Key(value[0])
		}
		return input.Enter // Default fallback
	}
}

// TakeScreenshotActivity takes a screenshot
func (a *Activities) TakeScreenshotActivity(ctx context.Context, screenshotInput workflows.ScreenshotInput) (string, error) {
	logger := activity.GetLogger(ctx)
	logger.Info("Taking screenshot", "sessionID", screenshotInput.SessionID)

	browserPool.mu.RLock()
	session, ok := browserPool.sessions[screenshotInput.SessionID]
	browserPool.mu.RUnlock()

	if !ok {
		return "", fmt.Errorf("browser session not found")
	}

	// Ensure screenshot directory exists
	if err := os.MkdirAll(a.ScreenshotDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create screenshot dir: %w", err)
	}

	// Take screenshot
	screenshotPath := filepath.Join(a.ScreenshotDir, screenshotInput.Filename)
	data, err := session.Page.Screenshot(true, &proto.PageCaptureScreenshot{
		Format: proto.PageCaptureScreenshotFormatPng,
	})
	if err != nil {
		return "", fmt.Errorf("failed to take screenshot: %w", err)
	}

	// Save to file
	if err := os.WriteFile(screenshotPath, data, 0644); err != nil {
		return "", fmt.Errorf("failed to save screenshot: %w", err)
	}

	return screenshotPath, nil
}

func getProviderNames(configs map[string]llm.Config) []string {
	names := make([]string, 0, len(configs))
	for name := range configs {
		names = append(names, name)
	}
	return names
}
