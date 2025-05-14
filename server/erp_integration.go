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

const (
	ERPEndpoint    = "https://erp-demo.workdone.vn/api/method/frappe.desk.form.save.savedocs"
	ERPToken       = "98747a657e0431d:f30282990366777"
	ERPTokenHeader = "token " + ERPToken
)

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
	req, err := http.NewRequest("POST", ERPEndpoint, body)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Authorization", ERPTokenHeader)

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

// RecordEmployeeCheckout sends the check-out data to ERPNEXT
func (p *Plugin) RecordEmployeeCheckout(employeeName string, checkoutTime string) (string, error) {
	p.API.LogDebug("Recording employee check-out", "employee", employeeName, "time", checkoutTime)

	// Create checkout record
	checkout := &EmployeeCheckout{
		Docstatus:          0,
		Doctype:            "Employee Checkin",
		Name:               fmt.Sprintf("new-employee-checkout-%s", generateUniqueID()),
		IsLocal:            true,
		Unsaved:            true,
		Owner:              "demo@example.com",
		LogType:            "OUT", // Set as checkout
		Time:               checkoutTime,
		SkipAutoAttendance: 0,
		Offshift:           0,
		EmployeeName:       employeeName,
		Employee:           employeeName,
	}

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
	req, err := http.NewRequest("POST", ERPEndpoint, body)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Authorization", ERPTokenHeader)

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

	// Log details about the successful check-out
	p.API.LogDebug("Employee check-out recorded successfully",
		"employee", employeeName,
		"time", checkoutTime,
		"status", resp.Status)

	// Return the formatted time that was used for the check-out
	return checkoutTime, nil
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
