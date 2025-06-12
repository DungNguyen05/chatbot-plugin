// Copyright (c) 2023-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package attendance

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/mattermost/mattermost-plugin-ai/server/erp_modules"
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

// AttendanceModule handles attendance-related operations
type AttendanceModule struct {
	config           AttendanceConfig
	httpClient       *http.Client
	i18n             I18nBundle
	prompts          PromptsInterface
	getLLM           func() llm.LanguageModel
	notificationFunc func(userID, employeeName string, eventType RollCallEventType, eventTime string, reason string) error
	api              PluginAPI
	botUserID        string
}

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

// NewAttendanceModule creates a new attendance module
func NewAttendanceModule(
	config AttendanceConfig,
	httpClient *http.Client,
	i18n I18nBundle,
	prompts PromptsInterface,
	getLLM func() llm.LanguageModel,
	notificationFunc func(userID, employeeName string, eventType RollCallEventType, eventTime string, reason string) error,
	api PluginAPI,
	botUserID string,
) *AttendanceModule {
	return &AttendanceModule{
		config:           config,
		httpClient:       httpClient,
		i18n:             i18n,
		prompts:          prompts,
		getLLM:           getLLM,
		notificationFunc: notificationFunc,
		api:              api,
		botUserID:        botUserID,
	}
}

// GetCategory returns the category this module handles
func (m *AttendanceModule) GetCategory() string {
	return "attendance"
}

// GetSupportedActions returns list of actions this module supports
func (m *AttendanceModule) GetSupportedActions() []string {
	return []string{"check_in", "check_out", "absent"}
}

// CanHandle determines if this module can handle the given intent
func (m *AttendanceModule) CanHandle(intent *erp_modules.Intent) bool {
	if intent.Category != "attendance" {
		return false
	}

	supportedActions := m.GetSupportedActions()
	for _, action := range supportedActions {
		if intent.Action == action {
			return true
		}
	}

	return false
}

// Execute processes the attendance intent
func (m *AttendanceModule) Execute(ctx *erp_modules.ModuleContext, intent *erp_modules.Intent) (*erp_modules.ModuleResponse, error) {
	// Get employee ID
	employeeID, err := m.GetEmployeeIDFromUser(ctx.User)
	if err != nil {
		return &erp_modules.ModuleResponse{
			Success: false,
			Message: "❌ Không tìm thấy thông tin nhân viên của bạn trong hệ thống ERP. Vui lòng liên hệ quản trị viên.",
			Error:   err.Error(),
		}, nil
	}

	// Get employee name for notifications
	employeeName := ctx.User.Username
	if ctx.User.FirstName != "" || ctx.User.LastName != "" {
		employeeName = strings.TrimSpace(ctx.User.FirstName + " " + ctx.User.LastName)
	}

	// Execute specific action
	switch intent.Action {
	case "check_in":
		return m.handleCheckIn(employeeID, employeeName, ctx.User.Id)
	case "check_out":
		return m.handleCheckOut(employeeID, employeeName, ctx.User.Id)
	case "absent":
		reason := intent.Parameters["reason"]
		if reason == "" {
			reason = "Không có lý do cụ thể"
		}
		return m.handleAbsent(employeeID, employeeName, ctx.User.Id, reason)
	default:
		return &erp_modules.ModuleResponse{
			Success: false,
			Message: "Hành động không được hỗ trợ",
			Error:   fmt.Sprintf("unsupported action: %s", intent.Action),
		}, nil
	}
}

// GetDescription returns a description of what this module does
func (m *AttendanceModule) GetDescription() string {
	return "Quản lý điểm danh nhân viên: check-in, check-out, và báo nghỉ phép"
}

