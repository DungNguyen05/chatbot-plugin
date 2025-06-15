// Copyright (c) 2023-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package attendance

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/mattermost/mattermost-plugin-ai/server/erp_modules"
	"github.com/mattermost/mattermost-plugin-ai/server/llm"
	"github.com/mattermost/mattermost/server/public/model"
)

// AttendanceModule handles attendance-related operations
type AttendanceModule struct {
	config              AttendanceConfig
	erpClient           *ERPClient
	notificationManager *NotificationManager
	api                 PluginAPI
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
	// Create ERP client
	erpClient := NewERPClient(config, httpClient, api)

	// Create notification manager
	notificationManager := NewNotificationManager(config, i18n, prompts, getLLM, api, botUserID)

	return &AttendanceModule{
		config:              config,
		erpClient:           erpClient,
		notificationManager: notificationManager,
		api:                 api,
	}
}

// GetCategory returns the category this module handles
func (m *AttendanceModule) GetCategory() string {
	return "attendance"
}

// GetSupportedActions returns list of actions this module supports
func (m *AttendanceModule) GetSupportedActions() []string {
	return []string{"check_in", "check_out", "absent"}
}

// CanHandle determines if this module can handle the given intent
func (m *AttendanceModule) CanHandle(intent *erp_modules.Intent) bool {
	if intent.Category != "attendance" {
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

// Execute processes the attendance intent
func (m *AttendanceModule) Execute(ctx *erp_modules.ModuleContext, intent *erp_modules.Intent) (*erp_modules.ModuleResponse, error) {
	// Get employee ID
	employeeID, err := m.GetEmployeeIDFromUser(ctx.User)
	if err != nil {
		return &erp_modules.ModuleResponse{
			Success: false,
			Message: "❌ Không tìm thấy thông tin nhân viên của bạn trong hệ thống ERP. Vui lòng liên hệ quản trị viên.",
			Error:   err.Error(),
		}, nil
	}

	// Get employee name for notifications
	employeeName := ctx.User.Username
	if ctx.User.FirstName != "" || ctx.User.LastName != "" {
		employeeName = strings.TrimSpace(ctx.User.FirstName + " " + ctx.User.LastName)
	}

	// Execute specific action
	switch intent.Action {
	case "check_in":
		return m.handleCheckIn(employeeID, employeeName, ctx.User.Id)
	case "check_out":
		return m.handleCheckOut(employeeID, employeeName, ctx.User.Id)
	case "absent":
		reason := intent.Parameters["reason"]
		if reason == "" {
			reason = "Không có lý do cụ thể"
		}
		return m.handleAbsent(employeeID, employeeName, ctx.User.Id, reason)
	default:
		return &erp_modules.ModuleResponse{
			Success: false,
			Message: "Hành động không được hỗ trợ",
			Error:   fmt.Sprintf("unsupported action: %s", intent.Action),
		}, nil
	}
}

// GetDescription returns a description of what this module does
func (m *AttendanceModule) GetDescription() string {
	return "Quản lý điểm danh nhân viên: check-in, check-out, và báo nghỉ phép"
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
	}
}

// handleCheckIn processes check-in request
func (m *AttendanceModule) handleCheckIn(employeeID, employeeName, userID string) (*erp_modules.ModuleResponse, error) {
	formattedTime, err := m.erpClient.RecordEmployeeCheckin(employeeID)
	if err != nil {
		return &erp_modules.ModuleResponse{
			Success: false,
			Message: "⚠️ Có lỗi xảy ra khi ghi nhận check-in. Vui lòng thử lại.",
			Error:   err.Error(),
		}, nil
	}

	// Send notification asynchronously
	go func() {
		_ = m.notificationManager.SendNotification(userID, employeeName, RollCallEventCheckIn, formattedTime, "")
	}()

	return &erp_modules.ModuleResponse{
		Success:     true,
		Message:     fmt.Sprintf("✅ Đã ghi nhận check-in của bạn lúc **%s**!", formattedTime),
		ActionTaken: "check_in",
		Data: map[string]interface{}{
			"time":     formattedTime,
			"employee": employeeName,
		},
	}, nil
}

// handleCheckOut processes check-out request
func (m *AttendanceModule) handleCheckOut(employeeID, employeeName, userID string) (*erp_modules.ModuleResponse, error) {
	formattedTime, err := m.erpClient.RecordEmployeeCheckout(employeeID)
	if err != nil {
		return &erp_modules.ModuleResponse{
			Success: false,
			Message: "⚠️ Có lỗi xảy ra khi ghi nhận check-out. Vui lòng thử lại.",
			Error:   err.Error(),
		}, nil
	}

	// Send notification asynchronously
	go func() {
		_ = m.notificationManager.SendNotification(userID, employeeName, RollCallEventCheckOut, formattedTime, "")
	}()

	return &erp_modules.ModuleResponse{
		Success:     true,
		Message:     fmt.Sprintf("✅ Đã ghi nhận check-out của bạn lúc **%s**!", formattedTime),
		ActionTaken: "check_out",
		Data: map[string]interface{}{
			"time":     formattedTime,
			"employee": employeeName,
		},
	}, nil
}

// handleAbsent processes absent request
func (m *AttendanceModule) handleAbsent(employeeID, employeeName, userID, reason string) (*erp_modules.ModuleResponse, error) {
	recordedDate, err := m.erpClient.RecordEmployeeAbsent(employeeID, reason)
	if err != nil {
		return &erp_modules.ModuleResponse{
			Success: false,
			Message: "⚠️ Có lỗi xảy ra khi ghi nhận nghỉ phép. Vui lòng thử lại.",
			Error:   err.Error(),
		}, nil
	}

	// Send notification asynchronously
	go func() {
		_ = m.notificationManager.SendNotification(userID, employeeName, RollCallEventAbsent, recordedDate, reason)
	}()

	return &erp_modules.ModuleResponse{
		Success:     true,
		Message:     fmt.Sprintf("📝 Đã ghi nhận nghỉ phép của bạn cho ngày **%s** với lý do: \"%s\"", recordedDate, reason),
		ActionTaken: "absent",
		Data: map[string]interface{}{
			"date":     recordedDate,
			"reason":   reason,
			"employee": employeeName,
		},
	}, nil
}

// GetEmployeeIDFromUser gets employee ID from user
func (m *AttendanceModule) GetEmployeeIDFromUser(user *model.User) (string, error) {
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
func (m *AttendanceModule) SendNotification(userID, employeeName string, eventType RollCallEventType, eventTime string, reason string) error {
	return m.notificationManager.SendNotification(userID, employeeName, eventType, eventTime, reason)
}
