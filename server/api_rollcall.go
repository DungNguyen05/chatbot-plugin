// Copyright (c) 2023-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package main

import (
	"context"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/mattermost/mattermost-plugin-ai/server/erp_modules"
	"github.com/mattermost/mattermost-plugin-ai/server/erp_modules/attendance"
	"github.com/mattermost/mattermost/server/public/model"
)

// AbsentRequest represents the request payload for marking absent
type AbsentRequest struct {
	Reason string `json:"reason" binding:"required"`
}

// CheckInResponse represents the response for check-in/out operations
type CheckInResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Time    string `json:"time"`
	Error   string `json:"error,omitempty"` // Add error field
}

// handleAPICheckIn handles the API endpoint for check-in
func (p *Plugin) handleAPICheckIn(c *gin.Context) {
	userID := c.GetHeader("Mattermost-User-Id")

	// Get user info
	user, err := p.pluginAPI.User.Get(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, CheckInResponse{
			Success: false,
			Message: "Error getting user information. Please try again.",
			Error:   err.Error(),
		})
		return
	}

	// Process through ERP module with high confidence (bypass confirmation)
	response := p.processAttendanceRequestDirect(user, "check_in", "")

	// Map response to appropriate HTTP status
	statusCode := http.StatusOK
	if !response.Success {
		// Determine appropriate status code based on error type
		if strings.Contains(strings.ToLower(response.Error), "already") ||
			strings.Contains(strings.ToLower(response.Message), "already") {
			statusCode = http.StatusConflict // 409 for duplicate operations
		} else if strings.Contains(strings.ToLower(response.Error), "not found") ||
			strings.Contains(strings.ToLower(response.Message), "not found") {
			statusCode = http.StatusNotFound // 404 for not found
		} else {
			statusCode = http.StatusBadRequest // 400 for other client errors
		}
	}

	c.JSON(statusCode, CheckInResponse{
		Success: response.Success,
		Message: response.Message,
		Time:    getTimeFromData(response.Data),
		Error:   response.Error,
	})
}

// handleAPICheckOut handles the API endpoint for check-out
func (p *Plugin) handleAPICheckOut(c *gin.Context) {
	userID := c.GetHeader("Mattermost-User-Id")

	// Get user info
	user, err := p.pluginAPI.User.Get(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, CheckInResponse{
			Success: false,
			Message: "Error getting user information. Please try again.",
			Error:   err.Error(),
		})
		return
	}

	// Process through ERP module with high confidence (bypass confirmation)
	response := p.processAttendanceRequestDirect(user, "check_out", "")

	// Map response to appropriate HTTP status
	statusCode := http.StatusOK
	if !response.Success {
		// Determine appropriate status code based on error type
		if strings.Contains(strings.ToLower(response.Error), "already") ||
			strings.Contains(strings.ToLower(response.Message), "already") {
			statusCode = http.StatusConflict // 409 for duplicate operations
		} else if strings.Contains(strings.ToLower(response.Error), "not found") ||
			strings.Contains(strings.ToLower(response.Message), "not found") {
			statusCode = http.StatusNotFound // 404 for not found
		} else {
			statusCode = http.StatusBadRequest // 400 for other client errors
		}
	}

	c.JSON(statusCode, CheckInResponse{
		Success: response.Success,
		Message: response.Message,
		Time:    getTimeFromData(response.Data),
		Error:   response.Error,
	})
}

// handleAPIAbsent handles the API endpoint for marking absent
func (p *Plugin) handleAPIAbsent(c *gin.Context) {
	userID := c.GetHeader("Mattermost-User-Id")

	var req AbsentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, CheckInResponse{
			Success: false,
			Message: "Please provide a reason for your absence.",
			Error:   "Invalid request format",
		})
		return
	}

	// Get user info
	user, err := p.pluginAPI.User.Get(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, CheckInResponse{
			Success: false,
			Message: "Error getting user information. Please try again.",
			Error:   err.Error(),
		})
		return
	}

	reason := strings.TrimSpace(req.Reason)
	if reason == "" {
		c.JSON(http.StatusBadRequest, CheckInResponse{
			Success: false,
			Message: "Please provide a reason for your absence.",
			Error:   "Empty reason not allowed",
		})
		return
	}

	// Process through ERP module directly (bypass confirmation)
	response := p.processAttendanceRequestDirect(user, "absent", reason)

	// Map response to appropriate HTTP status
	statusCode := http.StatusOK
	if !response.Success {
		// Determine appropriate status code based on error type
		if strings.Contains(strings.ToLower(response.Error), "already") ||
			strings.Contains(strings.ToLower(response.Message), "already") ||
			strings.Contains(strings.ToLower(response.Error), "duplicate") ||
			strings.Contains(strings.ToLower(response.Message), "duplicate") {
			statusCode = http.StatusConflict // 409 for duplicate absence requests
		} else if strings.Contains(strings.ToLower(response.Error), "not found") ||
			strings.Contains(strings.ToLower(response.Message), "not found") {
			statusCode = http.StatusNotFound // 404 for not found
		} else if strings.Contains(strings.ToLower(response.Error), "unauthorized") ||
			strings.Contains(strings.ToLower(response.Message), "unauthorized") {
			statusCode = http.StatusUnauthorized // 401 for auth issues
		} else {
			statusCode = http.StatusBadRequest // 400 for other client errors
		}
	}

	c.JSON(statusCode, CheckInResponse{
		Success: response.Success,
		Message: response.Message,
		Time:    getTimeFromData(response.Data),
		Error:   response.Error,
	})
}

