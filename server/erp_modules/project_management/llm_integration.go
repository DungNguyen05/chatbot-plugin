// Copyright (c) 2023-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package project_management

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/mattermost/mattermost-plugin-ai/server/erp_modules"
	"github.com/mattermost/mattermost-plugin-ai/server/llm"
)

// generateProjectConfirmationMessage generates confirmation message for project using LLM
func (m *ProjectManagementModule) generateProjectConfirmationMessage(ctx *erp_modules.ModuleContext, action string, projectData *ProjectCreationRequest) (string, error) {
	// Create LLM context
	llmContext := &llm.Context{
		RequestingUser: ctx.User,
		Time:           time.Now().Format(time.RFC1123),
	}

	// Detect user language
	isVietnamese := detectUserLanguage(ctx.User)

	llmContext.Parameters = map[string]interface{}{
		"Action":       action,
		"ProjectData":  projectData,
		"UserName":     getUserDisplayName(ctx.User),
		"IsVietnamese": isVietnamese,
	}

	// Format the confirmation prompt
	systemPrompt, err := m.prompts.Format("project_confirmation_generation", llmContext)
	if err != nil {
		return "", fmt.Errorf("failed to format project confirmation prompt: %w", err)
	}

	// Create completion request
	completionRequest := llm.CompletionRequest{
		Posts: []llm.Post{
			{
				Role:    llm.PostRoleSystem,
				Message: systemPrompt,
			},
			{
				Role:    llm.PostRoleUser,
				Message: fmt.Sprintf("Generate confirmation for %s project creation", action),
			},
		},
		Context: llmContext,
	}

	// Get LLM response
	response, err := m.getLLM().ChatCompletionNoStream(completionRequest, llm.WithMaxGeneratedTokens(300))
	if err != nil {
		return "", fmt.Errorf("failed to generate project confirmation with LLM: %w", err)
	}

	return strings.TrimSpace(response), nil
}

// generateTaskConfirmationMessage generates confirmation message for task using LLM
func (m *ProjectManagementModule) generateTaskConfirmationMessage(ctx *erp_modules.ModuleContext, action string, taskData *TaskCreationRequest) (string, error) {
	// Create LLM context
	llmContext := &llm.Context{
		RequestingUser: ctx.User,
		Time:           time.Now().Format(time.RFC1123),
	}

	// Detect user language
	isVietnamese := detectUserLanguage(ctx.User)

	llmContext.Parameters = map[string]interface{}{
		"Action":       action,
		"TaskData":     taskData,
		"UserName":     getUserDisplayName(ctx.User),
		"IsVietnamese": isVietnamese,
	}

	// Format the confirmation prompt
	systemPrompt, err := m.prompts.Format("task_confirmation_generation", llmContext)
	if err != nil {
		return "", fmt.Errorf("failed to format task confirmation prompt: %w", err)
	}

	// Create completion request
	completionRequest := llm.CompletionRequest{
		Posts: []llm.Post{
			{
				Role:    llm.PostRoleSystem,
				Message: systemPrompt,
			},
			{
				Role:    llm.PostRoleUser,
				Message: fmt.Sprintf("Generate confirmation for %s task creation", action),
			},
		},
		Context: llmContext,
	}

	// Get LLM response
	response, err := m.getLLM().ChatCompletionNoStream(completionRequest, llm.WithMaxGeneratedTokens(300))
	if err != nil {
		return "", fmt.Errorf("failed to generate task confirmation with LLM: %w", err)
	}

	return strings.TrimSpace(response), nil
}

// parseConfirmationResponse parses user confirmation response using LLM
func (m *ProjectManagementModule) parseConfirmationResponse(ctx *erp_modules.ModuleContext, message string) (bool, error) {
	// Create LLM context
	llmContext := &llm.Context{
		RequestingUser: ctx.User,
	}

	llmContext.Parameters = map[string]interface{}{
		"UserMessage": message,
	}

	// Format the confirmation analysis prompt
	systemPrompt, err := m.prompts.Format("project_confirmation_analysis", llmContext)
	if err != nil {
		return false, fmt.Errorf("failed to format confirmation analysis prompt: %w", err)
	}

	// Create completion request
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
	response, err := m.getLLM().ChatCompletionNoStream(completionRequest, llm.WithMaxGeneratedTokens(150))
	if err != nil {
		return false, fmt.Errorf("failed to analyze confirmation with LLM: %w", err)
	}

	// Parse JSON response
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
		return false, nil
	}

	jsonStr := response[start:end]
	if err := json.Unmarshal([]byte(jsonStr), &confirmResult); err != nil {
		// If can't parse, assume denial for safety
		return false, nil
	}

	return confirmResult.Confirmed, nil
}

