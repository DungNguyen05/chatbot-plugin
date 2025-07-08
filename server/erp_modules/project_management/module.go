// Copyright (c) 2023-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package project_management

import (
	"fmt"
	"net/http"

	"github.com/mattermost/mattermost-plugin-ai/server/erp_modules"
	"github.com/mattermost/mattermost-plugin-ai/server/llm"
)

// ProjectManagementModule handles project and task management operations
type ProjectManagementModule struct {
	config              ProjectManagementConfig
	erpClient           *ERPClient
	api                 PluginAPI
	prompts             PromptsInterface
	getLLM              func() llm.LanguageModel
	confirmationManager *ConfirmationManager
}

// NewProjectManagementModule creates a new project management module
func NewProjectManagementModule(
	config ProjectManagementConfig,
	httpClient *http.Client,
	prompts PromptsInterface,
	getLLM func() llm.LanguageModel,
	api PluginAPI,
) *ProjectManagementModule {
	// Validate configuration
	if err := ValidateProjectManagementConfig(config); err != nil {
		api.LogError("Invalid project management configuration", "error", err.Error())
		// Continue with disabled module - this matches attendance module pattern
	}

	// Create ERP client
	erpClient := NewERPClient(config, httpClient, api)

	// Create confirmation manager
	confirmationManager := NewConfirmationManager()

	return &ProjectManagementModule{
		config:              config,
		erpClient:           erpClient,
		api:                 api,
		prompts:             prompts,
		getLLM:              getLLM,
		confirmationManager: confirmationManager,
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

	return isValidAction(intent.Action)
}

// ProcessUserMessage handles user messages including confirmations and modifications
func (m *ProjectManagementModule) ProcessUserMessage(ctx *erp_modules.ModuleContext, message string) (*erp_modules.ModuleResponse, error) {
	// Sanitize user input
	message = sanitizeUserInput(message)

	pending, hasPending := m.confirmationManager.GetPendingConfirmation(ctx.User.Id)
	if !hasPending {
		return nil, nil // Not handling this message
	}

	// Parse user response using LLM
	userResponse, err := m.parseConfirmationResponse(ctx, message, pending)
	if err != nil {
		m.api.LogError("Failed to parse user response", "error", err.Error())
		isVietnamese := detectUserLanguage(ctx.User)
		errorMsg := "⚠️ Không thể hiểu phản hồi của bạn. Vui lòng thử lại."
		if !isVietnamese {
			errorMsg = "⚠️ Cannot understand your response. Please try again."
		}
		return &erp_modules.ModuleResponse{
			Success: false,
			Message: errorMsg,
		}, nil
	}

	// Clear pending confirmation
	m.confirmationManager.ClearPendingConfirmation(ctx.User.Id)

	switch userResponse.Intent {
	case "confirm":
		return m.executeConfirmedAction(ctx, pending)
	case "modify":
		return m.handleModifyAction(ctx, pending, userResponse.Modifications)
	case "cancel":
		return m.handleCancelAction(ctx.User.Id)
	default:
		isVietnamese := detectUserLanguage(ctx.User)
		errorMsg := "⚠️ Vui lòng xác nhận (có/yes), chỉnh sửa thông tin, hoặc hủy bỏ (không/cancel)."
		if !isVietnamese {
			errorMsg = "⚠️ Please confirm (yes), modify information, or cancel (no/cancel)."
		}
		return &erp_modules.ModuleResponse{
			Success: false,
			Message: errorMsg,
		}, nil
	}
}

// Execute processes the project management intent
func (m *ProjectManagementModule) Execute(ctx *erp_modules.ModuleContext, intent *erp_modules.Intent) (*erp_modules.ModuleResponse, error) {
	isVietnamese := detectUserLanguage(ctx.User)

	// Get employee ID
	employeeID, err := m.getEmployeeIDFromUser(ctx.User)
	if err != nil {
		errorMsg := "Không tìm thấy thông tin nhân viên của bạn trong hệ thống ERP. Vui lòng liên hệ quản trị viên."
		if !isVietnamese {
			errorMsg = "Cannot find your employee information in the ERP system. Please contact administrator."
		}
		return &erp_modules.ModuleResponse{
			Success: false,
			Message: errorMsg,
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
		errorMsg := "Hành động không được hỗ trợ"
		if !isVietnamese {
			errorMsg = "Action not supported"
		}
		return &erp_modules.ModuleResponse{
			Success: false,
			Message: errorMsg,
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
			"tạo dự án cho Minh",
			"create project assign to John",
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
			"tạo task cho Nam",
			"create task assign to Mary",
		},
	}
}

// handleModifyAction handles user modification requests
func (m *ProjectManagementModule) handleModifyAction(ctx *erp_modules.ModuleContext, pending *ProjectManagementConfirmation, modifications map[string]interface{}) (*erp_modules.ModuleResponse, error) {
	// Handle assignee modification using dedicated function
	m.handleAssigneeModification(modifications)

	// Update the pending data with modifications
	for key, value := range modifications {
		pending.Data[key] = value
	}

	// Update the stored confirmation
	m.confirmationManager.StorePendingConfirmation(ctx.User.Id, pending)

	// Generate new confirmation message with updated data
	confirmationMsg, err := m.generateConfirmationMessage(ctx, pending.Type, pending.Data)
	if err != nil {
		isVietnamese := detectUserLanguage(ctx.User)
		errorMsg := "⚠️ Có lỗi xảy ra khi tạo tin nhắn xác nhận cập nhật."
		if !isVietnamese {
			errorMsg = "⚠️ An error occurred while creating updated confirmation message."
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
	return &erp_modules.ModuleResponse{
		Success:     true,
		Message:     "Đã hủy bỏ yêu cầu tạo dự án/task.",
		ActionTaken: "cancel_confirmation",
	}, nil
}
