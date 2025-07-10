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
	confirmationState                 map[string]*ProjectManagementConfirmation
	multiEmployeeDisambiguationState  map[string]*MultiEmployeeDisambiguationConfirmation
	existingEntityDisambiguationState map[string]*ExistingEntityDisambiguationConfirmation
	projectTaskDisambiguationState    map[string]*ProjectTaskDisambiguationConfirmation
	processingStates                  map[string]*ProcessingState // NEW
	confirmationMutex                 sync.RWMutex
}

// NewConfirmationManager creates a new confirmation manager
func NewConfirmationManager() *ConfirmationManager {
	return &ConfirmationManager{
		confirmationState:                 make(map[string]*ProjectManagementConfirmation),
		multiEmployeeDisambiguationState:  make(map[string]*MultiEmployeeDisambiguationConfirmation),
		existingEntityDisambiguationState: make(map[string]*ExistingEntityDisambiguationConfirmation),
		projectTaskDisambiguationState:    make(map[string]*ProjectTaskDisambiguationConfirmation),
	}
}

// Extended ConfirmationManager methods for processing states
func (cm *ConfirmationManager) StoreProcessingState(userID string, state *ProcessingState) {
	cm.confirmationMutex.Lock()
	defer cm.confirmationMutex.Unlock()
	if cm.processingStates == nil {
		cm.processingStates = make(map[string]*ProcessingState)
	}
	cm.processingStates[userID] = state
	// Clear other states
	delete(cm.confirmationState, userID)
	delete(cm.multiEmployeeDisambiguationState, userID)
	delete(cm.existingEntityDisambiguationState, userID)
	delete(cm.projectTaskDisambiguationState, userID)
}

func (cm *ConfirmationManager) GetProcessingState(userID string) (*ProcessingState, bool) {
	cm.confirmationMutex.RLock()
	defer cm.confirmationMutex.RUnlock()
	if cm.processingStates == nil {
		return nil, false
	}
	state, exists := cm.processingStates[userID]
	return state, exists
}

