package llm

import (
	"encoding/json"
	"fmt"

	"dev/bravebird/browser-automation-go/pkg/models"
)

// SystemPrompt returns the verbose system prompt for browser automation code generation
const SystemPromptTemplate = `You are an expert Golang automation engineer specializing in Go Rod browser automation.
Your task is to generate robust, production-ready Go code to execute browser actions.

## CRITICAL RULES FOR CODE GENERATION

### 1. SELECTOR GENERATION PRIORITY (Most Important)
The provided selector may be brittle. Analyze the target attributes and generate a robust selector:

**Priority Order:**
1. **Accessibility Attributes (Best)**: aria-label, aria-placeholder, role
   - Example: input[aria-label='Search Reddit']
   
2. **Form Attributes (Good)**: name, placeholder, title
   - Example: input[name='q'] or input[placeholder='Search...']
   
3. **Data Attributes**: data-testid, data-cy, data-* (if stable-looking)
   - Example: button[data-testid='submit-btn']
   
4. **Text Matching (For Buttons/Links)**: Use MustElementR with regex
   - Example: page.MustElementR("button", "Log In")
   
5. **ID (Only if stable)**: Avoid IDs with numbers/hashes
   - Good: #main-search
   - Bad: #search-a7f3d2

6. **Class (Last resort)**: Only use semantic class names
   - Good: .submit-button
   - Bad: .css-1n5ry8e (CSS-in-JS generated)

### 2. WAIT STRATEGIES
Always use appropriate wait strategies:
- Before click: .MustWaitVisible() or .MustWaitStable()
- Before input: .MustWaitVisible()
- After navigation: .MustWaitLoad() or .MustWaitIdle()
- For animations: .MustWaitStable()

### 3. INPUT HANDLING
- Clear existing text before typing: element.MustSelectAllText().MustInput("")
- For search fields: Consider pressing Enter after input
- Handle autocomplete dropdowns if they appear

### 4. ERROR HANDLING
- Check if elements exist before interacting
- Use timeouts appropriately
- Provide fallback selectors when possible

### 5. VARIABLE TOKEN HANDLING
When you see a value marked as a variable token, use the provided variable name instead of hardcoding:
- Input: searchQuery variable with value "cats"
- Code: element.MustInput(searchQuery) // NOT element.MustInput("cats")

## OUTPUT FORMAT
Return ONLY valid Go code. No markdown, no explanations.
The code should be a single block that can be executed directly.
Use the following pattern:

// Action: [description]
element := page.MustElement("selector").MustWaitVisible()
element.MustClick()

Or for inputs:
element := page.MustElement("selector").MustWaitVisible()
element.MustSelectAllText().MustInput(variableName)
`

// ActionPromptTemplate is the template for individual action prompts
const ActionPromptTemplate = `
Execute the following browser action:

**Action Type**: %s
**Sequence**: %d

**Target Element**:
- Tag: %s
- Selector: %s
- Text: %s
- Attributes: %s

**Value**: %s

**Current Page Context**:
- URL: %s
- Title: %s

%s

Generate the Go Rod code to execute this action. Remember:
1. Use the best available selector from the attributes
2. Include appropriate waits
3. If this is an input action with a variable value, use the variable name provided
4. Handle potential edge cases
`

// RetryPromptAddition is added when generating code for a retry attempt
const RetryPromptAddition = `
**RETRY ATTEMPT %d**
The previous attempt failed with error: %s

Please generate alternative code that:
1. Uses a different selector strategy
2. Adds additional waits if needed
3. Considers that the element might be in a different state
`

// BuildActionPrompt constructs the prompt for a single action
func BuildActionPrompt(action models.SemanticAction, ctx PageContext, retryCount int, lastError string) string {
	attrsJSON, _ := json.MarshalIndent(action.Target.Attributes, "", "  ")

	retrySection := ""
	if retryCount > 0 && lastError != "" {
		retrySection = fmt.Sprintf(RetryPromptAddition, retryCount, lastError)
	}

	return fmt.Sprintf(
		ActionPromptTemplate,
		action.ActionType,
		action.SequenceID,
		action.Target.Tag,
		action.Target.Selector,
		action.Target.Text,
		string(attrsJSON),
		action.Value,
		ctx.URL,
		ctx.Title,
		retrySection,
	)
}

// VariableTokenPrompt is used to identify variable tokens
const VariableTokenPrompt = `
Analyze the following browser actions and identify which input values are:
1. **Variable Tokens**: User-specific data that should be parameterized (e.g., search queries, usernames, passwords)
2. **Fixed Tokens**: Structural inputs that should remain constant (e.g., Enter key, Tab navigation)

For each variable token, provide:
- A descriptive camelCase parameter name
- The inferred data type (string, number, email, url)
- Whether it's required

**Actions to analyze:**
%s

**Output Format (JSON):**
{
  "parameters": [
    {
      "name": "searchQuery",
      "type": "string",
      "default_value": "original value from recording",
      "description": "Brief description",
      "required": true,
      "source_action": 1
    }
  ]
}

Analyze the actions and return the JSON.
`

