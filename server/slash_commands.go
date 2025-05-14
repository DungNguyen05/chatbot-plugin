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
		Description:      "Mark your attendance for the active roll call",
		AutoComplete:     true,
		AutoCompleteHint: "[optional note]",
		AutoCompleteDesc: "Mark yourself as present in the active roll call",
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
	default:
		return &model.CommandResponse{
			ResponseType: model.CommandResponseTypeEphemeral,
			Text:         fmt.Sprintf("Unknown command: %s", command),
		}, nil
	}
}

// executeCheckInCommand handles the /checkin command
func (p *Plugin) executeCheckInCommand(args *model.CommandArgs) *model.CommandResponse {
	// Get user and channel info
	user, err := p.pluginAPI.User.Get(args.UserId)
	if err != nil {
		return &model.CommandResponse{
			ResponseType: model.CommandResponseTypeEphemeral,
			Text:         "Error getting user information. Please try again.",
		}
	}

	channel, err := p.pluginAPI.Channel.Get(args.ChannelId)
	if err != nil {
		return &model.CommandResponse{
			ResponseType: model.CommandResponseTypeEphemeral,
			Text:         "Error getting channel information. Please try again.",
		}
	}

	// Check if there's an active roll call
	if !p.rollCallManager.IsRollCallActive(channel.Id) {
		return &model.CommandResponse{
			ResponseType: model.CommandResponseTypeEphemeral,
			Text:         "There is no active roll call in this channel. Please wait for a roll call to be started.",
		}
	}

	// Get the default bot
	bot := p.GetBotByUsernameOrFirst(p.getConfiguration().DefaultBotName)
	if bot == nil {
		return &model.CommandResponse{
			ResponseType: model.CommandResponseTypeEphemeral,
			Text:         "Error: Unable to find the AI bot. Please contact your administrator.",
		}
	}

	// Extract note from command
	note := ""
	if len(strings.Fields(args.Command)) > 1 {
		note = strings.TrimSpace(strings.TrimPrefix(args.Command, "/checkin"))
	}

	// Record attendance directly
	rollCall, isNewResponse, err := p.rollCallManager.RespondToRollCall(channel.Id, args.UserId)
	if err != nil {
		return &model.CommandResponse{
			ResponseType: model.CommandResponseTypeEphemeral,
			Text:         "Error recording attendance: " + err.Error(),
		}
	}

	// If note was provided, store it
	if note != "" {
		if rollCall.RespondedNotes == nil {
			rollCall.RespondedNotes = make(map[string]string)
		}
		rollCall.RespondedNotes[args.UserId] = note
	}

	// Record in ERP if this is a new response
	var checkinTimeFormatted string
	var erpMessage string

	if isNewResponse {
		// Check if user has already been recorded in ERP
		alreadyRecorded, _ := p.rollCallManager.IsUserERPRecorded(channel.Id, args.UserId)
		if !alreadyRecorded {
			// Get employee name from user
			employeeName := p.GetEmployeeNameFromUser(user)

			// Add note to ERP if provided
			if note != "" {
				employeeName += " (" + note + ")"
			}

			// Try to record check-in in ERP
			formattedTime, erpErr := p.RecordEmployeeCheckin(employeeName)
			if erpErr != nil {
				p.API.LogError("Failed to record employee check-in in ERP", "error", erpErr.Error())
				erpMessage = "\n\n⚠️ There was an issue recording your attendance in the ERP system. An administrator has been notified."

				// Use Vietnam time for the response even if ERP failed
				vietTimeStr, timeErr := FormatVietnamTime()
				if timeErr != nil {
					p.API.LogError("Failed to get Vietnam time", "error", timeErr.Error())
					// Fallback to server time if Vietnam time fails
					serverTime := model.GetMillis()
					checkinTimeFormatted = time.UnixMilli(serverTime).Format("2006-01-02 15:04:05")
				} else {
					checkinTimeFormatted = vietTimeStr
				}
			} else {
				// Mark user as recorded in ERP
				_ = p.rollCallManager.MarkUserERPRecorded(channel.Id, args.UserId)
				checkinTimeFormatted = formattedTime

				// Get configured checkout time
				configuredCheckoutTime := p.getConfiguration().AutoCheckoutTime
				if configuredCheckoutTime == "" {
					configuredCheckoutTime = DefaultAutoCheckoutTime
				}

				// Get today's date with the configured checkout time
				checkoutTime, timeErr := GetCheckoutTimeForToday(configuredCheckoutTime)
				if timeErr != nil {
					p.API.LogWarn("Failed to get checkout time", "error", timeErr.Error())
					erpMessage = fmt.Sprintf("\n\n✅ Your check-in has been recorded in the ERP system at **%s**.\nℹ️ No automatic checkout scheduled: %s",
						formattedTime, timeErr.Error())
				} else {
					// Format checkout time for ERP
					checkoutTimeStr := FormatTimeForERP(checkoutTime)

					p.API.LogDebug("Scheduled checkout",
						"user", user.Username,
						"checkin", formattedTime,
						"checkout", checkoutTimeStr)

					// Record checkout in ERP
					checkoutFormatted, checkoutErr := p.RecordEmployeeCheckout(employeeName, checkoutTimeStr)
					if checkoutErr != nil {
						p.API.LogError("Failed to record employee checkout in ERP", "error", checkoutErr.Error())
						erpMessage = fmt.Sprintf("\n\n✅ Your check-in has been recorded in the ERP system at **%s**.\n⚠️ There was an issue recording your automatic checkout. An administrator has been notified.", formattedTime)
					} else {
						// Mark checkout as recorded
						_ = p.rollCallManager.MarkUserCheckoutRecorded(channel.Id, args.UserId)

						erpMessage = fmt.Sprintf("\n\n✅ Your attendance has been recorded in the ERP system:\n- Check-in: **%s**\n- Check-out: **%s**",
							formattedTime, checkoutFormatted)
					}
				}
			}
		} else {
			// Get Vietnam time for cases where ERP recording isn't needed
			vietTimeStr, timeErr := FormatVietnamTime()
			if timeErr != nil {
				p.API.LogError("Failed to get Vietnam time", "error", timeErr.Error())
				// Fallback to server time if Vietnam time fails
				serverTime := model.GetMillis()
				checkinTimeFormatted = time.UnixMilli(serverTime).Format("2006-01-02 15:04:05")
			} else {
				checkinTimeFormatted = vietTimeStr
			}
		}
	} else {
		// User already responded previously, just use current Vietnam time
		vietTimeStr, timeErr := FormatVietnamTime()
		if timeErr != nil {
			p.API.LogError("Failed to get Vietnam time", "error", timeErr.Error())
			// Fallback to server time if Vietnam time fails
			serverTime := model.GetMillis()
			checkinTimeFormatted = time.UnixMilli(serverTime).Format("2006-01-02 15:04:05")
		} else {
			checkinTimeFormatted = vietTimeStr
		}
	}

	// Create response message
	responseText := fmt.Sprintf("✅ Your attendance has been recorded at **%s**!", checkinTimeFormatted)
	if note != "" {
		responseText = fmt.Sprintf("✅ Your attendance has been recorded at **%s** with note: \"%s\"", checkinTimeFormatted, note)
	}
	responseText += erpMessage

	// Return success response
	return &model.CommandResponse{
		ResponseType: model.CommandResponseTypeEphemeral,
		Text:         responseText,
	}
}
