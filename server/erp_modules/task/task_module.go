// Copyright (c) 2023-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package task

import (
	"fmt"
	"strings"

	"github.com/mattermost/mattermost-plugin-ai/server/erp_modules"
)

// TaskModule handles task management operations
type TaskModule struct {
	erpIntegration TaskERPIntegration
}

// TaskERPIntegration interface for task-related ERP operations
type TaskERPIntegration interface {
	CreateTask(employeeID, title, description, dueDate string) (string, error)
	GetUserTasks(employeeID string) ([]Task, error)
	UpdateTaskStatus(taskID, status string) error
	AssignTask(taskID, assigneeID string) error
}

type Task struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Status      string `json:"status"`
	DueDate     string `json:"due_date"`
	Assignee    string `json:"assignee"`
}

// NewTaskModule creates a new task module
func NewTaskModule(erpIntegration TaskERPIntegration) *TaskModule {
	return &TaskModule{
		erpIntegration: erpIntegration,
	}
}

// GetCategory returns the category this module handles
func (m *TaskModule) GetCategory() string {
	return "task"
}

// GetSupportedActions returns list of actions this module supports
func (m *TaskModule) GetSupportedActions() []string {
	return []string{"create", "list", "update_status", "assign"}
}

// CanHandle determines if this module can handle the given intent
func (m *TaskModule) CanHandle(intent *erp_modules.Intent) bool {
	if intent.Category != "task" {
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

// Execute processes the task intent
func (m *TaskModule) Execute(ctx *erp_modules.ModuleContext, intent *erp_modules.Intent) (*erp_modules.ModuleResponse, error) {
	switch intent.Action {
	case "create":
		return m.handleCreateTask(ctx, intent)
	case "list":
		return m.handleListTasks(ctx, intent)
	case "update_status":
		return m.handleUpdateTaskStatus(ctx, intent)
	case "assign":
		return m.handleAssignTask(ctx, intent)
	default:
		return &erp_modules.ModuleResponse{
			Success: false,
			Message: "Hành động không được hỗ trợ",
			Error:   fmt.Sprintf("unsupported action: %s", intent.Action),
		}, nil
	}
}

// GetDescription returns a description of what this module does
func (m *TaskModule) GetDescription() string {
	return "Quản lý công việc: tạo task mới, xem danh sách task, cập nhật trạng thái, phân công"
}

// GetActionExamples returns examples of user messages for each action
func (m *TaskModule) GetActionExamples() map[string][]string {
	return map[string][]string{
		"create": {
			"tạo task mới",
			"tạo công việc",
			"giao việc",
			"thêm task",
			"tạo nhiệm vụ mới",
		},
		"list": {
			"xem task của tôi",
			"danh sách công việc",
			"task nào cần làm",
			"việc gì hôm nay",
			"công việc còn lại",
		},
		"update_status": {
			"hoàn thành task",
			"đánh dấu xong",
			"cập nhật trạng thái",
			"task này xong rồi",
		},
		"assign": {
			"giao việc cho",
			"phân công task",
			"assign task",
			"giao cho ai đó",
		},
	}
}

// handleCreateTask processes task creation
func (m *TaskModule) handleCreateTask(ctx *erp_modules.ModuleContext, intent *erp_modules.Intent) (*erp_modules.ModuleResponse, error) {
	title := intent.Parameters["title"]
	description := intent.Parameters["description"]
	dueDate := intent.Parameters["due_date"]

	if title == "" {
		return &erp_modules.ModuleResponse{
			Success: false,
			Message: "Vui lòng cung cấp tiêu đề cho task",
		}, nil
	}

	// Get employee ID (this would need to be implemented)
	employeeID := "current_user_employee_id" // placeholder

	taskID, err := m.erpIntegration.CreateTask(employeeID, title, description, dueDate)
	if err != nil {
		return &erp_modules.ModuleResponse{
			Success: false,
			Message: "⚠️ Có lỗi xảy ra khi tạo task. Vui lòng thử lại.",
			Error:   err.Error(),
		}, nil
	}

	return &erp_modules.ModuleResponse{
		Success:     true,
		Message:     fmt.Sprintf("✅ Đã tạo task mới: **%s** (ID: %s)", title, taskID),
		ActionTaken: "create_task",
		Data: map[string]interface{}{
			"task_id":     taskID,
			"title":       title,
			"description": description,
			"due_date":    dueDate,
		},
	}, nil
}

// handleListTasks processes task listing
func (m *TaskModule) handleListTasks(ctx *erp_modules.ModuleContext, intent *erp_modules.Intent) (*erp_modules.ModuleResponse, error) {
	// Get employee ID (this would need to be implemented)
	employeeID := "current_user_employee_id" // placeholder

	tasks, err := m.erpIntegration.GetUserTasks(employeeID)
	if err != nil {
		return &erp_modules.ModuleResponse{
			Success: false,
			Message: "⚠️ Có lỗi xảy ra khi lấy danh sách task. Vui lòng thử lại.",
			Error:   err.Error(),
		}, nil
	}

	if len(tasks) == 0 {
		return &erp_modules.ModuleResponse{
			Success: true,
			Message: "📝 Bạn hiện không có task nào cần làm.",
			Data: map[string]interface{}{
				"tasks": tasks,
			},
		}, nil
	}

	// Format task list
	var message strings.Builder
	message.WriteString(fmt.Sprintf("📋 Bạn có **%d** task:\n\n", len(tasks)))

	for i, task := range tasks {
		status := "🔄"
		if task.Status == "completed" {
			status = "✅"
		} else if task.Status == "in_progress" {
			status = "🔄"
		} else {
			status = "📝"
		}

		message.WriteString(fmt.Sprintf("%d. %s **%s**\n", i+1, status, task.Title))
		if task.DueDate != "" {
			message.WriteString(fmt.Sprintf("   📅 Deadline: %s\n", task.DueDate))
		}
		message.WriteString("\n")
	}

	return &erp_modules.ModuleResponse{
		Success:     true,
		Message:     message.String(),
		ActionTaken: "list_tasks",
		Data: map[string]interface{}{
			"tasks": tasks,
		},
	}, nil
}

// handleUpdateTaskStatus processes task status updates
func (m *TaskModule) handleUpdateTaskStatus(ctx *erp_modules.ModuleContext, intent *erp_modules.Intent) (*erp_modules.ModuleResponse, error) {
	taskID := intent.Parameters["task_id"]
	status := intent.Parameters["status"]

	if taskID == "" {
		return &erp_modules.ModuleResponse{
			Success: false,
			Message: "Vui lòng cung cấp ID của task cần cập nhật",
		}, nil
	}

	if status == "" {
		status = "completed" // default to completed
	}

	err := m.erpIntegration.UpdateTaskStatus(taskID, status)
	if err != nil {
		return &erp_modules.ModuleResponse{
			Success: false,
			Message: "⚠️ Có lỗi xảy ra khi cập nhật task. Vui lòng thử lại.",
			Error:   err.Error(),
		}, nil
	}

	statusText := "hoàn thành"
	if status == "in_progress" {
		statusText = "đang thực hiện"
	} else if status == "pending" {
		statusText = "chờ xử lý"
	}

	return &erp_modules.ModuleResponse{
		Success:     true,
		Message:     fmt.Sprintf("✅ Đã cập nhật trạng thái task thành: **%s**", statusText),
		ActionTaken: "update_task_status",
		Data: map[string]interface{}{
			"task_id": taskID,
			"status":  status,
		},
	}, nil
}

// handleAssignTask processes task assignment
func (m *TaskModule) handleAssignTask(ctx *erp_modules.ModuleContext, intent *erp_modules.Intent) (*erp_modules.ModuleResponse, error) {
	taskID := intent.Parameters["task_id"]
	assigneeID := intent.Parameters["assignee_id"]

	if taskID == "" || assigneeID == "" {
		return &erp_modules.ModuleResponse{
			Success: false,
			Message: "Vui lòng cung cấp ID task và người được giao việc",
		}, nil
	}

	err := m.erpIntegration.AssignTask(taskID, assigneeID)
	if err != nil {
		return &erp_modules.ModuleResponse{
			Success: false,
			Message: "⚠️ Có lỗi xảy ra khi phân công task. Vui lòng thử lại.",
			Error:   err.Error(),
		}, nil
	}

	return &erp_modules.ModuleResponse{
		Success:     true,
		Message:     fmt.Sprintf("✅ Đã giao task cho nhân viên (ID: %s)", assigneeID),
		ActionTaken: "assign_task",
		Data: map[string]interface{}{
			"task_id":     taskID,
			"assignee_id": assigneeID,
		},
	}, nil
}
