package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"unicode"
)

// ==================== 1. Core Structures ====================

// SemanticEvent: The clean, compressed representation of an action
type SemanticEvent struct {
	Seq     int            `json:"seq"`
	Type    string         `json:"type"`
	Target  SemanticNode   `json:"target"`
	Rank    string         `json:"rank"` // High/Medium/Low
	Value   string         `json:"val,omitempty"`
	Context []SemanticNode `json:"ctx,omitempty"`
}

type SemanticNode struct {
	Tag        string                 `json:"tag"`
	Text       string                 `json:"text,omitempty"`
	Selector   string                 `json:"sel,omitempty"`
	Attributes map[string]interface{} `json:"attr,omitempty"`
}

// ==================== 2. RRWeb & Registry Logic ====================

const (
	EventFullSnapshot = 2
	EventIncremental  = 3
	EventMeta         = 4
	SourceMutation    = 0
	SourceMouse       = 2
	SourceInput       = 5
	Click             = 2
)

type RRWebEvent struct {
	Type int             `json:"type"`
	Data json.RawMessage `json:"data"`
}

type MetaData struct {
	Href string `json:"href"`
}
type FullSnapshotData struct {
	Node *SerializedNode `json:"node"`
}
type IncrementalSnapshotData struct {
	Source int            `json:"source"`
	Type   int            `json:"type"`
	ID     int            `json:"id"`
	Text   string         `json:"text"`
	Adds   []NodeAddition `json:"adds"`
}
type NodeAddition struct {
	ParentID int             `json:"parentId"`
	Node     *SerializedNode `json:"node"`
}
type SerializedNode struct {
	ID          int                    `json:"id"`
	TagName     string                 `json:"tagName"`
	Attributes  map[string]interface{} `json:"attributes"`
	ChildNodes  []*SerializedNode      `json:"childNodes"`
	TextContent string                 `json:"textContent,omitempty"`
}

type NodeRegistry struct {
	nodes   map[int]*SerializedNode
	parents map[int]int
}

func NewNodeRegistry() *NodeRegistry {
	return &NodeRegistry{nodes: make(map[int]*SerializedNode), parents: make(map[int]int)}
}

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

func (nr *NodeRegistry) IsInteractable(id int) bool {
	return nr.CalculateInteractionRank(id) != "Low"
}

func (nr *NodeRegistry) CalculateInteractionRank(id int) string {
	node, ok := nr.nodes[id]
	if !ok {
		return "Low"
	}
	tag := strings.ToLower(node.TagName)
	if tag == "button" || tag == "a" || tag == "select" || tag == "textarea" || tag == "input" {
		return "High"
	}
	if role, ok := node.Attributes["role"].(string); ok {
		if role == "button" || role == "link" || role == "menuitem" || role == "checkbox" || role == "combobox" {
			return "Medium"
		}
	}
	if style, ok := node.Attributes["style"].(string); ok {
		if strings.Contains(style, "cursor: pointer") {
			return "Medium"
		}
	}
	return "Low"
}

func (nr *NodeRegistry) GetClickableTarget(id int) int {
	currID := id
	for i := 0; i < 5; i++ {
		if nr.CalculateInteractionRank(currID) != "Low" {
			return currID
		}
		parentID, ok := nr.parents[currID]
		if !ok || parentID == 0 {
			break
		}
		currID = parentID
	}
	return id
}

func (nr *NodeRegistry) GetRobustSelector(id int) string {
	node, ok := nr.nodes[id]
	if !ok {
		return ""
	}
	// 1. ID (filter out dynamic looking IDs)
	if idVal, ok := node.Attributes["id"].(string); ok && !containsNumbersAndLetters(idVal) {
		return fmt.Sprintf("#%s", idVal)
	}
	// 2. Class (filter out dynamic looking classes)
	if classVal, ok := node.Attributes["class"].(string); ok {
		classes := strings.Split(classVal, " ")
		for _, c := range classes {
			c = strings.TrimSpace(c)
			if c != "" && !containsNumbersAndLetters(c) {
				return "." + c
			}
		}
	}
	return node.TagName
}

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

// ==================== 3. Semantic Builder ====================

type SemanticBuilder struct {
	registry       *NodeRegistry
	events         []SemanticEvent
	mutationBuffer []SemanticNode
	eventCounter   int
}

