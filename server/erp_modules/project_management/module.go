// Copyright (c) 2023-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package project_management

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/mattermost/mattermost-plugin-ai/server/erp_modules"
	"github.com/mattermost/mattermost-plugin-ai/server/llm"
	"github.com/mattermost/mattermost/server/public/model"
)

// ProjectManagementModule handles project and task management operations
type ProjectManagementModule struct {
	config            ProjectManagementConfig
	erpClient         *ERPClient
	api               PluginAPI
	prompts           PromptsInterface
	getLLM            func() llm.LanguageModel
	confirmationState *ConfirmationState
	confirmationMutex sync.RWMutex
}

// NewProjectManagementModule creates a new project management module
func NewProjectManagementModule(
	config ProjectManagementConfig,
	httpClient *http.Client,
	prompts PromptsInterface,
	getLLM func() llm.LanguageModel,
	api PluginAPI,
) *ProjectManagementModule {
	// Create ERP client
	erpClient := NewERPClient(config, httpClient, api)

	return &ProjectManagementModule{
		config:    config,
		erpClient: erpClient,
		api:       api,
		prompts:   prompts,
		getLLM:    getLLM,
		confirmationState: &ConfirmationState{
			pendingConfirmations: make(map[string]*PendingConfirmation),
		},
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

// ProcessUserMessage handles user messages including confirmations and modifications
func (m *ProjectManagementModule) ProcessUserMessage(ctx *erp_modules.ModuleContext, message string) (*erp_modules.ModuleResponse, error) {
	m.confirmationMutex.RLock()
	pending, hasPending := m.confirmationState.pendingConfirmations[ctx.User.Id]
	m.confirmationMutex.RUnlock()

	if !hasPending {
		return nil, nil // Not handling this message
	}

	// Parse user response using LLM
	userResponse, err := m.parseUserResponse(ctx, message, pending)
	if err != nil {
		m.api.LogError("Failed to parse user response", "error", err.Error())
		return &erp_modules.ModuleResponse{
			Success: false,
			Message: "⚠️ Không thể hiểu phản hồi của bạn. Vui lòng thử lại.",
			Error:   err.Error(),
		}, nil
	}

	switch userResponse.Intent {
	case "confirm":
		return m.handleConfirmAction(ctx, pending)
	case "modify":
		return m.handleModifyAction(ctx, pending, userResponse.Modifications)
	case "cancel":
		return m.handleCancelAction(ctx.User.Id)
	default:
		return &erp_modules.ModuleResponse{
			Success: false,
			Message: "⚠️ Vui lòng xác nhận (có/yes), chỉnh sửa thông tin, hoặc hủy bỏ (không/cancel).",
		}, nil
	}
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

	// Execute specific action
	switch intent.Action {
	case "create_project":
		return m.handleCreateProjectRequest(employeeID, ctx, intent)
	case "create_task":
		return m.handleCreateTaskRequest(employeeID, ctx, intent)
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

// handleCreateProjectRequest processes project creation request with confirmation
func (m *ProjectManagementModule) handleCreateProjectRequest(employeeID string, ctx *erp_modules.ModuleContext, intent *erp_modules.Intent) (*erp_modules.ModuleResponse, error) {
	// Extract project details using LLM with complete schema
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

	// Store pending confirmation with complete schema
	m.storePendingConfirmation(ctx.User.Id, "project", projectRequest, employeeID)

	// Generate confirmation message
	confirmationMsg, err := m.generateConfirmationMessage(ctx, "project", projectRequest)
	if err != nil {
		return &erp_modules.ModuleResponse{
			Success: false,
			Message: "⚠️ Có lỗi xảy ra khi tạo tin nhắn xác nhận.",
			Error:   err.Error(),
		}, nil
	}

	return &erp_modules.ModuleResponse{
		Success:     true,
		Message:     confirmationMsg,
		ActionTaken: "request_project_confirmation",
		Data: map[string]interface{}{
			"awaiting_confirmation": true,
			"type":                  "project",
		},
	}, nil
}

// handleCreateTaskRequest processes task creation request with confirmation
func (m *ProjectManagementModule) handleCreateTaskRequest(employeeID string, ctx *erp_modules.ModuleContext, intent *erp_modules.Intent) (*erp_modules.ModuleResponse, error) {
	// Extract task details using LLM with complete schema
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

	// Store pending confirmation with complete schema
	m.storePendingConfirmation(ctx.User.Id, "task", taskRequest, employeeID)

	// Generate confirmation message
	confirmationMsg, err := m.generateConfirmationMessage(ctx, "task", taskRequest)
	if err != nil {
		return &erp_modules.ModuleResponse{
			Success: false,
			Message: "⚠️ Có lỗi xảy ra khi tạo tin nhắn xác nhận.",
			Error:   err.Error(),
		}, nil
	}

	return &erp_modules.ModuleResponse{
		Success:     true,
		Message:     confirmationMsg,
		ActionTaken: "request_task_confirmation",
		Data: map[string]interface{}{
			"awaiting_confirmation": true,
			"type":                  "task",
		},
	}, nil
}

// storePendingConfirmation stores a pending confirmation
func (m *ProjectManagementModule) storePendingConfirmation(userID, confirmationType string, data interface{}, employeeID string) {
	m.confirmationMutex.Lock()
	defer m.confirmationMutex.Unlock()

	// Convert data to map for consistent storage
	dataMap := make(map[string]interface{})
	dataBytes, _ := json.Marshal(data)
	json.Unmarshal(dataBytes, &dataMap)

	m.confirmationState.pendingConfirmations[userID] = &PendingConfirmation{
		UserID:     userID,
		Type:       confirmationType,
		Data:       dataMap,
		CreatedAt:  time.Now().UnixMilli(),
		EmployeeID: employeeID,
	}
}

// generateConfirmationMessage generates confirmation message using LLM
func (m *ProjectManagementModule) generateConfirmationMessage(ctx *erp_modules.ModuleContext, confirmationType string, data interface{}) (string, error) {
	// Create LLM context
	llmContext := &llm.Context{
		RequestingUser: ctx.User,
		Time:           time.Now().Format(time.RFC1123),
	}

	// Convert data to map for template access
	dataMap := make(map[string]interface{})
	dataBytes, _ := json.Marshal(data)
	json.Unmarshal(dataBytes, &dataMap)

	llmContext.Parameters = map[string]interface{}{
		"Type": confirmationType,
		"Data": dataMap,
	}

	// Use appropriate template based on type
	templateName := "project_confirmation_generation"
	if confirmationType == "task" {
		templateName = "task_confirmation_generation"
	}

	// Format the confirmation prompt
	systemPrompt, err := m.prompts.Format(templateName, llmContext)
	if err != nil {
		return "", fmt.Errorf("failed to format confirmation prompt: %w", err)
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
				Message: fmt.Sprintf("Generate confirmation message for %s creation", confirmationType),
			},
		},
		Context: llmContext,
	}

	// Get LLM response
	response, err := m.getLLM().ChatCompletionNoStream(completionRequest, llm.WithMaxGeneratedTokens(300))
	if err != nil {
		return "", fmt.Errorf("failed to generate confirmation with LLM: %w", err)
	}

	return strings.TrimSpace(response), nil
}

// parseUserResponse parses user response using LLM
func (m *ProjectManagementModule) parseUserResponse(ctx *erp_modules.ModuleContext, message string, pending *PendingConfirmation) (*UserResponse, error) {
	// Create LLM context
	llmContext := &llm.Context{
		RequestingUser: ctx.User,
		Time:           time.Now().Format(time.RFC1123),
	}

	llmContext.Parameters = map[string]interface{}{
		"UserMessage":    message,
		"PendingType":    pending.Type,
		"PendingData":    pending.Data,
		"OriginalSchema": m.getOriginalSchema(pending.Type),
	}

	// Use appropriate template based on type
	templateName := "project_modification_analysis"
	if pending.Type == "task" {
		templateName = "task_modification_analysis"
	}

	// Format the analysis prompt
	systemPrompt, err := m.prompts.Format(templateName, llmContext)
	if err != nil {
		return nil, fmt.Errorf("failed to format modification analysis prompt: %w", err)
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
				Message: message,
			},
		},
		Context: llmContext,
	}

	// Get LLM response
	response, err := m.getLLM().ChatCompletionNoStream(completionRequest, llm.WithMaxGeneratedTokens(300))
	if err != nil {
		return nil, fmt.Errorf("failed to analyze user response with LLM: %w", err)
	}

	// Parse JSON response
	var userResponse UserResponse
	response = strings.TrimSpace(response)
	start := strings.Index(response, "{")
	end := strings.LastIndex(response, "}") + 1

	if start == -1 || end <= start {
		return nil, fmt.Errorf("no valid JSON found in LLM response: %s", response)
	}

	jsonStr := response[start:end]
	if err := json.Unmarshal([]byte(jsonStr), &userResponse); err != nil {
		return nil, fmt.Errorf("failed to parse LLM response as JSON: %w", err)
	}

	return &userResponse, nil
}

