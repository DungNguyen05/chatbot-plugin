// Copyright (c) 2023-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package erp_modules

import (
	"context"

	"github.com/mattermost/mattermost-plugin-ai/server/llm"
	"github.com/mattermost/mattermost/server/public/model"
)

// Intent represents a user's parsed intention
type Intent struct {
	Category   string            `json:"category"`    // "attendance", "task", "deadline"
	Action     string            `json:"action"`      // "check_in", "check_out", "absent", "create", "list"
	Parameters map[string]string `json:"parameters"`  // Additional parameters
	Confidence float64           `json:"confidence"`  // Confidence score 0.0-1.0
	RawMessage string            `json:"raw_message"` // Original user message
}

// ModuleResponse represents the response from a module execution
type ModuleResponse struct {
	Success     bool                   `json:"success"`
	Message     string                 `json:"message"`
	Data        map[string]interface{} `json:"data,omitempty"`
	Error       string                 `json:"error,omitempty"`
	ActionTaken string                 `json:"action_taken,omitempty"`
}

// ModuleContext contains context information for module execution
type ModuleContext struct {
	Context      context.Context
	User         *model.User
	Channel      *model.Channel
	LLMContext   *llm.Context
	OriginalPost *model.Post
}

// ERPModule defines the interface that all ERP modules must implement
type ERPModule interface {
	// GetCategory returns the category this module handles (e.g., "attendance", "task")
	GetCategory() string

	// GetSupportedActions returns list of actions this module supports
	GetSupportedActions() []string

	// CanHandle determines if this module can handle the given intent
	CanHandle(intent *Intent) bool

	// Execute processes the intent and returns a response
	Execute(ctx *ModuleContext, intent *Intent) (*ModuleResponse, error)

	// GetDescription returns a description of what this module does (for LLM)
	GetDescription() string

	// GetActionExamples returns examples of user messages for each action (for LLM training)
	GetActionExamples() map[string][]string
}

// IntentAnalyzer interface for analyzing user messages
type IntentAnalyzer interface {
	AnalyzeIntent(ctx context.Context, message string, user *model.User) (*Intent, error)
}

// ModuleRegistry interface for managing modules
type ModuleRegistry interface {
	RegisterModule(module ERPModule) error
	GetModule(category string) (ERPModule, bool)
	GetAllModules() []ERPModule
	RouteIntent(intent *Intent) (ERPModule, error)
}
