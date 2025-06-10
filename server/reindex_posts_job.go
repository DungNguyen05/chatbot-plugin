// Copyright (c) 2023-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/mattermost/mattermost-plugin-ai/server/embeddings"
	"github.com/mattermost/mattermost/server/public/model"
)

const (
	JobStatusRunning   = "running"
	JobStatusCompleted = "completed"
	JobStatusFailed    = "failed"
	JobStatusCanceled  = "canceled"

	defaultBatchSize = 100

	// KV store keys
	ReindexJobKey = "reindex_job_status"
)

// PostRecord represents a post record from the database
type PostRecord struct {
	ID       string `db:"id"`
	Message  string `db:"message"`
	UserID   string `db:"userid"`
	CreateAt int64  `db:"createat"`
	TeamID   string `db:"teamid"`

	ChannelID   string `db:"channelid"`
	ChannelName string `db:"channelname"`
	ChannelType string `db:"channeltype"`
}

// JobStatus represents the status of a reindex job
type JobStatus struct {
	Status        string    `json:"status"`
	Error         string    `json:"error,omitempty"`
	StartedAt     time.Time `json:"started_at"`
	CompletedAt   time.Time `json:"completed_at,omitempty"`
	ProcessedRows int64     `json:"processed_rows"`
	TotalRows     int64     `json:"total_rows"`
	LastKPosts    int       `json:"last_k_posts,omitempty"` // Number of recent posts to reindex (0 means all)
	FullReindex   bool      `json:"full_reindex,omitempty"` // Whether this is a full reindex
}

// runReindexJob runs the reindexing process
func (p *Plugin) runReindexJob(jobStatus *JobStatus) {
	defer func() {
		if r := recover(); r != nil {
			p.pluginAPI.Log.Error("Reindex job panicked", "panic", r)
			jobStatus.Status = JobStatusFailed
			jobStatus.Error = fmt.Sprintf("Job panicked: %v", r)
			jobStatus.CompletedAt = time.Now()
			p.saveJobStatus(jobStatus)
		}
	}()

	ctx := context.Background()

	// Get the current embedding dimensions from configuration
	cfg := p.getConfiguration()
	targetDimensions := cfg.EmbeddingSearchConfig.Dimensions

	p.pluginAPI.Log.Info("Starting reindex job",
		"target_dimensions", targetDimensions,
		"last_k_posts", jobStatus.LastKPosts,
		"full_reindex", jobStatus.FullReindex)

	// ALWAYS drop and recreate the vector table first (regardless of full or partial reindex)
	p.pluginAPI.Log.Info("Dropping and recreating vector table with new dimensions")
	if err := p.search.RecreateIndex(ctx, targetDimensions); err != nil {
		jobStatus.Status = JobStatusFailed
		jobStatus.Error = fmt.Sprintf("Failed to recreate search index: %s", err)
		jobStatus.CompletedAt = time.Now()
		p.saveJobStatus(jobStatus)
		return
	}
	p.pluginAPI.Log.Info("Vector table recreated successfully", "dimensions", targetDimensions)

	var posts []PostRecord
	var processedCount int64
	lastSavedCount := int64(0) // Track when we last saved status

	if jobStatus.FullReindex {
		// Full reindex: process all posts with pagination
		p.pluginAPI.Log.Info("Processing ALL posts with pagination")
		lastCreateAt := int64(0)
		lastID := ""

		for {
			// Check if the job was canceled
			if p.isJobCanceled() {
				p.pluginAPI.Log.Info("Reindex job was canceled")
				return
			}

			// Run a batch of indexing
			query := `SELECT
				Posts.Id as id,
				Posts.Message as message,
				Posts.UserId as userid,
				Posts.ChannelId as channelid,
				Posts.CreateAt as createat,
				Channels.TeamId as teamid,
				Channels.Name as channelname,
				Channels.Type as channeltype
			FROM Posts
			LEFT JOIN Channels ON Posts.ChannelId = Channels.Id
			WHERE Posts.DeleteAt = 0 AND Posts.Message != '' AND Posts.Type = ''
				AND (Posts.CreateAt, Posts.Id) > ($1, $2)
			ORDER BY Posts.CreateAt ASC, Posts.Id ASC
			LIMIT $3`

			err := p.db.Select(&posts, query, lastCreateAt, lastID, defaultBatchSize)
			if err != nil {
				jobStatus.Status = JobStatusFailed
				jobStatus.Error = fmt.Sprintf("Failed to fetch posts: %s", err)
				jobStatus.CompletedAt = time.Now()
				p.saveJobStatus(jobStatus)
				return
			}

			if len(posts) == 0 {
				break
			}

			if err := p.processBatch(posts, &processedCount, jobStatus, &lastSavedCount); err != nil {
				return
			}

			// Update cursors for next batch
			lastPost := posts[len(posts)-1]
			lastCreateAt = lastPost.CreateAt
			lastID = lastPost.ID
		}
	} else {
		// Partial reindex: get the most recent K posts (table already dropped and recreated above)
		p.pluginAPI.Log.Info("Processing RECENT posts only", "count", jobStatus.LastKPosts)
		query := `SELECT
			Posts.Id as id,
			Posts.Message as message,
			Posts.UserId as userid,
			Posts.ChannelId as channelid,
			Posts.CreateAt as createat,
			Channels.TeamId as teamid,
			Channels.Name as channelname,
			Channels.Type as channeltype
		FROM Posts
		LEFT JOIN Channels ON Posts.ChannelId = Channels.Id
		WHERE Posts.DeleteAt = 0 AND Posts.Message != '' AND Posts.Type = ''
		ORDER BY Posts.CreateAt DESC, Posts.Id DESC
		LIMIT $1`

		err := p.db.Select(&posts, query, jobStatus.LastKPosts)
		if err != nil {
			jobStatus.Status = JobStatusFailed
			jobStatus.Error = fmt.Sprintf("Failed to fetch recent posts: %s", err)
			jobStatus.CompletedAt = time.Now()
			p.saveJobStatus(jobStatus)
			return
		}

		p.pluginAPI.Log.Info("Fetched recent posts for partial reindex", "count", len(posts))

		// Process all posts in batches
		for i := 0; i < len(posts); i += defaultBatchSize {
			// Check if the job was canceled
			if p.isJobCanceled() {
				p.pluginAPI.Log.Info("Reindex job was canceled")
				return
			}

			end := i + defaultBatchSize
			if end > len(posts) {
				end = len(posts)
			}

			batch := posts[i:end]
			if err := p.processBatch(batch, &processedCount, jobStatus, &lastSavedCount); err != nil {
				return
			}
		}
	}

	// Completed successfully
	jobStatus.Status = JobStatusCompleted
	jobStatus.CompletedAt = time.Now()
	p.saveJobStatus(jobStatus)

	p.pluginAPI.Log.Info("Reindexing completed",
		"processed_posts", processedCount,
		"full_reindex", jobStatus.FullReindex,
		"last_k_posts", jobStatus.LastKPosts)
}

