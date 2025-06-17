// Copyright (c) 2023-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package project_management

import (
	"github.com/mattermost/mattermost-plugin-ai/server/llm"
	"github.com/mattermost/mattermost/server/public/model"
)

// API endpoint for ERPNext
const ERPEndpointSuffix = "/api/method/frappe.desk.form.save.savedocs"

// ProjectManagementEventType defines the type of project management event
type ProjectManagementEventType string

const (
	// ProjectManagementEventProjectCreated represents a project creation event
	ProjectManagementEventProjectCreated ProjectManagementEventType = "project_created"
	// ProjectManagementEventTaskCreated represents a task creation event
	ProjectManagementEventTaskCreated ProjectManagementEventType = "task_created"
	// ProjectManagementEventTaskAssigned represents a task assignment event
	ProjectManagementEventTaskAssigned ProjectManagementEventType = "task_assigned"
)

// ProjectManagementConfig represents the configuration for project management module
type ProjectManagementConfig struct {
	ERPDomain      string   `json:"erpDomain"`
	ERPAPIKey      string   `json:"erpAPIKey"`
	ERPAPISecret   string   `json:"erpAPISecret"`
	NotifyChannels []string `json:"notifyChannels"`
	Enabled        bool     `json:"enabled"`
}

// I18nBundle interface for internationalization
type I18nBundle interface {
	Localize(messageID, defaultMessage, locale string, params ...interface{}) string
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
	CreatePost(post *model.Post) error
	GetConfig() *model.Config
	BotDMNonResponse(botUserID, userID string, post *model.Post) error
	GetChannel(channelID string) (*model.Channel, error)
	GetChannelMember(channelID, userID string) (*model.ChannelMember, error)
	AddChannelMember(channelID, userID string) (*model.ChannelMember, error)
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
	Description       string `json:"description"`
	Priority          string `json:"priority"`
	ProjectType       string `json:"project_type"`
	ExpectedStartDate string `json:"expected_start_date"`
	ExpectedEndDate   string `json:"expected_end_date"`
	Department        string `json:"department"`
	Customer          string `json:"customer"`
}

// TaskCreationRequest represents parsed task creation intent
type TaskCreationRequest struct {
	Subject      string `json:"subject"`
	Description  string `json:"description"`
	Priority     string `json:"priority"`
	Project      string `json:"project"`
	AssignedTo   string `json:"assigned_to"`
	ExpStartDate string `json:"exp_start_date"`
	ExpEndDate   string `json:"exp_end_date"`
	Department   string `json:"department"`
}
