// Copyright (c) 2023-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package project_management

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/mattermost/mattermost-plugin-ai/server/erp_modules"
	"github.com/mattermost/mattermost-plugin-ai/server/llm"
)

// ProjectManagementModule handles project and task management operations
type ProjectManagementModule struct {
	config              ProjectManagementConfig
	erpClient           *ERPClient
	api                 PluginAPI
	prompts             PromptsInterface
	getLLM              func() llm.LanguageModel
	confirmationManager *ConfirmationManager
	processor           *MultiStepProcessor             // NEW
	responseHandler     *ProcessingStateResponseHandler // NEW
}

// NewProjectManagementModule creates a new project management module
func NewProjectManagementModule(
	config ProjectManagementConfig,
	httpClient *http.Client,
	prompts PromptsInterface,
	getLLM func() llm.LanguageModel,
	api PluginAPI,
) *ProjectManagementModule {
	// Validate configuration
	if err := ValidateProjectManagementConfig(config); err != nil {
		api.LogError("Invalid project management configuration", "error", err.Error())
	}

	// Create ERP client
	erpClient := NewERPClient(config, httpClient, api)

	// Create confirmation manager
	confirmationManager := NewConfirmationManager()

	// Create the module instance
	module := &ProjectManagementModule{
		config:              config,
		erpClient:           erpClient,
		api:                 api,
		prompts:             prompts,
		getLLM:              getLLM,
		confirmationManager: confirmationManager,
	}

	// Initialize processor and response handler
	module.processor = NewMultiStepProcessor(module)
	module.responseHandler = NewProcessingStateResponseHandler(module, module.processor)

	return module
}

// GetCategory returns the category this module handles
func (m *ProjectManagementModule) GetCategory() string {
	return "project_management"
}

// GetSupportedActions returns list of actions this module supports
func (m *ProjectManagementModule) GetSupportedActions() []string {
	return []string{"create_project", "create_task"}
}

// CanHandle determines if this module can handle the given intent
func (m *ProjectManagementModule) CanHandle(intent *erp_modules.Intent) bool {
	if intent.Category != "project_management" {
		return false
	}
	return isValidAction(intent.Action)
}

// ProcessUserMessage handles user messages including confirmations, modifications, and multi-employee disambiguations
func (m *ProjectManagementModule) ProcessUserMessage(ctx *erp_modules.ModuleContext, message string) (*erp_modules.ModuleResponse, error) {
	m.api.LogDebug("ProcessUserMessage called", "user_id", ctx.User.Id, "message", message)

	// Sanitize user input
	message = sanitizeUserInput(message)

	// PRIORITY 1: Check for processing states FIRST
	if processingState, hasProcessingState := m.confirmationManager.GetProcessingState(ctx.User.Id); hasProcessingState {
		m.api.LogDebug("Found processing state", "user_id", ctx.User.Id, "step", processingState.Step)
		response, err := m.responseHandler.HandleProcessingStateResponse(ctx, message, processingState)
		if err != nil {
			m.api.LogError("Error handling processing state response", "error", err.Error())
			// Clear state on error
			m.confirmationManager.ClearProcessingState(ctx.User.Id)
		}
		return response, err
	}

	// PRIORITY 2: Check for existing entity disambiguation (legacy support)
	existingEntityDisambiguation, hasExistingEntityDisambiguation := m.confirmationManager.GetPendingExistingEntityDisambiguation(ctx.User.Id)
	if hasExistingEntityDisambiguation {
		m.api.LogDebug("Found pending existing entity disambiguation", "user_id", ctx.User.Id)
		return m.handleExistingEntityDisambiguationResponse(ctx, message, existingEntityDisambiguation)
	}

	// PRIORITY 3: Check for project task disambiguation (legacy support)
	projectTaskDisambiguation, hasProjectTaskDisambiguation := m.confirmationManager.GetPendingProjectTaskDisambiguation(ctx.User.Id)
	if hasProjectTaskDisambiguation {
		m.api.LogDebug("Found pending project task disambiguation", "user_id", ctx.User.Id)
		return m.handleProjectTaskDisambiguationResponse(ctx, message, projectTaskDisambiguation)
	}

	// PRIORITY 4: Check for multi-employee disambiguation (legacy support)
	multiDisambiguation, hasMultiDisambiguation := m.confirmationManager.GetPendingMultiEmployeeDisambiguation(ctx.User.Id)
	if hasMultiDisambiguation {
		m.api.LogDebug("Found pending multi-employee disambiguation", "user_id", ctx.User.Id)
		return m.handleMultiEmployeeDisambiguationResponse(ctx, message, multiDisambiguation)
	}

	// PRIORITY 5: Check for regular confirmation (legacy support)
	pending, hasPending := m.confirmationManager.GetPendingConfirmation(ctx.User.Id)
	if hasPending {
		m.api.LogDebug("Found pending confirmation", "user_id", ctx.User.Id)
		return m.handleRegularConfirmationResponse(ctx, message, pending)
	}

	// No pending states - not handling this message
	m.api.LogDebug("No pending confirmation found", "user_id", ctx.User.Id)
	return nil, nil
}

