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
