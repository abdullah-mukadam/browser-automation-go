package main

import (
	"crypto/md5"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"strings"
)

// Event types based on rrweb specification
const (
	EventDomContentLoaded    = 0
	EventLoad                = 1
	EventFullSnapshot        = 2
	EventIncrementalSnapshot = 3
	EventMeta                = 4
	EventCustom              = 5
	EventPlugin              = 6
)

// Incremental snapshot sources (complete list from rrweb)
const (
	SourceMutation          = 0
	SourceMouseMove         = 1
	SourceMouseInteraction  = 2
	SourceScroll            = 3
	SourceViewportResize    = 4
	SourceInput             = 5
	SourceTouchMove         = 6
	SourceMediaInteraction  = 7
	SourceStyleSheetRule    = 8
	SourceCanvasMutation    = 9
	SourceFont              = 10
	SourceLog               = 11
	SourceDrag              = 12
	SourceStyleDeclaration  = 13
	SourceSelection         = 14
	SourceAdoptedStyleSheet = 15
	SourceCustomElement     = 16
)

// Mouse interaction types from rrweb
const (
	MouseUp           = 0
	MouseDown         = 1
	Click             = 2
	ContextMenu       = 3
	DblClick          = 4
	Focus             = 5
	Blur              = 6
	TouchStart        = 7
	TouchMoveDeparted = 8
	TouchEnd          = 9
	TouchCancel       = 10
)

type RRWebEvent struct {
	Type      int             `json:"type"`
	Data      json.RawMessage `json:"data"`
	Timestamp int64           `json:"timestamp"`
}

type FullSnapshotData struct {
	Node          Node `json:"node"`
	InitialOffset struct {
		Left int `json:"left"`
		Top  int `json:"top"`
	} `json:"initialOffset"`
}

type Node struct {
	Type        int                    `json:"type"`
	TagName     string                 `json:"tagName"`
	Attributes  map[string]interface{} `json:"attributes"`
	ChildNodes  []Node                 `json:"childNodes"`
	ID          int                    `json:"id"`
	TextContent string                 `json:"textContent"`
	Name        string                 `json:"name"`
	PublicId    string                 `json:"publicId"`
	SystemId    string                 `json:"systemId"`
	IsStyle     bool                   `json:"isStyle"`
	IsSVG       bool                   `json:"isSVG"`
}

type IncrementalSnapshotData struct {
	Source      int                 `json:"source"`
	Type        int                 `json:"type"`
	ID          int                 `json:"id"`
	X           float64             `json:"x"`
	Y           float64             `json:"y"`
	Text        string              `json:"text"`
	IsChecked   bool                `json:"isChecked"`
	Positions   []MousePosition     `json:"positions"`
	Adds        []AddedNode         `json:"adds"`
	Removes     []RemovedNode       `json:"removes"`
	Texts       []TextMutation      `json:"texts"`
	Attributes  []AttributeMutation `json:"attributes"`
	PointerType int                 `json:"pointerType"`
}

type MousePosition struct {
	X          float64 `json:"x"`
	Y          float64 `json:"y"`
	ID         int     `json:"id"`
	TimeOffset int64   `json:"timeOffset"`
}

type AddedNode struct {
	ParentID int  `json:"parentId"`
	NextID   *int `json:"nextId"`
	Node     Node `json:"node"`
}

type RemovedNode struct {
	ParentID int `json:"parentId"`
	ID       int `json:"id"`
}

type TextMutation struct {
	ID    int    `json:"id"`
	Value string `json:"value"`
}

type AttributeMutation struct {
	ID         int                    `json:"id"`
	Attributes map[string]interface{} `json:"attributes"`
}

