// Copyright (c) 2023-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package project_management

import (
	"fmt"
	"math"
	"strings"
)

// resolveAssignee resolves an assignee name to an employee ID and email with disambiguation support
func (m *ProjectManagementModule) resolveAssignee(assigneeName string) (*AssigneeResolutionResult, error) {
	// If no assignee name provided, return empty result (not an error)
	if assigneeName == "" {
		m.api.LogDebug("No assignee name provided, skipping resolution")
		return &AssigneeResolutionResult{Found: false}, nil
	}

	m.api.LogDebug("Resolving assignee", "assignee_name", assigneeName)

	// Search for employees by name
	employees, err := m.erpClient.SearchEmployeesByName(assigneeName)
	if err != nil {
		return &AssigneeResolutionResult{
			Found: false,
			Error: fmt.Sprintf("Failed to search employees: %v", err),
		}, err
	}

	if len(employees) == 0 {
		return &AssigneeResolutionResult{
			Found: false,
			Error: fmt.Sprintf("No employee found matching '%s'", assigneeName),
		}, nil
	}

	// Filter employees above the confidence threshold
	var highConfidenceMatches []Employee
	for _, emp := range employees {
		if emp.MatchConfidence >= EmployeeMatchThreshold {
			highConfidenceMatches = append(highConfidenceMatches, emp)
		}
	}

	// If no high confidence matches, check if we have reasonable matches
	if len(highConfidenceMatches) == 0 {
		// Look for matches above 0.7 threshold for disambiguation
		var moderateMatches []Employee
		for _, emp := range employees {
			if emp.MatchConfidence >= 0.7 {
				moderateMatches = append(moderateMatches, emp)
			}
		}

		if len(moderateMatches) == 0 {
			return &AssigneeResolutionResult{
				Found: false,
				Error: fmt.Sprintf("No employee found matching '%s' with sufficient confidence", assigneeName),
			}, nil
		}

		// Multiple moderate matches - require disambiguation
		if len(moderateMatches) > 1 {
			m.api.LogDebug("Multiple moderate confidence matches found",
				"search_name", assigneeName,
				"match_count", len(moderateMatches))

			return &AssigneeResolutionResult{
				Found:                  false,
				MultipleMatches:        true,
				RequiresDisambiguation: true,
				MatchingEmployees:      moderateMatches,
				Error:                  fmt.Sprintf("Multiple employees found matching '%s'", assigneeName),
			}, nil
		}

		// Single moderate match - use it but with lower confidence
		bestMatch := moderateMatches[0]
		m.api.LogDebug("Single moderate confidence match found",
			"input_name", assigneeName,
			"matched_employee_id", bestMatch.Name,
			"matched_employee_name", bestMatch.EmployeeName,
			"matched_employee_email", bestMatch.CompanyEmail,
			"confidence", bestMatch.MatchConfidence)

		return &AssigneeResolutionResult{
			Found:         true,
			EmployeeID:    bestMatch.Name,
			EmployeeName:  bestMatch.EmployeeName,
			EmployeeEmail: bestMatch.CompanyEmail,
		}, nil
	}

	// If exactly one high confidence match, use it
	if len(highConfidenceMatches) == 1 {
		bestMatch := highConfidenceMatches[0]
		m.api.LogDebug("Single high confidence match found",
			"input_name", assigneeName,
			"matched_employee_id", bestMatch.Name,
			"matched_employee_name", bestMatch.EmployeeName,
			"matched_employee_email", bestMatch.CompanyEmail,
			"confidence", bestMatch.MatchConfidence)

		return &AssigneeResolutionResult{
			Found:         true,
			EmployeeID:    bestMatch.Name,
			EmployeeName:  bestMatch.EmployeeName,
			EmployeeEmail: bestMatch.CompanyEmail,
		}, nil
	}

	// Multiple high confidence matches - require disambiguation
	m.api.LogDebug("Multiple high confidence matches found",
		"search_name", assigneeName,
		"match_count", len(highConfidenceMatches))

	return &AssigneeResolutionResult{
		Found:                  false,
		MultipleMatches:        true,
		RequiresDisambiguation: true,
		MatchingEmployees:      highConfidenceMatches,
		Error:                  fmt.Sprintf("Multiple employees found matching '%s'", assigneeName),
	}, nil
}

