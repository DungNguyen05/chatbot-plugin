// Copyright (c) 2023-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package attendance

import (
	"fmt"
	"strings"

	"github.com/mattermost/mattermost-plugin-ai/server/erp_modules"
)

// AttendanceModule handles attendance-related operations
type AttendanceModule struct {
	erpIntegration ERPIntegration
}

// ERPIntegration interface for ERP operations (to inject existing implementation)
type ERPIntegration interface {
	GetEmployeeIDFromUser(user interface{}) (string, error)
	RecordEmployeeCheckin(employeeID string) (string, error)
	RecordEmployeeCheckout(employeeID string) (string, error)
	RecordEmployeeAbsent(employeeID string, reason string) (string, error)
	SendRollCallNotification(userID, employeeName string, eventType string, eventTime string, reason string) error
}

// NewAttendanceModule creates a new attendance module
func NewAttendanceModule(erpIntegration ERPIntegration) *AttendanceModule {
	return &AttendanceModule{
		erpIntegration: erpIntegration,
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
	employeeID, err := m.erpIntegration.GetEmployeeIDFromUser(ctx.User)
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
		},
		"check_out": {
			"tôi về rồi",
			"tôi đã ra về",
			"check out",
			"điểm danh ra",
			"tôi đi về",
			"kết thúc làm việc",
		},
		"absent": {
			"tôi hôm nay nghỉ",
			"tôi không đi làm được",
			"nghỉ phép",
			"tôi ốm",
			"nghỉ ốm",
			"không thể đến công ty",
		},
	}
}

// handleCheckIn processes check-in request
func (m *AttendanceModule) handleCheckIn(employeeID, employeeName, userID string) (*erp_modules.ModuleResponse, error) {
	formattedTime, err := m.erpIntegration.RecordEmployeeCheckin(employeeID)
	if err != nil {
		return &erp_modules.ModuleResponse{
			Success: false,
			Message: "⚠️ Có lỗi xảy ra khi ghi nhận check-in. Vui lòng thử lại.",
			Error:   err.Error(),
		}, nil
	}

	// Send notification asynchronously
	go func() {
		_ = m.erpIntegration.SendRollCallNotification(userID, employeeName, "check_in", formattedTime, "")
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
	formattedTime, err := m.erpIntegration.RecordEmployeeCheckout(employeeID)
	if err != nil {
		return &erp_modules.ModuleResponse{
			Success: false,
			Message: "⚠️ Có lỗi xảy ra khi ghi nhận check-out. Vui lòng thử lại.",
			Error:   err.Error(),
		}, nil
	}

	// Send notification asynchronously
	go func() {
		_ = m.erpIntegration.SendRollCallNotification(userID, employeeName, "check_out", formattedTime, "")
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
	recordedDate, err := m.erpIntegration.RecordEmployeeAbsent(employeeID, reason)
	if err != nil {
		return &erp_modules.ModuleResponse{
			Success: false,
			Message: "⚠️ Có lỗi xảy ra khi ghi nhận nghỉ phép. Vui lòng thử lại.",
			Error:   err.Error(),
		}, nil
	}

	// Send notification asynchronously
	go func() {
		_ = m.erpIntegration.SendRollCallNotification(userID, employeeName, "absent", recordedDate, reason)
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
