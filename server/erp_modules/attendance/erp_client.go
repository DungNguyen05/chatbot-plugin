// Copyright (c) 2023-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package attendance

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

// ERPClient handles all ERP system integration
type ERPClient struct {
	config     AttendanceConfig
	httpClient *http.Client
	api        PluginAPI
}

// NewERPClient creates a new ERP client
func NewERPClient(config AttendanceConfig, httpClient *http.Client, api PluginAPI) *ERPClient {
	return &ERPClient{
		config:     config,
		httpClient: httpClient,
		api:        api,
	}
}

// GetEmployeeByChatID fetches employee information from ERPNext using chat ID
func (c *ERPClient) GetEmployeeByChatID(chatID string) (string, error) {
	c.api.LogDebug("Getting employee by chat ID", "chat_id", chatID)

	// Validate configuration
	if err := c.validateConfig(); err != nil {
		return "", err
	}

	// Combine API key and secret for token
	erpToken := c.config.ERPAPIKey + ":" + c.config.ERPAPISecret

	// Build the API endpoint for fetching employee by custom_chat_id
	baseURL := strings.TrimSuffix(c.config.ERPDomain, "/") + "/api/resource/Employee"

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

	c.api.LogDebug("Making request to ERPNext", "url", reqURL.String())

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
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	// Read the response
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	c.api.LogDebug("ERPNext API Response",
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

	c.api.LogDebug("Found employees", "count", len(apiResponse.Data))

	// Check if employee found with exact match
	if len(apiResponse.Data) > 0 {
		employee := apiResponse.Data[0]
		c.api.LogDebug("Found employee",
			"employee_id", employee.Name,
			"employee_name", employee.EmployeeName,
			"custom_chat_id", employee.CustomChatID)
		return employee.Name, nil
	}

	return "", fmt.Errorf("no employee found with chat_id: %s", chatID)
}

// RecordEmployeeCheckin sends the check-in data to ERPNEXT
func (c *ERPClient) RecordEmployeeCheckin(employeeID string) (string, error) {
	c.api.LogDebug("Recording employee check-in", "employee_id", employeeID)

	if err := c.validateConfig(); err != nil {
		return "", err
	}

	erpToken := c.config.ERPAPIKey + ":" + c.config.ERPAPISecret
	erpEndpoint := strings.TrimSuffix(c.config.ERPDomain, "/") + ERPEndpointSuffix

	// Get Vietnam time
	serverTime, err := c.getServerTime()
	if err != nil {
		return "", err
	}

	checkin, formattedTime := NewEmployeeCheckin(employeeID, serverTime)

	// Send to ERP
	if err := c.sendToERP(erpEndpoint, erpToken, checkin); err != nil {
		return "", err
	}

	c.api.LogDebug("Employee check-in recorded successfully",
		"employee_id", employeeID,
		"time", formattedTime)

	return formattedTime, nil
}

// RecordEmployeeCheckout sends the check-out data to ERPNEXT
func (c *ERPClient) RecordEmployeeCheckout(employeeID string) (string, error) {
	c.api.LogDebug("Recording employee check-out", "employee_id", employeeID)

	if err := c.validateConfig(); err != nil {
		return "", err
	}

	erpToken := c.config.ERPAPIKey + ":" + c.config.ERPAPISecret
	erpEndpoint := strings.TrimSuffix(c.config.ERPDomain, "/") + ERPEndpointSuffix

	// Get Vietnam time
	serverTime, err := c.getServerTime()
	if err != nil {
		return "", err
	}

	checkout, formattedTime := NewEmployeeCheckout(employeeID, serverTime)

	// Send to ERP
	if err := c.sendToERP(erpEndpoint, erpToken, checkout); err != nil {
		return "", err
	}

	c.api.LogDebug("Employee check-out recorded successfully",
		"employee_id", employeeID,
		"time", formattedTime)

	return formattedTime, nil
}

// RecordEmployeeAbsent records employee absence in ERP
func (c *ERPClient) RecordEmployeeAbsent(employeeID string, reason string) (string, error) {
	c.api.LogDebug("Recording employee absence", "employee_id", employeeID, "reason", reason)

	if err := c.validateConfig(); err != nil {
		return "", err
	}

	erpToken := c.config.ERPAPIKey + ":" + c.config.ERPAPISecret
	erpEndpoint := strings.TrimSuffix(c.config.ERPDomain, "/") + ERPEndpointSuffix

	// Get Vietnam time
	serverTime, err := c.getServerTime()
	if err != nil {
		return "", err
	}

	attendance, formattedDate := NewEmployeeAttendance(employeeID, reason, serverTime)

	// Send to ERP
	if err := c.sendToERP(erpEndpoint, erpToken, attendance); err != nil {
		return "", err
	}

	c.api.LogDebug("Employee absence recorded successfully",
		"employee_id", employeeID,
		"date", formattedDate,
		"reason", reason)

	return formattedDate, nil
}

// validateConfig validates the ERP configuration
func (c *ERPClient) validateConfig() error {
	if c.config.ERPDomain == "" {
		return fmt.Errorf("ERP domain not configured")
	}
	if c.config.ERPAPIKey == "" {
		return fmt.Errorf("ERP API key not configured")
	}
	if c.config.ERPAPISecret == "" {
		return fmt.Errorf("ERP API secret not configured")
	}
	return nil
}

// getServerTime gets the appropriate server time
func (c *ERPClient) getServerTime() (int64, error) {
	vietTime, err := GetVietnamTime()
	if err != nil {
		c.api.LogWarn("Failed to get Vietnam time, falling back to server time", "error", err.Error())
		return model.GetMillis(), nil
	}
	return vietTime.UnixMilli(), nil
}

// sendToERP sends data to ERP system
func (c *ERPClient) sendToERP(endpoint, token string, doc interface{}) error {
	// Create the form data
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	// Marshal the doc to JSON
	docJSON, err := json.Marshal(doc)
	if err != nil {
		return fmt.Errorf("failed to marshal document: %w", err)
	}

	// Add doc field
	if err := writer.WriteField("doc", string(docJSON)); err != nil {
		return fmt.Errorf("failed to write doc field: %w", err)
	}

	// Add action field
	if err := writer.WriteField("action", "Save"); err != nil {
		return fmt.Errorf("failed to write action field: %w", err)
	}

	// Close the writer
	if err := writer.Close(); err != nil {
		return fmt.Errorf("failed to close multipart writer: %w", err)
	}

	// Create the request
	req, err := http.NewRequest("POST", endpoint, body)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Authorization", "token "+token)
	req.Header.Set("Access-Control-Allow-Origin", "*")
	req.Header.Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	req.Header.Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

	// Make the request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	// Read the response
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}

	// Check the response status
	if resp.StatusCode >= 400 {
		return fmt.Errorf("ERP API error: %s - %s", resp.Status, string(respBody))
	}

	return nil
}

