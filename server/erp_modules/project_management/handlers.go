// Copyright (c) 2023-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package project_management

import (
	"fmt"
	"sync"
	"time"

	"github.com/mattermost/mattermost-plugin-ai/server/erp_modules"
	"github.com/mattermost/mattermost/server/public/model"
)

// ProjectConfirmation represents pending project/task confirmation
type ProjectConfirmation struct {
	UserID     string                 `json:"user_id"`
	Type       string                 `json:"type"` // "project", "task"
	Data       map[string]interface{} `json:"data"`
	CreatedAt  int64                  `json:"created_at"`
	EmployeeID string                 `json:"employee_id"`
}

// ConfirmationManager handles confirmation state
type ConfirmationManager struct {
	confirmationState map[string]*ProjectConfirmation
	confirmationMutex sync.RWMutex
}

// NewConfirmationManager creates a new confirmation manager
func NewConfirmationManager() *ConfirmationManager {
	return &ConfirmationManager{
		confirmationState: make(map[string]*ProjectConfirmation),
	}
}

// StorePendingConfirmation stores a pending confirmation
func (cm *ConfirmationManager) StorePendingConfirmation(userID string, confirmation *ProjectConfirmation) {
	cm.confirmationMutex.Lock()
	defer cm.confirmationMutex.Unlock()
	cm.confirmationState[userID] = confirmation
}

// GetPendingConfirmation retrieves a pending confirmation
func (cm *ConfirmationManager) GetPendingConfirmation(userID string) (*ProjectConfirmation, bool) {
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

// handleCreateProjectRequest processes project creation request with optional confirmation
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

	// For high confidence, execute directly
	if intent.Confidence >= 0.9 {
		return m.handleCreateProject(employeeID, getUserDisplayName(ctx.User), ctx.User.Id, projectRequest)
	}

	// Request confirmation for medium confidence
	return m.requestProjectConfirmation(ctx, "project", projectRequest, employeeID)
}

// handleCreateTaskRequest processes task creation request with optional confirmation
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

	// For high confidence, execute directly
	if intent.Confidence >= 0.9 {
		return m.handleCreateTask(employeeID, getUserDisplayName(ctx.User), ctx.User.Id, taskRequest)
	}

	// Request confirmation for medium confidence
	return m.requestTaskConfirmation(ctx, "task", taskRequest, employeeID)
}

// requestProjectConfirmation requests confirmation from user for project creation
func (m *ProjectManagementModule) requestProjectConfirmation(ctx *erp_modules.ModuleContext, confirmationType string, data *ProjectCreationRequest, employeeID string) (*erp_modules.ModuleResponse, error) {
	// Convert data to map for storage
	dataMap := convertToMap(data)

	// Store pending confirmation
	m.confirmationManager.StorePendingConfirmation(ctx.User.Id, &ProjectConfirmation{
		UserID:     ctx.User.Id,
		Type:       confirmationType,
		Data:       dataMap,
		CreatedAt:  time.Now().UnixMilli(),
		EmployeeID: employeeID,
	})

	// Generate confirmation message
	confirmationMsg, err := m.generateProjectConfirmationMessage(ctx, confirmationType, data)
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

// requestTaskConfirmation requests confirmation from user for task creation
func (m *ProjectManagementModule) requestTaskConfirmation(ctx *erp_modules.ModuleContext, confirmationType string, data *TaskCreationRequest, employeeID string) (*erp_modules.ModuleResponse, error) {
	// Convert data to map for storage
	dataMap := convertToMap(data)

	// Store pending confirmation
	m.confirmationManager.StorePendingConfirmation(ctx.User.Id, &ProjectConfirmation{
		UserID:     ctx.User.Id,
		Type:       confirmationType,
		Data:       dataMap,
		CreatedAt:  time.Now().UnixMilli(),
		EmployeeID: employeeID,
	})

	// Generate confirmation message
	confirmationMsg, err := m.generateTaskConfirmationMessage(ctx, confirmationType, data)
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
func (m *ProjectManagementModule) executeConfirmedAction(ctx *erp_modules.ModuleContext, pending *ProjectConfirmation) (*erp_modules.ModuleResponse, error) {
	employeeName := getUserDisplayName(ctx.User)

	switch pending.Type {
	case "project":
		var projectRequest ProjectCreationRequest
		if err := convertFromMap(pending.Data, &projectRequest); err != nil {
			return &erp_modules.ModuleResponse{
				Success: false,
				Message: "Error converting project data",
				Error:   err.Error(),
			}, nil
		}
		return m.handleCreateProject(pending.EmployeeID, employeeName, ctx.User.Id, &projectRequest)

	case "task":
		var taskRequest TaskCreationRequest
		if err := convertFromMap(pending.Data, &taskRequest); err != nil {
			return &erp_modules.ModuleResponse{
				Success: false,
				Message: "Error converting task data",
				Error:   err.Error(),
			}, nil
		}
		return m.handleCreateTask(pending.EmployeeID, employeeName, ctx.User.Id, &taskRequest)

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
func (m *ProjectManagementModule) handleCreateProject(employeeID, employeeName, userID string, projectRequest *ProjectCreationRequest) (*erp_modules.ModuleResponse, error) {
	projectName, err := m.erpClient.CreateProject(*projectRequest, employeeID)
	if err != nil {
		user, _ := m.api.GetUser(userID)
		isVietnamese := detectUserLanguage(user)
		errorMsg := "⚠️ Có lỗi xảy ra khi tạo dự án. Vui lòng thử lại."
		if !isVietnamese {
			errorMsg = "⚠️ An error occurred while creating project. Please try again."
		}
		return &erp_modules.ModuleResponse{
			Success: false,
			Message: errorMsg,
			Error:   err.Error(),
		}, nil
	}

	user, _ := m.api.GetUser(userID)
	isVietnamese := detectUserLanguage(user)
	successMsg := fmt.Sprintf("Đã tạo dự án **%s** thành công!", projectName)
	if !isVietnamese {
		successMsg = fmt.Sprintf("Successfully created project **%s**!", projectName)
	}

	return &erp_modules.ModuleResponse{
		Success:     true,
		Message:     successMsg,
		ActionTaken: "create_project",
		Data: map[string]interface{}{
			"project_name": projectName,
			"employee":     employeeName,
		},
	}, nil
}

// handleCreateTask processes task creation
func (m *ProjectManagementModule) handleCreateTask(employeeID, employeeName, userID string, taskRequest *TaskCreationRequest) (*erp_modules.ModuleResponse, error) {
	taskName, err := m.erpClient.CreateTask(*taskRequest, employeeID)
	if err != nil {
		user, _ := m.api.GetUser(userID)
		isVietnamese := detectUserLanguage(user)
		errorMsg := "⚠️ Có lỗi xảy ra khi tạo task. Vui lòng thử lại."
		if !isVietnamese {
			errorMsg = "⚠️ An error occurred while creating task. Please try again."
		}
		return &erp_modules.ModuleResponse{
			Success: false,
			Message: errorMsg,
			Error:   err.Error(),
		}, nil
	}

	user, _ := m.api.GetUser(userID)
	isVietnamese := detectUserLanguage(user)
	successMsg := fmt.Sprintf("Đã tạo task **%s** thành công!", taskName)
	if !isVietnamese {
		successMsg = fmt.Sprintf("Successfully created task **%s**!", taskName)
	}

	return &erp_modules.ModuleResponse{
		Success:     true,
		Message:     successMsg,
		ActionTaken: "create_task",
		Data: map[string]interface{}{
			"task_name": taskName,
			"employee":  employeeName,
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
