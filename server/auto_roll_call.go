// Copyright (c) 2023-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/mattermost/mattermost-plugin-ai/server/llm"
	"github.com/mattermost/mattermost/server/public/model"
)

const (
	DefaultRollCallStartTime = "08:00:00" // 8 AM Vietnam time
	DefaultAutoCheckoutTime  = "17:30:00" // Default checkout time if not configured
)

// StartDailyRollCall starts a roll call in all configured channels
func (p *Plugin) StartDailyRollCall() {
	// Get the bot
	bot := p.GetBotByUsernameOrFirst(p.getConfiguration().DefaultBotName)
	if bot == nil {
		p.API.LogError("Failed to get bot for automatic roll call")
		return
	}

	// Get channels from bot configuration instead of hardcoding town-square
	var channelsToPost []string

	// Handle all possible channel access configurations
	switch bot.cfg.ChannelAccessLevel {
	case llm.ChannelAccessLevelAll:
		// Allow for all channels - get all channels the bot has access to
		teams, appErr := p.API.GetTeams()
		if appErr != nil {
			p.API.LogError("Failed to get teams for automatic roll call", "error", appErr.Error())
			return
		}

		for _, team := range teams {
			channels, appErr := p.API.GetChannelsForTeamForUser(team.Id, bot.mmBot.UserId, false)
			if appErr != nil {
				p.API.LogError("Failed to get channels for team", "teamId", team.Id, "error", appErr.Error())
				continue
			}

			// Only include public and private channels (not DMs or GMs)
			for _, channel := range channels {
				if channel.Type == model.ChannelTypeOpen || channel.Type == model.ChannelTypePrivate {
					channelsToPost = append(channelsToPost, channel.Id)
				}
			}
		}

	case llm.ChannelAccessLevelAllow:
		// Allow for selected channels only
		if len(bot.cfg.ChannelIDs) > 0 {
			// Only post to the explicitly allowed channels
			channelsToPost = append(channelsToPost, bot.cfg.ChannelIDs...)
		} else {
			// If no channels are explicitly allowed, don't post anywhere
			p.API.LogInfo("No channels are explicitly allowed for roll call")
		}

	case llm.ChannelAccessLevelBlock:
		// Block selected channels - get all channels except the blocked ones
		if len(bot.cfg.ChannelIDs) == 0 {
			// If no channels are explicitly blocked, this behaves like "Allow for all channels"
			teams, appErr := p.API.GetTeams()
			if appErr != nil {
				p.API.LogError("Failed to get teams for automatic roll call", "error", appErr.Error())
				return
			}

			for _, team := range teams {
				channels, appErr := p.API.GetChannelsForTeamForUser(team.Id, bot.mmBot.UserId, false)
				if appErr != nil {
					p.API.LogError("Failed to get channels for team", "teamId", team.Id, "error", appErr.Error())
					continue
				}

				// Only include public and private channels (not DMs or GMs)
				for _, channel := range channels {
					if channel.Type == model.ChannelTypeOpen || channel.Type == model.ChannelTypePrivate {
						channelsToPost = append(channelsToPost, channel.Id)
					}
				}
			}
		} else {
			// Get all channels, exclude the blocked ones
			teams, appErr := p.API.GetTeams()
			if appErr != nil {
				p.API.LogError("Failed to get teams for automatic roll call", "error", appErr.Error())
				return
			}

			for _, team := range teams {
				channels, appErr := p.API.GetChannelsForTeamForUser(team.Id, bot.mmBot.UserId, false)
				if appErr != nil {
					p.API.LogError("Failed to get channels for team", "teamId", team.Id, "error", appErr.Error())
					continue
				}

				// Only include public and private channels (not DMs or GMs), excluding blocked ones
				for _, channel := range channels {
					if (channel.Type == model.ChannelTypeOpen || channel.Type == model.ChannelTypePrivate) &&
						!contains(bot.cfg.ChannelIDs, channel.Id) {
						channelsToPost = append(channelsToPost, channel.Id)
					}
				}
			}
		}

	case llm.ChannelAccessLevelNone:
		// Block all channels - don't post anywhere
		p.API.LogInfo("Channel access level is set to None, not posting roll call to any channels")
		return
	}

	// Log the number of channels found
	p.API.LogInfo("Starting automatic roll call", "channelCount", len(channelsToPost))

	// Get the current date in Vietnam format
	vietTime, err := GetVietnamTime()
	if err != nil {
		p.API.LogError("Failed to get Vietnam time", "error", err.Error())
		vietTime = time.Now() // Fallback to server time
	}
	dateStr := vietTime.Format("Monday, January 2, 2006")

	// Get configured auto checkout time
	autoCheckoutTime := p.getConfiguration().AutoCheckoutTime
	if autoCheckoutTime == "" {
		autoCheckoutTime = DefaultAutoCheckoutTime
	}

	// Post to each channel
	for _, channelID := range channelsToPost {
		// Start a roll call in this channel
		_, err := p.rollCallManager.StartRollCall(channelID, bot.mmBot.UserId)
		if err != nil {
			p.API.LogError("Failed to start roll call", "channelId", channelID, "error", err.Error())
			continue
		}

		// Create the announcement post
		post := &model.Post{
			ChannelId: channelID,
			UserId:    bot.mmBot.UserId,
			Message: fmt.Sprintf(
				"# 📋 Daily Roll Call - %s\n\n"+
					"Good morning team! Please respond with **'present'** to mark your attendance for today.\n\n"+
					"* Your attendance will be automatically recorded in the ERP system\n"+
					"* Automatic checkout will be recorded at %s\n"+
					"* If you're working remotely, please add a note with your location",
				dateStr,
				autoCheckoutTime,
			),
		}

		// Post the message
		if _, appErr := p.API.CreatePost(post); appErr != nil {
			p.API.LogError("Failed to create roll call post", "channelId", channelID, "error", appErr.Error())
			continue
		}

		p.API.LogInfo("Started automatic roll call", "channelId", channelID, "date", dateStr)
	}
}

