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

// LLMIntentAnalyzer uses LLM to analyze user intents
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

// AnalyzeIntent analyzes user message and returns parsed intent
func (a *LLMIntentAnalyzer) AnalyzeIntent(ctx context.Context, message string, user *model.User) (*Intent, error) {
	// Build context with available modules information
	llmContext := a.buildAnalysisContext(message, user)

	// Create completion request
	systemPrompt, err := a.prompts.Format("erp_intent_analysis", llmContext)
	if err != nil {
		return nil, fmt.Errorf("failed to format intent analysis prompt: %w", err)
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
		return nil, fmt.Errorf("failed to analyze intent with LLM: %w", err)
	}

	// Parse JSON response
	intent, err := a.parseIntentResponse(response, message)
	if err != nil {
		return nil, fmt.Errorf("failed to parse intent response: %w", err)
	}

	return intent, nil
}

// buildAnalysisContext creates LLM context with modules information
func (a *LLMIntentAnalyzer) buildAnalysisContext(message string, user *model.User) *llm.Context {
	// Get all registered modules and their information
	modules := a.registry.GetAllModules()

	moduleInfo := make(map[string]interface{})
	examples := make(map[string]interface{})

	for _, module := range modules {
		category := module.GetCategory()
		moduleInfo[category] = map[string]interface{}{
			"description": module.GetDescription(),
			"actions":     module.GetSupportedActions(),
		}
		examples[category] = module.GetActionExamples()
	}

	context := llm.NewContext()
	context.RequestingUser = user
	context.Parameters = map[string]interface{}{
		"UserMessage":      message,
		"AvailableModules": moduleInfo,
		"ActionExamples":   examples,
	}

	return context
}

// parseIntentResponse parses LLM response into Intent struct
func (a *LLMIntentAnalyzer) parseIntentResponse(response string, originalMessage string) (*Intent, error) {
	// Clean response to extract JSON
	response = strings.TrimSpace(response)

	// Try to find JSON in the response
	start := strings.Index(response, "{")
	end := strings.LastIndex(response, "}") + 1

	if start == -1 || end <= start {
		return nil, fmt.Errorf("no valid JSON found in response: %s", response)
	}

	jsonStr := response[start:end]

	var intent Intent
	if err := json.Unmarshal([]byte(jsonStr), &intent); err != nil {
		return nil, fmt.Errorf("failed to unmarshal intent JSON: %w", err)
	}

	intent.RawMessage = originalMessage

	// Validate intent
	if err := a.validateIntent(&intent); err != nil {
		return nil, fmt.Errorf("invalid intent: %w", err)
	}

	return &intent, nil
}

// validateIntent validates the parsed intent
func (a *LLMIntentAnalyzer) validateIntent(intent *Intent) error {
	if intent.Category == "" {
		return fmt.Errorf("category cannot be empty")
	}

	if intent.Action == "" {
		return fmt.Errorf("action cannot be empty")
	}

	if intent.Confidence < 0 || intent.Confidence > 1 {
		return fmt.Errorf("confidence must be between 0 and 1")
	}

	// Check if category exists in registry
	if _, exists := a.registry.GetModule(intent.Category); !exists {
		return fmt.Errorf("unknown category: %s", intent.Category)
	}

	return nil
}
