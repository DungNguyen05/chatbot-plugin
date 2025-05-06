// Copyright (c) 2023-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package main

import (
	"fmt"
	"net/http"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/gin-gonic/gin"
)

func (p *Plugin) handleGetUserTasks(c *gin.Context) {
	userID := c.Param("userid")

	tasks, err := p.GetTasksForUser(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, tasks)
}

func (p *Plugin) handleCreateTask(c *gin.Context) {
	var req struct {
		Title       string `json:"title" binding:"required"`
		Description string `json:"description"`
		AssigneeID  string `json:"assignee_id" binding:"required"`
		ChannelID   string `json:"channel_id" binding:"required"`
		Deadline    string `json:"deadline"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	creatorID := c.GetHeader("Mattermost-User-Id")

	// Parse deadline
	deadline := time.Now().Add(24 * time.Hour)
	if req.Deadline != "" {
		parsedDeadline, err := parseHumanReadableDate(req.Deadline)
		if err == nil {
			deadline = parsedDeadline
		}
	}

	task, err := p.CreateTask(req.Title, req.Description, req.AssigneeID, creatorID, req.ChannelID, deadline.UnixMilli())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Notify the assignee
	assignee, _ := p.pluginAPI.User.Get(req.AssigneeID)
	if assignee != nil {
		_ = p.sendTaskNotification(task, assignee)
	}

	c.JSON(http.StatusOK, task)
}

func (p *Plugin) handleUpdateTaskStatus(c *gin.Context) {
	taskID := c.Param("taskid")

	var req struct {
		Status string `json:"status" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var status TaskStatus
	switch req.Status {
	case "complete":
		status = TaskStatusComplete
	case "open":
		status = TaskStatusOpen
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid status. Use 'open' or 'complete'."})
		return
	}

	err := p.UpdateTaskStatus(taskID, status)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.Status(http.StatusOK)
}

func (p *Plugin) handleGetActiveRollCall(c *gin.Context) {
	channelID := c.Param("channelid")

	rollCall, err := p.GetActiveRollCall(channelID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if rollCall == nil {
		c.Status(http.StatusNotFound)
		return
	}

	c.JSON(http.StatusOK, rollCall)
}

func (p *Plugin) handleStartRollCall(c *gin.Context) {
	var req struct {
		ChannelID string `json:"channel_id" binding:"required"`
		Title     string `json:"title" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	creatorID := c.GetHeader("Mattermost-User-Id")

	// Check if there's already an active roll call
	existingRollCall, err := p.GetActiveRollCall(req.ChannelID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to check for active roll calls"})
		return
	}

	if existingRollCall != nil {
		c.JSON(http.StatusConflict, gin.H{"error": "There is already an active roll call in this channel"})
		return
	}

	// Create roll call
	rollCall, err := p.CreateRollCall(req.ChannelID, creatorID, req.Title)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Post a message to the channel about the roll call
	channel, _ := p.pluginAPI.Channel.Get(req.ChannelID)
	if channel != nil {
		_ = p.postRollCallAnnouncement(rollCall, channel)
	}

	c.JSON(http.StatusOK, rollCall)
}

func (p *Plugin) handleRespondToRollCall(c *gin.Context) {
	rollCallID := c.Param("rollcallid")
	userID := c.GetHeader("Mattermost-User-Id")

	var req struct {
		Response string `json:"response" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	err := p.RecordRollCallResponse(rollCallID, userID, req.Response)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.Status(http.StatusOK)
}

func (p *Plugin) handleEndRollCall(c *gin.Context) {
	rollCallID := c.Param("rollcallid")

	var req struct {
		ShowSummary bool `json:"show_summary"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		// Default to not showing summary if request is malformed
		req.ShowSummary = false
	}

	err := p.EndRollCall(rollCallID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if req.ShowSummary {
		rollCall, err := p.getRollCall(rollCallID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Roll call ended but failed to get roll call details"})
			return
		}

		summary, err := p.formatRollCallSummary(rollCall)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Roll call ended but failed to generate summary"})
			return
		}

		c.JSON(http.StatusOK, gin.H{"summary": summary})
		return
	}

	c.Status(http.StatusOK)
}

func (p *Plugin) handleGenerateRollup(c *gin.Context) {
	userID := c.GetHeader("Mattermost-User-Id")

	rollupType := c.DefaultQuery("type", string(RollupTypeDaily))
	channelID := c.Query("channel_id")

	validType := rollupType == string(RollupTypeDaily) || rollupType == string(RollupTypeWeekly)
	if !validType {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid rollup type. Use 'daily' or 'weekly'."})
		return
	}

	report, err := p.generateRollup(userID, channelID, RollupType(rollupType))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"report": report})
}

// Helper function to get a roll call by ID
func (p *Plugin) getRollCall(rollCallID string) (*RollCall, error) {
	var rollCalls []*RollCall

	err := p.doQuery(&rollCalls, p.builder.
		Select("*").
		From("LLM_RollCalls").
		Where(sq.Eq{"ID": rollCallID}))

	if err != nil {
		return nil, fmt.Errorf("failed to get roll call: %w", err)
	}

	if len(rollCalls) == 0 {
		return nil, fmt.Errorf("roll call not found")
	}

	return rollCalls[0], nil
}