type MetaData struct {
	Href   string `json:"href"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
}

// Action represents a user action
type Action struct {
	Type      string
	Selector  string
	Value     string
	Timestamp int64
	X         float64
	Y         float64
	Hash      string // For deduplication
}

// NodeRegistry tracks rrweb node IDs to selectors
type NodeRegistry struct {
	nodes map[int]NodeInfo
}

type NodeInfo struct {
	TagName     string
	Attributes  map[string]interface{}
	Parent      int
	TextContent string
}

func NewNodeRegistry() *NodeRegistry {
	return &NodeRegistry{
		nodes: make(map[int]NodeInfo),
	}
}

func (nr *NodeRegistry) Register(node Node, parentID int) {
	nr.nodes[node.ID] = NodeInfo{
		TagName:     node.TagName,
		Attributes:  node.Attributes,
		Parent:      parentID,
		TextContent: node.TextContent,
	}
	for _, child := range node.ChildNodes {
		nr.Register(child, node.ID)
	}
}

func (nr *NodeRegistry) GetSelector(id int) string {
	info, ok := nr.nodes[id]
	if !ok {
		return fmt.Sprintf("[data-rrweb-id='%d']", id)
	}

	getAttrString := func(key string) (string, bool) {
		val, ok := info.Attributes[key]
		if !ok {
			return "", false
		}
		switch v := val.(type) {
		case string:
			return v, v != ""
		case bool:
			return "", false
		default:
			return fmt.Sprintf("%v", v), true
		}
	}

	// Priority: id > name > data-testid > role > type > class > tag
	if idAttr, ok := getAttrString("id"); ok {
		return fmt.Sprintf("#%s", idAttr)
	}

	if name, ok := getAttrString("name"); ok {
		return fmt.Sprintf("%s[name='%s']", info.TagName, name)
	}

	if dataTestId, ok := getAttrString("data-testid"); ok {
		return fmt.Sprintf("[data-testid='%s']", dataTestId)
	}

	if role, ok := getAttrString("role"); ok {
		return fmt.Sprintf("%s[role='%s']", info.TagName, role)
	}

	if typ, ok := getAttrString("type"); ok {
		return fmt.Sprintf("%s[type='%s']", info.TagName, typ)
	}

	if class, ok := getAttrString("class"); ok {
		classes := strings.Fields(class)
		if len(classes) > 0 {
			// Use first meaningful class (not utility classes)
			for _, cls := range classes {
				if !isUtilityClass(cls) {
					return fmt.Sprintf("%s.%s", info.TagName, cls)
				}
			}
			return fmt.Sprintf("%s.%s", info.TagName, classes[0])
		}
	}

	if ariaLabel, ok := getAttrString("aria-label"); ok {
		return fmt.Sprintf("%s[aria-label='%s']", info.TagName, ariaLabel)
	}

	return info.TagName
}

func isUtilityClass(class string) bool {
	// Common utility class prefixes
	prefixes := []string{"m-", "p-", "mt-", "mb-", "ml-", "mr-", "pt-", "pb-", "pl-", "pr-",
		"w-", "h-", "flex-", "grid-", "text-", "bg-", "border-"}
	for _, prefix := range prefixes {
		if strings.HasPrefix(class, prefix) {
			return true
		}
	}
	return false
}

// Converter converts rrweb events to Go Rod actions
type Converter struct {
	registry        *NodeRegistry
	baseURL         string
	actions         []Action
	lastTimestamp   int64
	lastClickTarget string
	lastInputTarget string
	clickCount      map[string]int
}

func NewConverter() *Converter {
	return &Converter{
		registry:   NewNodeRegistry(),
		actions:    []Action{},
		clickCount: make(map[string]int),
	}
}

func (c *Converter) ProcessEvents(events []RRWebEvent) error {
	for _, event := range events {
		switch event.Type {
		case EventFullSnapshot:
			var data FullSnapshotData
			if err := json.Unmarshal(event.Data, &data); err != nil {
				return fmt.Errorf("failed to unmarshal full snapshot: %w", err)
			}
			c.registry.Register(data.Node, 0)

		case EventMeta:
			var data MetaData
			if err := json.Unmarshal(event.Data, &data); err != nil {
				return fmt.Errorf("failed to unmarshal meta: %w", err)
			}
			c.baseURL = data.Href
			c.addAction(Action{
				Type:      "navigate",
				Value:     data.Href,
				Timestamp: event.Timestamp,
			})

		case EventIncrementalSnapshot:
			var data IncrementalSnapshotData
			if err := json.Unmarshal(event.Data, &data); err != nil {
				return fmt.Errorf("failed to unmarshal incremental snapshot: %w", err)
			}
			c.processIncrementalSnapshot(data, event.Timestamp)
		}
	}
	return nil
}

func (c *Converter) processIncrementalSnapshot(data IncrementalSnapshotData, timestamp int64) {
	switch data.Source {
	case SourceMouseInteraction:
		c.processMouseInteraction(data, timestamp)
	case SourceInput:
		c.processInput(data, timestamp)
	case SourceScroll:
		c.processScroll(data, timestamp)
	case SourceMutation:
		c.processMutation(data)
	case SourceViewportResize:
		// Viewport resize is usually not needed for replay
	case SourceMouseMove, SourceTouchMove, SourceDrag:
		// Mouse movements are too noisy for automation scripts
	}
}

func (c *Converter) processMouseInteraction(data IncrementalSnapshotData, timestamp int64) {
	selector := c.registry.GetSelector(data.ID)

	switch data.Type {
	case Click:
		// Track repeated clicks on same element
		c.clickCount[selector]++

		// Avoid duplicate clicks within 500ms
		if c.lastClickTarget == selector && timestamp-c.lastTimestamp < 500 {
			return
		}

		c.lastClickTarget = selector
		c.addAction(Action{
			Type:      "click",
			Selector:  selector,
			Timestamp: timestamp,
			X:         data.X,
			Y:         data.Y,
		})

	case DblClick:
		// Replace last click with dblclick if it was on same element
		if len(c.actions) > 0 {
			lastAction := c.actions[len(c.actions)-1]
			if lastAction.Type == "click" && lastAction.Selector == selector {
				c.actions[len(c.actions)-1].Type = "dblclick"
				return
			}
		}
		c.addAction(Action{
			Type:      "dblclick",
			Selector:  selector,
			Timestamp: timestamp,
		})

	case Focus:
		// Only record focus if not followed by immediate input
		c.addAction(Action{
			Type:      "focus",
			Selector:  selector,
			Timestamp: timestamp,
		})

	case ContextMenu:
		c.addAction(Action{
			Type:      "contextmenu",
			Selector:  selector,
			Timestamp: timestamp,
		})
	}

	c.lastTimestamp = timestamp
}

func (c *Converter) processInput(data IncrementalSnapshotData, timestamp int64) {
	selector := c.registry.GetSelector(data.ID)

	// Remove unnecessary focus before input
	if len(c.actions) > 0 {
		lastAction := &c.actions[len(c.actions)-1]
		if lastAction.Type == "focus" && lastAction.Selector == selector {
			c.actions = c.actions[:len(c.actions)-1]
		}
	}

	actionType := "input"
	if data.IsChecked {
		actionType = "check"
	}

	// Merge consecutive inputs on same element
	if c.lastInputTarget == selector && len(c.actions) > 0 {
		lastAction := &c.actions[len(c.actions)-1]
		if lastAction.Type == "input" && timestamp-lastAction.Timestamp < 1000 {
			lastAction.Value = data.Text
			lastAction.Timestamp = timestamp
			return
		}
	}

	c.lastInputTarget = selector
	c.addAction(Action{
		Type:      actionType,
		Selector:  selector,
		Value:     data.Text,
		Timestamp: timestamp,
	})
}

func (c *Converter) processScroll(data IncrementalSnapshotData, timestamp int64) {
	// Only record significant scrolls (>100px change)
	if len(c.actions) > 0 {
		lastAction := &c.actions[len(c.actions)-1]
		if lastAction.Type == "scroll" {
			deltaY := data.Y - lastAction.Y
			if deltaY < 100 && deltaY > -100 {
				lastAction.Y = data.Y
				lastAction.X = data.X
				lastAction.Timestamp = timestamp
				return
			}
		}
	}

	c.addAction(Action{
		Type:      "scroll",
		X:         data.X,
		Y:         data.Y,
		Timestamp: timestamp,
	})
}

func (c *Converter) processMutation(data IncrementalSnapshotData) {
	// Register new nodes for selector resolution
	for _, add := range data.Adds {
		c.registry.Register(add.Node, add.ParentID)
	}

	// Update text mutations
	for _, text := range data.Texts {
		if info, ok := c.registry.nodes[text.ID]; ok {
			info.TextContent = text.Value
			c.registry.nodes[text.ID] = info
		}
	}

	// Update attribute mutations
	for _, attr := range data.Attributes {
		if info, ok := c.registry.nodes[attr.ID]; ok {
			for k, v := range attr.Attributes {
				info.Attributes[k] = v
			}
			c.registry.nodes[attr.ID] = info
		}
	}
}

func (c *Converter) addAction(action Action) {
	// Generate hash for deduplication
	action.Hash = c.hashAction(action)
	c.actions = append(c.actions, action)
}

func (c *Converter) hashAction(action Action) string {
	data := fmt.Sprintf("%s:%s:%s", action.Type, action.Selector, action.Value)
	hash := md5.Sum([]byte(data))
	return fmt.Sprintf("%x", hash)
}

// GenerateGoRodScript generates the final Go Rod script with deduplication
func (c *Converter) GenerateGoRodScript() string {
	var sb strings.Builder

	sb.WriteString(`package main

import (
	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"log"
	"time"
)

func main() {
	// Launch browser
	u := launcher.New().Headless(false).MustLaunch()
	browser := rod.New().ControlURL(u).MustConnect()
	defer browser.MustClose()

	page := browser.MustPage()
	
`)

	// Deduplicate and optimize actions
	optimizedActions := c.deduplicateActions(c.actions)

	lastTime := int64(0)
	for i, action := range optimizedActions {
		// Add realistic delays between actions
		if lastTime > 0 && action.Timestamp > lastTime {
			delay := action.Timestamp - lastTime
			if delay > 100 && delay < 10000 { // Between 100ms and 10s
				sb.WriteString(fmt.Sprintf("\ttime.Sleep(%d * time.Millisecond)\n", delay))
			} else if delay >= 10000 {
				// Cap very long delays
				sb.WriteString("\ttime.Sleep(2 * time.Second)\n")
			}
		}
		lastTime = action.Timestamp

		switch action.Type {
		case "navigate":
			sb.WriteString(fmt.Sprintf("\t// Step %d: Navigate to page\n", i+1))
			sb.WriteString(fmt.Sprintf("\tpage.MustNavigate(\"%s\").MustWaitLoad()\n", escapeString(action.Value)))

		case "click":
			sb.WriteString(fmt.Sprintf("\t// Step %d: Click element\n", i+1))
			sb.WriteString(fmt.Sprintf("\tpage.MustElement(\"%s\").MustWaitVisible().MustClick()\n",
				escapeSelector(action.Selector)))

		case "dblclick":
			sb.WriteString(fmt.Sprintf("\t// Step %d: Double-click element\n", i+1))
			sb.WriteString(fmt.Sprintf("\tel := page.MustElement(\"%s\").MustWaitVisible()\n",
				escapeSelector(action.Selector)))
			sb.WriteString("\tel.MustClick().MustClick()\n")

		case "input":
			sb.WriteString(fmt.Sprintf("\t// Step %d: Input text\n", i+1))
			sb.WriteString(fmt.Sprintf("\tpage.MustElement(\"%s\").MustWaitVisible().MustInput(\"%s\")\n",
				escapeSelector(action.Selector), escapeString(action.Value)))

		case "check":
			sb.WriteString(fmt.Sprintf("\t// Step %d: Toggle checkbox\n", i+1))
			sb.WriteString(fmt.Sprintf("\tpage.MustElement(\"%s\").MustWaitVisible().MustClick()\n",
				escapeSelector(action.Selector)))

		case "scroll":
			sb.WriteString(fmt.Sprintf("\t// Step %d: Scroll page\n", i+1))
			sb.WriteString(fmt.Sprintf("\tpage.MustEval(`window.scrollTo(%f, %f)`)\n",
				action.X, action.Y))

		case "focus":
			sb.WriteString(fmt.Sprintf("\t// Step %d: Focus element\n", i+1))
			sb.WriteString(fmt.Sprintf("\tpage.MustElement(\"%s\").MustWaitVisible().MustFocus()\n",
				escapeSelector(action.Selector)))

		case "contextmenu":
			sb.WriteString(fmt.Sprintf("\t// Step %d: Right-click element\n", i+1))
			sb.WriteString(fmt.Sprintf("\tpage.MustElement(\"%s\").MustWaitVisible().MustClick(proto.InputMouseButtonRight)\n",
				escapeSelector(action.Selector)))
		}

		sb.WriteString("\n")
	}

	sb.WriteString(`	// Wait for final state
	time.Sleep(2 * time.Second)
	
	log.Println("✓ Test completed successfully")
	log.Println("Press Ctrl+C to exit...")
	time.Sleep(1 * time.Hour)
}
`)

	return sb.String()
}

// deduplicateActions removes redundant consecutive actions
func (c *Converter) deduplicateActions(actions []Action) []Action {
	if len(actions) == 0 {
		return actions
	}

	var result []Action
	result = append(result, actions[0])

	for i := 1; i < len(actions); i++ {
		curr := actions[i]
		prev := result[len(result)-1]

		// Skip if same action on same element within 500ms
		if curr.Type == prev.Type &&
			curr.Selector == prev.Selector &&
			curr.Type == "click" &&
			curr.Timestamp-prev.Timestamp < 500 {
			continue
		}

		// Skip redundant scrolls
		if curr.Type == "scroll" && prev.Type == "scroll" {
			result[len(result)-1] = curr
			continue
		}

		// Skip focus immediately followed by input/click
		if prev.Type == "focus" {
			if curr.Selector == prev.Selector &&
				(curr.Type == "input" || curr.Type == "click") {
				result = result[:len(result)-1]
			}
		}

		result = append(result, curr)
	}

	return result
}

func escapeSelector(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return s
}

func escapeString(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	s = strings.ReplaceAll(s, "\n", `\n`)
	s = strings.ReplaceAll(s, "\t", `\t`)
	s = strings.ReplaceAll(s, "\r", `\r`)
	return s
}

func main() {
	// Read rrweb events from file
	data, err := ioutil.ReadFile("events.json")
	if err != nil {
		log.Fatalf("❌ Failed to read events.json: %v", err)
	}

	// Parse events - handle both single event and array
	var events []RRWebEvent

	// Try parsing as array first
	if err := json.Unmarshal(data, &events); err != nil {
		// Try parsing as single event
		var singleEvent RRWebEvent
		if err := json.Unmarshal(data, &singleEvent); err != nil {
			log.Fatalf("❌ Failed to parse events: %v", err)
		}
		events = []RRWebEvent{singleEvent}
	}

	log.Printf("📊 Loaded %d rrweb events\n", len(events))

	// Convert events to actions
	converter := NewConverter()
	if err := converter.ProcessEvents(events); err != nil {
		log.Fatalf("❌ Failed to process events: %v", err)
	}

	log.Printf("🔄 Extracted %d raw actions\n", len(converter.actions))

	// Generate Go Rod script
	script := converter.GenerateGoRodScript()

	// Write to file
	if err := ioutil.WriteFile("generated_test.go", []byte(script), 0644); err != nil {
		log.Fatalf("❌ Failed to write script: %v", err)
	}

	// Count optimized actions
	optimized := converter.deduplicateActions(converter.actions)

	fmt.Println("✅ Generated Go Rod script: generated_test.go")
	fmt.Printf("📝 Actions: %d raw → %d optimized\n", len(converter.actions), len(optimized))
	fmt.Printf("🎯 Base URL: %s\n", converter.baseURL)
	fmt.Println("\n💡 Run with: go run generated_test.go")
}
