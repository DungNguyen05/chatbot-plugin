// Copyright (c) 2023-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package attendance

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
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
	confirmationState   map[string]*AttendanceConfirmation
	confirmationMutex   sync.RWMutex
}

// AttendanceConfirmation represents pending attendance confirmation
type AttendanceConfirmation struct {
	UserID     string `json:"user_id"`
	Action     string `json:"action"` // "check_in", "check_out", "absent"
	Reason     string `json:"reason,omitempty"`
	CreatedAt  int64  `json:"created_at"`
	EmployeeID string `json:"employee_id"`
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

// detectUserLanguage determines if user prefers Vietnamese or English
func detectUserLanguage(user *model.User) bool {
	return strings.HasPrefix(user.Locale, "vi")
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
		confirmationState:   make(map[string]*AttendanceConfirmation),
	}
}

// GetCategory returns the category this module handles
func (m *AttendanceModule) GetCategory() string {
	return "attendance"
}

// GetSupportedActions returns list of actions this module supports
func (m *AttendanceModule) GetSupportedActions() []string {
	return []string{"check_in", "check_out", "absent", "get_status_count", "get_attendance_report"}
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

// ProcessUserMessage handles user messages including confirmations
func (m *AttendanceModule) ProcessUserMessage(ctx *erp_modules.ModuleContext, message string) (*erp_modules.ModuleResponse, error) {
	m.confirmationMutex.RLock()
	pending, hasPending := m.confirmationState[ctx.User.Id]
	m.confirmationMutex.RUnlock()

	if !hasPending {
		return nil, nil // Not handling this message
	}

	// Parse user response using LLM
	confirmed, err := m.parseConfirmationResponse(ctx, message)
	if err != nil {
		m.api.LogError("Failed to parse confirmation response", "error", err.Error())
		isVietnamese := detectUserLanguage(ctx.User)
		errorMsg := "⚠️ Không thể hiểu phản hồi của bạn. Vui lòng trả lời 'có' để xác nhận hoặc 'không' để hủy bỏ."
		if !isVietnamese {
			errorMsg = "⚠️ Cannot understand your response. Please reply 'yes' to confirm or 'no' to cancel."
		}
		return &erp_modules.ModuleResponse{
			Success: false,
			Message: errorMsg,
		}, nil
	}

	// Clear pending confirmation
	m.clearPendingConfirmation(ctx.User.Id)

	if confirmed {
		return m.executeConfirmedAction(ctx, pending)
	} else {
		isVietnamese := detectUserLanguage(ctx.User)
		cancelMsg := "❌ Đã hủy bỏ yêu cầu."
		if !isVietnamese {
			cancelMsg = "❌ Request cancelled."
		}
		return &erp_modules.ModuleResponse{
			Success:     true,
			Message:     cancelMsg,
			ActionTaken: "cancel_confirmation",
		}, nil
	}
}

// Execute processes the attendance intent
func (m *AttendanceModule) Execute(ctx *erp_modules.ModuleContext, intent *erp_modules.Intent) (*erp_modules.ModuleResponse, error) {
	isVietnamese := detectUserLanguage(ctx.User)

	// Execute specific action based on intent.Action
	switch intent.Action {
	case "check_in":
		// Get employee ID for check-in/check-out actions
		employeeID, err := m.getEmployeeIDFromUser(ctx.User)
		if err != nil {
			errorMsg := "❌ Không tìm thấy thông tin nhân viên của bạn trong hệ thống ERP. Vui lòng liên hệ quản trị viên."
			if !isVietnamese {
				errorMsg = "❌ Cannot find your employee information in the ERP system. Please contact administrator."
			}
			return &erp_modules.ModuleResponse{
				Success: false,
				Message: errorMsg,
				Error:   err.Error(),
			}, nil
		}
		return m.handleCheckInRequest(employeeID, ctx, intent)

	case "check_out":
		// Get employee ID for check-in/check-out actions
		employeeID, err := m.getEmployeeIDFromUser(ctx.User)
		if err != nil {
			errorMsg := "❌ Không tìm thấy thông tin nhân viên của bạn trong hệ thống ERP. Vui lòng liên hệ quản trị viên."
			if !isVietnamese {
				errorMsg = "❌ Cannot find your employee information in the ERP system. Please contact administrator."
			}
			return &erp_modules.ModuleResponse{
				Success: false,
				Message: errorMsg,
				Error:   err.Error(),
			}, nil
		}
		return m.handleCheckOutRequest(employeeID, ctx, intent)

	case "absent":
		// Get employee ID for absence reporting
		employeeID, err := m.getEmployeeIDFromUser(ctx.User)
		if err != nil {
			errorMsg := "❌ Không tìm thấy thông tin nhân viên của bạn trong hệ thống ERP. Vui lòng liên hệ quản trị viên."
			if !isVietnamese {
				errorMsg = "❌ Cannot find your employee information in the ERP system. Please contact administrator."
			}
			return &erp_modules.ModuleResponse{
				Success: false,
				Message: errorMsg,
				Error:   err.Error(),
			}, nil
		}
		reason := ""
		if reason == "" {
			if isVietnamese {
				reason = "Không có lý do cụ thể"
			} else {
				reason = "No specific reason"
			}
		}
		return m.handleAbsentRequest(employeeID, ctx, intent, reason)

	case "get_status_count":
		// Get employee ID for status count queries
		employeeID, err := m.getEmployeeIDFromUser(ctx.User)
		if err != nil {
			errorMsg := "❌ Không tìm thấy thông tin nhân viên của bạn trong hệ thống ERP. Vui lòng liên hệ quản trị viên."
			if !isVietnamese {
				errorMsg = "❌ Cannot find your employee information in the ERP system. Please contact administrator."
			}
			return &erp_modules.ModuleResponse{
				Success: false,
				Message: errorMsg,
				Error:   err.Error(),
			}, nil
		}
		return m.handleGetStatusCountRequest(employeeID, ctx, intent)

	case "get_attendance_report":
		// For attendance reports, we don't need the current user's employee ID
		// since we'll be searching for other employees
		return m.handleGetAttendanceReportRequest(ctx, intent)

	default:
		errorMsg := "Hành động không được hỗ trợ"
		if !isVietnamese {
			errorMsg = "Action not supported"
		}
		return &erp_modules.ModuleResponse{
			Success: false,
			Message: errorMsg,
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
		"get_attendance_report": {
			"báo cáo chấm công của Minh",
			"attendance report for Minh",
			"xem chấm công của Minh và An",
			"báo cáo điểm danh team",
			"attendance của tất cả nhân viên",
			"báo cáo chấm công toàn bộ",
			"xem chấm công Nguyễn Văn An",
			"attendance report all employees",
			"chấm công của nhân viên",
			"điểm danh báo cáo",
		},
	}
}

// handleCheckInRequest processes check-in request with optional confirmation
func (m *AttendanceModule) handleCheckInRequest(employeeID string, ctx *erp_modules.ModuleContext, intent *erp_modules.Intent) (*erp_modules.ModuleResponse, error) {
	// For check-in, we can execute directly with high confidence
	if intent.Confidence >= 0.9 {
		return m.handleCheckIn(employeeID, getUserDisplayName(ctx.User), ctx.User.Id)
	}

	// Request confirmation for medium confidence
	return m.requestConfirmation(ctx, "check_in", "", employeeID)
}

// handleCheckOutRequest processes check-out request with optional confirmation
func (m *AttendanceModule) handleCheckOutRequest(employeeID string, ctx *erp_modules.ModuleContext, intent *erp_modules.Intent) (*erp_modules.ModuleResponse, error) {
	// For check-out, we can execute directly with high confidence
	if intent.Confidence >= 0.9 {
		return m.handleCheckOut(employeeID, getUserDisplayName(ctx.User), ctx.User.Id)
	}

	// Request confirmation for medium confidence
	return m.requestConfirmation(ctx, "check_out", "", employeeID)
}

// handleAbsentRequest processes absent request with confirmation
func (m *AttendanceModule) handleAbsentRequest(employeeID string, ctx *erp_modules.ModuleContext, intent *erp_modules.Intent, reason string) (*erp_modules.ModuleResponse, error) {
	// Always request confirmation for absence reporting as it's important
	return m.requestConfirmation(ctx, "absent", reason, employeeID)
}

// handleGetStatusCountRequest processes attendance status count queries
func (m *AttendanceModule) handleGetStatusCountRequest(employeeID string, ctx *erp_modules.ModuleContext, intent *erp_modules.Intent) (*erp_modules.ModuleResponse, error) {
	isVietnamese := detectUserLanguage(ctx.User)

	// Use LLM to analyze the user's query and extract structured request
	queryRequest, err := m.analyzeAttendanceQuery(ctx, intent.RawMessage)
	if err != nil {
		errorMsg := "⚠️ Không thể hiểu được yêu cầu của bạn. Vui lòng thử lại với câu hỏi rõ ràng hơn."
		if !isVietnamese {
			errorMsg = "⚠️ Cannot understand your request. Please try again with a clearer question."
		}
		return &erp_modules.ModuleResponse{
			Success: false,
			Message: errorMsg,
			Error:   err.Error(),
		}, nil
	}

	// Query ERPNext for attendance records
	count, err := m.erpClient.QueryAttendanceCount(employeeID, queryRequest.Status, queryRequest.TimePeriod.StartDate, queryRequest.TimePeriod.EndDate)
	if err != nil {
		errorMsg := "⚠️ Có lỗi xảy ra khi truy vấn dữ liệu chấm công. Vui lòng thử lại."
		if !isVietnamese {
			errorMsg = "⚠️ An error occurred while querying attendance data. Please try again."
		}
		return &erp_modules.ModuleResponse{
			Success: false,
			Message: errorMsg,
			Error:   err.Error(),
		}, nil
	}

	// Generate natural language response using LLM
	response, err := m.generateQueryResponse(ctx, intent.RawMessage, queryRequest, count)
	if err != nil {
		// Fallback to simple response if LLM fails
		if isVietnamese {
			response = fmt.Sprintf("Bạn có %d ngày %s trong %s", count, queryRequest.Status, queryRequest.TimePeriod.Description)
		} else {
			response = fmt.Sprintf("You have %d %s days in %s", count, queryRequest.Status, queryRequest.TimePeriod.Description)
		}
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

// requestConfirmation requests confirmation from user
func (m *AttendanceModule) requestConfirmation(ctx *erp_modules.ModuleContext, action, reason, employeeID string) (*erp_modules.ModuleResponse, error) {
	// Store pending confirmation
	m.confirmationMutex.Lock()
	m.confirmationState[ctx.User.Id] = &AttendanceConfirmation{
		UserID:     ctx.User.Id,
		Action:     action,
		Reason:     reason,
		CreatedAt:  time.Now().UnixMilli(),
		EmployeeID: employeeID,
	}
	m.confirmationMutex.Unlock()

	// Generate confirmation message
	confirmationMsg, err := m.generateConfirmationMessage(ctx, action, reason)
	if err != nil {
		isVietnamese := detectUserLanguage(ctx.User)
		errorMsg := "⚠️ Có lỗi xảy ra khi tạo tin nhắn xác nhận."
		if !isVietnamese {
			errorMsg = "⚠️ An error occurred while creating confirmation message."
		}
		return &erp_modules.ModuleResponse{
			Success: false,
			Message: errorMsg,
			Error:   err.Error(),
		}, nil
	}

	return &erp_modules.ModuleResponse{
		Success:     true,
		Message:     confirmationMsg,
		ActionTaken: "request_confirmation",
		Data: map[string]interface{}{
			"awaiting_confirmation": true,
			"action":                action,
		},
	}, nil
}

// generateConfirmationMessage generates confirmation message using LLM
func (m *AttendanceModule) generateConfirmationMessage(ctx *erp_modules.ModuleContext, action, reason string) (string, error) {
	// Create LLM context
	llmContext := &llm.Context{
		RequestingUser: ctx.User,
		Time:           time.Now().Format(time.RFC1123),
	}

	// Detect user language
	isVietnamese := detectUserLanguage(ctx.User)

	llmContext.Parameters = map[string]interface{}{
		"Action":       action,
		"Reason":       reason,
		"UserName":     getUserDisplayName(ctx.User),
		"IsVietnamese": isVietnamese,
	}

	// Format the confirmation prompt
	systemPrompt, err := m.prompts.Format("attendance_confirmation_generation", llmContext)
	if err != nil {
		return "", fmt.Errorf("failed to format confirmation prompt: %w", err)
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
				Message: fmt.Sprintf("Generate confirmation for %s action", action),
			},
		},
		Context: llmContext,
	}

	// Get LLM response
	response, err := m.getLLM().ChatCompletionNoStream(completionRequest, llm.WithMaxGeneratedTokens(200))
	if err != nil {
		return "", fmt.Errorf("failed to generate confirmation with LLM: %w", err)
	}

	return strings.TrimSpace(response), nil
}

// parseConfirmationResponse parses user confirmation response using LLM
func (m *AttendanceModule) parseConfirmationResponse(ctx *erp_modules.ModuleContext, message string) (bool, error) {
	// Create LLM context
	llmContext := &llm.Context{
		RequestingUser: ctx.User,
	}

	llmContext.Parameters = map[string]interface{}{
		"UserMessage": message,
	}

	// Format the confirmation analysis prompt
	systemPrompt, err := m.prompts.Format("attendance_confirmation_analysis", llmContext)
	if err != nil {
		return false, fmt.Errorf("failed to format confirmation analysis prompt: %w", err)
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
				Message: message,
			},
		},
		Context: llmContext,
	}

	// Get LLM response
	response, err := m.getLLM().ChatCompletionNoStream(completionRequest, llm.WithMaxGeneratedTokens(150))
	if err != nil {
		return false, fmt.Errorf("failed to analyze confirmation with LLM: %w", err)
	}

	// Parse JSON response
	var confirmResult struct {
		Confirmed bool   `json:"confirmed"`
		Reasoning string `json:"reasoning"`
	}

	// Clean and parse JSON response
	response = strings.TrimSpace(response)
	start := strings.Index(response, "{")
	end := strings.LastIndex(response, "}") + 1

	if start == -1 || end <= start {
		// If can't parse, assume denial for safety
		return false, nil
	}

	jsonStr := response[start:end]
	if err := json.Unmarshal([]byte(jsonStr), &confirmResult); err != nil {
		// If can't parse, assume denial for safety
		return false, nil
	}

	return confirmResult.Confirmed, nil
}

