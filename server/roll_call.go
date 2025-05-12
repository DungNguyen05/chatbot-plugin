// Copyright (c) 2023-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package main

import (
	"fmt"
	"sync"
	"time"
)

// RollCall holds the state of an active roll call
type RollCall struct {
	ChannelID             string
	StartTime             time.Time
	InitiatorID           string
	RespondedIDs          map[string]bool
	ERPRecordedUsers      map[string]bool // Track which users were recorded in ERP
	Active                bool
	ResponseCount         int
	CheckoutRecordedUsers map[string]bool
}

// RollCallManager manages active roll calls
type RollCallManager struct {
	activeRollCalls map[string]*RollCall // channelID -> RollCall
	mu              sync.RWMutex
}

// NewRollCallManager creates a new RollCallManager
func NewRollCallManager() *RollCallManager {
	return &RollCallManager{
		activeRollCalls: make(map[string]*RollCall),
	}
}

// StartRollCall starts a new roll call in the given channel
func (r *RollCallManager) StartRollCall(channelID string, initiatorID string) (*RollCall, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Check if there's already an active roll call
	if rollCall, exists := r.activeRollCalls[channelID]; exists && rollCall.Active {
		return nil, fmt.Errorf("a roll call is already active in this channel")
	}

	// Create a new roll call
	rollCall := &RollCall{
		ChannelID:        channelID,
		StartTime:        time.Now(),
		InitiatorID:      initiatorID,
		RespondedIDs:     make(map[string]bool),
		ERPRecordedUsers: make(map[string]bool),
		Active:           true,
	}

	r.activeRollCalls[channelID] = rollCall
	return rollCall, nil
}

// EndRollCall ends an active roll call
func (r *RollCallManager) EndRollCall(channelID string) (*RollCall, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	rollCall, exists := r.activeRollCalls[channelID]
	if !exists || !rollCall.Active {
		return nil, fmt.Errorf("no active roll call in this channel")
	}

	rollCall.Active = false
	return rollCall, nil
}

// RespondToRollCall records a user's response to an active roll call
func (r *RollCallManager) RespondToRollCall(channelID string, userID string) (*RollCall, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	rollCall, exists := r.activeRollCalls[channelID]
	if !exists || !rollCall.Active {
		return nil, false, fmt.Errorf("no active roll call in this channel")
	}

	// Check if this is a new response
	isNewResponse := false
	if _, responded := rollCall.RespondedIDs[userID]; !responded {
		rollCall.RespondedIDs[userID] = true
		rollCall.ResponseCount++
		isNewResponse = true
	}

	return rollCall, isNewResponse, nil
}

// MarkUserERPRecorded marks that a user's attendance has been recorded in ERP
func (r *RollCallManager) MarkUserERPRecorded(channelID string, userID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	rollCall, exists := r.activeRollCalls[channelID]
	if !exists {
		return fmt.Errorf("no roll call in this channel")
	}

	rollCall.ERPRecordedUsers[userID] = true
	return nil
}

// IsUserERPRecorded checks if a user's attendance has been recorded in ERP
func (r *RollCallManager) IsUserERPRecorded(channelID string, userID string) (bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	rollCall, exists := r.activeRollCalls[channelID]
	if !exists {
		return false, fmt.Errorf("no roll call in this channel")
	}

	return rollCall.ERPRecordedUsers[userID], nil
}

// GetRollCall gets the roll call for a channel
func (r *RollCallManager) GetRollCall(channelID string) (*RollCall, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	rollCall, exists := r.activeRollCalls[channelID]
	if !exists {
		return nil, fmt.Errorf("no roll call in this channel")
	}

	return rollCall, nil
}

// MarkUserCheckoutRecorded marks that a user's checkout has been recorded in ERP
func (r *RollCallManager) MarkUserCheckoutRecorded(channelID string, userID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	rollCall, exists := r.activeRollCalls[channelID]
	if !exists {
		return fmt.Errorf("no roll call in this channel")
	}

	// Initialize map if needed
	if rollCall.CheckoutRecordedUsers == nil {
		rollCall.CheckoutRecordedUsers = make(map[string]bool)
	}

	rollCall.CheckoutRecordedUsers[userID] = true
	return nil
}

// IsUserCheckoutRecorded checks if a user's checkout has been recorded in ERP
func (r *RollCallManager) IsUserCheckoutRecorded(channelID string, userID string) (bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	rollCall, exists := r.activeRollCalls[channelID]
	if !exists {
		return false, fmt.Errorf("no roll call in this channel")
	}

	if rollCall.CheckoutRecordedUsers == nil {
		return false, nil
	}

	return rollCall.CheckoutRecordedUsers[userID], nil
}

// Helper function to format duration
func formatDuration(d time.Duration) string {
	d = d.Round(time.Second)
	h := d / time.Hour
	d -= h * time.Hour
	m := d / time.Minute
	d -= m * time.Minute
	s := d / time.Second

	if h > 0 {
		return fmt.Sprintf("%dh %dm %ds", h, m, s)
	}
	if m > 0 {
		return fmt.Sprintf("%dm %ds", m, s)
	}
	return fmt.Sprintf("%ds", s)
}