// Helper function to check if a slice contains a value
func contains(slice []string, val string) bool {
	for _, item := range slice {
		if item == val {
			return true
		}
	}
	return false
}

// EndDailyRollCall ends all active roll calls and posts summaries
func (p *Plugin) EndDailyRollCall() {
	// Get bot
	bot := p.GetBotByUsernameOrFirst(p.getConfiguration().DefaultBotName)
	if bot == nil {
		p.API.LogError("Failed to get bot for ending roll calls")
		return
	}

	// Get Vietnam time for the summary date
	vietTime, err := GetVietnamTime()
	if err != nil {
		p.API.LogError("Failed to get Vietnam time", "error", err.Error())
		vietTime = time.Now() // Fallback to server time
	}
	dateStr := vietTime.Format("Monday, January 2, 2006")

	// End all active roll calls
	p.rollCallManager.mu.RLock()
	activeChannels := make([]string, 0)
	for channelID, rollCall := range p.rollCallManager.activeRollCalls {
		if rollCall.Active {
			activeChannels = append(activeChannels, channelID)
		}
	}
	p.rollCallManager.mu.RUnlock()

	for _, channelID := range activeChannels {
		// End the roll call
		rollCall, err := p.rollCallManager.EndRollCall(channelID)
		if err != nil {
			p.API.LogError("Failed to end roll call", "channelId", channelID, "error", err.Error())
			continue
		}

		// Get the full names of users who responded
		var respondedMembers []string
		for userID := range rollCall.RespondedIDs {
			respondedUser, appErr := p.API.GetUser(userID)
			if appErr != nil {
				continue
			}

			name := respondedUser.Username
			if respondedUser.FirstName != "" || respondedUser.LastName != "" {
				name = fmt.Sprintf("%s %s (@%s)", respondedUser.FirstName, respondedUser.LastName, respondedUser.Username)
			}

			// Check if recorded in ERP
			status := ""
			if recorded, _ := p.rollCallManager.IsUserERPRecorded(channelID, userID); recorded {
				status += " ✓ (Check-in recorded)"
			}

			// Check if checkout was recorded
			if recorded, _ := p.rollCallManager.IsUserCheckoutRecorded(channelID, userID); recorded {
				status += " ✓ (Check-out recorded)"
			}

			respondedMembers = append(respondedMembers, name+status)
		}

		// Create a post with the roll call summary
		membersList := "No members have responded today."
		if len(respondedMembers) > 0 {
			membersList = "- " + strings.Join(respondedMembers, "\n- ")
		}

		// Count checkins and checkouts
		checkinCount := len(rollCall.ERPRecordedUsers)
		checkoutCount := 0
		if rollCall.CheckoutRecordedUsers != nil {
			checkoutCount = len(rollCall.CheckoutRecordedUsers)
		}

		// Format duration
		duration := time.Since(rollCall.StartTime)
		durationStr := formatDuration(duration)

		// Create the summary post
		post := &model.Post{
			ChannelId: channelID,
			UserId:    bot.mmBot.UserId,
			Message: fmt.Sprintf(
				"# 📋 Daily Roll Call Summary - %s\n\n"+
					"**Active Period:** %s\n"+
					"**Total Responses:** %d\n"+
					"**ERP Records:** %d check-ins, %d check-outs\n\n"+
					"### Members Present Today:\n%s",
				dateStr,
				durationStr,
				rollCall.ResponseCount,
				checkinCount,
				checkoutCount,
				membersList,
			),
		}

		// Post the message
		if _, appErr := p.API.CreatePost(post); appErr != nil {
			p.API.LogError("Failed to create roll call summary post", "channelId", channelID, "error", appErr.Error())
			continue
		}
	}
}
