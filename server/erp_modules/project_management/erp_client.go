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

	// Validate input
	if err := validateChatID(chatID); err != nil {
		return "", fmt.Errorf("invalid chat ID: %w", err)
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

	// Add query parameters - REMOVED email field as it's not permitted
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

// GetAllEmployees fetches all employees from ERPNext (UPDATED to include company_email)
func (c *ERPClient) GetAllEmployees() ([]Employee, error) {
	c.api.LogDebug("Getting all employees from ERPNext for project management")

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

	// Add query parameters - ADDED company_email field
	query := reqURL.Query()
	query.Add("fields", `["name","employee_name","custom_chat_id","status","department","designation","employee_number","company_email"]`)
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

	c.api.LogDebug("Found employees for project management", "count", len(apiResponse.Data))

	return apiResponse.Data, nil
}

// GetEmployeeEmail gets employee email by employee ID using company_email field
func (c *ERPClient) GetEmployeeEmail(employeeID string) (string, error) {
	c.api.LogDebug("Getting employee company email", "employee_id", employeeID)

	// Validate configuration
	if err := c.validateConfig(); err != nil {
		return "", err
	}

	// Combine API key and secret for token
	erpToken := c.config.ERPAPIKey + ":" + c.config.ERPAPISecret

	// Build the API endpoint for fetching employee by ID
	baseURL := strings.TrimSuffix(c.config.ERPDomain, "/") + "/api/resource/Employee"

	// Create the filter parameter
	filterParam := fmt.Sprintf(`[["name","=","%s"]]`, employeeID)

	// Parse the base URL
	reqURL, err := url.Parse(baseURL)
	if err != nil {
		return "", fmt.Errorf("failed to parse URL: %w", err)
	}

	// Add query parameters - use company_email field
	query := reqURL.Query()
	query.Add("filters", filterParam)
	query.Add("fields", `["name","company_email"]`)
	reqURL.RawQuery = query.Encode()

	c.api.LogDebug("Making request to ERPNext for employee company email", "url", reqURL.String())

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

	c.api.LogDebug("ERPNext API Response for employee company email",
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
			CompanyEmail string `json:"company_email"`
		} `json:"data"`
	}

	if err := json.Unmarshal(respBody, &apiResponse); err != nil {
		return "", fmt.Errorf("failed to parse response: %w", err)
	}

	if len(apiResponse.Data) > 0 {
		email := apiResponse.Data[0].CompanyEmail
		c.api.LogDebug("Found company email for employee", "employee_id", employeeID, "company_email", email)
		return email, nil
	}

	return "", fmt.Errorf("no employee found with ID: %s", employeeID)
}

// CreateProjectWithAssignment creates a project and assigns it to an employee (two-step process) - NEW METHOD
func (c *ERPClient) CreateProjectWithAssignment(projectData ProjectCreationRequest, creatorEmployeeID, creatorEmail string) (string, error) {
	c.api.LogDebug("Creating project with assignment",
		"project_name", projectData.ProjectName,
		"assigned_to", projectData.AssignedToName,
		"creator_email", creatorEmail)

	// Step 1: Create the project
	projectID, err := c.CreateProject(projectData, creatorEmployeeID)
	if err != nil {
		return "", fmt.Errorf("failed to create project: %w", err)
	}

	c.api.LogInfo("Project created successfully", "project_id", projectID)

	// Step 2: Assign if assignee is specified
	if projectData.AssignedToEmployeeID != "" && projectData.AssignedToEmail != "" {
		err = c.AssignProjectToEmployee(projectID, creatorEmail, projectData.AssignedToEmail, projectData.Priority)
		if err != nil {
			c.api.LogWarn("Project created but assignment failed",
				"project_id", projectID,
				"error", err.Error())
			// Return success with warning - project was created
			return projectID, nil
		}
		c.api.LogInfo("Project assigned successfully",
			"project_id", projectID,
			"assigned_to", projectData.AssignedToEmail)
	}

	return projectID, nil
}

// CreateTaskWithAssignment creates a task and assigns it to an employee (two-step process) - NEW METHOD
func (c *ERPClient) CreateTaskWithAssignment(taskData TaskCreationRequest, creatorEmployeeID, creatorEmail string) (string, error) {
	c.api.LogDebug("Creating task with assignment",
		"task_subject", taskData.Subject,
		"assigned_to", taskData.AssignedToName,
		"creator_email", creatorEmail)

	// Step 1: Create the task
	taskID, err := c.CreateTask(taskData, creatorEmployeeID)
	if err != nil {
		return "", fmt.Errorf("failed to create task: %w", err)
	}

	c.api.LogInfo("Task created successfully", "task_id", taskID)

	// Step 2: Assign if assignee is specified
	if taskData.AssignedToEmployeeID != "" && taskData.AssignedToEmail != "" {
		err = c.AssignTaskToEmployee(taskID, creatorEmail, taskData.AssignedToEmail, taskData.Priority)
		if err != nil {
			c.api.LogWarn("Task created but assignment failed",
				"task_id", taskID,
				"error", err.Error())
			// Return success with warning - task was created
			return taskID, nil
		}
		c.api.LogInfo("Task assigned successfully",
			"task_id", taskID,
			"assigned_to", taskData.AssignedToEmail)
	}

	return taskID, nil
}