// GetActionExamples returns examples of user messages for each action
func (m *AttendanceModule) GetActionExamples() map[string][]string {
	return map[string][]string{
		"check_in": {
			"tôi đã có mặt",
			"tôi đã đến công ty",
			"check in",
			"điểm danh vào",
			"tôi đã tới",
			"có mặt rồi",
			"đã đến rồi",
			"tôi đã ở công ty",
		},
		"check_out": {
			"tôi về rồi",
			"tôi đã ra về",
			"check out",
			"điểm danh ra",
			"tôi đi về",
			"kết thúc làm việc",
			"tôi ra về",
			"về nhà",
		},
		"absent": {
			"tôi hôm nay nghỉ",
			"tôi không đi làm được",
			"nghỉ phép",
			"tôi ốm",
			"nghỉ ốm",
			"không thể đến công ty",
			"tôi nghỉ",
			"hôm nay tôi không làm",
		},
	}
}

// handleCheckIn processes check-in request
func (m *AttendanceModule) handleCheckIn(employeeID, employeeName, userID string) (*erp_modules.ModuleResponse, error) {
	formattedTime, err := m.RecordEmployeeCheckin(employeeID)
	if err != nil {
		return &erp_modules.ModuleResponse{
			Success: false,
			Message: "⚠️ Có lỗi xảy ra khi ghi nhận check-in. Vui lòng thử lại.",
			Error:   err.Error(),
		}, nil
	}

	// Send notification asynchronously
	go func() {
		_ = m.notificationFunc(userID, employeeName, RollCallEventCheckIn, formattedTime, "")
	}()

	return &erp_modules.ModuleResponse{
		Success:     true,
		Message:     fmt.Sprintf("✅ Đã ghi nhận check-in của bạn lúc **%s**!", formattedTime),
		ActionTaken: "check_in",
		Data: map[string]interface{}{
			"time":     formattedTime,
			"employee": employeeName,
		},
	}, nil
}

// handleCheckOut processes check-out request
func (m *AttendanceModule) handleCheckOut(employeeID, employeeName, userID string) (*erp_modules.ModuleResponse, error) {
	formattedTime, err := m.RecordEmployeeCheckout(employeeID)
	if err != nil {
		return &erp_modules.ModuleResponse{
			Success: false,
			Message: "⚠️ Có lỗi xảy ra khi ghi nhận check-out. Vui lòng thử lại.",
			Error:   err.Error(),
		}, nil
	}

	// Send notification asynchronously
	go func() {
		_ = m.notificationFunc(userID, employeeName, RollCallEventCheckOut, formattedTime, "")
	}()

	return &erp_modules.ModuleResponse{
		Success:     true,
		Message:     fmt.Sprintf("✅ Đã ghi nhận check-out của bạn lúc **%s**!", formattedTime),
		ActionTaken: "check_out",
		Data: map[string]interface{}{
			"time":     formattedTime,
			"employee": employeeName,
		},
	}, nil
}

// handleAbsent processes absent request
func (m *AttendanceModule) handleAbsent(employeeID, employeeName, userID, reason string) (*erp_modules.ModuleResponse, error) {
	recordedDate, err := m.RecordEmployeeAbsent(employeeID, reason)
	if err != nil {
		return &erp_modules.ModuleResponse{
			Success: false,
			Message: "⚠️ Có lỗi xảy ra khi ghi nhận nghỉ phép. Vui lòng thử lại.",
			Error:   err.Error(),
		}, nil
	}

	// Send notification asynchronously
	go func() {
		_ = m.notificationFunc(userID, employeeName, RollCallEventAbsent, recordedDate, reason)
	}()

	return &erp_modules.ModuleResponse{
		Success:     true,
		Message:     fmt.Sprintf("📝 Đã ghi nhận nghỉ phép của bạn cho ngày **%s** với lý do: \"%s\"", recordedDate, reason),
		ActionTaken: "absent",
		Data: map[string]interface{}{
			"date":     recordedDate,
			"reason":   reason,
			"employee": employeeName,
		},
	}, nil
}

// GetEmployeeIDFromUser gets employee ID from user
func (m *AttendanceModule) GetEmployeeIDFromUser(user *model.User) (string, error) {
	// Use the user's ID as the chat ID to lookup in ERPNext
	chatID := user.Id

	employeeID, err := m.GetEmployeeByChatID(chatID)
	if err != nil {
		return "", fmt.Errorf("failed to get employee by chat ID %s: %w", chatID, err)
	}

	return employeeID, nil
}

