// Copyright (c) 2023-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package main

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// Modified handler to return an error message since search is not supported on MySQL
func (p *Plugin) handleRunSearch(c *gin.Context) {
	c.JSON(http.StatusNotImplemented, gin.H{
		"error": "Search functionality is not available when using MySQL. Vector search requires PostgreSQL with the pgvector extension.",
	})
}

// Modified handler to return an error message since search is not supported on MySQL
func (p *Plugin) handleSearchQuery(c *gin.Context) {
	c.JSON(http.StatusNotImplemented, gin.H{
		"error": "Search functionality is not available when using MySQL. Vector search requires PostgreSQL with the pgvector extension.",
	})
}
