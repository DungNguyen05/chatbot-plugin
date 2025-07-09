// Copyright (c) 2023-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package project_management

import (
	"fmt"
	"net/http"
	"time"

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

// ProcessUserMessage handles user messages including confirmations, modifications, and disambiguations - UPDATED
func (m *ProjectManagementModule) ProcessUserMessage(ctx *erp_modules.ModuleContext, message string) (*erp_modules.ModuleResponse, error) {
	// Sanitize user input
	message = sanitizeUserInput(message)

	// Check for pending modification disambiguation first - NEW
	modificationDisambiguation, hasModificationDisambiguation := m.confirmationManager.GetPendingModificationDisambiguation(ctx.User.Id)
	if hasModificationDisambiguation {
		return m.handleModificationDisambiguationResponse(ctx, message, modificationDisambiguation)
	}

	// Check for pending disambiguation
	disambiguation, hasDisambiguation := m.confirmationManager.GetPendingDisambiguation(ctx.User.Id)
	if hasDisambiguation {
		return m.handleEmployeeDisambiguationResponse(ctx, message, disambiguation)
	}

	// Check for pending confirmation
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

// handleModificationDisambiguationResponse handles user response to modification employee selection - NEW
func (m *ProjectManagementModule) handleModificationDisambiguationResponse(ctx *erp_modules.ModuleContext, message string, modificationDisambiguation *ModificationDisambiguationConfirmation) (*erp_modules.ModuleResponse, error) {
	// Parse disambiguation response using LLM - we can reuse the same parsing logic
	disambiguationResponse, err := m.parseEmployeeDisambiguationResponse(ctx, message, &EmployeeDisambiguationConfirmation{
		AvailableEmployees: modificationDisambiguation.AvailableEmployees,
	})
	if err != nil {
		m.api.LogError("Failed to parse modification disambiguation response", "error", err.Error())
		isVietnamese := detectUserLanguage(ctx.User)
		errorMsg := "⚠️ Không thể hiểu lựa chọn của bạn. Vui lòng chọn số thứ tự (1, 2, 3, ...)."
		if !isVietnamese {
			errorMsg = "⚠️ Cannot understand your selection. Please choose by number (1, 2, 3, ...)."
		}
		return &erp_modules.ModuleResponse{
			Success: false,
			Message: errorMsg,
		}, nil
	}

	// Clear pending modification disambiguation
	m.confirmationManager.ClearPendingConfirmation(ctx.User.Id)

	// Process the selection
	assigneeResult, err := m.resolveDisambiguatedEmployee(
		modificationDisambiguation.AvailableEmployees,
		disambiguationResponse.SelectedIndex,
		disambiguationResponse.ClarificationText,
		disambiguationResponse.Intent,
	)

	if err != nil {
		isVietnamese := detectUserLanguage(ctx.User)
		errorMsg := "⚠️ Có lỗi xảy ra khi xử lý lựa chọn nhân viên."
		if !isVietnamese {
			errorMsg = "⚠️ An error occurred while processing employee selection."
		}
		return &erp_modules.ModuleResponse{
			Success: false,
			Message: errorMsg,
			Error:   err.Error(),
		}, nil
	}

	if !assigneeResult.Found {
		isVietnamese := detectUserLanguage(ctx.User)
		errorMsg := assigneeResult.Error
		if errorMsg == "" {
			if isVietnamese {
				errorMsg = "Không thể chọn nhân viên."
			} else {
				errorMsg = "Unable to select employee."
			}
		}
		return &erp_modules.ModuleResponse{
			Success: false,
			Message: errorMsg,
		}, nil
	}

	// Apply the selected employee to the pending modifications
	modificationDisambiguation.PendingModifications["assigned_to_employee_id"] = assigneeResult.EmployeeID
	modificationDisambiguation.PendingModifications["assigned_to_email"] = assigneeResult.EmployeeEmail
	modificationDisambiguation.PendingModifications["assigned_to_name"] = assigneeResult.EmployeeName

	// Now apply all modifications to the original confirmation
	originalConfirmation := modificationDisambiguation.OriginalConfirmation

	// Update the pending data with all modifications
	for key, value := range modificationDisambiguation.PendingModifications {
		originalConfirmation.Data[key] = value
	}

	// Update assignee info in confirmation
	originalConfirmation.AssignedToEmail = assigneeResult.EmployeeEmail
	originalConfirmation.AssignedToName = assigneeResult.EmployeeName

	// Store the updated confirmation
	m.confirmationManager.StorePendingConfirmation(ctx.User.Id, originalConfirmation)

	// Generate new confirmation message with updated data
	confirmationMsg, err := m.generateConfirmationMessage(ctx, originalConfirmation.Type, originalConfirmation.Data)
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

	isVietnamese := detectUserLanguage(ctx.User)
	var selectedMsg string
	if isVietnamese {
		selectedMsg = fmt.Sprintf("**Đã chọn nhân viên:** %s (%s)\n\n", assigneeResult.EmployeeName, assigneeResult.EmployeeEmail)
	} else {
		selectedMsg = fmt.Sprintf("**Selected employee:** %s (%s)\n\n", assigneeResult.EmployeeName, assigneeResult.EmployeeEmail)
	}

	return &erp_modules.ModuleResponse{
		Success:     true,
		Message:     selectedMsg + confirmationMsg,
		ActionTaken: "modification_employee_selected_proceed_confirmation",
		Data: map[string]interface{}{
			"awaiting_confirmation": true,
			"type":                  originalConfirmation.Type,
			"selected_employee":     assigneeResult.EmployeeName,
			"updated":               true,
		},
	}, nil
}

// handleEmployeeDisambiguationResponse handles user response to employee selection - NEW
func (m *ProjectManagementModule) handleEmployeeDisambiguationResponse(ctx *erp_modules.ModuleContext, message string, disambiguation *EmployeeDisambiguationConfirmation) (*erp_modules.ModuleResponse, error) {
	// Parse disambiguation response using LLM
	disambiguationResponse, err := m.parseEmployeeDisambiguationResponse(ctx, message, disambiguation)
	if err != nil {
		m.api.LogError("Failed to parse disambiguation response", "error", err.Error())
		isVietnamese := detectUserLanguage(ctx.User)
		errorMsg := "⚠️ Không thể hiểu lựa chọn của bạn. Vui lòng chọn số thứ tự (1, 2, 3, ...)."
		if !isVietnamese {
			errorMsg = "⚠️ Cannot understand your selection. Please choose by number (1, 2, 3, ...)."
		}
		return &erp_modules.ModuleResponse{
			Success: false,
			Message: errorMsg,
		}, nil
	}

	// Clear pending disambiguation
	m.confirmationManager.ClearPendingConfirmation(ctx.User.Id)

	// Process the selection
	assigneeResult, err := m.resolveDisambiguatedEmployee(
		disambiguation.AvailableEmployees,
		disambiguationResponse.SelectedIndex,
		disambiguationResponse.ClarificationText,
		disambiguationResponse.Intent,
	)

	if err != nil {
		isVietnamese := detectUserLanguage(ctx.User)
		errorMsg := "⚠️ Có lỗi xảy ra khi xử lý lựa chọn nhân viên."
		if !isVietnamese {
			errorMsg = "⚠️ An error occurred while processing employee selection."
		}
		return &erp_modules.ModuleResponse{
			Success: false,
			Message: errorMsg,
			Error:   err.Error(),
		}, nil
	}

	if !assigneeResult.Found {
		isVietnamese := detectUserLanguage(ctx.User)
		errorMsg := assigneeResult.Error
		if errorMsg == "" {
			if isVietnamese {
				errorMsg = "Không thể chọn nhân viên."
			} else {
				errorMsg = "Unable to select employee."
			}
		}
		return &erp_modules.ModuleResponse{
			Success: false,
			Message: errorMsg,
		}, nil
	}

	// Update the data with selected employee
	disambiguation.Data["assigned_to_employee_id"] = assigneeResult.EmployeeID
	disambiguation.Data["assigned_to_email"] = assigneeResult.EmployeeEmail
	disambiguation.Data["assigned_to_name"] = assigneeResult.EmployeeName

	// Convert back to proper request structure and proceed with confirmation
	return m.proceedWithConfirmationAfterDisambiguation(ctx, disambiguation, assigneeResult)
}

// proceedWithConfirmationAfterDisambiguation proceeds with confirmation after employee selection - NEW
func (m *ProjectManagementModule) proceedWithConfirmationAfterDisambiguation(ctx *erp_modules.ModuleContext, disambiguation *EmployeeDisambiguationConfirmation, assigneeResult *AssigneeResolutionResult) (*erp_modules.ModuleResponse, error) {
	// Store regular confirmation with selected employee
	confirmation := &ProjectManagementConfirmation{
		UserID:          disambiguation.UserID,
		Type:            disambiguation.Type,
		Action:          disambiguation.Action,
		Data:            disambiguation.Data,
		CreatedAt:       time.Now().UnixMilli(),
		EmployeeID:      disambiguation.EmployeeID,
		CreatorEmail:    disambiguation.CreatorEmail,
		AssignedToEmail: assigneeResult.EmployeeEmail,
		AssignedToName:  assigneeResult.EmployeeName,
	}

	m.confirmationManager.StorePendingConfirmation(ctx.User.Id, confirmation)

	// Generate confirmation message
	confirmationMsg, err := m.generateConfirmationMessage(ctx, disambiguation.Type, disambiguation.Data)
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

	isVietnamese := detectUserLanguage(ctx.User)
	var selectedMsg string
	if isVietnamese {
		selectedMsg = fmt.Sprintf("**Đã chọn nhân viên:** %s (%s)\n\n", assigneeResult.EmployeeName, assigneeResult.EmployeeEmail)
	} else {
		selectedMsg = fmt.Sprintf("**Selected employee:** %s (%s)\n\n", assigneeResult.EmployeeName, assigneeResult.EmployeeEmail)
	}

	return &erp_modules.ModuleResponse{
		Success:     true,
		Message:     selectedMsg + confirmationMsg,
		ActionTaken: "employee_selected_proceed_confirmation",
		Data: map[string]interface{}{
			"awaiting_confirmation": true,
			"type":                  disambiguation.Type,
			"selected_employee":     assigneeResult.EmployeeName,
		},
	}, nil
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
	return "Quản lý dự án và công việc: tạo dự án mới, tạo task, phân công công việc với khả năng gán nhân viên tự động và xử lý trường hợp trùng tên"
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
			"tạo dự án phân công cho Nguyễn Văn An",
			"create marketing project assign to Mary",
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
			"tạo task thiết kế giao diện cho developer",
			"create API development task assign to backend team",
		},
	}
}