func NewSemanticBuilder() *SemanticBuilder {
	return &SemanticBuilder{registry: NewNodeRegistry(), events: []SemanticEvent{}}
}

func (b *SemanticBuilder) Process(events []RRWebEvent) {
	for _, e := range events {
		if e.Type == EventMeta {
			var meta MetaData
			if json.Unmarshal(e.Data, &meta) == nil && meta.Href != "" {
				b.eventCounter++
				b.events = append(b.events, SemanticEvent{Seq: b.eventCounter, Type: "navigate", Value: meta.Href, Rank: "High", Target: SemanticNode{Selector: "window"}})
			}
			continue
		}
		if e.Type == EventFullSnapshot {
			var fs FullSnapshotData
			if json.Unmarshal(e.Data, &fs) == nil {
				b.registry.Register(fs.Node, 0)
			}
			continue
		}
		if e.Type == EventIncremental {
			var data IncrementalSnapshotData
			json.Unmarshal(e.Data, &data)
			if data.Source == SourceMutation {
				for _, add := range data.Adds {
					b.registry.Register(add.Node, add.ParentID)
					if b.registry.IsInteractable(add.Node.ID) {
						b.mutationBuffer = append(b.mutationBuffer, b.toSemantic(add.Node.ID))
					}
				}
				continue
			}
			if (data.Source == SourceMouse && data.Type == Click) || data.Source == SourceInput {
				b.eventCounter++
				aType := "click"
				if data.Source == SourceInput {
					aType = "input"
				}
				targetID := b.registry.GetClickableTarget(data.ID)
				rank := b.registry.CalculateInteractionRank(targetID)

				if aType == "click" && rank == "Low" && len(b.mutationBuffer) == 0 {
					b.mutationBuffer = []SemanticNode{}
					continue
				}

				evt := SemanticEvent{
					Seq: b.eventCounter, Type: aType, Target: b.toSemantic(targetID),
					Rank: rank, Value: data.Text, Context: b.mutationBuffer,
				}
				b.events = append(b.events, evt)
				b.mutationBuffer = []SemanticNode{}
			}
		}
	}
}

func (b *SemanticBuilder) toSemantic(id int) SemanticNode {
	node, ok := b.registry.nodes[id]
	if !ok {
		return SemanticNode{Tag: "unknown", Selector: "unknown"}
	}
	text := node.TextContent
	if text == "" {
		if t, ok := node.Attributes["aria-label"].(string); ok {
			text = t
		}
	}
	return SemanticNode{
		Tag: node.TagName, Text: text, Selector: b.registry.GetRobustSelector(id), Attributes: node.Attributes,
	}
}

func (b *SemanticBuilder) PostProcess() {
	var compressed []SemanticEvent
	validAttrs := map[string]bool{"id": true, "name": true, "class": true, "role": true, "placeholder": true}

	for i, evt := range b.events {
		// 1. Clean Attributes
		cleanedAttrs := make(map[string]interface{})
		for k, v := range evt.Target.Attributes {
			if validAttrs[k] {
				cleanedAttrs[k] = v
			}
		}
		evt.Target.Attributes = cleanedAttrs
		evt.Context = nil // Aggressive Context Cleaning

		// 2. Debounce Inputs
		if i > 0 {
			prev := &compressed[len(compressed)-1]
			if prev.Type == "input" && evt.Type == "input" && prev.Target.Selector == evt.Target.Selector {
				prev.Value = evt.Value
				continue
			}
		}

		// 3. Enter Key Fix
		if evt.Type == "input" && evt.Value == "en" && evt.Target.Tag == "unknown" {
			evt.Type = "keypress"
			evt.Value = "Enter"
			if len(compressed) > 0 {
				evt.Target.Selector = compressed[len(compressed)-1].Target.Selector
			}
		}

		compressed = append(compressed, evt)
	}
	b.events = compressed
}

// ==================== 4. Prompt Generation ====================