// executeConfirmedAction executes the confirmed action
func (m *AttendanceModule) executeConfirmedAction(ctx *erp_modules.ModuleContext, pending *AttendanceConfirmation) (*erp_modules.ModuleResponse, error) {
	employeeName := getUserDisplayName(ctx.User)

	switch pending.Action {
	case "check_in":
		return m.handleCheckIn(pending.EmployeeID, employeeName, ctx.User.Id)
	case "check_out":
		return m.handleCheckOut(pending.EmployeeID, employeeName, ctx.User.Id)
	case "absent":
		return m.handleAbsent(pending.EmployeeID, employeeName, ctx.User.Id, pending.Reason)
	default:
		isVietnamese := detectUserLanguage(ctx.User)
		errorMsg := "Hành động không hợp lệ"
		if !isVietnamese {
			errorMsg = "Invalid action"
		}
		return &erp_modules.ModuleResponse{
			Success: false,
			Message: errorMsg,
		}, nil
	}
}

// clearPendingConfirmation clears pending confirmation for a user
func (m *AttendanceModule) clearPendingConfirmation(userID string) {
	m.confirmationMutex.Lock()
	defer m.confirmationMutex.Unlock()
	delete(m.confirmationState, userID)
}

// handleCheckIn processes check-in request
func (m *AttendanceModule) handleCheckIn(employeeID, employeeName, userID string) (*erp_modules.ModuleResponse, error) {
	formattedTime, err := m.erpClient.RecordEmployeeCheckin(employeeID)
	if err != nil {
		user, _ := m.api.GetUser(userID)
		isVietnamese := detectUserLanguage(user)
		errorMsg := "⚠️ Có lỗi xảy ra khi ghi nhận check-in. Vui lòng thử lại."
		if !isVietnamese {
			errorMsg = "⚠️ An error occurred while recording check-in. Please try again."
		}
		return &erp_modules.ModuleResponse{
			Success: false,
			Message: errorMsg,
			Error:   err.Error(),
		}, nil
	}

	// Send notification asynchronously
	go func() {
		_ = m.notificationManager.SendNotification(userID, employeeName, RollCallEventCheckIn, formattedTime, "")
	}()

	user, _ := m.api.GetUser(userID)
	isVietnamese := detectUserLanguage(user)
	successMsg := fmt.Sprintf("Đã ghi nhận check-in của bạn lúc **%s**!", formattedTime)
	if !isVietnamese {
		successMsg = fmt.Sprintf("Successfully recorded your check-in at **%s**!", formattedTime)
	}

	return &erp_modules.ModuleResponse{
		Success:     true,
		Message:     successMsg,
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
		user, _ := m.api.GetUser(userID)
		isVietnamese := detectUserLanguage(user)
		errorMsg := "⚠️ Có lỗi xảy ra khi ghi nhận check-out. Vui lòng thử lại."
		if !isVietnamese {
			errorMsg = "⚠️ An error occurred while recording check-out. Please try again."
		}
		return &erp_modules.ModuleResponse{
			Success: false,
			Message: errorMsg,
			Error:   err.Error(),
		}, nil
	}

	// Send notification asynchronously
	go func() {
		_ = m.notificationManager.SendNotification(userID, employeeName, RollCallEventCheckOut, formattedTime, "")
	}()

	user, _ := m.api.GetUser(userID)
	isVietnamese := detectUserLanguage(user)
	successMsg := fmt.Sprintf("Đã ghi nhận check-out của bạn lúc **%s**!", formattedTime)
	if !isVietnamese {
		successMsg = fmt.Sprintf("Successfully recorded your check-out at **%s**!", formattedTime)
	}

	return &erp_modules.ModuleResponse{
		Success:     true,
		Message:     successMsg,
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
		user, _ := m.api.GetUser(userID)
		isVietnamese := detectUserLanguage(user)
		errorMsg := "⚠️ Có lỗi xảy ra khi ghi nhận nghỉ phép. Vui lòng thử lại."
		if !isVietnamese {
			errorMsg = "⚠️ An error occurred while recording absence. Please try again."
		}
		return &erp_modules.ModuleResponse{
			Success: false,
			Message: errorMsg,
			Error:   err.Error(),
		}, nil
	}

	// Send notification asynchronously
	go func() {
		_ = m.notificationManager.SendNotification(userID, employeeName, RollCallEventAbsent, recordedDate, reason)
	}()

	user, _ := m.api.GetUser(userID)
	isVietnamese := detectUserLanguage(user)
	successMsg := fmt.Sprintf("Đã ghi nhận nghỉ phép của bạn cho ngày **%s** với lý do: \"%s\"", recordedDate, reason)
	if !isVietnamese {
		successMsg = fmt.Sprintf("Successfully recorded your absence for **%s** with reason: \"%s\"", recordedDate, reason)
	}

	return &erp_modules.ModuleResponse{
		Success:     true,
		Message:     successMsg,
		ActionTaken: "absent",
		Data: map[string]interface{}{
			"date":     recordedDate,
			"reason":   reason,
			"employee": employeeName,
		},
	}, nil
}