// handleModifyAction handles user modification requests - ENHANCED WITH DISAMBIGUATION
func (m *ProjectManagementModule) handleModifyAction(ctx *erp_modules.ModuleContext, pending *ProjectManagementConfirmation, modifications map[string]interface{}) (*erp_modules.ModuleResponse, error) {
	// Check if the modification involves an assignee change that might need disambiguation
	if assigneeName, ok := modifications["assigned_to_name"].(string); ok && assigneeName != "" {
		// Try to resolve the assignee
		assigneeResult, err := m.resolveAssignee(assigneeName)
		if err != nil {
			m.api.LogError("Failed to resolve modified assignee", "error", err.Error())
			// Continue with regular modification handling (clear assignee)
			modifications["assigned_to_employee_id"] = ""
			modifications["assigned_to_email"] = ""
		} else if assigneeResult.RequiresDisambiguation {
			// Store modification disambiguation state
			modificationDisambiguation := &ModificationDisambiguationConfirmation{
				UserID:               ctx.User.Id,
				OriginalConfirmation: pending,
				ModificationField:    "assigned_to_name",
				OriginalSearchName:   assigneeName,
				AvailableEmployees:   assigneeResult.MatchingEmployees,
				PendingModifications: modifications,
				CreatedAt:            time.Now().UnixMilli(),
			}

			m.confirmationManager.StorePendingModificationDisambiguation(ctx.User.Id, modificationDisambiguation)

			// Generate disambiguation message
			disambiguationMsg, err := m.generateDisambiguationMessage(ctx, assigneeResult.MatchingEmployees)
			if err != nil {
				isVietnamese := detectUserLanguage(ctx.User)
				errorMsg := "⚠️ Có lỗi xảy ra khi tạo tin nhắn lựa chọn nhân viên."
				if !isVietnamese {
					errorMsg = "⚠️ An error occurred while creating employee selection message."
				}
				return &erp_modules.ModuleResponse{
					Success: false,
					Message: errorMsg,
					Error:   err.Error(),
				}, nil
			}

			return &erp_modules.ModuleResponse{
				Success:     true,
				Message:     disambiguationMsg,
				ActionTaken: "request_modification_employee_disambiguation",
				Data: map[string]interface{}{
					"awaiting_disambiguation": true,
					"type":                    "modification",
					"employee_count":          len(assigneeResult.MatchingEmployees),
				},
			}, nil
		} else if assigneeResult.Found {
			// Single match found, proceed with regular modification
			modifications["assigned_to_employee_id"] = assigneeResult.EmployeeID
			modifications["assigned_to_email"] = assigneeResult.EmployeeEmail
			modifications["assigned_to_name"] = assigneeResult.EmployeeName
		} else {
			// No match found, clear assignee fields
			modifications["assigned_to_employee_id"] = ""
			modifications["assigned_to_email"] = ""
		}
	} else {
		// Handle other assignee modifications using existing logic
		m.handleAssigneeModification(modifications)
	}

	// Update the pending data with modifications
	for key, value := range modifications {
		pending.Data[key] = value
	}

	// Update assignee info in confirmation if modified
	if email, ok := modifications["assigned_to_email"].(string); ok {
		pending.AssignedToEmail = email
	}
	if name, ok := modifications["assigned_to_name"].(string); ok {
		pending.AssignedToName = name
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