// processAttendanceRequestDirect processes attendance requests directly without confirmation
// This is used for API button clicks that should bypass confirmation
func (p *Plugin) processAttendanceRequestDirect(user *model.User, action, reason string) *erp_modules.ModuleResponse {
	if p.moduleRegistry == nil {
		return &erp_modules.ModuleResponse{
			Success: false,
			Message: "ERP module registry not initialized",
			Error:   "Module registry not available",
		}
	}

	// Get attendance module
	module, exists := p.moduleRegistry.GetModule("attendance")
	if !exists {
		return &erp_modules.ModuleResponse{
			Success: false,
			Message: "Attendance module not found",
			Error:   "Attendance module not registered",
		}
	}

	attendanceModule, ok := module.(*attendance.AttendanceModule)
	if !ok {
		return &erp_modules.ModuleResponse{
			Success: false,
			Message: "Invalid attendance module type",
			Error:   "Module type assertion failed",
		}
	}

	// Get employee ID for the user
	employeeID, err := p.getEmployeeIDFromUser(attendanceModule, user)
	if err != nil {
		isVietnamese := strings.HasPrefix(user.Locale, "vi")
		errorMsg := "Không tìm thấy thông tin nhân viên của bạn trong hệ thống ERP. Vui lòng liên hệ quản trị viên."
		if !isVietnamese {
			errorMsg = "Cannot find your employee information in the ERP system. Please contact administrator."
		}
		return &erp_modules.ModuleResponse{
			Success: false,
			Message: errorMsg,
			Error:   err.Error(),
		}
	}

	// Call the appropriate handler directly based on action
	switch action {
	case "check_in":
		return p.handleCheckInDirect(attendanceModule, employeeID, user)
	case "check_out":
		return p.handleCheckOutDirect(attendanceModule, employeeID, user)
	case "absent":
		return p.handleAbsentDirect(attendanceModule, employeeID, user, reason)
	default:
		return &erp_modules.ModuleResponse{
			Success: false,
			Message: "Unknown action: " + action,
			Error:   "Invalid action parameter",
		}
	}
}

// handleCheckInDirect directly processes check-in without confirmation
func (p *Plugin) handleCheckInDirect(attendanceModule *attendance.AttendanceModule, employeeID string, user *model.User) *erp_modules.ModuleResponse {
	ctx := &erp_modules.ModuleContext{
		Context: context.Background(),
		User:    user,
	}

	intent := &erp_modules.Intent{
		Category:   "attendance",
		Action:     "check_in",
		Confidence: 1.0, // Maximum confidence to bypass confirmation
		RawMessage: "check_in",
	}

	return p.executeWithMaxConfidence(attendanceModule, ctx, intent)
}

// handleCheckOutDirect directly processes check-out without confirmation
func (p *Plugin) handleCheckOutDirect(attendanceModule *attendance.AttendanceModule, employeeID string, user *model.User) *erp_modules.ModuleResponse {
	ctx := &erp_modules.ModuleContext{
		Context: context.Background(),
		User:    user,
	}

	intent := &erp_modules.Intent{
		Category:   "attendance",
		Action:     "check_out",
		Confidence: 1.0, // Maximum confidence to bypass confirmation
		RawMessage: "check_out",
	}

	return p.executeWithMaxConfidence(attendanceModule, ctx, intent)
}

// handleAbsentDirect directly processes absent without confirmation
func (p *Plugin) handleAbsentDirect(attendanceModule *attendance.AttendanceModule, employeeID string, user *model.User, reason string) *erp_modules.ModuleResponse {
	ctx := &erp_modules.ModuleContext{
		Context: context.Background(),
		User:    user,
		// Create a dummy post to carry the reason
		OriginalPost: &model.Post{
			Message: "absent " + reason,
			UserId:  user.Id,
		},
	}

	intent := &erp_modules.Intent{
		Category:   "attendance",
		Action:     "absent",
		Confidence: 1.0, // Maximum confidence to bypass confirmation
		RawMessage: "absent " + reason,
	}

	return p.executeWithMaxConfidence(attendanceModule, ctx, intent)
}