// GetEmployeeByChatID fetches employee information from ERPNext using chat ID
func (m *AttendanceModule) GetEmployeeByChatID(chatID string) (string, error) {
	m.api.LogDebug("Getting employee by chat ID", "chat_id", chatID)

	// Validate configuration
	if m.config.ERPDomain == "" {
		return "", fmt.Errorf("ERP domain not configured")
	}
	if m.config.ERPAPIKey == "" {
		return "", fmt.Errorf("ERP API key not configured")
	}
	if m.config.ERPAPISecret == "" {
		return "", fmt.Errorf("ERP API secret not configured")
	}

	// Combine API key and secret for token
	erpToken := m.config.ERPAPIKey + ":" + m.config.ERPAPISecret

	// Build the API endpoint for fetching employee by custom_chat_id
	baseURL := strings.TrimSuffix(m.config.ERPDomain, "/") + "/api/resource/Employee"

	// Create the filter parameter - use exact match first, then try partial match
	filterParam := fmt.Sprintf(`[["custom_chat_id","=","%s"]]`, chatID)

	// Parse the base URL
	reqURL, err := url.Parse(baseURL)
	if err != nil {
		return "", fmt.Errorf("failed to parse URL: %w", err)
	}

	// Add query parameters
	query := reqURL.Query()
	query.Add("filters", filterParam)
	query.Add("fields", `["name","employee_name","custom_chat_id"]`)
	reqURL.RawQuery = query.Encode()

	m.api.LogDebug("Making request to ERPNext", "url", reqURL.String())

	// Create the request
	req, err := http.NewRequest("GET", reqURL.String(), nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	req.Header.Set("Authorization", "token "+erpToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	// Make the request
	resp, err := m.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	// Read the response
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	m.api.LogDebug("ERPNext API Response",
		"status", resp.Status,
		"body", string(respBody))

	// Check the response status
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("ERP API error: %s - %s", resp.Status, string(respBody))
	}

	// Parse the response
	var apiResponse struct {
		Data []struct {
			Name         string `json:"name"`
			EmployeeName string `json:"employee_name"`
			CustomChatID string `json:"custom_chat_id"`
		} `json:"data"`
	}

	if err := json.Unmarshal(respBody, &apiResponse); err != nil {
		return "", fmt.Errorf("failed to parse response: %w", err)
	}

	m.api.LogDebug("Found employees", "count", len(apiResponse.Data))

	// Check if employee found with exact match
	if len(apiResponse.Data) > 0 {
		employee := apiResponse.Data[0]
		m.api.LogDebug("Found employee",
			"employee_id", employee.Name,
			"employee_name", employee.EmployeeName,
			"custom_chat_id", employee.CustomChatID)
		return employee.Name, nil
	}

	return "", fmt.Errorf("no employee found with chat_id: %s", chatID)
}

// RecordEmployeeCheckin sends the check-in data to ERPNEXT
func (m *AttendanceModule) RecordEmployeeCheckin(employeeID string) (string, error) {
	m.api.LogDebug("Recording employee check-in", "employee_id", employeeID)

	// Validate configuration
	if m.config.ERPDomain == "" {
		return "", fmt.Errorf("ERP domain not configured")
	}
	if m.config.ERPAPIKey == "" {
		return "", fmt.Errorf("ERP API key not configured")
	}
	if m.config.ERPAPISecret == "" {
		return "", fmt.Errorf("ERP API secret not configured")
	}

	// Combine API key and secret for token
	erpToken := m.config.ERPAPIKey + ":" + m.config.ERPAPISecret

	// Build the complete ERP endpoint
	erpEndpoint := strings.TrimSuffix(m.config.ERPDomain, "/") + ERPEndpointSuffix

	// Get Vietnam time instead of server time
	var serverTime int64
	vietTime, err := GetVietnamTime()
	if err != nil {
		m.api.LogWarn("Failed to get Vietnam time, falling back to server time", "error", err.Error())
		serverTime = model.GetMillis() // Fallback to server time
	} else {
		serverTime = vietTime.UnixMilli()
	}

	checkin, formattedTime := NewEmployeeCheckin(employeeID, serverTime)

	// Create the form data
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	// Marshal the doc to JSON
	docJSON, err := json.Marshal(checkin)
	if err != nil {
		return "", fmt.Errorf("failed to marshal employee checkin: %w", err)
	}

	// Add doc field
	if err := writer.WriteField("doc", string(docJSON)); err != nil {
		return "", fmt.Errorf("failed to write doc field: %w", err)
	}

	// Add action field
	if err := writer.WriteField("action", "Save"); err != nil {
		return "", fmt.Errorf("failed to write action field: %w", err)
	}

	// Close the writer
	if err := writer.Close(); err != nil {
		return "", fmt.Errorf("failed to close multipart writer: %w", err)
	}

	// Create the request
	req, err := http.NewRequest("POST", erpEndpoint, body)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Authorization", "token "+erpToken)

	// Add CORS headers
	req.Header.Set("Access-Control-Allow-Origin", "*")
	req.Header.Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	req.Header.Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

	// Make the request
	resp, err := m.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	// Read the response
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	// Check the response status
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("ERP API error: %s - %s", resp.Status, string(respBody))
	}

	// Log details about the successful check-in including the time used
	m.api.LogDebug("Employee check-in recorded successfully",
		"employee_id", employeeID,
		"time", formattedTime,
		"status", resp.Status,
		"response", string(respBody))

	// Return the formatted time that was used for the check-in
	return formattedTime, nil
}