// handleConfirmAction handles user confirmation
func (m *ProjectManagementModule) handleConfirmAction(ctx *erp_modules.ModuleContext, pending *PendingConfirmation) (*erp_modules.ModuleResponse, error) {
	// Clear pending confirmation
	m.clearPendingConfirmation(ctx.User.Id)

	// Create the project/task in ERP
	if pending.Type == "project" {
		var projectRequest ProjectCreationRequest
		dataBytes, _ := json.Marshal(pending.Data)
		json.Unmarshal(dataBytes, &projectRequest)

		projectName, err := m.erpClient.CreateProject(projectRequest, pending.EmployeeID)
		if err != nil {
			return &erp_modules.ModuleResponse{
				Success: false,
				Message: "⚠️ Có lỗi xảy ra khi tạo dự án trong hệ thống. Vui lòng thử lại.",
				Error:   err.Error(),
			}, nil
		}

		return &erp_modules.ModuleResponse{
			Success:     true,
			Message:     fmt.Sprintf("✅ Đã tạo dự án mới thành công: **%s**!", projectName),
			ActionTaken: "create_project",
			Data: map[string]interface{}{
				"project_name": projectName,
				"description":  projectRequest.Description,
				"priority":     projectRequest.Priority,
			},
		}, nil

	} else if pending.Type == "task" {
		var taskRequest TaskCreationRequest
		dataBytes, _ := json.Marshal(pending.Data)
		json.Unmarshal(dataBytes, &taskRequest)

		taskName, err := m.erpClient.CreateTask(taskRequest, pending.EmployeeID)
		if err != nil {
			return &erp_modules.ModuleResponse{
				Success: false,
				Message: "⚠️ Có lỗi xảy ra khi tạo task trong hệ thống. Vui lòng thử lại.",
				Error:   err.Error(),
			}, nil
		}

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
			},
		}, nil
	}

	return &erp_modules.ModuleResponse{
		Success: false,
		Message: "⚠️ Loại xác nhận không hợp lệ.",
	}, nil
}

