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
	confirmationManager *ConfirmationManager
	api                 PluginAPI
	prompts             PromptsInterface
	getLLM              func() llm.LanguageModel
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
		// Continue with disabled module
		config.Enabled = false
	}

	// Create ERP client
	erpClient := NewERPClient(config, httpClient, api)

	// Create confirmation manager
	confirmationManager := NewConfirmationManager()

	return &ProjectManagementModule{
		config:              config,
		erpClient:           erpClient,
		confirmationManager: confirmationManager,
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
	if !m.config.Enabled {
		return false
	}

	if intent.Category != "project_management" {
		return false
	}

	return isValidAction(intent.Action)
}

// ProcessUserMessage handles user messages including confirmations
func (m *ProjectManagementModule) ProcessUserMessage(ctx *erp_modules.ModuleContext, message string) (*erp_modules.ModuleResponse, error) {
	if !m.config.Enabled {
		return nil, nil
	}

	// Sanitize user input
	message = sanitizeUserInput(message)

	pending, hasPending := m.confirmationManager.GetPendingConfirmation(ctx.User.Id)
	if !hasPending {
		return nil, nil // Not handling this message
	}

	// Parse user response using LLM
	confirmed, err := m.parseConfirmationResponse(ctx, message)
	if err != nil {
		m.api.LogError("Failed to parse confirmation response", "error", err.Error())
		isVietnamese := detectUserLanguage(ctx.User)
		errorMsg := "⚠️ Không thể hiểu phản hồi của bạn. Vui lòng trả lời 'có' để xác nhận hoặc 'không' để hủy bỏ."
		if !isVietnamese {
			errorMsg = "⚠️ Cannot understand your response. Please reply 'yes' to confirm or 'no' to cancel."
		}
		return &erp_modules.ModuleResponse{
			Success: false,
			Message: errorMsg,
		}, nil
	}

	// Clear pending confirmation
	m.confirmationManager.ClearPendingConfirmation(ctx.User.Id)

	if confirmed {
		return m.executeConfirmedAction(ctx, pending)
	} else {
		isVietnamese := detectUserLanguage(ctx.User)
		cancelMsg := "Đã hủy bỏ yêu cầu."
		if !isVietnamese {
			cancelMsg = "Request cancelled."
		}
		return &erp_modules.ModuleResponse{
			Success:     true,
			Message:     cancelMsg,
			ActionTaken: "cancel_confirmation",
		}, nil
	}
}

// Execute processes the project management intent
func (m *ProjectManagementModule) Execute(ctx *erp_modules.ModuleContext, intent *erp_modules.Intent) (*erp_modules.ModuleResponse, error) {
	if !m.config.Enabled {
		isVietnamese := detectUserLanguage(ctx.User)
		errorMsg := "Tính năng quản lý dự án hiện không khả dụng."
		if !isVietnamese {
			errorMsg = "Project management feature is currently not available."
		}
		return &erp_modules.ModuleResponse{
			Success: false,
			Message: errorMsg,
		}, nil
	}

	isVietnamese := detectUserLanguage(ctx.User)

	// Execute specific action based on intent.Action
	switch intent.Action {
	case "create_project":
		// Get employee ID for project creation
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
		return m.handleCreateProjectRequest(employeeID, ctx, intent)

	case "create_task":
		// Get employee ID for task creation
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