// RecordEmployeeCheckout sends the check-out data to ERPNEXT
func (m *AttendanceModule) RecordEmployeeCheckout(employeeID string) (string, error) {
	m.api.LogDebug("Recording employee check-out", "employee_id", employeeID)

	// Validate configuration
	if m.config.ERPDomain == "" {
		return "", fmt.Errorf("ERP domain not configured")
	}
	if m.config.ERPAPIKey == "" {
		return "", fmt.Errorf("ERP API key not configured")
	}
	if m.config.ERPAPISecret == "" {
		return "", fmt.Errorf("ERP API secret not configured")
	}

	// Combine API key and secret for token
	erpToken := m.config.ERPAPIKey + ":" + m.config.ERPAPISecret

	// Build the complete ERP endpoint
	erpEndpoint := strings.TrimSuffix(m.config.ERPDomain, "/") + ERPEndpointSuffix

	// Get Vietnam time instead of server time
	var serverTime int64
	vietTime, err := GetVietnamTime()
	if err != nil {
		m.api.LogWarn("Failed to get Vietnam time, falling back to server time", "error", err.Error())
		serverTime = model.GetMillis() // Fallback to server time
	} else {
		serverTime = vietTime.UnixMilli()
	}

	checkout, formattedTime := NewEmployeeCheckout(employeeID, serverTime)

	// Create the form data
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	// Marshal the doc to JSON
	docJSON, err := json.Marshal(checkout)
	if err != nil {
		return "", fmt.Errorf("failed to marshal employee checkout: %w", err)
	}

	// Add doc field
	if err := writer.WriteField("doc", string(docJSON)); err != nil {
		return "", fmt.Errorf("failed to write doc field: %w", err)
	}

	// Add action field
	if err := writer.WriteField("action", "Save"); err != nil {
		return "", fmt.Errorf("failed to write action field: %w", err)
	}

	// Close the writer
	if err := writer.Close(); err != nil {
		return "", fmt.Errorf("failed to close multipart writer: %w", err)
	}

	// Create the request
	req, err := http.NewRequest("POST", erpEndpoint, body)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Authorization", "token "+erpToken)

	// Add CORS headers
	req.Header.Set("Access-Control-Allow-Origin", "*")
	req.Header.Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	req.Header.Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

	// Make the request
	resp, err := m.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	// Read the response
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	// Check the response status
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("ERP API error: %s - %s", resp.Status, string(respBody))
	}

	// Log details about the successful check-out including the time used
	m.api.LogDebug("Employee check-out recorded successfully",
		"employee_id", employeeID,
		"time", formattedTime,
		"status", resp.Status,
		"response", string(respBody))

	// Return the formatted time that was used for the check-out
	return formattedTime, nil
}

