// Copyright (c) 2023-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package attendance

import (
	"fmt"
	"strings"
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

// validateAttendanceConfig validates attendance module configuration
func ValidateAttendanceConfig(config AttendanceConfig) error {
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

	// Validate notification channels (optional)
	for _, channelID := range config.NotifyChannels {
		if channelID == "" {
			return fmt.Errorf("notification channel ID cannot be empty")
		}
		if len(channelID) != 26 {
			return fmt.Errorf("invalid notification channel ID format: %s", channelID)
		}
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

// validateAbsentReason validates absence reason
func validateAbsentReason(reason string) error {
	reason = strings.TrimSpace(reason)

	if reason == "" {
		return fmt.Errorf("absence reason cannot be empty")
	}

	if len(reason) > 200 {
		return fmt.Errorf("absence reason too long (max 200 characters)")
	}

	return nil
}

// validateDateRange validates date range for reports
func validateDateRange(startDate, endDate string) error {
	if startDate == "" {
		return fmt.Errorf("start date cannot be empty")
	}

	if endDate == "" {
		return fmt.Errorf("end date cannot be empty")
	}

	// Basic date format validation (YYYY-MM-DD)
	if len(startDate) != 10 || len(endDate) != 10 {
		return fmt.Errorf("dates must be in YYYY-MM-DD format")
	}

	if startDate > endDate {
		return fmt.Errorf("start date cannot be after end date")
	}

	return nil
}

// isValidAction checks if the action is supported
func isValidAction(action string) bool {
	validActions := map[string]bool{
		"check_in":              true,
		"check_out":             true,
		"absent":                true,
		"get_attendance_report": true,
	}

	return validActions[action]
}

// validateSearchName validates employee search name
func validateSearchName(name string) error {
	name = strings.TrimSpace(name)

	if name == "" {
		return fmt.Errorf("search name cannot be empty")
	}

	if len(name) > 100 {
		return fmt.Errorf("search name too long (max 100 characters)")
	}

	// Allow letters, numbers, spaces, and common name characters
	for _, char := range name {
		if !((char >= 'a' && char <= 'z') ||
			(char >= 'A' && char <= 'Z') ||
			(char >= '0' && char <= '9') ||
			char == ' ' || char == '.' || char == '-' ||
			char == '\'' || char >= 128) { // Allow unicode for Vietnamese names
			return fmt.Errorf("search name contains invalid characters")
		}
	}

	return nil
}
