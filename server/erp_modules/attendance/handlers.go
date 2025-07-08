// Copyright (c) 2023-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package attendance

import (
	"fmt"
	"sync"
	"time"

	"github.com/mattermost/mattermost-plugin-ai/server/erp_modules"
	"github.com/mattermost/mattermost/server/public/model"
)

// AttendanceConfirmation represents pending attendance confirmation
type AttendanceConfirmation struct {
	UserID     string `json:"user_id"`
	Action     string `json:"action"` // "check_in", "check_out", "absent"
	Reason     string `json:"reason,omitempty"`
	CreatedAt  int64  `json:"created_at"`
	EmployeeID string `json:"employee_id"`
}

// ConfirmationManager handles confirmation state
type ConfirmationManager struct {
	confirmationState map[string]*AttendanceConfirmation
	confirmationMutex sync.RWMutex
}

// NewConfirmationManager creates a new confirmation manager
func NewConfirmationManager() *ConfirmationManager {
	return &ConfirmationManager{
		confirmationState: make(map[string]*AttendanceConfirmation),
	}
}

// StorePendingConfirmation stores a pending confirmation
func (cm *ConfirmationManager) StorePendingConfirmation(userID string, confirmation *AttendanceConfirmation) {
	cm.confirmationMutex.Lock()
	defer cm.confirmationMutex.Unlock()
	cm.confirmationState[userID] = confirmation
}

// GetPendingConfirmation retrieves a pending confirmation
func (cm *ConfirmationManager) GetPendingConfirmation(userID string) (*AttendanceConfirmation, bool) {
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

// handleCheckInRequest processes check-in request with optional confirmation
func (m *AttendanceModule) handleCheckInRequest(employeeID string, ctx *erp_modules.ModuleContext, intent *erp_modules.Intent) (*erp_modules.ModuleResponse, error) {
	// For check-in, we can execute directly with high confidence
	if intent.Confidence >= 0.9 {
		return m.handleCheckIn(employeeID, getUserDisplayName(ctx.User), ctx.User.Id)
	}

	// Request confirmation for medium confidence
	return m.requestConfirmation(ctx, "check_in", "", employeeID)
}

// handleCheckOutRequest processes check-out request with optional confirmation
func (m *AttendanceModule) handleCheckOutRequest(employeeID string, ctx *erp_modules.ModuleContext, intent *erp_modules.Intent) (*erp_modules.ModuleResponse, error) {
	// For check-out, we can execute directly with high confidence
	if intent.Confidence >= 0.9 {
		return m.handleCheckOut(employeeID, getUserDisplayName(ctx.User), ctx.User.Id)
	}

	// Request confirmation for medium confidence
	return m.requestConfirmation(ctx, "check_out", "", employeeID)
}

// handleAbsentRequest processes absent request with confirmation
func (m *AttendanceModule) handleAbsentRequest(employeeID string, ctx *erp_modules.ModuleContext, intent *erp_modules.Intent, reason string) (*erp_modules.ModuleResponse, error) {
	reason, err := m.analyzeAbsentReason(ctx, intent.RawMessage)
	if err != nil {
		// Log the error but continue with fallback
		m.api.LogWarn("Failed to extract absence reason using LLM, using fallback", "error", err.Error())
		isVietnamese := detectUserLanguage(ctx.User)
		if isVietnamese {
			reason = "Việc cá nhân"
		} else {
			reason = "Personal matters"
		}
	}

	// Always request confirmation for absence reporting as it's important
	return m.requestConfirmation(ctx, "absent", reason, employeeID)
}

// requestConfirmation requests confirmation from user
func (m *AttendanceModule) requestConfirmation(ctx *erp_modules.ModuleContext, action, reason, employeeID string) (*erp_modules.ModuleResponse, error) {
	// Store pending confirmation
	m.confirmationManager.StorePendingConfirmation(ctx.User.Id, &AttendanceConfirmation{
		UserID:     ctx.User.Id,
		Action:     action,
		Reason:     reason,
		CreatedAt:  time.Now().UnixMilli(),
		EmployeeID: employeeID,
	})

	// Generate confirmation message
	confirmationMsg, err := m.generateConfirmationMessage(ctx, action, reason)
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
			"action":                action,
		},
	}, nil
}