// analyzeProjectCreation uses LLM to extract project details from user message
func (m *ProjectManagementModule) analyzeProjectCreation(ctx *erp_modules.ModuleContext, userMessage string) (*ProjectCreationRequest, error) {
	// Create LLM context
	llmContext := &llm.Context{
		RequestingUser: ctx.User,
		Time:           time.Now().Format(time.RFC1123),
	}

	// Detect user language
	isVietnamese := detectUserLanguage(ctx.User)

	llmContext.Parameters = map[string]interface{}{
		"UserMessage":  userMessage,
		"IsVietnamese": isVietnamese,
	}

	// Format the project creation analysis prompt
	systemPrompt, err := m.prompts.Format("project_creation_analysis", llmContext)
	if err != nil {
		return nil, fmt.Errorf("failed to format project creation analysis prompt: %w", err)
	}

	// Create completion request
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
	response, err := m.getLLM().ChatCompletionNoStream(completionRequest, llm.WithMaxGeneratedTokens(400))
	if err != nil {
		return nil, fmt.Errorf("failed to analyze project creation with LLM: %w", err)
	}

	// Parse JSON response
	var projectRequest ProjectCreationRequest

	// Clean response to extract JSON
	response = strings.TrimSpace(response)
	start := strings.Index(response, "{")
	end := strings.LastIndex(response, "}") + 1

	if start == -1 || end <= start {
		return nil, fmt.Errorf("no valid JSON found in LLM response: %s", response)
	}

	jsonStr := response[start:end]
	if err := json.Unmarshal([]byte(jsonStr), &projectRequest); err != nil {
		return nil, fmt.Errorf("failed to parse LLM response as JSON: %w", err)
	}

	return &projectRequest, nil
}

// analyzeTaskCreation uses LLM to extract task details from user message
func (m *ProjectManagementModule) analyzeTaskCreation(ctx *erp_modules.ModuleContext, userMessage string) (*TaskCreationRequest, error) {
	// Create LLM context
	llmContext := &llm.Context{
		RequestingUser: ctx.User,
		Time:           time.Now().Format(time.RFC1123),
	}

	// Detect user language
	isVietnamese := detectUserLanguage(ctx.User)

	llmContext.Parameters = map[string]interface{}{
		"UserMessage":  userMessage,
		"IsVietnamese": isVietnamese,
	}

	// Format the task creation analysis prompt
	systemPrompt, err := m.prompts.Format("task_creation_analysis", llmContext)
	if err != nil {
		return nil, fmt.Errorf("failed to format task creation analysis prompt: %w", err)
	}

	// Create completion request
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
	response, err := m.getLLM().ChatCompletionNoStream(completionRequest, llm.WithMaxGeneratedTokens(400))
	if err != nil {
		return nil, fmt.Errorf("failed to analyze task creation with LLM: %w", err)
	}

	// Parse JSON response
	var taskRequest TaskCreationRequest

	// Clean response to extract JSON
	response = strings.TrimSpace(response)
	start := strings.Index(response, "{")
	end := strings.LastIndex(response, "}") + 1

	if start == -1 || end <= start {
		return nil, fmt.Errorf("no valid JSON found in LLM response: %s", response)
	}

	jsonStr := response[start:end]
	if err := json.Unmarshal([]byte(jsonStr), &taskRequest); err != nil {
		return nil, fmt.Errorf("failed to parse LLM response as JSON: %w", err)
	}

	return &taskRequest, nil
}

// parseModificationResponse parses user modification response using LLM
func (m *ProjectManagementModule) parseModificationResponse(ctx *erp_modules.ModuleContext, message string, pending *ProjectConfirmation) (*UserResponse, error) {
	// Create LLM context
	llmContext := &llm.Context{
		RequestingUser: ctx.User,
		Time:           time.Now().Format(time.RFC1123),
	}

	llmContext.Parameters = map[string]interface{}{
		"UserMessage":    message,
		"PendingType":    pending.Type,
		"PendingData":    pending.Data,
		"OriginalSchema": m.getOriginalSchema(pending.Type),
	}

	// Use appropriate template based on type
	templateName := "project_modification_analysis"
	if pending.Type == "task" {
		templateName = "task_modification_analysis"
	}

	// Format the analysis prompt
	systemPrompt, err := m.prompts.Format(templateName, llmContext)
	if err != nil {
		return nil, fmt.Errorf("failed to format modification analysis prompt: %w", err)
	}

	// Create completion request
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
	response, err := m.getLLM().ChatCompletionNoStream(completionRequest, llm.WithMaxGeneratedTokens(300))
	if err != nil {
		return nil, fmt.Errorf("failed to analyze user response with LLM: %w", err)
	}

	// Parse JSON response
	var userResponse UserResponse
	response = strings.TrimSpace(response)
	start := strings.Index(response, "{")
	end := strings.LastIndex(response, "}") + 1

	if start == -1 || end <= start {
		return nil, fmt.Errorf("no valid JSON found in LLM response: %s", response)
	}

	jsonStr := response[start:end]
	if err := json.Unmarshal([]byte(jsonStr), &userResponse); err != nil {
		return nil, fmt.Errorf("failed to parse LLM response as JSON: %w", err)
	}

	return &userResponse, nil
}
