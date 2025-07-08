// Copyright (c) 2023-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package project_management

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/mattermost/mattermost-plugin-ai/server/erp_modules"
	"github.com/mattermost/mattermost/server/public/model"
)

// ConfirmationManager handles confirmation state
type ConfirmationManager struct {
	confirmationState map[string]*ProjectManagementConfirmation
	confirmationMutex sync.RWMutex
}

// NewConfirmationManager creates a new confirmation manager
func NewConfirmationManager() *ConfirmationManager {
	return &ConfirmationManager{
		confirmationState: make(map[string]*ProjectManagementConfirmation),
	}
}

// StorePendingConfirmation stores a pending confirmation
func (cm *ConfirmationManager) StorePendingConfirmation(userID string, confirmation *ProjectManagementConfirmation) {
	cm.confirmationMutex.Lock()
	defer cm.confirmationMutex.Unlock()
	cm.confirmationState[userID] = confirmation
}

// GetPendingConfirmation retrieves a pending confirmation
func (cm *ConfirmationManager) GetPendingConfirmation(userID string) (*ProjectManagementConfirmation, bool) {
	cm.confirmationMutex.RLock()
	defer cm.confirmationMutex.RUnlock()
	confirmation, exists := cm.confirmationState[userID]
	return confirmation, exists
}

// ClearPendingConfirmation clears pending confirmation for a user
func (cm *ConfirmationManager) ClearPendingConfirmation(userID string) {
	cm.confirmationMutex.Lock()
	defer cm.confirmationMutex.Unlock()
	delete(cm.confirmationState, userID)
}

// handleCreateProjectRequest processes project creation request
func (m *ProjectManagementModule) handleCreateProjectRequest(employeeID string, ctx *erp_modules.ModuleContext, intent *erp_modules.Intent) (*erp_modules.ModuleResponse, error) {
	isVietnamese := detectUserLanguage(ctx.User)

	// Extract project details using LLM
	projectRequest, err := m.analyzeProjectCreation(ctx, intent.RawMessage)
	if err != nil {
		errorMsg := "⚠️ Không thể hiểu được yêu cầu tạo dự án. Vui lòng thử lại với thông tin rõ ràng hơn."
		if !isVietnamese {
			errorMsg = "⚠️ Cannot understand the project creation request. Please try again with clearer information."
		}
		return &erp_modules.ModuleResponse{
			Success: false,
			Message: errorMsg,
			Error:   err.Error(),
		}, nil
	}

	// Validate required fields
	if projectRequest.ProjectName == "" {
		errorMsg := "📝 Vui lòng cung cấp tên dự án. Ví dụ: 'Tạo dự án phát triển website bán hàng'"
		if !isVietnamese {
			errorMsg = "📝 Please provide project name. Example: 'Create e-commerce website development project'"
		}
		return &erp_modules.ModuleResponse{
			Success: false,
			Message: errorMsg,
		}, nil
	}

	// Request confirmation
	return m.requestConfirmation(ctx, "create_project", "project", projectRequest, employeeID)
}

// handleCreateTaskRequest processes task creation request
func (m *ProjectManagementModule) handleCreateTaskRequest(employeeID string, ctx *erp_modules.ModuleContext, intent *erp_modules.Intent) (*erp_modules.ModuleResponse, error) {
	isVietnamese := detectUserLanguage(ctx.User)

	// Extract task details using LLM
	taskRequest, err := m.analyzeTaskCreation(ctx, intent.RawMessage)
	if err != nil {
		errorMsg := "⚠️ Không thể hiểu được yêu cầu tạo task. Vui lòng thử lại với thông tin rõ ràng hơn."
		if !isVietnamese {
			errorMsg = "⚠️ Cannot understand the task creation request. Please try again with clearer information."
		}
		return &erp_modules.ModuleResponse{
			Success: false,
			Message: errorMsg,
			Error:   err.Error(),
		}, nil
	}

	// Validate required fields
	if taskRequest.Subject == "" {
		errorMsg := "📝 Vui lòng cung cấp tên công việc. Ví dụ: 'Tạo task thiết kế giao diện trang chủ'"
		if !isVietnamese {
			errorMsg = "📝 Please provide task name. Example: 'Create homepage design task'"
		}
		return &erp_modules.ModuleResponse{
			Success: false,
			Message: errorMsg,
		}, nil
	}

	// Request confirmation
	return m.requestConfirmation(ctx, "create_task", "task", taskRequest, employeeID)
}