// NewEmployeeCheckin creates a new check-in record with default values
func NewEmployeeCheckin(employeeID string, serverTimeMillis int64) (*EmployeeCheckin, string) {
	uniqueName := fmt.Sprintf("new-employee-checkin-%s", generateUniqueID())

	formattedTime := formatTimeForERP(serverTimeMillis)

	return &EmployeeCheckin{
		Docstatus:          1,
		Doctype:            "Employee Checkin",
		Name:               uniqueName,
		IsLocal:            true,
		Unsaved:            true,
		Owner:              "demo@example.com",
		LogType:            "IN",
		Time:               formattedTime,
		SkipAutoAttendance: 0,
		Offshift:           0,
		EmployeeName:       employeeID,
		Employee:           employeeID,
	}, formattedTime
}

// NewEmployeeCheckout creates a new check-out record with default values
func NewEmployeeCheckout(employeeID string, serverTimeMillis int64) (*EmployeeCheckin, string) {
	uniqueName := fmt.Sprintf("new-employee-checkout-%s", generateUniqueID())

	formattedTime := formatTimeForERP(serverTimeMillis)

	return &EmployeeCheckin{
		Docstatus:          1,
		Doctype:            "Employee Checkin",
		Name:               uniqueName,
		IsLocal:            true,
		Unsaved:            true,
		Owner:              "demo@example.com",
		LogType:            "OUT",
		Time:               formattedTime,
		SkipAutoAttendance: 0,
		Offshift:           0,
		EmployeeName:       employeeID,
		Employee:           employeeID,
	}, formattedTime
}

// NewEmployeeAttendance creates a new attendance record for absence
func NewEmployeeAttendance(employeeID string, reason string, serverTimeMillis int64) (*EmployeeAttendance, string) {
	uniqueName := fmt.Sprintf("new-employee-attendance-%s", generateUniqueID())

	formattedDate := formatDateForERP(serverTimeMillis)

	return &EmployeeAttendance{
		Docstatus:      1,
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
		Company:        "",
	}, formattedDate
}

// formatTimeForERP formats time for ERP system
func formatTimeForERP(serverTimeMillis int64) string {
	vietTime, err := GetVietnamTime()
	if err == nil {
		return vietTime.Format("2006-01-02 15:04:05")
	}
	serverTime := time.UnixMilli(serverTimeMillis)
	return serverTime.Format("2006-01-02 15:04:05")
}

// formatDateForERP formats date for ERP system
func formatDateForERP(serverTimeMillis int64) string {
	vietTime, err := GetVietnamTime()
	if err == nil {
		return vietTime.Format("2006-01-02")
	}
	serverTime := time.UnixMilli(serverTimeMillis)
	return serverTime.Format("2006-01-02")
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

// GetVietnamTime returns the current time in Vietnam timezone (Asia/Ho_Chi_Minh)
func GetVietnamTime() (time.Time, error) {
	loc, err := time.LoadLocation("Asia/Ho_Chi_Minh")
	if err != nil {
		return time.Time{}, err
	}
	return time.Now().In(loc), nil
}