// handleExistingEntityDisambiguationResponse handles user response to existing entity selection
func (m *ProjectManagementModule) handleExistingEntityDisambiguationResponse(ctx *erp_modules.ModuleContext, message string, disambiguation *ExistingEntityDisambiguationConfirmation) (*erp_modules.ModuleResponse, error) {
	m.api.LogInfo("Handling existing entity disambiguation response",
		"user_id", ctx.User.Id,
		"message", message,
		"entity_type", disambiguation.EntityType)

	// Parse disambiguation response using LLM
	disambiguationResponse, err := m.parseExistingEntityDisambiguationResponse(ctx, message, disambiguation)
	if err != nil {
		m.api.LogError("Failed to parse existing entity disambiguation response", "error", err.Error())
		isVietnamese := detectUserLanguage(ctx.User)
		errorMsg := "⚠️ Không thể hiểu lựa chọn của bạn. Vui lòng trả lời 'tạo mới', số thứ tự, hoặc 'hủy'."
		if !isVietnamese {
			errorMsg = "⚠️ Cannot understand your selection. Please reply 'create new', number, or 'cancel'."
		}
		return &erp_modules.ModuleResponse{
			Success: false,
			Message: errorMsg,
		}, nil
	}

	m.api.LogInfo("Parsed existing entity disambiguation response",
		"intent", disambiguationResponse.Intent,
		"selected_index", disambiguationResponse.SelectedIndex)

	// Clear pending disambiguation
	m.confirmationManager.ClearPendingConfirmation(ctx.User.Id)

	switch disambiguationResponse.Intent {
	case "create_new":
		// User wants to create new, proceed with original flow
		if disambiguation.Type == "project" {
			var projectRequest ProjectCreationRequest
			requestBytes, _ := json.Marshal(disambiguation.OriginalRequest)
			json.Unmarshal(requestBytes, &projectRequest)
			return m.handleProjectCreationFlow(disambiguation.EmployeeID, ctx, &projectRequest)
		} else {
			var taskRequest TaskCreationRequest
			requestBytes, _ := json.Marshal(disambiguation.OriginalRequest)
			json.Unmarshal(requestBytes, &taskRequest)

			// Handle project selection if needed
			if taskRequest.Project != "" {
				return m.handleTaskProjectSelection(disambiguation.EmployeeID, ctx, &taskRequest)
			}
			return m.handleTaskCreationFlow(disambiguation.EmployeeID, ctx, &taskRequest)
		}

	case "use_existing":
		// User wants to use existing entity for assignment only
		return m.handleUseExistingEntity(ctx, disambiguation, disambiguationResponse.SelectedIndex)

	case "cancel":
		return m.handleCancelAction(ctx.User.Id)

	default:
		isVietnamese := detectUserLanguage(ctx.User)
		errorMsg := "⚠️ Vui lòng trả lời 'tạo mới', số thứ tự, hoặc 'hủy'."
		if !isVietnamese {
			errorMsg = "⚠️ Please reply 'create new', number, or 'cancel'."
		}
		return &erp_modules.ModuleResponse{
			Success: false,
			Message: errorMsg,
		}, nil
	}
}

