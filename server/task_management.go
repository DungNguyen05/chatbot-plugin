// Copyright (c) 2023-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package main

import (
	"database/sql"
	"fmt"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/google/uuid"
)

type TaskStatus string

const (
	TaskStatusOpen     TaskStatus = "open"
	TaskStatusComplete TaskStatus = "complete"
	TaskStatusOverdue  TaskStatus = "overdue"
)

type Task struct {
	ID          string     `json:"id" db:"ID"`
	Title       string     `json:"title" db:"Title"`
	Description string     `json:"description" db:"Description"`
	AssigneeID  string     `json:"assignee_id" db:"AssigneeID"`
	CreatorID   string     `json:"creator_id" db:"CreatorID"`
	ChannelID   string     `json:"channel_id" db:"ChannelID"`
	Deadline    int64      `json:"deadline" db:"Deadline"`
	Status      TaskStatus `json:"status" db:"Status"`
	CreatedAt   int64      `json:"created_at" db:"CreatedAt"`
	UpdatedAt   int64      `json:"updated_at" db:"UpdatedAt"`
}

type RollCallStatus string

const (
	RollCallStatusActive RollCallStatus = "active"
	RollCallStatusClosed RollCallStatus = "closed"
)

type RollCall struct {
	ID        string         `json:"id" db:"ID"`
	ChannelID string         `json:"channel_id" db:"ChannelID"`
	CreatorID string         `json:"creator_id" db:"CreatorID"`
	Title     string         `json:"title" db:"Title"`
	Status    RollCallStatus `json:"status" db:"Status"`
	CreatedAt int64          `json:"created_at" db:"CreatedAt"`
	EndedAt   sql.NullInt64  `json:"ended_at" db:"EndedAt"`
}

type RollCallResponse struct {
	RollCallID   string `json:"roll_call_id" db:"RollCallID"`
	UserID       string `json:"user_id" db:"UserID"`
	Response     string `json:"response" db:"Response"`
	ResponseTime int64  `json:"response_time" db:"ResponseTime"`
}

// Creates a new task for a user
func (p *Plugin) CreateTask(title, description, assigneeID, creatorID, channelID string, deadline int64) (*Task, error) {
	task := &Task{
		ID:          uuid.New().String(),
		Title:       title,
		Description: description,
		AssigneeID:  assigneeID,
		CreatorID:   creatorID,
		ChannelID:   channelID,
		Deadline:    deadline,
		Status:      TaskStatusOpen,
		CreatedAt:   time.Now().UnixMilli(),
		UpdatedAt:   time.Now().UnixMilli(),
	}

	_, err := p.execBuilder(p.builder.Insert("LLM_Tasks").
		Columns("ID", "Title", "Description", "AssigneeID", "CreatorID", "ChannelID", "Deadline", "Status", "CreatedAt", "UpdatedAt").
		Values(task.ID, task.Title, task.Description, task.AssigneeID, task.CreatorID, task.ChannelID, task.Deadline, task.Status, task.CreatedAt, task.UpdatedAt))

	if err != nil {
		return nil, fmt.Errorf("failed to create task: %w", err)
	}

	return task, nil
}

// Gets tasks assigned to a user
func (p *Plugin) GetTasksForUser(userID string) ([]*Task, error) {
	var tasks []*Task

	err := p.doQuery(&tasks, p.builder.
		Select("*").
		From("LLM_Tasks").
		Where(sq.Eq{"AssigneeID": userID}).
		Where(sq.Eq{"Status": TaskStatusOpen}).
		OrderBy("Deadline ASC"))

	if err != nil {
		return nil, fmt.Errorf("failed to get tasks for user: %w", err)
	}

	return tasks, nil
}

// Updates task status
func (p *Plugin) UpdateTaskStatus(taskID string, status TaskStatus) error {
	_, err := p.execBuilder(p.builder.Update("LLM_Tasks").
		Set("Status", status).
		Set("UpdatedAt", time.Now().UnixMilli()).
		Where(sq.Eq{"ID": taskID}))

	if err != nil {
		return fmt.Errorf("failed to update task status: %w", err)
	}

	return nil
}

// Creates a new roll call
func (p *Plugin) CreateRollCall(channelID, creatorID, title string) (*RollCall, error) {
	rollCall := &RollCall{
		ID:        uuid.New().String(),
		ChannelID: channelID,
		CreatorID: creatorID,
		Title:     title,
		Status:    RollCallStatusActive,
		CreatedAt: time.Now().UnixMilli(),
	}

	_, err := p.execBuilder(p.builder.Insert("LLM_RollCalls").
		Columns("ID", "ChannelID", "CreatorID", "Title", "Status", "CreatedAt").
		Values(rollCall.ID, rollCall.ChannelID, rollCall.CreatorID, rollCall.Title, rollCall.Status, rollCall.CreatedAt))

	if err != nil {
		return nil, fmt.Errorf("failed to create roll call: %w", err)
	}

	return rollCall, nil
}

// Records a response to a roll call
func (p *Plugin) RecordRollCallResponse(rollCallID, userID, response string) error {
	_, err := p.execBuilder(p.builder.Insert("LLM_RollCallResponses").
		Columns("RollCallID", "UserID", "Response", "ResponseTime").
		Values(rollCallID, userID, response, time.Now().UnixMilli()))

	if err != nil {
		return fmt.Errorf("failed to record roll call response: %w", err)
	}

	return nil
}

// Gets active roll call for a channel
func (p *Plugin) GetActiveRollCall(channelID string) (*RollCall, error) {
	var rollCalls []*RollCall

	err := p.doQuery(&rollCalls, p.builder.
		Select("*").
		From("LLM_RollCalls").
		Where(sq.Eq{"ChannelID": channelID, "Status": RollCallStatusActive}).
		OrderBy("CreatedAt DESC").
		Limit(1))

	if err != nil {
		return nil, fmt.Errorf("failed to get active roll call: %w", err)
	}

	if len(rollCalls) == 0 {
		return nil, nil
	}

	return rollCalls[0], nil
}

// Ends a roll call
func (p *Plugin) EndRollCall(rollCallID string) error {
	_, err := p.execBuilder(p.builder.Update("LLM_RollCalls").
		Set("Status", RollCallStatusClosed).
		Set("EndedAt", time.Now().UnixMilli()).
		Where(sq.Eq{"ID": rollCallID}))

	if err != nil {
		return fmt.Errorf("failed to end roll call: %w", err)
	}

	return nil
}

// Get roll call summary
func (p *Plugin) GetRollCallSummary(rollCallID string) (map[string]*RollCallResponse, error) {
	var responses []*RollCallResponse

	err := p.doQuery(&responses, p.builder.
		Select("*").
		From("LLM_RollCallResponses").
		Where(sq.Eq{"RollCallID": rollCallID}))

	if err != nil {
		return nil, fmt.Errorf("failed to get roll call responses: %w", err)
	}

	result := make(map[string]*RollCallResponse)
	for _, resp := range responses {
		result[resp.UserID] = resp
	}

	return result, nil
}
