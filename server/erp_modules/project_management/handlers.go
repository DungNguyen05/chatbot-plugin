// Copyright (c) 2023-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package project_management

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/mattermost/mattermost-plugin-ai/server/erp_modules"
	"github.com/mattermost/mattermost/server/public/model"
)

// ConfirmationManager handles confirmation state with multi-employee disambiguation
type ConfirmationManager struct {
	confirmationState                map[string]*ProjectManagementConfirmation
	multiEmployeeDisambiguationState map[string]*MultiEmployeeDisambiguationConfirmation
	confirmationMutex                sync.RWMutex
}

// NewConfirmationManager creates a new confirmation manager
func NewConfirmationManager() *ConfirmationManager {
	return &ConfirmationManager{
		confirmationState:                make(map[string]*ProjectManagementConfirmation),
		multiEmployeeDisambiguationState: make(map[string]*MultiEmployeeDisambiguationConfirmation),
	}
}

// StorePendingConfirmation stores a pending confirmation
func (cm *ConfirmationManager) StorePendingConfirmation(userID string, confirmation *ProjectManagementConfirmation) {
	cm.confirmationMutex.Lock()
	defer cm.confirmationMutex.Unlock()
	cm.confirmationState[userID] = confirmation
	delete(cm.multiEmployeeDisambiguationState, userID)
}

// GetPendingConfirmation retrieves a pending confirmation
func (cm *ConfirmationManager) GetPendingConfirmation(userID string) (*ProjectManagementConfirmation, bool) {
	cm.confirmationMutex.RLock()
	defer cm.confirmationMutex.RUnlock()
	confirmation, exists := cm.confirmationState[userID]
	return confirmation, exists
}

// ClearPendingConfirmation clears pending confirmation for a user
func (cm *ConfirmationManager) ClearPendingConfirmation(userID string) {
	cm.confirmationMutex.Lock()
	defer cm.confirmationMutex.Unlock()
	delete(cm.confirmationState, userID)
	delete(cm.multiEmployeeDisambiguationState, userID)
}

// StorePendingMultiEmployeeDisambiguation stores a pending multi-employee disambiguation
func (cm *ConfirmationManager) StorePendingMultiEmployeeDisambiguation(userID string, disambiguation *MultiEmployeeDisambiguationConfirmation) {
	cm.confirmationMutex.Lock()
	defer cm.confirmationMutex.Unlock()
	cm.multiEmployeeDisambiguationState[userID] = disambiguation
	delete(cm.confirmationState, userID)
}

// GetPendingMultiEmployeeDisambiguation retrieves a pending multi-employee disambiguation
func (cm *ConfirmationManager) GetPendingMultiEmployeeDisambiguation(userID string) (*MultiEmployeeDisambiguationConfirmation, bool) {
	cm.confirmationMutex.RLock()
	defer cm.confirmationMutex.RUnlock()
	disambiguation, exists := cm.multiEmployeeDisambiguationState[userID]
	return disambiguation, exists
}

// handleCreateProjectRequest processes project creation request with multi-employee disambiguation support
func (m *ProjectManagementModule) handleCreateProjectRequest(employeeID string, ctx *erp_modules.ModuleContext, intent *erp_modules.Intent) (*erp_modules.ModuleResponse, error) {
	isVietnamese := detectUserLanguage(ctx.User)

	// Extract project details using LLM
	projectRequest, err := m.analyzeProjectCreation(ctx, intent.RawMessage)
	if err != nil {
		errorMsg := "⚠️ Không thể hiểu được yêu cầu tạo dự án. Vui lòng thử lại với thông tin rõ ràng hơn."
		if !isVietnamese {
			errorMsg = "⚠️ Cannot understand the project creation request. Please try again with clearer information."
		}
		return &erp_modules.ModuleResponse{
			Success: false,
			Message: errorMsg,
			Error:   err.Error(),
		}, nil
	}

	// Validate required fields
	if projectRequest.ProjectName == "" {
		errorMsg := "📝 Vui lòng cung cấp tên dự án. Ví dụ: 'Tạo dự án phát triển website bán hàng'"
		if !isVietnamese {
			errorMsg = "📝 Please provide project name. Example: 'Create e-commerce website development project'"
		}
		return &erp_modules.ModuleResponse{
			Success: false,
			Message: errorMsg,
		}, nil
	}

	// Process assignees if provided
	if len(projectRequest.AssignedToNames) > 0 {
		assigneeResult, err := m.resolveMultipleAssignees(projectRequest.AssignedToNames)
		if err != nil {
			m.api.LogError("Failed to resolve project assignees", "error", err.Error())
		} else if assigneeResult.RequiresDisambiguation {
			// Return disambiguation request
			return m.requestMultiEmployeeDisambiguation(ctx, "create_project", "project", projectRequest, employeeID, assigneeResult)
		} else {
			projectRequest.AssignedToEmployees = assigneeResult.ResolvedEmployees
		}
	}

	// Get creator's email for assignment
	creatorEmail, err := m.getEmployeeEmailFromUser(ctx.User)
	if err != nil {
		m.api.LogWarn("Failed to get creator email", "error", err.Error())
		creatorEmail = "demo@example.com"
	}

	// Request confirmation
	return m.requestConfirmation(ctx, "create_project", "project", projectRequest, employeeID, creatorEmail)
}

