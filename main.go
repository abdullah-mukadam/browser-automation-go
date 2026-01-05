package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"unicode"
)

// ==================== 0. Constants & RRWeb Types ====================

const (
	EventFullSnapshot = 2
	EventIncremental  = 3

	SourceMutation         = 0
	SourceMouseInteraction = 2
	SourceInput            = 5

	// Incremental Types
	MutationAdd = 0 // implied for adds in this simplified struct
	Click       = 2
	Input       = 0 // Input event type in incremental data usually 0 or similar depending on version
)

type RRWebEvent struct {
	Type      int             `json:"type"`
	Data      json.RawMessage `json:"data"`
	Timestamp int64           `json:"timestamp"`
}

type FullSnapshotData struct {
	Node *SerializedNode `json:"node"`
}

type IncrementalSnapshotData struct {
	Source int            `json:"source"`
	Type   int            `json:"type"` // Interaction type (click vs others)
	ID     int            `json:"id"`   // Target ID for interaction
	Text   string         `json:"text"` // For input events
	Adds   []NodeAddition `json:"adds"` // For mutations
}

type NodeAddition struct {
	ParentID int             `json:"parentId"`
	Node     *SerializedNode `json:"node"`
}

// SerializedNode represents the raw node from RRWeb
type SerializedNode struct {
	ID          int                    `json:"id"`
	Type        int                    `json:"type"`
	TagName     string                 `json:"tagName"`
	Attributes  map[string]interface{} `json:"attributes"`
	ChildNodes  []*SerializedNode      `json:"childNodes"`
	TextContent string                 `json:"textContent,omitempty"` // simplified
}

// ==================== 1. New Semantic Structures ====================

type SemanticNode struct {
	Tag        string                 `json:"tag"`
	Text       string                 `json:"text,omitempty"`
	Selector   string                 `json:"selector"`
	Attributes map[string]interface{} `json:"attributes,omitempty"`
}

type SemanticEvent struct {
	Seq     int            `json:"sequence_id"`
	Type    string         `json:"action_type"`
	Target  SemanticNode   `json:"target"`
	Value   string         `json:"input_value,omitempty"`
	Rank    string         `json:"interaction_rank"`
	Context []SemanticNode `json:"new_elements_appeared"`
}

// ==================== 2. Enhanced Registry & Logic ====================

type NodeRegistry struct {
	nodes   map[int]*SerializedNode
	parents map[int]int
}

func NewNodeRegistry() *NodeRegistry {
	return &NodeRegistry{
		nodes:   make(map[int]*SerializedNode),
		parents: make(map[int]int),
	}
}

// Register adds a node and recursively registers children
func (nr *NodeRegistry) Register(n *SerializedNode, parentID int) {
	if n == nil {
		return
	}
	nr.nodes[n.ID] = n
	if parentID != 0 {
		nr.parents[n.ID] = parentID
	}
	for _, child := range n.ChildNodes {
		nr.Register(child, n.ID)
	}
}

// containsNumbersAndLetters identifies dynamic/hash classes (e.g., "k1zIA")
func containsNumbersAndLetters(s string) bool {
	var hasL, hasN bool
	for _, r := range s {
		if unicode.IsLetter(r) {
			hasL = true
		}
		if unicode.IsNumber(r) {
			hasN = true
		}
	}
	return hasL && hasN
}

func getStrAttr(attrs map[string]interface{}, key string) (string, bool) {
	if val, ok := attrs[key]; ok {
		if s, ok := val.(string); ok {
			return s, true
		}
	}
	return "", false
}

func (nr *NodeRegistry) IsInteractable(id int) bool {
	node, ok := nr.nodes[id]
	if !ok {
		return false
	}
	tag := strings.ToLower(node.TagName)

	// Direct interactable tags
	if tag == "button" || tag == "a" || tag == "input" || tag == "select" || tag == "textarea" {
		return true
	}
	// ARIA roles
	if role, ok := getStrAttr(node.Attributes, "role"); ok {
		if role == "button" || role == "link" || role == "checkbox" || role == "menuitem" {
			return true
		}
	}
	return false
}