// handleGetAttendanceReportRequest processes attendance report requests
func (m *AttendanceModule) handleGetAttendanceReportRequest(ctx *erp_modules.ModuleContext, intent *erp_modules.Intent) (*erp_modules.ModuleResponse, error) {
	isVietnamese := detectUserLanguage(ctx.User)

	// Use LLM to analyze the user's query and extract structured request
	reportRequest, err := m.analyzeAttendanceReportQuery(ctx, intent.RawMessage)
	if err != nil {
		errorMsg := "⚠️ Không thể hiểu được yêu cầu báo cáo chấm công. Vui lòng thử lại với câu hỏi rõ ràng hơn."
		if !isVietnamese {
			errorMsg = "⚠️ Cannot understand the attendance report request. Please try again with a clearer question."
		}
		return &erp_modules.ModuleResponse{
			Success: false,
			Message: errorMsg,
			Error:   err.Error(),
		}, nil
	}

	// Generate attendance report based on request type
	switch reportRequest.Type {
	case "by_names":
		return m.generateNameBasedReport(ctx, reportRequest, isVietnamese)
	case "all_employees":
		return m.generateAllEmployeesReport(ctx, reportRequest, isVietnamese)
	default:
		errorMsg := "⚠️ Loại báo cáo không được hỗ trợ."
		if !isVietnamese {
			errorMsg = "⚠️ Unsupported report type."
		}
		return &erp_modules.ModuleResponse{
			Success: false,
			Message: errorMsg,
		}, nil
	}
}

