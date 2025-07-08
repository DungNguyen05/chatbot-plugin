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

	// Process assignee if provided (using dedicated function)
	m.processAssigneeForProject(projectRequest)

	// Get creator's email for assignment
	creatorEmail, err := m.getEmployeeEmailFromUser(ctx.User)
	if err != nil {
		m.api.LogWarn("Failed to get creator email", "error", err.Error())
		creatorEmail = "demo@example.com" // Fallback
	}

	// Request confirmation
	return m.requestConfirmation(ctx, "create_project", "project", projectRequest, employeeID, creatorEmail)
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

	// Process assignee if provided (using dedicated function)
	m.processAssigneeForTask(taskRequest)

	// Get creator's email for assignment
	creatorEmail, err := m.getEmployeeEmailFromUser(ctx.User)
	if err != nil {
		m.api.LogWarn("Failed to get creator email", "error", err.Error())
		creatorEmail = "demo@example.com" // Fallback
	}

	// Request confirmation
	return m.requestConfirmation(ctx, "create_task", "task", taskRequest, employeeID, creatorEmail)
}

// requestConfirmation requests confirmation from user (UPDATED)
func (m *ProjectManagementModule) requestConfirmation(ctx *erp_modules.ModuleContext, action, confirmationType string, data interface{}, employeeID, creatorEmail string) (*erp_modules.ModuleResponse, error) {
	// Convert data to map for consistent storage
	dataMap := make(map[string]interface{})
	dataBytes, _ := json.Marshal(data)
	json.Unmarshal(dataBytes, &dataMap)

	// Extract assignee information for confirmation
	var assignedToEmail, assignedToName string
	if email, ok := dataMap["assigned_to_email"].(string); ok {
		assignedToEmail = email
	}
	if name, ok := dataMap["assigned_to_name"].(string); ok {
		assignedToName = name
	}

	// Store pending confirmation with additional email info
	m.confirmationManager.StorePendingConfirmation(ctx.User.Id, &ProjectManagementConfirmation{
		UserID:          ctx.User.Id,
		Type:            confirmationType,
		Action:          action,
		Data:            dataMap,
		CreatedAt:       time.Now().UnixMilli(),
		EmployeeID:      employeeID,
		CreatorEmail:    creatorEmail,
		AssignedToEmail: assignedToEmail,
		AssignedToName:  assignedToName,
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
		return m.handleCreateProject(pending.EmployeeID, ctx.User.Id, pending.Data, pending.CreatorEmail)
	case "create_task":
		return m.handleCreateTask(pending.EmployeeID, ctx.User.Id, pending.Data, pending.CreatorEmail)
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

// handleCreateProject processes project creation (UPDATED for two-step process)
func (m *ProjectManagementModule) handleCreateProject(employeeID, userID string, data map[string]interface{}, creatorEmail string) (*erp_modules.ModuleResponse, error) {
	var projectRequest ProjectCreationRequest
	dataBytes, _ := json.Marshal(data)
	json.Unmarshal(dataBytes, &projectRequest)

	// Use the new two-step creation method
	projectID, err := m.erpClient.CreateProjectWithAssignment(projectRequest, employeeID, creatorEmail)
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

	var successMsg string
	if projectRequest.AssignedToEmployeeID != "" && projectRequest.AssignedToEmail != "" {
		if isVietnamese {
			successMsg = fmt.Sprintf("✅ Đã tạo dự án thành công: **%s** và phân công cho **%s**!", projectID, projectRequest.AssignedToName)
		} else {
			successMsg = fmt.Sprintf("✅ Successfully created project: **%s** and assigned to **%s**!", projectID, projectRequest.AssignedToName)
		}
	} else {
		if isVietnamese {
			successMsg = fmt.Sprintf("✅ Đã tạo dự án thành công: **%s**!", projectID)
		} else {
			successMsg = fmt.Sprintf("✅ Successfully created project: **%s**!", projectID)
		}
	}

	return &erp_modules.ModuleResponse{
		Success:     true,
		Message:     successMsg,
		ActionTaken: "create_project",
		Data: map[string]interface{}{
			"project_id":              projectID,
			"project_name":            projectRequest.ProjectName,
			"description":             projectRequest.Description,
			"priority":                projectRequest.Priority,
			"assigned_to_employee_id": projectRequest.AssignedToEmployeeID,
			"assigned_to_email":       projectRequest.AssignedToEmail,
			"assigned_to_name":        projectRequest.AssignedToName,
		},
	}, nil
}

// handleCreateTask processes task creation (UPDATED for two-step process)
func (m *ProjectManagementModule) handleCreateTask(employeeID, userID string, data map[string]interface{}, creatorEmail string) (*erp_modules.ModuleResponse, error) {
	var taskRequest TaskCreationRequest
	dataBytes, _ := json.Marshal(data)
	json.Unmarshal(dataBytes, &taskRequest)

	// Use the new two-step creation method
	taskID, err := m.erpClient.CreateTaskWithAssignment(taskRequest, employeeID, creatorEmail)
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

	var successMsg string
	if taskRequest.AssignedToEmployeeID != "" && taskRequest.AssignedToEmail != "" {
		if isVietnamese {
			successMsg = fmt.Sprintf("✅ Đã tạo task thành công: **%s** và phân công cho **%s**!", taskID, taskRequest.AssignedToName)
		} else {
			successMsg = fmt.Sprintf("✅ Successfully created task: **%s** and assigned to **%s**!", taskID, taskRequest.AssignedToName)
		}
	} else {
		if isVietnamese {
			successMsg = fmt.Sprintf("✅ Đã tạo task thành công: **%s**!", taskID)
		} else {
			successMsg = fmt.Sprintf("✅ Successfully created task: **%s**!", taskID)
		}
	}

	return &erp_modules.ModuleResponse{
		Success:     true,
		Message:     successMsg,
		ActionTaken: "create_task",
		Data: map[string]interface{}{
			"task_id":                 taskID,
			"task_name":               taskRequest.Subject,
			"description":             taskRequest.Description,
			"assigned_to_employee_id": taskRequest.AssignedToEmployeeID,
			"assigned_to_email":       taskRequest.AssignedToEmail,
			"assigned_to_name":        taskRequest.AssignedToName,
			"priority":                taskRequest.Priority,
			"project":                 taskRequest.Project,
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

// getEmployeeEmailFromUser gets employee email from user - NEW METHOD
func (m *ProjectManagementModule) getEmployeeEmailFromUser(user *model.User) (string, error) {
	// First get employee ID
	employeeID, err := m.getEmployeeIDFromUser(user)
	if err != nil {
		return "", err
	}

	// Then get email from employee ID
	email, err := m.erpClient.GetEmployeeEmail(employeeID)
	if err != nil {
		return "", fmt.Errorf("failed to get email for employee %s: %w", employeeID, err)
	}

	return email, nil
}
