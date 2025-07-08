// Copyright (c) 2023-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package main

import (
	"database/sql"
	"fmt"

	"errors"

	sq "github.com/Masterminds/squirrel"
	"github.com/jmoiron/sqlx"
	"github.com/mattermost/mattermost/server/public/model"
)

type builder interface {
	ToSql() (string, []interface{}, error)
}

func (p *Plugin) SetupDB() error {
	driverName := p.pluginAPI.Store.DriverName()
	if driverName != model.DatabaseDriverPostgres && driverName != model.DatabaseDriverMysql {
		return errors.New("this plugin is only supported on PostgreSQL and MySQL")
	}

	origDB, err := p.pluginAPI.Store.GetMasterDB()
	if err != nil {
		return err
	}
	p.db = sqlx.NewDb(origDB, driverName)

	builder := sq.StatementBuilder.PlaceholderFormat(sq.Question)
	if driverName == model.DatabaseDriverPostgres {
		builder = builder.PlaceholderFormat(sq.Dollar)
	}
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
	driverName := p.pluginAPI.Store.DriverName()

	var createTableSQL string

	if driverName == model.DatabaseDriverPostgres {
		createTableSQL = `
			CREATE TABLE IF NOT EXISTS LLM_PostMeta (
				RootPostID TEXT NOT NULL REFERENCES Posts(ID) ON DELETE CASCADE PRIMARY KEY,
				Title TEXT NOT NULL
			);
		`
	} else {
		// MySQL
		createTableSQL = `
			CREATE TABLE IF NOT EXISTS LLM_PostMeta (
				RootPostID VARCHAR(26) NOT NULL PRIMARY KEY,
				Title TEXT NOT NULL,
				FOREIGN KEY (RootPostID) REFERENCES Posts(ID) ON DELETE CASCADE
			);
		`
	}

	if _, err := p.db.Exec(createTableSQL); err != nil {
		return fmt.Errorf("can't create llm titles table: %w", err)
	}

	// Handle migration from old LLM_Threads table if it exists
	// This fixes data retention issues when a post is deleted for an older version of the postmeta table.
	if err := p.migrateLegacyTable(); err != nil {
		return fmt.Errorf("failed to migrate legacy table: %w", err)
	}

	return nil
}

// migrateLegacyTable handles the migration from the old LLM_Threads table
func (p *Plugin) migrateLegacyTable() error {
	driverName := p.pluginAPI.Store.DriverName()

	// First, check if the old LLM_Threads table exists
	var tableExists bool
	if driverName == model.DatabaseDriverPostgres {
		err := p.db.Get(&tableExists,
			"SELECT EXISTS (SELECT FROM information_schema.tables WHERE table_name = 'llm_threads')")
		if err != nil {
			// If we can't check, assume table doesn't exist and continue
			return nil
		}
	} else {
		// MySQL
		err := p.db.Get(&tableExists,
			"SELECT COUNT(*) > 0 FROM information_schema.tables WHERE table_name = 'LLM_Threads' AND table_schema = DATABASE()")
		if err != nil {
			// If we can't check, assume table doesn't exist and continue
			return nil
		}
	}

	if !tableExists {
		return nil
	}

	// If old table exists, migrate data and drop constraints
	if driverName == model.DatabaseDriverPostgres {
		// Drop constraint if it exists (PostgreSQL supports IF EXISTS)
		if _, err := p.db.Exec("ALTER TABLE IF EXISTS LLM_Threads DROP CONSTRAINT IF EXISTS llm_threads_rootpostid_fkey"); err != nil {
			// Log but don't fail - constraint might not exist
			p.pluginAPI.Log.Warn("Could not drop constraint from LLM_Threads table", "error", err)
		}
	} else {
		// MySQL - need to check if constraint exists before dropping
		var constraintExists bool
		err := p.db.Get(&constraintExists, `
			SELECT COUNT(*) > 0 
			FROM information_schema.table_constraints 
			WHERE constraint_name = 'llm_threads_rootpostid_fkey' 
			AND table_name = 'LLM_Threads' 
			AND table_schema = DATABASE()`)

		if err == nil && constraintExists {
			if _, err := p.db.Exec("ALTER TABLE LLM_Threads DROP FOREIGN KEY llm_threads_rootpostid_fkey"); err != nil {
				// Log but don't fail - we'll try to continue
				p.pluginAPI.Log.Warn("Could not drop constraint from LLM_Threads table", "error", err)
			}
		}
	}

	// Migrate data from old table to new table
	_, err := p.db.Exec("INSERT IGNORE INTO LLM_PostMeta(RootPostID, Title) SELECT RootPostID, Title FROM LLM_Threads")
	if err != nil {
		// For PostgreSQL, use ON CONFLICT instead of INSERT IGNORE
		if driverName == model.DatabaseDriverPostgres {
			_, err = p.db.Exec("INSERT INTO LLM_PostMeta(RootPostID, Title) SELECT RootPostID, Title FROM LLM_Threads ON CONFLICT (RootPostID) DO NOTHING")
		}
		if err != nil {
			p.pluginAPI.Log.Warn("Could not migrate data from LLM_Threads to LLM_PostMeta", "error", err)
		}
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
	driverName := p.pluginAPI.Store.DriverName()

	if driverName == model.DatabaseDriverPostgres {
		_, err := p.execBuilder(p.builder.Insert("LLM_PostMeta").
			Columns("RootPostID", "Title").
			Values(threadID, title).
			Suffix("ON CONFLICT (RootPostID) DO UPDATE SET Title = ?", title))
		return err
	} else {
		// MySQL
		_, err := p.execBuilder(p.builder.Insert("LLM_PostMeta").
			Columns("RootPostID", "Title").
			Values(threadID, title).
			Suffix("ON DUPLICATE KEY UPDATE Title = VALUES(Title)"))
		return err
	}
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

// GetDatabaseType returns the database type being used
func (p *Plugin) GetDatabaseType() string {
	return p.pluginAPI.Store.DriverName()
}

// IsPostgreSQLDatabase returns true if using PostgreSQL
func (p *Plugin) IsPostgreSQLDatabase() bool {
	return p.pluginAPI.Store.DriverName() == model.DatabaseDriverPostgres
}

// IsMySQLDatabase returns true if using MySQL
func (p *Plugin) IsMySQLDatabase() bool {
	return p.pluginAPI.Store.DriverName() == model.DatabaseDriverMysql
}
