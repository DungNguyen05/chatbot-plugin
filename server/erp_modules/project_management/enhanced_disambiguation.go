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

// resolveEmployeesByNames resolves employees from name-based selection
func (m *ProjectManagementModule) resolveEmployeesByNames(
	unresolvedMatches []UnresolvedEmployeeMatch,
	selectedNames []string,
) ([]AssignedEmployee, error) {
	var resolvedEmployees []AssignedEmployee

	// Create a map of all available employees for easy lookup
	employeeByName := make(map[string]Employee)
	employeeByOriginalName := make(map[string]string) // Maps employee name to original search name

	for _, unresolvedMatch := range unresolvedMatches {
		for _, emp := range unresolvedMatch.MatchingEmployees {
			employeeByName[emp.EmployeeName] = emp
			employeeByOriginalName[emp.EmployeeName] = unresolvedMatch.OriginalName
		}
	}

	// Track which employees we've already added to avoid duplicates
	addedEmployees := make(map[string]bool)

	// Resolve each selected name
	for _, selectedName := range selectedNames {
		selectedName = strings.TrimSpace(selectedName)
		if selectedName == "" {
			continue
		}

		// Try exact match first
		if emp, exists := employeeByName[selectedName]; exists {
			if addedEmployees[emp.Name] {
				m.api.LogInfo("Skipping duplicate employee selection by name",
					"employee_name", emp.EmployeeName,
					"employee_id", emp.Name)
				continue
			}

			originalName := employeeByOriginalName[emp.EmployeeName]
			resolvedEmployees = append(resolvedEmployees, AssignedEmployee{
				EmployeeID:   emp.Name,
				EmployeeName: emp.EmployeeName,
				Email:        emp.CompanyEmail,
				OriginalName: originalName,
			})

			addedEmployees[emp.Name] = true

			m.api.LogInfo("Employee resolved by name",
				"selected_name", selectedName,
				"employee_id", emp.Name,
				"employee_name", emp.EmployeeName,
				"original_name", originalName)
		} else {
			// Try fuzzy matching if exact match fails
			var bestMatch Employee
			var bestMatchOriginal string
			bestScore := 0.0

			for _, unresolvedMatch := range unresolvedMatches {
				for _, emp := range unresolvedMatch.MatchingEmployees {
					score := calculateNameSimilarity(selectedName, emp.EmployeeName)
					if score > bestScore && score >= 0.7 { // 70% similarity threshold
						bestScore = score
						bestMatch = emp
						bestMatchOriginal = unresolvedMatch.OriginalName
					}
				}
			}

			if bestScore >= 0.7 && !addedEmployees[bestMatch.Name] {
				resolvedEmployees = append(resolvedEmployees, AssignedEmployee{
					EmployeeID:   bestMatch.Name,
					EmployeeName: bestMatch.EmployeeName,
					Email:        bestMatch.CompanyEmail,
					OriginalName: bestMatchOriginal,
				})

				addedEmployees[bestMatch.Name] = true

				m.api.LogInfo("Employee resolved by fuzzy name matching",
					"selected_name", selectedName,
					"matched_name", bestMatch.EmployeeName,
					"employee_id", bestMatch.Name,
					"similarity_score", bestScore,
					"original_name", bestMatchOriginal)
			} else {
				m.api.LogWarn("Could not resolve employee by name",
					"selected_name", selectedName,
					"best_score", bestScore)
			}
		}
	}

	if len(resolvedEmployees) == 0 {
		return nil, fmt.Errorf("no valid employees were resolved from the selected names")
	}

	m.api.LogInfo("Successfully resolved employees by names",
		"total_selected", len(resolvedEmployees),
		"input_names_count", len(selectedNames))

	return resolvedEmployees, nil
}

// calculateNameSimilarity calculates similarity between two names
func calculateNameSimilarity(name1, name2 string) float64 {
	name1 = strings.ToLower(strings.TrimSpace(name1))
	name2 = strings.ToLower(strings.TrimSpace(name2))

	if name1 == name2 {
		return 1.0
	}

	// Check if one name contains the other
	if strings.Contains(name2, name1) || strings.Contains(name1, name2) {
		return 0.9
	}

	// Check word-by-word similarity
	words1 := strings.Fields(name1)
	words2 := strings.Fields(name2)

	if len(words1) == 0 || len(words2) == 0 {
		return 0.0
	}

	var matchCount float64
	for _, word1 := range words1 {
		for _, word2 := range words2 {
			if strings.Contains(word2, word1) || strings.Contains(word1, word2) {
				matchCount++
				break
			}
		}
	}

	return matchCount / float64(len(words1))
}
