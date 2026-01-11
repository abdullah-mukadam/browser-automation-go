package models

import (
	"encoding/json"
	"time"
)

// ==================== Hybrid Event Types ====================

// HybridEvent represents a unified event from hybrid_events.json
// Can be either rrweb source or custom source
type HybridEvent struct {
	Source    string          `json:"source"`
	Timestamp int64           `json:"timestamp"`
	Type      interface{}     `json:"type"` // int for rrweb, string for custom
	Data      json.RawMessage `json:"data,omitempty"`
	// Custom source fields
	Target    *EventTarget  `json:"target,omitempty"`
	Key       string        `json:"key,omitempty"`
	Modifiers *KeyModifiers `json:"modifiers,omitempty"`
	Shortcut  string        `json:"shortcut,omitempty"`
	Value     string        `json:"value,omitempty"`
}

// EventTarget represents the target element of a custom event
type EventTarget struct {
	Tag      string `json:"tag"`
	Selector string `json:"selector"`
	Text     string `json:"text,omitempty"`
}

// KeyModifiers represents keyboard modifiers for key events
type KeyModifiers struct {
	Alt   bool `json:"alt"`
	Ctrl  bool `json:"ctrl"`
	Meta  bool `json:"meta"`
	Shift bool `json:"shift"`
}

// ==================== RRWeb Event Types ====================

// RRWeb event type constants
const (
	RRWebEventDomContentLoaded = 0
	RRWebEventLoad             = 1
	RRWebEventFullSnapshot     = 2
	RRWebEventIncremental      = 3
	RRWebEventMeta             = 4
	RRWebEventCustom           = 5
)

// RRWeb incremental source types (complete list)
const (
	SourceMutation          = 0  // DOM mutations (adds, removes, changes)
	SourceMouseMove         = 1  // Mouse movement
	SourceMouseInteraction  = 2  // Clicks, double-clicks, context menu, etc.
	SourceScroll            = 3  // Scroll events
	SourceViewportResize    = 4  // Viewport size changes
	SourceInput             = 5  // Form input changes
	SourceTouchMove         = 6  // Touch movement
	SourceMediaInteraction  = 7  // Video/audio play, pause, seek
	SourceStyleSheetRule    = 8  // CSS rule changes
	SourceCanvasMutation    = 9  // Canvas element changes
	SourceFont              = 10 // Font loading
	SourceLog               = 11 // Console log
	SourceDrag              = 12 // Drag events
	SourceStyleDeclaration  = 13 // Inline style changes
	SourceSelection         = 14 // Text selection
	SourceAdoptedStyleSheet = 15 // Adopted stylesheets
	SourceCustomElement     = 16 // Custom element mutations
)

// MouseInteraction types (for SourceMouseInteraction)
const (
	MouseInteractionMouseUp     = 0
	MouseInteractionMouseDown   = 1
	MouseInteractionClick       = 2
	MouseInteractionContextMenu = 3
	MouseInteractionDblClick    = 4
	MouseInteractionFocus       = 5
	MouseInteractionBlur        = 6
	MouseInteractionTouchStart  = 7
	MouseInteractionTouchEnd    = 9
)

// RRWebEventData represents the nested data structure in rrweb events
type RRWebEventData struct {
	Data      json.RawMessage `json:"data"`
	Timestamp int64           `json:"timestamp"`
	Type      int             `json:"type"`
}

