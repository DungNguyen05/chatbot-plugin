// Copyright (c) 2023-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package erp_modules

import (
	"fmt"
)

// ModuleRouter handles routing intents to appropriate modules
type ModuleRouter struct {
	registry ModuleRegistry
	analyzer IntentAnalyzer
}

// NewModuleRouter creates a new module router
func NewModuleRouter(registry ModuleRegistry, analyzer IntentAnalyzer) *ModuleRouter {
	return &ModuleRouter{
		registry: registry,
		analyzer: analyzer,
	}
}

// RouteAndExecute analyzes user message and executes appropriate module
func (r *ModuleRouter) RouteAndExecute(ctx *ModuleContext, userMessage string) (*ModuleResponse, error) {
	// Analyze intent
	intent, err := r.analyzer.AnalyzeIntent(ctx.Context, userMessage, ctx.User)
	if err != nil {
		return &ModuleResponse{
			Success: false,
			Error:   fmt.Sprintf("Failed to analyze intent: %s", err.Error()),
		}, err
	}

	// Check confidence threshold
	if intent.Confidence < 0.6 {
		return &ModuleResponse{
			Success: false,
			Message: "Tôi không hiểu rõ yêu cầu của bạn. Bạn có thể nói rõ hơn không?",
			Error:   "Low confidence in intent analysis",
		}, nil
	}

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
