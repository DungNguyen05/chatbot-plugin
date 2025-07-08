// Copyright (c) 2023-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package attendance

import (
	"fmt"
	"net/http"

	"github.com/mattermost/mattermost-plugin-ai/server/erp_modules"
	"github.com/mattermost/mattermost-plugin-ai/server/llm"
)

// AttendanceModule handles attendance-related operations
type AttendanceModule struct {
	config              AttendanceConfig
	erpClient           *ERPClient
	notificationManager *NotificationManager
	confirmationManager *ConfirmationManager
	api                 PluginAPI
	prompts             PromptsInterface
	getLLM              func() llm.LanguageModel
}

// NewAttendanceModule creates a new attendance module
func NewAttendanceModule(
	config AttendanceConfig,
	httpClient *http.Client,
	i18n I18nBundle,
	prompts PromptsInterface,
	getLLM func() llm.LanguageModel,
	notificationFunc func(userID, employeeName string, eventType RollCallEventType, eventTime string, reason string) error,
	api PluginAPI,
	botUserID string,
) *AttendanceModule {
	// Validate configuration
	if err := ValidateAttendanceConfig(config); err != nil {
		api.LogError("Invalid attendance configuration", "error", err.Error())
		// Continue with disabled module
		config.Enabled = false
	}

	// Create ERP client
	erpClient := NewERPClient(config, httpClient, api)

	// Create notification manager
	notificationManager := NewNotificationManager(config, i18n, prompts, getLLM, api, botUserID)

	// Create confirmation manager
	confirmationManager := NewConfirmationManager()

	return &AttendanceModule{
		config:              config,
		erpClient:           erpClient,
		notificationManager: notificationManager,
		confirmationManager: confirmationManager,
		api:                 api,
		prompts:             prompts,
		getLLM:              getLLM,
	}
}

// GetCategory returns the category this module handles
func (m *AttendanceModule) GetCategory() string {
	return "attendance"
}

// GetSupportedActions returns list of actions this module supports
func (m *AttendanceModule) GetSupportedActions() []string {
	return []string{"check_in", "check_out", "absent", "get_attendance_report"}
}

// CanHandle determines if this module can handle the given intent
func (m *AttendanceModule) CanHandle(intent *erp_modules.Intent) bool {
	if !m.config.Enabled {
		return false
	}

	if intent.Category != "attendance" {
		return false
	}

	return isValidAction(intent.Action)
}

// ProcessUserMessage handles user messages including confirmations
func (m *AttendanceModule) ProcessUserMessage(ctx *erp_modules.ModuleContext, message string) (*erp_modules.ModuleResponse, error) {
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

// Execute processes the attendance intent
func (m *AttendanceModule) Execute(ctx *erp_modules.ModuleContext, intent *erp_modules.Intent) (*erp_modules.ModuleResponse, error) {
	if !m.config.Enabled {
		isVietnamese := detectUserLanguage(ctx.User)
		errorMsg := "Tính năng chấm công hiện không khả dụng."
		if !isVietnamese {
			errorMsg = "Attendance feature is currently not available."
		}
		return &erp_modules.ModuleResponse{
			Success: false,
			Message: errorMsg,
		}, nil
	}

	isVietnamese := detectUserLanguage(ctx.User)

	// Execute specific action based on intent.Action
	switch intent.Action {
	case "check_in":
		// Get employee ID for check-in actions
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
		return m.handleCheckInRequest(employeeID, ctx, intent)

	case "check_out":
		// Get employee ID for check-out actions
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
		return m.handleCheckOutRequest(employeeID, ctx, intent)

	case "absent":
		// Get employee ID for absence reporting
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
		return m.handleAbsentRequest(employeeID, ctx, intent, "")

	case "get_attendance_report":
		// Handle unified attendance report (for self, by names, or all employees)
		return m.handleAttendanceReportRequest(ctx, intent)

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
func (m *AttendanceModule) GetDescription() string {
	return "Quản lý điểm danh nhân viên: check-in, check-out, báo nghỉ phép, và truy vấn báo cáo chấm công"
}

// GetActionExamples returns examples of user messages for each action
func (m *AttendanceModule) GetActionExamples() map[string][]string {
	return map[string][]string{
		"check_in": {
			"tôi đã có mặt",
			"tôi đã đến công ty",
			"check in",
			"điểm danh vào",
			"tôi đã tới",
			"có mặt rồi",
			"đã đến rồi",
			"tôi đã ở công ty",
		},
		"check_out": {
			"tôi về rồi",
			"tôi đã ra về",
			"check out",
			"điểm danh ra",
			"tôi đi về",
			"kết thúc làm việc",
			"tôi ra về",
			"về nhà",
		},
		"absent": {
			"tôi hôm nay nghỉ",
			"tôi không đi làm được",
			"nghỉ phép",
			"tôi ốm",
			"nghỉ ốm",
			"không thể đến công ty",
			"tôi nghỉ",
			"hôm nay tôi không làm",
		},
		"get_attendance_report": {
			// Self queries
			"tôi nghỉ bao nhiêu ngày",
			"how many days was I absent",
			"báo cáo chấm công của tôi",
			"số ngày nghỉ tháng này",
			"attendance report this month",
			"tôi đi làm mấy ngày tuần trước",
			"show my present days",
			"xem số ngày có mặt",
			// Others queries
			"báo cáo chấm công của Minh",
			"attendance report for Minh",
			"xem chấm công của Minh và An",
			"báo cáo điểm danh team",
			"attendance của tất cả nhân viên",
			"báo cáo chấm công toàn bộ",
			"xem chấm công Nguyễn Văn An",
			"attendance report all employees",
			"chấm công của nhân viên",
			"điểm danh báo cáo",
		},
	}
}

// SendNotification is a legacy method that delegates to the notification manager
// This method is kept for backward compatibility
func (m *AttendanceModule) SendNotification(userID, employeeName string, eventType RollCallEventType, eventTime string, reason string) error {
	return m.notificationManager.SendNotification(userID, employeeName, eventType, eventTime, reason)
}
