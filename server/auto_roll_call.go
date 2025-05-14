// Copyright (c) 2023-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package main

import (
	"fmt"

	"github.com/mattermost/mattermost/server/public/model"
)

const (
	DefaultAutoCheckoutTime = "17:30:00" // Default checkout time if not configured
)

// AutoRecordCheckouts attempts to record checkouts for users who haven't checked out yet
func (p *Plugin) AutoRecordCheckouts() {
	// Get the bot
	bot := p.GetBotByUsernameOrFirst(p.getConfiguration().DefaultBotName)
	if bot == nil {
		p.API.LogError("Failed to get bot for automatic checkout")
		return
	}

	// Get configured auto checkout time
	autoCheckoutTime := p.getConfiguration().AutoCheckoutTime
	if autoCheckoutTime == "" {
		autoCheckoutTime = DefaultAutoCheckoutTime
	}

	p.API.LogInfo("Auto checkout process started", "time", autoCheckoutTime)

	// In a stateless system, we can't know who checked in but didn't check out
	// This is a limitation of removing the roll call state tracking

	// An alternative would be to log a message in configured channels
	// about automatic checkout

	teams, appErr := p.API.GetTeams()
	if appErr != nil {
		p.API.LogError("Failed to get teams for automatic checkout announcement", "error", appErr.Error())
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
				// Create the announcement post
				post := &model.Post{
					ChannelId: channel.Id,
					UserId:    bot.mmBot.UserId,
					Message: fmt.Sprintf(
						"# 🕒 Automatic Checkout Time: %s\n\n"+
							"If you haven't checked out yet, please use `/checkout` to record your departure time.",
						autoCheckoutTime,
					),
				}

				// Post the message
				if _, appErr := p.API.CreatePost(post); appErr != nil {
					p.API.LogError("Failed to create auto checkout announcement", "channelId", channel.Id, "error", appErr.Error())
					continue
				}

				p.API.LogInfo("Sent auto checkout announcement", "channelId", channel.Id)
			}
		}
	}
}
