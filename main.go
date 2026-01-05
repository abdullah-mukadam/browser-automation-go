package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"sort"
	"strings"
)

// ==================== 1. RRWeb & Data Structures ====================

const (
	EventFullSnapshot        = 2
	EventIncrementalSnapshot = 3
	EventMeta                = 4
)

const (
	SourceMutation         = 0
	SourceMouseInteraction = 2
	SourceInput            = 5
)

const (
	Click = 2
)

type RRWebEvent struct {
	Type      int             `json:"type"`
	Data      json.RawMessage `json:"data"`
	Timestamp int64           `json:"timestamp"`
}

type Node struct {
	ID          int                    `json:"id"`
	Type        int                    `json:"type"`
	TagName     string                 `json:"tagName"`
	Attributes  map[string]interface{} `json:"attributes"`
	ChildNodes  []Node                 `json:"childNodes"`
	TextContent string                 `json:"textContent"`
}

type FullSnapshotData struct {
	Node Node `json:"node"`
}

type IncrementalSnapshotData struct {
	Source     int                 `json:"source"`
	Type       int                 `json:"type"` // Interaction Type (Click, etc.)
	ID         int                 `json:"id"`
	Text       string              `json:"text"`      // Input text
	IsChecked  bool                `json:"isChecked"` // Checkbox state
	Adds       []AddedNode         `json:"adds"`
	Removes    []RemovedNode       `json:"removes"`
	Attributes []AttributeMutation `json:"attributes"`
}

type AddedNode struct {
	ParentID int  `json:"parentId"`
	Node     Node `json:"node"`
}

type RemovedNode struct {
	ParentID int `json:"parentId"`
	ID       int `json:"id"`
}

type AttributeMutation struct {
	ID         int                    `json:"id"`
	Attributes map[string]interface{} `json:"attributes"`
}

type MetaData struct {
	Href string `json:"href"`
}

// ==================== 2. Job Graph Structures ====================

type JobType string

const (
	JobTypeNavigation  JobType = "navigation"
	JobTypeInteraction JobType = "interaction" // Clicks
	JobTypeInput       JobType = "input"       // Typing
	JobTypeWait        JobType = "wait"        // Waiting for DOM
)

type Job struct {
	ID           string
	Type         JobType
	Selector     string
	Action       string
	IsFixed      bool
	FixedValue   string
	VariableKey  string
	Dependencies []string // IDs of jobs this depends on
	Timestamp    int64
}

type JobGraph struct {
	Jobs      map[string]*Job
	Variables map[string]string // Key -> Description
	Layers    [][]string        // Execution order
}

// ==================== 3. The "Perfect" Selector Engine ====================

// NodeInfo stores the state of the DOM tree at any point in time
type NodeInfo struct {
	ID         int
	TagName    string
	Attributes map[string]interface{}
	ParentID   int
	Children   []int // Ordered list of child IDs for nth-of-type calculation
}

type NodeRegistry struct {
	nodes map[int]*NodeInfo
}

func NewNodeRegistry() *NodeRegistry {
	return &NodeRegistry{
		nodes: make(map[int]*NodeInfo),
	}
}

// Register recursively adds nodes to the registry
func (nr *NodeRegistry) Register(node Node, parentID int) {
	info := &NodeInfo{
		ID:         node.ID,
		TagName:    strings.ToLower(node.TagName),
		Attributes: node.Attributes,
		ParentID:   parentID,
		Children:   make([]int, 0),
	}
	nr.nodes[node.ID] = info

	// Update parent's children list
	if parent, ok := nr.nodes[parentID]; ok {
		parent.Children = append(parent.Children, node.ID)
	}

	for _, child := range node.ChildNodes {
		nr.Register(child, node.ID)
	}
}

func (nr *NodeRegistry) Remove(id int) {
	node, ok := nr.nodes[id]
	if !ok {
		return
	}
	// Remove from parent's children list
	if parent, ok := nr.nodes[node.ParentID]; ok {
		newChildren := make([]int, 0)
		for _, childID := range parent.Children {
			if childID != id {
				newChildren = append(newChildren, childID)
			}
		}
		parent.Children = newChildren
	}
	delete(nr.nodes, id)
}

//func isGeneratedClass(className string) bool {
//	// Logic: If the class is exactly 5-8 chars and alphanumeric, it's likely dynamic
//	// e.g., "k1zIA" matches this.
//	parts := strings.Fields(className)
//	for _, p := range parts {
//		if len(p) < 10 && containsNumbersAndLetters(p) {
//			return true
//		}
//	}
//	return false
//}

