// Copyright (c) 2023-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package project_management

import (
	"fmt"
	"strings"
	"time"
)

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

// ValidateProjectManagementConfig validates project management module configuration
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

	if len(name) > 140 {
		return fmt.Errorf("project name too long (max 140 characters)")
	}

	return nil
}

// validateTaskSubject validates task subject
func validateTaskSubject(subject string) error {
	subject = strings.TrimSpace(subject)

	if subject == "" {
		return fmt.Errorf("task subject cannot be empty")
	}

	if len(subject) > 140 {
		return fmt.Errorf("task subject too long (max 140 characters)")
	}

	return nil
}

// validateDescription validates description field
func validateDescription(description string) error {
	if len(description) > 1000 {
		return fmt.Errorf("description too long (max 1000 characters)")
	}

	return nil
}

// validatePriority validates priority field
func validatePriority(priority string) error {
	if priority == "" {
		return nil // Priority is optional
	}

	validPriorities := map[string]bool{
		"Low":      true,
		"Medium":   true,
		"High":     true,
		"Critical": true,
		"Urgent":   true,
	}

	if !validPriorities[priority] {
		return fmt.Errorf("invalid priority: %s. Valid options: Low, Medium, High, Critical, Urgent", priority)
	}

	return nil
}

// validateDateFormat validates date format (YYYY-MM-DD)
func validateDateFormat(dateStr string) error {
	if dateStr == "" {
		return nil // Date is optional
	}

	if len(dateStr) != 10 {
		return fmt.Errorf("date must be in YYYY-MM-DD format")
	}

	_, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		return fmt.Errorf("invalid date format: %s", dateStr)
	}

	return nil
}

// validateProjectRequest validates complete project creation request
func validateProjectRequest(req *ProjectCreationRequest) error {
	if err := validateProjectName(req.ProjectName); err != nil {
		return fmt.Errorf("invalid project name: %w", err)
	}

	if err := validateDescription(req.Description); err != nil {
		return fmt.Errorf("invalid description: %w", err)
	}

	if err := validatePriority(req.Priority); err != nil {
		return fmt.Errorf("invalid priority: %w", err)
	}

	if err := validateDateFormat(req.ExpectedStartDate); err != nil {
		return fmt.Errorf("invalid start date: %w", err)
	}

	if err := validateDateFormat(req.ExpectedEndDate); err != nil {
		return fmt.Errorf("invalid end date: %w", err)
	}

	// Validate date range if both dates are provided
	if req.ExpectedStartDate != "" && req.ExpectedEndDate != "" {
		if req.ExpectedStartDate > req.ExpectedEndDate {
			return fmt.Errorf("start date cannot be after end date")
		}
	}

	return nil
}

// validateTaskRequest validates complete task creation request
func validateTaskRequest(req *TaskCreationRequest) error {
	if err := validateTaskSubject(req.Subject); err != nil {
		return fmt.Errorf("invalid task subject: %w", err)
	}

	if err := validateDescription(req.Description); err != nil {
		return fmt.Errorf("invalid description: %w", err)
	}

	if err := validatePriority(req.Priority); err != nil {
		return fmt.Errorf("invalid priority: %w", err)
	}

	if err := validateDateFormat(req.ExpStartDate); err != nil {
		return fmt.Errorf("invalid start date: %w", err)
	}

	if err := validateDateFormat(req.ExpEndDate); err != nil {
		return fmt.Errorf("invalid end date: %w", err)
	}

	// Validate date range if both dates are provided
	if req.ExpStartDate != "" && req.ExpEndDate != "" {
		if req.ExpStartDate > req.ExpEndDate {
			return fmt.Errorf("start date cannot be after end date")
		}
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