// executeConfirmedAction executes the confirmed action
func (m *AttendanceModule) executeConfirmedAction(ctx *erp_modules.ModuleContext, pending *AttendanceConfirmation) (*erp_modules.ModuleResponse, error) {
	employeeName := getUserDisplayName(ctx.User)

	switch pending.Action {
	case "check_in":
		return m.handleCheckIn(pending.EmployeeID, employeeName, ctx.User.Id)
	case "check_out":
		return m.handleCheckOut(pending.EmployeeID, employeeName, ctx.User.Id)
	case "absent":
		return m.handleAbsent(pending.EmployeeID, employeeName, ctx.User.Id, pending.Reason)
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

// handleCheckIn processes check-in request
func (m *AttendanceModule) handleCheckIn(employeeID, employeeName, userID string) (*erp_modules.ModuleResponse, error) {
	formattedTime, err := m.erpClient.RecordEmployeeCheckin(employeeID)
	if err != nil {
		user, _ := m.api.GetUser(userID)
		isVietnamese := detectUserLanguage(user)
		errorMsg := "⚠️ Có lỗi xảy ra khi ghi nhận check-in. Vui lòng thử lại."
		if !isVietnamese {
			errorMsg = "⚠️ An error occurred while recording check-in. Please try again."
		}
		return &erp_modules.ModuleResponse{
			Success: false,
			Message: errorMsg,
			Error:   err.Error(),
		}, nil
	}

	// Send notification asynchronously
	go func() {
		_ = m.notificationManager.SendNotification(userID, employeeName, RollCallEventCheckIn, formattedTime, "")
	}()

	user, _ := m.api.GetUser(userID)
	isVietnamese := detectUserLanguage(user)
	successMsg := fmt.Sprintf("Đã ghi nhận check-in của bạn lúc **%s**!", formattedTime)
	if !isVietnamese {
		successMsg = fmt.Sprintf("Successfully recorded your check-in at **%s**!", formattedTime)
	}

	return &erp_modules.ModuleResponse{
		Success:     true,
		Message:     successMsg,
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
		user, _ := m.api.GetUser(userID)
		isVietnamese := detectUserLanguage(user)
		errorMsg := "⚠️ Có lỗi xảy ra khi ghi nhận check-out. Vui lòng thử lại."
		if !isVietnamese {
			errorMsg = "⚠️ An error occurred while recording check-out. Please try again."
		}
		return &erp_modules.ModuleResponse{
			Success: false,
			Message: errorMsg,
			Error:   err.Error(),
		}, nil
	}

	// Send notification asynchronously
	go func() {
		_ = m.notificationManager.SendNotification(userID, employeeName, RollCallEventCheckOut, formattedTime, "")
	}()

	user, _ := m.api.GetUser(userID)
	isVietnamese := detectUserLanguage(user)
	successMsg := fmt.Sprintf("Đã ghi nhận check-out của bạn lúc **%s**!", formattedTime)
	if !isVietnamese {
		successMsg = fmt.Sprintf("Successfully recorded your check-out at **%s**!", formattedTime)
	}

	return &erp_modules.ModuleResponse{
		Success:     true,
		Message:     successMsg,
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
		user, _ := m.api.GetUser(userID)
		isVietnamese := detectUserLanguage(user)
		errorMsg := "⚠️ Có lỗi xảy ra khi ghi nhận nghỉ phép. Vui lòng thử lại."
		if !isVietnamese {
			errorMsg = "⚠️ An error occurred while recording absence. Please try again."
		}
		return &erp_modules.ModuleResponse{
			Success: false,
			Message: errorMsg,
			Error:   err.Error(),
		}, nil
	}

	// Send notification asynchronously
	go func() {
		_ = m.notificationManager.SendNotification(userID, employeeName, RollCallEventAbsent, recordedDate, reason)
	}()

	user, _ := m.api.GetUser(userID)
	isVietnamese := detectUserLanguage(user)
	successMsg := fmt.Sprintf("Đã ghi nhận nghỉ phép của bạn cho ngày **%s** với lý do: \"%s\"", recordedDate, reason)
	if !isVietnamese {
		successMsg = fmt.Sprintf("Successfully recorded your absence for **%s** with reason: \"%s\"", recordedDate, reason)
	}

	return &erp_modules.ModuleResponse{
		Success:     true,
		Message:     successMsg,
		ActionTaken: "absent",
		Data: map[string]interface{}{
			"date":     recordedDate,
			"reason":   reason,
			"employee": employeeName,
		},
	}, nil
}

// getEmployeeIDFromUser gets employee ID from user
func (m *AttendanceModule) getEmployeeIDFromUser(user *model.User) (string, error) {
	// Use the user's ID as the chat ID to lookup in ERPNext
	chatID := user.Id

	employeeID, err := m.erpClient.GetEmployeeByChatID(chatID)
	if err != nil {
		return "", fmt.Errorf("failed to get employee by chat ID %s: %w", chatID, err)
	}

	return employeeID, nil
}
