// Copyright (c) 2023-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package main

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
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

	// Get user's locale for translations
	T := i18nLocalizerFunc(p.i18n, user.Locale)

	// Get employee ID from ERPNext using chat ID
	employeeID, err := p.GetEmployeeIDFromUser(user)
	if err != nil {
		p.API.LogError("Failed to get employee ID for user", "user_id", user.Id, "error", err.Error())
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": T("rollcall.employee.not.found", "Unable to find your employee record in the ERP system. Please contact your administrator to ensure your Mattermost account is linked to your employee profile."),
		})
		return
	}

	// Try to record check-in in ERP
	formattedTime, erpErr := p.RecordEmployeeCheckin(employeeID)
	if erpErr != nil {
		p.API.LogError("Failed to record employee check-in in ERP", "employee_id", employeeID, "error", erpErr.Error())
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": T("rollcall.erp.error", "There was an issue recording your check-in in the ERP system. An administrator has been notified."),
		})
		return
	}

	// Get employee name for notifications
	employeeName := user.Username // Fallback to username for display
	if user.FirstName != "" || user.LastName != "" {
		employeeName = strings.TrimSpace(user.FirstName + " " + user.LastName)
	}

	// Asynchronously send notifications about the check-in
	go func() {
		if err := p.sendRollCallNotification(
			user.Id,
			employeeName,
			RollCallEventCheckIn,
			formattedTime,
			""); err != nil {
			p.API.LogError("Failed to send check-in notification", "error", err.Error())
		}
	}()

	successMessage := T("rollcall.checkin.success", "Your check-in has been recorded in the ERP system at %s!", formattedTime)

	c.JSON(http.StatusOK, CheckInResponse{
		Success: true,
		Message: successMessage,
		Time:    formattedTime,
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

	// Get user's locale for translations
	T := i18nLocalizerFunc(p.i18n, user.Locale)

	// Get employee ID from ERPNext using chat ID
	employeeID, err := p.GetEmployeeIDFromUser(user)
	if err != nil {
		p.API.LogError("Failed to get employee ID for user", "user_id", user.Id, "error", err.Error())
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": T("rollcall.employee.not.found", "Unable to find your employee record in the ERP system. Please contact your administrator to ensure your Mattermost account is linked to your employee profile."),
		})
		return
	}

	// Try to record check-out in ERP
	formattedTime, erpErr := p.RecordEmployeeCheckout(employeeID)
	if erpErr != nil {
		p.API.LogError("Failed to record employee check-out in ERP", "employee_id", employeeID, "error", erpErr.Error())
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": T("rollcall.erp.error", "There was an issue recording your check-out in the ERP system. An administrator has been notified."),
		})
		return
	}

	// Get employee name for notifications
	employeeName := user.Username // Fallback to username for display
	if user.FirstName != "" || user.LastName != "" {
		employeeName = strings.TrimSpace(user.FirstName + " " + user.LastName)
	}

	// Asynchronously send notifications about the check-out
	go func() {
		if err := p.sendRollCallNotification(
			user.Id,
			employeeName,
			RollCallEventCheckOut,
			formattedTime,
			""); err != nil {
			p.API.LogError("Failed to send check-out notification", "error", err.Error())
		}
	}()

	successMessage := T("rollcall.checkout.success", "Your check-out has been recorded in the ERP system at %s!", formattedTime)

	c.JSON(http.StatusOK, CheckInResponse{
		Success: true,
		Message: successMessage,
		Time:    formattedTime,
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

	// Get user's locale for translations
	T := i18nLocalizerFunc(p.i18n, user.Locale)

	reason := strings.TrimSpace(req.Reason)
	if reason == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": T("rollcall.absent.reason.required", "Please provide a reason for your absence."),
		})
		return
	}

	// Get employee ID from ERPNext using chat ID
	employeeID, err := p.GetEmployeeIDFromUser(user)
	if err != nil {
		p.API.LogError("Failed to get employee ID for user", "user_id", user.Id, "error", err.Error())
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": T("rollcall.employee.not.found", "Unable to find your employee record in the ERP system. Please contact your administrator to ensure your Mattermost account is linked to your employee profile."),
		})
		return
	}

	// Record absence in ERP
	recordedDate, absenceErr := p.RecordEmployeeAbsent(employeeID, reason)
	if absenceErr != nil {
		p.API.LogError("Failed to record employee absence in ERP", "employee_id", employeeID, "error", absenceErr.Error())
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": T("rollcall.erp.error", "There was an issue recording your absence in the ERP system. An administrator has been notified."),
		})
		return
	}

	// Get employee name for notifications
	employeeName := user.Username // Fallback to username for display
	if user.FirstName != "" || user.LastName != "" {
		employeeName = strings.TrimSpace(user.FirstName + " " + user.LastName)
	}

	// Asynchronously send notifications about the absence
	go func() {
		if err := p.sendRollCallNotification(
			user.Id,
			employeeName,
			RollCallEventAbsent,
			recordedDate,
			reason); err != nil {
			p.API.LogError("Failed to send absence notification", "error", err.Error())
		}
	}()

	successMessage := T("rollcall.absent.success", "Your absence has been recorded for %s with reason: \"%s\"", recordedDate, reason)

	c.JSON(http.StatusOK, CheckInResponse{
		Success: true,
		Message: successMessage,
		Time:    recordedDate,
	})
}