// RecordEmployeeAbsent records employee absence in ERP
func (m *AttendanceModule) RecordEmployeeAbsent(employeeID string, reason string) (string, error) {
	m.api.LogDebug("Recording employee absence", "employee_id", employeeID, "reason", reason)

	// Validate configuration
	if m.config.ERPDomain == "" {
		return "", fmt.Errorf("ERP domain not configured")
	}
	if m.config.ERPAPIKey == "" {
		return "", fmt.Errorf("ERP API key not configured")
	}
	if m.config.ERPAPISecret == "" {
		return "", fmt.Errorf("ERP API secret not configured")
	}

	// Combine API key and secret for token
	erpToken := m.config.ERPAPIKey + ":" + m.config.ERPAPISecret

	// Build the complete ERP endpoint
	erpEndpoint := strings.TrimSuffix(m.config.ERPDomain, "/") + ERPEndpointSuffix

	// Get Vietnam time for the record
	var serverTime int64
	vietTime, err := GetVietnamTime()
	if err != nil {
		m.api.LogWarn("Failed to get Vietnam time, falling back to server time", "error", err.Error())
		serverTime = model.GetMillis() // Fallback to server time
	} else {
		serverTime = vietTime.UnixMilli()
	}

	attendance, formattedDate := NewEmployeeAttendance(employeeID, reason, serverTime)

	// Create the form data
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	// Marshal the doc to JSON
	docJSON, err := json.Marshal(attendance)
	if err != nil {
		return "", fmt.Errorf("failed to marshal employee attendance: %w", err)
	}

	// Add doc field
	if err := writer.WriteField("doc", string(docJSON)); err != nil {
		return "", fmt.Errorf("failed to write doc field: %w", err)
	}

	// Add action field
	if err := writer.WriteField("action", "Save"); err != nil {
		return "", fmt.Errorf("failed to write action field: %w", err)
	}

	// Close the writer
	if err := writer.Close(); err != nil {
		return "", fmt.Errorf("failed to close multipart writer: %w", err)
	}

	// Create the request
	req, err := http.NewRequest("POST", erpEndpoint, body)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Authorization", "token "+erpToken)

	// Add CORS headers
	req.Header.Set("Access-Control-Allow-Origin", "*")
	req.Header.Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	req.Header.Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

	// Make the request
	resp, err := m.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	// Read the response
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	// Check the response status
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("ERP API error: %s - %s", resp.Status, string(respBody))
	}

	// Log details about the successful absence recording
	m.api.LogDebug("Employee absence recorded successfully",
		"employee_id", employeeID,
		"date", formattedDate,
		"reason", reason,
		"status", resp.Status,
		"response", string(respBody))

	// Return the formatted date that was used for the absence
	return formattedDate, nil
}

// NewEmployeeCheckin creates a new check-in record with default values
func NewEmployeeCheckin(employeeID string, serverTimeMillis int64) (*EmployeeCheckin, string) {
	// Generate a unique name with timestamp and random characters
	uniqueName := fmt.Sprintf("new-employee-checkin-%s", generateUniqueID())

	// Try to get Vietnam time first
	var formattedTime string
	vietTime, err := GetVietnamTime()
	if err == nil {
		// Format Vietnam time in YYYY-MM-DD HH:MM:SS format for ERP
		formattedTime = vietTime.Format("2006-01-02 15:04:05")
	} else {
		// Fallback to server time if Vietnam time fails
		serverTime := time.UnixMilli(serverTimeMillis)
		formattedTime = serverTime.Format("2006-01-02 15:04:05")
	}

	return &EmployeeCheckin{
		Docstatus:          1, // Set to 1 to submit the document
		Doctype:            "Employee Checkin",
		Name:               uniqueName,
		IsLocal:            true,
		Unsaved:            true,
		Owner:              "demo@example.com",
		LogType:            "IN",
		Time:               formattedTime,
		SkipAutoAttendance: 0,
		Offshift:           0,
		EmployeeName:       employeeID, // This should be the ERPNext employee ID
		Employee:           employeeID, // This should be the ERPNext employee ID
	}, formattedTime
}

