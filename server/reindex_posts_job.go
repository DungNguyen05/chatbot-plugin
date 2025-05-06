// Copyright (c) 2023-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package main

import (
	"encoding/json"
	"time"
)

const (
	JobStatusRunning   = "running"
	JobStatusCompleted = "completed"
	JobStatusFailed    = "failed"
	JobStatusCanceled  = "canceled"

	// KV store keys
	ReindexJobKey = "reindex_job_status"
)

// JobStatus represents the status of a reindex job
type JobStatus struct {
	Status        string    `json:"status"`
	Error         string    `json:"error,omitempty"`
	StartedAt     time.Time `json:"started_at"`
	CompletedAt   time.Time `json:"completed_at,omitempty"`
	ProcessedRows int64     `json:"processed_rows"`
	TotalRows     int64     `json:"total_rows"`
}

// Since vector search is not available in MySQL, this is a stub implementation
// that just returns an error status
func (p *Plugin) runReindexJob(jobStatus *JobStatus) {
	jobStatus.Status = JobStatusFailed
	jobStatus.Error = "Reindexing is not available when using MySQL. Vector search requires PostgreSQL with the pgvector extension."
	jobStatus.CompletedAt = time.Now()
	p.saveJobStatus(jobStatus)

	p.pluginAPI.Log.Warn("Reindexing not available with MySQL database")
}

// saveJobStatus saves the job status to KV store
func (p *Plugin) saveJobStatus(status *JobStatus) {
	data, _ := json.Marshal(status)
	if err := p.API.KVSet(ReindexJobKey, data); err != nil {
		p.pluginAPI.Log.Error("Failed to save job status", "error", err)
	}
}
