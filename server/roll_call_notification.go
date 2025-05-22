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

// sendRollCallNotification sends a notification to configured channels
// instead of all bot-allowed channels
func (p *Plugin) sendRollCallNotification(userID, employeeName string, eventType RollCallEventType, eventTime string, reason string) error {
	p.API.LogDebug("Sending roll call notification",
		"user_id", userID,
		"event_type", string(eventType),
		"time", eventTime)

	config := p.getConfiguration()

	// Check if roll call is enabled
	if !config.RollCall.Enabled {
		p.API.LogDebug("Roll call is disabled, skipping notification")
		return nil
	}

	// Get configured notification channels
	notifyChannelIDs := config.RollCall.NotifyChannels
	if len(notifyChannelIDs) == 0 {
		p.API.LogDebug("No notification channels configured for roll call")
		return nil
	}

	// Find bot to use for notifications
	p.botsLock.RLock()
	defer p.botsLock.RUnlock()

	if len(p.bots) == 0 {
		return fmt.Errorf("no bots available")
	}

	// Use the first bot for notifications
	bot := p.bots[0]

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

	// Send to configured notification channels only
	for _, channelID := range notifyChannelIDs {
		post := &model.Post{
			UserId:    bot.mmBot.UserId,
			ChannelId: channelID,
			Message:   message,
		}

		if err := p.pluginAPI.Post.CreatePost(post); err != nil {
			p.API.LogError("Failed to send roll call notification",
				"channel_id", channelID,
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
