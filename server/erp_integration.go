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
func NewEmployeeCheckin(employeeID string, serverTimeMillis int64) (*EmployeeCheckin, string) {
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
		EmployeeName:       employeeID, // This should be the ERPNext employee ID
		Employee:           employeeID, // This should be the ERPNext employee ID
	}, formattedTime
}

// RecordEmployeeCheckin sends the check-in data to ERPNEXT
// It uses Vietnam time for recording the attendance
func (p *Plugin) RecordEmployeeCheckin(employeeID string) (string, error) {
	p.API.LogDebug("Recording employee check-in", "employee_id", employeeID)

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

	checkin, formattedTime := NewEmployeeCheckin(employeeID, serverTime)

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
		"employee_id", employeeID,
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
func NewEmployeeCheckout(employeeID string, serverTimeMillis int64) (*EmployeeCheckout, string) {
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
		EmployeeName:       employeeID, // This should be the ERPNext employee ID
		Employee:           employeeID, // This should be the ERPNext employee ID
	}, formattedTime
} // RecordEmployeeCheckout - modify similarly
func (p *Plugin) RecordEmployeeCheckout(employeeID string) (string, error) {
	p.API.LogDebug("Recording employee check-out", "employee_id", employeeID)

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

	checkout, formattedTime := NewEmployeeCheckout(employeeID, serverTime)

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
		"employee_id", employeeID,
		"time", formattedTime,
		"status", resp.Status,
		"response", string(respBody))

	// Return the formatted time that was used for the check-out
	return formattedTime, nil
}

// RecordEmployeeAbsent - modify to use employee ID
func (p *Plugin) RecordEmployeeAbsent(employeeID string, reason string) (string, error) {
	p.API.LogDebug("Recording employee absence", "employee_id", employeeID, "reason", reason)

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
		"employee_id", employeeID,
		"date", formattedDate,
		"reason", reason)

	return formattedDate, nil
}

func (p *Plugin) GetEmployeeIDFromUser(user *model.User) (string, error) {
	// Use the user's ID as the chat ID to lookup in ERPNext
	chatID := user.Id

	employeeID, err := p.GetEmployeeByChatID(chatID)
	if err != nil {
		return "", fmt.Errorf("failed to get employee by chat ID %s: %w", chatID, err)
	}

	return employeeID, nil
}

// GetEmployeeByChatID fetches employee information from ERPNext using chat ID
func (p *Plugin) GetEmployeeByChatID(chatID string) (string, error) {
	p.API.LogDebug("Getting employee by chat ID", "chat_id", chatID)

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

	// Build the API endpoint for fetching employee by custom_chat_id
	baseURL := strings.TrimSuffix(erpDomain, "/") + "/api/resource/Employee"

	// Try different URL formats for ERPNext API
	urls := []string{
		// Format 1: Standard ERPNext filter format
		fmt.Sprintf(`%s?fields=["name","employee_name","custom_chat_id"]&filters=[["custom_chat_id","=","%s"]]`, baseURL, chatID),
		// Format 2: JSON object filter format
		fmt.Sprintf(`%s?fields=["name","employee_name","custom_chat_id"]&filters={"custom_chat_id":"%s"}`, baseURL, chatID),
		// Format 3: Simple filter format
		fmt.Sprintf(`%s?fields=["name","employee_name","custom_chat_id"]&custom_chat_id=%s`, baseURL, chatID),
	}

	for i, testURL := range urls {
		p.API.LogDebug("Trying URL format", "attempt", i+1, "url", testURL)

		// Create the request
		req, err := http.NewRequest("GET", testURL, nil)
		if err != nil {
			p.API.LogError("Failed to create request", "error", err.Error())
			continue
		}

		// Set headers
		req.Header.Set("Authorization", "token "+erpToken)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")

		// Make the request
		client := p.createExternalHTTPClient()
		resp, err := client.Do(req)
		if err != nil {
			p.API.LogError("Failed to send request", "error", err.Error())
			continue
		}

		// Read the response
		respBody, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			p.API.LogError("Failed to read response", "error", err.Error())
			continue
		}

		p.API.LogDebug("ERPNext API Response", "attempt", i+1, "status", resp.Status, "body", string(respBody))

		// Check the response status
		if resp.StatusCode >= 400 {
			p.API.LogError("ERP API error", "status", resp.Status, "body", string(respBody))
			continue
		}

		// Parse the response
		var apiResponse struct {
			Data []struct {
				Name         string `json:"name"`
				EmployeeName string `json:"employee_name"`
				CustomChatID string `json:"custom_chat_id"`
			} `json:"data"`
		}

		if err := json.Unmarshal(respBody, &apiResponse); err != nil {
			p.API.LogError("Failed to parse response", "error", err.Error())
			continue
		}

		// Filter results manually if the API didn't filter properly
		var matchedEmployees []struct {
			Name         string `json:"name"`
			EmployeeName string `json:"employee_name"`
			CustomChatID string `json:"custom_chat_id"`
		}

		for _, emp := range apiResponse.Data {
			if emp.CustomChatID == chatID {
				matchedEmployees = append(matchedEmployees, emp)
			}
		}

		// Check if employee found
		if len(matchedEmployees) == 0 {
			// If this was the last URL format to try, return error
			if i == len(urls)-1 {
				return "", fmt.Errorf("no employee found with chat_id: %s", chatID)
			}
			// Otherwise, try next URL format
			continue
		}

		if len(matchedEmployees) > 1 {
			return "", fmt.Errorf("multiple employees found with chat_id: %s", chatID)
		}

		p.API.LogDebug("Found employee", "employee_id", matchedEmployees[0].Name, "employee_name", matchedEmployees[0].EmployeeName)

		// Return the employee name (ID) for ERPNext operations
		return matchedEmployees[0].Name, nil
	}

	return "", fmt.Errorf("failed to get employee with all URL formats tried")
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
