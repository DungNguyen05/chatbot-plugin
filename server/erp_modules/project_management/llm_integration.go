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
func (m *ProjectManagementModule) generateConfirmationMessage(ctx *erp_modules.ModuleContext, confirmationType string, data map[string]interface{}) (string, error) {
	// Create LLM context
	llmContext := &llm.Context{
		RequestingUser: ctx.User,
		Time:           time.Now().Format(time.RFC1123),
	}

	// Detect user language
	isVietnamese := detectUserLanguage(ctx.User)

	llmContext.Parameters = map[string]interface{}{
		"Type":         confirmationType,
		"Data":         data,
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

// generateMultiEmployeeDisambiguationMessage generates message asking user to choose employees
func (m *ProjectManagementModule) generateMultiEmployeeDisambiguationMessage(ctx *erp_modules.ModuleContext, assigneeResult *AssigneeResolutionResult) (string, error) {
	isVietnamese := detectUserLanguage(ctx.User)

	var message strings.Builder

	// Show successfully resolved employees if any
	if len(assigneeResult.ResolvedEmployees) > 0 {
		if isVietnamese {
			message.WriteString("**Đã xác định thành công:**\n")
		} else {
			message.WriteString("**Successfully identified:**\n")
		}
		for _, emp := range assigneeResult.ResolvedEmployees {
			message.WriteString(fmt.Sprintf("- **%s** (%s)\n", emp.EmployeeName, emp.Email))
		}
		message.WriteString("\n")
	}

	// Show employees that need disambiguation
	if isVietnamese {
		message.WriteString("**Cần làm rõ cho các nhân viên sau:**\n\n")
	} else {
		message.WriteString("**Need clarification for the following employees:**\n\n")
	}

	globalIndex := 1
	for _, unresolvedMatch := range assigneeResult.UnresolvedEmployeeMatches {
		if isVietnamese {
			message.WriteString(fmt.Sprintf("**Tên '%s'** có thể là:\n", unresolvedMatch.OriginalName))
		} else {
			message.WriteString(fmt.Sprintf("**Name '%s'** could be:\n", unresolvedMatch.OriginalName))
		}

		for _, emp := range unresolvedMatch.MatchingEmployees {
			message.WriteString(fmt.Sprintf("%d. **%s** (%s, %s)\n",
				globalIndex,
				emp.EmployeeName,
				emp.CompanyEmail,
				emp.Name))
			globalIndex++
		}
		message.WriteString("\n")
	}

	// Enhanced selection instructions
	if isVietnamese {
		message.WriteString("**Bạn có thể chọn nhân viên bằng 2 cách:**\n\n")
		message.WriteString("**1. Chọn bằng số thứ tự:**\n")
		message.WriteString("   - Ví dụ: `1, 3, 5` để chọn nhân viên thứ 1, 3 và 5\n")
		message.WriteString("   - Ví dụ: `2, 7` để chọn nhân viên thứ 2 và 7\n\n")
		message.WriteString("**2. Chọn bằng tên cụ thể:**\n")
		message.WriteString("   - Ví dụ: `Phạm Tiến Đạt, Trung Đức` để chọn các nhân viên này\n")
		message.WriteString("   - Ví dụ: `Duy Anh, Anh Tài Phan` để chọn các nhân viên này\n\n")
		message.WriteString("Hoặc trả lời `hủy` để hủy bỏ yêu cầu.")
	} else {
		message.WriteString("**You can select employees in 2 ways:**\n\n")
		message.WriteString("**1. Select by numbers:**\n")
		message.WriteString("   - Example: `1, 3, 5` to select employees 1, 3, and 5\n")
		message.WriteString("   - Example: `2, 7` to select employees 2 and 7\n\n")
		message.WriteString("**2. Select by specific names:**\n")
		message.WriteString("   - Example: `Phạm Tiến Đạt, Trung Đức` to select these employees\n")
		message.WriteString("   - Example: `Duy Anh, Anh Tài Phan` to select these employees\n\n")
		message.WriteString("Or reply `cancel` to cancel the request.")
	}

	return message.String(), nil
}

// parseMultiEmployeeDisambiguationResponse parses user response to multi-employee selection using LLM
func (m *ProjectManagementModule) parseMultiEmployeeDisambiguationResponse(ctx *erp_modules.ModuleContext, message string, unresolvedMatches []UnresolvedEmployeeMatch) (*MultiEmployeeDisambiguationResponse, error) {
	// Use enhanced parsing
	enhancedResponse, err := m.parseEnhancedMultiEmployeeDisambiguationResponse(ctx, message, unresolvedMatches)
	if err != nil {
		return nil, err
	}

	// Convert to old format for backward compatibility
	oldResponse := &MultiEmployeeDisambiguationResponse{
		Intent:    enhancedResponse.Intent,
		Reasoning: enhancedResponse.Reasoning,
	}

	// Handle different intents
	switch enhancedResponse.Intent {
	case "index_selection":
		oldResponse.SelectedIndexes = enhancedResponse.SelectedIndexes
	case "name_selection":
		// For name selection, we need to resolve names to indexes
		// This is handled by the new resolveEmployeesByNames method
		oldResponse.Intent = "index_selection" // Convert to index selection for compatibility
		// The actual resolution will be handled in the workflow engine
	case "cancel":
		oldResponse.Intent = "cancel"
		oldResponse.SelectedIndexes = []int{}
	}

	return oldResponse, nil
}

// analyzeProjectCreation uses LLM to extract project details with multi-employee support
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

// analyzeTaskCreation uses LLM to extract task details with multi-employee support
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

// getOriginalSchema returns the original schema for the given type with multi-employee support
func (m *ProjectManagementModule) getOriginalSchema(confirmationType string) map[string]interface{} {
	if confirmationType == "project" {
		return map[string]interface{}{
			"project_name":        "",
			"description":         "",
			"priority":            "",
			"project_type":        "",
			"expected_start_date": "",
			"expected_end_date":   "",
			"department":          "",
			"customer":            "",
			"assigned_to_names":   []string{},
		}
	} else if confirmationType == "task" {
		return map[string]interface{}{
			"subject":           "",
			"description":       "",
			"priority":          "",
			"project":           "",
			"assigned_to_names": []string{},
			"exp_start_date":    "",
			"exp_end_date":      "",
			"department":        "",
		}
	}
	return make(map[string]interface{})
}
