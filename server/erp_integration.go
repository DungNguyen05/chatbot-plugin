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
	"net/url"
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

// EmployeeAttendance represents the data structure for ERPNEXT employee attendance (for absence)
type EmployeeAttendance struct {
	Docstatus      int    `json:"docstatus"`
	Doctype        string `json:"doctype"`
	Name           string `json:"name"`
	IsLocal        bool   `json:"__islocal"`
	Unsaved        bool   `json:"__unsaved"`
	Owner          string `json:"owner"`
	Employee       string `json:"employee"`
	EmployeeName   string `json:"employee_name"`
	AttendanceDate string `json:"attendance_date"`
	Status         string `json:"status"`
	Leave          string `json:"leave_type,omitempty"`
	Reason         string `json:"reason,omitempty"`
	Company        string `json:"company,omitempty"`
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
		Docstatus:          1, // Set to 1 to submit the document
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
		Docstatus:          1, // Set to 1 to submit the document
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
}

// NewEmployeeAttendance creates a new attendance record for absence
func NewEmployeeAttendance(employeeID string, reason string, serverTimeMillis int64) (*EmployeeAttendance, string) {
	// Generate a unique name with timestamp and random characters
	uniqueName := fmt.Sprintf("new-employee-attendance-%s", generateUniqueID())

	// Try to get Vietnam time first
	var formattedDate string
	vietTime, err := GetVietnamTime()
	if err == nil {
		// Format Vietnam time in YYYY-MM-DD format for ERP
		formattedDate = vietTime.Format("2006-01-02")
	} else {
		// Fallback to server time if Vietnam time fails
		serverTime := time.UnixMilli(serverTimeMillis)
		formattedDate = serverTime.Format("2006-01-02")
	}

	return &EmployeeAttendance{
		Docstatus:      1, // Set to 1 to submit the document
		Doctype:        "Attendance",
		Name:           uniqueName,
		IsLocal:        true,
		Unsaved:        true,
		Owner:          "demo@example.com",
		Employee:       employeeID,
		EmployeeName:   employeeID,
		AttendanceDate: formattedDate,
		Status:         "Absent",
		Reason:         reason,
		Company:        "", // You may want to set this based on your ERP setup
	}, formattedDate
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

// RecordEmployeeCheckout - modify similarly
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

// RecordEmployeeAbsent - COMPLETE IMPLEMENTATION
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

	// Combine API key and secret for token
	erpToken := erpAPIKey + ":" + erpAPISecret

	// Build the complete ERP endpoint
	erpEndpoint := strings.TrimSuffix(erpDomain, "/") + ERPEndpointSuffix

	// Get Vietnam time for the record
	var serverTime int64
	vietTime, err := GetVietnamTime()
	if err != nil {
		p.API.LogWarn("Failed to get Vietnam time, falling back to server time", "error", err.Error())
		serverTime = model.GetMillis() // Fallback to server time
	} else {
		serverTime = vietTime.UnixMilli()
	}

	attendance, formattedDate := NewEmployeeAttendance(employeeID, reason, serverTime)

	// Create the form data
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	// Marshal the doc to JSON
	docJSON, err := json.Marshal(attendance)
	if err != nil {
		return "", fmt.Errorf("failed to marshal employee attendance: %w", err)
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

	// Log details about the successful absence recording
	p.API.LogDebug("Employee absence recorded successfully",
		"employee_id", employeeID,
		"date", formattedDate,
		"reason", reason,
		"status", resp.Status,
		"response", string(respBody))

	// Return the formatted date that was used for the absence
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

	// Create the filter parameter - use exact match first, then try partial match
	filterParam := fmt.Sprintf(`[["custom_chat_id","=","%s"]]`, chatID)

	// Parse the base URL
	reqURL, err := url.Parse(baseURL)
	if err != nil {
		return "", fmt.Errorf("failed to parse URL: %w", err)
	}

	// Add query parameters
	query := reqURL.Query()
	query.Add("filters", filterParam)
	query.Add("fields", `["name","employee_name","custom_chat_id"]`)
	reqURL.RawQuery = query.Encode()

	p.API.LogDebug("Making request to ERPNext", "url", reqURL.String())

	// Create the request
	req, err := http.NewRequest("GET", reqURL.String(), nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	req.Header.Set("Authorization", "token "+erpToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

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

	p.API.LogDebug("ERPNext API Response",
		"status", resp.Status,
		"body", string(respBody))

	// Check the response status
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("ERP API error: %s - %s", resp.Status, string(respBody))
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
		return "", fmt.Errorf("failed to parse response: %w", err)
	}

	p.API.LogDebug("Found employees", "count", len(apiResponse.Data))

	// Check if employee found with exact match
	if len(apiResponse.Data) > 0 {
		employee := apiResponse.Data[0]
		p.API.LogDebug("Found employee",
			"employee_id", employee.Name,
			"employee_name", employee.EmployeeName,
			"custom_chat_id", employee.CustomChatID)
		return employee.Name, nil
	}

	// If exact match failed, try partial match (like search)
	p.API.LogDebug("Exact match failed, trying partial match")

	// Create partial match filter
	partialFilterParam := fmt.Sprintf(`[["custom_chat_id","like","%%%s%%"]]`, chatID)

	// Update query with partial match
	query = reqURL.Query()
	query.Set("filters", partialFilterParam) // Use Set instead of Add to replace
	reqURL.RawQuery = query.Encode()

	p.API.LogDebug("Making partial match request", "url", reqURL.String())

	// Create new request for partial match
	req, err = http.NewRequest("GET", reqURL.String(), nil)
	if err != nil {
		return "", fmt.Errorf("failed to create partial match request: %w", err)
	}

	req.Header.Set("Authorization", "token "+erpToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	// Make the partial match request
	resp, err = client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to send partial match request: %w", err)
	}
	defer resp.Body.Close()

	// Read the response
	respBody, err = io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read partial match response: %w", err)
	}

	p.API.LogDebug("ERPNext API Partial Match Response",
		"status", resp.Status,
		"body", string(respBody))

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("ERP API partial match error: %s - %s", resp.Status, string(respBody))
	}

	// Parse the partial match response
	if err := json.Unmarshal(respBody, &apiResponse); err != nil {
		return "", fmt.Errorf("failed to parse partial match response: %w", err)
	}

	p.API.LogDebug("Found employees with partial match", "count", len(apiResponse.Data))

	// Check results from partial match
	if len(apiResponse.Data) == 0 {
		return "", fmt.Errorf("no employee found with chat_id: %s", chatID)
	}

	if len(apiResponse.Data) > 1 {
		p.API.LogWarn("Multiple employees found with similar chat_id",
			"chat_id", chatID,
			"count", len(apiResponse.Data))
		// Log all matches for debugging
		for i, emp := range apiResponse.Data {
			p.API.LogDebug("Match",
				"index", i,
				"employee_id", emp.Name,
				"employee_name", emp.EmployeeName,
				"custom_chat_id", emp.CustomChatID)
		}
	}

	// Return the first matching employee
	employee := apiResponse.Data[0]
	p.API.LogDebug("Using first match",
		"employee_id", employee.Name,
		"employee_name", employee.EmployeeName,
		"custom_chat_id", employee.CustomChatID)

	return employee.Name, nil
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