// handleCreateTaskRequest processes task creation request with multi-employee disambiguation support
func (m *ProjectManagementModule) handleCreateTaskRequest(employeeID string, ctx *erp_modules.ModuleContext, intent *erp_modules.Intent) (*erp_modules.ModuleResponse, error) {
	isVietnamese := detectUserLanguage(ctx.User)

	// Extract task details using LLM
	taskRequest, err := m.analyzeTaskCreation(ctx, intent.RawMessage)
	if err != nil {
		errorMsg := "⚠️ Không thể hiểu được yêu cầu tạo task. Vui lòng thử lại với thông tin rõ ràng hơn."
		if !isVietnamese {
			errorMsg = "⚠️ Cannot understand the task creation request. Please try again with clearer information."
		}
		return &erp_modules.ModuleResponse{
			Success: false,
			Message: errorMsg,
			Error:   err.Error(),
		}, nil
	}

	// Validate required fields
	if taskRequest.Subject == "" {
		errorMsg := "📝 Vui lòng cung cấp tên công việc. Ví dụ: 'Tạo task thiết kế giao diện trang chủ'"
		if !isVietnamese {
			errorMsg = "📝 Please provide task name. Example: 'Create homepage design task'"
		}
		return &erp_modules.ModuleResponse{
			Success: false,
			Message: errorMsg,
		}, nil
	}

	// Process assignees if provided
	if len(taskRequest.AssignedToNames) > 0 {
		assigneeResult, err := m.resolveMultipleAssignees(taskRequest.AssignedToNames)
		if err != nil {
			m.api.LogError("Failed to resolve task assignees", "error", err.Error())
		} else if assigneeResult.RequiresDisambiguation {
			// Return disambiguation request
			return m.requestMultiEmployeeDisambiguation(ctx, "create_task", "task", taskRequest, employeeID, assigneeResult)
		} else {
			taskRequest.AssignedToEmployees = assigneeResult.ResolvedEmployees
		}
	}

	// Get creator's email for assignment
	creatorEmail, err := m.getEmployeeEmailFromUser(ctx.User)
	if err != nil {
		m.api.LogWarn("Failed to get creator email", "error", err.Error())
		creatorEmail = "demo@example.com"
	}

	// Request confirmation
	return m.requestConfirmation(ctx, "create_task", "task", taskRequest, employeeID, creatorEmail)
}

// requestMultiEmployeeDisambiguation requests user to choose from multiple employees for multiple assignees
func (m *ProjectManagementModule) requestMultiEmployeeDisambiguation(
	ctx *erp_modules.ModuleContext,
	action, confirmationType string,
	data interface{},
	employeeID string,
	assigneeResult *AssigneeResolutionResult,
) (*erp_modules.ModuleResponse, error) {

	// Convert data to map for consistent storage
	dataMap := make(map[string]interface{})
	dataBytes, _ := json.Marshal(data)
	json.Unmarshal(dataBytes, &dataMap)

	// Get creator's email
	creatorEmail, err := m.getEmployeeEmailFromUser(ctx.User)
	if err != nil {
		m.api.LogWarn("Failed to get creator email for disambiguation", "error", err.Error())
		creatorEmail = "demo@example.com"
	}

	// Store pending disambiguation
	disambiguation := &MultiEmployeeDisambiguationConfirmation{
		UserID:                    ctx.User.Id,
		Type:                      confirmationType,
		Action:                    action,
		Data:                      dataMap,
		CreatedAt:                 time.Now().UnixMilli(),
		EmployeeID:                employeeID,
		CreatorEmail:              creatorEmail,
		UnresolvedEmployeeMatches: assigneeResult.UnresolvedEmployeeMatches,
		ResolvedEmployees:         assigneeResult.ResolvedEmployees,
		IsModification:            false,
	}

	m.confirmationManager.StorePendingMultiEmployeeDisambiguation(ctx.User.Id, disambiguation)

	// Generate disambiguation message
	disambiguationMsg, err := m.generateMultiEmployeeDisambiguationMessage(ctx, assigneeResult)
	if err != nil {
		isVietnamese := detectUserLanguage(ctx.User)
		errorMsg := "⚠️ Có lỗi xảy ra khi tạo tin nhắn lựa chọn nhân viên."
		if !isVietnamese {
			errorMsg = "⚠️ An error occurred while creating employee selection message."
		}
		return &erp_modules.ModuleResponse{
			Success: false,
			Message: errorMsg,
			Error:   err.Error(),
		}, nil
	}

	return &erp_modules.ModuleResponse{
		Success:     true,
		Message:     disambiguationMsg,
		ActionTaken: "request_multi_employee_disambiguation",
		Data: map[string]interface{}{
			"awaiting_disambiguation": true,
			"type":                    confirmationType,
			"unresolved_count":        len(assigneeResult.UnresolvedEmployeeMatches),
			"resolved_count":          len(assigneeResult.ResolvedEmployees),
		},
	}, nil
}

