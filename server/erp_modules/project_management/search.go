// Copyright (c) 2023-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package project_management

import (
	"math"
	"sort"
	"strings"
)

// SearchEmployeesByName searches for employees by name using fuzzy matching
func (c *ERPClient) SearchEmployeesByName(searchName string) ([]Employee, error) {
	c.api.LogDebug("Searching employees by name for project management", "search_name", searchName)

	// Get all employees first
	allEmployees, err := c.GetAllEmployees()
	if err != nil {
		return nil, err
	}

	// Filter employees using fuzzy matching
	var matchedEmployees []Employee
	searchNameLower := strings.ToLower(searchName)

	for _, employee := range allEmployees {
		// Check multiple fields for matches
		employeeNameLower := strings.ToLower(employee.EmployeeName)
		employeeIDLower := strings.ToLower(employee.Name)
		employeeNumberLower := strings.ToLower(employee.EmployeeNumber)
		employeeEmailLower := strings.ToLower(employee.CompanyEmail)

		// Calculate confidence score
		confidence := calculateNameMatchConfidence(searchNameLower, employeeNameLower, employeeIDLower, employeeNumberLower, employeeEmailLower)

		// Only include employees with confidence above threshold
		if confidence >= 0.8 { // 80% confidence threshold
			employee.MatchConfidence = confidence
			matchedEmployees = append(matchedEmployees, employee)
		}
	}

	// Sort by confidence (highest first)
	sort.Slice(matchedEmployees, func(i, j int) bool {
		return matchedEmployees[i].MatchConfidence > matchedEmployees[j].MatchConfidence
	})

	c.api.LogDebug("Found matching employees for project management", "count", len(matchedEmployees), "search_name", searchName)

	return matchedEmployees, nil
}

// SearchProjectsByName searches for projects by name using fuzzy matching
func (c *ERPClient) SearchProjectsByName(searchName string) ([]Project, error) {
	c.api.LogDebug("Searching projects by name", "search_name", searchName)

	// Get all projects first
	allProjects, err := c.GetAllProjects()
	if err != nil {
		return nil, err
	}

	// Filter projects using fuzzy matching
	var matchedProjects []Project
	searchNameLower := strings.ToLower(searchName)

	for _, project := range allProjects {
		// Check project name for matches
		projectNameLower := strings.ToLower(project.ProjectName)
		projectIDLower := strings.ToLower(project.Name)

		// Calculate confidence score
		confidence := calculateProjectMatchConfidence(searchNameLower, projectNameLower, projectIDLower)

		// Only include projects with confidence above threshold
		if confidence >= 0.8 { // 80% confidence threshold
			project.MatchConfidence = confidence
			matchedProjects = append(matchedProjects, project)
		}
	}

	// Sort by confidence (highest first)
	sort.Slice(matchedProjects, func(i, j int) bool {
		return matchedProjects[i].MatchConfidence > matchedProjects[j].MatchConfidence
	})

	c.api.LogDebug("Found matching projects", "count", len(matchedProjects), "search_name", searchName)

	return matchedProjects, nil
}

// SearchTasksByName searches for tasks by name using fuzzy matching
func (c *ERPClient) SearchTasksByName(searchName string) ([]Task, error) {
	c.api.LogDebug("Searching tasks by name", "search_name", searchName)

	// Get all tasks first
	allTasks, err := c.GetAllTasks()
	if err != nil {
		return nil, err
	}

	// Filter tasks using fuzzy matching
	var matchedTasks []Task
	searchNameLower := strings.ToLower(searchName)

	for _, task := range allTasks {
		// Check task subject for matches
		taskSubjectLower := strings.ToLower(task.Subject)
		taskIDLower := strings.ToLower(task.Name)

		// Calculate confidence score
		confidence := calculateTaskMatchConfidence(searchNameLower, taskSubjectLower, taskIDLower)

		// Only include tasks with confidence above threshold
		if confidence >= 0.8 { // 80% confidence threshold
			task.MatchConfidence = confidence
			matchedTasks = append(matchedTasks, task)
		}
	}

	// Sort by confidence (highest first)
	sort.Slice(matchedTasks, func(i, j int) bool {
		return matchedTasks[i].MatchConfidence > matchedTasks[j].MatchConfidence
	})

	c.api.LogDebug("Found matching tasks", "count", len(matchedTasks), "search_name", searchName)

	return matchedTasks, nil
}

// calculateProjectMatchConfidence calculates project name matching confidence
func calculateProjectMatchConfidence(searchName, projectName, projectID string) float64 {
	var maxConfidence float64

	// Exact match
	if searchName == projectName {
		return 1.0
	}

	// Check if search name is contained in project name
	if strings.Contains(projectName, searchName) {
		maxConfidence = math.Max(maxConfidence, 0.9)
	}

	// Check if project name is contained in search name
	if strings.Contains(searchName, projectName) {
		maxConfidence = math.Max(maxConfidence, 0.85)
	}

	// Check individual words
	searchWords := strings.Fields(searchName)
	projectWords := strings.Fields(projectName)

	var matchCount float64
	for _, searchWord := range searchWords {
		for _, projWord := range projectWords {
			if strings.Contains(projWord, searchWord) || strings.Contains(searchWord, projWord) {
				matchCount++
				break
			}
		}
	}

	if len(searchWords) > 0 {
		wordMatchConfidence := matchCount / float64(len(searchWords)) * 0.8
		maxConfidence = math.Max(maxConfidence, wordMatchConfidence)
	}

	// Check project ID match
	if strings.Contains(projectID, searchName) {
		maxConfidence = math.Max(maxConfidence, 0.95)
	}

	// Fuzzy string matching using Levenshtein distance
	if len(searchName) > 2 && len(projectName) > 2 {
		fuzzyConfidence := calculateFuzzyMatch(searchName, projectName)
		maxConfidence = math.Max(maxConfidence, fuzzyConfidence)
	}

	return maxConfidence
}

