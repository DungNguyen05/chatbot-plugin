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
	Shift              string `json:"shift"`
	EmployeeName       string `json:"employee_name"`
	Employee           string `json:"employee"`
}

// NewEmployeeCheckin creates a new check-in record with default values
// It uses the server time (milliseconds since epoch) from the Mattermost server
func NewEmployeeCheckin(employeeName string, serverTimeMillis int64) (*EmployeeCheckin, string) {
	// Generate a unique name with timestamp and random characters
	uniqueName := fmt.Sprintf("new-employee-checkin-%s", generateUniqueID())

	// Convert server timestamp (milliseconds) to time object
	serverTime := time.UnixMilli(serverTimeMillis)

	// Format time in YYYY-MM-DD HH:MM:SS format for ERP
	formattedTime := serverTime.Format("2006-01-02 15:04:05")

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
		Shift:              "Hành Chính",
		EmployeeName:       employeeName,
		Employee:           employeeName,
	}, formattedTime
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

// RecordEmployeeCheckin sends the check-in data to ERPNEXT
// It uses the server's time for recording the attendance
// Returns the formatted time string used for recording
func (p *Plugin) RecordEmployeeCheckin(employeeName string) (string, error) {
	p.API.LogDebug("Recording employee check-in", "employee", employeeName)

	// Get current server time (milliseconds since epoch)
	serverTime := model.GetMillis()

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
