// Copyright (c) 2023-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package main

import (
	"time"
)

// GetVietnamTime returns the current time in Vietnam timezone (Asia/Ho_Chi_Minh)
// This is used for the attendance feature where timestamps need to be
// in Vietnam local time for ERP integration
func GetVietnamTime() (time.Time, error) {
	// Load Vietnam timezone (Ho Chi Minh City)
	loc, err := time.LoadLocation("Asia/Ho_Chi_Minh")
	if err != nil {
		return time.Time{}, err
	}

	// Get current time in Vietnam timezone
	return time.Now().In(loc), nil
}

// FormatTimeForERP formats time for ERP in the standard format
func FormatTimeForERP(t time.Time) string {
	return t.Format("2006-01-02 15:04:05")
}