// resolveDisambiguatedEmployee resolves employee selection from disambiguation response
func (m *ProjectManagementModule) resolveDisambiguatedEmployee(
	availableEmployees []Employee,
	selectedIndex int,
	clarificationText string,
	intent string,
) (*AssigneeResolutionResult, error) {

	switch intent {
	case "index_selection":
		// Validate index range (1-based)
		if selectedIndex < 1 || selectedIndex > len(availableEmployees) {
			return &AssigneeResolutionResult{
				Found: false,
				Error: fmt.Sprintf("Invalid selection. Please choose between 1 and %d", len(availableEmployees)),
			}, nil
		}

		// Get selected employee (convert to 0-based index)
		selectedEmployee := availableEmployees[selectedIndex-1]

		m.api.LogInfo("Employee selected by index",
			"selected_index", selectedIndex,
			"employee_id", selectedEmployee.Name,
			"employee_name", selectedEmployee.EmployeeName,
			"employee_email", selectedEmployee.CompanyEmail)

		return &AssigneeResolutionResult{
			Found:         true,
			EmployeeID:    selectedEmployee.Name,
			EmployeeName:  selectedEmployee.EmployeeName,
			EmployeeEmail: selectedEmployee.CompanyEmail,
		}, nil

	case "name_clarification":
		// Try to match clarification text against available employees
		bestMatch, bestScore := m.findBestMatchFromClarification(availableEmployees, clarificationText)

		if bestScore < 0.8 {
			return &AssigneeResolutionResult{
				Found: false,
				Error: fmt.Sprintf("Could not clearly identify employee from '%s'. Please select by number (1, 2, 3, etc.)", clarificationText),
			}, nil
		}

		m.api.LogInfo("Employee selected by clarification",
			"clarification", clarificationText,
			"employee_id", bestMatch.Name,
			"employee_name", bestMatch.EmployeeName,
			"employee_email", bestMatch.CompanyEmail,
			"match_score", bestScore)

		return &AssigneeResolutionResult{
			Found:         true,
			EmployeeID:    bestMatch.Name,
			EmployeeName:  bestMatch.EmployeeName,
			EmployeeEmail: bestMatch.CompanyEmail,
		}, nil

	case "cancel":
		return &AssigneeResolutionResult{
			Found: false,
			Error: "Assignment cancelled by user",
		}, nil

	default:
		return &AssigneeResolutionResult{
			Found: false,
			Error: "Invalid response. Please select by number (1, 2, 3, etc.) or provide more specific details",
		}, nil
	}
}

// findBestMatchFromClarification finds the best employee match from clarification text
func (m *ProjectManagementModule) findBestMatchFromClarification(employees []Employee, clarificationText string) (Employee, float64) {
	var bestMatch Employee
	var bestScore float64

	clarificationLower := strings.ToLower(clarificationText)

	for _, emp := range employees {
		score := m.calculateClarificationMatchScore(emp, clarificationLower)
		if score > bestScore {
			bestScore = score
			bestMatch = emp
		}
	}

	return bestMatch, bestScore
}

// calculateClarificationMatchScore calculates how well an employee matches the clarification text
func (m *ProjectManagementModule) calculateClarificationMatchScore(emp Employee, clarificationLower string) float64 {
	var maxScore float64

	// Check full name match
	empNameLower := strings.ToLower(emp.EmployeeName)
	if strings.Contains(clarificationLower, empNameLower) || strings.Contains(empNameLower, clarificationLower) {
		maxScore = math.Max(maxScore, 0.95)
	}

	// Check email match
	empEmailLower := strings.ToLower(emp.CompanyEmail)
	if strings.Contains(clarificationLower, empEmailLower) || strings.Contains(empEmailLower, clarificationLower) {
		maxScore = math.Max(maxScore, 0.9)
	}

	// Check employee ID match
	empIDLower := strings.ToLower(emp.Name)
	if strings.Contains(clarificationLower, empIDLower) || strings.Contains(empIDLower, clarificationLower) {
		maxScore = math.Max(maxScore, 0.95)
	}

	// Check department match
	if emp.Department != "" {
		empDeptLower := strings.ToLower(emp.Department)
		if strings.Contains(clarificationLower, empDeptLower) {
			maxScore = math.Max(maxScore, 0.7)
		}
	}

	// Check individual words in name
	nameWords := strings.Fields(empNameLower)
	clarificationWords := strings.Fields(clarificationLower)

	var wordMatches float64
	for _, clarWord := range clarificationWords {
		for _, nameWord := range nameWords {
			if strings.Contains(nameWord, clarWord) || strings.Contains(clarWord, nameWord) {
				wordMatches++
				break
			}
		}
	}

	if len(clarificationWords) > 0 {
		wordMatchScore := wordMatches / float64(len(clarificationWords)) * 0.8
		maxScore = math.Max(maxScore, wordMatchScore)
	}

	return maxScore
}

