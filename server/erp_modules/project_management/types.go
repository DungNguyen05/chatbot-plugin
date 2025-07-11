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
	Docstatus         int     `json:"docstatus"`
	Doctype           string  `json:"doctype"`
	Name              string  `json:"name"`
	IsLocal           bool    `json:"__islocal"`
	Unsaved           bool    `json:"__unsaved"`
	Owner             string  `json:"owner"`
	ProjectName       string  `json:"project_name"`
	Status            string  `json:"status"`
	Priority          string  `json:"priority,omitempty"`
	ProjectType       string  `json:"project_type,omitempty"`
	Description       string  `json:"description,omitempty"`
	ExpectedStartDate string  `json:"expected_start_date,omitempty"`
	ExpectedEndDate   string  `json:"expected_end_date,omitempty"`
	Department        string  `json:"department,omitempty"`
	Customer          string  `json:"customer,omitempty"`
	Company           string  `json:"company,omitempty"`
	MatchConfidence   float64 `json:"match_confidence"`
}

// Task represents the data structure for ERPNext Task
type Task struct {
	Docstatus       int     `json:"docstatus"`
	Doctype         string  `json:"doctype"`
	Name            string  `json:"name"`
	IsLocal         bool    `json:"__islocal"`
	Unsaved         bool    `json:"__unsaved"`
	Owner           string  `json:"owner"`
	Subject         string  `json:"subject"`
	Status          string  `json:"status"`
	Priority        string  `json:"priority,omitempty"`
	Project         string  `json:"project,omitempty"`
	Description     string  `json:"description,omitempty"`
	ExpStartDate    string  `json:"exp_start_date,omitempty"`
	ExpEndDate      string  `json:"exp_end_date,omitempty"`
	Department      string  `json:"department,omitempty"`
	Company         string  `json:"company,omitempty"`
	MatchConfidence float64 `json:"match_confidence"`
}

// ProjectCreationRequest represents parsed project creation intent with multi-employee support
type ProjectCreationRequest struct {
	ProjectName         string             `json:"project_name"`
	Description         string             `json:"description,omitempty"`
	Priority            string             `json:"priority,omitempty"`
	ProjectType         string             `json:"project_type,omitempty"`
	ExpectedStartDate   string             `json:"expected_start_date,omitempty"`
	ExpectedEndDate     string             `json:"expected_end_date,omitempty"`
	Department          string             `json:"department,omitempty"`
	Customer            string             `json:"customer,omitempty"`
	Company             string             `json:"company,omitempty"`
	AssignedToNames     []string           `json:"assigned_to_names,omitempty"`     // Multiple names extracted from user input
	AssignedToEmployees []AssignedEmployee `json:"assigned_to_employees,omitempty"` // Resolved employees
}

// TaskCreationRequest represents parsed task creation intent with multi-employee support
type TaskCreationRequest struct {
	Subject             string             `json:"subject"`
	Description         string             `json:"description,omitempty"`
	Priority            string             `json:"priority,omitempty"`
	Project             string             `json:"project,omitempty"`
	AssignedToNames     []string           `json:"assigned_to_names,omitempty"`     // Multiple names extracted from user input
	AssignedToEmployees []AssignedEmployee `json:"assigned_to_employees,omitempty"` // Resolved employees
	ExpStartDate        string             `json:"exp_start_date,omitempty"`
	ExpEndDate          string             `json:"exp_end_date,omitempty"`
	Department          string             `json:"department,omitempty"`
	Company             string             `json:"company,omitempty"`
}

// AssignedEmployee represents a resolved employee assignment
type AssignedEmployee struct {
	EmployeeID   string `json:"employee_id"`
	EmployeeName string `json:"employee_name"`
	Email        string `json:"email"`
	OriginalName string `json:"original_name"` // The original name from user input
}

// UnresolvedEmployeeMatch represents an employee name that needs disambiguation
type UnresolvedEmployeeMatch struct {
	OriginalName      string     `json:"original_name"`
	MatchingEmployees []Employee `json:"matching_employees"`
	Index             int        `json:"index"` // Display index for user selection (1-based)
}

// MultiEmployeeDisambiguationResponse represents parsed user response to multiple employee selection
type MultiEmployeeDisambiguationResponse struct {
	Intent          string `json:"intent"`           // "index_selection", "cancel"
	SelectedIndexes []int  `json:"selected_indexes"` // Multiple 1-based index selections
	Reasoning       string `json:"reasoning"`        // LLM reasoning
}

// AssigneeResolutionResult represents the result of resolving multiple assignee names
type AssigneeResolutionResult struct {
	ResolvedEmployees         []AssignedEmployee        `json:"resolved_employees"`
	UnresolvedEmployeeMatches []UnresolvedEmployeeMatch `json:"unresolved_employee_matches"`
	RequiresDisambiguation    bool                      `json:"requires_disambiguation"`
	Error                     string                    `json:"error,omitempty"`
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
