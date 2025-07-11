// server/erp_modules/project_management/enhanced_entity_disambiguation.go
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

// EnhancedEntityDisambiguationResponse supports both index and name-based selection for projects/tasks
type EnhancedEntityDisambiguationResponse struct {
	Intent        string `json:"intent"`         // "index_selection", "name_selection", "create_new", "cancel"
	SelectedIndex int    `json:"selected_index"` // For index-based selection (1-based)
	SelectedName  string `json:"selected_name"`  // For name-based selection
	Reasoning     string `json:"reasoning"`      // LLM reasoning
}

// parseEnhancedExistingEntityDisambiguationResponse parses enhanced entity disambiguation
func (m *ProjectManagementModule) parseEnhancedExistingEntityDisambiguationResponse(
	ctx *erp_modules.ModuleContext,
	message, entityType string,
	existingEntities []interface{},
) (*EnhancedEntityDisambiguationResponse, error) {
	// Create LLM context
	llmContext := &llm.Context{
		RequestingUser: ctx.User,
		Time:           time.Now().Format(time.RFC1123),
	}

	isVietnamese := detectUserLanguage(ctx.User)

	// Build entity options for LLM analysis
	var entityOptions []map[string]interface{}
	for i, entity := range existingEntities {
		var entityName, entityID, entityStatus string

		if entityType == "project" {
			if projectBytes, err := json.Marshal(entity); err == nil {
				var project Project
				if json.Unmarshal(projectBytes, &project) == nil && project.ProjectName != "" {
					entityName = project.ProjectName
					entityID = project.Name
					entityStatus = project.Status
				} else {
					var task Task
					if json.Unmarshal(projectBytes, &task) == nil && task.Subject != "" {
						entityName = task.Subject
						entityID = task.Name
						entityStatus = task.Status
					}
				}
			}
		} else {
			if taskBytes, err := json.Marshal(entity); err == nil {
				var task Task
				if json.Unmarshal(taskBytes, &task) == nil && task.Subject != "" {
					entityName = task.Subject
					entityID = task.Name
					entityStatus = task.Status
				}
			}
		}

		entityOptions = append(entityOptions, map[string]interface{}{
			"index":  i + 1,
			"name":   entityName,
			"id":     entityID,
			"status": entityStatus,
		})
	}

	llmContext.Parameters = map[string]interface{}{
		"UserMessage":   message,
		"EntityType":    entityType,
		"EntityOptions": entityOptions,
		"EntityCount":   len(existingEntities),
		"IsVietnamese":  isVietnamese,
	}

	// Format the enhanced disambiguation analysis prompt
	systemPrompt, err := m.prompts.Format("enhanced_entity_disambiguation_analysis", llmContext)
	if err != nil {
		return nil, fmt.Errorf("failed to format enhanced entity disambiguation prompt: %w", err)
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
		return nil, fmt.Errorf("failed to analyze enhanced entity disambiguation with LLM: %w", err)
	}

	// Parse JSON response
	var disambiguationResponse EnhancedEntityDisambiguationResponse
	response = strings.TrimSpace(response)
	start := strings.Index(response, "{")
	end := strings.LastIndex(response, "}") + 1

	if start == -1 || end <= start {
		return nil, fmt.Errorf("no valid JSON found in LLM response: %s", response)
	}

	jsonStr := response[start:end]
	if err := json.Unmarshal([]byte(jsonStr), &disambiguationResponse); err != nil {
		return nil, fmt.Errorf("failed to parse LLM enhanced entity disambiguation response as JSON: %w", err)
	}

	return &disambiguationResponse, nil
}
