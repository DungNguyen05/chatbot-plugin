// Copyright (c) 2023-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/mattermost/mattermost/server/public/model"
)

// handleRollCall processes roll call related commands
func (p *Plugin) handleRollCall(bot *Bot, channel *model.Channel, user *model.User, post *model.Post) error {
	message := strings.ToLower(post.Message)

	if strings.Contains(message, "start roll call") {
		return p.handleStartRollCall(bot, channel, user, post)
	} else if strings.Contains(message, "end roll call") {
		return p.handleEndRollCall(bot, channel, user, post)
	} else if strings.Contains(message, "roll call summary") {
		return p.handleRollCallSummary(bot, channel, user, post)
	} else if strings.Contains(message, "present") {
		return p.handleRollCallResponse(bot, channel, user, post)
	}

	return nil
}

// handleStartRollCall handles a request to start a roll call
func (p *Plugin) handleStartRollCall(bot *Bot, channel *model.Channel, user *model.User, post *model.Post) error {
	// Start a new roll call
	_, err := p.rollCallManager.StartRollCall(channel.Id, user.Id)
	if err != nil {
		responsePost := &model.Post{
			ChannelId: channel.Id,
			Message:   fmt.Sprintf("Error starting roll call: %s", err.Error()),
		}
		if post.RootId != "" {
			responsePost.RootId = post.RootId
		}
		return p.botCreateNonResponsePost(bot.mmBot.UserId, user.Id, responsePost)
	}

	// Create a post to announce the roll call
	responsePost := &model.Post{
		ChannelId: channel.Id,
		Message:   "📋 **Roll Call Started!** Please respond with 'present' to mark your attendance. Your attendance will be automatically recorded in the ERP system.",
	}
	if post.RootId != "" {
		responsePost.RootId = post.RootId
	}
	return p.botCreateNonResponsePost(bot.mmBot.UserId, user.Id, responsePost)
}

// handleEndRollCall handles a request to end a roll call
func (p *Plugin) handleEndRollCall(bot *Bot, channel *model.Channel, user *model.User, post *model.Post) error {
	// End an active roll call
	rollCall, err := p.rollCallManager.EndRollCall(channel.Id)
	if err != nil {
		responsePost := &model.Post{
			ChannelId: channel.Id,
			Message:   fmt.Sprintf("Error ending roll call: %s", err.Error()),
		}
		if post.RootId != "" {
			responsePost.RootId = post.RootId
		}
		return p.botCreateNonResponsePost(bot.mmBot.UserId, user.Id, responsePost)
	}

	// Create a post to announce the roll call has ended
	erpCount := len(rollCall.ERPRecordedUsers)
	erpMessage := ""
	if erpCount > 0 {
		erpMessage = fmt.Sprintf(" %d attendance records were sent to the ERP system.", erpCount)
	}

	responsePost := &model.Post{
		ChannelId: channel.Id,
		Message:   fmt.Sprintf("📋 **Roll Call Ended!** %d members have responded.%s Ask for a 'roll call summary' to see details.", rollCall.ResponseCount, erpMessage),
	}
	if post.RootId != "" {
		responsePost.RootId = post.RootId
	}
	return p.botCreateNonResponsePost(bot.mmBot.UserId, user.Id, responsePost)
}

