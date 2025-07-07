// Copyright (c) 2023-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package attendance

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"mime/multipart"
	"net/http"
	"net/url"
	"sort"
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

// QueryAttendanceCount queries ERPNext for attendance count with specific status and date range
func (c *ERPClient) QueryAttendanceCount(employeeID, status, startDate, endDate string) (int, error) {
	c.api.LogDebug("Querying attendance count",
		"employee_id", employeeID,
		"status", status,
		"start_date", startDate,
		"end_date", endDate)

	// Validate configuration
	if err := c.validateConfig(); err != nil {
		return 0, err
	}

	// Combine API key and secret for token
	erpToken := c.config.ERPAPIKey + ":" + c.config.ERPAPISecret

	// Build the API endpoint for fetching attendance records
	baseURL := strings.TrimSuffix(c.config.ERPDomain, "/") + "/api/resource/Attendance"

	// Create filters for the query
	filters := fmt.Sprintf(`[["employee","=","%s"],["attendance_date",">=","%s"],["attendance_date","<=","%s"],["status","=","%s"]]`,
		employeeID, startDate, endDate, status)

	// Parse the base URL
	reqURL, err := url.Parse(baseURL)
	if err != nil {
		return 0, fmt.Errorf("failed to parse URL: %w", err)
	}

	// Add query parameters
	query := reqURL.Query()
	query.Add("filters", filters)
	query.Add("fields", `["name","status","attendance_date"]`)
	query.Add("limit_page_length", "1000") // Set a reasonable limit
	reqURL.RawQuery = query.Encode()

	c.api.LogDebug("Making attendance query request", "url", reqURL.String())

	// Create the request
	req, err := http.NewRequest("GET", reqURL.String(), nil)
	if err != nil {
		return 0, fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	req.Header.Set("Authorization", "token "+erpToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	// Make the request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return 0, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	// Read the response
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, fmt.Errorf("failed to read response: %w", err)
	}

	c.api.LogDebug("ERPNext attendance query response",
		"status", resp.Status,
		"body", string(respBody))

	// Check the response status
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("ERP API error: %s - %s", resp.Status, string(respBody))
	}

	// Parse the response
	var apiResponse struct {
		Data []struct {
			Name           string `json:"name"`
			Status         string `json:"status"`
			AttendanceDate string `json:"attendance_date"`
		} `json:"data"`
	}

	if err := json.Unmarshal(respBody, &apiResponse); err != nil {
		return 0, fmt.Errorf("failed to parse response: %w", err)
	}

	count := len(apiResponse.Data)
	c.api.LogDebug("Found attendance records", "count", count)

	return count, nil
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

// GetAllEmployees fetches all employees from ERPNext
func (c *ERPClient) GetAllEmployees() ([]Employee, error) {
	c.api.LogDebug("Getting all employees from ERPNext")

	// Validate configuration
	if err := c.validateConfig(); err != nil {
		return nil, err
	}

	// Combine API key and secret for token
	erpToken := c.config.ERPAPIKey + ":" + c.config.ERPAPISecret

	// Build the API endpoint for fetching all employees
	baseURL := strings.TrimSuffix(c.config.ERPDomain, "/") + "/api/resource/Employee"

	// Parse the base URL
	reqURL, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse URL: %w", err)
	}

	// Add query parameters
	query := reqURL.Query()
	query.Add("fields", `["name","employee_name","custom_chat_id","status","department","designation","employee_number"]`)
	query.Add("filters", `[["status","=","Active"]]`) // Only active employees
	query.Add("limit_page_length", "1000")            // Get more employees
	reqURL.RawQuery = query.Encode()

	c.api.LogDebug("Making request to ERPNext for all employees", "url", reqURL.String())

	// Create the request
	req, err := http.NewRequest("GET", reqURL.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	req.Header.Set("Authorization", "token "+erpToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	// Make the request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	// Read the response
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	c.api.LogDebug("ERPNext API Response for all employees",
		"status", resp.Status,
		"response_length", len(respBody))

	// Check the response status
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ERP API error: %s - %s", resp.Status, string(respBody))
	}

	// Parse the response
	var apiResponse struct {
		Data []Employee `json:"data"`
	}

	if err := json.Unmarshal(respBody, &apiResponse); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	c.api.LogDebug("Found employees", "count", len(apiResponse.Data))

	return apiResponse.Data, nil
}

// SearchEmployeesByName searches for employees by name using fuzzy matching
func (c *ERPClient) SearchEmployeesByName(searchName string) ([]Employee, error) {
	c.api.LogDebug("Searching employees by name", "search_name", searchName)

	// Get all employees first
	allEmployees, err := c.GetAllEmployees()
	if err != nil {
		return nil, fmt.Errorf("failed to get all employees: %w", err)
	}

	// Filter employees using fuzzy matching
	var matchedEmployees []Employee
	searchNameLower := strings.ToLower(searchName)

	for _, employee := range allEmployees {
		// Check multiple fields for matches
		employeeNameLower := strings.ToLower(employee.EmployeeName)
		employeeIDLower := strings.ToLower(employee.Name)
		employeeNumberLower := strings.ToLower(employee.EmployeeNumber)

		// Calculate confidence score
		confidence := calculateNameMatchConfidence(searchNameLower, employeeNameLower, employeeIDLower, employeeNumberLower)

		// Only include employees with confidence above threshold
		if confidence >= 0.7 { // 70% confidence threshold
			employee.MatchConfidence = confidence
			matchedEmployees = append(matchedEmployees, employee)
		}
	}

	// Sort by confidence (highest first)
	sort.Slice(matchedEmployees, func(i, j int) bool {
		return matchedEmployees[i].MatchConfidence > matchedEmployees[j].MatchConfidence
	})

	c.api.LogDebug("Found matching employees", "count", len(matchedEmployees), "search_name", searchName)

	return matchedEmployees, nil
}

// GetAttendanceForEmployee gets attendance data for a specific employee
func (c *ERPClient) GetAttendanceForEmployee(employeeID string, startDate, endDate string) (*EmployeeAttendanceReport, error) {
	c.api.LogDebug("Getting attendance for employee", "employee_id", employeeID, "start_date", startDate, "end_date", endDate)

	// Validate configuration
	if err := c.validateConfig(); err != nil {
		return nil, err
	}

	// Combine API key and secret for token
	erpToken := c.config.ERPAPIKey + ":" + c.config.ERPAPISecret

	// Build the API endpoint for fetching attendance records
	baseURL := strings.TrimSuffix(c.config.ERPDomain, "/") + "/api/resource/Attendance"

	// Create filters for the query
	filters := fmt.Sprintf(`[["employee","=","%s"],["attendance_date",">=","%s"],["attendance_date","<=","%s"]]`,
		employeeID, startDate, endDate)

	// Parse the base URL
	reqURL, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse URL: %w", err)
	}

	// Add query parameters
	query := reqURL.Query()
	query.Add("filters", filters)
	query.Add("fields", `["name","status","attendance_date","in_time","out_time","working_hours","late_entry","early_exit"]`)
	query.Add("limit_page_length", "1000")
	reqURL.RawQuery = query.Encode()

	c.api.LogDebug("Making attendance query request", "url", reqURL.String())

	// Create the request
	req, err := http.NewRequest("GET", reqURL.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	req.Header.Set("Authorization", "token "+erpToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	// Make the request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	// Read the response
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	c.api.LogDebug("ERPNext attendance query response",
		"status", resp.Status,
		"employee_id", employeeID)

	// Check the response status
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ERP API error: %s - %s", resp.Status, string(respBody))
	}

	// Parse the response
	var apiResponse struct {
		Data []AttendanceRecord `json:"data"`
	}

	if err := json.Unmarshal(respBody, &apiResponse); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	// Calculate summary statistics
	report := &EmployeeAttendanceReport{
		EmployeeID: employeeID,
		StartDate:  startDate,
		EndDate:    endDate,
		Records:    apiResponse.Data,
		TotalDays:  len(apiResponse.Data),
	}

	// Calculate statistics
	for _, record := range apiResponse.Data {
		switch record.Status {
		case "Present":
			report.PresentDays++
		case "Absent":
			report.AbsentDays++
		case "Half Day":
			report.HalfDays++
		case "Work From Home":
			report.WorkFromHomeDays++
		}

		if record.LateEntry == 1 {
			report.LateDays++
		}
		if record.EarlyExit == 1 {
			report.EarlyExitDays++
		}
	}

	c.api.LogDebug("Generated attendance report", "employee_id", employeeID, "total_days", report.TotalDays)

	return report, nil
}

// Helper function to calculate name matching confidence
func calculateNameMatchConfidence(searchName, employeeName, employeeID, employeeNumber string) float64 {
	var maxConfidence float64

	// Exact match
	if searchName == employeeName {
		return 1.0
	}

	// Check if search name is contained in employee name
	if strings.Contains(employeeName, searchName) {
		maxConfidence = math.Max(maxConfidence, 0.9)
	}

	// Check if employee name is contained in search name
	if strings.Contains(searchName, employeeName) {
		maxConfidence = math.Max(maxConfidence, 0.85)
	}

	// Check individual words
	searchWords := strings.Fields(searchName)
	employeeWords := strings.Fields(employeeName)

	var matchCount float64
	for _, searchWord := range searchWords {
		for _, empWord := range employeeWords {
			if strings.Contains(empWord, searchWord) || strings.Contains(searchWord, empWord) {
				matchCount++
				break
			}
		}
	}

	if len(searchWords) > 0 {
		wordMatchConfidence := matchCount / float64(len(searchWords)) * 0.8
		maxConfidence = math.Max(maxConfidence, wordMatchConfidence)
	}

	// Check employee ID match
	if strings.Contains(strings.ToLower(employeeID), searchName) {
		maxConfidence = math.Max(maxConfidence, 0.95)
	}

	// Check employee number match
	if employeeNumber != "" && strings.Contains(strings.ToLower(employeeNumber), searchName) {
		maxConfidence = math.Max(maxConfidence, 0.95)
	}

	// Fuzzy string matching using Levenshtein distance
	if len(searchName) > 2 && len(employeeName) > 2 {
		fuzzyConfidence := calculateFuzzyMatch(searchName, employeeName)
		maxConfidence = math.Max(maxConfidence, fuzzyConfidence)
	}

	return maxConfidence
}

// Simple fuzzy matching using Levenshtein distance
func calculateFuzzyMatch(s1, s2 string) float64 {
	if len(s1) == 0 {
		return 0
	}
	if len(s2) == 0 {
		return 0
	}

	// Create matrix
	matrix := make([][]int, len(s1)+1)
	for i := range matrix {
		matrix[i] = make([]int, len(s2)+1)
	}

	// Initialize first row and column
	for i := 0; i <= len(s1); i++ {
		matrix[i][0] = i
	}
	for j := 0; j <= len(s2); j++ {
		matrix[0][j] = j
	}

	// Fill matrix
	for i := 1; i <= len(s1); i++ {
		for j := 1; j <= len(s2); j++ {
			cost := 0
			if s1[i-1] != s2[j-1] {
				cost = 1
			}

			matrix[i][j] = minInt(
				matrix[i-1][j]+1,      // deletion
				matrix[i][j-1]+1,      // insertion
				matrix[i-1][j-1]+cost, // substitution
			)
		}
	}

	distance := matrix[len(s1)][len(s2)]
	maxLen := maxInt(len(s1), len(s2))

	if maxLen == 0 {
		return 1.0
	}

	similarity := 1.0 - float64(distance)/float64(maxLen)

	// Only return as fuzzy match if similarity is reasonable
	if similarity >= 0.6 {
		return similarity * 0.7 // Scale down fuzzy matches
	}

	return 0
}

// Helper function for minimum of 3 integers
func minInt(a, b, c int) int {
	if a <= b && a <= c {
		return a
	}
	if b <= c {
		return b
	}
	return c
}

// Helper function for minimum of 2 integers
func minInt2(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// Helper function for maximum of 2 integers
func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
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
