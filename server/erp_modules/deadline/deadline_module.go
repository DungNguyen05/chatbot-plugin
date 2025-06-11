// Copyright (c) 2023-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package deadline

import (
	"fmt"
	"strings"
	"time"

	"github.com/mattermost/mattermost-plugin-ai/server/erp_modules"
)

// DeadlineModule handles deadline-related queries
type DeadlineModule struct {
	erpIntegration DeadlineERPIntegration
}

// DeadlineERPIntegration interface for deadline-related ERP operations
type DeadlineERPIntegration interface {
	GetUserDeadlines(employeeID string, daysAhead int) ([]Deadline, error)
	GetProjectDeadlines(projectID string) ([]Deadline, error)
	GetOverdueItems(employeeID string) ([]Deadline, error)
}

type Deadline struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	DueDate     time.Time `json:"due_date"`
	Type        string    `json:"type"` // "task", "project", "milestone"
	Status      string    `json:"status"`
	Priority    string    `json:"priority"`
}

// NewDeadlineModule creates a new deadline module
func NewDeadlineModule(erpIntegration DeadlineERPIntegration) *DeadlineModule {
	return &DeadlineModule{
		erpIntegration: erpIntegration,
	}
}

// GetCategory returns the category this module handles
func (m *DeadlineModule) GetCategory() string {
	return "deadline"
}

// GetSupportedActions returns list of actions this module supports
func (m *DeadlineModule) GetSupportedActions() []string {
	return []string{"check_upcoming", "check_overdue", "check_project"}
}

