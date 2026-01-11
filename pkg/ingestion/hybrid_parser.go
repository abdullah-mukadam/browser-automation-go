package ingestion

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"dev/bravebird/browser-automation-go/pkg/models"
)

// HybridParser parses hybrid_events.json files containing both rrweb and custom events
type HybridParser struct {
	events       []models.HybridEvent
	nodeRegistry *NodeRegistry
}

// NodeRegistry tracks DOM nodes from rrweb snapshots
type NodeRegistry struct {
	nodes   map[int]*models.SerializedNode
	parents map[int]int
}

// NewHybridParser creates a new parser instance
func NewHybridParser() *HybridParser {
	return &HybridParser{
		events: make([]models.HybridEvent, 0),
		nodeRegistry: &NodeRegistry{
			nodes:   make(map[int]*models.SerializedNode),
			parents: make(map[int]int),
		},
	}
}

// ParseFile reads and parses a hybrid_events.json file
func (p *HybridParser) ParseFile(filepath string) error {
	data, err := os.ReadFile(filepath)
	if err != nil {
		return fmt.Errorf("failed to read file: %w", err)
	}

	return p.Parse(data)
}

// Parse parses JSON data containing hybrid events
func (p *HybridParser) Parse(data []byte) error {
	var rawEvents []json.RawMessage
	if err := json.Unmarshal(data, &rawEvents); err != nil {
		return fmt.Errorf("failed to parse JSON array: %w", err)
	}

	for _, raw := range rawEvents {
		event, err := p.parseEvent(raw)
		if err != nil {
			// Skip unparseable events but log them
			continue
		}
		p.events = append(p.events, event)
	}

	// Sort events by timestamp
	sort.Slice(p.events, func(i, j int) bool {
		return p.events[i].Timestamp < p.events[j].Timestamp
	})

	return nil
}

// parseEvent parses a single event, handling both rrweb and custom sources
func (p *HybridParser) parseEvent(raw json.RawMessage) (models.HybridEvent, error) {
	// First, determine the source
	var peek struct {
		Source string `json:"source"`
	}
	if err := json.Unmarshal(raw, &peek); err != nil {
		return models.HybridEvent{}, err
	}

	var event models.HybridEvent
	if err := json.Unmarshal(raw, &event); err != nil {
		return models.HybridEvent{}, err
	}

	// For rrweb events, unwrap the nested data structure
	if event.Source == "rrweb" && event.Data != nil {
		var rrwebData models.RRWebEventData
		if err := json.Unmarshal(event.Data, &rrwebData); err == nil {
			// Update the main event with unwrapped data
			event.Data = rrwebData.Data
			event.Type = rrwebData.Type
		}
	}

	return event, nil
}

// GetEvents returns all parsed events
func (p *HybridParser) GetEvents() []models.HybridEvent {
	return p.events
}

// GetRRWebEvents returns only rrweb source events
func (p *HybridParser) GetRRWebEvents() []models.HybridEvent {
	var result []models.HybridEvent
	for _, e := range p.events {
		if e.Source == "rrweb" {
			result = append(result, e)
		}
	}
	return result
}

// GetCustomEvents returns only custom source events
func (p *HybridParser) GetCustomEvents() []models.HybridEvent {
	var result []models.HybridEvent
	for _, e := range p.events {
		if e.Source == "custom" {
			result = append(result, e)
		}
	}
	return result
}

