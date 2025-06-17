// Copyright (c) 2023-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package attendance

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/mattermost/mattermost-plugin-ai/server/erp_modules"
	"github.com/mattermost/mattermost-plugin-ai/server/llm"
	"github.com/mattermost/mattermost/server/public/model"
)

// AttendanceModule handles attendance-related operations
type AttendanceModule struct {
	config              AttendanceConfig
	erpClient           *ERPClient
	notificationManager *NotificationManager
	api                 PluginAPI
	prompts             PromptsInterface
	getLLM              func() llm.LanguageModel
}

// AttendanceQueryRequest represents the structured query request
type AttendanceQueryRequest struct {
	Status     string `json:"status"`
	TimePeriod struct {
		Type        string `json:"type"`
		StartDate   string `json:"start_date"`
		EndDate     string `json:"end_date"`
		Description string `json:"description"`
	} `json:"time_period"`
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
	// Create ERP client
	erpClient := NewERPClient(config, httpClient, api)

	// Create notification manager
	notificationManager := NewNotificationManager(config, i18n, prompts, getLLM, api, botUserID)

	return &AttendanceModule{
		config:              config,
		erpClient:           erpClient,
		notificationManager: notificationManager,
		api:                 api,
		prompts:             prompts,
		getLLM:              getLLM,
	}
}

// GetCategory returns the category this module handles
func (m *AttendanceModule) GetCategory() string {
	return "attendance"
}

// GetSupportedActions returns list of actions this module supports
func (m *AttendanceModule) GetSupportedActions() []string {
	return []string{"check_in", "check_out", "absent", "get_status_count"}
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
	case "get_status_count":
		return m.handleGetStatusCount(employeeID, ctx, intent)
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
	return "Quản lý điểm danh nhân viên: check-in, check-out, báo nghỉ phép, và truy vấn báo cáo chấm công"
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
		"get_status_count": {
			"tôi nghỉ bao nhiêu ngày",
			"how many days was I absent",
			"báo cáo chấm công",
			"số ngày nghỉ tháng này",
			"attendance report this month",
			"tôi đi làm mấy ngày tuần trước",
			"show my present days",
			"xem số ngày có mặt",
		},
	}
}