// calculateTaskMatchConfidence calculates task name matching confidence
func calculateTaskMatchConfidence(searchName, taskSubject, taskID string) float64 {
	var maxConfidence float64

	// Exact match
	if searchName == taskSubject {
		return 1.0
	}

	// Check if search name is contained in task subject
	if strings.Contains(taskSubject, searchName) {
		maxConfidence = math.Max(maxConfidence, 0.9)
	}

	// Check if task subject is contained in search name
	if strings.Contains(searchName, taskSubject) {
		maxConfidence = math.Max(maxConfidence, 0.85)
	}

	// Check individual words
	searchWords := strings.Fields(searchName)
	taskWords := strings.Fields(taskSubject)

	var matchCount float64
	for _, searchWord := range searchWords {
		for _, taskWord := range taskWords {
			if strings.Contains(taskWord, searchWord) || strings.Contains(searchWord, taskWord) {
				matchCount++
				break
			}
		}
	}

	if len(searchWords) > 0 {
		wordMatchConfidence := matchCount / float64(len(searchWords)) * 0.8
		maxConfidence = math.Max(maxConfidence, wordMatchConfidence)
	}

	// Check task ID match
	if strings.Contains(taskID, searchName) {
		maxConfidence = math.Max(maxConfidence, 0.95)
	}

	// Fuzzy string matching using Levenshtein distance
	if len(searchName) > 2 && len(taskSubject) > 2 {
		fuzzyConfidence := calculateFuzzyMatch(searchName, taskSubject)
		maxConfidence = math.Max(maxConfidence, fuzzyConfidence)
	}

	return maxConfidence
}

// calculateNameMatchConfidence calculates name matching confidence
func calculateNameMatchConfidence(searchName, employeeName, employeeID, employeeNumber, employeeEmail string) float64 {
	var maxConfidence float64

	// Exact match
	if searchName == employeeName {
		return 1.0
	}

	// Check if search name is contained in employee name
	if strings.Contains(employeeName, searchName) {
		maxConfidence = math.Max(maxConfidence, 0.9)
	}

	// Check if employee name is contained in search name
	if strings.Contains(searchName, employeeName) {
		maxConfidence = math.Max(maxConfidence, 0.85)
	}

	// Check individual words
	searchWords := strings.Fields(searchName)
	employeeWords := strings.Fields(employeeName)

	var matchCount float64
	for _, searchWord := range searchWords {
		for _, empWord := range employeeWords {
			if strings.Contains(empWord, searchWord) || strings.Contains(searchWord, empWord) {
				matchCount++
				break
			}
		}
	}

	if len(searchWords) > 0 {
		wordMatchConfidence := matchCount / float64(len(searchWords)) * 0.8
		maxConfidence = math.Max(maxConfidence, wordMatchConfidence)
	}

	// Check employee ID match
	if strings.Contains(employeeID, searchName) {
		maxConfidence = math.Max(maxConfidence, 0.95)
	}

	// Check employee number match
	if employeeNumber != "" && strings.Contains(employeeNumber, searchName) {
		maxConfidence = math.Max(maxConfidence, 0.95)
	}

	// Check employee company email match
	if employeeEmail != "" && strings.Contains(employeeEmail, searchName) {
		maxConfidence = math.Max(maxConfidence, 0.9)
	}

	// Fuzzy string matching using Levenshtein distance
	if len(searchName) > 2 && len(employeeName) > 2 {
		fuzzyConfidence := calculateFuzzyMatch(searchName, employeeName)
		maxConfidence = math.Max(maxConfidence, fuzzyConfidence)
	}

	return maxConfidence
}

// calculateFuzzyMatch calculates fuzzy matching using Levenshtein distance
func calculateFuzzyMatch(s1, s2 string) float64 {
	if len(s1) == 0 {
		return 0
	}
	if len(s2) == 0 {
		return 0
	}

	// Create matrix
	matrix := make([][]int, len(s1)+1)
	for i := range matrix {
		matrix[i] = make([]int, len(s2)+1)
	}

	// Initialize first row and column
	for i := 0; i <= len(s1); i++ {
		matrix[i][0] = i
	}
	for j := 0; j <= len(s2); j++ {
		matrix[0][j] = j
	}

	// Fill matrix
	for i := 1; i <= len(s1); i++ {
		for j := 1; j <= len(s2); j++ {
			cost := 0
			if s1[i-1] != s2[j-1] {
				cost = 1
			}

			matrix[i][j] = minInt(
				matrix[i-1][j]+1,      // deletion
				matrix[i][j-1]+1,      // insertion
				matrix[i-1][j-1]+cost, // substitution
			)
		}
	}

	distance := matrix[len(s1)][len(s2)]
	maxLen := maxInt(len(s1), len(s2))

	if maxLen == 0 {
		return 1.0
	}

	similarity := 1.0 - float64(distance)/float64(maxLen)

	// Only return as fuzzy match if similarity is reasonable
	if similarity >= 0.6 {
		return similarity * 0.7 // Scale down fuzzy matches
	}

	return 0
}

// Helper function for minimum of 3 integers
func minInt(a, b, c int) int {
	if a <= b && a <= c {
		return a
	}
	if b <= c {
		return b
	}
	return c
}

// Helper function for maximum of 2 integers
func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