// handleProjectTaskDisambiguationResponse handles user response to project selection for task
func (m *ProjectManagementModule) handleProjectTaskDisambiguationResponse(ctx *erp_modules.ModuleContext, message string, disambiguation *ProjectTaskDisambiguationConfirmation) (*erp_modules.ModuleResponse, error) {
	m.api.LogInfo("Handling project task disambiguation response",
		"user_id", ctx.User.Id,
		"message", message)

	// Parse selection response using LLM
	selectionResponse, err := m.parseProjectSelectionResponse(ctx, message, disambiguation)
	if err != nil {
		m.api.LogError("Failed to parse project selection response", "error", err.Error())
		isVietnamese := detectUserLanguage(ctx.User)
		errorMsg := "⚠️ Không thể hiểu lựa chọn của bạn. Vui lòng chọn số thứ tự, 'không', hoặc 'hủy'."
		if !isVietnamese {
			errorMsg = "⚠️ Cannot understand your selection. Please choose number, 'no project', or 'cancel'."
		}
		return &erp_modules.ModuleResponse{
			Success: false,
			Message: errorMsg,
		}, nil
	}

	m.api.LogInfo("Parsed project selection response",
		"intent", selectionResponse.Intent,
		"selected_index", selectionResponse.SelectedIndex)

	// Clear pending disambiguation
	m.confirmationManager.ClearPendingConfirmation(ctx.User.Id)

	switch selectionResponse.Intent {
	case "select_project":
		// Validate index
		if selectionResponse.SelectedIndex < 1 || selectionResponse.SelectedIndex > len(disambiguation.MatchingProjects) {
			isVietnamese := detectUserLanguage(ctx.User)
			errorMsg := "⚠️ Số thứ tự không hợp lệ."
			if !isVietnamese {
				errorMsg = "⚠️ Invalid selection number."
			}
			return &erp_modules.ModuleResponse{
				Success: false,
				Message: errorMsg,
			}, nil
		}

		// Set selected project
		selectedProject := disambiguation.MatchingProjects[selectionResponse.SelectedIndex-1]
		disambiguation.OriginalTaskRequest.Project = selectedProject.Name

		return m.handleTaskCreationFlow(disambiguation.EmployeeID, ctx, disambiguation.OriginalTaskRequest)

	case "no_project":
		// Clear project and proceed
		disambiguation.OriginalTaskRequest.Project = ""
		return m.handleTaskCreationFlow(disambiguation.EmployeeID, ctx, disambiguation.OriginalTaskRequest)

	case "cancel":
		return m.handleCancelAction(ctx.User.Id)

	default:
		isVietnamese := detectUserLanguage(ctx.User)
		errorMsg := "⚠️ Vui lòng chọn số thứ tự, 'không', hoặc 'hủy'."
		if !isVietnamese {
			errorMsg = "⚠️ Please choose number, 'no project', or 'cancel'."
		}
		return &erp_modules.ModuleResponse{
			Success: false,
			Message: errorMsg,
		}, nil
	}
}

