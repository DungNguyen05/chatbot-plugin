// Copyright (c) 2023-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package main

import (
	"bytes"
	"fmt"
	"html/template"
	"time"

	"github.com/mattermost/mattermost-plugin-ai/server/llm"
	"github.com/mattermost/mattermost/server/public/model"
)

// RollCallEventType defines the type of roll call event
type RollCallEventType string

const (
	// RollCallEventCheckIn represents a check-in event
	RollCallEventCheckIn RollCallEventType = "check_in"
	// RollCallEventCheckOut represents a check-out event
	RollCallEventCheckOut RollCallEventType = "check_out"
	// RollCallEventAbsent represents an absence notification
	RollCallEventAbsent RollCallEventType = "absent"
)

// sendRollCallNotification sends a notification to all bot-allowed public channels
// about a user checking in or checking out
func (p *Plugin) sendRollCallNotification(userID, employeeName string, eventType RollCallEventType, eventTime string, reason string) error {
	p.API.LogDebug("Sending roll call notification",
		"user_id", userID,
		"event_type", string(eventType),
		"time", eventTime)

	// Find bot-allowed public channels
	var channelsToNotify []*model.Channel

	// Get all bots
	p.botsLock.RLock()
	defer p.botsLock.RUnlock()

	if len(p.bots) == 0 {
		return fmt.Errorf("no bots available")
	}

	// Use the first bot for notifications
	bot := p.bots[0]

	// Determine which channels to send to based on channel access configuration
	switch bot.cfg.ChannelAccessLevel {
	case llm.ChannelAccessLevelAll:
		// Allow for all channels - get all channels the bot has access to
		teams, appErr := p.API.GetTeams()
		if appErr != nil {
			p.API.LogError("Failed to get teams for automatic notifications", "error", appErr.Error())
			return appErr
		}

		for _, team := range teams {
			channels, appErr := p.API.GetChannelsForTeamForUser(team.Id, bot.mmBot.UserId, false)
			if appErr != nil {
				p.API.LogError("Failed to get channels for team", "teamId", team.Id, "error", appErr.Error())
				continue
			}

			// Include both public and private channels (not DMs or GMs)
			for _, channel := range channels {
				if channel.Type == model.ChannelTypeOpen || channel.Type == model.ChannelTypePrivate {
					channelsToNotify = append(channelsToNotify, channel)
				}
			}
		}

	case llm.ChannelAccessLevelAllow:
		// Allow for selected channels only
		if len(bot.cfg.ChannelIDs) > 0 {
			// Only notify the explicitly allowed channels
			for _, channelID := range bot.cfg.ChannelIDs {
				channel, err := p.API.GetChannel(channelID)
				if err != nil {
					p.API.LogError("Failed to get channel", "channelId", channelID, "error", err.Error())
					continue
				}

				// Include both public and private channels
				if channel.Type == model.ChannelTypeOpen || channel.Type == model.ChannelTypePrivate {
					channelsToNotify = append(channelsToNotify, channel)
				}
			}
		} else {
			// If no channels are explicitly allowed, don't notify anywhere
			p.API.LogInfo("No channels are explicitly allowed for notifications")
		}

	case llm.ChannelAccessLevelBlock:
		// Block selected channels - get all channels except the blocked ones
		teams, appErr := p.API.GetTeams()
		if appErr != nil {
			p.API.LogError("Failed to get teams for automatic notifications", "error", appErr.Error())
			return appErr
		}

		blockedChannelIDs := make(map[string]bool)
		for _, channelID := range bot.cfg.ChannelIDs {
			blockedChannelIDs[channelID] = true
		}

		for _, team := range teams {
			channels, appErr := p.API.GetChannelsForTeamForUser(team.Id, bot.mmBot.UserId, false)
			if appErr != nil {
				p.API.LogError("Failed to get channels for team", "teamId", team.Id, "error", appErr.Error())
				continue
			}

			// Include both public and private channels (not DMs or GMs), excluding blocked ones
			for _, channel := range channels {
				if (channel.Type == model.ChannelTypeOpen || channel.Type == model.ChannelTypePrivate) && !blockedChannelIDs[channel.Id] {
					channelsToNotify = append(channelsToNotify, channel)
				}
			}
		}

	case llm.ChannelAccessLevelNone:
		// Block all channels - don't notify anywhere
		p.API.LogInfo("Channel access level is set to None, not sending notifications to any channels")
		return nil
	}

	// Get the user's details for more personalized messages
	user, err := p.pluginAPI.User.Get(userID)
	if err != nil {
		return err
	}

	// Create notification message
	var message string

	switch eventType {
	case RollCallEventCheckIn:
		message = fmt.Sprintf("**%s** has checked in at %s", employeeName, eventTime)
	case RollCallEventCheckOut:
		message = fmt.Sprintf("**%s** has checked out at %s", employeeName, eventTime)
	case RollCallEventAbsent:
		message = fmt.Sprintf("**%s** has reported absence for today: \"%s\"", employeeName, reason)
	}

	// Send to all allowed channels
	for _, channel := range channelsToNotify {
		post := &model.Post{
			UserId:    bot.mmBot.UserId,
			ChannelId: channel.Id,
			Message:   message,
		}

		if err := p.pluginAPI.Post.CreatePost(post); err != nil {
			p.API.LogError("Failed to send roll call notification",
				"channel_id", channel.Id,
				"error", err.Error())
		}
	}

	// Send personalized message to the user via LLM
	if err := p.sendPersonalizedRollCallMessage(bot, user, eventType, eventTime); err != nil {
		p.API.LogError("Failed to send personalized message",
			"user_id", userID,
			"error", err.Error())
	}

	return nil
}

