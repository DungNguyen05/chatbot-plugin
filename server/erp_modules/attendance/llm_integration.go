// Copyright (c) 2023-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package attendance

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/mattermost/mattermost-plugin-ai/server/erp_modules"
	"github.com/mattermost/mattermost-plugin-ai/server/llm"
)

// generateConfirmationMessage generates confirmation message using LLM
func (m *AttendanceModule) generateConfirmationMessage(ctx *erp_modules.ModuleContext, action, reason string) (string, error) {
	// Create LLM context
	llmContext := &llm.Context{
		RequestingUser: ctx.User,
		Time:           time.Now().Format(time.RFC1123),
	}

	// Detect user language
	isVietnamese := detectUserLanguage(ctx.User)

	llmContext.Parameters = map[string]interface{}{
		"Action":       action,
		"Reason":       reason,
		"UserName":     getUserDisplayName(ctx.User),
		"IsVietnamese": isVietnamese,
	}

	// Format the confirmation prompt
	systemPrompt, err := m.prompts.Format("attendance_confirmation_generation", llmContext)
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
				Message: fmt.Sprintf("Generate confirmation for %s action", action),
			},
		},
		Context: llmContext,
	}

	// Get LLM response
	response, err := m.getLLM().ChatCompletionNoStream(completionRequest, llm.WithMaxGeneratedTokens(200))
	if err != nil {
		return "", fmt.Errorf("failed to generate confirmation with LLM: %w", err)
	}

	return strings.TrimSpace(response), nil
}

// parseConfirmationResponse parses user confirmation response using LLM
func (m *AttendanceModule) parseConfirmationResponse(ctx *erp_modules.ModuleContext, message string) (bool, error) {
	// Create LLM context
	llmContext := &llm.Context{
		RequestingUser: ctx.User,
	}

	llmContext.Parameters = map[string]interface{}{
		"UserMessage": message,
	}

	// Format the confirmation analysis prompt
	systemPrompt, err := m.prompts.Format("attendance_confirmation_analysis", llmContext)
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

// analyzeAttendanceQuery uses LLM to convert natural language to structured request
func (m *AttendanceModule) analyzeAttendanceQuery(ctx *erp_modules.ModuleContext, userMessage string) (*AttendanceQueryRequest, error) {
	// Create LLM context
	llmContext := &llm.Context{
		RequestingUser: ctx.User,
		Time:           time.Now().Format(time.RFC1123),
	}
	llmContext.Parameters = map[string]interface{}{
		"UserMessage": userMessage,
	}

	// Format the unified analysis prompt
	systemPrompt, err := m.prompts.Format("attendance_unified_analysis", llmContext)
	if err != nil {
		return nil, fmt.Errorf("failed to format unified analysis prompt: %w", err)
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
	response, err := m.getLLM().ChatCompletionNoStream(completionRequest, llm.WithMaxGeneratedTokens(300))
	if err != nil {
		return nil, fmt.Errorf("failed to analyze attendance query with LLM: %w", err)
	}

	// Parse JSON response
	var queryRequest AttendanceQueryRequest

	// Clean response to extract JSON
	response = strings.TrimSpace(response)
	start := strings.Index(response, "{")
	end := strings.LastIndex(response, "}") + 1

	if start == -1 || end <= start {
		return nil, fmt.Errorf("no valid JSON found in LLM response: %s", response)
	}

	jsonStr := response[start:end]
	if err := json.Unmarshal([]byte(jsonStr), &queryRequest); err != nil {
		return nil, fmt.Errorf("failed to parse LLM response as JSON: %w", err)
	}

	return &queryRequest, nil
}

// analyzeAbsentReason uses LLM to extract the absence reason from user message
func (m *AttendanceModule) analyzeAbsentReason(ctx *erp_modules.ModuleContext, userMessage string) (string, error) {
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

	// Format the reason analysis prompt
	systemPrompt, err := m.prompts.Format("attendance_reason_analysis", llmContext)
	if err != nil {
		return "", fmt.Errorf("failed to format reason analysis prompt: %w", err)
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
	response, err := m.getLLM().ChatCompletionNoStream(completionRequest, llm.WithMaxGeneratedTokens(100))
	if err != nil {
		return "", fmt.Errorf("failed to analyze absence reason with LLM: %w", err)
	}

	// Clean and return the response
	reason := strings.TrimSpace(response)

	// Validate that we got a reasonable response
	if reason == "" {
		isVietnamese := detectUserLanguage(ctx.User)
		if isVietnamese {
			reason = "Việc cá nhân"
		} else {
			reason = "Personal matters"
		}
	}

	return reason, nil
}