// generateNameBasedReport generates report for specific employees by name
func (m *AttendanceModule) generateNameBasedReport(ctx *erp_modules.ModuleContext, request *AttendanceReportRequest, isVietnamese bool) (*erp_modules.ModuleResponse, error) {
	var allReports []*EmployeeAttendanceReport
	var allMatchedEmployees []Employee

	// Search for employees matching each name
	for _, name := range request.Names {
		matchedEmployees, err := m.erpClient.SearchEmployeesByName(name)
		if err != nil {
			m.api.LogError("Failed to search employees by name", "name", name, "error", err.Error())
			continue
		}
		allMatchedEmployees = append(allMatchedEmployees, matchedEmployees...)
	}

	// Remove duplicates (same employee matched by different names)
	uniqueEmployees := m.removeDuplicateEmployees(allMatchedEmployees)

	if len(uniqueEmployees) == 0 {
		errorMsg := fmt.Sprintf("❌ Không tìm thấy nhân viên nào phù hợp với tên: %s", strings.Join(request.Names, ", "))
		if !isVietnamese {
			errorMsg = fmt.Sprintf("❌ No employees found matching names: %s", strings.Join(request.Names, ", "))
		}
		return &erp_modules.ModuleResponse{
			Success: false,
			Message: errorMsg,
		}, nil
	}

	// Limit results to prevent overwhelming reports
	maxResults := 20
	if len(uniqueEmployees) > maxResults {
		uniqueEmployees = uniqueEmployees[:maxResults]
	}

	// Generate attendance reports for each matched employee
	for _, employee := range uniqueEmployees {
		report, err := m.erpClient.GetAttendanceForEmployee(
			employee.Name,
			request.TimePeriod.StartDate,
			request.TimePeriod.EndDate,
		)
		if err != nil {
			// Create error report for failed employee
			report = &EmployeeAttendanceReport{
				EmployeeID:   employee.Name,
				EmployeeName: employee.EmployeeName,
				ErrorMessage: err.Error(),
			}
		} else {
			report.EmployeeName = employee.EmployeeName
		}
		allReports = append(allReports, report)
	}

	// Generate formatted response
	responseMessage := m.formatAttendanceReports(allReports, request, isVietnamese, "name_based")

	return &erp_modules.ModuleResponse{
		Success:     true,
		Message:     responseMessage,
		ActionTaken: "get_attendance_report",
		Data: map[string]interface{}{
			"report_type":       "by_names",
			"searched_names":    request.Names,
			"employees_found":   len(uniqueEmployees),
			"reports_generated": len(allReports),
			"time_period":       request.TimePeriod.Description,
		},
	}, nil
}

