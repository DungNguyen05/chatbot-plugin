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
	MatchConfidence float64 `json:"match_confidence"` // NEW FIELD
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

// ProjectManagementConfirmation represents pending confirmation
type ProjectManagementConfirmation struct {
	UserID       string                 `json:"user_id"`
	Type         string                 `json:"type"`   // "project" or "task"
	Action       string                 `json:"action"` // "create_project", "create_task"
	Data         map[string]interface{} `json:"data"`   // Complete schema data
	CreatedAt    int64                  `json:"created_at"`
	EmployeeID   string                 `json:"employee_id"`
	CreatorEmail string                 `json:"creator_email"`
}

// MultiEmployeeDisambiguationConfirmation represents pending multiple employee selection
type MultiEmployeeDisambiguationConfirmation struct {
	UserID                    string                         `json:"user_id"`
	Type                      string                         `json:"type"`   // "project" or "task"
	Action                    string                         `json:"action"` // "create_project", "create_task"
	Data                      map[string]interface{}         `json:"data"`   // Complete schema data
	CreatedAt                 int64                          `json:"created_at"`
	EmployeeID                string                         `json:"employee_id"`
	CreatorEmail              string                         `json:"creator_email"`
	UnresolvedEmployeeMatches []UnresolvedEmployeeMatch      `json:"unresolved_employee_matches"`     // All unresolved employees
	ResolvedEmployees         []AssignedEmployee             `json:"resolved_employees"`              // Successfully resolved employees
	IsModification            bool                           `json:"is_modification"`                 // Whether this is during modification
	OriginalConfirmation      *ProjectManagementConfirmation `json:"original_confirmation,omitempty"` // For modifications
	PendingModifications      map[string]interface{}         `json:"pending_modifications,omitempty"` // For modifications
}

// UnresolvedEmployeeMatch represents an employee name that needs disambiguation
type UnresolvedEmployeeMatch struct {
	OriginalName      string     `json:"original_name"`
	MatchingEmployees []Employee `json:"matching_employees"`
	Index             int        `json:"index"` // Display index for user selection (1-based)
}

// UserResponse represents parsed user response to confirmation
type UserResponse struct {
	Intent        string                 `json:"intent"`        // "confirm", "modify", "cancel"
	Modifications map[string]interface{} `json:"modifications"` // Fields to modify
	Reasoning     string                 `json:"reasoning"`     // LLM reasoning
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

// ExistingEntityDisambiguationConfirmation represents pending existing entity selection
type ExistingEntityDisambiguationConfirmation struct {
	UserID                 string                                   `json:"user_id"`
	Type                   string                                   `json:"type"`             // "project" or "task"
	Action                 string                                   `json:"action"`           // "create_project", "create_task"
	OriginalRequest        map[string]interface{}                   `json:"original_request"` // Original parsed request
	CreatedAt              int64                                    `json:"created_at"`
	EmployeeID             string                                   `json:"employee_id"`
	CreatorEmail           string                                   `json:"creator_email"`
	ExistingEntities       []interface{}                            `json:"existing_entities"`                 // []Project or []Task
	EntityType             string                                   `json:"entity_type"`                       // "project" or "task"
	ResolvedEmployees      []AssignedEmployee                       `json:"resolved_employees"`                // Already resolved employees
	EmployeeDisambiguation *MultiEmployeeDisambiguationConfirmation `json:"employee_disambiguation,omitempty"` // If employee disambiguation needed
}

// ExistingEntityDisambiguationResponse represents parsed user response to existing entity selection
type ExistingEntityDisambiguationResponse struct {
	Intent        string `json:"intent"`         // "create_new", "use_existing", "cancel"
	SelectedIndex int    `json:"selected_index"` // 1-based index for use_existing
	Reasoning     string `json:"reasoning"`      // LLM reasoning
}

// ProjectTaskDisambiguationConfirmation handles project selection for task creation
type ProjectTaskDisambiguationConfirmation struct {
	UserID              string               `json:"user_id"`
	OriginalTaskRequest *TaskCreationRequest `json:"original_task_request"`
	CreatedAt           int64                `json:"created_at"`
	EmployeeID          string               `json:"employee_id"`
	CreatorEmail        string               `json:"creator_email"`
	MatchingProjects    []Project            `json:"matching_projects"`
	ResolvedEmployees   []AssignedEmployee   `json:"resolved_employees"`
}

// ProjectSelectionResponse represents parsed user response to project selection
type ProjectSelectionResponse struct {
	Intent        string `json:"intent"`         // "select_project", "no_project", "cancel"
	SelectedIndex int    `json:"selected_index"` // 1-based index for select_project
	Reasoning     string `json:"reasoning"`      // LLM reasoning
}

// ProcessingStep represents a step in the multi-step processing workflow
type ProcessingStep string

const (
	StepAnalyzeExistingTask    ProcessingStep = "analyze_existing_task"
	StepAnalyzeExistingProject ProcessingStep = "analyze_existing_project"
	StepResolveEmployees       ProcessingStep = "resolve_employees"
	StepResolveProject         ProcessingStep = "resolve_project_selection"
	StepFinalConfirmation      ProcessingStep = "final_confirmation"
	StepComplete               ProcessingStep = "complete"
)

// ProcessingState holds the complete state for multi-step processing
type ProcessingState struct {
	UserID          string                 `json:"user_id"`
	Step            ProcessingStep         `json:"step"`
	Type            string                 `json:"type"`   // "task" or "project"
	Action          string                 `json:"action"` // "create_task", "create_project"
	OriginalRequest map[string]interface{} `json:"original_request"`
	EmployeeID      string                 `json:"employee_id"`
	CreatorEmail    string                 `json:"creator_email"`
	CreatedAt       int64                  `json:"created_at"`

	// Step-specific data
	PendingEmployeeResolution *AssigneeResolutionResult `json:"pending_employee_resolution,omitempty"`
	PendingExistingEntities   []interface{}             `json:"pending_existing_entities,omitempty"`
	PendingProjectSelection   []Project                 `json:"pending_project_selection,omitempty"`

	// Resolved data that carries forward
	ResolvedEmployees      []AssignedEmployee     `json:"resolved_employees"`
	SelectedExistingEntity map[string]interface{} `json:"selected_existing_entity,omitempty"`
	SelectedProject        string                 `json:"selected_project,omitempty"`

	// Completion tracking
	CompletedSteps map[ProcessingStep]bool `json:"completed_steps"`
}
