// Copyright (c) 2023-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package attendance

import (
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

// formatTimeForERP formats time for ERP system
func formatTimeForERP(serverTimeMillis int64) string {
	vietTime, err := GetVietnamTime()
	if err == nil {
		return vietTime.Format("2006-01-02 15:04:05")
	}
	serverTime := time.UnixMilli(serverTimeMillis)
	return serverTime.Format("2006-01-02 15:04:05")
}

// formatDateForERP formats date for ERP system
func formatDateForERP(serverTimeMillis int64) string {
	vietTime, err := GetVietnamTime()
	if err == nil {
		return vietTime.Format("2006-01-02")
	}
	serverTime := time.UnixMilli(serverTimeMillis)
	return serverTime.Format("2006-01-02")
}

// generateUniqueID creates a simple unique ID for the checkin record
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

// getTimeOfDay returns a string describing the time of day
func getTimeOfDay(t time.Time) string {
	hour := t.Hour()

	switch {
	case hour >= 5 && hour < 12:
		return "morning"
	case hour >= 12 && hour < 17:
		return "afternoon"
	case hour >= 17 && hour < 21:
		return "evening"
	default:
		return "night"
	}
}

// getVietnameseDayOfWeek returns Vietnamese day of week
func getVietnameseDayOfWeek(weekday time.Weekday) string {
	switch weekday {
	case time.Sunday:
		return "Chủ nhật"
	case time.Monday:
		return "Thứ hai"
	case time.Tuesday:
		return "Thứ ba"
	case time.Wednesday:
		return "Thứ tư"
	case time.Thursday:
		return "Thứ năm"
	case time.Friday:
		return "Thứ sáu"
	case time.Saturday:
		return "Thứ bảy"
	default:
		return "Chủ nhật"
	}
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

// Helper function for minimum of 2 integers
func minInt2(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// Helper function for maximum of 2 integers
func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
