// Copyright (c) 2023-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package project_management

import (
	"strings"
	"time"

	"github.com/mattermost/mattermost/server/public/model"
)

// detectUserLanguage determines if user prefers Vietnamese or English
func detectUserLanguage(user *model.User) bool {
	return strings.HasPrefix(user.Locale, "vi")
}

// generateUniqueID creates a simple unique ID
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

// Helper functions

// minInt returns the minimum of 3 integers
func minInt(a, b, c int) int {
	if a <= b && a <= c {
		return a
	}
	if b <= c {
		return b
	}
	return c
}

// minInt4 returns the minimum of 4 integers
func minInt4(a, b, c, d int) int {
	return minInt(minInt(a, b, c), d, d)
}

// maxInt returns the maximum of 2 integers
func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