// handleRollCallResponse handles a user's response to a roll call
// handleRollCallResponse handles a user's response to a roll call
func (p *Plugin) handleRollCallResponse(bot *Bot, channel *model.Channel, user *model.User, post *model.Post) error {
	// Record a user's response to a roll call
	_, isNewResponse, err := p.rollCallManager.RespondToRollCall(channel.Id, user.Id)
	if err != nil {
		// Don't respond if there's no active roll call - could be a normal message
		return nil
	}

	// If this is a new response, record it in ERP
	var erpMessage string
	var timeFormatted string

	if isNewResponse {
		// Check if user has already been recorded in ERP
		alreadyRecorded, _ := p.rollCallManager.IsUserERPRecorded(channel.Id, user.Id)
		if !alreadyRecorded {
			// Get employee name from user
			employeeName := p.GetEmployeeNameFromUser(user)

			// Try to record in ERP - this now returns the formatted time string used
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
					timeFormatted = time.UnixMilli(serverTime).Format("2006-01-02 15:04:05")
				} else {
					timeFormatted = vietTimeStr
				}
			} else {
				// Mark user as recorded in ERP
				_ = p.rollCallManager.MarkUserERPRecorded(channel.Id, user.Id)
				erpMessage = fmt.Sprintf("\n\n✅ Your attendance has also been recorded in the ERP system at **%s**.", formattedTime)

				// Use the same formatted time that was sent to ERP
				timeFormatted = formattedTime
			}
		} else {
			// Get Vietnam time for cases where ERP recording isn't needed
			vietTimeStr, timeErr := FormatVietnamTime()
			if timeErr != nil {
				p.API.LogError("Failed to get Vietnam time", "error", timeErr.Error())
				// Fallback to server time if Vietnam time fails
				serverTime := model.GetMillis()
				timeFormatted = time.UnixMilli(serverTime).Format("2006-01-02 15:04:05")
			} else {
				timeFormatted = vietTimeStr
			}
		}
	} else {
		// User already responded previously, just use current Vietnam time
		vietTimeStr, timeErr := FormatVietnamTime()
		if timeErr != nil {
			p.API.LogError("Failed to get Vietnam time", "error", timeErr.Error())
			// Fallback to server time if Vietnam time fails
			serverTime := model.GetMillis()
			timeFormatted = time.UnixMilli(serverTime).Format("2006-01-02 15:04:05")
		} else {
			timeFormatted = vietTimeStr
		}
	}

	// For DM channels, acknowledge the response
	responsePost := &model.Post{
		ChannelId: channel.Id,
		Message:   fmt.Sprintf("✅ Your attendance has been recorded at **%s**!%s", timeFormatted, erpMessage),
	}
	if post.RootId != "" {
		responsePost.RootId = post.RootId
	}
	return p.botCreateNonResponsePost(bot.mmBot.UserId, user.Id, responsePost)
}

// handleRollCallSummary handles a request for a roll call summary
func (p *Plugin) handleRollCallSummary(bot *Bot, channel *model.Channel, user *model.User, post *model.Post) error {
	// Get the roll call summary
	rollCall, err := p.rollCallManager.GetRollCall(channel.Id)
	if err != nil {
		responsePost := &model.Post{
			ChannelId: channel.Id,
			Message:   fmt.Sprintf("Error getting roll call: %s", err.Error()),
		}
		if post.RootId != "" {
			responsePost.RootId = post.RootId
		}
		return p.botCreateNonResponsePost(bot.mmBot.UserId, user.Id, responsePost)
	}

	// Get the full names of users who responded
	var respondedMembers []string
	for userID := range rollCall.RespondedIDs {
		respondedUser, userErr := p.pluginAPI.User.Get(userID)
		if userErr != nil {
			continue
		}

		name := respondedUser.Username
		if respondedUser.FirstName != "" || respondedUser.LastName != "" {
			name = fmt.Sprintf("%s %s (@%s)", respondedUser.FirstName, respondedUser.LastName, respondedUser.Username)
		}

		// Check if recorded in ERP
		erpStatus := ""
		if recorded, _ := p.rollCallManager.IsUserERPRecorded(channel.Id, userID); recorded {
			erpStatus = " ✓ (ERP)"
		}

		respondedMembers = append(respondedMembers, name+erpStatus)
	}

	// Format duration
	duration := time.Since(rollCall.StartTime)
	var durationStr string
	if rollCall.Active {
		durationStr = fmt.Sprintf("(Active for %s)", formatDuration(duration))
	} else {
		durationStr = fmt.Sprintf("(Was active for %s)", formatDuration(duration))
	}

	// Create a post with the roll call summary
	status := "Active"
	if !rollCall.Active {
		status = "Ended"
	}

	membersList := "No members have responded yet."
	if len(respondedMembers) > 0 {
		membersList = "- " + strings.Join(respondedMembers, "\n- ")
	}

	// Add ERP info
	erpInfo := fmt.Sprintf("\n\n**ERP Integration:** %d attendance records sent to ERP system", len(rollCall.ERPRecordedUsers))

	responsePost := &model.Post{
		ChannelId: channel.Id,
		Message:   fmt.Sprintf("## Roll Call Summary %s\n\n**Status:** %s\n**Responses:** %d%s\n\n### Members Present:\n%s", durationStr, status, rollCall.ResponseCount, erpInfo, membersList),
	}
	if post.RootId != "" {
		responsePost.RootId = post.RootId
	}
	return p.botCreateNonResponsePost(bot.mmBot.UserId, user.Id, responsePost)
}
