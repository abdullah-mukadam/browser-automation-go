package semantic

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"unicode"

	"dev/bravebird/browser-automation-go/pkg/ingestion"
	"dev/bravebird/browser-automation-go/pkg/models"
)

// ToleranceLevel determines how sensitive the extractor is
type ToleranceLevel int

const (
	ToleranceLow    ToleranceLevel = 0 // Strict: High rank only
	ToleranceMedium ToleranceLevel = 1 // Default: High + Medium
	ToleranceHigh   ToleranceLevel = 2 // Permissive: All actions
)

// Extractor processes hybrid events and extracts rich semantic context
type Extractor struct {
	parser    *ingestion.HybridParser
	tolerance ToleranceLevel
}

// NewExtractor creates a new semantic extractor
func NewExtractor(parser *ingestion.HybridParser, tolerance ToleranceLevel) *Extractor {
	return &Extractor{
		parser:    parser,
		tolerance: tolerance,
	}
}

// ExtractActions extracts semantic actions from parsed events
func (e *Extractor) ExtractActions() []models.SemanticAction {
	actions := e.parser.ExtractSemanticActions()

	// Post-process actions
	actions = e.deduplicateNavigations(actions)
	actions = e.debounceInputs(actions)
	actions = e.enrichSelectors(actions)
	actions = e.filterLowValueActions(actions)
	actions = e.resequence(actions)

	return actions
}

// deduplicateNavigations removes consecutive duplicate navigation events
func (e *Extractor) deduplicateNavigations(actions []models.SemanticAction) []models.SemanticAction {
	if len(actions) == 0 {
		return actions
	}

	var result []models.SemanticAction
	result = append(result, actions[0])

	// Track the current URL context
	currentURL := actions[0].Value

	for i := 1; i < len(actions); i++ {
		curr := actions[i]
		prev := result[len(result)-1]

		// Skip duplicate navigations or navigations that are consequential to an interaction
		if curr.ActionType == models.ActionNavigate {

			// Case 1: Consecutive navigations
			if prev.ActionType == models.ActionNavigate {
				// Check if URLs are essentially the same (ignoring query params for tracking)
				if e.normalizeURL(curr.Value) == e.normalizeURL(prev.Value) {
					continue
				}
				// Check if same domain (likely a redirect or consequential navigation)
				if e.isSameDomain(curr.Value, prev.Value) {
					// Update current URL even if we skip? No, if we skip, we stay on "effective" URL.
					continue
				}
			}

			// Case 2: Navigation immediately following an interaction (consequential)
			// If we just clicked or typed, and staying on the same domain, it's likely a result of that action
			if isInteractiveAction(prev.ActionType) {
				if e.isSameDomain(curr.Value, currentURL) {
					// Exception: If it's a completely new path segment, maybe keep it?
					// But usually, if I type "cats" and hit enter, the URL changes to /search?q=cats.
					// We want to capture "type cats" + "press enter", NOT "navigate to /search?q=cats".
					continue
				}
			}

			// Update current URL if we decided to keep this navigation
			currentURL = curr.Value
		}

		result = append(result, curr)
	}

	return result
}

// isInteractiveAction returns true if the action is a user interaction
func isInteractiveAction(t models.ActionType) bool {
	switch t {
	case models.ActionClick, models.ActionDblClick, models.ActionInput, models.ActionKeypress, models.ActionSubmit, models.ActionBlur, models.ActionFocus:
		return true
	}
	return false
}

// isSameDomain checks if two URLs belong to the same domain
func (e *Extractor) isSameDomain(url1, url2 string) bool {
	// Simple domain extraction
	d1 := extractDomain(url1)
	d2 := extractDomain(url2)
	return d1 != "" && d2 != "" && d1 == d2
}

func extractDomain(u string) string {
	// Remove protocol
	u = strings.TrimPrefix(u, "https://")
	u = strings.TrimPrefix(u, "http://")
	// Get domain part
	parts := strings.SplitN(u, "/", 2)
	return parts[0]
}

