// Copyright (c) 2023-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package main

import (
	"fmt"
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

// ParseCheckoutTime parses and validates the checkout time format (HH:MM:SS)
func ParseCheckoutTime(timeStr string) (time.Time, error) {
	if timeStr == "" {
		return time.Time{}, fmt.Errorf("checkout time is not configured")
	}

	// Parse the time
	layout := "15:04:05" // 24-hour format
	t, err := time.Parse(layout, timeStr)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid checkout time format (must be HH:MM:SS): %w", err)
	}

	return t, nil
}

// GetCheckoutTimeForToday returns a time.Time for today with the configured checkout time
func GetCheckoutTimeForToday(configuredTime string) (time.Time, error) {
	// Parse the configured time
	parsedTime, err := ParseCheckoutTime(configuredTime)
	if err != nil {
		return time.Time{}, err
	}

	// Get current date in Vietnam timezone
	vietNow, err := GetVietnamTime()
	if err != nil {
		return time.Time{}, err
	}

	// Combine current date with configured time
	checkoutTime := time.Date(
		vietNow.Year(), vietNow.Month(), vietNow.Day(),
		parsedTime.Hour(), parsedTime.Minute(), parsedTime.Second(),
		0, vietNow.Location())

	// If the configured time is already past for today, return error
	if checkoutTime.Before(vietNow) {
		return time.Time{}, fmt.Errorf("configured checkout time %s has already passed for today", configuredTime)
	}

	return checkoutTime, nil
}

// FormatTimeForERP formats time for ERP in the standard format
func FormatTimeForERP(t time.Time) string {
	return t.Format("2006-01-02 15:04:05")
}