// processAssigneeForProject resolves assignee for project creation
func (m *ProjectManagementModule) processAssigneeForProject(projectRequest *ProjectCreationRequest) {
	if projectRequest.AssignedToName == "" {
		m.api.LogDebug("No assignee specified for project")
		return
	}

	assigneeResult, err := m.resolveAssignee(projectRequest.AssignedToName)
	if err != nil {
		m.api.LogError("Failed to resolve project assignee", "error", err.Error())
		return
	}

	if assigneeResult.Found {
		projectRequest.AssignedToEmployeeID = assigneeResult.EmployeeID
		projectRequest.AssignedToEmail = assigneeResult.EmployeeEmail
		m.api.LogInfo("Resolved project assignee",
			"input_name", projectRequest.AssignedToName,
			"resolved_employee_id", assigneeResult.EmployeeID,
			"resolved_employee_name", assigneeResult.EmployeeName,
			"resolved_employee_email", assigneeResult.EmployeeEmail)
	} else {
		// Log warning but continue - assignee resolution is optional
		m.api.LogWarn("Could not resolve project assignee",
			"input_name", projectRequest.AssignedToName,
			"error", assigneeResult.Error,
			"requires_disambiguation", assigneeResult.RequiresDisambiguation)
		// Clear both employee ID and email if resolution failed
		projectRequest.AssignedToEmployeeID = ""
		projectRequest.AssignedToEmail = ""
	}
}

// processAssigneeForTask resolves assignee for task creation
func (m *ProjectManagementModule) processAssigneeForTask(taskRequest *TaskCreationRequest) {
	if taskRequest.AssignedToName == "" {
		m.api.LogDebug("No assignee specified for task")
		return
	}

	assigneeResult, err := m.resolveAssignee(taskRequest.AssignedToName)
	if err != nil {
		m.api.LogError("Failed to resolve task assignee", "error", err.Error())
		return
	}

	if assigneeResult.Found {
		taskRequest.AssignedToEmployeeID = assigneeResult.EmployeeID
		taskRequest.AssignedToEmail = assigneeResult.EmployeeEmail
		m.api.LogInfo("Resolved task assignee",
			"input_name", taskRequest.AssignedToName,
			"resolved_employee_id", assigneeResult.EmployeeID,
			"resolved_employee_name", assigneeResult.EmployeeName,
			"resolved_employee_email", assigneeResult.EmployeeEmail)
	} else {
		// Log warning but continue - assignee resolution is optional
		m.api.LogWarn("Could not resolve task assignee",
			"input_name", taskRequest.AssignedToName,
			"error", assigneeResult.Error,
			"requires_disambiguation", assigneeResult.RequiresDisambiguation)
		// Clear both employee ID and email if resolution failed
		taskRequest.AssignedToEmployeeID = ""
		taskRequest.AssignedToEmail = ""
	}
}

// handleAssigneeModification handles assignee changes during modification
func (m *ProjectManagementModule) handleAssigneeModification(modifications map[string]interface{}) {
	assigneeName, hasAssignee := modifications["assigned_to_name"]
	if !hasAssignee {
		return
	}

	// If empty string, clear the employee ID and email
	if assigneeName == "" || assigneeName == nil {
		modifications["assigned_to_employee_id"] = ""
		modifications["assigned_to_email"] = ""
		m.api.LogDebug("Cleared assignee")
		return
	}

	// Resolve the new assignee
	assigneeResult, err := m.resolveAssignee(assigneeName.(string))
	if err != nil {
		m.api.LogError("Failed to resolve modified assignee", "error", err.Error())
		modifications["assigned_to_employee_id"] = ""
		modifications["assigned_to_email"] = ""
		return
	}

	if assigneeResult.Found {
		modifications["assigned_to_employee_id"] = assigneeResult.EmployeeID
		modifications["assigned_to_email"] = assigneeResult.EmployeeEmail
		m.api.LogInfo("Resolved modified assignee",
			"input_name", assigneeName,
			"resolved_employee_id", assigneeResult.EmployeeID,
			"resolved_employee_name", assigneeResult.EmployeeName,
			"resolved_employee_email", assigneeResult.EmployeeEmail)
	} else {
		// Clear both employee ID and email if resolution failed
		modifications["assigned_to_employee_id"] = ""
		modifications["assigned_to_email"] = ""
		m.api.LogWarn("Could not resolve modified assignee",
			"input_name", assigneeName,
			"error", assigneeResult.Error,
			"requires_disambiguation", assigneeResult.RequiresDisambiguation)
	}
}
