// Copyright (c) 2023-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"time"

	"github.com/mattermost/mattermost/server/public/model"
)

// API endpoint suffix for ERP (this is fixed)
const ERPEndpointSuffix = "/api/method/frappe.desk.form.save.savedocs"

// EmployeeCheckin represents the data structure for ERPNEXT employee check-in
type EmployeeCheckin struct {
	Docstatus          int    `json:"docstatus"`
	Doctype            string `json:"doctype"`
	Name               string `json:"name"`
	IsLocal            bool   `json:"__islocal"`
	Unsaved            bool   `json:"__unsaved"`
	Owner              string `json:"owner"`
	LogType            string `json:"log_type"`
	Time               string `json:"time"`
	SkipAutoAttendance int    `json:"skip_auto_attendance"`
	Offshift           int    `json:"offshift"`
	EmployeeName       string `json:"employee_name"`
	Employee           string `json:"employee"`
}

// NewEmployeeCheckin creates a new check-in record with default values
// It uses the server time (milliseconds since epoch) from the Mattermost server
func NewEmployeeCheckin(employeeName string, serverTimeMillis int64) (*EmployeeCheckin, string) {
	// Generate a unique name with timestamp and random characters
	uniqueName := fmt.Sprintf("new-employee-checkin-%s", generateUniqueID())

	// Try to get Vietnam time first
	var formattedTime string
	vietTime, err := GetVietnamTime()
	if err == nil {
		// Format Vietnam time in YYYY-MM-DD HH:MM:SS format for ERP
		formattedTime = vietTime.Format("2006-01-02 15:04:05")
	} else {
		// Fallback to server time if Vietnam time fails
		serverTime := time.UnixMilli(serverTimeMillis)
		formattedTime = serverTime.Format("2006-01-02 15:04:05")
	}

	return &EmployeeCheckin{
		Docstatus:          0,
		Doctype:            "Employee Checkin",
		Name:               uniqueName,
		IsLocal:            true,
		Unsaved:            true,
		Owner:              "demo@example.com",
		LogType:            "IN",
		Time:               formattedTime,
		SkipAutoAttendance: 0,
		Offshift:           0,
		EmployeeName:       employeeName,
		Employee:           employeeName,
	}, formattedTime
}

// RecordEmployeeCheckin sends the check-in data to ERPNEXT
// It uses Vietnam time for recording the attendance
func (p *Plugin) RecordEmployeeCheckin(employeeName string) (string, error) {
	p.API.LogDebug("Recording employee check-in", "employee", employeeName)

	// Get ERP configuration from roll call settings
	config := p.getConfiguration()
	erpDomain := config.RollCall.ERPDomain
	erpAPIKey := config.RollCall.ERPAPIKey
	erpAPISecret := config.RollCall.ERPAPISecret

	// Validate configuration
	if erpDomain == "" {
		return "", fmt.Errorf("ERP domain not configured")
	}
	if erpAPIKey == "" {
		return "", fmt.Errorf("ERP API key not configured")
	}
	if erpAPISecret == "" {
		return "", fmt.Errorf("ERP API secret not configured")
	}

	// Combine API key and secret for token
	erpToken := erpAPIKey + ":" + erpAPISecret

	// Build the complete ERP endpoint
	erpEndpoint := strings.TrimSuffix(erpDomain, "/") + ERPEndpointSuffix

	// Get Vietnam time instead of server time
	var serverTime int64
	vietTime, err := GetVietnamTime()
	if err != nil {
		p.API.LogWarn("Failed to get Vietnam time, falling back to server time", "error", err.Error())
		serverTime = model.GetMillis() // Fallback to server time
	} else {
		serverTime = vietTime.UnixMilli()
	}

	checkin, formattedTime := NewEmployeeCheckin(employeeName, serverTime)

	// Create the form data
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	// Marshal the doc to JSON
	docJSON, err := json.Marshal(checkin)
	if err != nil {
		return "", fmt.Errorf("failed to marshal employee checkin: %w", err)
	}

	// Add doc field
	if err := writer.WriteField("doc", string(docJSON)); err != nil {
		return "", fmt.Errorf("failed to write doc field: %w", err)
	}

	// Add action field
	if err := writer.WriteField("action", "Save"); err != nil {
		return "", fmt.Errorf("failed to write action field: %w", err)
	}

	// Close the writer
	if err := writer.Close(); err != nil {
		return "", fmt.Errorf("failed to close multipart writer: %w", err)
	}

	// Create the request
	req, err := http.NewRequest("POST", erpEndpoint, body)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Authorization", "token "+erpToken)

	// Add CORS headers
	req.Header.Set("Access-Control-Allow-Origin", "*")
	req.Header.Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	req.Header.Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

	// Make the request
	client := p.createExternalHTTPClient()
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	// Read the response
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	// Check the response status
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("ERP API error: %s - %s", resp.Status, string(respBody))
	}

	// Log details about the successful check-in including the time used
	p.API.LogDebug("Employee check-in recorded successfully",
		"employee", employeeName,
		"time", formattedTime,
		"status", resp.Status,
		"response", string(respBody))

	// Return the formatted time that was used for the check-in
	return formattedTime, nil
}