// generateMultiEmployeeDisambiguationMessage generates message asking user to choose employees
func (m *ProjectManagementModule) generateMultiEmployeeDisambiguationMessage(ctx *erp_modules.ModuleContext, assigneeResult *AssigneeResolutionResult) (string, error) {
	isVietnamese := detectUserLanguage(ctx.User)

	var message strings.Builder

	// Show successfully resolved employees if any
	if len(assigneeResult.ResolvedEmployees) > 0 {
		if isVietnamese {
			message.WriteString("✅ **Đã xác định thành công:**\n")
		} else {
			message.WriteString("✅ **Successfully identified:**\n")
		}
		for _, emp := range assigneeResult.ResolvedEmployees {
			message.WriteString(fmt.Sprintf("- **%s** (%s)\n", emp.EmployeeName, emp.Email))
		}
		message.WriteString("\n")
	}

	// Show employees that need disambiguation
	if isVietnamese {
		message.WriteString("❓ **Cần làm rõ cho các nhân viên sau:**\n\n")
	} else {
		message.WriteString("❓ **Need clarification for the following employees:**\n\n")
	}

	globalIndex := 1
	for _, unresolvedMatch := range assigneeResult.UnresolvedEmployeeMatches {
		if isVietnamese {
			message.WriteString(fmt.Sprintf("**Tên '%s'** có thể là:\n", unresolvedMatch.OriginalName))
		} else {
			message.WriteString(fmt.Sprintf("**Name '%s'** could be:\n", unresolvedMatch.OriginalName))
		}

		for _, emp := range unresolvedMatch.MatchingEmployees {
			message.WriteString(fmt.Sprintf("%d. **%s** (%s, %s)\n",
				globalIndex,
				emp.EmployeeName,
				emp.CompanyEmail,
				emp.Name))
			globalIndex++
		}
		message.WriteString("\n")
	}

	if isVietnamese {
		message.WriteString("Vui lòng chọn nhân viên bằng cách trả lời các số thứ tự tương ứng.\n")
		message.WriteString("**Ví dụ:** `1, 3, 5` để chọn nhân viên thứ 1, 3 và 5.")
	} else {
		message.WriteString("Please select employees by replying with the corresponding numbers.\n")
		message.WriteString("**Example:** `1, 3, 5` to select employees 1, 3, and 5.")
	}

	return message.String(), nil
}

// requestConfirmation requests confirmation from user
func (m *ProjectManagementModule) requestConfirmation(ctx *erp_modules.ModuleContext, action, confirmationType string, data interface{}, employeeID, creatorEmail string) (*erp_modules.ModuleResponse, error) {
	// Convert data to map for consistent storage
	dataMap := make(map[string]interface{})
	dataBytes, _ := json.Marshal(data)
	json.Unmarshal(dataBytes, &dataMap)

	// Store pending confirmation
	m.confirmationManager.StorePendingConfirmation(ctx.User.Id, &ProjectManagementConfirmation{
		UserID:       ctx.User.Id,
		Type:         confirmationType,
		Action:       action,
		Data:         dataMap,
		CreatedAt:    time.Now().UnixMilli(),
		EmployeeID:   employeeID,
		CreatorEmail: creatorEmail,
	})

	// Generate confirmation message
	confirmationMsg, err := m.generateConfirmationMessage(ctx, confirmationType, data)
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
			"type":                  confirmationType,
		},
	}, nil
}

