// Copyright (c) 2023-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/plugin"
)

// registerSlashCommands registers all slash commands the plugin uses
func (p *Plugin) registerSlashCommands() error {
	if err := p.API.RegisterCommand(&model.Command{
		Trigger:          "checkin",
		DisplayName:      "Check-in",
		Description:      "Record your attendance for today",
		AutoComplete:     true,
		AutoCompleteDesc: "Mark yourself as present in the system",
	}); err != nil {
		return err
	}

	if err := p.API.RegisterCommand(&model.Command{
		Trigger:          "checkout",
		DisplayName:      "Check-out",
		Description:      "Record your departure for today",
		AutoComplete:     true,
		AutoCompleteDesc: "Record when you're leaving for the day",
	}); err != nil {
		return err
	}

	if err := p.API.RegisterCommand(&model.Command{
		Trigger:          "absent",
		DisplayName:      "Absent",
		Description:      "Mark yourself as absent",
		AutoComplete:     true,
		AutoCompleteHint: "<reason>",
		AutoCompleteDesc: "Record that you'll be absent today with a reason",
	}); err != nil {
		return err
	}

	return nil
}

// ExecuteCommand handles slash command execution
func (p *Plugin) ExecuteCommand(c *plugin.Context, args *model.CommandArgs) (*model.CommandResponse, *model.AppError) {
	parts := strings.Fields(args.Command)
	command := parts[0]

	// Get trimmed command by removing the slash
	command = strings.TrimPrefix(command, "/")

	switch command {
	case "checkin":
		return p.executeCheckInCommand(args), nil
	case "checkout":
		return p.executeCheckOutCommand(args), nil
	case "absent":
		return p.executeAbsentCommand(args), nil
	default:
		return &model.CommandResponse{
			ResponseType: model.CommandResponseTypeEphemeral,
			Text:         fmt.Sprintf("Unknown command: %s", command),
		}, nil
	}
}

// executeCheckInCommand - modify to use employee ID lookup
func (p *Plugin) executeCheckInCommand(args *model.CommandArgs) *model.CommandResponse {
	// Get user info
	user, err := p.pluginAPI.User.Get(args.UserId)
	if err != nil {
		return &model.CommandResponse{
			ResponseType: model.CommandResponseTypeEphemeral,
			Text:         "Error getting user information. Please try again.",
		}
	}

	// Check if there are additional arguments (notes are no longer allowed)
	if len(strings.Fields(args.Command)) > 1 {
		return &model.CommandResponse{
			ResponseType: model.CommandResponseTypeEphemeral,
			Text:         "The check-in command doesn't accept additional parameters. Please use `/checkin` without any notes.",
		}
	}

	// Get employee ID from ERPNext using chat ID
	employeeID, err := p.GetEmployeeIDFromUser(user)
	if err != nil {
		p.API.LogError("Failed to get employee ID for user", "user_id", user.Id, "error", err.Error())
		return &model.CommandResponse{
			ResponseType: model.CommandResponseTypeEphemeral,
			Text:         "❌ Unable to find your employee record in the ERP system. Please contact your administrator to ensure your Mattermost account is linked to your employee profile.",
		}
	}

	// Try to record check-in in ERP
	formattedTime, erpErr := p.RecordEmployeeCheckin(employeeID)
	if erpErr != nil {
		p.API.LogError("Failed to record employee check-in in ERP", "employee_id", employeeID, "error", erpErr.Error())
		return &model.CommandResponse{
			ResponseType: model.CommandResponseTypeEphemeral,
			Text:         "⚠️ There was an issue recording your check-in in the ERP system. An administrator has been notified.",
		}
	}

	// Create response message with successful ERP recording
	responseText := fmt.Sprintf("✅ Your check-in has been recorded in the ERP system at **%s**!", formattedTime)

	// Get employee name for notifications (you may want to store this from the API call)
	employeeName := user.Username // Fallback to username for display
	if user.FirstName != "" || user.LastName != "" {
		employeeName = strings.TrimSpace(user.FirstName + " " + user.LastName)
	}

	// Asynchronously send notifications about the check-in
	go func() {
		if err := p.sendRollCallNotification(
			user.Id,
			employeeName, // Use display name for notifications
			RollCallEventCheckIn,
			formattedTime,
			""); err != nil {
			p.API.LogError("Failed to send check-in notification", "error", err.Error())
		}
	}()

	// Return success response
	return &model.CommandResponse{
		ResponseType: model.CommandResponseTypeEphemeral,
		Text:         responseText,
	}
}

