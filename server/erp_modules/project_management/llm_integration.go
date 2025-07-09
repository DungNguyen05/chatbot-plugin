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

// generateConfirmationMessage generates confirmation message using LLM
func (m *ProjectManagementModule) generateConfirmationMessage(ctx *erp_modules.ModuleContext, confirmationType string, data interface{}) (string, error) {
	// Create LLM context
	llmContext := &llm.Context{
		RequestingUser: ctx.User,
		Time:           time.Now().Format(time.RFC1123),
	}

	// Detect user language
	isVietnamese := detectUserLanguage(ctx.User)

	// Convert data to map for template access
	dataMap := make(map[string]interface{})
	dataBytes, _ := json.Marshal(data)
	json.Unmarshal(dataBytes, &dataMap)

	llmContext.Parameters = map[string]interface{}{
		"Type":         confirmationType,
		"Data":         dataMap,
		"IsVietnamese": isVietnamese,
	}

	// Use appropriate template based on type
	templateName := "project_confirmation_generation"
	if confirmationType == "task" {
		templateName = "task_confirmation_generation"
	}

	// Format the confirmation prompt
	systemPrompt, err := m.prompts.Format(templateName, llmContext)
	if err != nil {
		return "", fmt.Errorf("failed to format confirmation prompt: %w", err)
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
				Message: fmt.Sprintf("Generate confirmation message for %s creation", confirmationType),
			},
		},
		Context: llmContext,
	}

	// Get LLM response
	response, err := m.getLLM().ChatCompletionNoStream(completionRequest, llm.WithMaxGeneratedTokens(300))
	if err != nil {
		return "", fmt.Errorf("failed to generate confirmation with LLM: %w", err)
	}

	return strings.TrimSpace(response), nil
}

// parseConfirmationResponse parses user confirmation response using LLM - ENHANCED FOR DISAMBIGUATION
func (m *ProjectManagementModule) parseConfirmationResponse(ctx *erp_modules.ModuleContext, message string, pending *ProjectManagementConfirmation) (*UserResponse, error) {
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

// parseEmployeeDisambiguationResponse parses user response to employee selection using LLM - NEW
func (m *ProjectManagementModule) parseEmployeeDisambiguationResponse(ctx *erp_modules.ModuleContext, message string, disambiguation *EmployeeDisambiguationConfirmation) (*EmployeeDisambiguationResponse, error) {
	// Create LLM context
	llmContext := &llm.Context{
		RequestingUser: ctx.User,
		Time:           time.Now().Format(time.RFC1123),
	}

	// Detect user language
	isVietnamese := detectUserLanguage(ctx.User)

	llmContext.Parameters = map[string]interface{}{
		"UserMessage":        message,
		"AvailableEmployees": disambiguation.AvailableEmployees,
		"IsVietnamese":       isVietnamese,
	}

	// Format the disambiguation analysis prompt
	systemPrompt, err := m.prompts.Format("employee_disambiguation_analysis", llmContext)
	if err != nil {
		return nil, fmt.Errorf("failed to format employee disambiguation prompt: %w", err)
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
	response, err := m.getLLM().ChatCompletionNoStream(completionRequest, llm.WithMaxGeneratedTokens(200))
	if err != nil {
		return nil, fmt.Errorf("failed to analyze employee disambiguation with LLM: %w", err)
	}

	// Parse JSON response
	var disambiguationResponse EmployeeDisambiguationResponse
	response = strings.TrimSpace(response)
	start := strings.Index(response, "{")
	end := strings.LastIndex(response, "}") + 1

	if start == -1 || end <= start {
		return nil, fmt.Errorf("no valid JSON found in LLM response: %s", response)
	}

	jsonStr := response[start:end]
	if err := json.Unmarshal([]byte(jsonStr), &disambiguationResponse); err != nil {
		return nil, fmt.Errorf("failed to parse LLM disambiguation response as JSON: %w", err)
	}

	return &disambiguationResponse, nil
}

// analyzeProjectCreation uses LLM to extract project details
func (m *ProjectManagementModule) analyzeProjectCreation(ctx *erp_modules.ModuleContext, userMessage string) (*ProjectCreationRequest, error) {
	// Create LLM context
	llmContext := &llm.Context{
		RequestingUser: ctx.User,
		Time:           time.Now().Format(time.RFC1123),
	}
	llmContext.Parameters = map[string]interface{}{
		"UserMessage": userMessage,
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
	response, err := m.getLLM().ChatCompletionNoStream(completionRequest, llm.WithMaxGeneratedTokens(200))
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

// analyzeTaskCreation uses LLM to extract task details
func (m *ProjectManagementModule) analyzeTaskCreation(ctx *erp_modules.ModuleContext, userMessage string) (*TaskCreationRequest, error) {
	// Create LLM context
	llmContext := &llm.Context{
		RequestingUser: ctx.User,
		Time:           time.Now().Format(time.RFC1123),
	}
	llmContext.Parameters = map[string]interface{}{
		"UserMessage": userMessage,
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
	response, err := m.getLLM().ChatCompletionNoStream(completionRequest, llm.WithMaxGeneratedTokens(200))
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