// CreateProject creates a new project in ERPNext (UPDATED to return actual project ID)
func (c *ERPClient) CreateProject(projectData ProjectCreationRequest, creatorEmployeeID string) (string, error) {
	c.api.LogDebug("Creating project in ERPNext", "project_name", projectData.ProjectName)

	if err := c.validateConfig(); err != nil {
		return "", err
	}

	if err := validateEmployeeID(creatorEmployeeID); err != nil {
		return "", fmt.Errorf("invalid creator employee ID: %w", err)
	}

	if err := validateProjectName(projectData.ProjectName); err != nil {
		return "", fmt.Errorf("invalid project name: %w", err)
	}

	erpToken := c.config.ERPAPIKey + ":" + c.config.ERPAPISecret
	erpEndpoint := strings.TrimSuffix(c.config.ERPDomain, "/") + ERPEndpointSuffix

	// Create project document
	project := NewProject(projectData, creatorEmployeeID)

	// Send to ERP and get response
	responseData, err := c.sendToERPWithResponse(erpEndpoint, erpToken, project)
	if err != nil {
		return "", err
	}

	// Extract the actual project ID from response
	projectID, err := c.extractDocumentID(responseData)
	if err != nil {
		c.api.LogWarn("Created project but couldn't extract ID, using name",
			"project_name", projectData.ProjectName,
			"error", err.Error())
		return project.ProjectName, nil
	}

	c.api.LogDebug("Project created successfully in ERPNext",
		"project_name", projectData.ProjectName,
		"project_id", projectID,
		"creator", creatorEmployeeID)

	return projectID, nil
}

// CreateTask creates a new task in ERPNext (UPDATED to return actual task ID)
func (c *ERPClient) CreateTask(taskData TaskCreationRequest, creatorEmployeeID string) (string, error) {
	c.api.LogDebug("Creating task in ERPNext", "task_subject", taskData.Subject)

	if err := c.validateConfig(); err != nil {
		return "", err
	}

	if err := validateEmployeeID(creatorEmployeeID); err != nil {
		return "", fmt.Errorf("invalid creator employee ID: %w", err)
	}

	if err := validateTaskSubject(taskData.Subject); err != nil {
		return "", fmt.Errorf("invalid task subject: %w", err)
	}

	erpToken := c.config.ERPAPIKey + ":" + c.config.ERPAPISecret
	erpEndpoint := strings.TrimSuffix(c.config.ERPDomain, "/") + ERPEndpointSuffix

	// Create task document
	task := NewTask(taskData, creatorEmployeeID)

	// Send to ERP and get response
	responseData, err := c.sendToERPWithResponse(erpEndpoint, erpToken, task)
	if err != nil {
		return "", err
	}

	// Extract the actual task ID from response
	taskID, err := c.extractDocumentID(responseData)
	if err != nil {
		c.api.LogWarn("Created task but couldn't extract ID, using subject",
			"task_subject", taskData.Subject,
			"error", err.Error())
		return task.Subject, nil
	}

	c.api.LogDebug("Task created successfully in ERPNext",
		"task_subject", taskData.Subject,
		"task_id", taskID,
		"creator", creatorEmployeeID)

	return taskID, nil
}

// AssignProjectToEmployee assigns a project to an employee via ToDo - NEW METHOD
func (c *ERPClient) AssignProjectToEmployee(projectID, assignedBy, allocatedTo, priority string) error {
	c.api.LogDebug("Assigning project to employee",
		"project_id", projectID,
		"assigned_by", assignedBy,
		"allocated_to", allocatedTo)

	if err := c.validateConfig(); err != nil {
		return err
	}

	erpToken := c.config.ERPAPIKey + ":" + c.config.ERPAPISecret
	erpEndpoint := strings.TrimSuffix(c.config.ERPDomain, "/") + "/api/resource/ToDo"

	// Create ToDo assignment
	toDo := &ToDoAssignment{
		AssignedBy:    assignedBy,
		AllocatedTo:   allocatedTo,
		ReferenceType: "Project",
		ReferenceName: projectID,
		Description:   fmt.Sprintf("Assigned via ToDo for project %s", projectID),
		Priority:      priority,
		Status:        "Open",
	}

	// Convert to JSON
	todoJSON, err := json.Marshal(toDo)
	if err != nil {
		return fmt.Errorf("failed to marshal ToDo assignment: %w", err)
	}

	// Create the request
	req, err := http.NewRequest("POST", erpEndpoint, bytes.NewBuffer(todoJSON))
	if err != nil {
		return fmt.Errorf("failed to create assignment request: %w", err)
	}

	// Set headers
	req.Header.Set("Authorization", "token "+erpToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	// Make the request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send assignment request: %w", err)
	}
	defer resp.Body.Close()

	// Read the response
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read assignment response: %w", err)
	}

	c.api.LogDebug("Task assignment response",
		"status", resp.Status,
		"body", string(respBody))

	// Check the response status
	if resp.StatusCode >= 400 {
		return fmt.Errorf("task assignment failed: %s - %s", resp.Status, string(respBody))
	}

	return nil
}