// GetRobustSelector generates the most specific yet stable selector possible
func (nr *NodeRegistry) GetRobustSelector(id int) string {
	node, ok := nr.nodes[id]
	if !ok {
		return "body" // Fallback
	}

	// Strategy 1: Unique ID (Gold Standard)
	if val, ok := getStrAttr(node.Attributes, "id"); ok {
		// Ensure ID is valid CSS (doesn't start with numbers, etc - simplified check)
		if !strings.ContainsAny(val, " .:") {
			return fmt.Sprintf("#%s", val)
		}
	}

	// Strategy 2: Unique Name (Silver Standard for inputs)
	if val, ok := getStrAttr(node.Attributes, "name"); ok {
		return fmt.Sprintf("%s[name=%q]", node.TagName, val)
	}

	// Strategy 3: High-Value Attributes (Test IDs, Aria)
	targetAttrs := []string{"data-testid", "data-test-id", "aria-label", "placeholder", "role"}
	for _, attr := range targetAttrs {
		if val, ok := getStrAttr(node.Attributes, attr); ok {
			return fmt.Sprintf("%s[%s=%q]", node.TagName, attr, val)
		}
	}

	// Strategy 4: Specific Classes (Bronze Standard)
	// We avoid generic layout classes like "flex", "mt-4"
	if val, ok := getStrAttr(node.Attributes, "class"); ok {
		classes := strings.Fields(val)
		validClasses := make([]string, 0)
		for _, c := range classes {
			if !isUtilityClass(c) {
				validClasses = append(validClasses, "."+c)
			}
		}
		// If we found specific classes, try to use them
		if len(validClasses) > 0 && len(validClasses) <= 2 { // Don't chain too many
			return fmt.Sprintf("%s%s", node.TagName, strings.Join(validClasses, ""))
		}
	}

	return nr.GetStructuralPath(node.ParentID)

	//// Strategy 5: Structural Fallback (The "Nth-Child" Hammer)
	//// If all else fails, describe the path relative to the parent
	//if node.ParentID != 0 {
	//	parentSelector := nr.GetRobustSelector(node.ParentID)
	//
	//	// Calculate nth-of-type
	//	index := 1
	//	siblings := nr.nodes[node.ParentID].Children
	//	for _, siblingID := range siblings {
	//		if siblingID == id {
	//			break
	//		}
	//		sibling, exists := nr.nodes[siblingID]
	//		if exists && sibling.TagName == node.TagName {
	//			index++
	//		}
	//	}
	//
	//	return fmt.Sprintf("%s > %s:nth-of-type(%d)", parentSelector, node.TagName, index)
	//}

	return node.TagName
}

// GetStructuralPath builds a selector from the element up to the body.
// Example: html > body > div:nth-child(2) > div:nth-child(1) > button
func (nr *NodeRegistry) GetStructuralPath(id int) string {
	node, ok := nr.nodes[id]
	if !ok || node.TagName == "" || node.TagName == "html" {
		return "html"
	}

	if node.TagName == "body" {
		return "body"
	}

	// 1. Get Parent
	parent, exists := nr.nodes[node.ParentID]
	if !exists {
		return node.TagName
	}

	// 2. Calculate nth-child index
	// We count all siblings (regardless of tag name) to get the exact index
	index := 1
	for _, siblingID := range parent.Children {
		if siblingID == id {
			break
		}
		index++
	}

	// 3. Recurse up the tree
	parentPath := nr.GetStructuralPath(node.ParentID)

	// 4. Return the combined path
	// Only add :nth-child if there's a reason to (helps readability)
	return fmt.Sprintf("%s > %s:nth-child(%d)", parentPath, node.TagName, index)
}

func isUtilityClass(c string) bool {
	// Simple heuristic to ignore Tailwind/Bootstrap utility classes
	prefixes := []string{"m-", "p-", "text-", "bg-", "flex", "grid", "w-", "h-", "d-", "col-"}
	for _, p := range prefixes {
		if strings.HasPrefix(c, p) {
			return true
		}
	}
	return false
}

func getStrAttr(attrs map[string]interface{}, key string) (string, bool) {
	if val, ok := attrs[key]; ok {
		if str, ok := val.(string); ok && str != "" {
			return str, true
		}
	}
	return "", false
}