// BuildVariableTokenPrompt constructs the prompt for variable token identification
func BuildVariableTokenPrompt(actions []models.SemanticAction) string {
	actionsJSON, _ := json.MarshalIndent(actions, "", "  ")
	return fmt.Sprintf(VariableTokenPrompt, string(actionsJSON))
}

// WorkflowPrompt is used to generate complete workflow code
const WorkflowPrompt = `
Generate a complete, production-ready Go function that executes the following browser automation workflow.

**Workflow Parameters:**
%s

**Semantic Actions:**
%s

**Requirements:**
1. Function signature: func ExecuteWorkflow(page *rod.Page, params WorkflowParams) error
2. Define WorkflowParams struct with all parameters
3. Include proper error handling with context
4. Add comments for each major step
5. Use robust selectors (prioritize aria-label, name, placeholder over dynamic classes)
6. Include appropriate waits between actions
7. Return descriptive errors on failure

**Important Notes:**
- Replace all variable token values with the corresponding parameter
- Handle navigation between pages correctly
- Consider race conditions and timing issues
- Add retry logic for flaky elements if needed

Generate the complete Go code:
`

// BuildWorkflowPrompt constructs the prompt for complete workflow generation
func BuildWorkflowPrompt(actions []models.SemanticAction, params []models.WorkflowParameter) string {
	paramsJSON, _ := json.MarshalIndent(params, "", "  ")
	actionsJSON, _ := json.MarshalIndent(actions, "", "  ")
	return fmt.Sprintf(WorkflowPrompt, string(paramsJSON), string(actionsJSON))
}

// NavigateTemplate returns Go code template for navigation
const NavigateTemplate = `// Navigate to %s
page.MustNavigate(%s).MustWaitLoad()
`

// ClickTemplate returns Go code template for clicking
const ClickTemplate = `// Click %s
page.MustElement(%s).MustWaitVisible().MustClick()
`

// InputTemplate returns Go code template for input
const InputTemplate = `// Input into %s
elem := page.MustElement(%s).MustWaitVisible()
elem.MustSelectAllText().MustInput(%s)
`

// KeypressTemplate returns Go code template for keypress
const KeypressTemplate = `// Press %s key
page.Keyboard.MustType(input.%s)
`

// DblClickTemplate returns Go code template for double click
const DblClickTemplate = `// Double click %s
page.MustElement(%s).MustWaitVisible().MustAny("dblclick")
`

// RightClickTemplate returns Go code template for right click
const RightClickTemplate = `// Right click %s
page.MustElement(%s).MustWaitVisible().MustClick("right")
`

// SelectTemplate returns Go code template for select text
const SelectTemplate = `// Select text %s
page.MustElement(%s).MustWaitVisible().MustSelectAllText()
`

// ScrollTemplate returns Go code template for scrolling
const ScrollTemplate = `// Scroll %s
page.MustElement(%s).MustWaitVisible().MustScrollIntoView()
`

// GenerateCodeFromAction generates simple Go code from an action without LLM
// This is a fallback when LLM is not available
func GenerateCodeFromAction(action models.SemanticAction, variables map[string]string) string {
	selector := fmt.Sprintf("%q", action.Target.Selector)
	value := action.Value

	// Check if value should be replaced with a variable
	for varName, varValue := range variables {
		if value == varValue {
			value = varName
			break
		}
	}
	if action.ActionType == models.ActionInput && value == action.Value {
		value = fmt.Sprintf("%q", value)
	}

	switch action.ActionType {
	case models.ActionNavigate:
		url := action.Value
		if urlVar, ok := variables[action.Value]; ok {
			return fmt.Sprintf(NavigateTemplate, action.Value, urlVar)
		}
		return fmt.Sprintf(NavigateTemplate, action.Value, fmt.Sprintf("%q", url))

	case models.ActionClick:
		desc := action.Target.Text
		if desc == "" {
			desc = action.Target.Selector
		}
		return fmt.Sprintf(ClickTemplate, desc, selector)

	case models.ActionInput:
		desc := action.Target.Selector
		return fmt.Sprintf(InputTemplate, desc, selector, value)

	case models.ActionKeypress:
		key := action.Value
		if key == "Enter" {
			return fmt.Sprintf(KeypressTemplate, key, "Enter")
		}
		return fmt.Sprintf(KeypressTemplate, key, key)

	case models.ActionDblClick:
		return fmt.Sprintf(DblClickTemplate, action.Target.Selector, selector)

	case models.ActionRightClick:
		return fmt.Sprintf(RightClickTemplate, action.Target.Selector, selector)

	case models.ActionSelect:
		return fmt.Sprintf(SelectTemplate, action.Value, selector)

	case models.ActionScroll:
		return fmt.Sprintf(ScrollTemplate, action.Target.Selector, selector)

	case models.ActionFocus:
		return fmt.Sprintf("// Focus %s\npage.MustElement(%s).MustWaitVisible().MustFocus()", action.Target.Selector, selector)

	case models.ActionBlur:
		return fmt.Sprintf("// Blur %s\npage.MustElement(%s).MustWaitVisible().MustBlur()", action.Target.Selector, selector)

	default:
		return fmt.Sprintf("// Unsupported action type: %s\n", action.ActionType)
	}
}