// NewEmployeeCheckout creates a new check-out record with default values
func NewEmployeeCheckout(employeeID string, serverTimeMillis int64) (*EmployeeCheckin, string) {
	// Generate a unique name with timestamp and random characters
	uniqueName := fmt.Sprintf("new-employee-checkout-%s", generateUniqueID())

	// Try to get Vietnam time first
	var formattedTime string
	vietTime, err := GetVietnamTime()
	if err == nil {
		// Format Vietnam time in YYYY-MM-DD HH:MM:SS format for ERP
		formattedTime = vietTime.Format("2006-01-02 15:04:05")
	} else {
		// Fallback to server time if Vietnam time fails
		serverTime := time.UnixMilli(serverTimeMillis)
		formattedTime = serverTime.Format("2006-01-02 15:04:05")
	}

	return &EmployeeCheckin{
		Docstatus:          1, // Set to 1 to submit the document
		Doctype:            "Employee Checkin",
		Name:               uniqueName,
		IsLocal:            true,
		Unsaved:            true,
		Owner:              "demo@example.com",
		LogType:            "OUT",
		Time:               formattedTime,
		SkipAutoAttendance: 0,
		Offshift:           0,
		EmployeeName:       employeeID, // This should be the ERPNext employee ID
		Employee:           employeeID, // This should be the ERPNext employee ID
	}, formattedTime
}

// NewEmployeeAttendance creates a new attendance record for absence
func NewEmployeeAttendance(employeeID string, reason string, serverTimeMillis int64) (*EmployeeAttendance, string) {
	// Generate a unique name with timestamp and random characters
	uniqueName := fmt.Sprintf("new-employee-attendance-%s", generateUniqueID())

	// Try to get Vietnam time first
	var formattedDate string
	vietTime, err := GetVietnamTime()
	if err == nil {
		// Format Vietnam time in YYYY-MM-DD format for ERP
		formattedDate = vietTime.Format("2006-01-02")
	} else {
		// Fallback to server time if Vietnam time fails
		serverTime := time.UnixMilli(serverTimeMillis)
		formattedDate = serverTime.Format("2006-01-02")
	}

	return &EmployeeAttendance{
		Docstatus:      1, // Set to 1 to submit the document
		Doctype:        "Attendance",
		Name:           uniqueName,
		IsLocal:        true,
		Unsaved:        true,
		Owner:          "demo@example.com",
		Employee:       employeeID,
		EmployeeName:   employeeID,
		AttendanceDate: formattedDate,
		Status:         "Absent",
		Reason:         reason,
		Company:        "", // You may want to set this based on your ERP setup
	}, formattedDate
}

// generateUniqueID creates a simple unique ID for the checkin record
func generateUniqueID() string {
	const letters = "abcdefghijklmnopqrstuvwxyz"
	result := make([]byte, 10)
	for i := range result {
		result[i] = letters[time.Now().UnixNano()%int64(len(letters))]
	}
	return string(result)
}

// GetVietnamTime returns the current time in Vietnam timezone (Asia/Ho_Chi_Minh)
func GetVietnamTime() (time.Time, error) {
	// Load Vietnam timezone (Ho Chi Minh City)
	loc, err := time.LoadLocation("Asia/Ho_Chi_Minh")
	if err != nil {
		return time.Time{}, err
	}

	// Get current time in Vietnam timezone
	return time.Now().In(loc), nil
}

