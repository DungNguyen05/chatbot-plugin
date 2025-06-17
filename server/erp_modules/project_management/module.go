// Copyright (c) 2023-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package project_management

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/mattermost/mattermost-plugin-ai/server/erp_modules"
	"github.com/mattermost/mattermost-plugin-ai/server/llm"
	"github.com/mattermost/mattermost/server/public/model"
)

// ProjectManagementModule handles project and task management operations
type ProjectManagementModule struct {
	config              ProjectManagementConfig
	erpClient           *ERPClient
	notificationManager *NotificationManager
	api                 PluginAPI
	prompts             PromptsInterface
	getLLM              func() llm.LanguageModel
}

// NewProjectManagementModule creates a new project management module
func NewProjectManagementModule(
	config ProjectManagementConfig,
	httpClient *http.Client,
	i18n I18nBundle,
	prompts PromptsInterface,
	getLLM func() llm.LanguageModel,
	notificationFunc func(userID, userName string, eventType ProjectManagementEventType, itemName string, details string) error,
	api PluginAPI,
	botUserID string,
) *ProjectManagementModule {
	// Create ERP client
	erpClient := NewERPClient(config, httpClient, api)

	// Create notification manager
	notificationManager := NewNotificationManager(config, i18n, prompts, getLLM, api, botUserID)

	return &ProjectManagementModule{
		config:              config,
		erpClient:           erpClient,
		notificationManager: notificationManager,
		api:                 api,
		prompts:             prompts,
		getLLM:              getLLM,
	}
}

// GetCategory returns the category this module handles
func (m *ProjectManagementModule) GetCategory() string {
	return "project_management"
}

// GetSupportedActions returns list of actions this module supports
func (m *ProjectManagementModule) GetSupportedActions() []string {
	return []string{"create_project", "create_task"}
}

// CanHandle determines if this module can handle the given intent
func (m *ProjectManagementModule) CanHandle(intent *erp_modules.Intent) bool {
	if intent.Category != "project_management" {
		return false
	}

	supportedActions := m.GetSupportedActions()
	for _, action := range supportedActions {
		if intent.Action == action {
			return true
		}
	}

	return false
}

// Execute processes the project management intent
func (m *ProjectManagementModule) Execute(ctx *erp_modules.ModuleContext, intent *erp_modules.Intent) (*erp_modules.ModuleResponse, error) {
	// Get employee ID
	employeeID, err := m.GetEmployeeIDFromUser(ctx.User)
	if err != nil {
		return &erp_modules.ModuleResponse{
			Success: false,
			Message: "❌ Không tìm thấy thông tin nhân viên của bạn trong hệ thống ERP. Vui lòng liên hệ quản trị viên.",
			Error:   err.Error(),
		}, nil
	}

	// Get user name for notifications
	userName := ctx.User.Username
	if ctx.User.FirstName != "" || ctx.User.LastName != "" {
		userName = strings.TrimSpace(ctx.User.FirstName + " " + ctx.User.LastName)
	}

	// Execute specific action
	switch intent.Action {
	case "create_project":
		return m.handleCreateProject(employeeID, userName, ctx.User.Id, ctx, intent)
	case "create_task":
		return m.handleCreateTask(employeeID, userName, ctx.User.Id, ctx, intent)
	default:
		return &erp_modules.ModuleResponse{
			Success: false,
			Message: "Hành động không được hỗ trợ",
			Error:   fmt.Sprintf("unsupported action: %s", intent.Action),
		}, nil
	}
}

// GetDescription returns a description of what this module does
func (m *ProjectManagementModule) GetDescription() string {
	return "Quản lý dự án và công việc: tạo dự án mới, tạo task, phân công công việc"
}

// GetActionExamples returns examples of user messages for each action
func (m *ProjectManagementModule) GetActionExamples() map[string][]string {
	return map[string][]string{
		"create_project": {
			"tạo dự án mới",
			"khởi tạo project",
			"tạo project mới",
			"bắt đầu dự án",
			"create new project",
			"start project",
			"initialize project",
			"new project",
		},
		"create_task": {
			"tạo task mới",
			"tạo công việc",
			"giao việc",
			"thêm task",
			"tạo nhiệm vụ mới",
			"create new task",
			"add task",
			"assign work",
			"new task",
			"create job",
		},
	}
}

