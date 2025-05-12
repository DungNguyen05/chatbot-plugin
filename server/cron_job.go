// Copyright (c) 2023-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package main

import (
	"time"
)

// CronJob manages the daily automatic roll call
type CronJob struct {
	plugin    *Plugin
	stopChan  chan struct{}
	isRunning bool
}

// NewCronJob creates a new cron job manager
func NewCronJob(plugin *Plugin) *CronJob {
	return &CronJob{
		plugin:    plugin,
		stopChan:  make(chan struct{}),
		isRunning: false,
	}
}

// Start begins the cron job that checks for scheduled tasks
func (c *CronJob) Start() {
	if c.isRunning {
		return
	}
	c.isRunning = true

	go c.run()
}

// Stop stops the cron job
func (c *CronJob) Stop() {
	if !c.isRunning {
		return
	}
	c.isRunning = false
	c.stopChan <- struct{}{}
}

// run is the main loop that checks for scheduled tasks
func (c *CronJob) run() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			c.checkScheduledTasks()
		case <-c.stopChan:
			return
		}
	}
}

// checkScheduledTasks checks if any tasks need to be run
func (c *CronJob) checkScheduledTasks() {
	// Get current time in Vietnam timezone
	vietTime, err := GetVietnamTime()
	if err != nil {
		c.plugin.API.LogError("Failed to get Vietnam time", "error", err.Error())
		return
	}

	// Format current time as HH:MM:00 for comparison
	currentTimeStr := vietTime.Format("15:04:00")

	// Check if it's time to start roll call (8:00 AM)
	if currentTimeStr == RollCallStartTime {
		c.plugin.API.LogInfo("Starting daily roll call")
		go c.plugin.StartDailyRollCall()
	}

	// Check if it's time to end roll call (5:30 PM)
	if currentTimeStr == RollCallEndTime {
		c.plugin.API.LogInfo("Ending daily roll call")
		go c.plugin.EndDailyRollCall()
	}
}