// requestConfirmation requests confirmation from user
func (m *ProjectManagementModule) requestConfirmation(ctx *erp_modules.ModuleContext, action, confirmationType string, data interface{}, employeeID string) (*erp_modules.ModuleResponse, error) {
	// Convert data to map for consistent storage
	dataMap := make(map[string]interface{})
	dataBytes, _ := json.Marshal(data)
	json.Unmarshal(dataBytes, &dataMap)

	// Store pending confirmation
	m.confirmationManager.StorePendingConfirmation(ctx.User.Id, &ProjectManagementConfirmation{
		UserID:     ctx.User.Id,
		Type:       confirmationType,
		Action:     action,
		Data:       dataMap,
		CreatedAt:  time.Now().UnixMilli(),
		EmployeeID: employeeID,
	})

	// Generate confirmation message
	confirmationMsg, err := m.generateConfirmationMessage(ctx, confirmationType, data)
	if err != nil {
		isVietnamese := detectUserLanguage(ctx.User)
		errorMsg := "⚠️ Có lỗi xảy ra khi tạo tin nhắn xác nhận."
		if !isVietnamese {
			errorMsg = "⚠️ An error occurred while creating confirmation message."
		}
		return &erp_modules.ModuleResponse{
			Success: false,
			Message: errorMsg,
			Error:   err.Error(),
		}, nil
	}

	return &erp_modules.ModuleResponse{
		Success:     true,
		Message:     confirmationMsg,
		ActionTaken: "request_confirmation",
		Data: map[string]interface{}{
			"awaiting_confirmation": true,
			"type":                  confirmationType,
		},
	}, nil
}

// executeConfirmedAction executes the confirmed action
func (m *ProjectManagementModule) executeConfirmedAction(ctx *erp_modules.ModuleContext, pending *ProjectManagementConfirmation) (*erp_modules.ModuleResponse, error) {
	switch pending.Action {
	case "create_project":
		return m.handleCreateProject(pending.EmployeeID, ctx.User.Id, pending.Data)
	case "create_task":
		return m.handleCreateTask(pending.EmployeeID, ctx.User.Id, pending.Data)
	default:
		isVietnamese := detectUserLanguage(ctx.User)
		errorMsg := "Hành động không hợp lệ"
		if !isVietnamese {
			errorMsg = "Invalid action"
		}
		return &erp_modules.ModuleResponse{
			Success: false,
			Message: errorMsg,
		}, nil
	}
}

// handleCreateProject processes project creation
func (m *ProjectManagementModule) handleCreateProject(employeeID, userID string, data map[string]interface{}) (*erp_modules.ModuleResponse, error) {
	var projectRequest ProjectCreationRequest
	dataBytes, _ := json.Marshal(data)
	json.Unmarshal(dataBytes, &projectRequest)

	projectName, err := m.erpClient.CreateProject(projectRequest, employeeID)
	if err != nil {
		user, _ := m.api.GetUser(userID)
		isVietnamese := detectUserLanguage(user)
		errorMsg := "⚠️ Có lỗi xảy ra khi tạo dự án trong hệ thống. Vui lòng thử lại."
		if !isVietnamese {
			errorMsg = "⚠️ An error occurred while creating project in the system. Please try again."
		}
		return &erp_modules.ModuleResponse{
			Success: false,
			Message: errorMsg,
			Error:   err.Error(),
		}, nil
	}

	user, _ := m.api.GetUser(userID)
	isVietnamese := detectUserLanguage(user)
	successMsg := fmt.Sprintf("Đã tạo dự án mới thành công: **%s**!", projectName)
	if !isVietnamese {
		successMsg = fmt.Sprintf("Successfully created new project: **%s**!", projectName)
	}

	return &erp_modules.ModuleResponse{
		Success:     true,
		Message:     successMsg,
		ActionTaken: "create_project",
		Data: map[string]interface{}{
			"project_name": projectName,
			"description":  projectRequest.Description,
			"priority":     projectRequest.Priority,
		},
	}, nil
}

// handleCreateTask processes task creation
func (m *ProjectManagementModule) handleCreateTask(employeeID, userID string, data map[string]interface{}) (*erp_modules.ModuleResponse, error) {
	var taskRequest TaskCreationRequest
	dataBytes, _ := json.Marshal(data)
	json.Unmarshal(dataBytes, &taskRequest)

	taskName, err := m.erpClient.CreateTask(taskRequest, employeeID)
	if err != nil {
		user, _ := m.api.GetUser(userID)
		isVietnamese := detectUserLanguage(user)
		errorMsg := "⚠️ Có lỗi xảy ra khi tạo task trong hệ thống. Vui lòng thử lại."
		if !isVietnamese {
			errorMsg = "⚠️ An error occurred while creating task in the system. Please try again."
		}
		return &erp_modules.ModuleResponse{
			Success: false,
			Message: errorMsg,
			Error:   err.Error(),
		}, nil
	}

	user, _ := m.api.GetUser(userID)
	isVietnamese := detectUserLanguage(user)
	successMsg := fmt.Sprintf("Đã tạo task mới thành công: **%s**!", taskName)
	if !isVietnamese {
		successMsg = fmt.Sprintf("Successfully created new task: **%s**!", taskName)
	}

	return &erp_modules.ModuleResponse{
		Success:     true,
		Message:     successMsg,
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

// getEmployeeIDFromUser gets employee ID from user
func (m *ProjectManagementModule) getEmployeeIDFromUser(user *model.User) (string, error) {
	// Use the user's ID as the chat ID to lookup in ERPNext
	chatID := user.Id

	employeeID, err := m.erpClient.GetEmployeeByChatID(chatID)
	if err != nil {
		return "", fmt.Errorf("failed to get employee by chat ID %s: %w", chatID, err)
	}

	return employeeID, nil
}