// GetClickableTarget bubbles up from a specific node (like a generic span)
// to find the nearest parent that is actually interactable (like a button).
func (nr *NodeRegistry) GetClickableTarget(id int) int {
	currID := id
	// Traverse up max 5 levels to avoid infinite loops or going too high
	for i := 0; i < 5; i++ {
		if nr.IsInteractable(currID) {
			return currID
		}
		parentID, ok := nr.parents[currID]
		if !ok || parentID == 0 {
			break
		}
		currID = parentID
	}
	// If no interactable parent found, return original ID (best effort)
	return id
}

// GetRobustSelector generates a Go Rod-friendly selector, preferring IDs,
// then stable classes, then falling back to tag structure.
func (nr *NodeRegistry) GetRobustSelector(id int) string {
	node, ok := nr.nodes[id]
	if !ok {
		return ""
	}

	// 1. Try ID (if it looks stable)
	if idVal, ok := getStrAttr(node.Attributes, "id"); ok {
		if !containsNumbersAndLetters(idVal) {
			return fmt.Sprintf("#%s", idVal)
		}
	}

	// 2. Try Classes (filtering out dynamic hashes)
	if classVal, ok := getStrAttr(node.Attributes, "class"); ok {
		classes := strings.Split(classVal, " ")
		var stableClasses []string
		for _, c := range classes {
			c = strings.TrimSpace(c)
			if c != "" && !containsNumbersAndLetters(c) {
				stableClasses = append(stableClasses, "."+c)
			}
		}
		if len(stableClasses) > 0 {
			// Join stable classes. Using just the first distinct one is often safer for Rod
			// unless specificity is needed, but let's join all stable ones for now.
			return strings.Join(stableClasses, "")
		}
	}

	// 3. Fallback: Tag + Attribute Text (simplified XPath-like logic for LLM)
	// For the LLM context, sometimes just "button[text='Submit']" is better than a CSS selector.
	// We will return a basic Tag representation here.
	return node.TagName
}

// CalculateInteractionRank determines how "standard" an interaction is.
// High: Native interactive elements (buttons, links, inputs).
// Medium: Generic elements masking as interactive (ARIA roles, "btn" classes).
// Low: Generic containers (divs, spans) with no semantic hints.
func (nr *NodeRegistry) CalculateInteractionRank(id int) string {
	node, ok := nr.nodes[id]
	if !ok {
		return "Low"
	}

	tag := strings.ToLower(node.TagName)
	attrs := node.Attributes

	// 1. High Confidence: Native Interactive Tags
	if tag == "button" || tag == "a" || tag == "select" || tag == "textarea" || tag == "input" {
		return "High"
	}

	// 2. Medium Confidence: ARIA Roles & Semantic Classes
	if role, ok := getStrAttr(attrs, "role"); ok {
		if role == "button" || role == "link" || role == "menuitem" || role == "checkbox" {
			return "Medium"
		}
	}

	// Check for "cursor: pointer" style (common indicator of clickability)
	if style, ok := getStrAttr(attrs, "style"); ok {
		if strings.Contains(style, "cursor: pointer") {
			return "Medium"
		}
	}

	// Check for "btn" or "button" in class names (heuristic)
	if class, ok := getStrAttr(attrs, "class"); ok {
		classLower := strings.ToLower(class)
		if strings.Contains(classLower, "btn") || strings.Contains(classLower, "button") || strings.Contains(classLower, "clickable") {
			return "Medium"
		}
	}

	// 3. Low Confidence: Everything else (div, span, img)
	return "Low"
}

// ==================== 3. The Semantic Graph Builder ====================

type SemanticBuilder struct {
	registry       *NodeRegistry
	events         []SemanticEvent
	mutationBuffer []SemanticNode
	eventCounter   int
}

