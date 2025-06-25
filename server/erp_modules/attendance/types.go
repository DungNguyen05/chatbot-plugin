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
	GetUserByUsername(username string) (*model.User, error)
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