// normalizeURL removes tracking parameters from URLs for comparison
func (e *Extractor) normalizeURL(url string) string {
	// Remove common tracking parameters
	trackingParams := []string{"utm_", "fbclid", "gclid", "ref", "source", "sxsrf", "ved", "ei"}

	parts := strings.SplitN(url, "?", 2)
	if len(parts) == 1 {
		return url
	}

	baseURL := parts[0]
	queryString := parts[1]

	// Parse and filter query parameters
	params := strings.Split(queryString, "&")
	var filtered []string
	for _, param := range params {
		key := strings.SplitN(param, "=", 2)[0]
		isTracking := false
		for _, tp := range trackingParams {
			if strings.HasPrefix(key, tp) {
				isTracking = true
				break
			}
		}
		if !isTracking {
			filtered = append(filtered, param)
		}
	}

	if len(filtered) == 0 {
		return baseURL
	}
	return baseURL + "?" + strings.Join(filtered, "&")
}

// debounceInputs combines consecutive input actions on the same element
func (e *Extractor) debounceInputs(actions []models.SemanticAction) []models.SemanticAction {
	if len(actions) == 0 {
		return actions
	}

	var result []models.SemanticAction

	for i := 0; i < len(actions); i++ {
		curr := actions[i]

		// If this is an input action, look ahead for more inputs on the same element
		if curr.ActionType == models.ActionInput && i < len(actions)-1 {
			for j := i + 1; j < len(actions); j++ {
				next := actions[j]
				if next.ActionType == models.ActionInput && next.Target.Selector == curr.Target.Selector {
					// Take the later value (final input)
					curr.Value = next.Value
					i = j
				} else {
					break
				}
			}
		}

		result = append(result, curr)
	}

	return result
}

// enrichSelectors improves selectors with semantic information
func (e *Extractor) enrichSelectors(actions []models.SemanticAction) []models.SemanticAction {
	for i := range actions {
		action := &actions[i]

		// Try to fetch latest node info from registry if available
		// This helps if the node was registered after the event (e.g. out-of-order processing)
		// or if we simply want the most complete attribute set
		if action.Target.NodeID > 0 {
			if node := e.parser.GetNode(action.Target.NodeID); node != nil {
				// Only update if node info is meaningful (e.g. not a text node overwriting our fallback)
				if node.TagName != "" {
					action.Target.Tag = node.TagName
					action.Target.Attributes = node.Attributes
				}
			}
		}

		// Skip window/document targets
		if action.Target.Selector == "window" || action.Target.Selector == "" {
			// One last try: if we have node info now, generate it
			if action.Target.Tag != "" && len(action.Target.Attributes) > 0 {
				robustSelector := e.generateRobustSelector(action.Target)
				if robustSelector != "" {
					action.Target.Selector = robustSelector
				}
			}
			continue
		}

		// Fallback for empty tags
		if action.Target.Tag == "" {
			switch action.ActionType {
			case models.ActionInput:
				action.Target.Tag = "input"
			case models.ActionClick, models.ActionDblClick:
				// If generic click, maybe it's a div/button
				// But check attributes?
				action.Target.Tag = "element" // Generic fallback
			case models.ActionCopy, models.ActionPaste:
				// Usually input or text area
				action.Target.Tag = "element"
			}
		}

		// Try to generate a more robust selector
		robustSelector := e.generateRobustSelector(action.Target)
		if robustSelector != "" && robustSelector != action.Target.Selector {
			action.Target.Selector = robustSelector
		}
	}

	return actions
}

// generateRobustSelector creates a selector that's more likely to work across runs
func (e *Extractor) generateRobustSelector(target models.SemanticTarget) string {
	attrs := target.Attributes
	tag := strings.ToLower(target.Tag)

	// Priority 1: Accessibility attributes (most stable)
	if ariaLabel, ok := attrs["aria-label"].(string); ok && ariaLabel != "" {
		return fmt.Sprintf("%s[aria-label='%s']", tag, escapeAttrValue(ariaLabel))
	}

	// Priority 2: Name attribute (stable for forms)
	if name, ok := attrs["name"].(string); ok && name != "" {
		return fmt.Sprintf("%s[name='%s']", tag, escapeAttrValue(name))
	}

	// Priority 3: Placeholder (stable for inputs)
	if placeholder, ok := attrs["placeholder"].(string); ok && placeholder != "" {
		return fmt.Sprintf("%s[placeholder='%s']", tag, escapeAttrValue(placeholder))
	}

	// Priority 4: Data attributes (often stable)
	for key, value := range attrs {
		if strings.HasPrefix(key, "data-") && !containsNumbersAndLetters(key) {
			if strVal, ok := value.(string); ok && strVal != "" && len(strVal) < 50 {
				return fmt.Sprintf("%s[%s='%s']", tag, key, escapeAttrValue(strVal))
			}
		}
	}

	// Priority 5: ID (if it doesn't look dynamic)
	if id, ok := attrs["id"].(string); ok && id != "" && !containsNumbersAndLetters(id) {
		return "#" + id
	}

	// Priority 6: Class (filter out dynamic classes)
	if class, ok := attrs["class"].(string); ok && class != "" {
		staticClass := e.extractStaticClass(class)
		if staticClass != "" {
			return "." + staticClass
		}
	}

	// Fallback: Use the original selector
	return target.Selector
}

