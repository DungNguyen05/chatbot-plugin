// server/erp_modules/project_management/response_handlers.go

package project_management

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/mattermost/mattermost-plugin-ai/server/erp_modules"
)

// ProcessingStateResponseHandler handles responses during multi-step processing
type ProcessingStateResponseHandler struct {
	module    *ProjectManagementModule
	processor *MultiStepProcessor
}

// NewProcessingStateResponseHandler creates a new response handler
func NewProcessingStateResponseHandler(module *ProjectManagementModule, processor *MultiStepProcessor) *ProcessingStateResponseHandler {
	return &ProcessingStateResponseHandler{
		module:    module,
		processor: processor,
	}
}

// HandleProcessingStateResponse handles user responses during processing workflow
func (h *ProcessingStateResponseHandler) HandleProcessingStateResponse(ctx *erp_modules.ModuleContext, message string, state *ProcessingState) (*erp_modules.ModuleResponse, error) {
	h.module.api.LogInfo("Handling processing state response",
		"user_id", ctx.User.Id,
		"step", state.Step,
		"message", message)

	switch state.Step {
	case StepAnalyzeExistingTask, StepAnalyzeExistingProject:
		return h.handleExistingEntityResponse(ctx, message, state)
	case StepResolveEmployees:
		return h.handleEmployeeResolutionResponse(ctx, message, state)
	case StepResolveProject:
		return h.handleProjectSelectionResponse(ctx, message, state)
	case StepFinalConfirmation:
		return h.handleFinalConfirmationResponse(ctx, message, state)
	default:
		return nil, fmt.Errorf("cannot handle response for step: %s", state.Step)
	}
}