// RRWebMetaData represents meta event data
type RRWebMetaData struct {
	Href   string `json:"href"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
}

// RRWebIncrementalData represents incremental snapshot data
type RRWebIncrementalData struct {
	Source  int            `json:"source"`
	Type    int            `json:"type,omitempty"` // Mouse event type
	ID      int            `json:"id,omitempty"`
	X       int            `json:"x,omitempty"`
	Y       int            `json:"y,omitempty"`
	Text    string         `json:"text,omitempty"`
	Adds    []NodeAddition `json:"adds,omitempty"`
	Removes []NodeRemoval  `json:"removes,omitempty"`
}

// NodeAddition represents a DOM node addition
type NodeAddition struct {
	ParentID int             `json:"parentId"`
	Node     *SerializedNode `json:"node"`
}

// NodeRemoval represents a DOM node removal
type NodeRemoval struct {
	ParentID int `json:"parentId"`
	ID       int `json:"id"`
}

// SerializedNode represents a serialized DOM node
type SerializedNode struct {
	ID          int                    `json:"id"`
	Type        int                    `json:"type"`
	TagName     string                 `json:"tagName,omitempty"`
	Attributes  map[string]interface{} `json:"attributes,omitempty"`
	ChildNodes  []*SerializedNode      `json:"childNodes,omitempty"`
	TextContent string                 `json:"textContent,omitempty"`
}

// ==================== Semantic Action Types ====================

// SemanticAction represents a processed, meaningful browser action
type SemanticAction struct {
	ID              string                 `json:"id"`
	WorkflowID      string                 `json:"workflow_id"`
	SequenceID      int                    `json:"sequence_id"`
	ActionType      ActionType             `json:"action_type"`
	Target          SemanticTarget         `json:"target"`
	Value           string                 `json:"value,omitempty"`
	InteractionRank InteractionRank        `json:"interaction_rank"`
	Context         []SemanticTarget       `json:"context,omitempty"` // New elements that appeared
	Embeddings      []float32              `json:"embeddings,omitempty"`
	Metadata        map[string]interface{} `json:"metadata,omitempty"`
	Timestamp       int64                  `json:"timestamp"`
}

// ActionType represents the type of browser action
type ActionType string

const (
	ActionNavigate   ActionType = "navigate"    // Navigate to URL
	ActionClick      ActionType = "click"       // Single click
	ActionDblClick   ActionType = "dblclick"    // Double click
	ActionRightClick ActionType = "rightclick"  // Context menu / right click
	ActionInput      ActionType = "input"       // Type text
	ActionKeypress   ActionType = "keypress"    // Keyboard key
	ActionScroll     ActionType = "scroll"      // Scroll
	ActionHover      ActionType = "hover"       // Mouse hover
	ActionFocus      ActionType = "focus"       // Element focus
	ActionBlur       ActionType = "blur"        // Element blur
	ActionSelect     ActionType = "select"      // Text selection
	ActionCopy       ActionType = "copy"        // Ctrl+C
	ActionPaste      ActionType = "paste"       // Ctrl+V
	ActionCut        ActionType = "cut"         // Ctrl+X
	ActionDrag       ActionType = "drag"        // Drag start
	ActionDrop       ActionType = "drop"        // Drop
	ActionMediaPlay  ActionType = "media_play"  // Video/audio play
	ActionMediaPause ActionType = "media_pause" // Video/audio pause
	ActionMediaSeek  ActionType = "media_seek"  // Video/audio seek
	ActionFileUpload ActionType = "file_upload" // File input
	ActionSubmit     ActionType = "submit"      // Form submit
)

// InteractionRank represents how important/reliable an interaction is
type InteractionRank string

const (
	RankHigh   InteractionRank = "High"
	RankMedium InteractionRank = "Medium"
	RankLow    InteractionRank = "Low"
)

// SemanticTarget represents the target element of an action with rich context
type SemanticTarget struct {
	Tag        string                 `json:"tag"`
	Text       string                 `json:"text,omitempty"`
	Selector   string                 `json:"selector,omitempty"`
	Attributes map[string]interface{} `json:"attributes,omitempty"`
	XPath      string                 `json:"xpath,omitempty"`
	NodeID     int                    `json:"node_id,omitempty"`
}

// ==================== Workflow Types ====================

// WorkflowDefinition represents a stored workflow created from recorded events
type WorkflowDefinition struct {
	ID                  string    `json:"id" db:"id"`
	Name                string    `json:"name" db:"name"`
	EventsFilePath      string    `json:"events_file_path" db:"events_file_path"`
	IsWorkflowGenerated bool      `json:"is_workflow_generated" db:"is_workflow_generated"`
	SemanticContext     string    `json:"semantic_context" db:"semantic_context"` // JSON string
	ParametersJSON      string    `json:"parameters" db:"parameters"`             // JSON string
	StartURL            string    `json:"start_url" db:"start_url"`
	CreatedAt           time.Time `json:"created_at" db:"created_at"`
	UpdatedAt           time.Time `json:"updated_at" db:"updated_at"`

	// Computed fields (not stored directly)
	Actions    []SemanticAction    `json:"actions,omitempty"`
	Parameters []WorkflowParameter `json:"params,omitempty"`
}

// WorkflowParameter represents a variable or fixed token in the workflow
type WorkflowParameter struct {
	Name         string        `json:"name"`
	Type         ParameterType `json:"type"`
	DefaultValue string        `json:"default_value,omitempty"`
	Description  string        `json:"description,omitempty"`
	Required     bool          `json:"required"`
	TokenType    TokenType     `json:"token_type"`              // Variable or Fixed
	SourceAction int           `json:"source_action,omitempty"` // Which action this came from
}

// ParameterType represents the data type of a parameter
type ParameterType string

const (
	ParamTypeString  ParameterType = "string"
	ParamTypeNumber  ParameterType = "number"
	ParamTypeBoolean ParameterType = "boolean"
	ParamTypeEmail   ParameterType = "email"
	ParamTypeURL     ParameterType = "url"
)

// TokenType represents whether a token is variable or fixed
type TokenType string

const (
	TokenVariable TokenType = "variable"
	TokenFixed    TokenType = "fixed"
)

// ==================== Workflow Run Types ====================

// WorkflowRun represents a single execution of a workflow
type WorkflowRun struct {
	ID                 string     `json:"id" db:"id"`
	WorkflowID         string     `json:"workflow_id" db:"workflow_id"`
	TemporalRunID      string     `json:"temporal_run_id" db:"temporal_run_id"`
	TemporalWorkflowID string     `json:"temporal_workflow_id" db:"temporal_workflow_id"`
	Status             RunStatus  `json:"status" db:"status"`
	ParametersJSON     string     `json:"parameters" db:"parameters"` // JSON string
	StartedAt          *time.Time `json:"started_at" db:"started_at"`
	CompletedAt        *time.Time `json:"completed_at" db:"completed_at"`
	ErrorMessage       string     `json:"error_message,omitempty" db:"error_message"`

	// Computed fields
	Parameters    map[string]string `json:"params,omitempty"`
	ActionResults []ActionResult    `json:"action_results,omitempty"`
}

// RunStatus represents the status of a workflow run
type RunStatus string

const (
	StatusPending  RunStatus = "pending"
	StatusRunning  RunStatus = "running"
	StatusSuccess  RunStatus = "success"
	StatusFailed   RunStatus = "failed"
	StatusCanceled RunStatus = "canceled"
)

// ActionResult represents the result of executing a single action
type ActionResult struct {
	ID             string     `json:"id" db:"id"`
	RunID          string     `json:"run_id" db:"run_id"`
	ActionID       string     `json:"action_id" db:"action_id"`
	SequenceID     int        `json:"sequence_id" db:"sequence_id"`
	Status         RunStatus  `json:"status" db:"status"`
	RetryCount     int        `json:"retry_count" db:"retry_count"`
	ScreenshotPath string     `json:"screenshot_path,omitempty" db:"screenshot_path"`
	GeneratedCode  string     `json:"generated_code,omitempty" db:"generated_code"`
	ErrorMessage   string     `json:"error_message,omitempty" db:"error_message"`
	ExecutedAt     *time.Time `json:"executed_at" db:"executed_at"`
	Duration       int64      `json:"duration_ms,omitempty" db:"duration_ms"`
}

// ==================== API Request/Response Types ====================

// WorkflowInput represents input for executing a workflow
type WorkflowInput struct {
	WorkflowID    string            `json:"workflow_id"`
	RunID         string            `json:"run_id"`
	Parameters    map[string]string `json:"parameters"`
	Actions       []SemanticAction  `json:"actions"`
	LLMProvider   string            `json:"llm_provider"`
	Headless      bool              `json:"headless"`
	Timeout       int               `json:"timeout_seconds"`
	RetryAttempts int               `json:"retry_attempts"`
}

// WorkflowResult represents the result of a workflow execution
type WorkflowResult struct {
	RunID         string         `json:"run_id"`
	Status        RunStatus      `json:"status"`
	ActionResults []ActionResult `json:"action_results"`
	TotalDuration int64          `json:"total_duration_ms"`
	ErrorMessage  string         `json:"error_message,omitempty"`
}

// ExecuteRequest represents a request to execute a workflow
type ExecuteRequest struct {
	WorkflowID  string            `json:"workflow_id"`
	Parameters  map[string]string `json:"parameters"`
	Parallelism int               `json:"parallelism"`
	LLMProvider string            `json:"llm_provider"`
	Headless    bool              `json:"headless"`
}

// ==================== WebSocket Message Types ====================

// WSMessage represents a WebSocket message for real-time updates
type WSMessage struct {
	Type    string      `json:"type"`
	Payload interface{} `json:"payload"`
}

// ActionStatusUpdate represents a status update for a single action
type ActionStatusUpdate struct {
	RunID      string    `json:"run_id"`
	ActionID   string    `json:"action_id"`
	SequenceID int       `json:"sequence_id"`
	Status     RunStatus `json:"status"`
	Message    string    `json:"message,omitempty"`
}
