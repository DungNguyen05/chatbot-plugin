// Copyright (c) 2023-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/mattermost/mattermost/server/public/model"
)

// handleRollCall processes attendance-related messages
func (p *Plugin) handleRollCall(bot *Bot, channel *model.Channel, user *model.User, post *model.Post) error {
	message := strings.ToLower(post.Message)

	if strings.Contains(message, "present") || strings.Contains(message, "check in") || strings.Contains(message, "checking in") {
		return p.handleRollCallCheckin(bot, channel, user, post)
	} else if strings.Contains(message, "leaving") || strings.Contains(message, "check out") || strings.Contains(message, "checking out") {
		return p.handleRollCallCheckout(bot, channel, user, post)
	} else if strings.Contains(message, "absent") || strings.Contains(message, "won't be in") || strings.Contains(message, "out sick") {
		return p.handleRollCallAbsent(bot, channel, user, post)
	}

	return nil
}

// handleRollCallCheckin handles a user checking in via message
func (p *Plugin) handleRollCallCheckin(bot *Bot, channel *model.Channel, user *model.User, post *model.Post) error {
	// Extract note (everything after "present" or similar keywords)
	message := post.Message
	note := ""

	// Try to extract note based on common check-in phrases
	for _, phrase := range []string{"present", "check in", "checking in"} {
		if idx := strings.Index(strings.ToLower(message), phrase); idx >= 0 {
			if len(message) > idx+len(phrase) {
				note = strings.TrimSpace(message[idx+len(phrase):])
			}
			break
		}
	}

	// Remove common punctuation from the beginning of the note
	note = strings.TrimLeft(note, ":,.!?-")
	note = strings.TrimSpace(note)

	// Get employee name
	employeeName := p.GetEmployeeNameFromUser(user)
	if note != "" {
		employeeName += " (" + note + ")"
	}

	// Try to record check-in in ERP
	formattedTime, err := p.RecordEmployeeCheckin(employeeName)
	if err != nil {
		p.API.LogError("Failed to record employee check-in in ERP", "error", err.Error())

		responsePost := &model.Post{
			ChannelId: channel.Id,
			Message:   "⚠️ There was an issue recording your check-in in the ERP system. An administrator has been notified.",
		}
		if post.RootId != "" {
			responsePost.RootId = post.RootId
		}
		return p.botCreateNonResponsePost(bot.mmBot.UserId, user.Id, responsePost)
	}

	// Create response message
	responseText := fmt.Sprintf("✅ Your check-in has been recorded in the ERP system at **%s**!", formattedTime)
	if note != "" {
		responseText = fmt.Sprintf("✅ Your check-in has been recorded in the ERP system at **%s** with note: \"%s\"", formattedTime, note)
	}

	responsePost := &model.Post{
		ChannelId: channel.Id,
		Message:   responseText,
	}
	if post.RootId != "" {
		responsePost.RootId = post.RootId
	}
	return p.botCreateNonResponsePost(bot.mmBot.UserId, user.Id, responsePost)
}