// executeWithMaxConfidence executes the attendance module with maximum confidence
func (p *Plugin) executeWithMaxConfidence(attendanceModule *attendance.AttendanceModule, ctx *erp_modules.ModuleContext, intent *erp_modules.Intent) *erp_modules.ModuleResponse {
	// Set confidence to maximum to bypass confirmation
	intent.Confidence = 1.0

	response, err := attendanceModule.Execute(ctx, intent)
	if err != nil {
		// Log the actual error for debugging
		p.API.LogError("Attendance module execution failed",
			"action", intent.Action,
			"error", err.Error(),
			"user_id", ctx.User.Id)

		return &erp_modules.ModuleResponse{
			Success: false,
			Message: "Failed to process attendance request",
			Error:   err.Error(),
		}
	}

	if response == nil {
		return &erp_modules.ModuleResponse{
			Success: false,
			Message: "No response from attendance module",
			Error:   "Nil response from module",
		}
	}

	// If still asking for confirmation, log this issue but return the response
	if response.ActionTaken == "request_confirmation" {
		p.API.LogWarn("Module still requesting confirmation despite max confidence",
			"action", intent.Action,
			"response_message", response.Message)
	}

	// Log the response for debugging
	p.API.LogInfo("Attendance module response",
		"action", intent.Action,
		"success", response.Success,
		"message", response.Message,
		"error", response.Error,
		"action_taken", response.ActionTaken)

	return response
}

// getEmployeeIDFromUser gets employee ID from user using the attendance module
func (p *Plugin) getEmployeeIDFromUser(attendanceModule *attendance.AttendanceModule, user *model.User) (string, error) {
	// Use user ID as employee lookup
	return user.Id, nil
}

// processAttendanceRequest processes attendance requests through the ERP module (legacy method)
func (p *Plugin) processAttendanceRequest(user *model.User, action, reason string) *erp_modules.ModuleResponse {
	if p.moduleManager == nil {
		return &erp_modules.ModuleResponse{
			Success: false,
			Message: "ERP modules not initialized",
			Error:   "Module manager not available",
		}
	}

	if p.moduleRegistry == nil {
		return &erp_modules.ModuleResponse{
			Success: false,
			Message: "ERP module registry not initialized",
			Error:   "Module registry not available",
		}
	}

	// Create intent with proper raw message for absent requests
	rawMessage := action
	if action == "absent" && reason != "" {
		rawMessage = action + " " + reason
	}

	intent := &erp_modules.Intent{
		Category:   "attendance",
		Action:     action,
		Confidence: 1.0,
		RawMessage: rawMessage,
	}

	// Create module context
	ctx := &erp_modules.ModuleContext{
		Context: context.Background(),
		User:    user,
	}

	// Get attendance module and execute
	module, exists := p.moduleRegistry.GetModule("attendance")
	if !exists {
		return &erp_modules.ModuleResponse{
			Success: false,
			Message: "Attendance module not found",
			Error:   "Attendance module not registered",
		}
	}

	attendanceModule, ok := module.(*attendance.AttendanceModule)
	if !ok {
		return &erp_modules.ModuleResponse{
			Success: false,
			Message: "Invalid attendance module type",
			Error:   "Module type assertion failed",
		}
	}

	// For absent requests, try ProcessUserMessage first
	if action == "absent" {
		absenceContext := &erp_modules.ModuleContext{
			Context: context.Background(),
			User:    user,
		}

		dummyPost := &model.Post{
			Message: "absent " + reason,
			UserId:  user.Id,
		}
		absenceContext.OriginalPost = dummyPost

		response, err := attendanceModule.ProcessUserMessage(absenceContext, "absent "+reason)
		if err == nil && response != nil {
			return response
		}
	}

	response, err := attendanceModule.Execute(ctx, intent)
	if err != nil {
		return &erp_modules.ModuleResponse{
			Success: false,
			Message: "Failed to process attendance request",
			Error:   err.Error(),
		}
	}

	if response == nil {
		return &erp_modules.ModuleResponse{
			Success: false,
			Message: "No response from attendance module",
			Error:   "Nil response from module",
		}
	}

	return response
}

// getTimeFromData extracts time from response data
func getTimeFromData(data map[string]interface{}) string {
	if data == nil {
		return ""
	}

	if time, ok := data["time"].(string); ok {
		return time
	}

	if date, ok := data["date"].(string); ok {
		return date
	}

	return ""
}
