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

	if err := c.validateConfig(); err != nil {
		return "", err
	}

	if err := validateChatID(chatID); err != nil {
		return "", fmt.Errorf("invalid chat ID: %w", err)
	}

	erpToken := c.config.ERPAPIKey + ":" + c.config.ERPAPISecret
	baseURL := strings.TrimSuffix(c.config.ERPDomain, "/") + "/api/resource/Employee"
	filterParam := fmt.Sprintf(`[["custom_chat_id","=","%s"]]`, chatID)

	reqURL, err := url.Parse(baseURL)
	if err != nil {
		return "", fmt.Errorf("failed to parse URL: %w", err)
	}

	query := reqURL.Query()
	query.Add("filters", filterParam)
	query.Add("fields", `["name","employee_name","custom_chat_id"]`)
	reqURL.RawQuery = query.Encode()

	c.api.LogDebug("Making request to ERPNext for employee lookup", "url", reqURL.String())

	req, err := http.NewRequest("GET", reqURL.String(), nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "token "+erpToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	c.api.LogDebug("ERPNext API Response for employee lookup",
		"status", resp.Status,
		"body", string(respBody))

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("ERP API error: %s - %s", resp.Status, string(respBody))
	}

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

// GetAllEmployees fetches all employees from ERPNext
func (c *ERPClient) GetAllEmployees() ([]Employee, error) {
	c.api.LogDebug("Getting all employees from ERPNext for project management")

	if err := c.validateConfig(); err != nil {
		return nil, err
	}

	erpToken := c.config.ERPAPIKey + ":" + c.config.ERPAPISecret
	baseURL := strings.TrimSuffix(c.config.ERPDomain, "/") + "/api/resource/Employee"

	reqURL, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse URL: %w", err)
	}

	query := reqURL.Query()
	query.Add("fields", `["name","employee_name","custom_chat_id","status","department","designation","employee_number","company_email"]`)
	query.Add("filters", `[["status","=","Active"]]`)
	query.Add("limit_page_length", "1000")
	reqURL.RawQuery = query.Encode()

	c.api.LogDebug("Making request to ERPNext for all employees", "url", reqURL.String())

	req, err := http.NewRequest("GET", reqURL.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "token "+erpToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	c.api.LogDebug("ERPNext API Response for all employees",
		"status", resp.Status,
		"response_length", len(respBody))

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ERP API error: %s - %s", resp.Status, string(respBody))
	}

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

	if err := c.validateConfig(); err != nil {
		return "", err
	}

	erpToken := c.config.ERPAPIKey + ":" + c.config.ERPAPISecret
	baseURL := strings.TrimSuffix(c.config.ERPDomain, "/") + "/api/resource/Employee"
	filterParam := fmt.Sprintf(`[["name","=","%s"]]`, employeeID)

	reqURL, err := url.Parse(baseURL)
	if err != nil {
		return "", fmt.Errorf("failed to parse URL: %w", err)
	}

	query := reqURL.Query()
	query.Add("filters", filterParam)
	query.Add("fields", `["name","company_email"]`)
	reqURL.RawQuery = query.Encode()

	c.api.LogDebug("Making request to ERPNext for employee company email", "url", reqURL.String())

	req, err := http.NewRequest("GET", reqURL.String(), nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "token "+erpToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	c.api.LogDebug("ERPNext API Response for employee company email",
		"status", resp.Status,
		"body", string(respBody))

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("ERP API error: %s - %s", resp.Status, string(respBody))
	}

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

// CreateProjectWithMultipleAssignments creates a project and assigns it to multiple employees
func (c *ERPClient) CreateProjectWithMultipleAssignments(projectData ProjectCreationRequest, creatorEmployeeID, creatorEmail string) (string, []string, error) {
	c.api.LogDebug("Creating project with multiple assignments",
		"project_name", projectData.ProjectName,
		"assignee_count", len(projectData.AssignedToEmployees),
		"creator_email", creatorEmail)

	// Step 1: Create the project
	projectID, err := c.CreateProject(projectData, creatorEmployeeID)
	if err != nil {
		return "", nil, fmt.Errorf("failed to create project: %w", err)
	}

	c.api.LogInfo("Project created successfully", "project_id", projectID)

	// Step 2: Assign to all employees
	var assignmentErrors []string
	for _, assignedEmployee := range projectData.AssignedToEmployees {
		err = c.AssignProjectToEmployee(projectID, creatorEmail, assignedEmployee.Email, projectData.Priority)
		if err != nil {
			c.api.LogWarn("Project created but assignment failed",
				"project_id", projectID,
				"assignee", assignedEmployee.EmployeeName,
				"error", err.Error())
			assignmentErrors = append(assignmentErrors, assignedEmployee.EmployeeName)
		} else {
			c.api.LogInfo("Project assigned successfully",
				"project_id", projectID,
				"assignee", assignedEmployee.EmployeeName)
		}
	}

	return projectID, assignmentErrors, nil
}