// generateAllEmployeesReport generates report for all employees
func (m *AttendanceModule) generateAllEmployeesReport(ctx *erp_modules.ModuleContext, request *AttendanceReportRequest, isVietnamese bool) (*erp_modules.ModuleResponse, error) {
	// Get all employees from ERP
	allEmployees, err := m.erpClient.GetAllEmployees()
	if err != nil {
		errorMsg := "⚠️ Không thể lấy danh sách nhân viên từ hệ thống ERP."
		if !isVietnamese {
			errorMsg = "⚠️ Cannot retrieve employee list from ERP system."
		}
		return &erp_modules.ModuleResponse{
			Success: false,
			Message: errorMsg,
			Error:   err.Error(),
		}, nil
	}

	if len(allEmployees) == 0 {
		errorMsg := "❌ Không có nhân viên nào trong hệ thống."
		if !isVietnamese {
			errorMsg = "❌ No employees found in the system."
		}
		return &erp_modules.ModuleResponse{
			Success: false,
			Message: errorMsg,
		}, nil
	}

	var allReports []*EmployeeAttendanceReport

	// Generate attendance reports for all employees
	successCount := 0
	for _, employee := range allEmployees {
		report, err := m.erpClient.GetAttendanceForEmployee(
			employee.Name,
			request.TimePeriod.StartDate,
			request.TimePeriod.EndDate,
		)
		if err != nil {
			// Create error report for failed employee
			report = &EmployeeAttendanceReport{
				EmployeeID:   employee.Name,
				EmployeeName: employee.EmployeeName,
				ErrorMessage: err.Error(),
			}
		} else {
			report.EmployeeName = employee.EmployeeName
			successCount++
		}
		allReports = append(allReports, report)
	}

	// Generate formatted response
	responseMessage := m.formatAttendanceReports(allReports, request, isVietnamese, "all_employees")

	return &erp_modules.ModuleResponse{
		Success:     true,
		Message:     responseMessage,
		ActionTaken: "get_attendance_report",
		Data: map[string]interface{}{
			"report_type":        "all_employees",
			"total_employees":    len(allEmployees),
			"successful_reports": successCount,
			"failed_reports":     len(allEmployees) - successCount,
			"time_period":        request.TimePeriod.Description,
		},
	}, nil
}

