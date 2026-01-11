package workflows

import (
	"time"

	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"

	"dev/bravebird/browser-automation-go/pkg/models"
)

// BrowserAutomationWorkflow executes a browser automation workflow
func BrowserAutomationWorkflow(ctx workflow.Context, input models.WorkflowInput) (models.WorkflowResult, error) {
	logger := workflow.GetLogger(ctx)
	logger.Info("Starting browser automation workflow", "workflowID", input.WorkflowID, "runID", input.RunID)

	result := models.WorkflowResult{
		RunID:         input.RunID,
		Status:        models.StatusRunning,
		ActionResults: make([]models.ActionResult, 0, len(input.Actions)),
	}

	// Register query handler for real-time progress
	err := workflow.SetQueryHandler(ctx, "getProgress", func() (models.WorkflowResult, error) {
		return result, nil
	})
	if err != nil {
		logger.Error("Failed to register query handler", "error", err)
	}

	startTime := workflow.Now(ctx)

	// Configure activity options with retry policy
	activityOptions := workflow.ActivityOptions{
		StartToCloseTimeout: time.Duration(input.Timeout) * time.Second,
		HeartbeatTimeout:    30 * time.Second,
		RetryPolicy: &temporal.RetryPolicy{
			InitialInterval:        time.Second,
			BackoffCoefficient:     2.0,
			MaximumInterval:        time.Minute,
			MaximumAttempts:        int32(input.RetryAttempts),
			NonRetryableErrorTypes: []string{"FatalBrowserError", "InvalidSelectorError"},
		},
	}
	ctx = workflow.WithActivityOptions(ctx, activityOptions)

	// Pre-generate Go Rod code for all actions BEFORE browser initialization
	logger.Info("Pre-generating Go Rod code for all actions", "actionCount", len(input.Actions), "llmProvider", input.LLMProvider)
	var preGeneratedCode PreGeneratedCode
	err = workflow.ExecuteActivity(ctx, "PreGenerateCodeActivity", PreGenerateCodeInput{
		WorkflowID:  input.WorkflowID,
		Actions:     input.Actions,
		Parameters:  input.Parameters,
		LLMProvider: input.LLMProvider,
	}).Get(ctx, &preGeneratedCode)
	if err != nil {
		logger.Warn("Pre-generation failed, will generate code during execution", "error", err.Error())
		preGeneratedCode.ActionCodes = make(map[int]string)
	} else {
		logger.Info("Pre-generated code for actions", "count", len(preGeneratedCode.ActionCodes))
	}

	// Execute browser initialization activity
	var browserSession BrowserSession
	err = workflow.ExecuteActivity(ctx, "InitializeBrowserActivity", BrowserInitInput{
		Headless:    input.Headless,
		LLMProvider: input.LLMProvider,
	}).Get(ctx, &browserSession)
	if err != nil {
		result.Status = models.StatusFailed
		result.ErrorMessage = "Failed to initialize browser: " + err.Error()
		return result, nil
	}

	defer func() {
		// Cleanup browser session
		_ = workflow.ExecuteActivity(ctx, "CloseBrowserActivity", browserSession.SessionID).Get(ctx, nil)
	}()

	// Execute each action sequentially
	for i, action := range input.Actions {
		logger.Info("Executing action", "sequence", action.SequenceID, "type", action.ActionType)

		// Get pre-generated code if available
		generatedCode := preGeneratedCode.ActionCodes[action.SequenceID]

		actionInput := ActionInput{
			SessionID:     browserSession.SessionID,
			Action:        action,
			Parameters:    input.Parameters,
			LLMProvider:   input.LLMProvider,
			GeneratedCode: generatedCode,
		}

		var actionResult models.ActionResult
		err := workflow.ExecuteActivity(ctx, "ExecuteBrowserActionActivity", actionInput).Get(ctx, &actionResult)

		actionResult.SequenceID = action.SequenceID
		actionResult.ActionID = action.ID

		if err != nil {
			actionResult.Status = models.StatusFailed
			actionResult.ErrorMessage = err.Error()

			// Take screenshot on failure
			var screenshotPath string
			_ = workflow.ExecuteActivity(ctx, "TakeScreenshotActivity", ScreenshotInput{
				SessionID: browserSession.SessionID,
				Filename:  action.ID + "_failure.png",
			}).Get(ctx, &screenshotPath)
			actionResult.ScreenshotPath = screenshotPath

			result.ActionResults = append(result.ActionResults, actionResult)

			// Check if we should continue on failure
			if !shouldContinueOnFailure(action) {
				result.Status = models.StatusFailed
				result.ErrorMessage = "Action " + string(action.ActionType) + " failed: " + err.Error()
				break
			}
		} else {
			actionResult.Status = models.StatusSuccess
			result.ActionResults = append(result.ActionResults, actionResult)
		}

		// Signal progress for UI updates
		if i < len(input.Actions)-1 {
			workflow.SignalExternalWorkflow(ctx, "", "", "actionComplete", actionResult)
		}
	}

	// Calculate total duration
	result.TotalDuration = workflow.Now(ctx).Sub(startTime).Milliseconds()

	// Set final status
	if result.Status != models.StatusFailed {
		allSuccess := true
		for _, ar := range result.ActionResults {
			if ar.Status != models.StatusSuccess {
				allSuccess = false
				break
			}
		}
		if allSuccess {
			result.Status = models.StatusSuccess
		} else {
			result.Status = models.StatusFailed
		}
	}

	logger.Info("Workflow completed", "status", result.Status, "duration", result.TotalDuration)
	return result, nil
}