// CreateTaskWithMultipleAssignments creates a task and assigns it to multiple employees
func (c *ERPClient) CreateTaskWithMultipleAssignments(taskData TaskCreationRequest, creatorEmployeeID, creatorEmail string) (string, []string, error) {
	c.api.LogDebug("Creating task with multiple assignments",
		"task_subject", taskData.Subject,
		"assignee_count", len(taskData.AssignedToEmployees),
		"creator_email", creatorEmail)

	// Step 1: Create the task
	taskID, err := c.CreateTask(taskData, creatorEmployeeID)
	if err != nil {
		return "", nil, fmt.Errorf("failed to create task: %w", err)
	}

	c.api.LogInfo("Task created successfully", "task_id", taskID)

	// Step 2: Assign to all employees
	var assignmentErrors []string
	for _, assignedEmployee := range taskData.AssignedToEmployees {
		err = c.AssignTaskToEmployee(taskID, creatorEmail, assignedEmployee.Email, taskData.Priority)
		if err != nil {
			c.api.LogWarn("Task created but assignment failed",
				"task_id", taskID,
				"assignee", assignedEmployee.EmployeeName,
				"error", err.Error())
			assignmentErrors = append(assignmentErrors, assignedEmployee.EmployeeName)
		} else {
			c.api.LogInfo("Task assigned successfully",
				"task_id", taskID,
				"assignee", assignedEmployee.EmployeeName)
		}
	}

	return taskID, assignmentErrors, nil
}

// CreateProject creates a new project in ERPNext
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

	project := NewProject(projectData, creatorEmployeeID)

	responseData, err := c.sendToERPWithResponse(erpEndpoint, erpToken, project)
	if err != nil {
		return "", err
	}

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

// CreateTask creates a new task in ERPNext
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

	task := NewTask(taskData, creatorEmployeeID)

	responseData, err := c.sendToERPWithResponse(erpEndpoint, erpToken, task)
	if err != nil {
		return "", err
	}

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

// AssignProjectToEmployee assigns a project to an employee via ToDo
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

	toDo := &ToDoAssignment{
		AssignedBy:    assignedBy,
		AllocatedTo:   allocatedTo,
		ReferenceType: "Project",
		ReferenceName: projectID,
		Description:   fmt.Sprintf("Assigned via ToDo for project %s", projectID),
		Priority:      priority,
		Status:        "Open",
	}

	todoJSON, err := json.Marshal(toDo)
	if err != nil {
		return fmt.Errorf("failed to marshal ToDo assignment: %w", err)
	}

	req, err := http.NewRequest("POST", erpEndpoint, bytes.NewBuffer(todoJSON))
	if err != nil {
		return fmt.Errorf("failed to create assignment request: %w", err)
	}

	req.Header.Set("Authorization", "token "+erpToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send assignment request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read assignment response: %w", err)
	}

	c.api.LogDebug("Project assignment response",
		"status", resp.Status,
		"body", string(respBody))

	if resp.StatusCode >= 400 {
		return fmt.Errorf("project assignment failed: %s - %s", resp.Status, string(respBody))
	}

	return nil
}

// AssignTaskToEmployee assigns a task to an employee via ToDo
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

	toDo := &ToDoAssignment{
		AssignedBy:    assignedBy,
		AllocatedTo:   allocatedTo,
		ReferenceType: "Task",
		ReferenceName: taskID,
		Description:   fmt.Sprintf("Assigned via ToDo for task %s", taskID),
		Priority:      priority,
		Status:        "Open",
	}

	todoJSON, err := json.Marshal(toDo)
	if err != nil {
		return fmt.Errorf("failed to marshal ToDo assignment: %w", err)
	}

	req, err := http.NewRequest("POST", erpEndpoint, bytes.NewBuffer(todoJSON))
	if err != nil {
		return fmt.Errorf("failed to create assignment request: %w", err)
	}

	req.Header.Set("Authorization", "token "+erpToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send assignment request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read assignment response: %w", err)
	}

	c.api.LogDebug("Task assignment response",
		"status", resp.Status,
		"body", string(respBody))

	if resp.StatusCode >= 400 {
		return fmt.Errorf("task assignment failed: %s - %s", resp.Status, string(respBody))
	}

	return nil
}

