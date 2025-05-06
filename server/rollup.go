// Copyright (c) 2023-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/mattermost/mattermost-plugin-ai/server/llm"

	sq "github.com/Masterminds/squirrel"
)

type RollupType string

const (
	RollupTypeDaily  RollupType = "daily"
	RollupTypeWeekly RollupType = "weekly"
)

type RollupArgs struct {
	Type      string `jsonschema_description:"The type of rollup to generate (daily, weekly)"`
	ChannelID string `jsonschema_description:"Optional channel ID to limit the rollup to a specific channel"`
}

// Task rollup tool
func (p *Plugin) toolResolveGenerateRollup(context *llm.Context, argsGetter llm.ToolArgumentGetter) (string, error) {
	var args RollupArgs
	err := argsGetter(&args)
	if err != nil {
		return "Invalid parameters to function", fmt.Errorf("failed to get arguments for tool GenerateRollup: %w", err)
	}

	rollupType := RollupTypeDaily
	if args.Type == string(RollupTypeWeekly) {
		rollupType = RollupTypeWeekly
	}

	channelID := ""
	if args.ChannelID != "" {
		// Validate channel
		_, err := p.pluginAPI.Channel.Get(args.ChannelID)
		if err != nil {
			return "Invalid channel ID", nil
		}
		channelID = args.ChannelID
	} else if context.Channel != nil {
		channelID = context.Channel.Id
	}

	// Generate rollup
	rollup, err := p.generateRollup(context.RequestingUser.Id, channelID, rollupType)
	if err != nil {
		return "Failed to generate rollup", err
	}

	return rollup, nil
}

// Generate a rollup report for tasks and activities
func (p *Plugin) generateRollup(userID, channelID string, rollupType RollupType) (string, error) {
	var startTime time.Time
	now := time.Now()

	switch rollupType {
	case RollupTypeDaily:
		startTime = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local).AddDate(0, 0, -1)
	case RollupTypeWeekly:
		// Get start of the week (assuming week starts on Monday)
		daysSinceMonday := int(now.Weekday())
		if daysSinceMonday == 0 { // Sunday
			daysSinceMonday = 7
		}
		startTime = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local).AddDate(0, 0, -7)
	default:
		return "", fmt.Errorf("invalid rollup type: %s", rollupType)
	}

	// Build rollup report
	var report strings.Builder

	switch rollupType {
	case RollupTypeDaily:
		report.WriteString("# Daily Roll-up Report\n\n")
		report.WriteString(fmt.Sprintf("**Date**: %s\n\n", now.Format("Monday, January 2, 2006")))
	case RollupTypeWeekly:
		report.WriteString("# Weekly Roll-up Report\n\n")
		report.WriteString(fmt.Sprintf("**Week of**: %s to %s\n\n",
			startTime.Format("January 2"),
			now.Format("January 2, 2006")))
	}

	// Get tasks for user
	tasks, err := p.GetUserTasksForReport(userID, startTime.UnixMilli(), channelID)
	if err != nil {
		return "", fmt.Errorf("failed to get tasks: %w", err)
	}

	// Add tasks section
	report.WriteString("## Tasks\n\n")

	completedTasks := []*Task{}
	openTasks := []*Task{}
	overdueTasks := []*Task{}

	for _, task := range tasks {
		switch task.Status {
		case TaskStatusComplete:
			completedTasks = append(completedTasks, task)
		case TaskStatusOpen:
			if task.Deadline < now.UnixMilli() {
				overdueTasks = append(overdueTasks, task)
			} else {
				openTasks = append(openTasks, task)
			}
		}
	}

	// Completed tasks
	report.WriteString("### Completed Tasks\n\n")
	if len(completedTasks) > 0 {
		for _, task := range completedTasks {
			deadline := time.UnixMilli(task.Deadline).Format("Jan 2")
			report.WriteString(fmt.Sprintf("- [x] **%s** (Due: %s)\n", task.Title, deadline))
		}
	} else {
		report.WriteString("No tasks completed during this period.\n")
	}
	report.WriteString("\n")

	// Open tasks
	report.WriteString("### Open Tasks\n\n")
	if len(openTasks) > 0 {
		for _, task := range openTasks {
			deadline := time.UnixMilli(task.Deadline).Format("Jan 2")
			report.WriteString(fmt.Sprintf("- [ ] **%s** (Due: %s)\n", task.Title, deadline))
		}
	} else {
		report.WriteString("No open tasks during this period.\n")
	}
	report.WriteString("\n")

	// Overdue tasks
	if len(overdueTasks) > 0 {
		report.WriteString("### Overdue Tasks\n\n")
		for _, task := range overdueTasks {
			deadline := time.UnixMilli(task.Deadline).Format("Jan 2")
			report.WriteString(fmt.Sprintf("- [ ] **%s** (Due: %s) :warning:\n", task.Title, deadline))
		}
		report.WriteString("\n")
	}

	// Get roll calls if looking at a specific channel
	if channelID != "" {
		rollCalls, err := p.GetRollCallsForReport(channelID, startTime.UnixMilli())
		if err != nil {
			return "", fmt.Errorf("failed to get roll calls: %w", err)
		}

		if len(rollCalls) > 0 {
			report.WriteString("## Roll Calls\n\n")
			for _, rollCall := range rollCalls {
				report.WriteString(fmt.Sprintf("### %s\n\n", rollCall.Title))
				report.WriteString(fmt.Sprintf("**Date**: %s\n\n", time.UnixMilli(rollCall.CreatedAt).Format("January 2, 2006 15:04")))

				responses, err := p.GetRollCallSummary(rollCall.ID)
				if err != nil {
					continue
				}

				totalResponses := len(responses)

				report.WriteString(fmt.Sprintf("**Total Responses**: %d\n\n", totalResponses))
			}
		}
	}

	return report.String(), nil
}

// Get tasks for a user during a specific time period, optionally filtered by channel
func (p *Plugin) GetUserTasksForReport(userID string, startTime int64, channelID string) ([]*Task, error) {
	var tasks []*Task

	query := p.builder.
		Select("*").
		From("LLM_Tasks").
		Where(sq.Or{
			sq.Eq{"AssigneeID": userID},
			sq.Eq{"CreatorID": userID},
		}).
		Where(sq.GtOrEq{"UpdatedAt": startTime})

	if channelID != "" {
		query = query.Where(sq.Eq{"ChannelID": channelID})
	}

	err := p.doQuery(&tasks, query)

	if err != nil {
		return nil, fmt.Errorf("failed to get tasks for report: %w", err)
	}

	return tasks, nil
}

// Get roll calls for a channel during a specific time period
func (p *Plugin) GetRollCallsForReport(channelID string, startTime int64) ([]*RollCall, error) {
	var rollCalls []*RollCall

	err := p.doQuery(&rollCalls, p.builder.
		Select("*").
		From("LLM_RollCalls").
		Where(sq.Eq{"ChannelID": channelID}).
		Where(sq.GtOrEq{"CreatedAt": startTime}).
		OrderBy("CreatedAt DESC"))

	if err != nil {
		return nil, fmt.Errorf("failed to get roll calls for report: %w", err)
	}

	return rollCalls, nil
}