// handleUseExistingEntity handles assignment to existing project/task
func (m *ProjectManagementModule) handleUseExistingEntity(ctx *erp_modules.ModuleContext, disambiguation *ExistingEntityDisambiguationConfirmation, selectedIndex int) (*erp_modules.ModuleResponse, error) {
	// Validate index
	if selectedIndex < 1 || selectedIndex > len(disambiguation.ExistingEntities) {
		isVietnamese := detectUserLanguage(ctx.User)
		errorMsg := "⚠️ Số thứ tự không hợp lệ."
		if !isVietnamese {
			errorMsg = "⚠️ Invalid selection number."
		}
		return &erp_modules.ModuleResponse{
			Success: false,
			Message: errorMsg,
		}, nil
	}

	// Get selected entity
	selectedEntity := disambiguation.ExistingEntities[selectedIndex-1]

	// Extract assigned employees from original request - IMPROVED EXTRACTION
	var assignedEmployees []AssignedEmployee

	// Method 1: Try to get from assigned_to_employees field (already resolved)
	if assignedToEmployees, ok := disambiguation.OriginalRequest["assigned_to_employees"]; ok {
		if employees, ok := assignedToEmployees.([]interface{}); ok {
			for _, emp := range employees {
				if empBytes, err := json.Marshal(emp); err == nil {
					var assignedEmp AssignedEmployee
					if json.Unmarshal(empBytes, &assignedEmp) == nil {
						assignedEmployees = append(assignedEmployees, assignedEmp)
					}
				}
			}
		}
	}

	// Method 2: If no resolved employees, try to resolve from assigned_to_names
	if len(assignedEmployees) == 0 {
		if assignedToNames, ok := disambiguation.OriginalRequest["assigned_to_names"]; ok {
			if names, ok := assignedToNames.([]interface{}); ok {
				var nameStrings []string
				for _, name := range names {
					if nameStr, ok := name.(string); ok {
						nameStrings = append(nameStrings, nameStr)
					}
				}

				if len(nameStrings) > 0 {
					// Try to resolve the names now
					assigneeResult, err := m.resolveMultipleAssignees(nameStrings)
					if err != nil {
						m.api.LogError("Failed to resolve employees for existing entity assignment", "error", err.Error())
					} else if assigneeResult.RequiresDisambiguation {
						// Handle employee disambiguation for existing entity
						return m.handleEmployeeDisambiguationForExistingEntity(ctx, disambiguation, selectedIndex, assigneeResult)
					} else {
						assignedEmployees = assigneeResult.ResolvedEmployees
					}
				}
			}
		}
	}

	// Log what we found
	m.api.LogInfo("Extracting employees for existing entity assignment",
		"entity_type", disambiguation.EntityType,
		"resolved_employees_count", len(assignedEmployees),
		"original_request_keys", getMapKeys(disambiguation.OriginalRequest))

	// Only proceed with assignment if there are employees to assign
	if len(assignedEmployees) == 0 {
		isVietnamese := detectUserLanguage(ctx.User)
		var entityName string
		if disambiguation.EntityType == "project" {
			if projBytes, err := json.Marshal(selectedEntity); err == nil {
				var project Project
				if json.Unmarshal(projBytes, &project) == nil {
					entityName = project.ProjectName
				}
			}
		} else {
			if taskBytes, err := json.Marshal(selectedEntity); err == nil {
				var task Task
				if json.Unmarshal(taskBytes, &task) == nil {
					entityName = task.Subject
				}
			}
		}

		var successMsg string
		if isVietnamese {
			if disambiguation.EntityType == "project" {
				successMsg = fmt.Sprintf("Đã chọn dự án hiện có: **%s**. Tuy nhiên, không có nhân viên nào được chỉ định để phân công.", entityName)
			} else {
				successMsg = fmt.Sprintf("Đã chọn task hiện có: **%s**. Tuy nhiên, không có nhân viên nào được chỉ định để phân công.", entityName)
			}
		} else {
			if disambiguation.EntityType == "project" {
				successMsg = fmt.Sprintf("Selected existing project: **%s**. However, no employees were specified for assignment.", entityName)
			} else {
				successMsg = fmt.Sprintf("Selected existing task: **%s**. However, no employees were specified for assignment.", entityName)
			}
		}

		return &erp_modules.ModuleResponse{
			Success:     true,
			Message:     successMsg,
			ActionTaken: "use_existing_without_assignment",
			Data: map[string]interface{}{
				"entity_type": disambiguation.EntityType,
				"entity_name": entityName,
			},
		}, nil
	}

	// Perform assignments to existing entity
	var entityID, entityName string
	if disambiguation.EntityType == "project" {
		if projBytes, err := json.Marshal(selectedEntity); err == nil {
			var project Project
			if json.Unmarshal(projBytes, &project) == nil {
				entityID = project.Name
				entityName = project.ProjectName
			}
		}
	} else {
		if taskBytes, err := json.Marshal(selectedEntity); err == nil {
			var task Task
			if json.Unmarshal(taskBytes, &task) == nil {
				entityID = task.Name
				entityName = task.Subject
			}
		}
	}

	// Assign to all employees
	var assignmentErrors []string
	priority := "Medium" // Default priority for assignments
	if originalPriority, ok := disambiguation.OriginalRequest["priority"]; ok {
		if priorityStr, ok := originalPriority.(string); ok && priorityStr != "" {
			priority = priorityStr
		}
	}

	for _, assignedEmployee := range assignedEmployees {
		var err error
		if disambiguation.EntityType == "project" {
			err = m.erpClient.AssignProjectToEmployee(entityID, disambiguation.CreatorEmail, assignedEmployee.Email, priority)
		} else {
			err = m.erpClient.AssignTaskToEmployee(entityID, disambiguation.CreatorEmail, assignedEmployee.Email, priority)
		}

		if err != nil {
			m.api.LogWarn("Assignment to existing entity failed",
				"entity_type", disambiguation.EntityType,
				"entity_id", entityID,
				"assignee", assignedEmployee.EmployeeName,
				"error", err.Error())
			assignmentErrors = append(assignmentErrors, assignedEmployee.EmployeeName)
		} else {
			m.api.LogInfo("Assignment to existing entity successful",
				"entity_type", disambiguation.EntityType,
				"entity_id", entityID,
				"assignee", assignedEmployee.EmployeeName)
		}
	}

	isVietnamese := detectUserLanguage(ctx.User)

	var successMsg string
	assigneeNames := make([]string, len(assignedEmployees))
	for i, emp := range assignedEmployees {
		assigneeNames[i] = emp.EmployeeName
	}
	assigneesStr := strings.Join(assigneeNames, ", ")

	if len(assignmentErrors) == 0 {
		if isVietnamese {
			if disambiguation.EntityType == "project" {
				successMsg = fmt.Sprintf("Đã phân công dự án hiện có **%s** cho **%s**!", entityName, assigneesStr)
			} else {
				successMsg = fmt.Sprintf("Đã phân công task hiện có **%s** cho **%s**!", entityName, assigneesStr)
			}
		} else {
			if disambiguation.EntityType == "project" {
				successMsg = fmt.Sprintf("Successfully assigned existing project **%s** to **%s**!", entityName, assigneesStr)
			} else {
				successMsg = fmt.Sprintf("Successfully assigned existing task **%s** to **%s**!", entityName, assigneesStr)
			}
		}
	} else {
		if isVietnamese {
			if disambiguation.EntityType == "project" {
				successMsg = fmt.Sprintf("Đã phân công dự án hiện có **%s**. Thành công: **%s**. Lỗi: **%s**.",
					entityName, assigneesStr, strings.Join(assignmentErrors, ", "))
			} else {
				successMsg = fmt.Sprintf("Đã phân công task hiện có **%s**. Thành công: **%s**. Lỗi: **%s**.",
					entityName, assigneesStr, strings.Join(assignmentErrors, ", "))
			}
		} else {
			if disambiguation.EntityType == "project" {
				successMsg = fmt.Sprintf("Assigned existing project **%s**. Successful: **%s**. Failed: **%s**.",
					entityName, assigneesStr, strings.Join(assignmentErrors, ", "))
			} else {
				successMsg = fmt.Sprintf("Assigned existing task **%s**. Successful: **%s**. Failed: **%s**.",
					entityName, assigneesStr, strings.Join(assignmentErrors, ", "))
			}
		}
	}

	return &erp_modules.ModuleResponse{
		Success:     true,
		Message:     successMsg,
		ActionTaken: "assign_existing_entity",
		Data: map[string]interface{}{
			"entity_type":        disambiguation.EntityType,
			"entity_id":          entityID,
			"entity_name":        entityName,
			"assigned_employees": assignedEmployees,
			"assignment_errors":  assignmentErrors,
		},
	}, nil
}