// SendNotification sends a notification about attendance events
func (m *AttendanceModule) SendNotification(userID, employeeName string, eventType RollCallEventType, eventTime string, reason string) error {
	m.api.LogDebug("Sending roll call notification",
		"user_id", userID,
		"event_type", string(eventType),
		"time", eventTime)

	// Check if roll call is enabled
	if !m.config.Enabled {
		m.api.LogDebug("Roll call is disabled, skipping notification")
		return nil
	}

	// Get configured notification channels
	notifyChannelIDs := m.config.NotifyChannels
	if len(notifyChannelIDs) == 0 {
		m.api.LogDebug("No notification channels configured for roll call")
		return nil
	}

	// Get the user's details for more personalized messages
	user, err := m.api.GetUser(userID)
	if err != nil {
		return err
	}

	// Create notification message based on user's locale
	defaultLocale := *m.api.GetConfig().LocalizationSettings.DefaultServerLocale
	var message string

	switch eventType {
	case RollCallEventCheckIn:
		message = m.i18n.Localize("rollcall.notification.checkin", "**%s** has checked in at %s", defaultLocale, employeeName, eventTime)
	case RollCallEventCheckOut:
		message = m.i18n.Localize("rollcall.notification.checkout", "**%s** has checked out at %s", defaultLocale, employeeName, eventTime)
	case RollCallEventAbsent:
		message = m.i18n.Localize("rollcall.notification.absent", "**%s** has reported absence for today: \"%s\"", defaultLocale, employeeName, reason)
	}

	// Send to configured notification channels only
	for _, channelID := range notifyChannelIDs {
		post := &model.Post{
			ChannelId: channelID,
			Message:   message,
		}

		if err := m.api.CreatePost(post); err != nil {
			m.api.LogError("Failed to send roll call notification",
				"channel_id", channelID,
				"error", err.Error())
		}
	}

	// Send personalized message to the user via LLM
	if err := m.sendPersonalizedRollCallMessage(user, eventType, eventTime); err != nil {
		m.api.LogError("Failed to send personalized message",
			"user_id", userID,
			"error", err.Error())
	}

	return nil
}

