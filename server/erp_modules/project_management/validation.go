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

// isValidAction checks if the action is supported
func isValidAction(action string) bool {
	validActions := map[string]bool{
		"create_project": true,
		"create_task":    true,
	}

	return validActions[action]
}
