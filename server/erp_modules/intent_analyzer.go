// Copyright (c) 2023-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package erp_modules

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/mattermost/mattermost-plugin-ai/server/llm"
	"github.com/mattermost/mattermost/server/public/model"
)

// ModuleClassificationResult represents the result of module classification
type ModuleClassificationResult struct {
	Category   string  `json:"category"`
	Confidence float64 `json:"confidence"`
	Reasoning  string  `json:"reasoning"`
}

// ActionClassificationResult represents the result of action classification within a module
type ActionClassificationResult struct {
	Action     string  `json:"action"`
	Confidence float64 `json:"confidence"`
	Reasoning  string  `json:"reasoning"`
}

// LLMIntentAnalyzer uses LLM to analyze user intents with dynamic 2-step classification
type LLMIntentAnalyzer struct {
	llmProvider func() llm.LanguageModel
	prompts     *llm.Prompts
	registry    ModuleRegistry
}

// NewLLMIntentAnalyzer creates a new LLM-based intent analyzer
func NewLLMIntentAnalyzer(llmProvider func() llm.LanguageModel, prompts *llm.Prompts, registry ModuleRegistry) *LLMIntentAnalyzer {
	return &LLMIntentAnalyzer{
		llmProvider: llmProvider,
		prompts:     prompts,
		registry:    registry,
	}
}

func (a *LLMIntentAnalyzer) AnalyzeIntent(ctx context.Context, message string, user *model.User) (*Intent, error) {
	// Log the incoming message
	fmt.Printf("=== ANALYZING INTENT ===\n")
	fmt.Printf("User: %s\n", user.Username)
	fmt.Printf("Message: %s\n", message)

	// STEP 1: Classify module first
	fmt.Printf("STEP 1: Classifying module...\n")
	moduleResult, err := a.classifyModule(ctx, message, user)
	if err != nil {
		fmt.Printf("Failed to classify module: %v\n", err)
		return nil, fmt.Errorf("failed to classify module: %w", err)
	}

	fmt.Printf("Module classification result: %s (confidence: %.3f)\n", moduleResult.Category, moduleResult.Confidence)

	// If module confidence is too low, return general fallback
	if moduleResult.Confidence < 0.3 {
		fmt.Printf("Module confidence too low, returning general fallback\n")
		return &Intent{
			Category:   "general",
			Action:     "fallback",
			Confidence: moduleResult.Confidence,
			RawMessage: message,
		}, nil
	}

	// Check if this is general category
	if moduleResult.Category == "general" {
		return &Intent{
			Category:   "general",
			Action:     "fallback",
			Confidence: moduleResult.Confidence,
			RawMessage: message,
		}, nil
	}

	// STEP 2: Classify action within the detected module
	fmt.Printf("STEP 2: Classifying action within module '%s'...\n", moduleResult.Category)
	actionResult, err := a.classifyAction(ctx, message, user, moduleResult.Category)
	if err != nil {
		fmt.Printf("Failed to classify action: %v\n", err)
		return nil, fmt.Errorf("failed to classify action: %w", err)
	}

	fmt.Printf("Action classification result: %s (confidence: %.3f)\n", actionResult.Action, actionResult.Confidence)

	// Combine confidences - use minimum to be conservative
	finalConfidence := min(moduleResult.Confidence, actionResult.Confidence)

	// Create final intent
	intent := &Intent{
		Category:   moduleResult.Category,
		Action:     actionResult.Action,
		Confidence: finalConfidence,
		RawMessage: message,
	}

	fmt.Printf("Final Intent:\n")
	fmt.Printf("  Category: %s\n", intent.Category)
	fmt.Printf("  Action: %s\n", intent.Action)
	fmt.Printf("  Final Confidence: %.3f\n", intent.Confidence)
	fmt.Printf("=== END INTENT ANALYSIS ===\n")

	return intent, nil
}

