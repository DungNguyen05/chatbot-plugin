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
		AutoCompleteHint: "[optional note]",
		AutoCompleteDesc: "Mark yourself as present in the system",
	}); err != nil {
		return err
	}

	if err := p.API.RegisterCommand(&model.Command{
		Trigger:          "checkout",
		DisplayName:      "Check-out",
		Description:      "Record your departure for today",
		AutoComplete:     true,
		AutoCompleteHint: "[optional note]",
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

// executeCheckInCommand handles the /checkin command
func (p *Plugin) executeCheckInCommand(args *model.CommandArgs) *model.CommandResponse {
	// Get user info
	user, err := p.pluginAPI.User.Get(args.UserId)
	if err != nil {
		return &model.CommandResponse{
			ResponseType: model.CommandResponseTypeEphemeral,
			Text:         "Error getting user information. Please try again.",
		}
	}

	// Extract note from command
	note := ""
	if len(strings.Fields(args.Command)) > 1 {
		note = strings.TrimSpace(strings.TrimPrefix(args.Command, "/checkin"))
	}

	// Get employee name
	employeeName := p.GetEmployeeNameFromUser(user)
	if note != "" {
		employeeName += " (" + note + ")"
	}

	// Try to record check-in in ERP
	formattedTime, erpErr := p.RecordEmployeeCheckin(employeeName)
	if erpErr != nil {
		p.API.LogError("Failed to record employee check-in in ERP", "error", erpErr.Error())
		return &model.CommandResponse{
			ResponseType: model.CommandResponseTypeEphemeral,
			Text:         "⚠️ There was an issue recording your check-in in the ERP system. An administrator has been notified.",
		}
	}

	// Create response message with successful ERP recording
	responseText := fmt.Sprintf("✅ Your check-in has been recorded in the ERP system at **%s**!", formattedTime)
	if note != "" {
		responseText = fmt.Sprintf("✅ Your check-in has been recorded in the ERP system at **%s** with note: \"%s\"", formattedTime, note)
	}

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

	// Extract note from command
	note := ""
	if len(strings.Fields(args.Command)) > 1 {
		note = strings.TrimSpace(strings.TrimPrefix(args.Command, "/checkout"))
	}

	// Get employee name
	employeeName := p.GetEmployeeNameFromUser(user)
	if note != "" {
		employeeName += " (" + note + ")"
	}

	// Try to record check-out in ERP
	formattedTime, erpErr := p.RecordEmployeeCheckout(employeeName)
	if erpErr != nil {
		p.API.LogError("Failed to record employee check-out in ERP", "error", erpErr.Error())
		return &model.CommandResponse{
			ResponseType: model.CommandResponseTypeEphemeral,
			Text:         "⚠️ There was an issue recording your check-out in the ERP system. An administrator has been notified.",
		}
	}

	// Create response message with successful ERP recording
	responseText := fmt.Sprintf("✅ Your check-out has been recorded in the ERP system at **%s**!", formattedTime)
	if note != "" {
		responseText = fmt.Sprintf("✅ Your check-out has been recorded in the ERP system at **%s** with note: \"%s\"", formattedTime, note)
	}

	// Return success response
	return &model.CommandResponse{
		ResponseType: model.CommandResponseTypeEphemeral,
		Text:         responseText,
	}
}

// executeAbsentCommand handles the /absent command
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
		"date", dateStr,
		"reason", reason)

	// Get employee name with reason
	employeeName := p.GetEmployeeNameFromUser(user) + " (ABSENT: " + reason + ")"

	// Record absence in ERP
	recordedDate, absenceErr := p.RecordEmployeeAbsent(employeeName, reason)
	if absenceErr != nil {
		p.API.LogError("Failed to record employee absence in ERP", "error", absenceErr.Error())
		return &model.CommandResponse{
			ResponseType: model.CommandResponseTypeEphemeral,
			Text:         "⚠️ There was an issue recording your absence in the ERP system. An administrator has been notified.",
		}
	}

	// Create response message
	responseText := fmt.Sprintf("📝 Your absence has been recorded for **%s** with reason: \"%s\"", recordedDate, reason)

	// Return success response
	return &model.CommandResponse{
		ResponseType: model.CommandResponseTypeEphemeral,
		Text:         responseText,
	}
}