// ==================== 4. The Graph Builder Logic ====================

type JobGraphBuilder struct {
	registry   *NodeRegistry
	graph      *JobGraph
	jobCounter int
	baseURL    string
}

func NewJobGraphBuilder() *JobGraphBuilder {
	return &JobGraphBuilder{
		registry: NewNodeRegistry(),
		graph: &JobGraph{
			Jobs:      make(map[string]*Job),
			Variables: make(map[string]string),
		},
	}
}

func (b *JobGraphBuilder) ProcessEvents(events []RRWebEvent) {
	log.Println("⚙️ Processing events to build Semantic Job Graph...")

	for _, event := range events {
		switch event.Type {
		case EventMeta:
			var data MetaData
			json.Unmarshal(event.Data, &data)
			b.baseURL = data.Href
			b.addJob(&Job{
				Type:       JobTypeNavigation,
				Action:     "navigate",
				IsFixed:    true,
				FixedValue: data.Href,
				Timestamp:  event.Timestamp,
			})

		case EventFullSnapshot:
			var data FullSnapshotData
			json.Unmarshal(event.Data, &data)
			b.registry.Register(data.Node, 0)

		case EventIncrementalSnapshot:
			var data IncrementalSnapshotData
			json.Unmarshal(event.Data, &data)

			// 1. Handle DOM Mutations (Maintain Registry State)
			if data.Source == SourceMutation {
				for _, add := range data.Adds {
					b.registry.Register(add.Node, add.ParentID)
				}
				for _, rem := range data.Removes {
					b.registry.Remove(rem.ID)
				}
				// Note: In a deeper implementation, we would create "Wait" jobs
				// here if a mutation inserts a critical element we interact with later.
			}

			// 2. Handle Interactions (Clicks)
			if data.Source == SourceMouseInteraction && data.Type == Click {
				selector := b.registry.GetRobustSelector(data.ID)
				b.addJob(&Job{
					Type:      JobTypeInteraction,
					Selector:  selector,
					Action:    "click",
					IsFixed:   true,
					Timestamp: event.Timestamp,
				})
			}

			// 3. Handle Inputs (Typing)
			if data.Source == SourceInput {
				node, ok := b.registry.nodes[data.ID]
				if !ok {
					continue
				}

				inputType, _ := getStrAttr(node.Attributes, "type")
				isStatic := data.IsChecked || inputType == "checkbox" || inputType == "radio"

				job := &Job{
					Type:      JobTypeInput,
					Selector:  b.registry.GetRobustSelector(data.ID),
					Action:    "input",
					IsFixed:   isStatic,
					Timestamp: event.Timestamp,
				}

				if isStatic {
					job.FixedValue = "true" // Simplified for checkbox
				} else {
					// Semantic Variable Extraction
					fieldName := extractFieldName(node.Attributes)
					varKey := toCamelCase("Input " + fieldName)
					job.VariableKey = varKey
					b.graph.Variables[varKey] = fmt.Sprintf("Value for %s field", fieldName)
				}
				b.addJob(job)
			}
		}
	}
	b.structureGraph()
}

func (b *JobGraphBuilder) addJob(job *Job) {
	b.jobCounter++
	job.ID = fmt.Sprintf("job_%03d", b.jobCounter)
	b.graph.Jobs[job.ID] = job
}

// structureGraph builds dependencies and layers (Simple sequential logic for now)
func (b *JobGraphBuilder) structureGraph() {
	// Sort jobs by timestamp
	var sortedJobs []*Job
	for _, job := range b.graph.Jobs {
		sortedJobs = append(sortedJobs, job)
	}
	sort.Slice(sortedJobs, func(i, j int) bool {
		return sortedJobs[i].Timestamp < sortedJobs[j].Timestamp
	})

	// Create simplified layers
	b.graph.Layers = make([][]string, 0)
	for _, job := range sortedJobs {
		b.graph.Layers = append(b.graph.Layers, []string{job.ID})
	}
}

// ==================== 5. Code Generator ====================