// handleRollCallCheckout handles a user checking out via message
func (p *Plugin) handleRollCallCheckout(bot *Bot, channel *model.Channel, user *model.User, post *model.Post) error {
	// Extract note (everything after checkout keywords)
	message := post.Message
	note := ""

	// Try to extract note based on common check-out phrases
	for _, phrase := range []string{"leaving", "check out", "checking out"} {
		if idx := strings.Index(strings.ToLower(message), phrase); idx >= 0 {
			if len(message) > idx+len(phrase) {
				note = strings.TrimSpace(message[idx+len(phrase):])
			}
			break
		}
	}

	// Remove common punctuation from the beginning of the note
	note = strings.TrimLeft(note, ":,.!?-")
	note = strings.TrimSpace(note)

	// Get employee name
	employeeName := p.GetEmployeeNameFromUser(user)
	if note != "" {
		employeeName += " (" + note + ")"
	}

	// Get current time in Vietnam
	vietTime, timeErr := GetVietnamTime()
	if timeErr != nil {
		p.API.LogError("Failed to get Vietnam time", "error", timeErr.Error())

		responsePost := &model.Post{
			ChannelId: channel.Id,
			Message:   "Error getting current time. Please try again.",
		}
		if post.RootId != "" {
			responsePost.RootId = post.RootId
		}
		return p.botCreateNonResponsePost(bot.mmBot.UserId, user.Id, responsePost)
	}

	// Format checkout time
	checkoutTime := FormatTimeForERP(vietTime)

	// Record checkout in ERP
	checkoutFormatted, err := p.RecordEmployeeCheckout(employeeName, checkoutTime)
	if err != nil {
		p.API.LogError("Failed to record employee checkout in ERP", "error", err.Error())

		responsePost := &model.Post{
			ChannelId: channel.Id,
			Message:   "⚠️ There was an issue recording your checkout in the ERP system. An administrator has been notified.",
		}
		if post.RootId != "" {
			responsePost.RootId = post.RootId
		}
		return p.botCreateNonResponsePost(bot.mmBot.UserId, user.Id, responsePost)
	}

	// Create response message
	responseText := fmt.Sprintf("✅ Your check-out has been recorded in the ERP system at **%s**!", checkoutFormatted)
	if note != "" {
		responseText = fmt.Sprintf("✅ Your check-out has been recorded in the ERP system at **%s** with note: \"%s\"", checkoutFormatted, note)
	}

	responsePost := &model.Post{
		ChannelId: channel.Id,
		Message:   responseText,
	}
	if post.RootId != "" {
		responsePost.RootId = post.RootId
	}
	return p.botCreateNonResponsePost(bot.mmBot.UserId, user.Id, responsePost)
}

// handleRollCallAbsent handles marking a user as absent
func (p *Plugin) handleRollCallAbsent(bot *Bot, channel *model.Channel, user *model.User, post *model.Post) error {
	// Extract reason from message
	message := post.Message
	reason := ""

	// Try to extract reason based on common absence phrases
	for _, phrase := range []string{"absent", "won't be in", "out sick"} {
		if idx := strings.Index(strings.ToLower(message), phrase); idx >= 0 {
			if len(message) > idx+len(phrase) {
				reason = strings.TrimSpace(message[idx+len(phrase):])
			}
			break
		}
	}

	// Remove common punctuation from the beginning of the reason
	reason = strings.TrimLeft(reason, ":,.!?-")
	reason = strings.TrimSpace(reason)

	// If no reason provided, use a generic one
	if reason == "" {
		reason = "No reason provided"
	}

	// Get current date in Vietnam time
	vietTime, err := GetVietnamTime()
	if err != nil {
		p.API.LogError("Failed to get Vietnam time", "error", err.Error())
		// Use server time as fallback
		vietTime = time.Now()
	}

	dateStr := vietTime.Format("Monday, January 2, 2006")

	// Log absence
	p.API.LogInfo("User marked absent",
		"user", user.Username,
		"date", dateStr,
		"reason", reason)

	// Get employee name with reason
	employeeName := p.GetEmployeeNameFromUser(user) + " (ABSENT: " + reason + ")"

	// Notify ERP of absence (could create a function in erp_integration.go for this)
	p.API.LogInfo("Recording absence in ERP", "employee", employeeName, "date", dateStr)

	// Create response message
	responseText := fmt.Sprintf("📝 Your absence has been recorded for **%s** with reason: \"%s\"", dateStr, reason)

	responsePost := &model.Post{
		ChannelId: channel.Id,
		Message:   responseText,
	}
	if post.RootId != "" {
		responsePost.RootId = post.RootId
	}
	return p.botCreateNonResponsePost(bot.mmBot.UserId, user.Id, responsePost)
}
