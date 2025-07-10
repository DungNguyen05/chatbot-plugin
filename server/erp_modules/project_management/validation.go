// Copyright (c) 2023-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package project_management

import (
	"fmt"
	"strings"
)

// validateProjectManagementConfig validates project management module configuration
func ValidateProjectManagementConfig(config ProjectManagementConfig) error {
	if config.ERPDomain == "" {
		return fmt.Errorf("ERP domain is required")
	}

	if config.ERPAPIKey == "" {
		return fmt.Errorf("ERP API key is required")
	}

	if config.ERPAPISecret == "" {
		return fmt.Errorf("ERP API secret is required")
	}

	// Validate ERP domain format
	if !strings.HasPrefix(config.ERPDomain, "http://") && !strings.HasPrefix(config.ERPDomain, "https://") {
		return fmt.Errorf("ERP domain must start with http:// or https://")
	}

	return nil
}

// sanitizeUserInput sanitizes user input to prevent injection attacks
func sanitizeUserInput(input string) string {
	// Remove leading/trailing whitespace
	input = strings.TrimSpace(input)

	// Remove potential SQL injection characters
	dangerous := []string{"'", "\"", ";", "--", "/*", "*/", "xp_", "sp_"}
	for _, char := range dangerous {
		input = strings.ReplaceAll(input, char, "")
	}

	return input
}

// validateEmployeeID validates employee ID format
func validateEmployeeID(employeeID string) error {
	if employeeID == "" {
		return fmt.Errorf("employee ID cannot be empty")
	}

	if len(employeeID) > 50 {
		return fmt.Errorf("employee ID too long (max 50 characters)")
	}

	// Check for basic format (alphanumeric, dash, underscore allowed)
	for _, char := range employeeID {
		if !((char >= 'a' && char <= 'z') ||
			(char >= 'A' && char <= 'Z') ||
			(char >= '0' && char <= '9') ||
			char == '-' || char == '_' || char == '.') {
			return fmt.Errorf("employee ID contains invalid characters")
		}
	}

	return nil
}

// validateChatID validates chat ID format
func validateChatID(chatID string) error {
	if chatID == "" {
		return fmt.Errorf("chat ID cannot be empty")
	}

	if len(chatID) != 26 {
		return fmt.Errorf("invalid chat ID format (expected 26 characters)")
	}

	// Mattermost user IDs are 26 character alphanumeric strings
	for _, char := range chatID {
		if !((char >= 'a' && char <= 'z') || (char >= '0' && char <= '9')) {
			return fmt.Errorf("chat ID contains invalid characters")
		}
	}

	return nil
}

// validateConfig validates the ERP configuration
func (c *ERPClient) validateConfig() error {
	if c.config.ERPDomain == "" {
		return fmt.Errorf("ERP domain not configured")
	}
	if c.config.ERPAPIKey == "" {
		return fmt.Errorf("ERP API key not configured")
	}
	if c.config.ERPAPISecret == "" {
		return fmt.Errorf("ERP API secret not configured")
	}
	return nil
}

// validateProjectName validates project name
func validateProjectName(name string) error {
	name = strings.TrimSpace(name)

	if name == "" {
		return fmt.Errorf("project name cannot be empty")
	}

	if len(name) > 200 {
		return fmt.Errorf("project name too long (max 200 characters)")
	}

	return nil
}

// validateTaskSubject validates task subject
func validateTaskSubject(subject string) error {
	subject = strings.TrimSpace(subject)

	if subject == "" {
		return fmt.Errorf("task subject cannot be empty")
	}

	if len(subject) > 200 {
		return fmt.Errorf("task subject too long (max 200 characters)")
	}

	return nil
}

// validateEmployeeEmail validates employee email format - NEW FUNCTION
func validateEmployeeEmail(email string) error {
	if email == "" {
		return fmt.Errorf("employee email cannot be empty")
	}

	if len(email) > 100 {
		return fmt.Errorf("employee email too long (max 100 characters)")
	}

	// Basic email format validation
	if !strings.Contains(email, "@") {
		return fmt.Errorf("invalid email format: missing @")
	}

	if !strings.Contains(email, ".") {
		return fmt.Errorf("invalid email format: missing domain")
	}

	// Check for basic format
	if strings.Count(email, "@") != 1 {
		return fmt.Errorf("invalid email format: multiple @ symbols")
	}

	parts := strings.Split(email, "@")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return fmt.Errorf("invalid email format")
	}

	return nil
}

// validateToDoAssignment validates ToDo assignment data - NEW FUNCTION
func validateToDoAssignment(assignedBy, allocatedTo, referenceType, referenceName string) error {
	if err := validateEmployeeEmail(assignedBy); err != nil {
		return fmt.Errorf("invalid assigned_by email: %w", err)
	}

	if err := validateEmployeeEmail(allocatedTo); err != nil {
		return fmt.Errorf("invalid allocated_to email: %w", err)
	}

	if referenceType == "" {
		return fmt.Errorf("reference type cannot be empty")
	}

	if referenceType != "Task" && referenceType != "Project" {
		return fmt.Errorf("reference type must be 'Task' or 'Project'")
	}

	if referenceName == "" {
		return fmt.Errorf("reference name cannot be empty")
	}

	return nil
}

// isValidAction checks if the action is supported
func isValidAction(action string) bool {
	validActions := map[string]bool{
		"create_project": true,
		"create_task":    true,
	}

	return validActions[action]
}

// validatePriority validates priority field - NEW FUNCTION
func validatePriority(priority string) error {
	if priority == "" {
		return nil // Empty priority is allowed
	}

	validPriorities := map[string]bool{
		"High":   true,
		"Medium": true,
		"Low":    true,
	}

	if !validPriorities[priority] {
		return fmt.Errorf("invalid priority: must be 'High', 'Medium', or 'Low'")
	}

	return nil
}

// validateCompany validates company field - NEW FUNCTION
func validateCompany(company string) error {
	if company == "" {
		return nil // Empty company is allowed
	}

	if len(company) > 100 {
		return fmt.Errorf("company name too long (max 100 characters)")
	}

	return nil
}