// processBatch processes a batch of posts and updates job status
func (p *Plugin) processBatch(posts []PostRecord, processedCount *int64, jobStatus *JobStatus, lastSavedCount *int64) error {
	ctx := context.Background()

	// Process batch and index posts
	docs := make([]embeddings.PostDocument, 0, len(posts))
	for _, post := range posts {
		modelPost := &model.Post{
			Id:        post.ID,
			ChannelId: post.ChannelID,
			UserId:    post.UserID,
			Message:   post.Message,
			Type:      model.PostTypeDefault, // We already filter out non-default post types in the SQL query
			DeleteAt:  0,                     // We already filter deleted posts in the SQL query
		}

		// Create a minimal channel object with necessary fields for filtering
		channel := &model.Channel{
			Id:     post.ChannelID,
			TeamId: post.TeamID,
			Name:   post.ChannelName,
			Type:   model.ChannelType(post.ChannelType),
		}

		// Apply same indexing rules as indexPost
		if !p.ShouldIndexPost(modelPost, channel) {
			continue
		}

		docs = append(docs, embeddings.PostDocument{
			PostID:    modelPost.Id,
			CreateAt:  modelPost.CreateAt,
			TeamID:    post.TeamID,
			ChannelID: post.ChannelID,
			UserID:    post.UserID,
			Content:   post.Message,
		})
	}

	// Store the batch
	if len(docs) > 0 {
		if err := p.search.Store(ctx, docs); err != nil {
			jobStatus.Status = JobStatusFailed
			jobStatus.Error = fmt.Sprintf("Failed to store documents: %s", err)
			jobStatus.CompletedAt = time.Now()
			p.saveJobStatus(jobStatus)
			return err
		}
	}

	// Update progress
	*processedCount += int64(len(posts))
	jobStatus.ProcessedRows = *processedCount

	// Save progress every 500 additional processed records
	if *processedCount >= *lastSavedCount+500 {
		p.saveJobStatus(jobStatus)
		p.pluginAPI.Log.Info("Reindexing progress",
			"processed", *processedCount,
			"estimated_total", jobStatus.TotalRows,
			"full_reindex", jobStatus.FullReindex)
		*lastSavedCount = *processedCount
	}

	return nil
}

// isJobCanceled checks if the current job has been canceled
func (p *Plugin) isJobCanceled() bool {
	data, _ := p.API.KVGet(ReindexJobKey)
	if data != nil {
		var currentStatus JobStatus
		if err := json.Unmarshal(data, &currentStatus); err == nil {
			return currentStatus.Status == JobStatusCanceled
		}
	}
	return false
}

// saveJobStatus saves the job status to KV store
func (p *Plugin) saveJobStatus(status *JobStatus) {
	data, _ := json.Marshal(status)
	if err := p.API.KVSet(ReindexJobKey, data); err != nil {
		p.pluginAPI.Log.Error("Failed to save job status", "error", err)
	}
}