// ExtractSemanticActions processes events and extracts semantic actions
func (p *HybridParser) ExtractSemanticActions() []models.SemanticAction {
	var actions []models.SemanticAction
	sequenceID := 0

	// First pass: Build node registry from rrweb full snapshots
	for _, event := range p.events {
		if event.Source == "rrweb" {
			eventType, ok := event.Type.(int)
			if !ok {
				// Try float64 (JSON numbers are float64 by default)
				if f, ok := event.Type.(float64); ok {
					eventType = int(f)
				} else {
					continue
				}
			}

			if eventType == models.RRWebEventFullSnapshot {
				p.processFullSnapshot(event.Data)
			} else if eventType == models.RRWebEventIncremental {
				p.processIncrementalSnapshot(event.Data)
			}
		}
	}

	// Track current URL
	currentURL := ""

	// Second pass: Extract semantic actions from custom and rrweb events
	for _, event := range p.events {
		if event.Source == "custom" {
			eventType, ok := event.Type.(string)
			if !ok {
				continue
			}

			sequenceID++
			action := p.customEventToAction(event, eventType, sequenceID)
			if action != nil {
				actions = append(actions, *action)
			}
		} else if event.Source == "rrweb" {
			eventType := getEventType(event.Type)
			intEventType := eventType

			// Handle Meta events (navigation)
			if intEventType == models.RRWebEventMeta {
				var meta models.RRWebMetaData
				if err := json.Unmarshal(event.Data, &meta); err == nil && meta.Href != "" {
					if meta.Href != currentURL {
						currentURL = meta.Href
						sequenceID++
						actions = append(actions, models.SemanticAction{
							SequenceID:      sequenceID,
							ActionType:      models.ActionNavigate,
							Value:           meta.Href,
							InteractionRank: models.RankHigh,
							Target: models.SemanticTarget{
								Selector: "window",
							},
							Timestamp: event.Timestamp,
							Metadata: map[string]interface{}{
								"source": "rrweb_meta",
							},
						})
					}
				}
			}

			// Handle Incremental events
			if intEventType == models.RRWebEventIncremental {
				action := p.rrwebIncrementalToAction(event, &sequenceID)
				if action != nil {
					actions = append(actions, *action)
				}
			}
		}
	}

	return actions
}

// rrwebIncrementalToAction converts an rrweb incremental event to a semantic action
func (p *HybridParser) rrwebIncrementalToAction(event models.HybridEvent, sequenceID *int) *models.SemanticAction {
	var incr models.RRWebIncrementalData
	if err := json.Unmarshal(event.Data, &incr); err != nil {
		return nil
	}

	switch incr.Source {
	case models.SourceMouseInteraction:
		return p.mouseInteractionToAction(event, incr, sequenceID)
	case models.SourceInput:
		return p.inputToAction(event, incr, sequenceID)
	case models.SourceScroll:
		// Only capture significant scrolls
		return nil // Scrolls are low-value for automation
	case models.SourceDrag:
		*sequenceID++
		return &models.SemanticAction{
			SequenceID:      *sequenceID,
			ActionType:      models.ActionDrag,
			InteractionRank: models.RankMedium,
			Target: models.SemanticTarget{
				NodeID: incr.ID,
			},
			Timestamp: event.Timestamp,
			Metadata: map[string]interface{}{
				"source": "rrweb_drag",
				"x":      incr.X,
				"y":      incr.Y,
			},
		}
	case models.SourceSelection:
		*sequenceID++
		return &models.SemanticAction{
			SequenceID:      *sequenceID,
			ActionType:      models.ActionSelect,
			InteractionRank: models.RankMedium,
			Value:           incr.Text,
			Timestamp:       event.Timestamp,
			Metadata: map[string]interface{}{
				"source": "rrweb_selection",
			},
		}
	case models.SourceMediaInteraction:
		return p.mediaInteractionToAction(event, incr, sequenceID)
	}

	return nil
}

// mouseInteractionToAction converts a mouse interaction event to an action
func (p *HybridParser) mouseInteractionToAction(event models.HybridEvent, incr models.RRWebIncrementalData, sequenceID *int) *models.SemanticAction {
	*sequenceID++
	action := &models.SemanticAction{
		SequenceID: *sequenceID,
		Target: models.SemanticTarget{
			NodeID: incr.ID,
		},
		Timestamp: event.Timestamp,
		Metadata: map[string]interface{}{
			"source": "rrweb_mouse_interaction",
			"x":      incr.X,
			"y":      incr.Y,
		},
	}

	// Enrich with node info if available
	if node := p.GetNode(incr.ID); node != nil {
		action.Target.Tag = node.TagName
		action.Target.Text = node.TextContent
	}

	switch incr.Type {
	case models.MouseInteractionClick:
		action.ActionType = models.ActionClick
		// Default to RankHigh, but downgrade if target is weak
		action.InteractionRank = models.RankHigh
		if action.Target.Tag == "" || (action.Target.Tag == "div" && len(action.Target.Text) == 0 && action.Target.Selector == "") {
			// Empty/Generic click
			action.InteractionRank = models.RankLow
		}

	case models.MouseInteractionDblClick:
		action.ActionType = models.ActionDblClick
		action.InteractionRank = models.RankHigh

	case models.MouseInteractionContextMenu:
		action.ActionType = models.ActionRightClick
		action.InteractionRank = models.RankHigh

	case models.MouseInteractionFocus:
		action.ActionType = models.ActionFocus
		action.InteractionRank = models.RankMedium
		// Downgrade empty focus
		if action.Target.Tag == "" {
			action.InteractionRank = models.RankLow
		}

	case models.MouseInteractionBlur:
		action.ActionType = models.ActionBlur
		action.InteractionRank = models.RankLow

	default:
		// Mouse up/down are intermediate events, skip
		*sequenceID--
		return nil
	}

	return action
}

