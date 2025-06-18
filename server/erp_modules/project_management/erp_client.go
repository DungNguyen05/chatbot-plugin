// Copyright (c) 2023-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package project_management

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
)

// ERPClient handles all ERP system integration for project management
type ERPClient struct {
	config     ProjectManagementConfig
	httpClient *http.Client
	api        PluginAPI
}

// NewERPClient creates a new ERP client for project management
func NewERPClient(config ProjectManagementConfig, httpClient *http.Client, api PluginAPI) *ERPClient {
	return &ERPClient{
		config:     config,
		httpClient: httpClient,
		api:        api,
	}
}

// GetEmployeeByChatID fetches employee information from ERPNext using chat ID
func (c *ERPClient) GetEmployeeByChatID(chatID string) (string, error) {
	c.api.LogDebug("Getting employee by chat ID for project management", "chat_id", chatID)

	// Validate configuration
	if err := c.validateConfig(); err != nil {
		return "", err
	}

	// Combine API key and secret for token
	erpToken := c.config.ERPAPIKey + ":" + c.config.ERPAPISecret

	// Build the API endpoint for fetching employee by custom_chat_id
	baseURL := strings.TrimSuffix(c.config.ERPDomain, "/") + "/api/resource/Employee"

	// Create the filter parameter
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

	c.api.LogDebug("Making request to ERPNext for employee lookup", "url", reqURL.String())

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

	c.api.LogDebug("ERPNext API Response for employee lookup",
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

	c.api.LogDebug("Found employees for project management", "count", len(apiResponse.Data))

	// Check if employee found
	if len(apiResponse.Data) > 0 {
		employee := apiResponse.Data[0]
		c.api.LogDebug("Found employee for project management",
			"employee_id", employee.Name,
			"employee_name", employee.EmployeeName,
			"custom_chat_id", employee.CustomChatID)
		return employee.Name, nil
	}

	return "", fmt.Errorf("no employee found with chat_id: %s", chatID)
}

// CreateProject creates a new project in ERPNext
func (c *ERPClient) CreateProject(projectData ProjectCreationRequest, creatorEmployeeID string) (string, error) {
	c.api.LogDebug("Creating project in ERPNext", "project_name", projectData.ProjectName)

	if err := c.validateConfig(); err != nil {
		return "", err
	}

	erpToken := c.config.ERPAPIKey + ":" + c.config.ERPAPISecret
	erpEndpoint := strings.TrimSuffix(c.config.ERPDomain, "/") + ERPEndpointSuffix

	// Create project document
	project := NewProject(projectData, creatorEmployeeID)

	// Send to ERP
	if err := c.sendToERP(erpEndpoint, erpToken, project); err != nil {
		return "", err
	}

	c.api.LogDebug("Project created successfully in ERPNext",
		"project_name", projectData.ProjectName,
		"creator", creatorEmployeeID)

	return project.ProjectName, nil
}

// CreateTask creates a new task in ERPNext
func (c *ERPClient) CreateTask(taskData TaskCreationRequest, creatorEmployeeID string) (string, error) {
	c.api.LogDebug("Creating task in ERPNext", "task_subject", taskData.Subject)

	if err := c.validateConfig(); err != nil {
		return "", err
	}

	erpToken := c.config.ERPAPIKey + ":" + c.config.ERPAPISecret
	erpEndpoint := strings.TrimSuffix(c.config.ERPDomain, "/") + ERPEndpointSuffix

	// Create task document
	task := NewTask(taskData, creatorEmployeeID)

	// Send to ERP
	if err := c.sendToERP(erpEndpoint, erpToken, task); err != nil {
		return "", err
	}

	c.api.LogDebug("Task created successfully in ERPNext",
		"task_subject", taskData.Subject,
		"creator", creatorEmployeeID)

	return task.Subject, nil
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

// NewProject creates a new project document with default values
func NewProject(data ProjectCreationRequest, creatorEmployeeID string) *Project {
	uniqueName := fmt.Sprintf("new-project-%s", generateUniqueID())

	project := &Project{
		Docstatus:   1,
		Doctype:     "Project",
		Name:        uniqueName,
		IsLocal:     true,
		Unsaved:     true,
		Owner:       "demo@example.com",
		ProjectName: data.ProjectName,
		Status:      "Open",
		Company:     "",
	}

	// Only set optional fields if they are not empty
	if data.Priority != "" {
		project.Priority = data.Priority
	}
	if data.ProjectType != "" {
		project.ProjectType = data.ProjectType
	}
	if data.Description != "" {
		project.Description = data.Description
	}
	if data.ExpectedStartDate != "" {
		project.ExpectedStartDate = data.ExpectedStartDate
	}
	if data.ExpectedEndDate != "" {
		project.ExpectedEndDate = data.ExpectedEndDate
	}
	if data.Department != "" {
		project.Department = data.Department
	}
	if data.Customer != "" {
		project.Customer = data.Customer
	}

	return project
}

// NewTask creates a new task document with default values
func NewTask(data TaskCreationRequest, creatorEmployeeID string) *Task {
	uniqueName := fmt.Sprintf("new-task-%s", generateUniqueID())

	task := &Task{
		Docstatus: 1,
		Doctype:   "Task",
		Name:      uniqueName,
		IsLocal:   true,
		Unsaved:   true,
		Owner:     "demo@example.com",
		Subject:   data.Subject,
		Status:    "Open",
		Company:   "",
	}

	// Only set optional fields if they are not empty
	if data.Priority != "" {
		task.Priority = data.Priority
	}
	if data.Project != "" {
		task.Project = data.Project
	}
	if data.AssignedTo != "" {
		task.AssignedTo = data.AssignedTo
	}
	if data.Description != "" {
		task.Description = data.Description
	}
	if data.ExpStartDate != "" {
		task.ExpStartDate = data.ExpStartDate
	}
	if data.ExpEndDate != "" {
		task.ExpEndDate = data.ExpEndDate
	}
	if data.Department != "" {
		task.Department = data.Department
	}

	return task
}

// generateUniqueID creates a simple unique ID
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