// handleCreateProject processes project creation request
func (m *ProjectManagementModule) handleCreateProject(employeeID, userName, userID string, ctx *erp_modules.ModuleContext, intent *erp_modules.Intent) (*erp_modules.ModuleResponse, error) {
	// Use LLM to extract project details from user message
	projectRequest, err := m.analyzeProjectCreation(ctx, intent.RawMessage)
	if err != nil {
		return &erp_modules.ModuleResponse{
			Success: false,
			Message: "⚠️ Không thể hiểu được yêu cầu tạo dự án. Vui lòng thử lại với thông tin rõ ràng hơn.",
			Error:   err.Error(),
		}, nil
	}

	// Validate required fields
	if projectRequest.ProjectName == "" {
		return &erp_modules.ModuleResponse{
			Success: false,
			Message: "📝 Vui lòng cung cấp tên dự án. Ví dụ: 'Tạo dự án phát triển website bán hàng'",
		}, nil
	}

	// Create project in ERPNext
	projectName, err := m.erpClient.CreateProject(*projectRequest, employeeID)
	if err != nil {
		return &erp_modules.ModuleResponse{
			Success: false,
			Message: "⚠️ Có lỗi xảy ra khi tạo dự án trong hệ thống. Vui lòng thử lại.",
			Error:   err.Error(),
		}, nil
	}

	// Send notification asynchronously
	go func() {
		details := fmt.Sprintf("Mô tả: %s", projectRequest.Description)
		if projectRequest.Priority != "" {
			details += fmt.Sprintf(", Mức độ: %s", projectRequest.Priority)
		}
		_ = m.notificationManager.SendNotification(userID, userName, ProjectManagementEventProjectCreated, projectName, details)
	}()

	return &erp_modules.ModuleResponse{
		Success:     true,
		Message:     fmt.Sprintf("✅ Đã tạo dự án mới thành công: **%s**!", projectName),
		ActionTaken: "create_project",
		Data: map[string]interface{}{
			"project_name": projectName,
			"description":  projectRequest.Description,
			"priority":     projectRequest.Priority,
			"creator":      userName,
		},
	}, nil
}

// handleCreateTask processes task creation request
func (m *ProjectManagementModule) handleCreateTask(employeeID, userName, userID string, ctx *erp_modules.ModuleContext, intent *erp_modules.Intent) (*erp_modules.ModuleResponse, error) {
	// Use LLM to extract task details from user message
	taskRequest, err := m.analyzeTaskCreation(ctx, intent.RawMessage)
	if err != nil {
		return &erp_modules.ModuleResponse{
			Success: false,
			Message: "⚠️ Không thể hiểu được yêu cầu tạo task. Vui lòng thử lại với thông tin rõ ràng hơn.",
			Error:   err.Error(),
		}, nil
	}

	// Validate required fields
	if taskRequest.Subject == "" {
		return &erp_modules.ModuleResponse{
			Success: false,
			Message: "📝 Vui lòng cung cấp tên công việc. Ví dụ: 'Tạo task thiết kế giao diện trang chủ'",
		}, nil
	}

	// Create task in ERPNext
	taskName, err := m.erpClient.CreateTask(*taskRequest, employeeID)
	if err != nil {
		return &erp_modules.ModuleResponse{
			Success: false,
			Message: "⚠️ Có lỗi xảy ra khi tạo task trong hệ thống. Vui lòng thử lại.",
			Error:   err.Error(),
		}, nil
	}

	// Send notification asynchronously
	go func() {
		details := fmt.Sprintf("Mô tả: %s", taskRequest.Description)
		if taskRequest.AssignedTo != "" {
			details += fmt.Sprintf(", Giao cho: %s", taskRequest.AssignedTo)
		}
		if taskRequest.Priority != "" {
			details += fmt.Sprintf(", Mức độ: %s", taskRequest.Priority)
		}
		_ = m.notificationManager.SendNotification(userID, userName, ProjectManagementEventTaskCreated, taskName, details)
	}()

	return &erp_modules.ModuleResponse{
		Success:     true,
		Message:     fmt.Sprintf("✅ Đã tạo task mới thành công: **%s**!", taskName),
		ActionTaken: "create_task",
		Data: map[string]interface{}{
			"task_name":   taskName,
			"description": taskRequest.Description,
			"assigned_to": taskRequest.AssignedTo,
			"priority":    taskRequest.Priority,
			"project":     taskRequest.Project,
			"creator":     userName,
		},
	}, nil
}

