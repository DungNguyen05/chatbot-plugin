// Copyright (c) 2023-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package attendance

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/mattermost/mattermost-plugin-ai/server/erp_modules"
	"github.com/mattermost/mattermost-plugin-ai/server/llm"
	"github.com/mattermost/mattermost/server/public/model"
)

// AttendanceQueryHandler handles attendance queries for multiple users
type AttendanceQueryHandler struct {
	erpClient       *ERPClient
	mentionAnalyzer *UserMentionAnalyzer
	api             PluginAPI
	prompts         PromptsInterface
	getLLM          func() llm.LanguageModel
}

// UserAttendanceData represents attendance data for a single user
type UserAttendanceData struct {
	UserID       string                  `json:"user_id"`
	Username     string                  `json:"username"`
	DisplayName  string                  `json:"display_name"`
	EmployeeID   string                  `json:"employee_id,omitempty"`
	FoundInERP   bool                    `json:"found_in_erp"`
	QueryRequest *AttendanceQueryRequest `json:"query_request,omitempty"`
	Count        int                     `json:"count,omitempty"`
	Error        string                  `json:"error,omitempty"`
}

// MultiUserAttendanceResult represents the result of querying multiple users
type MultiUserAttendanceResult struct {
	RequestingUser  *model.User          `json:"requesting_user"`
	QueryType       string               `json:"query_type"` // "self", "others"
	UsersData       []UserAttendanceData `json:"users_data"`
	OriginalQuery   string               `json:"original_query"`
	QueryDetails    string               `json:"query_details"`
	TotalUsers      int                  `json:"total_users"`
	SuccessfulUsers int                  `json:"successful_users"`
	FailedUsers     int                  `json:"failed_users"`
}

// NewAttendanceQueryHandler creates a new attendance query handler
func NewAttendanceQueryHandler(
	erpClient *ERPClient,
	api PluginAPI,
	prompts PromptsInterface,
	getLLM func() llm.LanguageModel,
) *AttendanceQueryHandler {
	mentionAnalyzer := NewUserMentionAnalyzer(api, prompts, getLLM)

	return &AttendanceQueryHandler{
		erpClient:       erpClient,
		mentionAnalyzer: mentionAnalyzer,
		api:             api,
		prompts:         prompts,
		getLLM:          getLLM,
	}
}

// HandleMultiUserAttendanceQuery handles attendance queries that may involve multiple users
func (h *AttendanceQueryHandler) HandleMultiUserAttendanceQuery(ctx *erp_modules.ModuleContext, intent *erp_modules.Intent) (*erp_modules.ModuleResponse, error) {
	isVietnamese := detectUserLanguage(ctx.User)

	// Analyze user mentions in the message
	mentionAnalysis, err := h.mentionAnalyzer.AnalyzeUserMentions(intent.RawMessage, ctx.User)
	if err != nil {
		h.api.LogError("Failed to analyze user mentions", "error", err.Error())
		errorMsg := "⚠️ Không thể phân tích yêu cầu của bạn. Vui lòng thử lại."
		if !isVietnamese {
			errorMsg = "⚠️ Cannot analyze your request. Please try again."
		}
		return &erp_modules.ModuleResponse{
			Success: false,
			Message: errorMsg,
			Error:   err.Error(),
		}, nil
	}

	// Determine which users to query
	var userIDsToQuery []string
	if mentionAnalysis.QueryType == "self" || len(mentionAnalysis.MentionedUsers) == 0 {
		// Query about requesting user
		userIDsToQuery = []string{ctx.User.Id}
	} else {
		// Query about mentioned users
		userIDsToQuery = mentionAnalysis.MentionedUsers
	}

	// Parse the attendance query request using LLM
	queryRequest, err := h.analyzeAttendanceQuery(ctx, intent.RawMessage)
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

	// Collect attendance data for all users
	result, err := h.collectMultiUserAttendanceData(userIDsToQuery, queryRequest, ctx.User, intent.RawMessage)
	if err != nil {
		errorMsg := "⚠️ Có lỗi xảy ra khi thu thập dữ liệu chấm công. Vui lòng thử lại."
		if !isVietnamese {
			errorMsg = "⚠️ An error occurred while collecting attendance data. Please try again."
		}
		return &erp_modules.ModuleResponse{
			Success: false,
			Message: errorMsg,
			Error:   err.Error(),
		}, nil
	}

	// Generate comprehensive report using LLM
	reportMessage, err := h.generateMultiUserReport(ctx, result)
	if err != nil {
		// Fallback to simple report if LLM fails
		reportMessage = h.generateSimpleReport(result, isVietnamese)
	}

	return &erp_modules.ModuleResponse{
		Success:     true,
		Message:     reportMessage,
		ActionTaken: "multi_user_attendance_query",
		Data: map[string]interface{}{
			"query_type":       result.QueryType,
			"total_users":      result.TotalUsers,
			"successful_users": result.SuccessfulUsers,
			"failed_users":     result.FailedUsers,
			"users_data":       result.UsersData,
		},
	}, nil
}