// handleModifyAction handles user modification requests
func (m *ProjectManagementModule) handleModifyAction(ctx *erp_modules.ModuleContext, pending *PendingConfirmation, modifications map[string]interface{}) (*erp_modules.ModuleResponse, error) {
	// Update the pending data with modifications while preserving schema
	for key, value := range modifications {
		pending.Data[key] = value
	}

	// Update the stored confirmation
	m.confirmationMutex.Lock()
	m.confirmationState.pendingConfirmations[ctx.User.Id] = pending
	m.confirmationMutex.Unlock()

	// Generate new confirmation message with updated data
	confirmationMsg, err := m.generateConfirmationMessage(ctx, pending.Type, pending.Data)
	if err != nil {
		return &erp_modules.ModuleResponse{
			Success: false,
			Message: "⚠️ Có lỗi xảy ra khi tạo tin nhắn xác nhận cập nhật.",
			Error:   err.Error(),
		}, nil
	}

	return &erp_modules.ModuleResponse{
		Success:     true,
		Message:     confirmationMsg,
		ActionTaken: "update_confirmation",
		Data: map[string]interface{}{
			"awaiting_confirmation": true,
			"type":                  pending.Type,
			"updated":               true,
		},
	}, nil
}

// handleCancelAction handles user cancellation
func (m *ProjectManagementModule) handleCancelAction(userID string) (*erp_modules.ModuleResponse, error) {
	m.clearPendingConfirmation(userID)

	return &erp_modules.ModuleResponse{
		Success:     true,
		Message:     "❌ Đã hủy bỏ yêu cầu tạo dự án/task.",
		ActionTaken: "cancel_confirmation",
	}, nil
}

// clearPendingConfirmation clears pending confirmation for a user
func (m *ProjectManagementModule) clearPendingConfirmation(userID string) {
	m.confirmationMutex.Lock()
	defer m.confirmationMutex.Unlock()
	delete(m.confirmationState.pendingConfirmations, userID)
}

// getOriginalSchema returns the original schema for the given type
func (m *ProjectManagementModule) getOriginalSchema(confirmationType string) map[string]interface{} {
	if confirmationType == "project" {
		return map[string]interface{}{
			"project_name":        "",
			"description":         "",
			"priority":            "",
			"project_type":        "",
			"expected_start_date": "",
			"expected_end_date":   "",
			"department":          "",
			"customer":            "",
		}
	} else if confirmationType == "task" {
		return map[string]interface{}{
			"subject":        "",
			"description":    "",
			"priority":       "",
			"project":        "",
			"assigned_to":    "",
			"exp_start_date": "",
			"exp_end_date":   "",
			"department":     "",
		}
	}
	return make(map[string]interface{})
}

// analyzeProjectCreation uses LLM to extract project details with complete schema
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
	response, err := m.getLLM().ChatCompletionNoStream(completionRequest, llm.WithMaxGeneratedTokens(200))
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

// analyzeTaskCreation uses LLM to extract task details with complete schema
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
	response, err := m.getLLM().ChatCompletionNoStream(completionRequest, llm.WithMaxGeneratedTokens(200))
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
