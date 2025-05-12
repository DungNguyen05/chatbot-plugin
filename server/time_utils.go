// Copyright (c) 2023-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package main

import (
	"time"
)

// GetVietnamTime returns the current time in Vietnam timezone (Asia/Ho_Chi_Minh)
// This is used specifically for the roll call feature where timestamps need to be
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

// FormatVietnamTime returns the current time in Vietnam timezone formatted as "2006-01-02 15:04:05"
// This is the standard format expected by the ERP system
func FormatVietnamTime() (string, error) {
	vietTime, err := GetVietnamTime()
	if err != nil {
		return "", err
	}

	return vietTime.Format("2006-01-02 15:04:05"), nil
}
