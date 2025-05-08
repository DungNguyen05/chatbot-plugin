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

	if strings.Contains(message, "start roll call") || strings.Contains(message, "start rollcall") {
		return p.handleStartRollCall(bot, channel, user, post)
	} else if strings.Contains(message, "end roll call") || strings.Contains(message, "end rollcall") {
		return p.handleEndRollCall(bot, channel, user, post)
	} else if strings.Contains(message, "roll call summary") || strings.Contains(message, "rollcall summary") {
		return p.handleRollCallSummary(bot, channel, user, post)
	} else if strings.Contains(message, "here") || strings.Contains(message, "present") ||
		strings.Contains(message, "attending") || strings.Contains(message, "i am here") {
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

	// Automatically mark the initiator as present
	_, _ = p.rollCallManager.RespondToRollCall(channel.Id, user.Id)

	// Create a post to announce the roll call
	responsePost := &model.Post{
		ChannelId: channel.Id,
		Message:   "📋 **Roll Call Started!** Please respond with 'here', 'present', or similar message to mark your attendance.",
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
	responsePost := &model.Post{
		ChannelId: channel.Id,
		Message:   fmt.Sprintf("📋 **Roll Call Ended!** %d members have responded. Ask for a 'roll call summary' to see details.", rollCall.ResponseCount),
	}
	if post.RootId != "" {
		responsePost.RootId = post.RootId
	}
	return p.botCreateNonResponsePost(bot.mmBot.UserId, user.Id, responsePost)
}

// handleRollCallResponse handles a user's response to a roll call
func (p *Plugin) handleRollCallResponse(bot *Bot, channel *model.Channel, user *model.User, post *model.Post) error {
	// Record a user's response to a roll call
	_, err := p.rollCallManager.RespondToRollCall(channel.Id, user.Id)
	if err != nil {
		// Don't respond if there's no active roll call - could be a normal message
		return nil
	}

	// For DM channels, acknowledge the response
	if channel.Type == model.ChannelTypeDirect {
		responsePost := &model.Post{
			ChannelId: channel.Id,
			Message:   "✅ Your attendance has been recorded!",
		}
		if post.RootId != "" {
			responsePost.RootId = post.RootId
		}
		return p.botCreateNonResponsePost(bot.mmBot.UserId, user.Id, responsePost)
	}

	return nil
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
		respondedMembers = append(respondedMembers, name)
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

	responsePost := &model.Post{
		ChannelId: channel.Id,
		Message:   fmt.Sprintf("## Roll Call Summary %s\n\n**Status:** %s\n**Responses:** %d\n\n### Members Present:\n%s", durationStr, status, rollCall.ResponseCount, membersList),
	}
	if post.RootId != "" {
		responsePost.RootId = post.RootId
	}
	return p.botCreateNonResponsePost(bot.mmBot.UserId, user.Id, responsePost)
}