// sendPersonalizedRollCallMessage sends a personalized message to the user using the LLM
func (m *AttendanceModule) sendPersonalizedRollCallMessage(user *model.User, eventType RollCallEventType, eventTime string) error {
	// Set up context for LLM
	context := &llm.Context{
		RequestingUser: user,
	}

	// Current time info for more context
	vietTime, err := GetVietnamTime()
	if err != nil {
		vietTime = time.Now()
	}

	timeOfDay := getTimeOfDay(vietTime)
	dayOfWeek := vietTime.Weekday().String()

	// Determine language for prompts based on user locale
	isVietnamese := strings.HasPrefix(user.Locale, "vi")

	// Get Vietnamese day of week if needed
	vietnameseDayOfWeek := dayOfWeek
	if isVietnamese {
		vietnameseDayOfWeek = getVietnameseDayOfWeek(vietTime.Weekday())
	}

	// Build parameters for LLM
	context.Parameters = map[string]any{
		"EventType":           string(eventType),
		"EventTime":           eventTime,
		"TimeOfDay":           timeOfDay,
		"DayOfWeek":           dayOfWeek,
		"VietnameseDayOfWeek": vietnameseDayOfWeek,
		"UserName":            getUserDisplayName(user),
		"IsCheckIn":           eventType == RollCallEventCheckIn,
		"IsCheckOut":          eventType == RollCallEventCheckOut,
		"IsVietnamese":        isVietnamese,
	}

	// Define the prompt based on event type and language
	var promptText string

	switch eventType {
	case RollCallEventCheckIn:
		if isVietnamese {
			promptText = `Bạn là trợ lý thân thiện tại nơi làm việc. Tạo một tin nhắn chào mừng NGẮN GỌN, HIỆN ĐẠI và TÍCH CỰC (chỉ 1-2 câu) 
cho {{.UserName}} vừa điểm danh vào làm lúc {{.EventTime}}. 
Hiện tại là {{.TimeOfDay}} vào {{.VietnameseDayOfWeek}}. 
Làm cho nó nghe chuyên nghiệp nhưng thân thiện. KHÔNG SỬ DỤNG QUÁ 2 CÂU. Sử dụng tiếng Việt.`
		} else {
			promptText = `You are a friendly workplace assistant. Generate a SHORT, MODERN, and ENERGETIC welcome message (1-2 sentences only) 
for {{.UserName}} who just checked in to work at {{.EventTime}}. 
It's currently {{.TimeOfDay}} on {{.DayOfWeek}}. 
Make it sound professional but friendly. DO NOT USE MORE THAN 2 SENTENCES. Use English.`
		}

	case RollCallEventCheckOut:
		if isVietnamese {
			promptText = `Bạn là trợ lý thân thiện tại nơi làm việc. Tạo một tin nhắn tạm biệt NGẮN GỌN, HIỆN ĐẠI và THÂN THIỆN (chỉ 1-2 câu) 
cho {{.UserName}} vừa điểm danh ra về lúc {{.EventTime}}. 
Hiện tại là {{.TimeOfDay}} vào {{.VietnameseDayOfWeek}}. 
Chúc họ có thời gian nghỉ ngơi vui vẻ. KHÔNG SỬ DỤNG QUÁ 2 CÂU. Sử dụng tiếng Việt.`
		} else {
			promptText = `You are a friendly workplace assistant. Generate a SHORT, MODERN, and FRIENDLY goodbye message (1-2 sentences only) 
for {{.UserName}} who just checked out from work at {{.EventTime}}. 
It's currently {{.TimeOfDay}} on {{.DayOfWeek}}. 
Wish them a pleasant time off. DO NOT USE MORE THAN 2 SENTENCES. Use English.`
		}

	default:
		return fmt.Errorf("unsupported event type for personalized message")
	}

	// Replace template variables in the prompt
	processedPrompt, err := processTemplate(promptText, context.Parameters)
	if err != nil {
		return fmt.Errorf("failed to process template: %w", err)
	}

	// Use the LLM to generate the personalized message
	messageRequest := llm.CompletionRequest{
		Posts: []llm.Post{
			{
				Role:    llm.PostRoleSystem,
				Message: processedPrompt,
			},
		},
		Context: context,
	}

	result, err := m.getLLM().ChatCompletionNoStream(messageRequest)
	if err != nil {
		return fmt.Errorf("failed to generate personalized message: %w", err)
	}

	// Send the message to the user
	post := &model.Post{
		Message: result,
	}

	if err := m.api.BotDMNonResponse(m.botUserID, user.Id, post); err != nil {
		return fmt.Errorf("failed to send personalized DM: %w", err)
	}

	return nil
}

func processTemplate(templateText string, data map[string]any) (string, error) {
	tmpl, err := template.New("prompt").Parse(templateText)
	if err != nil {
		return "", fmt.Errorf("failed to parse template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("failed to execute template: %w", err)
	}

	return buf.String(), nil
}

// getTimeOfDay returns a string describing the time of day
func getTimeOfDay(t time.Time) string {
	hour := t.Hour()

	switch {
	case hour >= 5 && hour < 12:
		return "morning"
	case hour >= 12 && hour < 17:
		return "afternoon"
	case hour >= 17 && hour < 21:
		return "evening"
	default:
		return "night"
	}
}

// getVietnameseDayOfWeek returns Vietnamese day of week
func getVietnameseDayOfWeek(weekday time.Weekday) string {
	switch weekday {
	case time.Sunday:
		return "Chủ nhật"
	case time.Monday:
		return "Thứ hai"
	case time.Tuesday:
		return "Thứ ba"
	case time.Wednesday:
		return "Thứ tư"
	case time.Thursday:
		return "Thứ năm"
	case time.Friday:
		return "Thứ sáu"
	case time.Saturday:
		return "Thứ bảy"
	default:
		return "Chủ nhật"
	}
}

// getUserDisplayName returns the best display name for a user
func getUserDisplayName(user *model.User) string {
	if user.FirstName != "" {
		return user.FirstName
	}
	if user.Nickname != "" {
		return user.Nickname
	}
	return user.Username
}
