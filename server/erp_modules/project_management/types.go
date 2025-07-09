// Copyright (c) 2023-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package project_management

import (
	"github.com/mattermost/mattermost-plugin-ai/server/llm"
	"github.com/mattermost/mattermost/server/public/model"
)

// API endpoint for ERPNext
const ERPEndpointSuffix = "/api/method/frappe.desk.form.save.savedocs"

// Confidence threshold for automatic employee assignment
const EmployeeMatchThreshold = 0.85

// ProjectManagementConfig represents the configuration for project management module
type ProjectManagementConfig struct {
	ERPDomain    string `json:"erpDomain"`
	ERPAPIKey    string `json:"erpAPIKey"`
	ERPAPISecret string `json:"erpAPISecret"`
}

// PromptsInterface interface for prompts
type PromptsInterface interface {
	FormatString(templateCode string, context *llm.Context) (string, error)
	Format(templateName string, context *llm.Context) (string, error)
}

// PluginAPI interface for plugin API operations
type PluginAPI interface {
	LogDebug(message string, keyValuePairs ...interface{})
	LogError(message string, keyValuePairs ...interface{})
	LogInfo(message string, keyValuePairs ...interface{})
	LogWarn(message string, keyValuePairs ...interface{})
	GetUser(userID string) (*model.User, error)
}

// Employee represents an employee from ERPNext
type Employee struct {
	Name            string  `json:"name"`             // Employee ID
	EmployeeName    string  `json:"employee_name"`    // Full name
	CompanyEmail    string  `json:"company_email"`    // Company email
	CustomChatID    string  `json:"custom_chat_id"`   // Chat ID for integration
	Status          string  `json:"status"`           // Active/Inactive
	Department      string  `json:"department"`       // Department
	Designation     string  `json:"designation"`      // Job title
	EmployeeNumber  string  `json:"employee_number"`  // Employee number
	MatchConfidence float64 `json:"match_confidence"` // Search confidence score
}

// Project represents the data structure for ERPNext Project
type Project struct {
	Docstatus         int    `json:"docstatus"`
	Doctype           string `json:"doctype"`
	Name              string `json:"name"`
	IsLocal           bool   `json:"__islocal"`
	Unsaved           bool   `json:"__unsaved"`
	Owner             string `json:"owner"`
	ProjectName       string `json:"project_name"`
	Status            string `json:"status"`
	Priority          string `json:"priority,omitempty"`
	ProjectType       string `json:"project_type,omitempty"`
	Description       string `json:"description,omitempty"`
	ExpectedStartDate string `json:"expected_start_date,omitempty"`
	ExpectedEndDate   string `json:"expected_end_date,omitempty"`
	Department        string `json:"department,omitempty"`
	Customer          string `json:"customer,omitempty"`
	Company           string `json:"company,omitempty"`
}

// Task represents the data structure for ERPNext Task
type Task struct {
	Docstatus    int    `json:"docstatus"`
	Doctype      string `json:"doctype"`
	Name         string `json:"name"`
	IsLocal      bool   `json:"__islocal"`
	Unsaved      bool   `json:"__unsaved"`
	Owner        string `json:"owner"`
	Subject      string `json:"subject"`
	Status       string `json:"status"`
	Priority     string `json:"priority,omitempty"`
	Project      string `json:"project,omitempty"`
	Description  string `json:"description,omitempty"`
	ExpStartDate string `json:"exp_start_date,omitempty"`
	ExpEndDate   string `json:"exp_end_date,omitempty"`
	Department   string `json:"department,omitempty"`
	Company      string `json:"company,omitempty"`
}

// ProjectCreationRequest represents parsed project creation intent
type ProjectCreationRequest struct {
	ProjectName          string `json:"project_name"`
	Description          string `json:"description,omitempty"`
	Priority             string `json:"priority,omitempty"`
	ProjectType          string `json:"project_type,omitempty"`
	ExpectedStartDate    string `json:"expected_start_date,omitempty"`
	ExpectedEndDate      string `json:"expected_end_date,omitempty"`
	Department           string `json:"department,omitempty"`
	Customer             string `json:"customer,omitempty"`
	Company              string `json:"company,omitempty"`
	AssignedToName       string `json:"assigned_to_name,omitempty"`        // Name extracted from user input
	AssignedToEmployeeID string `json:"assigned_to_employee_id,omitempty"` // Resolved employee ID
	AssignedToEmail      string `json:"assigned_to_email,omitempty"`       // Resolved employee email
}

