// Copyright (c) 2023-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package main

import (
	"fmt"
	"strings"

	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/plugin"
)

// registerSlashCommands registers all slash commands the plugin uses
func (p *Plugin) registerSlashCommands() error {
	// Register the new UI-based rollcall command
	if err := p.API.RegisterCommand(&model.Command{
		Trigger:          "rollcall",
		DisplayName:      "Roll Call UI",
		Description:      "Open the Roll Call interface",
		AutoComplete:     true,
		AutoCompleteDesc: "Open the Roll Call interface for check-in, check-out, and absence marking",
	}); err != nil {
		return err
	}

	// Keep the original commands but modify them to suggest using the UI
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
	case "rollcall":
		return p.executeRollCallUICommand(args), nil
	case "checkin":
		return p.executeCheckInCommand(args), nil
	case "checkout":
		return p.executeCheckOutCommand(args), nil
	case "absent":
		return p.executeAbsentCommand(args), nil
	default:
		// Get user for localization
		user, err := p.pluginAPI.User.Get(args.UserId)
		if err != nil {
			return &model.CommandResponse{
				ResponseType: model.CommandResponseTypeEphemeral,
				Text:         fmt.Sprintf("Unknown command: %s", command),
			}, nil
		}
		T := i18nLocalizerFunc(p.i18n, user.Locale)
		return &model.CommandResponse{
			ResponseType: model.CommandResponseTypeEphemeral,
			Text:         T("rollcall.unknown.command", "Unknown command: %s", command),
		}, nil
	}
}

// executeRollCallUICommand shows a message about the UI
func (p *Plugin) executeRollCallUICommand(args *model.CommandArgs) *model.CommandResponse {
	// Get user for localization
	user, err := p.pluginAPI.User.Get(args.UserId)
	if err != nil {
		return &model.CommandResponse{
			ResponseType: model.CommandResponseTypeEphemeral,
			Text:         "Roll Call interface is opening...",
		}
	}

	T := i18nLocalizerFunc(p.i18n, user.Locale)
	return &model.CommandResponse{
		ResponseType: model.CommandResponseTypeEphemeral,
		Text:         T("rollcall.ui.opening", "Roll Call interface is opening... You can also access it from the main menu or channel header button for a better experience with clickable buttons!"),
	}
}

// executeCheckInCommand handles the /checkin command
func (p *Plugin) executeCheckInCommand(args *model.CommandArgs) *model.CommandResponse {
	user, err := p.pluginAPI.User.Get(args.UserId)
	if err != nil {
		return &model.CommandResponse{
			ResponseType: model.CommandResponseTypeEphemeral,
			Text:         "Error getting user information. Please try again.",
		}
	}

	// Get user's locale for translations
	T := i18nLocalizerFunc(p.i18n, user.Locale)

	if len(strings.Fields(args.Command)) > 1 {
		return &model.CommandResponse{
			ResponseType: model.CommandResponseTypeEphemeral,
			Text:         T("rollcall.checkin.no.params", "The check-in command doesn't accept additional parameters. Please use `/checkin` without any notes."),
		}
	}

	// Process through ERP module
	response := p.processAttendanceRequest(user, "check_in", "")

	return &model.CommandResponse{
		ResponseType: model.CommandResponseTypeEphemeral,
		Text:         response.Message,
	}
}

// executeCheckOutCommand handles the /checkout command
func (p *Plugin) executeCheckOutCommand(args *model.CommandArgs) *model.CommandResponse {
	user, err := p.pluginAPI.User.Get(args.UserId)
	if err != nil {
		return &model.CommandResponse{
			ResponseType: model.CommandResponseTypeEphemeral,
			Text:         "Error getting user information. Please try again.",
		}
	}

	// Get user's locale for translations
	T := i18nLocalizerFunc(p.i18n, user.Locale)

	if len(strings.Fields(args.Command)) > 1 {
		return &model.CommandResponse{
			ResponseType: model.CommandResponseTypeEphemeral,
			Text:         T("rollcall.checkout.no.params", "The check-out command doesn't accept additional parameters. Please use `/checkout` without any notes."),
		}
	}

	// Process through ERP module
	response := p.processAttendanceRequest(user, "check_out", "")

	return &model.CommandResponse{
		ResponseType: model.CommandResponseTypeEphemeral,
		Text:         response.Message,
	}
}

// executeAbsentCommand handles the /absent command
func (p *Plugin) executeAbsentCommand(args *model.CommandArgs) *model.CommandResponse {
	user, err := p.pluginAPI.User.Get(args.UserId)
	if err != nil {
		return &model.CommandResponse{
			ResponseType: model.CommandResponseTypeEphemeral,
			Text:         "Error getting user information. Please try again.",
		}
	}

	// Get user's locale for translations
	T := i18nLocalizerFunc(p.i18n, user.Locale)

	parts := strings.Fields(args.Command)
	if len(parts) <= 1 {
		return &model.CommandResponse{
			ResponseType: model.CommandResponseTypeEphemeral,
			Text:         T("rollcall.absent.reason.required", "Please provide a reason for your absence. Example: `/absent Sick leave`"),
		}
	}

	reason := strings.TrimSpace(strings.TrimPrefix(args.Command, "/absent"))

	// Process through ERP module
	response := p.processAttendanceRequest(user, "absent", reason)

	return &model.CommandResponse{
		ResponseType: model.CommandResponseTypeEphemeral,
		Text:         response.Message,
	}
}