// CanHandle determines if this module can handle the given intent
func (m *DeadlineModule) CanHandle(intent *erp_modules.Intent) bool {
	if intent.Category != "deadline" {
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

// Execute processes the deadline intent
func (m *DeadlineModule) Execute(ctx *erp_modules.ModuleContext, intent *erp_modules.Intent) (*erp_modules.ModuleResponse, error) {
	switch intent.Action {
	case "check_upcoming":
		return m.handleCheckUpcoming(ctx, intent)
	case "check_overdue":
		return m.handleCheckOverdue(ctx, intent)
	case "check_project":
		return m.handleCheckProject(ctx, intent)
	default:
		return &erp_modules.ModuleResponse{
			Success: false,
			Message: "Hành động không được hỗ trợ",
			Error:   fmt.Sprintf("unsupported action: %s", intent.Action),
		}, nil
	}
}

// GetDescription returns a description of what this module does
func (m *DeadlineModule) GetDescription() string {
	return "Kiểm tra deadline: xem deadline sắp tới, deadline quá hạn, deadline của dự án"
}

// GetActionExamples returns examples of user messages for each action
func (m *DeadlineModule) GetActionExamples() map[string][]string {
	return map[string][]string{
		"check_upcoming": {
			"deadline sắp tới",
			"việc gì cần làm tuần này",
			"deadline nào gần nhất",
			"công việc gấp",
			"hạn nộp sắp tới",
		},
		"check_overdue": {
			"deadline quá hạn",
			"việc gì trễ deadline",
			"task nào quá hạn",
			"công việc bị trễ",
		},
		"check_project": {
			"deadline dự án A",
			"hạn của project X",
			"khi nào dự án này xong",
		},
	}
}

// handleCheckUpcoming processes upcoming deadline checks
func (m *DeadlineModule) handleCheckUpcoming(ctx *erp_modules.ModuleContext, intent *erp_modules.Intent) (*erp_modules.ModuleResponse, error) {
	// Get number of days to look ahead (default 7 days)
	daysAhead := 7
	if daysStr, exists := intent.Parameters["days"]; exists {
		// Parse days from parameters if provided
		// Implementation would parse string to int
		_ = daysStr
	}

	// Get employee ID (this would need to be implemented)
	employeeID := "current_user_employee_id" // placeholder

	deadlines, err := m.erpIntegration.GetUserDeadlines(employeeID, daysAhead)
	if err != nil {
		return &erp_modules.ModuleResponse{
			Success: false,
			Message: "⚠️ Có lỗi xảy ra khi lấy thông tin deadline. Vui lòng thử lại.",
			Error:   err.Error(),
		}, nil
	}

	if len(deadlines) == 0 {
		return &erp_modules.ModuleResponse{
			Success: true,
			Message: "🎉 Bạn không có deadline nào trong 7 ngày tới!",
			Data: map[string]interface{}{
				"deadlines": deadlines,
			},
		}, nil
	}

	// Format deadline list
	var message strings.Builder
	message.WriteString(fmt.Sprintf("📅 Bạn có **%d** deadline sắp tới:\n\n", len(deadlines)))

	for i, deadline := range deadlines {
		urgency := m.getUrgencyIcon(deadline.DueDate)
		daysUntil := int(time.Until(deadline.DueDate).Hours() / 24)

		message.WriteString(fmt.Sprintf("%d. %s **%s**\n", i+1, urgency, deadline.Title))

		if daysUntil == 0 {
			message.WriteString("   ⏰ **HÔM NAY**\n")
		} else if daysUntil == 1 {
			message.WriteString("   ⏰ **NGÀY MAI**\n")
		} else {
			message.WriteString(fmt.Sprintf("   📅 Còn %d ngày\n", daysUntil))
		}

		if deadline.Priority != "" {
			message.WriteString(fmt.Sprintf("   🏷️ Mức độ: %s\n", deadline.Priority))
		}
		message.WriteString("\n")
	}

	return &erp_modules.ModuleResponse{
		Success:     true,
		Message:     message.String(),
		ActionTaken: "check_upcoming_deadlines",
		Data: map[string]interface{}{
			"deadlines": deadlines,
		},
	}, nil
}

// handleCheckOverdue processes overdue deadline checks
func (m *DeadlineModule) handleCheckOverdue(ctx *erp_modules.ModuleContext, intent *erp_modules.Intent) (*erp_modules.ModuleResponse, error) {
	// Get employee ID (this would need to be implemented)
	employeeID := "current_user_employee_id" // placeholder

	overdueItems, err := m.erpIntegration.GetOverdueItems(employeeID)
	if err != nil {
		return &erp_modules.ModuleResponse{
			Success: false,
			Message: "⚠️ Có lỗi xảy ra khi kiểm tra deadline quá hạn. Vui lòng thử lại.",
			Error:   err.Error(),
		}, nil
	}

	if len(overdueItems) == 0 {
		return &erp_modules.ModuleResponse{
			Success: true,
			Message: "✅ Tuyệt vời! Bạn không có deadline nào quá hạn.",
			Data: map[string]interface{}{
				"overdue_items": overdueItems,
			},
		}, nil
	}

	// Format overdue list
	var message strings.Builder
	message.WriteString(fmt.Sprintf("⚠️ Bạn có **%d** deadline đã quá hạn:\n\n", len(overdueItems)))

	for i, item := range overdueItems {
		daysOverdue := int(time.Since(item.DueDate).Hours() / 24)

		message.WriteString(fmt.Sprintf("%d. 🔴 **%s**\n", i+1, item.Title))
		message.WriteString(fmt.Sprintf("   ⏰ Quá hạn %d ngày\n", daysOverdue))

		if item.Priority != "" {
			message.WriteString(fmt.Sprintf("   🏷️ Mức độ: %s\n", item.Priority))
		}
		message.WriteString("\n")
	}

	message.WriteString("\n💡 **Gợi ý**: Hãy ưu tiên hoàn thành những công việc này sớm nhất có thể!")

	return &erp_modules.ModuleResponse{
		Success:     true,
		Message:     message.String(),
		ActionTaken: "check_overdue_deadlines",
		Data: map[string]interface{}{
			"overdue_items": overdueItems,
		},
	}, nil
}

// handleCheckProject processes project deadline checks
func (m *DeadlineModule) handleCheckProject(ctx *erp_modules.ModuleContext, intent *erp_modules.Intent) (*erp_modules.ModuleResponse, error) {
	projectID := intent.Parameters["project_id"]
	if projectID == "" {
		return &erp_modules.ModuleResponse{
			Success: false,
			Message: "Vui lòng cung cấp tên hoặc ID của dự án",
		}, nil
	}

	deadlines, err := m.erpIntegration.GetProjectDeadlines(projectID)
	if err != nil {
		return &erp_modules.ModuleResponse{
			Success: false,
			Message: "⚠️ Có lỗi xảy ra khi lấy thông tin deadline dự án. Vui lòng thử lại.",
			Error:   err.Error(),
		}, nil
	}

	if len(deadlines) == 0 {
		return &erp_modules.ModuleResponse{
			Success: true,
			Message: fmt.Sprintf("📋 Dự án **%s** hiện không có deadline nào.", projectID),
			Data: map[string]interface{}{
				"project_deadlines": deadlines,
			},
		}, nil
	}

	// Format project deadline list
	var message strings.Builder
	message.WriteString(fmt.Sprintf("📋 Dự án **%s** có **%d** deadline:\n\n", projectID, len(deadlines)))

	for i, deadline := range deadlines {
		urgency := m.getUrgencyIcon(deadline.DueDate)
		daysUntil := int(time.Until(deadline.DueDate).Hours() / 24)

		message.WriteString(fmt.Sprintf("%d. %s **%s**\n", i+1, urgency, deadline.Title))

		if daysUntil < 0 {
			message.WriteString(fmt.Sprintf("   🔴 Quá hạn %d ngày\n", -daysUntil))
		} else if daysUntil == 0 {
			message.WriteString("   ⏰ **HÔM NAY**\n")
		} else if daysUntil == 1 {
			message.WriteString("   ⏰ **NGÀY MAI**\n")
		} else {
			message.WriteString(fmt.Sprintf("   📅 Còn %d ngày\n", daysUntil))
		}

		message.WriteString(fmt.Sprintf("   📝 %s\n", deadline.Type))
		message.WriteString("\n")
	}

	return &erp_modules.ModuleResponse{
		Success:     true,
		Message:     message.String(),
		ActionTaken: "check_project_deadlines",
		Data: map[string]interface{}{
			"project_id":        projectID,
			"project_deadlines": deadlines,
		},
	}, nil
}

// getUrgencyIcon returns an appropriate icon based on how urgent the deadline is
func (m *DeadlineModule) getUrgencyIcon(dueDate time.Time) string {
	daysUntil := int(time.Until(dueDate).Hours() / 24)

	if daysUntil < 0 {
		return "🔴" // Overdue
	} else if daysUntil == 0 {
		return "🟠" // Due today
	} else if daysUntil <= 2 {
		return "🟡" // Due soon
	} else {
		return "🟢" // Not urgent
	}
}