// executeCheckOutCommand handles the /checkout command
func (p *Plugin) executeCheckOutCommand(args *model.CommandArgs) *model.CommandResponse {
	// Get user info
	user, err := p.pluginAPI.User.Get(args.UserId)
	if err != nil {
		return &model.CommandResponse{
			ResponseType: model.CommandResponseTypeEphemeral,
			Text:         "Error getting user information. Please try again.",
		}
	}

	// Check if there are additional arguments (notes are no longer allowed)
	if len(strings.Fields(args.Command)) > 1 {
		return &model.CommandResponse{
			ResponseType: model.CommandResponseTypeEphemeral,
			Text:         "The check-out command doesn't accept additional parameters. Please use `/checkout` without any notes.",
		}
	}

	// Get employee ID from ERPNext using chat ID
	employeeID, err := p.GetEmployeeIDFromUser(user)
	if err != nil {
		p.API.LogError("Failed to get employee ID for user", "user_id", user.Id, "error", err.Error())
		return &model.CommandResponse{
			ResponseType: model.CommandResponseTypeEphemeral,
			Text:         "❌ Unable to find your employee record in the ERP system. Please contact your administrator to ensure your Mattermost account is linked to your employee profile.",
		}
	}

	// Try to record check-out in ERP
	formattedTime, erpErr := p.RecordEmployeeCheckout(employeeID)
	if erpErr != nil {
		p.API.LogError("Failed to record employee check-out in ERP", "employee_id", employeeID, "error", erpErr.Error())
		return &model.CommandResponse{
			ResponseType: model.CommandResponseTypeEphemeral,
			Text:         "⚠️ There was an issue recording your check-out in the ERP system. An administrator has been notified.",
		}
	}

	// Create response message with successful ERP recording
	responseText := fmt.Sprintf("✅ Your check-out has been recorded in the ERP system at **%s**!", formattedTime)

	// Get employee name for notifications
	employeeName := user.Username // Fallback to username for display
	if user.FirstName != "" || user.LastName != "" {
		employeeName = strings.TrimSpace(user.FirstName + " " + user.LastName)
	}

	// Asynchronously send notifications about the check-out
	go func() {
		if err := p.sendRollCallNotification(
			user.Id,
			employeeName, // Use display name for notifications
			RollCallEventCheckOut,
			formattedTime,
			""); err != nil {
			p.API.LogError("Failed to send check-out notification", "error", err.Error())
		}
	}()

	// Return success response
	return &model.CommandResponse{
		ResponseType: model.CommandResponseTypeEphemeral,
		Text:         responseText,
	}
}

// executeAbsentCommand - modify similarly
func (p *Plugin) executeAbsentCommand(args *model.CommandArgs) *model.CommandResponse {
	// Get user info
	user, err := p.pluginAPI.User.Get(args.UserId)
	if err != nil {
		return &model.CommandResponse{
			ResponseType: model.CommandResponseTypeEphemeral,
			Text:         "Error getting user information. Please try again.",
		}
	}

	// Extract reason from command (required)
	parts := strings.Fields(args.Command)
	if len(parts) <= 1 {
		return &model.CommandResponse{
			ResponseType: model.CommandResponseTypeEphemeral,
			Text:         "Please provide a reason for your absence. Example: `/absent Sick leave`",
		}
	}

	reason := strings.TrimSpace(strings.TrimPrefix(args.Command, "/absent"))

	// Get employee ID from ERPNext using chat ID
	employeeID, err := p.GetEmployeeIDFromUser(user)
	if err != nil {
		p.API.LogError("Failed to get employee ID for user", "user_id", user.Id, "error", err.Error())
		return &model.CommandResponse{
			ResponseType: model.CommandResponseTypeEphemeral,
			Text:         "❌ Unable to find your employee record in the ERP system. Please contact your administrator to ensure your Mattermost account is linked to your employee profile.",
		}
	}

	// Get current date in Vietnam time
	vietTime, err := GetVietnamTime()
	if err != nil {
		p.API.LogError("Failed to get Vietnam time", "error", err.Error())
		// Use server time as fallback
		vietTime = time.Now()
	}

	dateStr := vietTime.Format("Monday, January 2, 2006")

	// Log absence
	p.API.LogInfo("User marked absent",
		"user", user.Username,
		"employee_id", employeeID,
		"date", dateStr,
		"reason", reason)

	// Record absence in ERP
	recordedDate, absenceErr := p.RecordEmployeeAbsent(employeeID, reason)
	if absenceErr != nil {
		p.API.LogError("Failed to record employee absence in ERP", "employee_id", employeeID, "error", absenceErr.Error())
		return &model.CommandResponse{
			ResponseType: model.CommandResponseTypeEphemeral,
			Text:         "⚠️ There was an issue recording your absence in the ERP system. An administrator has been notified.",
		}
	}

	// Create response message
	responseText := fmt.Sprintf("📝 Your absence has been recorded for **%s** with reason: \"%s\"", recordedDate, reason)

	// Get employee name for notifications
	employeeName := user.Username // Fallback to username for display
	if user.FirstName != "" || user.LastName != "" {
		employeeName = strings.TrimSpace(user.FirstName + " " + user.LastName)
	}

	// Asynchronously send notifications about the absence
	go func() {
		if err := p.sendRollCallNotification(
			user.Id,
			employeeName, // Use display name for notifications
			RollCallEventAbsent,
			recordedDate,
			reason); err != nil {
			p.API.LogError("Failed to send absence notification", "error", err.Error())
		}
	}()

	// Return success response
	return &model.CommandResponse{
		ResponseType: model.CommandResponseTypeEphemeral,
		Text:         responseText,
	}
}