// handleEmployeeDisambiguationForExistingEntity handles employee disambiguation when assigning to existing entity
func (m *ProjectManagementModule) handleEmployeeDisambiguationForExistingEntity(
	ctx *erp_modules.ModuleContext,
	existingEntityDisambiguation *ExistingEntityDisambiguationConfirmation,
	selectedEntityIndex int,
	assigneeResult *AssigneeResolutionResult,
) (*erp_modules.ModuleResponse, error) {

	// This is a complex case where we need to:
	// 1. Store the existing entity selection
	// 2. Handle employee disambiguation
	// 3. Then assign to the selected existing entity

	// For now, we'll do the employee disambiguation and then continue with assignment
	// In a more complex implementation, you might want to store this state differently

	return m.requestMultiEmployeeDisambiguationForExistingEntity(
		ctx,
		existingEntityDisambiguation,
		selectedEntityIndex,
		assigneeResult,
	)
}

// requestMultiEmployeeDisambiguationForExistingEntity requests employee disambiguation for existing entity assignment
func (m *ProjectManagementModule) requestMultiEmployeeDisambiguationForExistingEntity(
	ctx *erp_modules.ModuleContext,
	existingEntityDisambiguation *ExistingEntityDisambiguationConfirmation,
	selectedEntityIndex int,
	assigneeResult *AssigneeResolutionResult,
) (*erp_modules.ModuleResponse, error) {

	// Convert existing entity request data for employee disambiguation
	multiEmployeeDisambiguation := &MultiEmployeeDisambiguationConfirmation{
		UserID:                    ctx.User.Id,
		Type:                      existingEntityDisambiguation.Type,
		Action:                    "assign_existing_entity",
		Data:                      existingEntityDisambiguation.OriginalRequest,
		CreatedAt:                 time.Now().UnixMilli(),
		EmployeeID:                existingEntityDisambiguation.EmployeeID,
		CreatorEmail:              existingEntityDisambiguation.CreatorEmail,
		UnresolvedEmployeeMatches: assigneeResult.UnresolvedEmployeeMatches,
		ResolvedEmployees:         assigneeResult.ResolvedEmployees,
		IsModification:            false,
	}

	// Store additional context about selected entity
	multiEmployeeDisambiguation.Data["selected_entity_index"] = selectedEntityIndex
	multiEmployeeDisambiguation.Data["existing_entities"] = existingEntityDisambiguation.ExistingEntities

	m.confirmationManager.StorePendingMultiEmployeeDisambiguation(ctx.User.Id, multiEmployeeDisambiguation)

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
		ActionTaken: "request_employee_disambiguation_for_existing_entity",
		Data: map[string]interface{}{
			"awaiting_disambiguation": true,
			"type":                    "employee_selection_for_existing",
			"unresolved_count":        len(assigneeResult.UnresolvedEmployeeMatches),
			"resolved_count":          len(assigneeResult.ResolvedEmployees),
		},
	}, nil
}