// extractStaticClass finds a non-dynamic class from a class string
func (e *Extractor) extractStaticClass(classStr string) string {
	classes := strings.Fields(classStr)

	// Patterns that suggest dynamic classes
	dynamicPatterns := []*regexp.Regexp{
		regexp.MustCompile(`^[a-z]+-[a-f0-9]{6,}$`), // hash-based
		regexp.MustCompile(`^css-[a-z0-9]+$`),       // CSS-in-JS
		regexp.MustCompile(`^_[a-zA-Z0-9]+$`),       // minified
		regexp.MustCompile(`[0-9]{3,}`),             // contains many numbers
	}

	for _, class := range classes {
		class = strings.TrimSpace(class)
		if class == "" {
			continue
		}

		// Check if class looks dynamic
		isDynamic := false
		for _, pattern := range dynamicPatterns {
			if pattern.MatchString(class) {
				isDynamic = true
				break
			}
		}

		if !isDynamic && len(class) > 2 && len(class) < 30 {
			return class
		}
	}

	return ""
}

// filterLowValueActions removes actions that don't contribute meaningfully
func (e *Extractor) filterLowValueActions(actions []models.SemanticAction) []models.SemanticAction {
	var result []models.SemanticAction

	for _, action := range actions {
		// ALways drop inputs without selectors (useless for automation and likely duplicates of Custom Events)
		if action.ActionType == models.ActionInput && action.Target.Selector == "" {
			continue
		}

		// Drop empty tags (if they are still empty after enrichment fallback)
		// Exception: Navigation targets "window" which has no tag
		if action.Target.Tag == "" && action.ActionType != models.ActionNavigate {
			continue
		}

		// Drop media actions as per user request
		if action.ActionType == models.ActionMediaPlay ||
			action.ActionType == models.ActionMediaPause ||
			action.ActionType == models.ActionMediaSeek {
			continue
		}

		// Filter Focus and Blur actions as they are often redundant and cause noise
		if action.ActionType == models.ActionFocus || action.ActionType == models.ActionBlur {
			continue
		}

		// High Tolerance: Keep everything
		if e.tolerance == ToleranceHigh {
			result = append(result, action)
			continue
		}

		// Medium Tolerance (Default): RankHigh + RankMedium
		if e.tolerance == ToleranceMedium {
			if action.InteractionRank == models.RankHigh || action.InteractionRank == models.RankMedium {
				result = append(result, action)
				continue
			}
		}

		// Low Tolerance: RankHigh only
		if e.tolerance == ToleranceLow {
			if action.InteractionRank == models.RankHigh {
				result = append(result, action)
				continue
			}
		}
	}

	return result
}

// resequence reassigns sequence IDs after filtering
func (e *Extractor) resequence(actions []models.SemanticAction) []models.SemanticAction {
	for i := range actions {
		actions[i].SequenceID = i + 1
	}
	return actions
}

// containsNumbersAndLetters checks if a string contains both numbers and letters
// suggesting it might be a dynamic/generated value
func containsNumbersAndLetters(s string) bool {
	hasLetter := false
	hasNumber := false
	for _, r := range s {
		if unicode.IsLetter(r) {
			hasLetter = true
		}
		if unicode.IsNumber(r) {
			hasNumber = true
		}
		if hasLetter && hasNumber {
			return true
		}
	}
	return false
}

// escapeAttrValue escapes a value for use in CSS attribute selectors
func escapeAttrValue(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "'", "\\'")
	return s
}