// BrowserSession holds browser session information
type BrowserSession struct {
	SessionID string `json:"session_id"`
	PageURL   string `json:"page_url"`
}

// BrowserInitInput is the input for browser initialization
type BrowserInitInput struct {
	Headless    bool   `json:"headless"`
	LLMProvider string `json:"llm_provider"`
}

// ActionInput is the input for executing a browser action
type ActionInput struct {
	SessionID     string                `json:"session_id"`
	Action        models.SemanticAction `json:"action"`
	Parameters    map[string]string     `json:"parameters"`
	LLMProvider   string                `json:"llm_provider"`
	GeneratedCode string                `json:"generated_code,omitempty"` // Pre-generated Go Rod code
}

// ScreenshotInput is the input for taking a screenshot
type ScreenshotInput struct {
	SessionID string `json:"session_id"`
	Filename  string `json:"filename"`
}

// PreGenerateCodeInput is the input for pre-generating Go Rod code for all actions
// PreGenerateCodeInput is the input for PreGenerateCodeActivity
type PreGenerateCodeInput struct {
	WorkflowID  string                  `json:"workflow_id"`
	Actions     []models.SemanticAction `json:"actions"`
	Parameters  map[string]string       `json:"parameters"`
	LLMProvider string                  `json:"llm_provider"`
}

// PreGeneratedCode holds pre-generated code for actions
type PreGeneratedCode struct {
	ActionCodes map[int]string `json:"action_codes"` // SequenceID -> Generated Code
}

// shouldContinueOnFailure determines if workflow should continue after action failure
func shouldContinueOnFailure(action models.SemanticAction) bool {
	// Continue on low-rank actions
	if action.InteractionRank == models.RankLow {
		return true
	}
	// Don't continue on critical actions
	if action.ActionType == models.ActionNavigate || action.ActionType == models.ActionInput {
		return false
	}
	return true
}

// ParallelWorkflowInput represents input for parallel workflow execution
type ParallelWorkflowInput struct {
	WorkflowID  string                  `json:"workflow_id"`
	Actions     []models.SemanticAction `json:"actions"`
	RunConfigs  []RunConfig             `json:"run_configs"`
	LLMProvider string                  `json:"llm_provider"`
	Headless    bool                    `json:"headless"`
}

// RunConfig represents a single run configuration
type RunConfig struct {
	RunID      string            `json:"run_id"`
	Parameters map[string]string `json:"parameters"`
}

// ParallelWorkflowResult represents the result of parallel execution
type ParallelWorkflowResult struct {
	Results []models.WorkflowResult `json:"results"`
}

// ParallelBrowserAutomationWorkflow executes multiple workflow runs in parallel
func ParallelBrowserAutomationWorkflow(ctx workflow.Context, input ParallelWorkflowInput) (ParallelWorkflowResult, error) {
	logger := workflow.GetLogger(ctx)
	logger.Info("Starting parallel browser automation", "workflowID", input.WorkflowID, "runCount", len(input.RunConfigs))

	result := ParallelWorkflowResult{
		Results: make([]models.WorkflowResult, len(input.RunConfigs)),
	}

	// Create child workflow options
	childOptions := workflow.ChildWorkflowOptions{
		WorkflowID: input.WorkflowID + "-child",
	}
	childCtx := workflow.WithChildOptions(ctx, childOptions)

	// Execute child workflows in parallel using selectors
	selector := workflow.NewSelector(ctx)
	futures := make([]workflow.ChildWorkflowFuture, len(input.RunConfigs))

	for i, runConfig := range input.RunConfigs {
		childInput := models.WorkflowInput{
			WorkflowID:    input.WorkflowID,
			RunID:         runConfig.RunID,
			Parameters:    runConfig.Parameters,
			Actions:       input.Actions,
			LLMProvider:   input.LLMProvider,
			Headless:      input.Headless,
			Timeout:       300,
			RetryAttempts: 3,
		}

		future := workflow.ExecuteChildWorkflow(childCtx, BrowserAutomationWorkflow, childInput)
		futures[i] = future

		idx := i
		selector.AddFuture(future, func(f workflow.Future) {
			var childResult models.WorkflowResult
			if err := f.Get(ctx, &childResult); err != nil {
				childResult = models.WorkflowResult{
					RunID:        runConfig.RunID,
					Status:       models.StatusFailed,
					ErrorMessage: err.Error(),
				}
			}
			result.Results[idx] = childResult
		})
	}

	// Wait for all child workflows to complete
	for range input.RunConfigs {
		selector.Select(ctx)
	}

	logger.Info("Parallel workflow completed", "totalRuns", len(input.RunConfigs))
	return result, nil
}