// getMapKeys returns the keys of a map for debugging
func getMapKeys(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// handleMultiEmployeeDisambiguationResponse handles user response to multi-employee selection
func (m *ProjectManagementModule) handleMultiEmployeeDisambiguationResponse(ctx *erp_modules.ModuleContext, message string, disambiguation *MultiEmployeeDisambiguationConfirmation) (*erp_modules.ModuleResponse, error) {
	m.api.LogInfo("Handling multi-employee disambiguation response",
		"user_id", ctx.User.Id,
		"message", message,
		"unresolved_count", len(disambiguation.UnresolvedEmployeeMatches))

	// Parse disambiguation response using LLM
	disambiguationResponse, err := m.parseMultiEmployeeDisambiguationResponse(ctx, message, disambiguation)
	if err != nil {
		m.api.LogError("Failed to parse multi-employee disambiguation response", "error", err.Error())
		isVietnamese := detectUserLanguage(ctx.User)
		errorMsg := "⚠️ Không thể hiểu lựa chọn của bạn. Vui lòng chọn bằng số thứ tự (ví dụ: 1, 3, 5)."
		if !isVietnamese {
			errorMsg = "⚠️ Cannot understand your selection. Please choose by numbers (example: 1, 3, 5)."
		}
		return &erp_modules.ModuleResponse{
			Success: false,
			Message: errorMsg,
		}, nil
	}

	m.api.LogInfo("Parsed disambiguation response",
		"intent", disambiguationResponse.Intent,
		"selected_indexes", disambiguationResponse.SelectedIndexes)

	// Clear pending disambiguation IMMEDIATELY after parsing
	m.confirmationManager.ClearPendingConfirmation(ctx.User.Id)

	switch disambiguationResponse.Intent {
	case "index_selection":
		// Validate that we have selections
		if len(disambiguationResponse.SelectedIndexes) == 0 {
			isVietnamese := detectUserLanguage(ctx.User)
			errorMsg := "⚠️ Vui lòng chọn ít nhất một nhân viên."
			if !isVietnamese {
				errorMsg = "⚠️ Please select at least one employee."
			}
			return &erp_modules.ModuleResponse{
				Success: false,
				Message: errorMsg,
			}, nil
		}

		// Process the selections
		selectedEmployees, err := m.resolveDisambiguatedEmployees(
			disambiguation.UnresolvedEmployeeMatches,
			disambiguationResponse.SelectedIndexes,
		)
		if err != nil {
			isVietnamese := detectUserLanguage(ctx.User)
			errorMsg := "⚠️ Có lỗi xảy ra khi xử lý lựa chọn nhân viên: " + err.Error()
			if !isVietnamese {
				errorMsg = "⚠️ An error occurred while processing employee selection: " + err.Error()
			}
			return &erp_modules.ModuleResponse{
				Success: false,
				Message: errorMsg,
			}, nil
		}

		// Combine with previously resolved employees
		allResolvedEmployees := append(disambiguation.ResolvedEmployees, selectedEmployees...)

		// Update the data with all resolved employees
		disambiguation.Data["assigned_to_employees"] = allResolvedEmployees

		// Create and store the confirmation
		confirmation := &ProjectManagementConfirmation{
			UserID:       disambiguation.UserID,
			Type:         disambiguation.Type,
			Action:       disambiguation.Action,
			Data:         disambiguation.Data,
			CreatedAt:    time.Now().UnixMilli(),
			EmployeeID:   disambiguation.EmployeeID,
			CreatorEmail: disambiguation.CreatorEmail,
		}

		// Store the confirmation state
		m.confirmationManager.StorePendingConfirmation(ctx.User.Id, confirmation)

		// Generate success message with confirmation
		isVietnamese := detectUserLanguage(ctx.User)
		var selectedMsg strings.Builder
		if isVietnamese {
			selectedMsg.WriteString("**Đã chọn nhân viên:**\n")
		} else {
			selectedMsg.WriteString("**Selected employees:**\n")
		}
		for _, emp := range allResolvedEmployees {
			selectedMsg.WriteString(fmt.Sprintf("- **%s** (%s)\n", emp.EmployeeName, emp.Email))
		}
		selectedMsg.WriteString("\n")

		// Generate confirmation message
		confirmationMsg, err := m.generateConfirmationMessage(ctx, disambiguation.Type, disambiguation.Data)
		if err != nil {
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
			Message:     selectedMsg.String() + confirmationMsg,
			ActionTaken: "multi_employee_selected_proceed_confirmation",
			Data: map[string]interface{}{
				"awaiting_confirmation": true,
				"type":                  disambiguation.Type,
				"selected_employees":    len(allResolvedEmployees),
			},
		}, nil

	case "cancel":
		return m.handleCancelAction(ctx.User.Id)
	default:
		isVietnamese := detectUserLanguage(ctx.User)
		errorMsg := "⚠️ Vui lòng chọn bằng số thứ tự (ví dụ: 1, 3, 5) hoặc hủy bỏ (cancel)."
		if !isVietnamese {
			errorMsg = "⚠️ Please select by numbers (example: 1, 3, 5) or cancel."
		}
		return &erp_modules.ModuleResponse{
			Success: false,
			Message: errorMsg,
		}, nil
	}
}

// Helper method to handle regular confirmation responses (legacy support)
func (m *ProjectManagementModule) handleRegularConfirmationResponse(ctx *erp_modules.ModuleContext, message string, pending *ProjectManagementConfirmation) (*erp_modules.ModuleResponse, error) {
	// Parse user response using LLM
	userResponse, err := m.parseConfirmationResponse(ctx, message, pending)
	if err != nil {
		m.api.LogError("Failed to parse user response", "error", err.Error())
		isVietnamese := detectUserLanguage(ctx.User)
		errorMsg := "⚠️ Không thể hiểu phản hồi của bạn. Vui lòng thử lại."
		if !isVietnamese {
			errorMsg = "⚠️ Cannot understand your response. Please try again."
		}
		return &erp_modules.ModuleResponse{
			Success: false,
			Message: errorMsg,
		}, nil
	}

	// Clear pending confirmation
	m.confirmationManager.ClearPendingConfirmation(ctx.User.Id)

	switch userResponse.Intent {
	case "confirm":
		return m.executeConfirmedAction(ctx, pending)
	case "modify":
		return m.handleModifyAction(ctx, pending, userResponse.Modifications)
	case "cancel":
		return m.handleCancelAction(ctx.User.Id)
	default:
		isVietnamese := detectUserLanguage(ctx.User)
		errorMsg := "⚠️ Vui lòng xác nhận (có/yes), chỉnh sửa thông tin, hoặc hủy bỏ (không/cancel)."
		if !isVietnamese {
			errorMsg = "⚠️ Please confirm (yes), modify information, or cancel (no/cancel)."
		}
		return &erp_modules.ModuleResponse{
			Success: false,
			Message: errorMsg,
		}, nil
	}
}

// Execute processes the project management intent
func (m *ProjectManagementModule) Execute(ctx *erp_modules.ModuleContext, intent *erp_modules.Intent) (*erp_modules.ModuleResponse, error) {
	isVietnamese := detectUserLanguage(ctx.User)

	// Get employee ID
	employeeID, err := m.getEmployeeIDFromUser(ctx.User)
	if err != nil {
		errorMsg := "Không tìm thấy thông tin nhân viên của bạn trong hệ thống ERP. Vui lòng liên hệ quản trị viên."
		if !isVietnamese {
			errorMsg = "Cannot find your employee information in the ERP system. Please contact administrator."
		}
		return &erp_modules.ModuleResponse{
			Success: false,
			Message: errorMsg,
			Error:   err.Error(),
		}, nil
	}

	// Clear any existing states before starting new workflow
	m.confirmationManager.ClearPendingConfirmation(ctx.User.Id)
	m.confirmationManager.ClearProcessingState(ctx.User.Id)

	// Execute specific action using state machine
	switch intent.Action {
	case "create_project":
		return m.processor.ProcessProjectCreation(employeeID, ctx, intent)
	case "create_task":
		return m.processor.ProcessTaskCreation(employeeID, ctx, intent)
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
func (m *ProjectManagementModule) GetDescription() string {
	return "Quản lý dự án và công việc: tạo dự án mới, tạo task, phân công công việc cho nhiều nhân viên với khả năng gán nhân viên tự động và xử lý trường hợp trùng tên"
}

// GetActionExamples returns examples of user messages for each action
func (m *ProjectManagementModule) GetActionExamples() map[string][]string {
	return map[string][]string{
		"create_project": {
			"tạo dự án mới",
			"khởi tạo project",
			"tạo project mới",
			"bắt đầu dự án",
			"create new project",
			"start project",
			"initialize project",
			"new project",
			"tạo dự án cho Minh",
			"create project assign to John",
			"tạo dự án phân công cho Nguyễn Văn An",
			"create marketing project assign to Mary",
			"tạo dự án cho Minh, Nam và An",
			"create project assign to John, Mary and Peter",
			"tạo dự án phân công cho Nguyễn Văn An, Trần Thị Hoa",
		},
		"create_task": {
			"tạo task mới",
			"tạo công việc",
			"giao việc",
			"thêm task",
			"tạo nhiệm vụ mới",
			"create new task",
			"add task",
			"assign work",
			"new task",
			"create job",
			"tạo task cho Nam",
			"create task assign to Mary",
			"tạo task thiết kế giao diện cho developer",
			"create API development task assign to backend team",
			"tạo task cho Nam, An và Hoa",
			"create task assign to John, Mary and Peter",
			"tạo task dọn dẹp cho John, Tex và Lady",
			"create cleaning task assign to John, Tex, Lady",
		},
	}
}

// handleModifyAction handles user modification requests with multi-employee support
func (m *ProjectManagementModule) handleModifyAction(ctx *erp_modules.ModuleContext, pending *ProjectManagementConfirmation, modifications map[string]interface{}) (*erp_modules.ModuleResponse, error) {
	// Check if the modification involves assignee changes that might need disambiguation
	if assigneeNamesInterface, ok := modifications["assigned_to_names"].([]interface{}); ok {
		var assigneeNames []string
		for _, name := range assigneeNamesInterface {
			if nameStr, ok := name.(string); ok {
				assigneeNames = append(assigneeNames, nameStr)
			}
		}

		if len(assigneeNames) > 0 {
			// Try to resolve the assignees
			assigneeResult, err := m.resolveMultipleAssignees(assigneeNames)
			if err != nil {
				m.api.LogError("Failed to resolve modified assignees", "error", err.Error())
				modifications["assigned_to_employees"] = []AssignedEmployee{}
			} else if assigneeResult.RequiresDisambiguation {
				// Store modification disambiguation state
				multiDisambiguation := &MultiEmployeeDisambiguationConfirmation{
					UserID:                    ctx.User.Id,
					Type:                      pending.Type,
					Action:                    pending.Action,
					Data:                      pending.Data,
					CreatedAt:                 time.Now().UnixMilli(),
					EmployeeID:                pending.EmployeeID,
					CreatorEmail:              pending.CreatorEmail,
					UnresolvedEmployeeMatches: assigneeResult.UnresolvedEmployeeMatches,
					ResolvedEmployees:         assigneeResult.ResolvedEmployees,
					IsModification:            true,
					OriginalConfirmation:      pending,
					PendingModifications:      modifications,
				}

				m.confirmationManager.StorePendingMultiEmployeeDisambiguation(ctx.User.Id, multiDisambiguation)

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
					ActionTaken: "request_modification_multi_employee_disambiguation",
					Data: map[string]interface{}{
						"awaiting_disambiguation": true,
						"type":                    "modification",
						"unresolved_count":        len(assigneeResult.UnresolvedEmployeeMatches),
						"resolved_count":          len(assigneeResult.ResolvedEmployees),
					},
				}, nil
			} else {
				// All employees resolved successfully
				modifications["assigned_to_employees"] = assigneeResult.ResolvedEmployees
			}
		} else {
			// Empty assignment
			modifications["assigned_to_employees"] = []AssignedEmployee{}
		}
	} else {
		// Handle other assignee modifications using existing logic
		m.handleAssigneesModification(modifications)
	}

	// Update the pending data with modifications
	for key, value := range modifications {
		pending.Data[key] = value
	}

	// Update the stored confirmation
	m.confirmationManager.StorePendingConfirmation(ctx.User.Id, pending)

	// Generate new confirmation message with updated data
	confirmationMsg, err := m.generateConfirmationMessage(ctx, pending.Type, pending.Data)
	if err != nil {
		isVietnamese := detectUserLanguage(ctx.User)
		errorMsg := "⚠️ Có lỗi xảy ra khi tạo tin nhắn xác nhận cập nhật."
		if !isVietnamese {
			errorMsg = "⚠️ An error occurred while creating updated confirmation message."
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
		ActionTaken: "update_confirmation",
		Data: map[string]interface{}{
			"awaiting_confirmation": true,
			"type":                  pending.Type,
			"updated":               true,
		},
	}, nil
}

// handleCancelAction handles user cancellation
func (m *ProjectManagementModule) handleCancelAction(userID string) (*erp_modules.ModuleResponse, error) {
	return &erp_modules.ModuleResponse{
		Success:     true,
		Message:     "Đã hủy bỏ yêu cầu tạo dự án/task.",
		ActionTaken: "cancel_confirmation",
	}, nil
}