// classifyModule performs step 1: module classification
func (a *LLMIntentAnalyzer) classifyModule(ctx context.Context, message string, user *model.User) (*ModuleClassificationResult, error) {
	// Build context with available modules information
	llmContext := a.buildModuleAnalysisContext(message, user)

	// Create completion request for module classification
	systemPrompt, err := a.prompts.Format("erp_module_classification", llmContext)
	if err != nil {
		return nil, fmt.Errorf("failed to format module classification prompt: %w", err)
	}

	completionRequest := llm.CompletionRequest{
		Posts: []llm.Post{
			{
				Role:    llm.PostRoleSystem,
				Message: systemPrompt,
			},
			{
				Role:    llm.PostRoleUser,
				Message: message,
			},
		},
		Context: llmContext,
	}

	// Get LLM response
	response, err := a.llmProvider().ChatCompletionNoStream(completionRequest, llm.WithMaxGeneratedTokens(200))
	if err != nil {
		return nil, fmt.Errorf("failed to classify module with LLM: %w", err)
	}

	fmt.Printf("Module classification LLM response: %s\n", response)

	// Parse JSON response
	moduleResult, err := a.parseModuleResponse(response)
	if err != nil {
		return nil, fmt.Errorf("failed to parse module response: %w", err)
	}

	return moduleResult, nil
}

// classifyAction performs step 2: action classification within a specific module
func (a *LLMIntentAnalyzer) classifyAction(ctx context.Context, message string, user *model.User, category string) (*ActionClassificationResult, error) {
	// Build context with specific module's actions
	llmContext := a.buildActionAnalysisContext(message, user, category)

	// Create completion request for action classification
	systemPrompt, err := a.prompts.Format("erp_action_classification", llmContext)
	if err != nil {
		return nil, fmt.Errorf("failed to format action classification prompt: %w", err)
	}

	completionRequest := llm.CompletionRequest{
		Posts: []llm.Post{
			{
				Role:    llm.PostRoleSystem,
				Message: systemPrompt,
			},
			{
				Role:    llm.PostRoleUser,
				Message: message,
			},
		},
		Context: llmContext,
	}

	// Get LLM response
	response, err := a.llmProvider().ChatCompletionNoStream(completionRequest, llm.WithMaxGeneratedTokens(250))
	if err != nil {
		return nil, fmt.Errorf("failed to classify action with LLM: %w", err)
	}

	fmt.Printf("Action classification LLM response: %s\n", response)

	// Parse JSON response
	actionResult, err := a.parseActionResponse(response)
	if err != nil {
		return nil, fmt.Errorf("failed to parse action response: %w", err)
	}

	return actionResult, nil
}

// buildModuleAnalysisContext creates LLM context for module classification
func (a *LLMIntentAnalyzer) buildModuleAnalysisContext(message string, user *model.User) *llm.Context {
	// Get all registered modules and their information
	modules := a.registry.GetAllModules()

	// Build module information dynamically
	moduleCategories := make([]string, 0, len(modules))
	moduleInformation := make([]map[string]interface{}, 0, len(modules))

	for _, module := range modules {
		category := module.GetCategory()
		moduleCategories = append(moduleCategories, category)

		moduleInformation = append(moduleInformation, map[string]interface{}{
			"category":    category,
			"description": module.GetDescription(),
			"actions":     module.GetSupportedActions(),
		})
	}

	// Always include "general" as an option for non-ERP messages
	moduleCategories = append(moduleCategories, "general")
	moduleInformation = append(moduleInformation, map[string]interface{}{
		"category":    "general",
		"description": "General conversation, greetings, or queries not related to specific ERP functions",
		"actions":     []string{"fallback"},
	})

	context := llm.NewContext()
	context.RequestingUser = user
	context.Parameters = map[string]interface{}{
		"UserMessage":       message,
		"ModuleCategories":  moduleCategories,
		"ModuleInformation": moduleInformation,
	}

	return context
}

