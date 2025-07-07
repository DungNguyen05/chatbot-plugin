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
}

// handleAPICheckIn handles the API endpoint for check-in
func (p *Plugin) handleAPICheckIn(c *gin.Context) {
	userID := c.GetHeader("Mattermost-User-Id")

	// Get user info
	user, err := p.pluginAPI.User.Get(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Error getting user information. Please try again.",
		})
		return
	}

	// Process through ERP module
	response := p.processAttendanceRequest(user, "check_in", "")

	c.JSON(http.StatusOK, CheckInResponse{
		Success: response.Success,
		Message: response.Message,
		Time:    getTimeFromData(response.Data),
	})
}

// handleAPICheckOut handles the API endpoint for check-out
func (p *Plugin) handleAPICheckOut(c *gin.Context) {
	userID := c.GetHeader("Mattermost-User-Id")

	// Get user info
	user, err := p.pluginAPI.User.Get(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Error getting user information. Please try again.",
		})
		return
	}

	// Process through ERP module
	response := p.processAttendanceRequest(user, "check_out", "")

	c.JSON(http.StatusOK, CheckInResponse{
		Success: response.Success,
		Message: response.Message,
		Time:    getTimeFromData(response.Data),
	})
}

// handleAPIAbsent handles the API endpoint for marking absent
func (p *Plugin) handleAPIAbsent(c *gin.Context) {
	userID := c.GetHeader("Mattermost-User-Id")

	var req AbsentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Please provide a reason for your absence.",
		})
		return
	}

	// Get user info
	user, err := p.pluginAPI.User.Get(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Error getting user information. Please try again.",
		})
		return
	}

	reason := strings.TrimSpace(req.Reason)
	if reason == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Please provide a reason for your absence.",
		})
		return
	}

	// Process through ERP module
	response := p.processAttendanceRequest(user, "absent", reason)

	c.JSON(http.StatusOK, CheckInResponse{
		Success: response.Success,
		Message: response.Message,
		Time:    getTimeFromData(response.Data),
	})
}

// processAttendanceRequest processes attendance requests through the ERP module
func (p *Plugin) processAttendanceRequest(user *model.User, action, reason string) *erp_modules.ModuleResponse {
	if p.moduleManager == nil {
		return &erp_modules.ModuleResponse{
			Success: false,
			Message: "ERP modules not initialized",
		}
	}

	// Create intent
	intent := &erp_modules.Intent{
		Category:   "attendance",
		Action:     action,
		Confidence: 1.0,
		RawMessage: action,
	}

	// Create module context
	ctx := &erp_modules.ModuleContext{
		Context: context.Background(),
		User:    user,
	}

	// Get attendance module and execute
	if module, exists := p.moduleRegistry.GetModule("attendance"); exists {
		if attendanceModule, ok := module.(*attendance.AttendanceModule); ok {
			response, err := attendanceModule.Execute(ctx, intent)
			if err != nil {
				return &erp_modules.ModuleResponse{
					Success: false,
					Message: "Failed to process attendance request",
					Error:   err.Error(),
				}
			}
			return response
		}
	}

	return &erp_modules.ModuleResponse{
		Success: false,
		Message: "Attendance module not found",
	}
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