// handleExistingEntityResponse handles responses to existing entity disambiguation
func (h *ProcessingStateResponseHandler) handleExistingEntityResponse(ctx *erp_modules.ModuleContext, message string, state *ProcessingState) (*erp_modules.ModuleResponse, error) {
	// Parse response using existing method
	disambiguationResponse, err := h.module.parseExistingEntityDisambiguationResponse(ctx, message, &ExistingEntityDisambiguationConfirmation{
		EntityType:       state.Type,
		ExistingEntities: state.PendingExistingEntities,
	})
	if err != nil {
		h.module.api.LogError("Failed to parse existing entity response", "error", err.Error())
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

	switch disambiguationResponse.Intent {
	case "create_new":
		// User wants to create new, continue with workflow
		if state.Step == StepAnalyzeExistingTask {
			state.CompletedSteps[StepAnalyzeExistingTask] = true
		} else {
			state.CompletedSteps[StepAnalyzeExistingProject] = true
		}

		// Clear existing entities since we're creating new
		state.PendingExistingEntities = nil
		state.SelectedExistingEntity = nil

		// Store updated state
		h.module.confirmationManager.StoreProcessingState(ctx.User.Id, state)

		// Move to next step (resolve employees or final confirmation)
		return h.processor.moveToNextStep(ctx, state, StepResolveEmployees)

	case "use_existing":
		// User wants to use existing entity - handle assignment
		return h.handleUseExistingEntity(ctx, state, disambiguationResponse.SelectedIndex)

	case "cancel":
		return h.handleCancelAction(ctx.User.Id)

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

// handleEmployeeResolutionResponse handles responses to employee disambiguation
func (h *ProcessingStateResponseHandler) handleEmployeeResolutionResponse(ctx *erp_modules.ModuleContext, message string, state *ProcessingState) (*erp_modules.ModuleResponse, error) {
	// Parse response using existing method
	disambiguationResponse, err := h.module.parseMultiEmployeeDisambiguationResponse(ctx, message, &MultiEmployeeDisambiguationConfirmation{
		UnresolvedEmployeeMatches: state.PendingEmployeeResolution.UnresolvedEmployeeMatches,
	})
	if err != nil {
		h.module.api.LogError("Failed to parse employee disambiguation response", "error", err.Error())
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

	switch disambiguationResponse.Intent {
	case "index_selection":
		// Resolve selected employees
		selectedEmployees, err := h.module.resolveDisambiguatedEmployees(
			state.PendingEmployeeResolution.UnresolvedEmployeeMatches,
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
		state.ResolvedEmployees = append(state.ResolvedEmployees, selectedEmployees...)
		state.CompletedSteps[StepResolveEmployees] = true

		// Clear pending employee resolution
		state.PendingEmployeeResolution = nil

		// Check if we have selected an existing entity for assignment
		if state.SelectedExistingEntity != nil {
			// We're assigning to existing entity, perform assignment now
			h.module.confirmationManager.StoreProcessingState(ctx.User.Id, state)
			return h.performExistingEntityAssignment(ctx, state)
		}

		// Store updated state and move to next step
		h.module.confirmationManager.StoreProcessingState(ctx.User.Id, state)
		return h.processor.moveToNextStep(ctx, state, h.processor.getNextStepAfterEmployees(state))

	case "cancel":
		return h.handleCancelAction(ctx.User.Id)

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

// handleProjectSelectionResponse handles responses to project selection
func (h *ProcessingStateResponseHandler) handleProjectSelectionResponse(ctx *erp_modules.ModuleContext, message string, state *ProcessingState) (*erp_modules.ModuleResponse, error) {
	// Parse response using existing method
	selectionResponse, err := h.module.parseProjectSelectionResponse(ctx, message, &ProjectTaskDisambiguationConfirmation{
		MatchingProjects: state.PendingProjectSelection,
	})
	if err != nil {
		h.module.api.LogError("Failed to parse project selection response", "error", err.Error())
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

	switch selectionResponse.Intent {
	case "select_project":
		// Validate index
		if selectionResponse.SelectedIndex < 1 || selectionResponse.SelectedIndex > len(state.PendingProjectSelection) {
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
		selectedProject := state.PendingProjectSelection[selectionResponse.SelectedIndex-1]
		state.SelectedProject = selectedProject.Name
		state.OriginalRequest["project"] = selectedProject.Name
		state.CompletedSteps[StepResolveProject] = true

		// Clear pending project selection
		state.PendingProjectSelection = nil

		// Store updated state and move to final confirmation
		h.module.confirmationManager.StoreProcessingState(ctx.User.Id, state)
		return h.processor.moveToNextStep(ctx, state, StepFinalConfirmation)

	case "no_project":
		// Clear project and proceed
		state.OriginalRequest["project"] = ""
		state.CompletedSteps[StepResolveProject] = true

		// Clear pending project selection
		state.PendingProjectSelection = nil

		// Store updated state and move to final confirmation
		h.module.confirmationManager.StoreProcessingState(ctx.User.Id, state)
		return h.processor.moveToNextStep(ctx, state, StepFinalConfirmation)

	case "cancel":
		return h.handleCancelAction(ctx.User.Id)

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

// handleFinalConfirmationResponse handles responses to final confirmation
func (h *ProcessingStateResponseHandler) handleFinalConfirmationResponse(ctx *erp_modules.ModuleContext, message string, state *ProcessingState) (*erp_modules.ModuleResponse, error) {
	// Parse confirmation response using existing method
	userResponse, err := h.module.parseConfirmationResponse(ctx, message, &ProjectManagementConfirmation{
		Type: state.Type,
		Data: state.OriginalRequest,
	})
	if err != nil {
		h.module.api.LogError("Failed to parse final confirmation response", "error", err.Error())
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

	switch userResponse.Intent {
	case "confirm":
		// Execute final creation
		state.Step = StepComplete
		return h.processor.handleCompletion(ctx, state)

	case "modify":
		// Handle modifications and restart final confirmation
		return h.handleModifications(ctx, state, userResponse.Modifications)

	case "cancel":
		return h.handleCancelAction(ctx.User.Id)

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

// handleUseExistingEntity handles assignment to existing entity
func (h *ProcessingStateResponseHandler) handleUseExistingEntity(ctx *erp_modules.ModuleContext, state *ProcessingState, selectedIndex int) (*erp_modules.ModuleResponse, error) {
	// Validate index
	if selectedIndex < 1 || selectedIndex > len(state.PendingExistingEntities) {
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

	// Get selected entity and store it
	selectedEntity := state.PendingExistingEntities[selectedIndex-1]
	state.SelectedExistingEntity = make(map[string]interface{})
	entityBytes, _ := json.Marshal(selectedEntity)
	json.Unmarshal(entityBytes, &state.SelectedExistingEntity)

	// Mark the existing entity step as completed
	if state.Step == StepAnalyzeExistingTask {
		state.CompletedSteps[StepAnalyzeExistingTask] = true
	} else if state.Step == StepAnalyzeExistingProject {
		state.CompletedSteps[StepAnalyzeExistingProject] = true
	}

	// Clear pending existing entities since we've selected one
	state.PendingExistingEntities = nil

	// Check if we need to resolve employees from original request
	assigneeNamesInterface, hasAssignees := state.OriginalRequest["assigned_to_names"]
	if hasAssignees && len(state.ResolvedEmployees) == 0 {
		// Convert to string slice
		var assigneeNames []string
		if names, ok := assigneeNamesInterface.([]interface{}); ok {
			for _, name := range names {
				if nameStr, ok := name.(string); ok {
					assigneeNames = append(assigneeNames, nameStr)
				}
			}
		}

		if len(assigneeNames) > 0 {
			// Need to resolve employees for the existing entity
			state.Step = StepResolveEmployees
			h.module.confirmationManager.StoreProcessingState(ctx.User.Id, state)
			return h.processor.handleResolveEmployees(ctx, state)
		}
	}

	// If no employees to resolve or already resolved, proceed with assignment
	if len(state.ResolvedEmployees) == 0 {
		// No employees specified - just select existing entity without assignment
		isVietnamese := detectUserLanguage(ctx.User)
		var entityName string
		if state.Type == "project" {
			if project, ok := state.SelectedExistingEntity["project_name"]; ok {
				entityName = fmt.Sprintf("%v", project)
			} else if name, ok := state.SelectedExistingEntity["name"]; ok {
				entityName = fmt.Sprintf("%v", name)
			}
		} else {
			if subject, ok := state.SelectedExistingEntity["subject"]; ok {
				entityName = fmt.Sprintf("%v", subject)
			} else if name, ok := state.SelectedExistingEntity["name"]; ok {
				entityName = fmt.Sprintf("%v", name)
			}
		}

		var successMsg string
		if isVietnamese {
			if state.Type == "project" {
				successMsg = fmt.Sprintf("Đã chọn dự án hiện có: **%s**. Không có nhân viên nào được chỉ định để phân công.", entityName)
			} else {
				successMsg = fmt.Sprintf("Đã chọn task hiện có: **%s**. Không có nhân viên nào được chỉ định để phân công.", entityName)
			}
		} else {
			if state.Type == "project" {
				successMsg = fmt.Sprintf("Selected existing project: **%s**. No employees were specified for assignment.", entityName)
			} else {
				successMsg = fmt.Sprintf("Selected existing task: **%s**. No employees were specified for assignment.", entityName)
			}
		}

		// Clear processing state since we're done
		h.module.confirmationManager.ClearProcessingState(ctx.User.Id)

		return &erp_modules.ModuleResponse{
			Success:     true,
			Message:     successMsg,
			ActionTaken: "use_existing_without_assignment",
			Data: map[string]interface{}{
				"entity_type": state.Type,
				"entity_name": entityName,
			},
		}, nil
	}

	// Have employees to assign - proceed with assignment to existing entity
	return h.performExistingEntityAssignment(ctx, state)
}

// performExistingEntityAssignment performs the actual assignment to existing entity
func (h *ProcessingStateResponseHandler) performExistingEntityAssignment(ctx *erp_modules.ModuleContext, state *ProcessingState) (*erp_modules.ModuleResponse, error) {
	// Get entity ID and name
	var entityID, entityName string
	if state.Type == "project" {
		if id, ok := state.SelectedExistingEntity["name"]; ok {
			entityID = fmt.Sprintf("%v", id)
		}
		if name, ok := state.SelectedExistingEntity["project_name"]; ok {
			entityName = fmt.Sprintf("%v", name)
		}
	} else {
		if id, ok := state.SelectedExistingEntity["name"]; ok {
			entityID = fmt.Sprintf("%v", id)
		}
		if subject, ok := state.SelectedExistingEntity["subject"]; ok {
			entityName = fmt.Sprintf("%v", subject)
		}
	}

	// Assign to all employees
	var assignmentErrors []string
	priority := "Medium" // Default priority
	if originalPriority, ok := state.OriginalRequest["priority"]; ok {
		if priorityStr, ok := originalPriority.(string); ok && priorityStr != "" {
			priority = priorityStr
		}
	}

	for _, assignedEmployee := range state.ResolvedEmployees {
		var err error
		if state.Type == "project" {
			err = h.module.erpClient.AssignProjectToEmployee(entityID, state.CreatorEmail, assignedEmployee.Email, priority)
		} else {
			err = h.module.erpClient.AssignTaskToEmployee(entityID, state.CreatorEmail, assignedEmployee.Email, priority)
		}

		if err != nil {
			h.module.api.LogWarn("Assignment to existing entity failed",
				"entity_type", state.Type,
				"entity_id", entityID,
				"assignee", assignedEmployee.EmployeeName,
				"error", err.Error())
			assignmentErrors = append(assignmentErrors, assignedEmployee.EmployeeName)
		} else {
			h.module.api.LogInfo("Assignment to existing entity successful",
				"entity_type", state.Type,
				"entity_id", entityID,
				"assignee", assignedEmployee.EmployeeName)
		}
	}

	isVietnamese := detectUserLanguage(ctx.User)

	// Generate success message
	assigneeNames := make([]string, len(state.ResolvedEmployees))
	for i, emp := range state.ResolvedEmployees {
		assigneeNames[i] = emp.EmployeeName
	}
	assigneesStr := strings.Join(assigneeNames, ", ")

	var successMsg string
	if len(assignmentErrors) == 0 {
		if isVietnamese {
			if state.Type == "project" {
				successMsg = fmt.Sprintf(
					"**Phân công dự án thành công!**\n\n"+
						"- Dự án: `%s`\n"+
						"- Nhân viên: **%s**",
					entityName,
					assigneesStr)
			} else {
				successMsg = fmt.Sprintf(
					"**Phân công task thành công!**\n\n"+
						"- Task: `%s`\n"+
						"- Nhân viên: **%s**",
					entityName,
					assigneesStr)
			}

		} else {
			if state.Type == "project" {
				successMsg = fmt.Sprintf("Successfully assigned existing project **%s** to **%s**!", entityName, assigneesStr)
			} else {
				successMsg = fmt.Sprintf("Successfully assigned existing task **%s** to **%s**!", entityName, assigneesStr)
			}
		}
	} else {
		if isVietnamese {
			if state.Type == "project" {
				successMsg = fmt.Sprintf("Đã phân công dự án hiện có **%s**. Thành công: **%s**. Lỗi: **%s**.",
					entityName, assigneesStr, strings.Join(assignmentErrors, ", "))
			} else {
				successMsg = fmt.Sprintf("Đã phân công task hiện có **%s**. Thành công: **%s**. Lỗi: **%s**.",
					entityName, assigneesStr, strings.Join(assignmentErrors, ", "))
			}
		} else {
			if state.Type == "project" {
				successMsg = fmt.Sprintf("Assigned existing project **%s**. Successful: **%s**. Failed: **%s**.",
					entityName, assigneesStr, strings.Join(assignmentErrors, ", "))
			} else {
				successMsg = fmt.Sprintf("Assigned existing task **%s**. Successful: **%s**. Failed: **%s**.",
					entityName, assigneesStr, strings.Join(assignmentErrors, ", "))
			}
		}
	}

	// Clear processing state since we're done
	h.module.confirmationManager.ClearProcessingState(ctx.User.Id)

	return &erp_modules.ModuleResponse{
		Success:     true,
		Message:     successMsg,
		ActionTaken: "assign_existing_entity",
		Data: map[string]interface{}{
			"entity_type":        state.Type,
			"entity_id":          entityID,
			"entity_name":        entityName,
			"assigned_employees": state.ResolvedEmployees,
			"assignment_errors":  assignmentErrors,
		},
	}, nil
}

// handleModifications handles modifications during final confirmation
func (h *ProcessingStateResponseHandler) handleModifications(ctx *erp_modules.ModuleContext, state *ProcessingState, modifications map[string]interface{}) (*erp_modules.ModuleResponse, error) {
	// Apply modifications to original request
	for key, value := range modifications {
		state.OriginalRequest[key] = value
	}

	// Handle assignee modifications specifically
	if assigneeNamesInterface, ok := modifications["assigned_to_names"]; ok {
		var assigneeNames []string
		if names, ok := assigneeNamesInterface.([]interface{}); ok {
			for _, name := range names {
				if nameStr, ok := name.(string); ok {
					assigneeNames = append(assigneeNames, nameStr)
				}
			}
		}

		if len(assigneeNames) > 0 {
			// Re-resolve employees
			assigneeResult, err := h.module.resolveMultipleAssignees(assigneeNames)
			if err != nil {
				h.module.api.LogError("Failed to resolve modified assignees", "error", err.Error())
				state.ResolvedEmployees = []AssignedEmployee{}
			} else if assigneeResult.RequiresDisambiguation {
				// Need to go back to employee resolution step
				state.Step = StepResolveEmployees
				state.PendingEmployeeResolution = assigneeResult
				state.ResolvedEmployees = assigneeResult.ResolvedEmployees
				h.module.confirmationManager.StoreProcessingState(ctx.User.Id, state)
				return h.processor.requestEmployeeDisambiguation(ctx, state, assigneeResult)
			} else {
				state.ResolvedEmployees = assigneeResult.ResolvedEmployees
			}
		} else {
			state.ResolvedEmployees = []AssignedEmployee{}
		}
	}

	// Update assigned_to_employees with resolved employees
	state.OriginalRequest["assigned_to_employees"] = state.ResolvedEmployees

	// Store updated state
	h.module.confirmationManager.StoreProcessingState(ctx.User.Id, state)

	// Generate new confirmation message
	return h.processor.handleFinalConfirmation(ctx, state)
}

// handleCancelAction handles user cancellation
func (h *ProcessingStateResponseHandler) handleCancelAction(userID string) (*erp_modules.ModuleResponse, error) {
	h.module.confirmationManager.ClearPendingConfirmation(userID)
	h.module.confirmationManager.ClearProcessingState(userID)
	return &erp_modules.ModuleResponse{
		Success:     true,
		Message:     "Đã hủy bỏ yêu cầu tạo dự án/task.",
		ActionTaken: "cancel_confirmation",
	}, nil
}
