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

// VariableToken: Optimized for minimal token usage in JSON
// t = token text, b = bucket (if variable), c = confidence
type VariableToken struct {
	Text       string  `json:"t"`
	Bucket     string  `json:"b,omitempty"` // Empty if fixed text
	Confidence float64 `json:"c,omitempty"` // 0.0-1.0
}

type InputAnalysis struct {
	Tokens []VariableToken `json:"tokens"`
}

type SemanticEvent struct {
	Seq      int            `json:"seq"`
	Type     string         `json:"type"`
	Target   SemanticNode   `json:"target"`
	Rank     string         `json:"rank"` // High/Medium/Low
	Value    string         `json:"val,omitempty"`
	Analysis *InputAnalysis `json:"nlp,omitempty"` // <--- The compacted analysis layer
	Context  []SemanticNode `json:"ctx,omitempty"`
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
	// 1. ID
	if idVal, ok := node.Attributes["id"].(string); ok && !containsNumbersAndLetters(idVal) {
		return fmt.Sprintf("#%s", idVal)
	}
	// 2. Class
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

// ==================== 3. Semantic Logic & Analysis ====================

var stopWords = map[string]bool{
	"the": true, "of": true, "in": true, "at": true, "to": true,
	"for": true, "with": true, "best": true, "near": true, "me": true,
}

// AnalyzeInputSemantics performs token classification with Mock NLP
func AnalyzeInputSemantics(inputText string) InputAnalysis {
	words := strings.Fields(inputText)
	var tokens []VariableToken

	for _, word := range words {
		cleanWord := strings.ToLower(strings.Trim(word, ".,?!"))
		token := VariableToken{Text: word} // Default: just text

		// --- MOCK NLP LOGIC ---
		if !stopWords[cleanWord] {
			if cleanWord == "sushi" || cleanWord == "burger" || cleanWord == "pizza" {
				token.Bucket = "cuisineType"
				token.Confidence = 0.9
			} else if cleanWord == "nyc" || cleanWord == "london" || cleanWord == "paris" {
				token.Bucket = "targetCity"
				token.Confidence = 0.95
			} else if cleanWord != "restaurants" && cleanWord != "places" {
				// Unknown nouns
				token.Bucket = "searchQuery"
				token.Confidence = 0.5
			}
		}
		tokens = append(tokens, token)
	}
	return InputAnalysis{Tokens: tokens}
}

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
		evt.Context = nil // Aggressively clear context for token savings

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

		// 4. Run Variable Extraction (Only on final debounced inputs)
		if evt.Type == "input" && len(evt.Value) > 3 {
			analysis := AnalyzeInputSemantics(evt.Value)
			evt.Analysis = &analysis
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
Generate a robust, production-ready script based on the semantic session history.

SEMANTIC CONTEXT:
%s

---

### SUB-TASK: VARIABLE ABSTRACTION
I have performed a semantic analysis on the user inputs (see the 'nlp' field in the JSON).
1. Look at 'nlp.tokens'.
2. If a token has a 'b' (bucket) field, it is a VARIABLE.
3. DO NOT hardcode these values. Create a Go function signature accepting these variables.
   - Example JSON: {"t": "sushi", "b": "cuisineType"}
   - Result: func Search(cuisineType string) { ... }

### CRITICAL RULES FOR ROBUSTNESS:
1. **Handle Navigation**: 'action_type: navigate' -> 'page.MustNavigate("url").MustWaitLoad()'.
2. **Wait for Visibility**: Always chain '.MustWaitVisible()' before clicking or typing.
3. **Wait for Stability**: Use '.MustWaitStable()' for animations.
4. **Debouncing**: The JSON inputs are already debounced. Use the final value provided.

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

	// 3. Compress, Debounce, & Extract Variables
	builder.PostProcess()

	// 4. Generate LLM Prompt
	GeneratePrompt(builder.events)
}
