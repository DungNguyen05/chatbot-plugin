// Copyright (c) 2023-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package project_management

import (
	"fmt"
)

// resolveAssignee resolves an assignee name to an employee ID
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

	// Take the highest confidence match
	bestMatch := employees[0]
	m.api.LogDebug("Found assignee match",
		"input_name", assigneeName,
		"matched_employee_id", bestMatch.Name,
		"matched_employee_name", bestMatch.EmployeeName,
		"confidence", bestMatch.MatchConfidence)

	return &AssigneeResolutionResult{
		Found:        true,
		EmployeeID:   bestMatch.Name,
		EmployeeName: bestMatch.EmployeeName,
	}, nil
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
		m.api.LogInfo("Resolved project assignee",
			"input_name", projectRequest.AssignedToName,
			"resolved_employee_id", assigneeResult.EmployeeID,
			"resolved_employee_name", assigneeResult.EmployeeName)
	} else {
		// Log warning but continue - assignee resolution is optional
		m.api.LogWarn("Could not resolve project assignee",
			"input_name", projectRequest.AssignedToName,
			"error", assigneeResult.Error)
		// Keep the original name for user reference but clear the employee ID
		projectRequest.AssignedToEmployeeID = ""
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
		m.api.LogInfo("Resolved task assignee",
			"input_name", taskRequest.AssignedToName,
			"resolved_employee_id", assigneeResult.EmployeeID,
			"resolved_employee_name", assigneeResult.EmployeeName)
	} else {
		// Log warning but continue - assignee resolution is optional
		m.api.LogWarn("Could not resolve task assignee",
			"input_name", taskRequest.AssignedToName,
			"error", assigneeResult.Error)
		// Keep the original name for user reference but clear the employee ID
		taskRequest.AssignedToEmployeeID = ""
	}
}

// handleAssigneeModification handles assignee changes during modification
func (m *ProjectManagementModule) handleAssigneeModification(modifications map[string]interface{}) {
	assigneeName, hasAssignee := modifications["assigned_to_name"]
	if !hasAssignee {
		return
	}

	// If empty string, clear the employee ID
	if assigneeName == "" || assigneeName == nil {
		modifications["assigned_to_employee_id"] = ""
		m.api.LogDebug("Cleared assignee")
		return
	}

	// Resolve the new assignee
	assigneeResult, err := m.resolveAssignee(assigneeName.(string))
	if err != nil {
		m.api.LogError("Failed to resolve modified assignee", "error", err.Error())
		modifications["assigned_to_employee_id"] = ""
		return
	}

	if assigneeResult.Found {
		modifications["assigned_to_employee_id"] = assigneeResult.EmployeeID
		m.api.LogInfo("Resolved modified assignee",
			"input_name", assigneeName,
			"resolved_employee_id", assigneeResult.EmployeeID,
			"resolved_employee_name", assigneeResult.EmployeeName)
	} else {
		// Clear the employee ID if resolution failed
		modifications["assigned_to_employee_id"] = ""
		m.api.LogWarn("Could not resolve modified assignee",
			"input_name", assigneeName,
			"error", assigneeResult.Error)
	}
}