func NewSemanticBuilder() *SemanticBuilder {
	return &SemanticBuilder{
		registry: NewNodeRegistry(),
		events:   []SemanticEvent{},
	}
}

func (b *SemanticBuilder) Process(events []RRWebEvent) {
	for _, e := range events {
		// 1. Handle Full Snapshots (Initial Page Load)
		if e.Type == EventFullSnapshot {
			var fs FullSnapshotData
			if err := json.Unmarshal(e.Data, &fs); err == nil {
				b.registry.Register(fs.Node, 0)
			}
			continue
		}

		// 2. Handle Incremental Events (Mutations & Interactions)
		if e.Type == EventIncremental {
			var data IncrementalSnapshotData
			if err := json.Unmarshal(e.Data, &data); err != nil {
				continue
			}

			// Capture Mutations (Context)
			if data.Source == SourceMutation {
				for _, add := range data.Adds {
					if add.Node != nil {
						b.registry.Register(add.Node, add.ParentID)
						// Only buffer "meaningful" new elements for the LLM
						if b.registry.IsInteractable(add.Node.ID) {
							b.mutationBuffer = append(b.mutationBuffer, b.toSemantic(add.Node.ID))
						}
					}
				}
				continue
			}

			// Capture User Actions (Click or Input)
			// Note: RRWeb Click type is usually 2, Input source is 5.
			// Logic simplified here for demonstration.
			isClick := data.Source == SourceMouseInteraction && data.Type == Click
			isInput := data.Source == SourceInput

			if isClick || isInput {
				b.eventCounter++

				actionType := "click"
				if isInput {
					actionType = "input"
				}

				// Bubble up to find the real clickable target (e.g., span -> button)
				targetID := b.registry.GetClickableTarget(data.ID)

				// Calculate Rank
				rank := b.registry.CalculateInteractionRank(targetID)

				evt := SemanticEvent{
					Seq:     b.eventCounter,
					Type:    actionType,
					Target:  b.toSemantic(targetID),
					Value:   data.Text,
					Rank:    rank,
					Context: b.mutationBuffer,
				}

				// 2. For Clicks, check for "Noise"
				isLowConfidence := evt.Rank == "Low"
				hasNoVisibleEffect := len(evt.Context) == 0

				// If it's a generic div click that seemingly added nothing, skip it.
				if isLowConfidence && hasNoVisibleEffect {
					fmt.Printf("⚠️ Skipping Noise: Sequence %d (Low rank generic click)\n", evt.Seq)
					b.mutationBuffer = []SemanticNode{} // Clear buffer
					continue
				}

				b.events = append(b.events, evt)
				b.mutationBuffer = []SemanticNode{} // Clear buffer after associating with an action
			}
		}
	}
}

func (b *SemanticBuilder) toSemantic(id int) SemanticNode {
	node, ok := b.registry.nodes[id]
	if !ok {
		return SemanticNode{Tag: "unknown", Selector: "unknown"}
	}

	// Helper to get text content (simplified)
	text := node.TextContent
	// If empty, check generic text attribute or label
	if text == "" {
		if t, ok := getStrAttr(node.Attributes, "placeholder"); ok {
			text = t
		}
		if t, ok := getStrAttr(node.Attributes, "aria-label"); ok {
			text = t
		}
	}

	return SemanticNode{
		Tag:        node.TagName,
		Text:       text,
		Selector:   b.registry.GetRobustSelector(id),
		Attributes: node.Attributes,
	}
}