// inputToAction converts an input event to an action
func (p *HybridParser) inputToAction(event models.HybridEvent, incr models.RRWebIncrementalData, sequenceID *int) *models.SemanticAction {
	*sequenceID++
	action := &models.SemanticAction{
		SequenceID:      *sequenceID,
		ActionType:      models.ActionInput,
		Value:           incr.Text,
		InteractionRank: models.RankHigh,
		Target: models.SemanticTarget{
			NodeID: incr.ID,
		},
		Timestamp: event.Timestamp,
		Metadata: map[string]interface{}{
			"source": "rrweb_input",
		},
	}

	if node := p.GetNode(incr.ID); node != nil {
		action.Target.Tag = node.TagName
		action.Target.Attributes = node.Attributes
	} else {
		// Fallback: assume input if unknown, to be fixed by enrichSelectors later
		action.Target.Tag = "input"
	}

	return action
}

// mediaInteractionToAction converts a media interaction event to an action
func (p *HybridParser) mediaInteractionToAction(event models.HybridEvent, incr models.RRWebIncrementalData, sequenceID *int) *models.SemanticAction {
	// Parse media interaction type from data
	var mediaData struct {
		Type int `json:"type"` // 0=play, 1=pause, 2=seeked
	}
	if err := json.Unmarshal(event.Data, &mediaData); err != nil {
		return nil
	}

	*sequenceID++
	action := &models.SemanticAction{
		SequenceID:      *sequenceID,
		InteractionRank: models.RankMedium,
		Target: models.SemanticTarget{
			NodeID: incr.ID,
		},
		Timestamp: event.Timestamp,
		Metadata: map[string]interface{}{
			"source": "rrweb_media",
		},
	}

	switch mediaData.Type {
	case 0:
		action.ActionType = models.ActionMediaPlay
	case 1:
		action.ActionType = models.ActionMediaPause
	case 2:
		action.ActionType = models.ActionMediaSeek
	default:
		*sequenceID--
		return nil
	}

	return action
}

// customEventToAction converts a custom event to a semantic action
func (p *HybridParser) customEventToAction(event models.HybridEvent, eventType string, sequenceID int) *models.SemanticAction {
	action := &models.SemanticAction{
		SequenceID: sequenceID,
		Timestamp:  event.Timestamp,
	}

	// Set target if available
	if event.Target != nil {
		action.Target = models.SemanticTarget{
			Tag:      event.Target.Tag,
			Selector: event.Target.Selector,
			Text:     truncateText(event.Target.Text, 100),
		}
	}

	switch eventType {
	case "click":
		action.ActionType = models.ActionClick
		action.InteractionRank = p.calculateInteractionRank(event.Target)

	case "input":
		action.ActionType = models.ActionInput
		action.Value = event.Value
		action.InteractionRank = models.RankHigh

	case "copy":
		action.ActionType = models.ActionCopy
		action.InteractionRank = models.RankHigh

	case "paste":
		action.ActionType = models.ActionPaste
		action.InteractionRank = models.RankHigh
		action.Value = event.Value // Paste might have value?

	case "keydown", "keypress":
		// Handle keyboard shortcuts
		if event.Modifiers != nil && (event.Modifiers.Ctrl || event.Modifiers.Meta) {
			switch event.Shortcut {
			case "copy":
				action.ActionType = models.ActionCopy
			case "paste":
				action.ActionType = models.ActionPaste
			default:
				action.ActionType = models.ActionKeypress
				action.Value = p.formatKeyCombo(event)
			}
		} else {
			action.ActionType = models.ActionKeypress
			action.Value = event.Key

			// Downgrade empty or single modifier keypresses without value
			if action.Value == "" || action.Value == "Shift" || action.Value == "Control" || action.Value == "Alt" || action.Value == "Meta" {
				action.InteractionRank = models.RankLow
			} else {
				action.InteractionRank = models.RankMedium
			}
		}
		// If rank wasn't set above (e.g. shortcut case)
		if action.InteractionRank == "" {
			action.InteractionRank = models.RankMedium
		}

	case "scroll":
		action.ActionType = models.ActionScroll
		action.InteractionRank = models.RankLow

	default:
		// Unknown event type, skip
		return nil
	}

	// Add metadata
	action.Metadata = map[string]interface{}{
		"original_type": eventType,
	}
	if event.Modifiers != nil {
		action.Metadata["modifiers"] = event.Modifiers
	}

	return action
}

