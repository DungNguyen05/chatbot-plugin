// Copyright (c) 2023-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/mattermost/mattermost/server/public/model"
)

const (
	RollCallStartTime = "15:52:00" // 8 AM Vietnam time
	RollCallEndTime   = "17:30:00" // 5:30 PM Vietnam time
)

// StartDailyRollCall starts a roll call in all configured channels
func (p *Plugin) StartDailyRollCall() {
	// Get all teams
	teams, appErr := p.API.GetTeams()
	if appErr != nil {
		p.API.LogError("Failed to get teams for automatic roll call", "error", appErr.Error())
		return
	}

	// Get the bot
	bot := p.GetBotByUsernameOrFirst(p.getConfiguration().DefaultBotName)
	if bot == nil {
		p.API.LogError("Failed to get bot for automatic roll call")
		return
	}

	for _, team := range teams {
		// Get Town Square for each team
		channel, appErr := p.API.GetChannelByName(team.Id, "town-square", false)
		if appErr != nil {
			p.API.LogError("Failed to get Town Square for team", "teamId", team.Id, "error", appErr.Error())
			continue
		}

		// Start a roll call in this channel
		_, err := p.rollCallManager.StartRollCall(channel.Id, bot.mmBot.UserId)
		if err != nil {
			p.API.LogError("Failed to start roll call", "channelId", channel.Id, "error", err.Error())
			continue
		}

		// Get the current date in Vietnam format
		vietTime, err := GetVietnamTime()
		if err != nil {
			p.API.LogError("Failed to get Vietnam time", "error", err.Error())
			vietTime = time.Now() // Fallback to server time
		}
		dateStr := vietTime.Format("Monday, January 2, 2006")

		// Create the announcement post
		post := &model.Post{
			ChannelId: channel.Id,
			UserId:    bot.mmBot.UserId,
			Message: fmt.Sprintf(
				"# 📋 Daily Roll Call - %s\n\n"+
					"Good morning team! Please respond with **'present'** to mark your attendance for today.\n\n"+
					"* Your attendance will be automatically recorded in the ERP system\n"+
					"* Automatic checkout will be recorded at %s\n"+
					"* If you're working remotely, please add a note with your location",
				dateStr,
				AutoCheckoutTime,
			),
		}

		// Post the message
		if _, appErr := p.API.CreatePost(post); appErr != nil {
			p.API.LogError("Failed to create roll call post", "channelId", channel.Id, "error", appErr.Error())
			continue
		}

		p.API.LogInfo("Started automatic roll call", "channelId", channel.Id, "date", dateStr)
	}
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