func GeneratePrompt(semanticContext []SemanticEvent) {
	ctxBytes, _ := json.MarshalIndent(semanticContext, "", "  ")

	prompt := fmt.Sprintf(`
You are an expert Golang automation engineer using Go Rod. 
Your goal is to convert the raw recorded session below into a reusable, production-ready Go function.

SEMANTIC CONTEXT:
%s

---

### SUB-TASK 1: SMART SELECTOR GENERATION (CRITICAL)
The 'selector' field in the JSON is often brittle (e.g., dynamic IDs or obfuscated class names).
**You must generate robust selectors by analyzing the 'attr' (attributes) map.**

**Priority Order for Selectors:**
1. **Accessibility Attributes (Best)**: If you see 'aria-label', 'aria-placeholder', or 'role', use them.
   - *JSON*: {"tag": "input", "attr": {"aria-label": "Search Reddit"}}
   - *Code*: page.MustElement("input[aria-label='Search Reddit']")
2. **Visual Attributes (Good)**: If you see 'placeholder' or 'title', use them.
   - *JSON*: {"tag": "input", "attr": {"placeholder": "Search..."}}
   - *Code*: page.MustElement("input[placeholder='Search...']")
3. **Text Matching (For Non-Inputs)**: If the element is a Button or Link, ignore the CSS selector and match by text.
   - *JSON*: {"tag": "button", "text": "Log In"}
   - *Code*: page.MustElementR("button", "Log In")
4. **Fallback**: Only use ID or Name if no human-readable attributes exist.

### SUB-TASK2: SEMANTIC VARIABLE EXTRACTION & GENERALIZATION
You must transform this rigid recording into a flexible automation flow.

**Phase 1: Token Analysis (Fixed vs. Variable)**
1. Analyze user inputs in the JSON. Break them into "Fixed Tokens" (structure) and "Variable Tokens" (user intent).
2. **Bucketing:** For Variable Tokens, assign a generic CamelCase name (e.g., 'sushi' -> 'cuisineType', 'harry potter' -> 'searchTopic').
3. **Signature:** Create a function signature accepting these variables: 'func RunFlow(page *rod.Page, searchTopic string)'.

**Phase 2: Contextual Generalization (The "Search vs. Direct Link" Rule)**
Crucial: You must identify where these variable tokens appear LATER in the session (in URLs, selectors, or text matches) and generalize the action.

1. **Deep Link Sanitization:** - If the recording shows a navigation to a URL containing a variable token (e.g., 'reddit.com/r/HarryPotter'), **DO NOT** hardcode this URL.
   - **Reasoning:** If the user changes 'Harry Potter' to 'Lord of the Rings', the 'HarryPotter' URL is invalid.
   - **Solution:** Navigate to the **generic base URL** (e.g., 'reddit.com') and deduce the next steps (using search bars or menus) based on the user's previous intent.

2. **Cross-Step Replacement:**
   - If a selector relies on text that matches a variable (e.g., clicking a link with text "History of Harry Potter"), replace the hardcoded string with the variable.
   - Code: 'page.MustElementR("a", searchTopic).MustClick()'

### CRITICAL RULES FOR ROBUSTNESS:
1. **Handle Navigation**: 'action_type: navigate' -> 'page.MustNavigate("url").MustWaitLoad()'.
2. **Wait for Visibility**: Always chain '.MustWaitVisible()' before clicking or typing.
3. **Wait for Stability**: Use '.MustWaitStable()' for animations.
4. **Smart Selectors**: If a selector looks unstable, use 'page.MustElementR(...)' to match by text.

Output ONLY the Go code.
`, string(ctxBytes))

	err := os.WriteFile("prompt.txt", []byte(prompt), 0644)
	if err != nil {
		fmt.Printf("❌ Error: %v\n", err)
	} else {
		fmt.Println("💾 Prompt saved to 'prompt.txt'")
	}
}

func main() {
	// 1. Load Events
	file, err := os.ReadFile("events.json")
	if err != nil {
		log.Fatalf("Error reading events.json: %v", err)
	}
	var rawEvents []RRWebEvent
	if err := json.Unmarshal(file, &rawEvents); err != nil {
		log.Fatalf("Error parsing JSON: %v", err)
	}

	// 2. Build Semantic Graph
	builder := NewSemanticBuilder()
	builder.Process(rawEvents)

	// 3. Compress & Clean
	builder.PostProcess()

	// 4. Generate LLM Prompt
	GeneratePrompt(builder.events)
}
