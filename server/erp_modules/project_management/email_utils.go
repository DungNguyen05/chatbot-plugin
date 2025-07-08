// Copyright (c) 2023-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package project_management

import (
	"fmt"
	"strings"
)

// validateEmail validates email format (basic validation)
func validateEmail(email string) error {
	if email == "" {
		return fmt.Errorf("email cannot be empty")
	}

	if !strings.Contains(email, "@") {
		return fmt.Errorf("invalid email format: missing @")
	}

	if !strings.Contains(email, ".") {
		return fmt.Errorf("invalid email format: missing domain")
	}

	return nil
}

// extractEmailDomain extracts domain from email
func extractEmailDomain(email string) string {
	parts := strings.Split(email, "@")
	if len(parts) == 2 {
		return parts[1]
	}
	return ""
}

// isValidEmailForAssignment checks if email is suitable for assignment
func isValidEmailForAssignment(email string) bool {
	if err := validateEmail(email); err != nil {
		return false
	}

	// Additional checks can be added here
	// For example, check against company domain, blacklisted domains, etc.

	return true
}

// getDefaultAssigneeEmail returns a default email for assignments when no assignee is found
func getDefaultAssigneeEmail() string {
	return "demo@example.com"
}

// formatEmailForDisplay formats email for display in messages
func formatEmailForDisplay(email string) string {
	if email == "" {
		return "No email"
	}

	// Could implement masking for privacy if needed
	// For now, return as-is
	return email
}
