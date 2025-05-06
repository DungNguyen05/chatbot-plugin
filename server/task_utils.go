// Copyright (c) 2023-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/mattermost/mattermost/server/public/model"
)

// parseHumanReadableDate parses a human-readable date string into a time.Time
func parseHumanReadableDate(dateStr string) (time.Time, error) {
	// Try to parse exact date format first
	t, err := time.Parse("2006-01-02", dateStr)
	if err == nil {
		// Set the time to end of day
		return time.Date(t.Year(), t.Month(), t.Day(), 23, 59, 59, 0, time.Local), nil
	}

	// Parse relative dates
	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 23, 59, 59, 0, time.Local)

	dateStr = strings.ToLower(dateStr)

	switch {
	case strings.Contains(dateStr, "today"):
		return today, nil
	case strings.Contains(dateStr, "tomorrow"):
		return today.AddDate(0, 0, 1), nil
	case strings.Contains(dateStr, "next week"):
		return today.AddDate(0, 0, 7), nil
	case strings.Contains(dateStr, "next month"):
		return today.AddDate(0, 1, 0), nil
	}

	// Check for "in X days/weeks/months"
	if strings.HasPrefix(dateStr, "in ") {
		parts := strings.Split(dateStr, " ")
		if len(parts) >= 3 {
			num := 0
			fmt.Sscanf(parts[1], "%d", &num)
			if num > 0 {
				unit := parts[2]
				switch {
				case strings.HasPrefix(unit, "day"):
					return today.AddDate(0, 0, num), nil
				case strings.HasPrefix(unit, "week"):
					return today.AddDate(0, 0, 7*num), nil
				case strings.HasPrefix(unit, "month"):
					return today.AddDate(0, num, 0), nil
				}
			}
		}
	}

	// Default to tomorrow if we can't parse
	return today.AddDate(0, 0, 1), fmt.Errorf("could not parse date: %s", dateStr)
}

// sendTaskNotification sends a DM to the assignee about a new task
func (p *Plugin) sendTaskNotification(task *Task, assignee *model.User) error {
	creator, err := p.pluginAPI.User.Get(task.CreatorID)
	if err != nil {
		return err
	}

	channel, err := p.pluginAPI.Channel.Get(task.ChannelID)
	if err != nil {
		return err
	}

	deadline := time.UnixMilli(task.Deadline).Format("January 2, 2006")

	message := fmt.Sprintf("You have been assigned a new task by @%s:\n\n**%s**\n\n%s\n\nDeadline: %s\nChannel: %s\nTask ID: `%s`\n\nReply with 'mark task %s complete' when you've finished.",
		creator.Username,
		task.Title,
		task.Description,
		deadline,
		channel.DisplayName,
		task.ID,
		task.ID)

	bot := p.GetBotByUsernameOrFirst(p.getConfiguration().DefaultBotName)
	if bot == nil {
		return fmt.Errorf("could not find bot")
	}

	post := &model.Post{
		Message: message,
	}

	return p.botDMNonResponse(bot.mmBot.UserId, assignee.Id, post)
}

// postRollCallAnnouncement posts an announcement about a new roll call
func (p *Plugin) postRollCallAnnouncement(rollCall *RollCall, channel *model.Channel) error {
	creator, err := p.pluginAPI.User.Get(rollCall.CreatorID)
	if err != nil {
		return err
	}

	message := fmt.Sprintf("@channel **Roll Call: %s**\n\nStarted by @%s\n\nPlease respond with '@%s I'm here' or similar to be counted.",
		rollCall.Title,
		creator.Username,
		p.getConfiguration().DefaultBotName)

	bot := p.GetBotByUsernameOrFirst(p.getConfiguration().DefaultBotName)
	if bot == nil {
		return fmt.Errorf("could not find bot")
	}

	post := &model.Post{
		ChannelId: channel.Id,
		Message:   message,
	}

	if err := p.pluginAPI.Post.CreatePost(post); err != nil {
		return err
	}

	return nil
}

// formatRollCallSummary formats a summary of roll call responses
func (p *Plugin) formatRollCallSummary(rollCall *RollCall) (string, error) {
	responses, err := p.GetRollCallSummary(rollCall.ID)
	if err != nil {
		return "", err
	}

	// Get channel members to find who didn't respond
	channelMembers, err := p.API.GetChannelMembers(rollCall.ChannelID, 0, 1000)

	if err != nil {
		return "", err
	}

	responded := make(map[string]string)
	for userID, resp := range responses {
		user, err := p.pluginAPI.User.Get(userID)
		if err != nil {
			continue
		}
		responded[userID] = fmt.Sprintf("@%s: %s", user.Username, resp.Response)
	}

	notResponded := []string{}
	for _, member := range channelMembers {
		// Skip bots
		user, err := p.pluginAPI.User.Get(member.UserId)
		if err != nil || user.IsBot {
			continue
		}

		if _, ok := responses[member.UserId]; !ok {
			notResponded = append(notResponded, fmt.Sprintf("@%s", user.Username))
		}
	}

	// Build summary
	summary := fmt.Sprintf("**Roll Call Summary: %s**\n\n", rollCall.Title)

	if len(responded) > 0 {
		summary += "**Responded:**\n"
		for _, response := range responded {
			summary += "- " + response + "\n"
		}
		summary += "\n"
	}

	if len(notResponded) > 0 {
		summary += "**No Response:**\n"
		for _, username := range notResponded {
			summary += "- " + username + "\n"
		}
	}

	return summary, nil
}