func GenerateGoRodScript(graph *JobGraph) string {
	var sb strings.Builder

	sb.WriteString(`package main

import (
	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"time"
	"fmt"
)

`)

	// Generate Struct for Variables
	if len(graph.Variables) > 0 {
		sb.WriteString("type AutomationInputs struct {\n")
		for key, desc := range graph.Variables {
			sb.WriteString(fmt.Sprintf("\t%s string // %s\n", key, desc))
		}
		sb.WriteString("}\n\n")
	}

	sb.WriteString("func main() {\n")
	sb.WriteString("\t// 1. Setup Browser\n")
	sb.WriteString("\turl := launcher.New().Headless(false).MustLaunch()\n")
	sb.WriteString("\tbrowser := rod.New().ControlURL(url).MustConnect()\n")
	sb.WriteString("\tdefer browser.MustClose()\n")
	sb.WriteString("\tpage := browser.MustPage()\n\n")

	// Inject Variables
	if len(graph.Variables) > 0 {
		sb.WriteString("\t// 2. Define Inputs (Replace these with real data)\n")
		sb.WriteString("\tinputs := AutomationInputs{\n")
		for key := range graph.Variables {
			sb.WriteString(fmt.Sprintf("\t\t%s: \"SAMPLE_VALUE\",\n", key))
		}
		sb.WriteString("\t}\n\n")
	}

	sb.WriteString("\t// 3. Execute Graph\n")
	for i, layer := range graph.Layers {
		sb.WriteString(fmt.Sprintf("\t// --- Step %d ---\n", i+1))
		for _, jobID := range layer {
			job := graph.Jobs[jobID]
			writeJobLogic(&sb, job)
		}
	}

	sb.WriteString("\n\tfmt.Println(\"✅ Automation Complete\")\n")
	sb.WriteString("\ttime.Sleep(2 * time.Second)\n")
	sb.WriteString("}\n")

	return sb.String()
}

func writeJobLogic(sb *strings.Builder, job *Job) {
	switch job.Type {
	case JobTypeNavigation:
		sb.WriteString(fmt.Sprintf("\tpage.MustNavigate(%q).MustWaitLoad()\n", job.FixedValue))
	case JobTypeInteraction:
		sb.WriteString(fmt.Sprintf("\tpage.MustElement(%q).MustWaitVisible().MustClick()\n", job.Selector))
	case JobTypeInput:
		if job.IsFixed {
			sb.WriteString(fmt.Sprintf("\tpage.MustElement(%q).MustWaitVisible().MustClick() // Toggle/Check\n", job.Selector))
		} else {
			sb.WriteString(fmt.Sprintf("\tpage.MustElement(%q).MustWaitVisible().MustInput(inputs.%s)\n", job.Selector, job.VariableKey))
		}
	}
}

// ==================== 6. Helpers ====================

func extractFieldName(attrs map[string]interface{}) string {
	if v, ok := getStrAttr(attrs, "name"); ok {
		return v
	}
	if v, ok := getStrAttr(attrs, "id"); ok {
		return v
	}
	if v, ok := getStrAttr(attrs, "placeholder"); ok {
		return v
	}
	return "Field"
}

func toCamelCase(s string) string {
	s = strings.ReplaceAll(s, "-", " ")
	s = strings.ReplaceAll(s, "_", " ")
	words := strings.Fields(s)
	for i := range words {
		words[i] = strings.Title(words[i])
	}
	return strings.Join(words, "")
}

// ==================== 7. Main Entry Point ====================

func main() {
	// 1. Open the events.json file
	filePath := "events.json"
	file, err := os.Open(filePath)
	if err != nil {
		log.Fatalf("❌ Failed to open %s: %v", filePath, err)
	}
	defer file.Close()

	// 2. Decode the JSON into our RRWebEvent slice
	var events []RRWebEvent
	decoder := json.NewDecoder(file)
	if err := decoder.Decode(&events); err != nil {
		log.Fatalf("❌ Failed to parse JSON events: %v", err)
	}

	log.Printf("📊 Successfully loaded %d events from %s", len(events), filePath)

	// 3. Initialize the Builder and Process
	builder := NewJobGraphBuilder()
	builder.ProcessEvents(events)

	// 4. Generate the Go Rod Automation Script
	generatedCode := GenerateGoRodScript(builder.graph)

	// 5. Save the output
	outputFile := "generated_automation.go"
	err = os.WriteFile(outputFile, []byte(generatedCode), 0644)
	if err != nil {
		log.Fatalf("❌ Failed to write generated script: %v", err)
	}

	fmt.Printf("\n🚀 Success! Job Graph built with %d jobs.\n", len(builder.graph.Jobs))
	fmt.Printf("📄 Script saved to: %s\n", outputFile)
}
