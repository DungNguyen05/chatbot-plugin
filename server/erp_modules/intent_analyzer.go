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

// ConfirmationRequest represents a request awaiting user confirmation
type ConfirmationRequest struct {
	OriginalIntent  *Intent                `json:"original_intent"`
	ConfirmationMsg string                 `json:"confirmation_msg"`
	UserID          string                 `json:"user_id"`
	CreatedAt       int64                  `json:"created_at"`
	Context         map[string]interface{} `json:"context"`
}

// LLMIntentAnalyzer uses LLM to analyze user intents
type LLMIntentAnalyzer struct {
	llmProvider             func() llm.LanguageModel
	prompts                 *llm.Prompts
	registry                ModuleRegistry
	pendingConfirmations    map[string]*ConfirmationRequest // userID -> pending confirmation
	confirmationPromptCache map[string]string               // intent key -> cached prompt
}

// NewLLMIntentAnalyzer creates a new LLM-based intent analyzer
func NewLLMIntentAnalyzer(llmProvider func() llm.LanguageModel, prompts *llm.Prompts, registry ModuleRegistry) *LLMIntentAnalyzer {
	return &LLMIntentAnalyzer{
		llmProvider:             llmProvider,
		prompts:                 prompts,
		registry:                registry,
		pendingConfirmations:    make(map[string]*ConfirmationRequest),
		confirmationPromptCache: make(map[string]string),
	}
}

// AnalyzeIntent analyzes user message and returns parsed intent with precise confidence
func (a *LLMIntentAnalyzer) AnalyzeIntent(ctx context.Context, message string, user *model.User) (*Intent, error) {
	// Check if user has a pending confirmation
	if pending, exists := a.pendingConfirmations[user.Id]; exists {
		return a.handleConfirmationResponse(ctx, message, user, pending)
	}

	// Build context with available modules information
	llmContext := a.buildAnalysisContext(message, user)

	// Create completion request with enhanced prompt for precise confidence scoring
	systemPrompt, err := a.prompts.Format("erp_intent_analysis_precise", llmContext)
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
	response, err := a.llmProvider().ChatCompletionNoStream(completionRequest, llm.WithMaxGeneratedTokens(300))
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

// handleConfirmationResponse processes user's response to a confirmation request
func (a *LLMIntentAnalyzer) handleConfirmationResponse(ctx context.Context, message string, user *model.User, pending *ConfirmationRequest) (*Intent, error) {
	// Use LLM to analyze confirmation response
	confirmationContext := llm.NewContext()
	confirmationContext.RequestingUser = user
	confirmationContext.Parameters = map[string]interface{}{
		"UserMessage":     message,
		"OriginalIntent":  pending.OriginalIntent,
		"ConfirmationMsg": pending.ConfirmationMsg,
	}

	systemPrompt, err := a.prompts.Format("confirmation_analysis", confirmationContext)
	if err != nil {
		return nil, fmt.Errorf("failed to format confirmation analysis prompt: %w", err)
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
		Context: confirmationContext,
	}

	response, err := a.llmProvider().ChatCompletionNoStream(completionRequest, llm.WithMaxGeneratedTokens(150))
	if err != nil {
		return nil, fmt.Errorf("failed to analyze confirmation with LLM: %w", err)
	}

	// Parse confirmation response
	var confirmResult struct {
		Confirmed bool   `json:"confirmed"`
		Reasoning string `json:"reasoning"`
	}

	// Clean and parse JSON response
	response = strings.TrimSpace(response)
	start := strings.Index(response, "{")
	end := strings.LastIndex(response, "}") + 1

	if start == -1 || end <= start {
		// If can't parse, assume denial for safety
		delete(a.pendingConfirmations, user.Id)
		return &Intent{
			Category:   "general",
			Action:     "fallback",
			Confidence: 0.1,
			RawMessage: message,
		}, nil
	}

	jsonStr := response[start:end]
	if err := json.Unmarshal([]byte(jsonStr), &confirmResult); err != nil {
		// If can't parse, assume denial for safety
		delete(a.pendingConfirmations, user.Id)
		return &Intent{
			Category:   "general",
			Action:     "fallback",
			Confidence: 0.1,
			RawMessage: message,
		}, nil
	}

	// Clear pending confirmation
	delete(a.pendingConfirmations, user.Id)

	if confirmResult.Confirmed {
		// User confirmed, return the original intent with high confidence
		pending.OriginalIntent.Confidence = 0.98 // High confidence after confirmation
		return pending.OriginalIntent, nil
	} else {
		// User denied, return fallback intent
		return &Intent{
			Category:   "general",
			Action:     "fallback",
			Confidence: 0.1,
			RawMessage: message,
		}, nil
	}
}

// RequestConfirmation generates a confirmation message and stores the pending request
func (a *LLMIntentAnalyzer) RequestConfirmation(ctx context.Context, intent *Intent, user *model.User) (string, error) {
	// Generate confirmation message using LLM
	confirmationContext := llm.NewContext()
	confirmationContext.RequestingUser = user
	confirmationContext.Parameters = map[string]interface{}{
		"Intent":      intent,
		"Category":    intent.Category,
		"Action":      intent.Action,
		"Parameters":  intent.Parameters,
		"UserMessage": intent.RawMessage,
	}

	// Get module info for better confirmation message
	if module, exists := a.registry.GetModule(intent.Category); exists {
		confirmationContext.Parameters["ModuleDescription"] = module.GetDescription()

		if actionExamples := module.GetActionExamples(); actionExamples != nil {
			if examples, ok := actionExamples[intent.Action]; ok {
				confirmationContext.Parameters["ActionExamples"] = examples
			}
		}
	}

	systemPrompt, err := a.prompts.Format("confirmation_generation", confirmationContext)
	if err != nil {
		return "", fmt.Errorf("failed to format confirmation generation prompt: %w", err)
	}

	completionRequest := llm.CompletionRequest{
		Posts: []llm.Post{
			{
				Role:    llm.PostRoleSystem,
				Message: systemPrompt,
			},
			{
				Role:    llm.PostRoleUser,
				Message: intent.RawMessage,
			},
		},
		Context: confirmationContext,
	}

	confirmationMsg, err := a.llmProvider().ChatCompletionNoStream(completionRequest, llm.WithMaxGeneratedTokens(200))
	if err != nil {
		return "", fmt.Errorf("failed to generate confirmation message: %w", err)
	}

	confirmationMsg = strings.TrimSpace(confirmationMsg)

	// Store pending confirmation
	a.pendingConfirmations[user.Id] = &ConfirmationRequest{
		OriginalIntent:  intent,
		ConfirmationMsg: confirmationMsg,
		UserID:          user.Id,
		CreatedAt:       model.GetMillis(),
		Context:         confirmationContext.Parameters,
	}

	return confirmationMsg, nil
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

	return nil
}

// ClearPendingConfirmation removes any pending confirmation for a user
func (a *LLMIntentAnalyzer) ClearPendingConfirmation(userID string) {
	delete(a.pendingConfirmations, userID)
}

// HasPendingConfirmation checks if a user has a pending confirmation
func (a *LLMIntentAnalyzer) HasPendingConfirmation(userID string) bool {
	_, exists := a.pendingConfirmations[userID]
	return exists
}