// formatKeyCombo formats a keyboard shortcut
func (p *HybridParser) formatKeyCombo(event models.HybridEvent) string {
	var parts []string
	if event.Modifiers != nil {
		if event.Modifiers.Ctrl {
			parts = append(parts, "Ctrl")
		}
		if event.Modifiers.Meta {
			parts = append(parts, "Cmd")
		}
		if event.Modifiers.Alt {
			parts = append(parts, "Alt")
		}
		if event.Modifiers.Shift {
			parts = append(parts, "Shift")
		}
	}
	parts = append(parts, strings.ToUpper(event.Key))
	return strings.Join(parts, "+")
}

// calculateInteractionRank determines how important a click target is
func (p *HybridParser) calculateInteractionRank(target *models.EventTarget) models.InteractionRank {
	if target == nil {
		return models.RankLow
	}

	tag := strings.ToLower(target.Tag)

	// High-rank interactive elements
	highRankTags := []string{"button", "a", "input", "select", "textarea"}
	for _, t := range highRankTags {
		if tag == t {
			return models.RankHigh
		}
	}

	// Medium-rank elements (often have click handlers)
	mediumRankTags := []string{"div", "span", "li", "label"}
	for _, t := range mediumRankTags {
		if tag == t {
			// Check if selector suggests interactivity
			if strings.Contains(target.Selector, "button") ||
				strings.Contains(target.Selector, "btn") ||
				strings.Contains(target.Selector, "click") {
				return models.RankMedium
			}
		}
	}

	return models.RankLow
}

// processFullSnapshot registers nodes from a full DOM snapshot
func (p *HybridParser) processFullSnapshot(data json.RawMessage) {
	var snapshot struct {
		Node *models.SerializedNode `json:"node"`
	}
	if err := json.Unmarshal(data, &snapshot); err == nil && snapshot.Node != nil {
		p.registerNode(snapshot.Node, 0)
	}
}

// processIncrementalSnapshot handles DOM mutations
func (p *HybridParser) processIncrementalSnapshot(data json.RawMessage) {
	var incr models.RRWebIncrementalData
	if err := json.Unmarshal(data, &incr); err != nil {
		return
	}

	if incr.Source == models.SourceMutation {
		for _, add := range incr.Adds {
			if add.Node != nil {
				p.registerNode(add.Node, add.ParentID)
			}
		}
	}
}

// registerNode adds a node and its children to the registry
func (p *HybridParser) registerNode(node *models.SerializedNode, parentID int) {
	if node == nil {
		return
	}

	p.nodeRegistry.nodes[node.ID] = node
	if parentID != 0 {
		p.nodeRegistry.parents[node.ID] = parentID
	}

	for _, child := range node.ChildNodes {
		p.registerNode(child, node.ID)
	}
}

// GetNode retrieves a node by ID
func (p *HybridParser) GetNode(id int) *models.SerializedNode {
	return p.nodeRegistry.nodes[id]
}

// truncateText truncates text to a maximum length
func truncateText(text string, maxLen int) string {
	if len(text) <= maxLen {
		return text
	}
	return text[:maxLen]
}

// getEventType extracts the event type as int from interface{} (handles both int and float64)
func getEventType(t interface{}) int {
	switch v := t.(type) {
	case int:
		return v
	case float64:
		return int(v)
	default:
		return -1
	}
}

// GetStartURL extracts the initial URL from the events
func (p *HybridParser) GetStartURL() string {
	for _, event := range p.events {
		if event.Source == "rrweb" {
			eventType := getEventType(event.Type)
			if eventType == models.RRWebEventMeta {
				var meta models.RRWebMetaData
				if err := json.Unmarshal(event.Data, &meta); err == nil && meta.Href != "" {
					return meta.Href
				}
			}
		}
	}
	return ""
}