// sendPersonalizedRollCallMessage sends a personalized message to the user using the LLM
func (p *Plugin) sendPersonalizedRollCallMessage(bot *Bot, user *model.User, eventType RollCallEventType, eventTime string) error {
	// Set up context for LLM
	context := p.BuildLLMContextUserRequest(
		bot,
		user,
		nil,
	)

	// Current time info for more context
	vietTime, err := GetVietnamTime()
	if err != nil {
		vietTime = time.Now()
	}

	timeOfDay := getTimeOfDay(vietTime)
	dayOfWeek := vietTime.Weekday().String()

	// Build parameters for LLM
	context.Parameters = map[string]any{
		"EventType":  string(eventType),
		"EventTime":  eventTime,
		"TimeOfDay":  timeOfDay,
		"DayOfWeek":  dayOfWeek,
		"UserName":   user.FirstName,
		"IsCheckIn":  eventType == RollCallEventCheckIn,
		"IsCheckOut": eventType == RollCallEventCheckOut,
	}

	// Define the prompt based on event type
	var promptText string

	switch eventType {
	case RollCallEventCheckIn:
		promptText = `You are a friendly workplace assistant. Generate a SHORT, MODERN, and ENERGETIC welcome message (1-2 sentences only) 
for {{.UserName}} who just checked in to work at {{.EventTime}}. 
It's currently {{.TimeOfDay}} on {{.DayOfWeek}}. 
Make it sound professional but friendly. DO NOT USE MORE THAN 2 SENTENCES.`

	case RollCallEventCheckOut:
		promptText = `You are a friendly workplace assistant. Generate a SHORT, MODERN, and FRIENDLY goodbye message (1-2 sentences only) 
for {{.UserName}} who just checked out from work at {{.EventTime}}. 
It's currently {{.TimeOfDay}} on {{.DayOfWeek}}. 
Wish them a pleasant time off. DO NOT USE MORE THAN 2 SENTENCES.`

	default:
		return fmt.Errorf("unsupported event type for personalized message")
	}

	// Replace template variables in the prompt
	processedPrompt, err := processTemplate(promptText, context.Parameters)
	if err != nil {
		return fmt.Errorf("failed to process template: %w", err)
	}

	// Use the LLM to generate the personalized message
	messageRequest := llm.CompletionRequest{
		Posts: []llm.Post{
			{
				Role:    llm.PostRoleSystem,
				Message: processedPrompt,
			},
		},
		Context: context,
	}

	result, err := p.getLLM(bot.cfg).ChatCompletionNoStream(messageRequest)
	if err != nil {
		return fmt.Errorf("failed to generate personalized message: %w", err)
	}

	// Send the message to the user
	post := &model.Post{
		Message: result,
	}

	if err := p.botDMNonResponse(bot.mmBot.UserId, user.Id, post); err != nil {
		return fmt.Errorf("failed to send personalized DM: %w", err)
	}

	return nil
}

func processTemplate(templateText string, data map[string]any) (string, error) {
	tmpl, err := template.New("prompt").Parse(templateText)
	if err != nil {
		return "", fmt.Errorf("failed to parse template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("failed to execute template: %w", err)
	}

	return buf.String(), nil
}

// getTimeOfDay returns a string describing the time of day
func getTimeOfDay(t time.Time) string {
	hour := t.Hour()

	switch {
	case hour >= 5 && hour < 12:
		return "morning"
	case hour >= 12 && hour < 17:
		return "afternoon"
	case hour >= 17 && hour < 21:
		return "evening"
	default:
		return "night"
	}
}
