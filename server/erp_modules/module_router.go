// Copyright (c) 2023-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package erp_modules

import (
	"fmt"

	"github.com/mattermost/mattermost-plugin-ai/server/llm"
)

// ModuleRouter handles routing intents to appropriate modules with confidence-based logic
type ModuleRouter struct {
	registry    ModuleRegistry
	analyzer    *LLMIntentAnalyzer
	llmProvider func() llm.LanguageModel
	prompts     *llm.Prompts
}

// NewModuleRouter creates a new module router
func NewModuleRouter(registry ModuleRegistry, analyzer *LLMIntentAnalyzer, llmProvider func() llm.LanguageModel, prompts *llm.Prompts) *ModuleRouter {
	return &ModuleRouter{
		registry:    registry,
		analyzer:    analyzer,
		llmProvider: llmProvider,
		prompts:     prompts,
	}
}

// RouteAndExecute analyzes user message and executes appropriate module based on confidence
func (r *ModuleRouter) RouteAndExecute(ctx *ModuleContext, userMessage string) (*ModuleResponse, error) {
	// Analyze intent
	intent, err := r.analyzer.AnalyzeIntent(ctx.Context, userMessage, ctx.User)
	if err != nil {
		return &ModuleResponse{
			Success: false,
			Error:   fmt.Sprintf("Failed to analyze intent: %s", err.Error()),
		}, err
	}

	// Handle based on confidence level
	switch {
	case intent.Confidence >= 0.85:
		// High confidence - execute directly
		return r.executeModule(ctx, intent)

	case intent.Confidence >= 0.7:
		// Medium confidence - request confirmation
		confirmationMsg, err := r.analyzer.RequestConfirmation(ctx.Context, intent, ctx.User)
		if err != nil {
			return &ModuleResponse{
				Success: false,
				Error:   fmt.Sprintf("Failed to generate confirmation: %s", err.Error()),
			}, err
		}

		return &ModuleResponse{
			Success:     true,
			Message:     confirmationMsg,
			ActionTaken: "request_confirmation",
			Data: map[string]interface{}{
				"awaiting_confirmation": true,
				"confidence":            intent.Confidence,
				"category":              intent.Category,
				"action":                intent.Action,
			},
		}, nil

	default:
		// Low confidence - use general knowledge/fallback
		return r.handleGeneralQuery(ctx, userMessage)
	}
}

// executeModule executes the appropriate module for the given intent
func (r *ModuleRouter) executeModule(ctx *ModuleContext, intent *Intent) (*ModuleResponse, error) {
	// Route to appropriate module
	module, err := r.registry.RouteIntent(intent)
	if err != nil {
		return &ModuleResponse{
			Success: false,
			Message: "Tôi chưa hỗ trợ yêu cầu này. Vui lòng thử lại với yêu cầu khác.",
			Error:   err.Error(),
		}, err
	}

	// Execute module
	response, err := module.Execute(ctx, intent)
	if err != nil {
		return &ModuleResponse{
			Success: false,
			Error:   err.Error(),
		}, err
	}

	return response, nil
}

// handleGeneralQuery uses LLM to provide general knowledge response
func (r *ModuleRouter) handleGeneralQuery(ctx *ModuleContext, userMessage string) (*ModuleResponse, error) {
	// Create context for general response
	llmContext := llm.NewContext()
	llmContext.RequestingUser = ctx.User
	llmContext.Channel = ctx.Channel
	llmContext.Parameters = map[string]interface{}{
		"UserMessage": userMessage,
	}

	// Format system prompt for general response
	systemPrompt, err := r.prompts.Format("general_knowledge_response", llmContext)
	if err != nil {
		// Fallback prompt if template doesn't exist
		systemPrompt = `You are a helpful assistant. The user's message doesn't seem to be related to any specific system functions like attendance, tasks, or deadlines. Please provide a helpful, general response to their question or comment in Vietnamese if they wrote in Vietnamese, or in English if they wrote in English.

Be conversational and helpful, but also mention that if they need help with specific functions like attendance tracking, task management, or deadline checking, they can ask specifically about those topics.`
	}

	completionRequest := llm.CompletionRequest{
		Posts: []llm.Post{
			{
				Role:    llm.PostRoleSystem,
				Message: systemPrompt,
			},
			{
				Role:    llm.PostRoleUser,
				Message: userMessage,
			},
		},
		Context: llmContext,
	}

	// Get LLM response
	response, err := r.llmProvider().ChatCompletionNoStream(completionRequest, llm.WithMaxGeneratedTokens(300))
	if err != nil {
		return &ModuleResponse{
			Success: false,
			Message: "Xin lỗi, tôi không thể hiểu yêu cầu của bạn lúc này. Vui lòng thử lại sau.",
			Error:   err.Error(),
		}, err
	}

	return &ModuleResponse{
		Success:     true,
		Message:     response,
		ActionTaken: "general_response",
		Data: map[string]interface{}{
			"type": "general_knowledge",
		},
	}, nil
}

// GetAvailableModules returns information about all available modules
func (r *ModuleRouter) GetAvailableModules() map[string]interface{} {
	modules := r.registry.GetAllModules()
	result := make(map[string]interface{})

	for _, module := range modules {
		result[module.GetCategory()] = map[string]interface{}{
			"description": module.GetDescription(),
			"actions":     module.GetSupportedActions(),
			"examples":    module.GetActionExamples(),
		}
	}

	return result
}