// removeDuplicateEmployees removes duplicate employees from the list
func (m *AttendanceModule) removeDuplicateEmployees(employees []Employee) []Employee {
	seen := make(map[string]bool)
	var unique []Employee

	for _, emp := range employees {
		if !seen[emp.Name] {
			seen[emp.Name] = true
			unique = append(unique, emp)
		}
	}

	return unique
}

// formatAttendanceReports formats the attendance reports into a readable message
func (m *AttendanceModule) formatAttendanceReports(reports []*EmployeeAttendanceReport, request *AttendanceReportRequest, isVietnamese bool, reportType string) string {
	var message strings.Builder

	// Header
	if reportType == "all_employees" {
		if isVietnamese {
			message.WriteString(fmt.Sprintf("📊 **Báo cáo chấm công tất cả nhân viên** (%s)\n", request.TimePeriod.Description))
		} else {
			message.WriteString(fmt.Sprintf("📊 **All Employees Attendance Report** (%s)\n", request.TimePeriod.Description))
		}
	} else {
		searchedNames := strings.Join(request.Names, ", ")
		if isVietnamese {
			message.WriteString(fmt.Sprintf("📊 **Báo cáo chấm công cho '%s'** (%s)\n", searchedNames, request.TimePeriod.Description))
		} else {
			message.WriteString(fmt.Sprintf("📊 **Attendance Report for '%s'** (%s)\n", searchedNames, request.TimePeriod.Description))
		}
	}

	message.WriteString("\n")

	// Summary statistics
	totalEmployees := len(reports)
	var totalPresent, totalAbsent, totalHalfDay, totalWFH, successfulReports int

	for _, report := range reports {
		if report.ErrorMessage == "" {
			successfulReports++
			totalPresent += report.PresentDays
			totalAbsent += report.AbsentDays
			totalHalfDay += report.HalfDays
			totalWFH += report.WorkFromHomeDays
		}
	}

	if successfulReports > 0 {
		if isVietnamese {
			message.WriteString("**📈 Tổng quan:**\n")
			message.WriteString(fmt.Sprintf("• Tổng số nhân viên: %d\n", totalEmployees))
			message.WriteString(fmt.Sprintf("• Báo cáo thành công: %d\n", successfulReports))
			if totalEmployees > successfulReports {
				message.WriteString(fmt.Sprintf("• Báo cáo lỗi: %d\n", totalEmployees-successfulReports))
			}
		} else {
			message.WriteString("**📈 Summary:**\n")
			message.WriteString(fmt.Sprintf("• Total employees: %d\n", totalEmployees))
			message.WriteString(fmt.Sprintf("• Successful reports: %d\n", successfulReports))
			if totalEmployees > successfulReports {
				message.WriteString(fmt.Sprintf("• Failed reports: %d\n", totalEmployees-successfulReports))
			}
		}
		message.WriteString("\n")
	}

	// Individual employee reports
	if isVietnamese {
		message.WriteString("**👥 Chi tiết từng nhân viên:**\n")
	} else {
		message.WriteString("**👥 Individual Reports:**\n")
	}

	// Limit display to prevent overwhelming messages
	displayLimit := 50
	displayedCount := 0

	for _, report := range reports {
		if displayedCount >= displayLimit {
			remaining := len(reports) - displayedCount
			if isVietnamese {
				message.WriteString(fmt.Sprintf("\n... và %d nhân viên khác. Sử dụng tên cụ thể hơn để thu hẹp kết quả.\n", remaining))
			} else {
				message.WriteString(fmt.Sprintf("\n... and %d more employees. Use more specific names to narrow results.\n", remaining))
			}
			break
		}

		message.WriteString(fmt.Sprintf("\n**%s** (%s)", report.EmployeeName, report.EmployeeID))

		if report.ErrorMessage != "" {
			if isVietnamese {
				message.WriteString(fmt.Sprintf("\n❌ Lỗi: %s", report.ErrorMessage))
			} else {
				message.WriteString(fmt.Sprintf("\n❌ Error: %s", report.ErrorMessage))
			}
		} else {
			if isVietnamese {
				message.WriteString(fmt.Sprintf("\n• Tổng số ngày: %d | Có mặt: %d | Vắng mặt: %d",
					report.TotalDays, report.PresentDays, report.AbsentDays))
				if report.HalfDays > 0 {
					message.WriteString(fmt.Sprintf(" | Nửa ngày: %d", report.HalfDays))
				}
				if report.WorkFromHomeDays > 0 {
					message.WriteString(fmt.Sprintf(" | WFH: %d", report.WorkFromHomeDays))
				}
				if report.LateDays > 0 {
					message.WriteString(fmt.Sprintf(" | Đi muộn: %d", report.LateDays))
				}
			} else {
				message.WriteString(fmt.Sprintf("\n• Total: %d | Present: %d | Absent: %d",
					report.TotalDays, report.PresentDays, report.AbsentDays))
				if report.HalfDays > 0 {
					message.WriteString(fmt.Sprintf(" | Half Day: %d", report.HalfDays))
				}
				if report.WorkFromHomeDays > 0 {
					message.WriteString(fmt.Sprintf(" | WFH: %d", report.WorkFromHomeDays))
				}
				if report.LateDays > 0 {
					message.WriteString(fmt.Sprintf(" | Late: %d", report.LateDays))
				}
			}

			// Calculate attendance percentage
			if report.TotalDays > 0 {
				attendanceRate := float64(report.PresentDays+report.HalfDays+report.WorkFromHomeDays) / float64(report.TotalDays) * 100
				message.WriteString(fmt.Sprintf(" | %.1f%%", attendanceRate))
			}
		}

		displayedCount++
	}

	return message.String()
}

