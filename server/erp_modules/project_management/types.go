// Copyright (c) 2023-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package project_management

import (
	"github.com/mattermost/mattermost-plugin-ai/server/llm"
)

// API endpoint for ERPNext
const ERPEndpointSuffix = "/api/method/frappe.desk.form.save.savedocs"

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
	AssignedTo   string `json:"assigned_to,omitempty"`
	Description  string `json:"description,omitempty"`
	ExpStartDate string `json:"exp_start_date,omitempty"`
	ExpEndDate   string `json:"exp_end_date,omitempty"`
	Department   string `json:"department,omitempty"`
	Company      string `json:"company,omitempty"`
}

// ProjectCreationRequest represents parsed project creation intent
type ProjectCreationRequest struct {
	ProjectName       string `json:"project_name"`
	Description       string `json:"description,omitempty"`
	Priority          string `json:"priority,omitempty"`
	ProjectType       string `json:"project_type,omitempty"`
	ExpectedStartDate string `json:"expected_start_date,omitempty"`
	ExpectedEndDate   string `json:"expected_end_date,omitempty"`
	Department        string `json:"department,omitempty"`
	Customer          string `json:"customer,omitempty"`
}

// TaskCreationRequest represents parsed task creation intent
type TaskCreationRequest struct {
	Subject      string `json:"subject"`
	Description  string `json:"description,omitempty"`
	Priority     string `json:"priority,omitempty"`
	Project      string `json:"project,omitempty"`
	AssignedTo   string `json:"assigned_to,omitempty"`
	ExpStartDate string `json:"exp_start_date,omitempty"`
	ExpEndDate   string `json:"exp_end_date,omitempty"`
	Department   string `json:"department,omitempty"`
}
