// Copyright (c) 2023-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package attendance

import (
	"github.com/mattermost/mattermost-plugin-ai/server/llm"
	"github.com/mattermost/mattermost/server/public/model"
)

// API endpoint suffix for ERP (this is fixed)
const ERPEndpointSuffix = "/api/method/frappe.desk.form.save.savedocs"

// RollCallEventType defines the type of roll call event
type RollCallEventType string

const (
	// RollCallEventCheckIn represents a check-in event
	RollCallEventCheckIn RollCallEventType = "check_in"
	// RollCallEventCheckOut represents a check-out event
	RollCallEventCheckOut RollCallEventType = "check_out"
	// RollCallEventAbsent represents an absence notification
	RollCallEventAbsent RollCallEventType = "absent"
)

// AttendanceConfig represents the configuration for attendance module
type AttendanceConfig struct {
	ERPDomain      string   `json:"erpDomain"`
	ERPAPIKey      string   `json:"erpAPIKey"`
	ERPAPISecret   string   `json:"erpAPISecret"`
	NotifyChannels []string `json:"notifyChannels"`
	Enabled        bool     `json:"enabled"`
}

// I18nBundle interface for internationalization
type I18nBundle interface {
	Localize(messageID, defaultMessage, locale string, params ...interface{}) string
}

// PromptsInterface interface for prompts
type PromptsInterface interface {
	FormatString(templateCode string, context *llm.Context) (string, error)
	Format(templateName string, context *llm.Context) (string, error)
}

// PluginAPI interface for plugin API operations
type PluginAPI interface {
	LogDebug(message string, keyValuePairs ...interface{})
	LogError(message string, keyValuePairs ...interface{})
	LogInfo(message string, keyValuePairs ...interface{})
	LogWarn(message string, keyValuePairs ...interface{})
	GetUser(userID string) (*model.User, error)
	CreatePost(post *model.Post) error
	GetConfig() *model.Config
	BotDMNonResponse(botUserID, userID string, post *model.Post) error
	GetChannel(channelID string) (*model.Channel, error)
	GetChannelMember(channelID, userID string) (*model.ChannelMember, error)
	AddChannelMember(channelID, userID string) (*model.ChannelMember, error)
}

// EmployeeCheckin represents the data structure for ERPNEXT employee check-in
type EmployeeCheckin struct {
	Docstatus          int    `json:"docstatus"`
	Doctype            string `json:"doctype"`
	Name               string `json:"name"`
	IsLocal            bool   `json:"__islocal"`
	Unsaved            bool   `json:"__unsaved"`
	Owner              string `json:"owner"`
	LogType            string `json:"log_type"`
	Time               string `json:"time"`
	SkipAutoAttendance int    `json:"skip_auto_attendance"`
	Offshift           int    `json:"offshift"`
	EmployeeName       string `json:"employee_name"`
	Employee           string `json:"employee"`
}

// EmployeeAttendance represents the data structure for ERPNEXT employee attendance (for absence)
type EmployeeAttendance struct {
	Docstatus      int    `json:"docstatus"`
	Doctype        string `json:"doctype"`
	Name           string `json:"name"`
	IsLocal        bool   `json:"__islocal"`
	Unsaved        bool   `json:"__unsaved"`
	Owner          string `json:"owner"`
	Employee       string `json:"employee"`
	EmployeeName   string `json:"employee_name"`
	AttendanceDate string `json:"attendance_date"`
	Status         string `json:"status"`
	Leave          string `json:"leave_type,omitempty"`
	Reason         string `json:"reason,omitempty"`
	Company        string `json:"company,omitempty"`
}

// Add these types to the existing types.go file

// Employee represents an employee from ERPNext
type Employee struct {
	Name            string  `json:"name"`             // Employee ID
	EmployeeName    string  `json:"employee_name"`    // Full name
	CustomChatID    string  `json:"custom_chat_id"`   // Chat ID for integration
	Status          string  `json:"status"`           // Active/Inactive
	Department      string  `json:"department"`       // Department
	Designation     string  `json:"designation"`      // Job title
	EmployeeNumber  string  `json:"employee_number"`  // Employee number
	MatchConfidence float64 `json:"match_confidence"` // Search confidence score
}

// AttendanceRecord represents a single attendance record
type AttendanceRecord struct {
	Name           string  `json:"name"`            // Record ID
	Status         string  `json:"status"`          // Present/Absent/Half Day/Work From Home
	AttendanceDate string  `json:"attendance_date"` // Date in YYYY-MM-DD format
	InTime         string  `json:"in_time"`         // Check-in time
	OutTime        string  `json:"out_time"`        // Check-out time
	WorkingHours   float64 `json:"working_hours"`   // Total working hours
	LateEntry      int     `json:"late_entry"`      // 1 if late, 0 if on time
	EarlyExit      int     `json:"early_exit"`      // 1 if early exit, 0 if normal
}

// EmployeeAttendanceReport represents attendance report for a single employee
type EmployeeAttendanceReport struct {
	EmployeeID       string             `json:"employee_id"`
	EmployeeName     string             `json:"employee_name"`
	StartDate        string             `json:"start_date"`
	EndDate          string             `json:"end_date"`
	TotalDays        int                `json:"total_days"`
	PresentDays      int                `json:"present_days"`
	AbsentDays       int                `json:"absent_days"`
	HalfDays         int                `json:"half_days"`
	WorkFromHomeDays int                `json:"work_from_home_days"`
	LateDays         int                `json:"late_days"`
	EarlyExitDays    int                `json:"early_exit_days"`
	Records          []AttendanceRecord `json:"records"`
	ErrorMessage     string             `json:"error_message,omitempty"` // For failed queries
}

// AttendanceReportRequest represents the structured attendance report request
type AttendanceReportRequest struct {
	Type       string   `json:"type"`  // "by_names" or "all_employees"
	Names      []string `json:"names"` // List of names to search for
	TimePeriod struct {
		Type        string `json:"type"`
		StartDate   string `json:"start_date"`
		EndDate     string `json:"end_date"`
		Description string `json:"description"`
	} `json:"time_period"`
}
