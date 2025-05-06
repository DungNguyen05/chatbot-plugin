// Copyright (c) 2023-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package main

import (
	"database/sql"
	"fmt"

	sq "github.com/Masterminds/squirrel"
	"github.com/jmoiron/sqlx"
)

type builder interface {
	ToSql() (string, []interface{}, error)
}

func (p *Plugin) SetupDB() error {
	// Get database connection
	origDB, err := p.pluginAPI.Store.GetMasterDB()
	if err != nil {
		return err
	}
	p.db = sqlx.NewDb(origDB, p.pluginAPI.Store.DriverName())

	// Use the appropriate placeholder format - MySQL uses question marks
	builder := sq.StatementBuilder.PlaceholderFormat(sq.Question)
	p.builder = builder

	return p.SetupTables()
}

func (p *Plugin) doQuery(dest interface{}, b builder) error {
	sqlString, args, err := b.ToSql()
	if err != nil {
		return fmt.Errorf("failed to build sql: %w", err)
	}

	sqlString = p.db.Rebind(sqlString)

	return sqlx.Select(p.db, dest, sqlString, args...)
}

func (p *Plugin) execBuilder(b builder) (sql.Result, error) {
	sqlString, args, err := b.ToSql()
	if err != nil {
		return nil, fmt.Errorf("failed to build sql: %w", err)
	}

	sqlString = p.db.Rebind(sqlString)

	return p.db.Exec(sqlString, args...)
}

func (p *Plugin) SetupTables() error {
	// MySQL version of the table
	query := `
		CREATE TABLE IF NOT EXISTS LLM_PostMeta (
			RootPostID VARCHAR(26) NOT NULL PRIMARY KEY,
			Title TEXT NOT NULL,
			CONSTRAINT FK_LLM_PostMeta_Posts FOREIGN KEY (RootPostID) REFERENCES Posts(Id) ON DELETE CASCADE
		);
	`

	if _, err := p.db.Exec(query); err != nil {
		return fmt.Errorf("can't create llm titles table: %w", err)
	}

	// Add new tables for task management
	taskQuery := `
        CREATE TABLE IF NOT EXISTS LLM_Tasks (
            ID VARCHAR(36) NOT NULL PRIMARY KEY,
            Title TEXT NOT NULL,
            Description TEXT,
            AssigneeID VARCHAR(26) NOT NULL,
            CreatorID VARCHAR(26) NOT NULL,
            ChannelID VARCHAR(26) NOT NULL,
            Deadline BIGINT NOT NULL,
            Status VARCHAR(20) NOT NULL DEFAULT 'open',
            CreatedAt BIGINT NOT NULL,
            UpdatedAt BIGINT NOT NULL,
            CONSTRAINT FK_LLM_Tasks_Channels FOREIGN KEY (ChannelID) REFERENCES Channels(Id) ON DELETE CASCADE
        );
    `

	if _, err := p.db.Exec(taskQuery); err != nil {
		return fmt.Errorf("can't create llm tasks table: %w", err)
	}

	rollCallQuery := `
        CREATE TABLE IF NOT EXISTS LLM_RollCalls (
            ID VARCHAR(36) NOT NULL PRIMARY KEY,
            ChannelID VARCHAR(26) NOT NULL,
            CreatorID VARCHAR(26) NOT NULL,
            Title TEXT NOT NULL,
            Status VARCHAR(20) NOT NULL DEFAULT 'active',
            CreatedAt BIGINT NOT NULL,
            EndedAt BIGINT,
            CONSTRAINT FK_LLM_RollCalls_Channels FOREIGN KEY (ChannelID) REFERENCES Channels(Id) ON DELETE CASCADE
        );
    `

	if _, err := p.db.Exec(rollCallQuery); err != nil {
		return fmt.Errorf("can't create llm roll calls table: %w", err)
	}

	rollCallResponseQuery := `
        CREATE TABLE IF NOT EXISTS LLM_RollCallResponses (
            RollCallID VARCHAR(36) NOT NULL,
            UserID VARCHAR(26) NOT NULL,
            Response TEXT,
            ResponseTime BIGINT NOT NULL,
            PRIMARY KEY (RollCallID, UserID),
            CONSTRAINT FK_LLM_RollCallResponses_RollCalls FOREIGN KEY (RollCallID) REFERENCES LLM_RollCalls(Id) ON DELETE CASCADE
        );
    `

	if _, err := p.db.Exec(rollCallResponseQuery); err != nil {
		return fmt.Errorf("can't create llm roll call responses table: %w", err)
	}

	return nil
}

func (p *Plugin) saveTitleAsync(threadID, title string) {
	go func() {
		if err := p.saveTitle(threadID, title); err != nil {
			p.API.LogError("failed to save title: " + err.Error())
		}
	}()
}

func (p *Plugin) saveTitle(threadID, title string) error {
	_, err := p.execBuilder(p.builder.Insert("LLM_PostMeta").
		Columns("RootPostID", "Title").
		Values(threadID, title).
		Suffix("ON DUPLICATE KEY UPDATE Title = ?", title))
	return err
}

type AIThread struct {
	ID         string
	Message    string
	ChannelID  string
	Title      string
	ReplyCount int
	UpdateAt   int64
}

func (p *Plugin) getAIThreads(dmChannelIDs []string) ([]AIThread, error) {
	var posts []AIThread
	if err := p.doQuery(&posts, p.builder.
		Select(
			"p.Id",
			"p.Message",
			"p.ChannelID",
			"COALESCE(t.Title, '') as Title",
			"(SELECT COUNT(*) FROM Posts WHERE Posts.RootId = p.Id AND DeleteAt = 0) AS ReplyCount",
			"p.UpdateAt",
		).
		From("Posts as p").
		Where(sq.Eq{"ChannelID": dmChannelIDs}).
		Where(sq.Eq{"RootId": ""}).
		Where(sq.Eq{"DeleteAt": 0}).
		LeftJoin("LLM_PostMeta as t ON t.RootPostID = p.Id").
		OrderBy("CreateAt DESC").
		Limit(60).
		Offset(0),
	); err != nil {
		return nil, fmt.Errorf("failed to get posts for bot DM: %w", err)
	}

	return posts, nil
}

func (p *Plugin) getFirstPostBeforeTimeRangeID(channelID string, startTime, endTime int64) (string, error) {
	var result struct {
		ID string `db:"id"`
	}
	err := p.doQuery(&result, p.builder.
		Select("id").
		From("Posts").
		Where(sq.Eq{"ChannelId": channelID}).
		Where(sq.And{
			sq.GtOrEq{"CreateAt": startTime},
			sq.LtOrEq{"CreateAt": endTime},
			sq.Eq{"DeleteAt": 0},
		}).
		OrderBy("CreateAt ASC").
		Limit(1))

	if err != nil {
		return "", fmt.Errorf("failed to get first post ID: %w", err)
	}

	return result.ID, nil
}