// extractDocumentID extracts the document ID from ERP creation response - NEW METHOD
func (c *ERPClient) extractDocumentID(responseData []byte) (string, error) {
	var erpResponse ERPCreateResponse
	if err := json.Unmarshal(responseData, &erpResponse); err != nil {
		return "", fmt.Errorf("failed to parse ERP response: %w", err)
	}

	// Try to get ID from message.name first
	if erpResponse.Message.Name != "" {
		return erpResponse.Message.Name, nil
	}

	// Try to get ID from docs array
	if len(erpResponse.Docs) > 0 && erpResponse.Docs[0].Name != "" {
		return erpResponse.Docs[0].Name, nil
	}

	return "", fmt.Errorf("could not extract document ID from response")
}

// AssignTaskToEmployee assigns a task to an employee via ToDo - NEW METHOD
func (c *ERPClient) AssignTaskToEmployee(taskID, assignedBy, allocatedTo, priority string) error {
	c.api.LogDebug("Assigning task to employee",
		"task_id", taskID,
		"assigned_by", assignedBy,
		"allocated_to", allocatedTo)

	if err := c.validateConfig(); err != nil {
		return err
	}

	erpToken := c.config.ERPAPIKey + ":" + c.config.ERPAPISecret
	erpEndpoint := strings.TrimSuffix(c.config.ERPDomain, "/") + "/api/resource/ToDo"

	// Create ToDo assignment
	toDo := &ToDoAssignment{
		AssignedBy:    assignedBy,
		AllocatedTo:   allocatedTo,
		ReferenceType: "Task",
		ReferenceName: taskID,
		Description:   fmt.Sprintf("Assigned via ToDo for task %s", taskID),
		Priority:      priority,
		Status:        "Open",
	}

	// Convert to JSON
	todoJSON, err := json.Marshal(toDo)
	if err != nil {
		return fmt.Errorf("failed to marshal ToDo assignment: %w", err)
	}

	// Create the request
	req, err := http.NewRequest("POST", erpEndpoint, bytes.NewBuffer(todoJSON))
	if err != nil {
		return fmt.Errorf("failed to create assignment request: %w", err)
	}

	// Set headers
	req.Header.Set("Authorization", "token "+erpToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	// Make the request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send assignment request: %w", err)
	}
	defer resp.Body.Close()

	// Read the response
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read assignment response: %w", err)
	}

	c.api.LogDebug("Project assignment response",
		"status", resp.Status,
		"body", string(respBody))

	// Check the response status
	if resp.StatusCode >= 400 {
		return fmt.Errorf("project assignment failed: %s - %s", resp.Status, string(respBody))
	}

	return nil
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

// sendToERP sends data to ERP system (existing method)
func (c *ERPClient) sendToERP(endpoint, token string, doc interface{}) error {
	_, err := c.sendToERPWithResponse(endpoint, token, doc)
	return err
}

// sendToERPWithResponse sends data to ERP system and returns response - NEW METHOD
func (c *ERPClient) sendToERPWithResponse(endpoint, token string, doc interface{}) ([]byte, error) {
	// Create the form data
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	// Marshal the doc to JSON
	docJSON, err := json.Marshal(doc)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal document: %w", err)
	}

	// Add doc field
	if err := writer.WriteField("doc", string(docJSON)); err != nil {
		return nil, fmt.Errorf("failed to write doc field: %w", err)
	}

	// Add action field
	if err := writer.WriteField("action", "Save"); err != nil {
		return nil, fmt.Errorf("failed to write action field: %w", err)
	}

	// Close the writer
	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("failed to close multipart writer: %w", err)
	}

	// Create the request
	req, err := http.NewRequest("POST", endpoint, body)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
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
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	// Read the response
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	// Check the response status
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("ERP API error: %s - %s", resp.Status, string(respBody))
	}

	return respBody, nil
}

// NewProject creates a new project document with default values (UPDATED - removed ProjectManager field)
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
		Company:     data.Company,
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
	// Assignment is now handled separately via ToDo

	return project
}

// NewTask creates a new task document with default values (UPDATED - removed AssignedTo field)
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
		Company:   data.Company,
	}

	// Only set optional fields if they are not empty
	if data.Priority != "" {
		task.Priority = data.Priority
	}
	if data.Project != "" {
		task.Project = data.Project
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
	// Assignment is now handled separately via ToDo

	return task
}
