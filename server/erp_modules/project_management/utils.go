// Copyright (c) 2023-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package project_management

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/mattermost/mattermost/server/public/model"
)

// detectUserLanguage determines if user prefers Vietnamese or English
func detectUserLanguage(user *model.User) bool {
	return strings.HasPrefix(user.Locale, "vi")
}

// getUserDisplayName returns the best display name for a user
func getUserDisplayName(user *model.User) string {
	// Try to build full name first
	fullName := strings.TrimSpace(user.FirstName + " " + user.LastName)
	if fullName != "" && fullName != " " {
		return fullName
	}

	// Fallback to first name only if last name is not available
	if user.FirstName != "" {
		return user.FirstName
	}

	// Further fallbacks
	if user.Nickname != "" {
		return user.Nickname
	}

	return user.Username
}

// generateUniqueID creates a simple unique ID for the records
func generateUniqueID() string {
	const letters = "abcdefghijklmnopqrstuvwxyz"
	result := make([]byte, 10)
	for i := range result {
		result[i] = letters[time.Now().UnixNano()%int64(len(letters))]
	}
	return string(result)
}

// GetVietnamTime returns the current time in Vietnam timezone (Asia/Ho_Chi_Minh)
func GetVietnamTime() (time.Time, error) {
	loc, err := time.LoadLocation("Asia/Ho_Chi_Minh")
	if err != nil {
		return time.Time{}, err
	}
	return time.Now().In(loc), nil
}

// formatDateForERP formats date for ERP system
func formatDateForERP(t time.Time) string {
	return t.Format("2006-01-02")
}

// convertToMap converts a struct to map[string]interface{}
func convertToMap(data interface{}) map[string]interface{} {
	result := make(map[string]interface{})
	dataBytes, _ := json.Marshal(data)
	json.Unmarshal(dataBytes, &result)
	return result
}

// convertFromMap converts map[string]interface{} to struct
func convertFromMap(data map[string]interface{}, target interface{}) error {
	dataBytes, err := json.Marshal(data)
	if err != nil {
		return err
	}
	return json.Unmarshal(dataBytes, target)
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
