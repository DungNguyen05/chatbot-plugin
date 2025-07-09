// Copyright (c) 2023-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package project_management

import (
	"fmt"
	"net/http"
	"strings"
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

// ProcessUserMessage handles user messages including confirmations, modifications, and multi-employee disambiguations
func (m *ProjectManagementModule) ProcessUserMessage(ctx *erp_modules.ModuleContext, message string) (*erp_modules.ModuleResponse, error) {
	m.api.LogDebug("ProcessUserMessage called", "user_id", ctx.User.Id, "message", message)

	// Sanitize user input
	message = sanitizeUserInput(message)

	// Check for pending multi-employee disambiguation first - THIS IS CRITICAL
	multiDisambiguation, hasMultiDisambiguation := m.confirmationManager.GetPendingMultiEmployeeDisambiguation(ctx.User.Id)
	if hasMultiDisambiguation {
		m.api.LogDebug("Found pending multi-employee disambiguation", "user_id", ctx.User.Id)
		return m.handleMultiEmployeeDisambiguationResponse(ctx, message, multiDisambiguation)
	}

	// Check for pending confirmation
	pending, hasPending := m.confirmationManager.GetPendingConfirmation(ctx.User.Id)
	if hasPending {
		m.api.LogDebug("Found pending confirmation", "user_id", ctx.User.Id)
	} else {
		m.api.LogDebug("No pending confirmation found", "user_id", ctx.User.Id)
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

// handleMultiEmployeeDisambiguationResponse handles user response to multi-employee selection
func (m *ProjectManagementModule) handleMultiEmployeeDisambiguationResponse(ctx *erp_modules.ModuleContext, message string, disambiguation *MultiEmployeeDisambiguationConfirmation) (*erp_modules.ModuleResponse, error) {
	m.api.LogInfo("Handling multi-employee disambiguation response",
		"user_id", ctx.User.Id,
		"message", message,
		"unresolved_count", len(disambiguation.UnresolvedEmployeeMatches))

	// Parse disambiguation response using LLM
	disambiguationResponse, err := m.parseMultiEmployeeDisambiguationResponse(ctx, message, disambiguation)
	if err != nil {
		m.api.LogError("Failed to parse multi-employee disambiguation response", "error", err.Error())
		isVietnamese := detectUserLanguage(ctx.User)
		errorMsg := "⚠️ Không thể hiểu lựa chọn của bạn. Vui lòng chọn bằng số thứ tự (ví dụ: 1, 3, 5)."
		if !isVietnamese {
			errorMsg = "⚠️ Cannot understand your selection. Please choose by numbers (example: 1, 3, 5)."
		}
		return &erp_modules.ModuleResponse{
			Success: false,
			Message: errorMsg,
		}, nil
	}

	m.api.LogInfo("Parsed disambiguation response",
		"intent", disambiguationResponse.Intent,
		"selected_indexes", disambiguationResponse.SelectedIndexes)

	// Clear pending disambiguation IMMEDIATELY after parsing
	m.confirmationManager.ClearPendingConfirmation(ctx.User.Id)

	switch disambiguationResponse.Intent {
	case "index_selection":
		// Validate that we have selections
		if len(disambiguationResponse.SelectedIndexes) == 0 {
			isVietnamese := detectUserLanguage(ctx.User)
			errorMsg := "⚠️ Vui lòng chọn ít nhất một nhân viên."
			if !isVietnamese {
				errorMsg = "⚠️ Please select at least one employee."
			}
			return &erp_modules.ModuleResponse{
				Success: false,
				Message: errorMsg,
			}, nil
		}

		// Process the selections
		selectedEmployees, err := m.resolveDisambiguatedEmployees(
			disambiguation.UnresolvedEmployeeMatches,
			disambiguationResponse.SelectedIndexes,
		)
		if err != nil {
			isVietnamese := detectUserLanguage(ctx.User)
			errorMsg := "⚠️ Có lỗi xảy ra khi xử lý lựa chọn nhân viên: " + err.Error()
			if !isVietnamese {
				errorMsg = "⚠️ An error occurred while processing employee selection: " + err.Error()
			}
			return &erp_modules.ModuleResponse{
				Success: false,
				Message: errorMsg,
			}, nil
		}

		// Combine with previously resolved employees
		allResolvedEmployees := append(disambiguation.ResolvedEmployees, selectedEmployees...)

		// Update the data with all resolved employees
		disambiguation.Data["assigned_to_employees"] = allResolvedEmployees

		// Create and store the confirmation
		confirmation := &ProjectManagementConfirmation{
			UserID:       disambiguation.UserID,
			Type:         disambiguation.Type,
			Action:       disambiguation.Action,
			Data:         disambiguation.Data,
			CreatedAt:    time.Now().UnixMilli(),
			EmployeeID:   disambiguation.EmployeeID,
			CreatorEmail: disambiguation.CreatorEmail,
		}

		// Store the confirmation state
		m.confirmationManager.StorePendingConfirmation(ctx.User.Id, confirmation)

		// Generate success message with confirmation
		isVietnamese := detectUserLanguage(ctx.User)
		var selectedMsg strings.Builder
		if isVietnamese {
			selectedMsg.WriteString("✅ **Đã chọn nhân viên:**\n")
		} else {
			selectedMsg.WriteString("✅ **Selected employees:**\n")
		}
		for _, emp := range allResolvedEmployees {
			selectedMsg.WriteString(fmt.Sprintf("- **%s** (%s)\n", emp.EmployeeName, emp.Email))
		}
		selectedMsg.WriteString("\n")

		// Generate confirmation message
		confirmationMsg, err := m.generateConfirmationMessage(ctx, disambiguation.Type, disambiguation.Data)
		if err != nil {
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
			Message:     selectedMsg.String() + confirmationMsg,
			ActionTaken: "multi_employee_selected_proceed_confirmation",
			Data: map[string]interface{}{
				"awaiting_confirmation": true,
				"type":                  disambiguation.Type,
				"selected_employees":    len(allResolvedEmployees),
			},
		}, nil

	case "cancel":
		return m.handleCancelAction(ctx.User.Id)
	default:
		isVietnamese := detectUserLanguage(ctx.User)
		errorMsg := "⚠️ Vui lòng chọn bằng số thứ tự (ví dụ: 1, 3, 5) hoặc hủy bỏ (cancel)."
		if !isVietnamese {
			errorMsg = "⚠️ Please select by numbers (example: 1, 3, 5) or cancel."
		}
		return &erp_modules.ModuleResponse{
			Success: false,
			Message: errorMsg,
		}, nil
	}
}

// proceedWithConfirmationAfterMultiDisambiguation proceeds with confirmation after multi-employee selection
func (m *ProjectManagementModule) proceedWithConfirmationAfterMultiDisambiguation(ctx *erp_modules.ModuleContext, disambiguation *MultiEmployeeDisambiguationConfirmation, allResolvedEmployees []AssignedEmployee) (*erp_modules.ModuleResponse, error) {
	// Store regular confirmation with selected employees
	confirmation := &ProjectManagementConfirmation{
		UserID:       disambiguation.UserID,
		Type:         disambiguation.Type,
		Action:       disambiguation.Action,
		Data:         disambiguation.Data,
		CreatedAt:    time.Now().UnixMilli(),
		EmployeeID:   disambiguation.EmployeeID,
		CreatorEmail: disambiguation.CreatorEmail,
	}

	m.confirmationManager.StorePendingConfirmation(ctx.User.Id, confirmation)
	return nil, nil // This will be handled by the confirmation flow
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
	return "Quản lý dự án và công việc: tạo dự án mới, tạo task, phân công công việc cho nhiều nhân viên với khả năng gán nhân viên tự động và xử lý trường hợp trùng tên"
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
			"tạo dự án cho Minh, Nam và An",
			"create project assign to John, Mary and Peter",
			"tạo dự án phân công cho Nguyễn Văn An, Trần Thị Hoa",
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
			"tạo task cho Nam, An và Hoa",
			"create task assign to John, Mary and Peter",
			"tạo task dọn dẹp cho John, Tex và Lady",
			"create cleaning task assign to John, Tex, Lady",
		},
	}
}

// handleModifyAction handles user modification requests with multi-employee support
func (m *ProjectManagementModule) handleModifyAction(ctx *erp_modules.ModuleContext, pending *ProjectManagementConfirmation, modifications map[string]interface{}) (*erp_modules.ModuleResponse, error) {
	// Check if the modification involves assignee changes that might need disambiguation
	if assigneeNamesInterface, ok := modifications["assigned_to_names"].([]interface{}); ok {
		var assigneeNames []string
		for _, name := range assigneeNamesInterface {
			if nameStr, ok := name.(string); ok {
				assigneeNames = append(assigneeNames, nameStr)
			}
		}

		if len(assigneeNames) > 0 {
			// Try to resolve the assignees
			assigneeResult, err := m.resolveMultipleAssignees(assigneeNames)
			if err != nil {
				m.api.LogError("Failed to resolve modified assignees", "error", err.Error())
				modifications["assigned_to_employees"] = []AssignedEmployee{}
			} else if assigneeResult.RequiresDisambiguation {
				// Store modification disambiguation state
				multiDisambiguation := &MultiEmployeeDisambiguationConfirmation{
					UserID:                    ctx.User.Id,
					Type:                      pending.Type,
					Action:                    pending.Action,
					Data:                      pending.Data,
					CreatedAt:                 time.Now().UnixMilli(),
					EmployeeID:                pending.EmployeeID,
					CreatorEmail:              pending.CreatorEmail,
					UnresolvedEmployeeMatches: assigneeResult.UnresolvedEmployeeMatches,
					ResolvedEmployees:         assigneeResult.ResolvedEmployees,
					IsModification:            true,
					OriginalConfirmation:      pending,
					PendingModifications:      modifications,
				}

				m.confirmationManager.StorePendingMultiEmployeeDisambiguation(ctx.User.Id, multiDisambiguation)

				// Generate disambiguation message
				disambiguationMsg, err := m.generateMultiEmployeeDisambiguationMessage(ctx, assigneeResult)
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
					ActionTaken: "request_modification_multi_employee_disambiguation",
					Data: map[string]interface{}{
						"awaiting_disambiguation": true,
						"type":                    "modification",
						"unresolved_count":        len(assigneeResult.UnresolvedEmployeeMatches),
						"resolved_count":          len(assigneeResult.ResolvedEmployees),
					},
				}, nil
			} else {
				// All employees resolved successfully
				modifications["assigned_to_employees"] = assigneeResult.ResolvedEmployees
			}
		} else {
			// Empty assignment
			modifications["assigned_to_employees"] = []AssignedEmployee{}
		}
	} else {
		// Handle other assignee modifications using existing logic
		m.handleAssigneesModification(modifications)
	}

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