// EmployeeCheckout represents the data structure for ERPNEXT employee check-out
type EmployeeCheckout struct {
	Docstatus          int    `json:"docstatus"`
	Doctype            string `json:"doctype"`
	Name               string `json:"name"`
	IsLocal            bool   `json:"__islocal"`
	Unsaved            bool   `json:"__unsaved"`
	Owner              string `json:"owner"`
	LogType            string `json:"log_type"`
	Time               string `json:"time"`
	SkipAutoAttendance int    `json:"skip_auto_attendance"`
	Offshift           int    `json:"offshift"`
	EmployeeName       string `json:"employee_name"`
	Employee           string `json:"employee"`
}

// NewEmployeeCheckout creates a new check-out record with default values
// It uses the server time (milliseconds since epoch) from the Mattermost server
func NewEmployeeCheckout(employeeName string, serverTimeMillis int64) (*EmployeeCheckout, string) {
	// Generate a unique name with timestamp and random characters
	uniqueName := fmt.Sprintf("new-employee-checkout-%s", generateUniqueID())

	// Try to get Vietnam time first
	var formattedTime string
	vietTime, err := GetVietnamTime()
	if err == nil {
		// Format Vietnam time in YYYY-MM-DD HH:MM:SS format for ERP
		formattedTime = vietTime.Format("2006-01-02 15:04:05")
	} else {
		// Fallback to server time if Vietnam time fails
		serverTime := time.UnixMilli(serverTimeMillis)
		formattedTime = serverTime.Format("2006-01-02 15:04:05")
	}

	return &EmployeeCheckout{
		Docstatus:          0,
		Doctype:            "Employee Checkin",
		Name:               uniqueName,
		IsLocal:            true,
		Unsaved:            true,
		Owner:              "demo@example.com",
		LogType:            "OUT",
		Time:               formattedTime,
		SkipAutoAttendance: 0,
		Offshift:           0,
		EmployeeName:       employeeName,
		Employee:           employeeName,
	}, formattedTime
}

