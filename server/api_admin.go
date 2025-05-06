// Copyright (c) 2023-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package main

import (
	"net/http"

	"errors"

	"github.com/gin-gonic/gin"
	"github.com/mattermost/mattermost/server/public/model"
)

// handleReindexPosts returns an error message for MySQL since vector search is not supported
func (p *Plugin) handleReindexPosts(c *gin.Context) {
	c.JSON(http.StatusNotImplemented, gin.H{
		"error": "Reindexing is not available when using MySQL. Vector search requires PostgreSQL with the pgvector extension.",
	})
}

// handleGetJobStatus returns an error message for MySQL since vector search is not supported
func (p *Plugin) handleGetJobStatus(c *gin.Context) {
	c.JSON(http.StatusNotImplemented, gin.H{
		"error": "Search indexing is not available when using MySQL. Vector search requires PostgreSQL with the pgvector extension.",
	})
}

// handleCancelJob returns an error message for MySQL since vector search is not supported
func (p *Plugin) handleCancelJob(c *gin.Context) {
	c.JSON(http.StatusNotImplemented, gin.H{
		"error": "Search indexing is not available when using MySQL. Vector search requires PostgreSQL with the pgvector extension.",
	})
}

func (p *Plugin) mattermostAdminAuthorizationRequired(c *gin.Context) {
	userID := c.GetHeader("Mattermost-User-Id")

	if !p.pluginAPI.User.HasPermissionTo(userID, model.PermissionManageSystem) {
		c.AbortWithError(http.StatusForbidden, errors.New("must be a system admin"))
		return
	}
}