// TaskCreationRequest represents parsed task creation intent
type TaskCreationRequest struct {
	Subject              string `json:"subject"`
	Description          string `json:"description,omitempty"`
	Priority             string `json:"priority,omitempty"`
	Project              string `json:"project,omitempty"`
	AssignedToName       string `json:"assigned_to_name,omitempty"`        // Name extracted from user input
	AssignedToEmployeeID string `json:"assigned_to_employee_id,omitempty"` // Resolved employee ID
	AssignedToEmail      string `json:"assigned_to_email,omitempty"`       // Resolved employee email
	ExpStartDate         string `json:"exp_start_date,omitempty"`
	ExpEndDate           string `json:"exp_end_date,omitempty"`
	Department           string `json:"department,omitempty"`
	Company              string `json:"company,omitempty"`
}

// ProjectManagementConfirmation represents pending confirmation
type ProjectManagementConfirmation struct {
	UserID          string                 `json:"user_id"`
	Type            string                 `json:"type"`   // "project" or "task"
	Action          string                 `json:"action"` // "create_project", "create_task"
	Data            map[string]interface{} `json:"data"`   // Complete schema data
	CreatedAt       int64                  `json:"created_at"`
	EmployeeID      string                 `json:"employee_id"`
	CreatorEmail    string                 `json:"creator_email"`     // Creator's email
	AssignedToEmail string                 `json:"assigned_to_email"` // Assignee's email
	AssignedToName  string                 `json:"assigned_to_name"`  // Assignee's name
}

// EmployeeDisambiguationConfirmation represents pending employee selection - NEW
type EmployeeDisambiguationConfirmation struct {
	UserID             string                 `json:"user_id"`
	Type               string                 `json:"type"`   // "project" or "task"
	Action             string                 `json:"action"` // "create_project", "create_task"
	Data               map[string]interface{} `json:"data"`   // Complete schema data
	CreatedAt          int64                  `json:"created_at"`
	EmployeeID         string                 `json:"employee_id"`
	CreatorEmail       string                 `json:"creator_email"`
	OriginalSearchName string                 `json:"original_search_name"` // The ambiguous name user provided
	AvailableEmployees []Employee             `json:"available_employees"`  // List of matching employees
}

// ModificationDisambiguationConfirmation represents pending employee selection during modification - NEW
type ModificationDisambiguationConfirmation struct {
	UserID               string                         `json:"user_id"`
	OriginalConfirmation *ProjectManagementConfirmation `json:"original_confirmation"` // The original confirmation being modified
	ModificationField    string                         `json:"modification_field"`    // Which field is being modified ("assigned_to_name")
	OriginalSearchName   string                         `json:"original_search_name"`  // The ambiguous name user provided
	AvailableEmployees   []Employee                     `json:"available_employees"`   // List of matching employees
	PendingModifications map[string]interface{}         `json:"pending_modifications"` // Other modifications that were being applied
	CreatedAt            int64                          `json:"created_at"`
}

// UserResponse represents parsed user response to confirmation
type UserResponse struct {
	Intent        string                 `json:"intent"`        // "confirm", "modify", "cancel"
	Modifications map[string]interface{} `json:"modifications"` // Fields to modify
	Reasoning     string                 `json:"reasoning"`     // LLM reasoning
}

// EmployeeDisambiguationResponse represents parsed user response to employee selection - NEW
type EmployeeDisambiguationResponse struct {
	Intent            string `json:"intent"`             // "index_selection", "name_clarification", "cancel"
	SelectedIndex     int    `json:"selected_index"`     // 1-based index selection
	ClarificationText string `json:"clarification_text"` // Additional name details provided
	Reasoning         string `json:"reasoning"`          // LLM reasoning
}

// AssigneeResolutionResult represents the result of resolving an assignee name
type AssigneeResolutionResult struct {
	Found                  bool       `json:"found"`
	EmployeeID             string     `json:"employee_id"`
	EmployeeName           string     `json:"employee_name"`
	EmployeeEmail          string     `json:"employee_email"`
	Error                  string     `json:"error,omitempty"`
	MultipleMatches        bool       `json:"multiple_matches"`        // NEW: indicates multiple matches found
	MatchingEmployees      []Employee `json:"matching_employees"`      // NEW: list of matching employees
	RequiresDisambiguation bool       `json:"requires_disambiguation"` // NEW: indicates need for user selection
}

// ERPCreateResponse represents the response from ERP creation requests
type ERPCreateResponse struct {
	Message struct {
		Name string `json:"name"` // The created document name/ID
	} `json:"message"`
	Docs []struct {
		Name string `json:"name"` // Alternative location for document name
	} `json:"docs"`
}

// ToDoAssignment represents the structure for ToDo assignment
type ToDoAssignment struct {
	AssignedBy    string `json:"assigned_by"`    // Creator's email
	AllocatedTo   string `json:"allocated_to"`   // Assignee's email
	ReferenceType string `json:"reference_type"` // "Task" or "Project"
	ReferenceName string `json:"reference_name"` // The task/project ID from ERP
	Description   string `json:"description"`    // Assignment description
	Priority      string `json:"priority"`       // Priority level
	Status        string `json:"status"`         // Usually "Open"
}