// executeConfirmedAction executes the confirmed action
func (m *ProjectManagementModule) executeConfirmedAction(ctx *erp_modules.ModuleContext, pending *ProjectManagementConfirmation) (*erp_modules.ModuleResponse, error) {
	switch pending.Action {
	case "create_project":
		return m.handleCreateProject(pending.EmployeeID, ctx.User.Id, pending.Data, pending.CreatorEmail)
	case "create_task":
		return m.handleCreateTask(pending.EmployeeID, ctx.User.Id, pending.Data, pending.CreatorEmail)
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

// handleCreateProject processes project creation with multi-employee assignment
func (m *ProjectManagementModule) handleCreateProject(employeeID, userID string, data map[string]interface{}, creatorEmail string) (*erp_modules.ModuleResponse, error) {
	var projectRequest ProjectCreationRequest
	dataBytes, _ := json.Marshal(data)
	json.Unmarshal(dataBytes, &projectRequest)

	// Create project first
	projectID, err := m.erpClient.CreateProject(projectRequest, employeeID)
	if err != nil {
		user, _ := m.api.GetUser(userID)
		isVietnamese := detectUserLanguage(user)
		errorMsg := "⚠️ Có lỗi xảy ra khi tạo dự án trong hệ thống. Vui lòng thử lại."
		if !isVietnamese {
			errorMsg = "⚠️ An error occurred while creating project in the system. Please try again."
		}
		return &erp_modules.ModuleResponse{
			Success: false,
			Message: errorMsg,
			Error:   err.Error(),
		}, nil
	}

	// Assign to multiple employees if any
	var assignmentErrors []string
	for _, assignedEmployee := range projectRequest.AssignedToEmployees {
		err := m.erpClient.AssignProjectToEmployee(projectID, creatorEmail, assignedEmployee.Email, projectRequest.Priority)
		if err != nil {
			m.api.LogWarn("Project created but assignment failed",
				"project_id", projectID,
				"assignee", assignedEmployee.EmployeeName,
				"error", err.Error())
			assignmentErrors = append(assignmentErrors, assignedEmployee.EmployeeName)
		} else {
			m.api.LogInfo("Project assigned successfully",
				"project_id", projectID,
				"assignee", assignedEmployee.EmployeeName)
		}
	}

	user, _ := m.api.GetUser(userID)
	isVietnamese := detectUserLanguage(user)

	var successMsg string
	if len(projectRequest.AssignedToEmployees) > 0 {
		assigneeNames := make([]string, len(projectRequest.AssignedToEmployees))
		for i, emp := range projectRequest.AssignedToEmployees {
			assigneeNames[i] = emp.EmployeeName
		}
		assigneesStr := strings.Join(assigneeNames, ", ")

		if len(assignmentErrors) == 0 {
			if isVietnamese {
				successMsg = fmt.Sprintf("✅ Đã tạo dự án thành công: **%s** và phân công cho **%s**!", projectID, assigneesStr)
			} else {
				successMsg = fmt.Sprintf("✅ Successfully created project: **%s** and assigned to **%s**!", projectID, assigneesStr)
			}
		} else {
			if isVietnamese {
				successMsg = fmt.Sprintf("✅ Đã tạo dự án: **%s**. Phân công thành công cho **%s**. Lỗi phân công: **%s**.",
					projectID, assigneesStr, strings.Join(assignmentErrors, ", "))
			} else {
				successMsg = fmt.Sprintf("✅ Created project: **%s**. Successfully assigned to **%s**. Assignment failed for: **%s**.",
					projectID, assigneesStr, strings.Join(assignmentErrors, ", "))
			}
		}
	} else {
		if isVietnamese {
			successMsg = fmt.Sprintf("✅ Đã tạo dự án thành công: **%s**!", projectID)
		} else {
			successMsg = fmt.Sprintf("✅ Successfully created project: **%s**!", projectID)
		}
	}

	return &erp_modules.ModuleResponse{
		Success:     true,
		Message:     successMsg,
		ActionTaken: "create_project",
		Data: map[string]interface{}{
			"project_id":         projectID,
			"project_name":       projectRequest.ProjectName,
			"description":        projectRequest.Description,
			"priority":           projectRequest.Priority,
			"assigned_employees": projectRequest.AssignedToEmployees,
			"assignment_errors":  assignmentErrors,
		},
	}, nil
}

// handleCreateTask processes task creation with multi-employee assignment
func (m *ProjectManagementModule) handleCreateTask(employeeID, userID string, data map[string]interface{}, creatorEmail string) (*erp_modules.ModuleResponse, error) {
	var taskRequest TaskCreationRequest
	dataBytes, _ := json.Marshal(data)
	json.Unmarshal(dataBytes, &taskRequest)

	// Create task first
	taskID, err := m.erpClient.CreateTask(taskRequest, employeeID)
	if err != nil {
		user, _ := m.api.GetUser(userID)
		isVietnamese := detectUserLanguage(user)
		errorMsg := "⚠️ Có lỗi xảy ra khi tạo task trong hệ thống. Vui lòng thử lại."
		if !isVietnamese {
			errorMsg = "⚠️ An error occurred while creating task in the system. Please try again."
		}
		return &erp_modules.ModuleResponse{
			Success: false,
			Message: errorMsg,
			Error:   err.Error(),
		}, nil
	}

	// Assign to multiple employees if any
	var assignmentErrors []string
	for _, assignedEmployee := range taskRequest.AssignedToEmployees {
		err := m.erpClient.AssignTaskToEmployee(taskID, creatorEmail, assignedEmployee.Email, taskRequest.Priority)
		if err != nil {
			m.api.LogWarn("Task created but assignment failed",
				"task_id", taskID,
				"assignee", assignedEmployee.EmployeeName,
				"error", err.Error())
			assignmentErrors = append(assignmentErrors, assignedEmployee.EmployeeName)
		} else {
			m.api.LogInfo("Task assigned successfully",
				"task_id", taskID,
				"assignee", assignedEmployee.EmployeeName)
		}
	}

	user, _ := m.api.GetUser(userID)
	isVietnamese := detectUserLanguage(user)

	var successMsg string
	if len(taskRequest.AssignedToEmployees) > 0 {
		assigneeNames := make([]string, len(taskRequest.AssignedToEmployees))
		for i, emp := range taskRequest.AssignedToEmployees {
			assigneeNames[i] = emp.EmployeeName
		}
		assigneesStr := strings.Join(assigneeNames, ", ")

		if len(assignmentErrors) == 0 {
			if isVietnamese {
				successMsg = fmt.Sprintf("✅ Đã tạo task thành công: **%s** và phân công cho **%s**!", taskID, assigneesStr)
			} else {
				successMsg = fmt.Sprintf("✅ Successfully created task: **%s** and assigned to **%s**!", taskID, assigneesStr)
			}
		} else {
			if isVietnamese {
				successMsg = fmt.Sprintf("✅ Đã tạo task: **%s**. Phân công thành công cho **%s**. Lỗi phân công: **%s**.",
					taskID, assigneesStr, strings.Join(assignmentErrors, ", "))
			} else {
				successMsg = fmt.Sprintf("✅ Created task: **%s**. Successfully assigned to **%s**. Assignment failed for: **%s**.",
					taskID, assigneesStr, strings.Join(assignmentErrors, ", "))
			}
		}
	} else {
		if isVietnamese {
			successMsg = fmt.Sprintf("✅ Đã tạo task thành công: **%s**!", taskID)
		} else {
			successMsg = fmt.Sprintf("✅ Successfully created task: **%s**!", taskID)
		}
	}

	return &erp_modules.ModuleResponse{
		Success:     true,
		Message:     successMsg,
		ActionTaken: "create_task",
		Data: map[string]interface{}{
			"task_id":            taskID,
			"task_name":          taskRequest.Subject,
			"description":        taskRequest.Description,
			"assigned_employees": taskRequest.AssignedToEmployees,
			"assignment_errors":  assignmentErrors,
			"priority":           taskRequest.Priority,
			"project":            taskRequest.Project,
		},
	}, nil
}

// getEmployeeIDFromUser gets employee ID from user
func (m *ProjectManagementModule) getEmployeeIDFromUser(user *model.User) (string, error) {
	chatID := user.Id
	employeeID, err := m.erpClient.GetEmployeeByChatID(chatID)
	if err != nil {
		return "", fmt.Errorf("failed to get employee by chat ID %s: %w", chatID, err)
	}
	return employeeID, nil
}

// getEmployeeEmailFromUser gets employee email from user
func (m *ProjectManagementModule) getEmployeeEmailFromUser(user *model.User) (string, error) {
	employeeID, err := m.getEmployeeIDFromUser(user)
	if err != nil {
		return "", err
	}
	email, err := m.erpClient.GetEmployeeEmail(employeeID)
	if err != nil {
		return "", fmt.Errorf("failed to get email for employee %s: %w", employeeID, err)
	}
	return email, nil
}
