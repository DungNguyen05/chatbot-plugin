// server/erp_modules/project_management/enhanced_disambiguation.go
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

// EnhancedEmployeeDisambiguationResponse supports both index and name-based selection
type EnhancedEmployeeDisambiguationResponse struct {
	Intent          string   `json:"intent"`           // "index_selection", "name_selection", "cancel"
	SelectedIndexes []int    `json:"selected_indexes"` // For index-based selection
	SelectedNames   []string `json:"selected_names"`   // For name-based selection
	Reasoning       string   `json:"reasoning"`        // LLM reasoning
}

// parseEnhancedMultiEmployeeDisambiguationResponse parses both index and name-based selections
func (m *ProjectManagementModule) parseEnhancedMultiEmployeeDisambiguationResponse(ctx *erp_modules.ModuleContext, message string, unresolvedMatches []UnresolvedEmployeeMatch) (*EnhancedEmployeeDisambiguationResponse, error) {
	// Create LLM context
	llmContext := &llm.Context{
		RequestingUser: ctx.User,
		Time:           time.Now().Format(time.RFC1123),
	}

	// Detect user language
	isVietnamese := detectUserLanguage(ctx.User)

	// Build employee options for LLM analysis
	var allEmployeeOptions []map[string]interface{}
	globalIndex := 1
	for _, unresolvedMatch := range unresolvedMatches {
		for _, emp := range unresolvedMatch.MatchingEmployees {
			allEmployeeOptions = append(allEmployeeOptions, map[string]interface{}{
				"index":         globalIndex,
				"name":          emp.EmployeeName,
				"email":         emp.CompanyEmail,
				"employee_id":   emp.Name,
				"original_name": unresolvedMatch.OriginalName,
			})
			globalIndex++
		}
	}

	llmContext.Parameters = map[string]interface{}{
		"UserMessage":        message,
		"EmployeeOptions":    allEmployeeOptions,
		"IsVietnamese":       isVietnamese,
		"TotalEmployeeCount": len(allEmployeeOptions),
	}

	// Format the enhanced disambiguation analysis prompt
	systemPrompt, err := m.prompts.Format("enhanced_employee_disambiguation_analysis", llmContext)
	if err != nil {
		return nil, fmt.Errorf("failed to format enhanced employee disambiguation prompt: %w", err)
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
		return nil, fmt.Errorf("failed to analyze enhanced employee disambiguation with LLM: %w", err)
	}

	// Parse JSON response
	var disambiguationResponse EnhancedEmployeeDisambiguationResponse
	response = strings.TrimSpace(response)
	start := strings.Index(response, "{")
	end := strings.LastIndex(response, "}") + 1

	if start == -1 || end <= start {
		return nil, fmt.Errorf("no valid JSON found in LLM response: %s", response)
	}

	jsonStr := response[start:end]
	if err := json.Unmarshal([]byte(jsonStr), &disambiguationResponse); err != nil {
		return nil, fmt.Errorf("failed to parse LLM enhanced disambiguation response as JSON: %w", err)
	}

	return &disambiguationResponse, nil
}
