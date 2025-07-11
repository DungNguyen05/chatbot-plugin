// Copyright (c) 2023-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package project_management

import (
	"fmt"
	"strings"
)

// resolveMultipleAssignees resolves multiple assignee names with comprehensive disambiguation support
func (m *ProjectManagementModule) resolveMultipleAssignees(assigneeNames []string) (*AssigneeResolutionResult, error) {
	if len(assigneeNames) == 0 {
		m.api.LogDebug("No assignee names provided, skipping resolution")
		return &AssigneeResolutionResult{
			ResolvedEmployees:         []AssignedEmployee{},
			UnresolvedEmployeeMatches: []UnresolvedEmployeeMatch{},
			RequiresDisambiguation:    false,
		}, nil
	}

	m.api.LogDebug("Resolving multiple assignees", "assignee_names", assigneeNames)

	var resolvedEmployees []AssignedEmployee
	var unresolvedMatches []UnresolvedEmployeeMatch
	displayIndex := 1

	for _, assigneeName := range assigneeNames {
		assigneeName = strings.TrimSpace(assigneeName)
		if assigneeName == "" {
			continue
		}

		// Search for employees by name
		employees, err := m.erpClient.SearchEmployeesByName(assigneeName)
		if err != nil {
			m.api.LogError("Failed to search employees", "assignee_name", assigneeName, "error", err.Error())
			continue
		}

		if len(employees) == 0 {
			m.api.LogWarn("No employee found", "assignee_name", assigneeName)
			continue
		}

		// Filter employees above the confidence threshold
		var highConfidenceMatches []Employee
		for _, emp := range employees {
			if emp.MatchConfidence >= EmployeeMatchThreshold {
				highConfidenceMatches = append(highConfidenceMatches, emp)
			}
		}

		// If exactly one high confidence match, resolve it
		if len(highConfidenceMatches) == 1 {
			emp := highConfidenceMatches[0]
			resolvedEmployees = append(resolvedEmployees, AssignedEmployee{
				EmployeeID:   emp.Name,
				EmployeeName: emp.EmployeeName,
				Email:        emp.CompanyEmail,
				OriginalName: assigneeName,
			})
			m.api.LogDebug("Resolved employee with high confidence",
				"original_name", assigneeName,
				"employee_id", emp.Name,
				"employee_name", emp.EmployeeName,
				"confidence", emp.MatchConfidence)
		} else {
			// Multiple matches or no high confidence matches - needs disambiguation
			var candidateEmployees []Employee
			if len(highConfidenceMatches) > 1 {
				// Multiple high confidence matches
				candidateEmployees = highConfidenceMatches
			} else {
				// No high confidence matches, check for moderate confidence
				for _, emp := range employees {
					if emp.MatchConfidence >= 0.7 {
						candidateEmployees = append(candidateEmployees, emp)
					}
				}
			}

			if len(candidateEmployees) > 0 {
				unresolvedMatches = append(unresolvedMatches, UnresolvedEmployeeMatch{
					OriginalName:      assigneeName,
					MatchingEmployees: candidateEmployees,
					Index:             displayIndex,
				})
				displayIndex += len(candidateEmployees)
				m.api.LogDebug("Employee requires disambiguation",
					"original_name", assigneeName,
					"candidate_count", len(candidateEmployees))
			} else {
				m.api.LogWarn("No suitable candidates found for employee",
					"assignee_name", assigneeName)
			}
		}
	}

	result := &AssigneeResolutionResult{
		ResolvedEmployees:         resolvedEmployees,
		UnresolvedEmployeeMatches: unresolvedMatches,
		RequiresDisambiguation:    len(unresolvedMatches) > 0,
	}

	m.api.LogInfo("Multi-assignee resolution completed",
		"resolved_count", len(resolvedEmployees),
		"unresolved_count", len(unresolvedMatches),
		"requires_disambiguation", result.RequiresDisambiguation)

	return result, nil
}

// resolveDisambiguatedEmployees resolves employees from user disambiguation choices
func (m *ProjectManagementModule) resolveDisambiguatedEmployees(
	unresolvedMatches []UnresolvedEmployeeMatch,
	selectedIndexes []int,
) ([]AssignedEmployee, error) {
	var resolvedEmployees []AssignedEmployee

	// Build a map of global index to employee
	globalIndexToEmployee := make(map[int]Employee)
	globalIndexToOriginalName := make(map[int]string)

	currentIndex := 1
	for _, unresolvedMatch := range unresolvedMatches {
		for _, emp := range unresolvedMatch.MatchingEmployees {
			globalIndexToEmployee[currentIndex] = emp
			globalIndexToOriginalName[currentIndex] = unresolvedMatch.OriginalName
			currentIndex++
		}
	}

	// Validate that we have at least one selection
	if len(selectedIndexes) == 0 {
		return nil, fmt.Errorf("no employees selected")
	}

	// Track which employees we've already added to avoid duplicates
	addedEmployees := make(map[string]bool)

	// Validate and resolve selected indexes
	for _, selectedIndex := range selectedIndexes {
		if selectedIndex < 1 || selectedIndex >= currentIndex {
			return nil, fmt.Errorf("invalid selection index: %d (valid range: 1-%d)", selectedIndex, currentIndex-1)
		}

		emp, exists := globalIndexToEmployee[selectedIndex]
		if !exists {
			return nil, fmt.Errorf("employee not found for index: %d", selectedIndex)
		}

		// Avoid duplicate employees (in case user selects same person multiple times)
		if addedEmployees[emp.Name] {
			m.api.LogInfo("Skipping duplicate employee selection",
				"employee_id", emp.Name,
				"employee_name", emp.EmployeeName)
			continue
		}

		originalName := globalIndexToOriginalName[selectedIndex]

		resolvedEmployees = append(resolvedEmployees, AssignedEmployee{
			EmployeeID:   emp.Name,
			EmployeeName: emp.EmployeeName,
			Email:        emp.CompanyEmail,
			OriginalName: originalName,
		})

		addedEmployees[emp.Name] = true

		m.api.LogInfo("Employee selected by index",
			"selected_index", selectedIndex,
			"original_name", originalName,
			"employee_id", emp.Name,
			"employee_name", emp.EmployeeName)
	}

	if len(resolvedEmployees) == 0 {
		return nil, fmt.Errorf("no valid employees were resolved from the selected indexes")
	}

	m.api.LogInfo("Successfully resolved employees",
		"total_selected", len(resolvedEmployees),
		"original_unresolved_count", len(unresolvedMatches))

	return resolvedEmployees, nil
}