func (cm *ConfirmationManager) ClearProcessingState(userID string) {
	cm.confirmationMutex.Lock()
	defer cm.confirmationMutex.Unlock()
	if cm.processingStates != nil {
		delete(cm.processingStates, userID)
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

// Update ClearPendingConfirmation to also clear processing states
func (cm *ConfirmationManager) ClearPendingConfirmation(userID string) {
	cm.confirmationMutex.Lock()
	defer cm.confirmationMutex.Unlock()
	delete(cm.confirmationState, userID)
	delete(cm.multiEmployeeDisambiguationState, userID)
	delete(cm.existingEntityDisambiguationState, userID)
	delete(cm.projectTaskDisambiguationState, userID)
	if cm.processingStates != nil {
		delete(cm.processingStates, userID)
	}
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

// StorePendingExistingEntityDisambiguation stores a pending existing entity disambiguation
func (cm *ConfirmationManager) StorePendingExistingEntityDisambiguation(userID string, disambiguation *ExistingEntityDisambiguationConfirmation) {
	cm.confirmationMutex.Lock()
	defer cm.confirmationMutex.Unlock()
	cm.existingEntityDisambiguationState[userID] = disambiguation
	// Clear other states
	delete(cm.confirmationState, userID)
	delete(cm.multiEmployeeDisambiguationState, userID)
	delete(cm.projectTaskDisambiguationState, userID)
}

// GetPendingExistingEntityDisambiguation retrieves a pending existing entity disambiguation
func (cm *ConfirmationManager) GetPendingExistingEntityDisambiguation(userID string) (*ExistingEntityDisambiguationConfirmation, bool) {
	cm.confirmationMutex.RLock()
	defer cm.confirmationMutex.RUnlock()
	disambiguation, exists := cm.existingEntityDisambiguationState[userID]
	return disambiguation, exists
}

// StorePendingProjectTaskDisambiguation stores a pending project task disambiguation
func (cm *ConfirmationManager) StorePendingProjectTaskDisambiguation(userID string, disambiguation *ProjectTaskDisambiguationConfirmation) {
	cm.confirmationMutex.Lock()
	defer cm.confirmationMutex.Unlock()
	cm.projectTaskDisambiguationState[userID] = disambiguation
	// Clear other states
	delete(cm.confirmationState, userID)
	delete(cm.multiEmployeeDisambiguationState, userID)
	delete(cm.existingEntityDisambiguationState, userID)
}

// GetPendingProjectTaskDisambiguation retrieves a pending project task disambiguation
func (cm *ConfirmationManager) GetPendingProjectTaskDisambiguation(userID string) (*ProjectTaskDisambiguationConfirmation, bool) {
	cm.confirmationMutex.RLock()
	defer cm.confirmationMutex.RUnlock()
	disambiguation, exists := cm.projectTaskDisambiguationState[userID]
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

	// FIRST: Process assignees to resolve employees BEFORE checking existing projects
	var resolvedEmployees []AssignedEmployee
	if len(projectRequest.AssignedToNames) > 0 {
		assigneeResult, err := m.resolveMultipleAssignees(projectRequest.AssignedToNames)
		if err != nil {
			m.api.LogError("Failed to resolve project assignees", "error", err.Error())
		} else if assigneeResult.RequiresDisambiguation {
			// Return disambiguation request
			return m.requestMultiEmployeeDisambiguation(ctx, "create_project", "project", projectRequest, employeeID, assigneeResult)
		} else {
			resolvedEmployees = assigneeResult.ResolvedEmployees
			projectRequest.AssignedToEmployees = resolvedEmployees
		}
	}

	// SECOND: Check for existing projects with similar names
	existingProjects, err := m.erpClient.SearchProjectsByName(projectRequest.ProjectName)
	if err != nil {
		m.api.LogError("Failed to search existing projects", "error", err.Error())
		// Continue with creation even if search fails
	} else if len(existingProjects) > 0 {
		// Filter high confidence matches
		var highConfidenceProjects []Project
		for _, project := range existingProjects {
			if project.MatchConfidence >= 0.85 {
				highConfidenceProjects = append(highConfidenceProjects, project)
			}
		}

		if len(highConfidenceProjects) > 0 {
			// Store request for disambiguation WITH resolved employees
			projectRequest.AssignedToEmployees = resolvedEmployees
			return m.requestExistingEntityDisambiguation(ctx, "project", "create_project", projectRequest, employeeID, highConfidenceProjects, nil)
		}
	}

	// No high confidence matches, proceed with normal flow
	return m.handleProjectCreationFlow(employeeID, ctx, projectRequest)
}

// requestExistingEntityDisambiguation requests user to choose between creating new or using existing
func (m *ProjectManagementModule) requestExistingEntityDisambiguation(
	ctx *erp_modules.ModuleContext,
	entityType, action string,
	originalRequest interface{},
	employeeID string,
	existingProjects []Project,
	existingTasks []Task,
) (*erp_modules.ModuleResponse, error) {

	// Convert original request to map
	requestMap := make(map[string]interface{})
	requestBytes, _ := json.Marshal(originalRequest)
	json.Unmarshal(requestBytes, &requestMap)

	// Get creator's email
	creatorEmail, err := m.getEmployeeEmailFromUser(ctx.User)
	if err != nil {
		m.api.LogWarn("Failed to get creator email for entity disambiguation", "error", err.Error())
		creatorEmail = "demo@example.com"
	}

	// Store pending disambiguation
	disambiguation := &ExistingEntityDisambiguationConfirmation{
		UserID:          ctx.User.Id,
		Type:            entityType,
		Action:          action,
		OriginalRequest: requestMap,
		CreatedAt:       time.Now().UnixMilli(),
		EmployeeID:      employeeID,
		CreatorEmail:    creatorEmail,
		EntityType:      entityType,
	}

	if entityType == "project" {
		var entities []interface{}
		for _, p := range existingProjects {
			entities = append(entities, p)
		}
		disambiguation.ExistingEntities = entities
	} else {
		var entities []interface{}
		for _, t := range existingTasks {
			entities = append(entities, t)
		}
		disambiguation.ExistingEntities = entities
	}

	m.confirmationManager.StorePendingExistingEntityDisambiguation(ctx.User.Id, disambiguation)

	// Generate disambiguation message
	disambiguationMsg, err := m.generateExistingEntityDisambiguationMessage(ctx, disambiguation)
	if err != nil {
		isVietnamese := detectUserLanguage(ctx.User)
		errorMsg := "⚠️ Có lỗi xảy ra khi tạo tin nhắn lựa chọn."
		if !isVietnamese {
			errorMsg = "⚠️ An error occurred while creating selection message."
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
		ActionTaken: "request_existing_entity_disambiguation",
		Data: map[string]interface{}{
			"awaiting_disambiguation": true,
			"type":                    "existing_entity",
			"entity_type":             entityType,
			"matches_count":           len(disambiguation.ExistingEntities),
		},
	}, nil
}

// handleTaskProjectSelection handles project selection for task creation
func (m *ProjectManagementModule) handleTaskProjectSelection(employeeID string, ctx *erp_modules.ModuleContext, taskRequest *TaskCreationRequest) (*erp_modules.ModuleResponse, error) {
	// Search for matching projects
	matchingProjects, err := m.erpClient.SearchProjectsByName(taskRequest.Project)
	if err != nil {
		m.api.LogError("Failed to search projects for task", "error", err.Error())
		// Continue with task creation without project
		taskRequest.Project = ""
		return m.handleTaskCreationFlow(employeeID, ctx, taskRequest)
	}

	// Filter high confidence matches
	var highConfidenceProjects []Project
	for _, project := range matchingProjects {
		if project.MatchConfidence >= 0.85 {
			highConfidenceProjects = append(highConfidenceProjects, project)
		}
	}

	if len(highConfidenceProjects) == 0 {
		// No matches, proceed without project
		taskRequest.Project = ""
		return m.handleTaskCreationFlow(employeeID, ctx, taskRequest)
	}

	if len(highConfidenceProjects) == 1 {
		// Single match, use it
		taskRequest.Project = highConfidenceProjects[0].Name
		return m.handleTaskCreationFlow(employeeID, ctx, taskRequest)
	}

	// Multiple matches, need disambiguation
	return m.requestProjectTaskDisambiguation(ctx, taskRequest, employeeID, highConfidenceProjects)
}

// requestProjectTaskDisambiguation requests user to select a project for task
func (m *ProjectManagementModule) requestProjectTaskDisambiguation(
	ctx *erp_modules.ModuleContext,
	taskRequest *TaskCreationRequest,
	employeeID string,
	matchingProjects []Project,
) (*erp_modules.ModuleResponse, error) {

	// Get creator's email
	creatorEmail, err := m.getEmployeeEmailFromUser(ctx.User)
	if err != nil {
		m.api.LogWarn("Failed to get creator email for project disambiguation", "error", err.Error())
		creatorEmail = "demo@example.com"
	}

	// Store pending disambiguation
	disambiguation := &ProjectTaskDisambiguationConfirmation{
		UserID:              ctx.User.Id,
		OriginalTaskRequest: taskRequest,
		CreatedAt:           time.Now().UnixMilli(),
		EmployeeID:          employeeID,
		CreatorEmail:        creatorEmail,
		MatchingProjects:    matchingProjects,
	}

	m.confirmationManager.StorePendingProjectTaskDisambiguation(ctx.User.Id, disambiguation)

	// Generate disambiguation message
	disambiguationMsg, err := m.generateProjectTaskDisambiguationMessage(ctx, disambiguation)
	if err != nil {
		isVietnamese := detectUserLanguage(ctx.User)
		errorMsg := "⚠️ Có lỗi xảy ra khi tạo tin nhắn lựa chọn dự án."
		if !isVietnamese {
			errorMsg = "⚠️ An error occurred while creating project selection message."
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
		ActionTaken: "request_project_task_disambiguation",
		Data: map[string]interface{}{
			"awaiting_disambiguation": true,
			"type":                    "project_selection",
			"projects_count":          len(matchingProjects),
		},
	}, nil
}

// handleProjectCreationFlow handles the project creation flow (existing logic)
func (m *ProjectManagementModule) handleProjectCreationFlow(employeeID string, ctx *erp_modules.ModuleContext, projectRequest *ProjectCreationRequest) (*erp_modules.ModuleResponse, error) {
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

// handleTaskCreationFlow handles the task creation flow (existing logic)
func (m *ProjectManagementModule) handleTaskCreationFlow(employeeID string, ctx *erp_modules.ModuleContext, taskRequest *TaskCreationRequest) (*erp_modules.ModuleResponse, error) {
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

	// FIRST: Process assignees to resolve employees BEFORE checking existing tasks
	var resolvedEmployees []AssignedEmployee
	if len(taskRequest.AssignedToNames) > 0 {
		assigneeResult, err := m.resolveMultipleAssignees(taskRequest.AssignedToNames)
		if err != nil {
			m.api.LogError("Failed to resolve task assignees", "error", err.Error())
		} else if assigneeResult.RequiresDisambiguation {
			// Return disambiguation request - but we need to save the task context too
			return m.requestMultiEmployeeDisambiguation(ctx, "create_task", "task", taskRequest, employeeID, assigneeResult)
		} else {
			resolvedEmployees = assigneeResult.ResolvedEmployees
			taskRequest.AssignedToEmployees = resolvedEmployees
		}
	}

	// SECOND: Check for existing tasks with similar names
	existingTasks, err := m.erpClient.SearchTasksByName(taskRequest.Subject)
	if err != nil {
		m.api.LogError("Failed to search existing tasks", "error", err.Error())
		// Continue even if search fails
	} else if len(existingTasks) > 0 {
		// Filter high confidence matches
		var highConfidenceTasks []Task
		for _, task := range existingTasks {
			if task.MatchConfidence >= 0.85 {
				highConfidenceTasks = append(highConfidenceTasks, task)
			}
		}

		if len(highConfidenceTasks) > 0 {
			// Store request for disambiguation WITH resolved employees
			taskRequest.AssignedToEmployees = resolvedEmployees
			return m.requestExistingEntityDisambiguation(ctx, "task", "create_task", taskRequest, employeeID, nil, highConfidenceTasks)
		}
	}

	// No high confidence matches, proceed with project search if project mentioned
	if taskRequest.Project != "" {
		return m.handleTaskProjectSelection(employeeID, ctx, taskRequest)
	}

	// No existing matches and no project, proceed with normal flow
	return m.handleTaskCreationFlow(employeeID, ctx, taskRequest)
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