// GetAllProjects fetches all projects from ERPNext
func (c *ERPClient) GetAllProjects() ([]Project, error) {
	c.api.LogDebug("Getting all projects from ERPNext for project management")

	if err := c.validateConfig(); err != nil {
		return nil, err
	}

	erpToken := c.config.ERPAPIKey + ":" + c.config.ERPAPISecret
	baseURL := strings.TrimSuffix(c.config.ERPDomain, "/") + "/api/resource/Project"

	reqURL, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse URL: %w", err)
	}

	query := reqURL.Query()
	query.Add("fields", `["name","project_name","status","priority","customer","department"]`)
	query.Add("filters", `[["status","in",["Open","Completed","Cancelled"]]]`) // Get all status projects
	query.Add("limit_page_length", "1000")
	reqURL.RawQuery = query.Encode()

	c.api.LogDebug("Making request to ERPNext for all projects", "url", reqURL.String())

	req, err := http.NewRequest("GET", reqURL.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "token "+erpToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	c.api.LogDebug("ERPNext API Response for all projects",
		"status", resp.Status,
		"response_length", len(respBody))

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ERP API error: %s - %s", resp.Status, string(respBody))
	}

	var apiResponse struct {
		Data []Project `json:"data"`
	}

	if err := json.Unmarshal(respBody, &apiResponse); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	c.api.LogDebug("Found projects for project management", "count", len(apiResponse.Data))

	return apiResponse.Data, nil
}

// GetAllTasks fetches all tasks from ERPNext
func (c *ERPClient) GetAllTasks() ([]Task, error) {
	c.api.LogDebug("Getting all tasks from ERPNext for project management")

	if err := c.validateConfig(); err != nil {
		return nil, err
	}

	erpToken := c.config.ERPAPIKey + ":" + c.config.ERPAPISecret
	baseURL := strings.TrimSuffix(c.config.ERPDomain, "/") + "/api/resource/Task"

	reqURL, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse URL: %w", err)
	}

	query := reqURL.Query()
	query.Add("fields", `["name","subject","status","priority","description","project","department"]`)
	query.Add("filters", `[["status","in",["Open","Working","Pending Review","Completed","Cancelled"]]]`) // Get all status tasks
	query.Add("limit_page_length", "1000")
	reqURL.RawQuery = query.Encode()

	c.api.LogDebug("Making request to ERPNext for all tasks", "url", reqURL.String())

	req, err := http.NewRequest("GET", reqURL.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "token "+erpToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	c.api.LogDebug("ERPNext API Response for all tasks",
		"status", resp.Status,
		"response_length", len(respBody))

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ERP API error: %s - %s", resp.Status, string(respBody))
	}

	var apiResponse struct {
		Data []Task `json:"data"`
	}

	if err := json.Unmarshal(respBody, &apiResponse); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	c.api.LogDebug("Found tasks for project management", "count", len(apiResponse.Data))

	return apiResponse.Data, nil
}

// sendToERPWithResponse sends data to ERP system and returns response
func (c *ERPClient) sendToERPWithResponse(endpoint, token string, doc interface{}) ([]byte, error) {
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	docJSON, err := json.Marshal(doc)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal document: %w", err)
	}

	if err := writer.WriteField("doc", string(docJSON)); err != nil {
		return nil, fmt.Errorf("failed to write doc field: %w", err)
	}

	if err := writer.WriteField("action", "Save"); err != nil {
		return nil, fmt.Errorf("failed to write action field: %w", err)
	}

	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("failed to close multipart writer: %w", err)
	}

	req, err := http.NewRequest("POST", endpoint, body)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Authorization", "token "+token)
	req.Header.Set("Access-Control-Allow-Origin", "*")
	req.Header.Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	req.Header.Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("ERP API error: %s - %s", resp.Status, string(respBody))
	}

	return respBody, nil
}

// extractDocumentID extracts the document ID from ERP creation response
func (c *ERPClient) extractDocumentID(responseData []byte) (string, error) {
	var erpResponse ERPCreateResponse
	if err := json.Unmarshal(responseData, &erpResponse); err != nil {
		return "", fmt.Errorf("failed to parse ERP response: %w", err)
	}

	if erpResponse.Message.Name != "" {
		return erpResponse.Message.Name, nil
	}

	if len(erpResponse.Docs) > 0 && erpResponse.Docs[0].Name != "" {
		return erpResponse.Docs[0].Name, nil
	}

	return "", fmt.Errorf("could not extract document ID from response")
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
		Company:     data.Company,
	}

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
		Company:   data.Company,
	}

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

	return task
}