// collectMultiUserAttendanceData collects attendance data for multiple users
func (h *AttendanceQueryHandler) collectMultiUserAttendanceData(
	userIDs []string,
	queryRequest *AttendanceQueryRequest,
	requestingUser *model.User,
	originalQuery string,
) (*MultiUserAttendanceResult, error) {
	result := &MultiUserAttendanceResult{
		RequestingUser: requestingUser,
		OriginalQuery:  originalQuery,
		QueryDetails:   fmt.Sprintf("Status: %s, Period: %s", queryRequest.Status, queryRequest.TimePeriod.Description),
		TotalUsers:     len(userIDs),
		UsersData:      make([]UserAttendanceData, 0, len(userIDs)),
	}

	// Determine query type
	if len(userIDs) == 1 && userIDs[0] == requestingUser.Id {
		result.QueryType = "self"
	} else {
		result.QueryType = "others"
	}

	// Process each user
	for _, userID := range userIDs {
		userData := h.processUserAttendanceData(userID, queryRequest)
		result.UsersData = append(result.UsersData, userData)

		if userData.Error == "" {
			result.SuccessfulUsers++
		} else {
			result.FailedUsers++
		}
	}

	return result, nil
}

// processUserAttendanceData processes attendance data for a single user
func (h *AttendanceQueryHandler) processUserAttendanceData(userID string, queryRequest *AttendanceQueryRequest) UserAttendanceData {
	userData := UserAttendanceData{
		UserID: userID,
	}

	// Get user information
	user, err := h.api.GetUser(userID)
	if err != nil {
		userData.Error = fmt.Sprintf("Failed to get user information: %v", err)
		return userData
	}

	userData.Username = user.Username
	userData.DisplayName = GetUserDisplayName(user)

	// Try to get employee ID from ERP
	employeeID, err := h.erpClient.GetEmployeeByChatID(userID)
	if err != nil {
		userData.Error = fmt.Sprintf("User not found in ERP system")
		userData.FoundInERP = false
		return userData
	}

	userData.EmployeeID = employeeID
	userData.FoundInERP = true
	userData.QueryRequest = queryRequest

	// Query attendance data
	count, err := h.erpClient.QueryAttendanceCount(
		employeeID,
		queryRequest.Status,
		queryRequest.TimePeriod.StartDate,
		queryRequest.TimePeriod.EndDate,
	)
	if err != nil {
		userData.Error = fmt.Sprintf("Failed to query attendance data: %v", err)
		return userData
	}

	userData.Count = count
	return userData
}

// analyzeAttendanceQuery analyzes the attendance query using LLM
func (h *AttendanceQueryHandler) analyzeAttendanceQuery(ctx *erp_modules.ModuleContext, userMessage string) (*AttendanceQueryRequest, error) {
	// Create LLM context
	llmContext := &llm.Context{
		RequestingUser: ctx.User,
		Time:           time.Now().Format(time.RFC1123),
	}
	llmContext.Parameters = map[string]interface{}{
		"UserMessage": userMessage,
	}

	// Format the query analysis prompt
	systemPrompt, err := h.prompts.Format("attendance_query_analysis", llmContext)
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
	response, err := h.getLLM().ChatCompletionNoStream(completionRequest, llm.WithMaxGeneratedTokens(300))
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

// generateMultiUserReport generates a comprehensive report for multiple users using LLM
func (h *AttendanceQueryHandler) generateMultiUserReport(ctx *erp_modules.ModuleContext, result *MultiUserAttendanceResult) (string, error) {
	// Create LLM context
	llmContext := &llm.Context{
		RequestingUser: ctx.User,
	}

	isVietnamese := detectUserLanguage(ctx.User)

	llmContext.Parameters = map[string]interface{}{
		"OriginalQuery":   result.OriginalQuery,
		"QueryType":       result.QueryType,
		"TotalUsers":      result.TotalUsers,
		"SuccessfulUsers": result.SuccessfulUsers,
		"FailedUsers":     result.FailedUsers,
		"UsersData":       result.UsersData,
		"QueryDetails":    result.QueryDetails,
		"IsVietnamese":    isVietnamese,
		"RequestingUser":  result.RequestingUser.Username,
	}

	// Format the multi-user report prompt
	systemPrompt, err := h.prompts.Format("attendance_multi_user_report", llmContext)
	if err != nil {
		return "", fmt.Errorf("failed to format multi-user report prompt: %w", err)
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
				Message: fmt.Sprintf("Generate attendance report: %s", result.OriginalQuery),
			},
		},
		Context: llmContext,
	}

	// Get LLM response
	response, err := h.getLLM().ChatCompletionNoStream(completionRequest, llm.WithMaxGeneratedTokens(400))
	if err != nil {
		return "", fmt.Errorf("failed to generate report with LLM: %w", err)
	}

	return strings.TrimSpace(response), nil
}

// generateSimpleReport generates a simple fallback report
func (h *AttendanceQueryHandler) generateSimpleReport(result *MultiUserAttendanceResult, isVietnamese bool) string {
	var report strings.Builder

	if isVietnamese {
		report.WriteString("📊 **Báo cáo chấm công**\n\n")
		for _, userData := range result.UsersData {
			if userData.Error != "" {
				report.WriteString(fmt.Sprintf("❌ **%s (@%s)**: %s\n", userData.DisplayName, userData.Username, userData.Error))
			} else {
				report.WriteString(fmt.Sprintf("✅ **%s (@%s)**: %d ngày %s\n",
					userData.DisplayName, userData.Username, userData.Count, userData.QueryRequest.Status))
			}
		}
	} else {
		report.WriteString("📊 **Attendance Report**\n\n")
		for _, userData := range result.UsersData {
			if userData.Error != "" {
				report.WriteString(fmt.Sprintf("❌ **%s (@%s)**: %s\n", userData.DisplayName, userData.Username, userData.Error))
			} else {
				report.WriteString(fmt.Sprintf("✅ **%s (@%s)**: %d %s days\n",
					userData.DisplayName, userData.Username, userData.Count, userData.QueryRequest.Status))
			}
		}
	}

	return report.String()
}