// handleCheckIn processes check-in request
func (m *AttendanceModule) handleCheckIn(employeeID, employeeName, userID string) (*erp_modules.ModuleResponse, error) {
	formattedTime, err := m.erpClient.RecordEmployeeCheckin(employeeID)
	if err != nil {
		return &erp_modules.ModuleResponse{
			Success: false,
			Message: "⚠️ Có lỗi xảy ra khi ghi nhận check-in. Vui lòng thử lại.",
			Error:   err.Error(),
		}, nil
	}

	// Send notification asynchronously
	go func() {
		_ = m.notificationManager.SendNotification(userID, employeeName, RollCallEventCheckIn, formattedTime, "")
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
	formattedTime, err := m.erpClient.RecordEmployeeCheckout(employeeID)
	if err != nil {
		return &erp_modules.ModuleResponse{
			Success: false,
			Message: "⚠️ Có lỗi xảy ra khi ghi nhận check-out. Vui lòng thử lại.",
			Error:   err.Error(),
		}, nil
	}

	// Send notification asynchronously
	go func() {
		_ = m.notificationManager.SendNotification(userID, employeeName, RollCallEventCheckOut, formattedTime, "")
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
	recordedDate, err := m.erpClient.RecordEmployeeAbsent(employeeID, reason)
	if err != nil {
		return &erp_modules.ModuleResponse{
			Success: false,
			Message: "⚠️ Có lỗi xảy ra khi ghi nhận nghỉ phép. Vui lòng thử lại.",
			Error:   err.Error(),
		}, nil
	}

	// Send notification asynchronously
	go func() {
		_ = m.notificationManager.SendNotification(userID, employeeName, RollCallEventAbsent, recordedDate, reason)
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

// handleGetStatusCount processes attendance status count queries
func (m *AttendanceModule) handleGetStatusCount(employeeID string, ctx *erp_modules.ModuleContext, intent *erp_modules.Intent) (*erp_modules.ModuleResponse, error) {
	// Use LLM to analyze the user's query and extract structured request
	queryRequest, err := m.analyzeAttendanceQuery(ctx, intent.RawMessage)
	if err != nil {
		return &erp_modules.ModuleResponse{
			Success: false,
			Message: "⚠️ Không thể hiểu được yêu cầu của bạn. Vui lòng thử lại với câu hỏi rõ ràng hơn.",
			Error:   err.Error(),
		}, nil
	}

	// Query ERPNext for attendance records
	count, err := m.erpClient.QueryAttendanceCount(employeeID, queryRequest.Status, queryRequest.TimePeriod.StartDate, queryRequest.TimePeriod.EndDate)
	if err != nil {
		return &erp_modules.ModuleResponse{
			Success: false,
			Message: "⚠️ Có lỗi xảy ra khi truy vấn dữ liệu chấm công. Vui lòng thử lại.",
			Error:   err.Error(),
		}, nil
	}

	// Generate natural language response using LLM
	response, err := m.generateQueryResponse(ctx, intent.RawMessage, queryRequest, count)
	if err != nil {
		// Fallback to simple response if LLM fails
		response = fmt.Sprintf("📅 Bạn có %d ngày %s trong %s", count, queryRequest.Status, queryRequest.TimePeriod.Description)
	}

	return &erp_modules.ModuleResponse{
		Success:     true,
		Message:     response,
		ActionTaken: "get_status_count",
		Data: map[string]interface{}{
			"count":       count,
			"status":      queryRequest.Status,
			"start_date":  queryRequest.TimePeriod.StartDate,
			"end_date":    queryRequest.TimePeriod.EndDate,
			"description": queryRequest.TimePeriod.Description,
		},
	}, nil
}

// analyzeAttendanceQuery uses LLM to convert natural language to structured request
func (m *AttendanceModule) analyzeAttendanceQuery(ctx *erp_modules.ModuleContext, userMessage string) (*AttendanceQueryRequest, error) {
	// Create LLM context
	llmContext := &llm.Context{
		RequestingUser: ctx.User,
		Time:           time.Now().Format(time.RFC1123),
	}
	llmContext.Parameters = map[string]interface{}{
		"UserMessage": userMessage,
	}

	// Format the query analysis prompt
	systemPrompt, err := m.prompts.Format("attendance_query_analysis", llmContext)
	if err != nil {
		return nil, fmt.Errorf("failed to format query analysis prompt: %w", err)
	}

	// Create completion request
	completionRequest := llm.CompletionRequest{
		Posts: []llm.Post{
			{
				Role:    llm.PostRoleSystem,
				Message: systemPrompt,
			},
			{
				Role:    llm.PostRoleUser,
				Message: userMessage,
			},
		},
		Context: llmContext,
	}

	// Get LLM response
	response, err := m.getLLM().ChatCompletionNoStream(completionRequest, llm.WithMaxGeneratedTokens(300))
	if err != nil {
		return nil, fmt.Errorf("failed to analyze query with LLM: %w", err)
	}

	// Parse JSON response
	var queryRequest AttendanceQueryRequest

	// Clean response to extract JSON
	response = strings.TrimSpace(response)
	start := strings.Index(response, "{")
	end := strings.LastIndex(response, "}") + 1

	if start == -1 || end <= start {
		return nil, fmt.Errorf("no valid JSON found in LLM response: %s", response)
	}

	jsonStr := response[start:end]
	if err := json.Unmarshal([]byte(jsonStr), &queryRequest); err != nil {
		return nil, fmt.Errorf("failed to parse LLM response as JSON: %w", err)
	}

	return &queryRequest, nil
}

// generateQueryResponse uses LLM to format the query results into natural language
func (m *AttendanceModule) generateQueryResponse(ctx *erp_modules.ModuleContext, userMessage string, queryRequest *AttendanceQueryRequest, count int) (string, error) {
	// Create LLM context
	llmContext := &llm.Context{
		RequestingUser: ctx.User,
	}
	llmContext.Parameters = map[string]interface{}{
		"UserMessage":  userMessage,
		"QueryDetails": fmt.Sprintf("Status: %s, Period: %s", queryRequest.Status, queryRequest.TimePeriod.Description),
		"Count":        count,
		"TimePeriod":   queryRequest.TimePeriod.Description,
		"Status":       queryRequest.Status,
	}

	// Format the response generation prompt
	systemPrompt, err := m.prompts.Format("attendance_query_response", llmContext)
	if err != nil {
		return "", fmt.Errorf("failed to format response prompt: %w", err)
	}

	// Create completion request
	completionRequest := llm.CompletionRequest{
		Posts: []llm.Post{
			{
				Role:    llm.PostRoleSystem,
				Message: systemPrompt,
			},
			{
				Role:    llm.PostRoleUser,
				Message: fmt.Sprintf("Generate response for: %s (Count: %d)", userMessage, count),
			},
		},
		Context: llmContext,
	}

	// Get LLM response
	response, err := m.getLLM().ChatCompletionNoStream(completionRequest, llm.WithMaxGeneratedTokens(200))
	if err != nil {
		return "", fmt.Errorf("failed to generate response with LLM: %w", err)
	}

	return strings.TrimSpace(response), nil
}

// GetEmployeeIDFromUser gets employee ID from user
func (m *AttendanceModule) GetEmployeeIDFromUser(user *model.User) (string, error) {
	// Use the user's ID as the chat ID to lookup in ERPNext
	chatID := user.Id

	employeeID, err := m.erpClient.GetEmployeeByChatID(chatID)
	if err != nil {
		return "", fmt.Errorf("failed to get employee by chat ID %s: %w", chatID, err)
	}

	return employeeID, nil
}

// SendNotification is a legacy method that delegates to the notification manager
// This method is kept for backward compatibility
func (m *AttendanceModule) SendNotification(userID, employeeName string, eventType RollCallEventType, eventTime string, reason string) error {
	return m.notificationManager.SendNotification(userID, employeeName, eventType, eventTime, reason)
}