// CompressEvents reduces token usage by merging sequential inputs and cleaning attributes
func (b *SemanticBuilder) CompressEvents() {
	var compressed []SemanticEvent

	// Whitelist of attributes useful for automation selectors
	validAttrs := map[string]bool{
		"id": true, "name": true, "class": true, "role": true,
		"type": true, "placeholder": true, "href": true,
	}

	for i, evt := range b.events {
		// 1. Clean Attributes
		cleanedAttrs := make(map[string]interface{})
		for k, v := range evt.Target.Attributes {
			if validAttrs[k] {
				cleanedAttrs[k] = v
			}
		}
		evt.Target.Attributes = cleanedAttrs
		evt.Context = nil // Aggressively remove mutation context if you want extreme savings

		// 2. Input Debouncing Logic
		if i > 0 {
			prev := &compressed[len(compressed)-1]

			// If current and previous are both INPUTs on the SAME target
			if prev.Type == "input" && evt.Type == "input" && prev.Target.Selector == evt.Target.Selector {
				// Just update the value of the previous event to the new (longer) string
				// This merges "b", "be", "bes"... into just "best..."
				prev.Value = evt.Value
				continue
			}
		}

		// 3. Detect "Enter" key masking as unknown input
		if evt.Type == "input" && evt.Value == "en" && evt.Target.Tag == "unknown" {
			evt.Type = "keypress"
			evt.Value = "Enter"
			// Inherit selector from previous event if target is lost
			if len(compressed) > 0 {
				evt.Target.Selector = compressed[len(compressed)-1].Target.Selector
			}
		}

		compressed = append(compressed, evt)
	}

	b.events = compressed
}

// ==================== 4. LLM Integration ====================

func CallLLMGenerator(semanticContext []SemanticEvent) {
	// Convert context to JSON string
	ctxBytes, _ := json.MarshalIndent(semanticContext, "", "  ")
	ctxString := string(ctxBytes)

	// Construct the prompt
	prompt := fmt.Sprintf(`
You are an expert Golang automation engineer using Go Rod. 
Generate a robust, production-ready script based on the user's semantic session history below.

SEMANTIC CONTEXT:
%s

CRITICAL RULES FOR ROBUSTNESS:
1. Handle Navigation: When you see 'action_type: navigate', use 'page = browser.MustPage("url")' or 'page.MustNavigate("url").MustWaitLoad()'.
2. Wait for Visibility: Never click blindly. Always chain '.MustWaitVisible()' before interacting. 
   - BAD: page.MustElement("#submit").MustClick()
   - GOOD: page.MustElement("#submit").MustWaitVisible().MustClick()
3. Wait for Stability: If an element might be animating (like a dropdown), use '.MustWaitStable()'.
4. Smart Selectors: If the selector looks complex (e.g. div:nth-child...), prefer searching by text using 'page.MustElementR("tag", "text content")'.

Output ONLY the Go code.
`, ctxString)

	// 👇 WRITE TO FILE
	err := os.WriteFile("prompt.txt", []byte(prompt), 0644)
	if err != nil {
		fmt.Printf("❌ Error writing prompt to file: %v\n", err)
	} else {
		fmt.Println("💾 Prompt successfully saved to 'prompt.txt'")
	}

	// Mock Console Output
	fmt.Println("================ LLM PROMPT READY ================")
	fmt.Println("(Check prompt.txt for the full content)")
}

// ==================== 5. Main Execution ====================

func main() {
	// Ensure you have a valid events.json in the same folder
	file, err := os.ReadFile("events.json")
	if err != nil {
		log.Fatalf("Error reading events.json: %v", err)
	}

	var rawEvents []RRWebEvent
	if err := json.Unmarshal(file, &rawEvents); err != nil {
		log.Fatalf("Error parsing JSON: %v", err)
	}

	builder := NewSemanticBuilder()
	builder.Process(rawEvents)
	builder.CompressEvents()

	// Export the Semantic JSON for inspection
	output, _ := json.MarshalIndent(builder.events, "", "  ")
	_ = os.WriteFile("semantic_context.json", output, 0644)

	fmt.Println("🚀 Semantic Context Generated!")
	fmt.Printf("📦 Encapsulated %d user actions.\n", len(builder.events))

	// Call the placeholder LLM
	CallLLMGenerator(builder.events)
}