// RecordEmployeeCheckout sends the check-out data to ERPNEXT
// It uses Vietnam time for recording the attendance
func (p *Plugin) RecordEmployeeCheckout(employeeName string) (string, error) {
	p.API.LogDebug("Recording employee check-out", "employee", employeeName)

	// Get ERP configuration from roll call settings
	config := p.getConfiguration()
	erpDomain := config.RollCall.ERPDomain
	erpAPIKey := config.RollCall.ERPAPIKey
	erpAPISecret := config.RollCall.ERPAPISecret

	// Validate configuration
	if erpDomain == "" {
		return "", fmt.Errorf("ERP domain not configured")
	}
	if erpAPIKey == "" {
		return "", fmt.Errorf("ERP API key not configured")
	}
	if erpAPISecret == "" {
		return "", fmt.Errorf("ERP API secret not configured")
	}

	// Combine API key and secret for token
	erpToken := erpAPIKey + ":" + erpAPISecret

	// Build the complete ERP endpoint
	erpEndpoint := strings.TrimSuffix(erpDomain, "/") + ERPEndpointSuffix

	// Get Vietnam time instead of server time
	var serverTime int64
	vietTime, err := GetVietnamTime()
	if err != nil {
		p.API.LogWarn("Failed to get Vietnam time, falling back to server time", "error", err.Error())
		serverTime = model.GetMillis() // Fallback to server time
	} else {
		serverTime = vietTime.UnixMilli()
	}

	checkout, formattedTime := NewEmployeeCheckout(employeeName, serverTime)

	// Create the form data
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	// Marshal the doc to JSON
	docJSON, err := json.Marshal(checkout)
	if err != nil {
		return "", fmt.Errorf("failed to marshal employee checkout: %w", err)
	}

	// Add doc field
	if err := writer.WriteField("doc", string(docJSON)); err != nil {
		return "", fmt.Errorf("failed to write doc field: %w", err)
	}

	// Add action field
	if err := writer.WriteField("action", "Save"); err != nil {
		return "", fmt.Errorf("failed to write action field: %w", err)
	}

	// Close the writer
	if err := writer.Close(); err != nil {
		return "", fmt.Errorf("failed to close multipart writer: %w", err)
	}

	// Create the request
	req, err := http.NewRequest("POST", erpEndpoint, body)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Authorization", "token "+erpToken)

	// Add CORS headers
	req.Header.Set("Access-Control-Allow-Origin", "*")
	req.Header.Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	req.Header.Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

	// Make the request
	client := p.createExternalHTTPClient()
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	// Read the response
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	// Check the response status
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("ERP API error: %s - %s", resp.Status, string(respBody))
	}

	// Log details about the successful check-out including the time used
	p.API.LogDebug("Employee check-out recorded successfully",
		"employee", employeeName,
		"time", formattedTime,
		"status", resp.Status,
		"response", string(respBody))

	// Return the formatted time that was used for the check-out
	return formattedTime, nil
}

// RecordEmployeeAbsent sends an absence record to ERPNEXT
func (p *Plugin) RecordEmployeeAbsent(employeeName string, reason string) (string, error) {
	p.API.LogDebug("Recording employee absence", "employee", employeeName, "reason", reason)

	// Get ERP configuration from roll call settings
	// Get ERP configuration from roll call settings
	config := p.getConfiguration()
	erpDomain := config.RollCall.ERPDomain
	erpAPIKey := config.RollCall.ERPAPIKey
	erpAPISecret := config.RollCall.ERPAPISecret

	// Validate configuration
	if erpDomain == "" {
		return "", fmt.Errorf("ERP domain not configured")
	}
	if erpAPIKey == "" {
		return "", fmt.Errorf("ERP API key not configured")
	}
	if erpAPISecret == "" {
		return "", fmt.Errorf("ERP API secret not configured")
	}

	// Combine API key and secret for token
	// erpToken := erpAPIKey + ":" + erpAPISecret

	// Get Vietnam time for the record
	var formattedDate string
	vietTime, err := GetVietnamTime()
	if err == nil {
		// Format Vietnam time in YYYY-MM-DD format for ERP
		formattedDate = vietTime.Format("2006-01-02")
	} else {
		// Fallback to server time if Vietnam time fails
		serverTime := time.Now()
		formattedDate = serverTime.Format("2006-01-02")
	}

	// Here you would implement the actual ERP integration for absences
	// This could involve a different API endpoint or a different request structure
	// For now, we'll just log it
	p.API.LogInfo("Would record in ERP system:",
		"endpoint", erpDomain+ERPEndpointSuffix,
		"token", "[REDACTED]",
		"employee", employeeName,
		"date", formattedDate,
		"reason", reason)

	return formattedDate, nil
}

// GetEmployeeNameFromUser attempts to get the employee name from the user's full name or username
func (p *Plugin) GetEmployeeNameFromUser(user *model.User) string {
	// Try to use full name if available
	if user.FirstName != "" || user.LastName != "" {
		fullName := strings.TrimSpace(user.FirstName + " " + user.LastName)
		if fullName != "" {
			return fullName
		}
	}

	// Fall back to username
	return user.Username
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