// analyzeAttendanceReportQuery uses LLM to convert natural language to structured report request
func (m *AttendanceModule) analyzeAttendanceReportQuery(ctx *erp_modules.ModuleContext, userMessage string) (*AttendanceReportRequest, error) {
	// Create LLM context
	llmContext := &llm.Context{
		RequestingUser: ctx.User,
		Time:           time.Now().Format(time.RFC1123),
	}
	llmContext.Parameters = map[string]interface{}{
		"UserMessage": userMessage,
	}

	// Format the report analysis prompt
	systemPrompt, err := m.prompts.Format("attendance_report_analysis", llmContext)
	if err != nil {
		return nil, fmt.Errorf("failed to format report analysis prompt: %w", err)
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
		return nil, fmt.Errorf("failed to analyze report query with LLM: %w", err)
	}

	// Parse JSON response
	var reportRequest AttendanceReportRequest

	// Clean response to extract JSON
	response = strings.TrimSpace(response)
	start := strings.Index(response, "{")
	end := strings.LastIndex(response, "}") + 1

	if start == -1 || end <= start {
		return nil, fmt.Errorf("no valid JSON found in LLM response: %s", response)
	}

	jsonStr := response[start:end]
	if err := json.Unmarshal([]byte(jsonStr), &reportRequest); err != nil {
		return nil, fmt.Errorf("failed to parse LLM response as JSON: %w", err)
	}

	return &reportRequest, nil
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

// getEmployeeIDFromUser gets employee ID from user
func (m *AttendanceModule) getEmployeeIDFromUser(user *model.User) (string, error) {
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