// buildActionAnalysisContext creates LLM context for action classification within a specific module
func (a *LLMIntentAnalyzer) buildActionAnalysisContext(message string, user *model.User, category string) *llm.Context {
	// Get the specific module
	module, exists := a.registry.GetModule(category)
	if !exists {
		// Return context with empty module info if not found
		context := llm.NewContext()
		context.RequestingUser = user
		context.Parameters = map[string]interface{}{
			"UserMessage":       message,
			"ModuleCategory":    category,
			"ActionInformation": []map[string]interface{}{},
		}
		return context
	}

	// Build action information dynamically
	supportedActions := module.GetSupportedActions()
	actionExamples := module.GetActionExamples()

	actionInformation := make([]map[string]interface{}, 0, len(supportedActions))

	for _, action := range supportedActions {
		actionInfo := map[string]interface{}{
			"action":   action,
			"examples": []string{}, // Default empty examples
		}

		// Add examples if available
		if actionExamples != nil {
			if examples, ok := actionExamples[action]; ok {
				actionInfo["examples"] = examples
			}
		}

		actionInformation = append(actionInformation, actionInfo)
	}

	context := llm.NewContext()
	context.RequestingUser = user
	context.Parameters = map[string]interface{}{
		"UserMessage":       message,
		"ModuleCategory":    category,
		"ModuleDescription": module.GetDescription(),
		"ActionInformation": actionInformation,
	}

	return context
}

// parseModuleResponse parses LLM response into ModuleClassificationResult struct
func (a *LLMIntentAnalyzer) parseModuleResponse(response string) (*ModuleClassificationResult, error) {
	// Clean response to extract JSON
	response = strings.TrimSpace(response)

	// Try to find JSON in the response
	start := strings.Index(response, "{")
	end := strings.LastIndex(response, "}") + 1

	if start == -1 || end <= start {
		return nil, fmt.Errorf("no valid JSON found in module response: %s", response)
	}

	jsonStr := response[start:end]

	var moduleResult ModuleClassificationResult
	if err := json.Unmarshal([]byte(jsonStr), &moduleResult); err != nil {
		return nil, fmt.Errorf("failed to unmarshal module JSON: %w", err)
	}

	// Validate module result
	if err := a.validateModuleResult(&moduleResult); err != nil {
		return nil, fmt.Errorf("invalid module result: %w", err)
	}

	return &moduleResult, nil
}

// parseActionResponse parses LLM response into ActionClassificationResult struct
func (a *LLMIntentAnalyzer) parseActionResponse(response string) (*ActionClassificationResult, error) {
	// Clean response to extract JSON
	response = strings.TrimSpace(response)

	// Try to find JSON in the response
	start := strings.Index(response, "{")
	end := strings.LastIndex(response, "}") + 1

	if start == -1 || end <= start {
		return nil, fmt.Errorf("no valid JSON found in action response: %s", response)
	}

	jsonStr := response[start:end]

	var actionResult ActionClassificationResult
	if err := json.Unmarshal([]byte(jsonStr), &actionResult); err != nil {
		return nil, fmt.Errorf("failed to unmarshal action JSON: %w", err)
	}

	// Validate action result
	if err := a.validateActionResult(&actionResult); err != nil {
		return nil, fmt.Errorf("invalid action result: %w", err)
	}

	return &actionResult, nil
}

// validateModuleResult validates the parsed module result
func (a *LLMIntentAnalyzer) validateModuleResult(result *ModuleClassificationResult) error {
	if result.Category == "" {
		return fmt.Errorf("category cannot be empty")
	}

	if result.Confidence < 0 || result.Confidence > 1 {
		return fmt.Errorf("confidence must be between 0 and 1")
	}

	return nil
}

// validateActionResult validates the parsed action result
func (a *LLMIntentAnalyzer) validateActionResult(result *ActionClassificationResult) error {
	if result.Action == "" {
		return fmt.Errorf("action cannot be empty")
	}

	if result.Confidence < 0 || result.Confidence > 1 {
		return fmt.Errorf("confidence must be between 0 and 1")
	}

	return nil
}

// min returns the minimum of two float64 values
func min(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}
