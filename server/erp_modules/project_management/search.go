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
		employeeEmailLower := strings.ToLower(employee.CompanyEmail) // UPDATED to use CompanyEmail

		// Calculate confidence score (UPDATED to include company email)
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

// removeDuplicateEmployees removes duplicate employees from the list
func removeDuplicateEmployees(employees []Employee) []Employee {
	seen := make(map[string]bool)
	var unique []Employee

	for _, emp := range employees {
		if !seen[emp.Name] {
			seen[emp.Name] = true
			unique = append(unique, emp)
		}
	}

	return unique
}

// calculateNameMatchConfidence calculates name matching confidence (UPDATED to include email)
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

	// Check employee company email match (UPDATED field name)
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
