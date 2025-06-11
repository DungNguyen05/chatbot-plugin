// Copyright (c) 2023-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package main

import (
	"encoding/json"
	"net/http"
	"time"

	"errors"
	"fmt"

	"github.com/gin-gonic/gin"
	"github.com/mattermost/mattermost/server/public/model"
)

// ReindexRequest represents the request payload for reindexing
type ReindexRequest struct {
	LastKPosts  int  `json:"lastKPosts,omitempty"`  // If 0 or not provided, reindex all posts
	FullReindex bool `json:"fullReindex,omitempty"` // Explicitly request full reindex
}

// DatabaseTypeResponse represents the response for database type endpoint
type DatabaseTypeResponse struct {
	Type string `json:"type"`
}

// handleGetDatabaseType returns the database type being used
func (p *Plugin) handleGetDatabaseType(c *gin.Context) {
	databaseType := p.GetDatabaseType()
	c.JSON(http.StatusOK, DatabaseTypeResponse{
		Type: databaseType,
	})
}

// handleReindexPosts starts a background job to reindex posts
func (p *Plugin) handleReindexPosts(c *gin.Context) {
	// Check if search is initialized
	if p.search == nil {
		c.AbortWithError(http.StatusBadRequest, fmt.Errorf("search functionality is not configured"))
		return
	}

	// Check database compatibility
	if !p.IsPostgreSQLDatabase() {
		c.AbortWithError(http.StatusBadRequest, fmt.Errorf("search functionality requires PostgreSQL database, current database: %s", p.GetDatabaseType()))
		return
	}

	// Parse request body
	var req ReindexRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		// If no body or invalid JSON, default to full reindex
		req.FullReindex = true
		req.LastKPosts = 0
	}

	// Validate lastKPosts
	if req.LastKPosts < 0 {
		c.AbortWithError(http.StatusBadRequest, fmt.Errorf("lastKPosts must be non-negative"))
		return
	}

	// If lastKPosts is 0, treat as full reindex
	if req.LastKPosts == 0 {
		req.FullReindex = true
	}

	// Log current configuration
	cfg := p.getConfiguration()
	p.pluginAPI.Log.Info("Reindex requested",
		"configured_dimensions", cfg.EmbeddingSearchConfig.Dimensions,
		"last_k_posts", req.LastKPosts,
		"full_reindex", req.FullReindex)

	// Check if a job is already running
	data, appErr := p.API.KVGet(ReindexJobKey)
	if appErr != nil {
		c.AbortWithError(http.StatusInternalServerError, fmt.Errorf("failed to check job status: %w", appErr))
		return
	}

	if data != nil {
		var jobStatus JobStatus
		if err := json.Unmarshal(data, &jobStatus); err == nil {
			if jobStatus.Status == JobStatusRunning {
				c.JSON(http.StatusConflict, jobStatus)
				return
			}
		}
	}

	// Get an estimate of total posts for progress tracking
	var count int64
	var query string
	var args []interface{}

	if req.FullReindex {
		query = `SELECT COUNT(*) FROM Posts WHERE DeleteAt = 0 AND Message != '' AND Type = ''`
		args = []interface{}{}
	} else {
		query = `SELECT COUNT(*) FROM (
			SELECT Id FROM Posts 
			WHERE DeleteAt = 0 AND Message != '' AND Type = ''
			ORDER BY CreateAt DESC, Id DESC
			LIMIT $1
		) AS subq`
		args = []interface{}{req.LastKPosts}
	}

	err := p.db.Get(&count, query, args...)
	if err != nil {
		p.pluginAPI.Log.Warn("Failed to get post count for progress tracking", "error", err)
		count = 0 // Continue with zero estimate
	}

	// Create initial job status
	jobStatus := &JobStatus{
		Status:      JobStatusRunning,
		StartedAt:   time.Now(),
		TotalRows:   count,
		LastKPosts:  req.LastKPosts,
		FullReindex: req.FullReindex,
	}

	// Save initial job status
	jobData, _ := json.Marshal(jobStatus)
	if appErr := p.API.KVSet(ReindexJobKey, jobData); appErr != nil {
		c.AbortWithError(http.StatusInternalServerError, fmt.Errorf("failed to save job status: %w", appErr))
		return
	}

	// Start the reindexing job in background
	go p.runReindexJob(jobStatus)

	c.JSON(http.StatusOK, jobStatus)
}

// handleGetJobStatus gets the status of the reindex job
func (p *Plugin) handleGetJobStatus(c *gin.Context) {
	data, appErr := p.API.KVGet(ReindexJobKey)
	if appErr != nil {
		c.AbortWithError(http.StatusInternalServerError, fmt.Errorf("failed to get job status: %w", appErr))
		return
	}

	if data == nil {
		c.JSON(http.StatusNotFound, gin.H{
			"status": "no_job",
		})
		return
	}

	var jobStatus JobStatus
	if err := json.Unmarshal(data, &jobStatus); err != nil {
		c.AbortWithError(http.StatusInternalServerError, fmt.Errorf("failed to parse job status: %w", err))
		return
	}

	c.JSON(http.StatusOK, jobStatus)
}

// handleCancelJob cancels a running reindex job
func (p *Plugin) handleCancelJob(c *gin.Context) {
	data, appErr := p.API.KVGet(ReindexJobKey)
	if appErr != nil {
		c.AbortWithError(http.StatusInternalServerError, fmt.Errorf("failed to get job status: %w", appErr))
		return
	}

	if data == nil {
		c.JSON(http.StatusNotFound, gin.H{
			"status": "no_job",
		})
		return
	}

	var jobStatus JobStatus
	if err := json.Unmarshal(data, &jobStatus); err != nil {
		c.AbortWithError(http.StatusInternalServerError, fmt.Errorf("failed to parse job status: %w", err))
		return
	}

	if jobStatus.Status != JobStatusRunning {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "not_running",
		})
		return
	}

	// Update status to canceled
	jobStatus.Status = JobStatusCanceled
	jobStatus.CompletedAt = time.Now()

	// Save updated status
	data, _ = json.Marshal(jobStatus)
	if appErr := p.API.KVSet(ReindexJobKey, data); appErr != nil {
		c.AbortWithError(http.StatusInternalServerError, fmt.Errorf("failed to save job status: %w", appErr))
		return
	}

	c.JSON(http.StatusOK, jobStatus)
}

func (p *Plugin) mattermostAdminAuthorizationRequired(c *gin.Context) {
	userID := c.GetHeader("Mattermost-User-Id")

	if !p.pluginAPI.User.HasPermissionTo(userID, model.PermissionManageSystem) {
		c.AbortWithError(http.StatusForbidden, errors.New("must be a system admin"))
		return
	}
}
