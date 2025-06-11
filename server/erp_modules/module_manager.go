// Copyright (c) 2023-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package erp_modules

import (
	"context"
	"fmt"

	"github.com/mattermost/mattermost-plugin-ai/server/llm"
	"github.com/mattermost/mattermost/server/public/model"
)

// ModuleManager manages all ERP modules and handles user requests
type ModuleManager struct {
	registry ModuleRegistry
	analyzer IntentAnalyzer
	router   *ModuleRouter
}

// NewModuleManager creates a new module manager
func NewModuleManager(registry ModuleRegistry, analyzer IntentAnalyzer) *ModuleManager {
	router := NewModuleRouter(registry, analyzer)
	return &ModuleManager{
		registry: registry,
		analyzer: analyzer,
		router:   router,
	}
}

// ProcessUserRequest processes a user request and routes it to the appropriate module
func (m *ModuleManager) ProcessUserRequest(ctx context.Context, userMessage string, user *model.User, channel *model.Channel, originalPost *model.Post, llmContext *llm.Context) (*ModuleResponse, error) {
	// Create module context
	moduleContext := &ModuleContext{
		Context:      ctx,
		User:         user,
		Channel:      channel,
		LLMContext:   llmContext,
		OriginalPost: originalPost,
	}

	// Route and execute the request
	response, err := m.router.RouteAndExecute(moduleContext, userMessage)
	if err != nil {
		return nil, fmt.Errorf("failed to route and execute user request: %w", err)
	}

	return response, nil
}

// GetAvailableModules returns information about all available modules
func (m *ModuleManager) GetAvailableModules() map[string]interface{} {
	return m.router.GetAvailableModules()
}

// RegisterModule registers a new module
func (m *ModuleManager) RegisterModule(module ERPModule) error {
	return m.registry.RegisterModule(module)
}