// analyzeProjectCreation uses LLM to extract project details from user message
func (m *ProjectManagementModule) analyzeProjectCreation(ctx *erp_modules.ModuleContext, userMessage string) (*ProjectCreationRequest, error) {
	// Create LLM context
	llmContext := &llm.Context{
		RequestingUser: ctx.User,
		Time:           time.Now().Format(time.RFC1123),
	}
	llmContext.Parameters = map[string]interface{}{
		"UserMessage": userMessage,
	}

	// Format the project creation analysis prompt
	systemPrompt, err := m.prompts.Format("project_creation_analysis", llmContext)
	if err != nil {
		return nil, fmt.Errorf("failed to format project creation analysis prompt: %w", err)
	}

	// Create completion request
	completionRequest := llm.CompletionRequest{
		Posts: []llm.Post{
			{
				Role:    llm.PostRoleSystem,
				Message: systemPrompt,
			},
			{
				Role:    llm.PostRoleUser,
				Message: userMessage,
			},
		},
		Context: llmContext,
	}

	// Get LLM response
	response, err := m.getLLM().ChatCompletionNoStream(completionRequest, llm.WithMaxGeneratedTokens(400))
	if err != nil {
		return nil, fmt.Errorf("failed to analyze project creation with LLM: %w", err)
	}

	// Parse JSON response
	var projectRequest ProjectCreationRequest

	// Clean response to extract JSON
	response = strings.TrimSpace(response)
	start := strings.Index(response, "{")
	end := strings.LastIndex(response, "}") + 1

	if start == -1 || end <= start {
		return nil, fmt.Errorf("no valid JSON found in LLM response: %s", response)
	}

	jsonStr := response[start:end]
	if err := json.Unmarshal([]byte(jsonStr), &projectRequest); err != nil {
		return nil, fmt.Errorf("failed to parse LLM response as JSON: %w", err)
	}

	return &projectRequest, nil
}

// analyzeTaskCreation uses LLM to extract task details from user message
func (m *ProjectManagementModule) analyzeTaskCreation(ctx *erp_modules.ModuleContext, userMessage string) (*TaskCreationRequest, error) {
	// Create LLM context
	llmContext := &llm.Context{
		RequestingUser: ctx.User,
		Time:           time.Now().Format(time.RFC1123),
	}
	llmContext.Parameters = map[string]interface{}{
		"UserMessage": userMessage,
	}

	// Format the task creation analysis prompt
	systemPrompt, err := m.prompts.Format("task_creation_analysis", llmContext)
	if err != nil {
		return nil, fmt.Errorf("failed to format task creation analysis prompt: %w", err)
	}

	// Create completion request
	completionRequest := llm.CompletionRequest{
		Posts: []llm.Post{
			{
				Role:    llm.PostRoleSystem,
				Message: systemPrompt,
			},
			{
				Role:    llm.PostRoleUser,
				Message: userMessage,
			},
		},
		Context: llmContext,
	}

	// Get LLM response
	response, err := m.getLLM().ChatCompletionNoStream(completionRequest, llm.WithMaxGeneratedTokens(400))
	if err != nil {
		return nil, fmt.Errorf("failed to analyze task creation with LLM: %w", err)
	}

	// Parse JSON response
	var taskRequest TaskCreationRequest

	// Clean response to extract JSON
	response = strings.TrimSpace(response)
	start := strings.Index(response, "{")
	end := strings.LastIndex(response, "}") + 1

	if start == -1 || end <= start {
		return nil, fmt.Errorf("no valid JSON found in LLM response: %s", response)
	}

	jsonStr := response[start:end]
	if err := json.Unmarshal([]byte(jsonStr), &taskRequest); err != nil {
		return nil, fmt.Errorf("failed to parse LLM response as JSON: %w", err)
	}

	return &taskRequest, nil
}

// GetEmployeeIDFromUser gets employee ID from user
func (m *ProjectManagementModule) GetEmployeeIDFromUser(user *model.User) (string, error) {
	// Use the user's ID as the chat ID to lookup in ERPNext
	chatID := user.Id

	employeeID, err := m.erpClient.GetEmployeeByChatID(chatID)
	if err != nil {
		return "", fmt.Errorf("failed to get employee by chat ID %s: %w", chatID, err)
	}

	return employeeID, nil
}

// SendNotification is a legacy method that delegates to the notification manager
// This method is kept for backward compatibility
func (m *ProjectManagementModule) SendNotification(userID, userName string, eventType ProjectManagementEventType, itemName string, details string) error {
	return m.notificationManager.SendNotification(userID, userName, eventType, itemName, details)
}