// ValueClassifier determines the semantic category of a value
type ValueClassifier interface {
	ClassifyValue(ctx context.Context, value string) (string, error)
}

// IdentifyVariableTokens analyzes actions to detect variable vs fixed tokens
func (e *Extractor) IdentifyVariableTokens(ctx context.Context, actions []models.SemanticAction, classifier ValueClassifier) []models.WorkflowParameter {
	var params []models.WorkflowParameter
	seenValues := make(map[string]bool)

	for _, action := range actions {
		if action.ActionType != models.ActionInput || action.Value == "" {
			continue
		}

		// Skip if we've already processed this value
		if seenValues[action.Value] {
			continue
		}
		seenValues[action.Value] = true

		// Analyze the input value
		tokenType := e.classifyToken(action.Value)
		if tokenType == models.TokenVariable {
			paramName := e.generateParamName(ctx, action, classifier)
			param := models.WorkflowParameter{
				Name:         paramName,
				Type:         e.inferParamType(action.Value),
				DefaultValue: action.Value,
				TokenType:    models.TokenVariable,
				Required:     true,
				SourceAction: action.SequenceID,
			}
			params = append(params, param)
		}
	}

	return params
}

// classifyToken determines if a value is variable or fixed
func (e *Extractor) classifyToken(value string) models.TokenType {
	// Fixed tokens: Empty, single characters, common commands
	if len(value) <= 1 {
		return models.TokenFixed
	}

	// Fixed: Enter, Tab, Escape (keyboard commands)
	fixedKeys := []string{"Enter", "Tab", "Escape", "Backspace", "Delete"}
	for _, key := range fixedKeys {
		if value == key {
			return models.TokenFixed
		}
	}

	// Variable: Longer text inputs that look like user data
	// This is a heuristic - most user-typed content longer than a few chars is variable
	if len(value) > 3 {
		return models.TokenVariable
	}

	return models.TokenFixed
}

// generateParamName creates a parameter name from an action
func (e *Extractor) generateParamName(ctx context.Context, action models.SemanticAction, classifier ValueClassifier) string {
	// 1. Try semantic classification of the actual value first (user preference)
	if classifier != nil && action.Value != "" {
		if category, err := classifier.ClassifyValue(ctx, action.Value); err == nil && category != "input" && category != "" {
			return category // e.g., "email", "searchQuery", "username"
		}
	}

	// 2. Use placeholder or aria-label if available
	if placeholder, ok := action.Target.Attributes["placeholder"].(string); ok && placeholder != "" {
		return toCamelCase(placeholder)
	}
	if ariaLabel, ok := action.Target.Attributes["aria-label"].(string); ok && ariaLabel != "" {
		return toCamelCase(ariaLabel)
	}
	if name, ok := action.Target.Attributes["name"].(string); ok && name != "" {
		return toCamelCase(name)
	}

	// 3. Use target text context
	if action.Target.Text != "" {
		return toCamelCase(action.Target.Text)
	}

	// 4. Fallback
	return fmt.Sprintf("input%d", action.SequenceID)
}

// inferParamType guesses the parameter type from the value
func (e *Extractor) inferParamType(value string) models.ParameterType {
	// Email detection
	if strings.Contains(value, "@") && strings.Contains(value, ".") {
		return models.ParamTypeEmail
	}

	// URL detection
	if strings.HasPrefix(value, "http://") || strings.HasPrefix(value, "https://") {
		return models.ParamTypeURL
	}

	// Number detection
	if regexp.MustCompile(`^\d+$`).MatchString(value) {
		return models.ParamTypeNumber
	}

	return models.ParamTypeString
}

// toCamelCase converts a string to camelCase
func toCamelCase(s string) string {
	// Remove non-alphanumeric characters
	s = regexp.MustCompile(`[^a-zA-Z0-9\s]`).ReplaceAllString(s, " ")

	words := strings.Fields(s)
	if len(words) == 0 {
		return "param"
	}

	for i := range words {
		if i == 0 {
			words[i] = strings.ToLower(words[i])
		} else {
			words[i] = strings.Title(strings.ToLower(words[i]))
		}
	}

	result := strings.Join(words, "")
	// Truncate if too long
	if len(result) > 30 {
		result = result[:30]
	}

	return result
}
