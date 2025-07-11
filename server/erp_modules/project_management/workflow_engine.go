// Copyright (c) 2023-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package project_management

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/mattermost/mattermost-plugin-ai/server/erp_modules"
)

// WorkflowStepType represents different types of workflow steps
type WorkflowStepType string

const (
	StepTypeEmployeeDisambiguation  WorkflowStepType = "employee_disambiguation"
	StepTypeProjectSelection        WorkflowStepType = "project_selection"
	StepTypeExistingEntitySelection WorkflowStepType = "existing_entity_selection"
	StepTypeConfirmation            WorkflowStepType = "confirmation"
)

// WorkflowStep represents a single step in the workflow
type WorkflowStep struct {
	Type         WorkflowStepType       `json:"type"`
	ID           string                 `json:"id"`
	Title        string                 `json:"title"`
	Required     bool                   `json:"required"`
	Completed    bool                   `json:"completed"`
	Data         map[string]interface{} `json:"data"`
	Dependencies []string               `json:"dependencies"` // IDs of steps that must complete first
}

// WorkflowState represents the complete state of a workflow
type WorkflowState struct {
	UserID           string                 `json:"user_id"`
	WorkflowType     string                 `json:"workflow_type"` // "create_project", "create_task"
	EntityType       string                 `json:"entity_type"`   // "project", "task"
	OriginalRequest  map[string]interface{} `json:"original_request"`
	Steps            []WorkflowStep         `json:"steps"`
	CurrentStepIndex int                    `json:"current_step_index"`
	CreatedAt        int64                  `json:"created_at"`
	LastModified     int64                  `json:"last_modified"`
	EmployeeID       string                 `json:"employee_id"`
	CreatorEmail     string                 `json:"creator_email"`
	CompletedData    map[string]interface{} `json:"completed_data"` // Final resolved data
}

// WorkflowEngine manages the entire workflow process
type WorkflowEngine struct {
	module          *ProjectManagementModule
	activeWorkflows map[string]*WorkflowState
}

// NewWorkflowEngine creates a new workflow engine
func NewWorkflowEngine(module *ProjectManagementModule) *WorkflowEngine {
	return &WorkflowEngine{
		module:          module,
		activeWorkflows: make(map[string]*WorkflowState),
	}
}

// StartWorkflow starts a new workflow for project/task creation
func (we *WorkflowEngine) StartWorkflow(
	ctx *erp_modules.ModuleContext,
	workflowType, entityType string,
	originalRequest map[string]interface{},
	employeeID, creatorEmail string,
) (*erp_modules.ModuleResponse, error) {

	// Create workflow state
	workflow := &WorkflowState{
		UserID:           ctx.User.Id,
		WorkflowType:     workflowType,
		EntityType:       entityType,
		OriginalRequest:  originalRequest,
		Steps:            []WorkflowStep{},
		CurrentStepIndex: 0,
		CreatedAt:        time.Now().UnixMilli(),
		LastModified:     time.Now().UnixMilli(),
		EmployeeID:       employeeID,
		CreatorEmail:     creatorEmail,
		CompletedData:    make(map[string]interface{}),
	}

	// Copy original request to completed data
	for k, v := range originalRequest {
		workflow.CompletedData[k] = v
	}

	// Analyze and build workflow steps
	if err := we.buildWorkflowSteps(workflow); err != nil {
		return nil, fmt.Errorf("failed to build workflow steps: %w", err)
	}

	// Store workflow
	we.activeWorkflows[ctx.User.Id] = workflow

	// Execute first step
	return we.executeCurrentStep(ctx, workflow)
}

// ProcessWorkflowMessage processes user messages in an active workflow
func (we *WorkflowEngine) ProcessWorkflowMessage(
	ctx *erp_modules.ModuleContext,
	message string,
) (*erp_modules.ModuleResponse, error) {

	workflow, exists := we.activeWorkflows[ctx.User.Id]
	if !exists {
		return nil, nil // No active workflow
	}

	// Handle special commands
	if we.handleSpecialCommands(ctx, workflow, message) {
		return we.executeCurrentStep(ctx, workflow)
	}

	// Check if this is a modification request
	if modificationResponse := we.handleModification(ctx, workflow, message); modificationResponse != nil {
		return modificationResponse, nil
	}

	// Process current step
	return we.processCurrentStep(ctx, workflow, message)
}

// handleSpecialCommands handles special workflow commands
func (we *WorkflowEngine) handleSpecialCommands(
	ctx *erp_modules.ModuleContext,
	workflow *WorkflowState,
	message string,
) bool {
	messageLower := strings.ToLower(strings.TrimSpace(message))

	switch messageLower {
	case "cancel", "hủy", "stop", "dừng":
		we.cancelWorkflow(ctx.User.Id)
		return true
	case "restart", "bắt đầu lại", "làm lại":
		workflow.CurrentStepIndex = 0
		we.resetIncompleteSteps(workflow)
		return true
	case "skip", "bỏ qua", "next":
		if we.canSkipCurrentStep(workflow) {
			we.markCurrentStepCompleted(workflow)
			we.moveToNextStep(workflow)
			return true
		}
	}

	return false
}

// handleModification handles modification requests during workflow
func (we *WorkflowEngine) handleModification(
	ctx *erp_modules.ModuleContext,
	workflow *WorkflowState,
	message string,
) *erp_modules.ModuleResponse {

	// Parse modification using LLM
	modifications, err := we.parseModificationRequest(ctx, workflow, message)
	if err != nil || len(modifications) == 0 {
		return nil // Not a modification request
	}

	// Apply modifications to completed data
	for key, value := range modifications {
		workflow.CompletedData[key] = value
	}

	// Rebuild workflow steps based on new data
	if err := we.buildWorkflowSteps(workflow); err != nil {
		we.module.api.LogError("Failed to rebuild workflow steps", "error", err.Error())
		return nil
	}

	// Find first incomplete step
	workflow.CurrentStepIndex = we.findFirstIncompleteStep(workflow)
	workflow.LastModified = time.Now().UnixMilli()

	// Execute current step
	response, err := we.executeCurrentStep(ctx, workflow)
	if err != nil {
		we.module.api.LogError("Failed to execute step after modification", "error", err.Error())
		return nil
	}

	return response
}

// buildWorkflowSteps dynamically builds workflow steps based on current data
func (we *WorkflowEngine) buildWorkflowSteps(workflow *WorkflowState) error {
	workflow.Steps = []WorkflowStep{}

	// Step 1: Check for existing entities (projects/tasks with similar names)
	if we.needsExistingEntityCheck(workflow) {
		workflow.Steps = append(workflow.Steps, WorkflowStep{
			Type:      StepTypeExistingEntitySelection,
			ID:        "existing_entity_check",
			Title:     "Check Existing Entities",
			Required:  true,
			Completed: false,
			Data:      map[string]interface{}{},
		})
	}

	// Step 2: Employee disambiguation (if needed)
	if we.needsEmployeeDisambiguation(workflow) {
		workflow.Steps = append(workflow.Steps, WorkflowStep{
			Type:      StepTypeEmployeeDisambiguation,
			ID:        "employee_disambiguation",
			Title:     "Select Employees",
			Required:  true,
			Completed: false,
			Data:      map[string]interface{}{},
		})
	}

	// Step 3: Project selection for tasks (if needed)
	if we.needsProjectSelection(workflow) {
		workflow.Steps = append(workflow.Steps, WorkflowStep{
			Type:         StepTypeProjectSelection,
			ID:           "project_selection",
			Title:        "Select Project",
			Required:     false,
			Completed:    false,
			Data:         map[string]interface{}{},
			Dependencies: []string{"employee_disambiguation"}, // After employees are resolved
		})
	}

	// Step 4: Final confirmation
	workflow.Steps = append(workflow.Steps, WorkflowStep{
		Type:         StepTypeConfirmation,
		ID:           "final_confirmation",
		Title:        "Confirm Creation",
		Required:     true,
		Completed:    false,
		Data:         map[string]interface{}{},
		Dependencies: we.getAllPreviousStepIDs(workflow),
	})

	return nil
}

// needsExistingEntityCheck checks if we need to check for existing entities
func (we *WorkflowEngine) needsExistingEntityCheck(workflow *WorkflowState) bool {
	// Check if we already completed this step
	for _, step := range workflow.Steps {
		if step.ID == "existing_entity_check" && step.Completed {
			return false
		}
	}

	// Check if we have a name to search for
	if workflow.EntityType == "project" {
		if projectName, ok := workflow.CompletedData["project_name"].(string); ok && projectName != "" {
			return true
		}
	} else if workflow.EntityType == "task" {
		if subject, ok := workflow.CompletedData["subject"].(string); ok && subject != "" {
			return true
		}
	}

	return false
}

// needsEmployeeDisambiguation checks if employee disambiguation is needed
func (we *WorkflowEngine) needsEmployeeDisambiguation(workflow *WorkflowState) bool {
	// Check if we already completed this step
	for _, step := range workflow.Steps {
		if step.ID == "employee_disambiguation" && step.Completed {
			return false
		}
	}

	// Check if we have unresolved employee names
	if assigneeNames, ok := workflow.CompletedData["assigned_to_names"].([]interface{}); ok && len(assigneeNames) > 0 {
		// Check if we already have resolved employees
		if assigneeEmployees, ok := workflow.CompletedData["assigned_to_employees"].([]interface{}); ok && len(assigneeEmployees) > 0 {
			return false // Already resolved
		}
		return true
	}

	return false
}

// needsProjectSelection checks if project selection is needed for tasks
func (we *WorkflowEngine) needsProjectSelection(workflow *WorkflowState) bool {
	// Only for tasks
	if workflow.EntityType != "task" {
		return false
	}

	// Check if we already completed this step
	for _, step := range workflow.Steps {
		if step.ID == "project_selection" && step.Completed {
			return false
		}
	}

	// Check if we have a project mentioned but not resolved
	if project, ok := workflow.CompletedData["project"].(string); ok && project != "" {
		// Check if it's already a resolved project ID
		projects, err := we.module.erpClient.SearchProjectsByName(project)
		if err == nil && len(projects) > 1 {
			return true // Multiple matches, need selection
		}
	}

	return false
}

// executeCurrentStep executes the current workflow step
func (we *WorkflowEngine) executeCurrentStep(
	ctx *erp_modules.ModuleContext,
	workflow *WorkflowState,
) (*erp_modules.ModuleResponse, error) {

	if workflow.CurrentStepIndex >= len(workflow.Steps) {
		// All steps completed, execute final action
		return we.executeFinalAction(ctx, workflow)
	}

	currentStep := &workflow.Steps[workflow.CurrentStepIndex]

	// Check dependencies
	if !we.areDependenciesMet(workflow, currentStep) {
		// Find next step with met dependencies
		workflow.CurrentStepIndex = we.findNextAvailableStep(workflow)
		if workflow.CurrentStepIndex >= len(workflow.Steps) {
			return we.executeFinalAction(ctx, workflow)
		}
		currentStep = &workflow.Steps[workflow.CurrentStepIndex]
	}

	// Execute step based on type
	switch currentStep.Type {
	case StepTypeExistingEntitySelection:
		return we.executeExistingEntityStep(ctx, workflow, currentStep)
	case StepTypeEmployeeDisambiguation:
		return we.executeEmployeeDisambiguationStep(ctx, workflow, currentStep)
	case StepTypeProjectSelection:
		return we.executeProjectSelectionStep(ctx, workflow, currentStep)
	case StepTypeConfirmation:
		return we.executeConfirmationStep(ctx, workflow, currentStep)
	}

	return nil, fmt.Errorf("unknown step type: %s", currentStep.Type)
}

// executeExistingEntityStep executes existing entity selection step
func (we *WorkflowEngine) executeExistingEntityStep(
	ctx *erp_modules.ModuleContext,
	workflow *WorkflowState,
	step *WorkflowStep,
) (*erp_modules.ModuleResponse, error) {

	var existingEntities []interface{}
	var entityName string

	if workflow.EntityType == "project" {
		entityName = workflow.CompletedData["project_name"].(string)
		projects, err := we.module.erpClient.SearchProjectsByName(entityName)
		if err != nil {
			we.module.api.LogError("Failed to search projects", "error", err.Error())
			// Continue without checking
			we.markStepCompleted(workflow, step)
			return we.moveToNextStepAndExecute(ctx, workflow)
		}

		var highConfidenceProjects []Project
		for _, project := range projects {
			if project.MatchConfidence >= 0.85 {
				highConfidenceProjects = append(highConfidenceProjects, project)
			}
		}

		if len(highConfidenceProjects) == 0 {
			// No existing entities found
			we.markStepCompleted(workflow, step)
			return we.moveToNextStepAndExecute(ctx, workflow)
		}

		for _, p := range highConfidenceProjects {
			existingEntities = append(existingEntities, p)
		}
	} else {
		entityName = workflow.CompletedData["subject"].(string)
		tasks, err := we.module.erpClient.SearchTasksByName(entityName)
		if err != nil {
			we.module.api.LogError("Failed to search tasks", "error", err.Error())
			// Continue without checking
			we.markStepCompleted(workflow, step)
			return we.moveToNextStepAndExecute(ctx, workflow)
		}

		var highConfidenceTasks []Task
		for _, task := range tasks {
			if task.MatchConfidence >= 0.85 {
				highConfidenceTasks = append(highConfidenceTasks, task)
			}
		}

		if len(highConfidenceTasks) == 0 {
			// No existing entities found
			we.markStepCompleted(workflow, step)
			return we.moveToNextStepAndExecute(ctx, workflow)
		}

		for _, t := range highConfidenceTasks {
			existingEntities = append(existingEntities, t)
		}
	}

	// Store existing entities in step data
	step.Data["existing_entities"] = existingEntities
	step.Data["entity_name"] = entityName

	// Generate disambiguation message
	message, err := we.module.generateExistingEntityDisambiguationMessage(ctx, workflow.EntityType, existingEntities)
	if err != nil {
		return nil, fmt.Errorf("failed to generate existing entity message: %w", err)
	}

	return &erp_modules.ModuleResponse{
		Success:     true,
		Message:     message,
		ActionTaken: "workflow_existing_entity_disambiguation",
		Data: map[string]interface{}{
			"workflow_active": true,
			"step_type":       string(step.Type),
			"step_id":         step.ID,
			"entity_count":    len(existingEntities),
		},
	}, nil
}

// executeEmployeeDisambiguationStep executes employee disambiguation step
func (we *WorkflowEngine) executeEmployeeDisambiguationStep(
	ctx *erp_modules.ModuleContext,
	workflow *WorkflowState,
	step *WorkflowStep,
) (*erp_modules.ModuleResponse, error) {

	assigneeNames, ok := workflow.CompletedData["assigned_to_names"].([]interface{})
	if !ok || len(assigneeNames) == 0 {
		// No employees to resolve
		we.markStepCompleted(workflow, step)
		return we.moveToNextStepAndExecute(ctx, workflow)
	}

	// Convert to string slice
	var nameStrings []string
	for _, name := range assigneeNames {
		if nameStr, ok := name.(string); ok {
			nameStrings = append(nameStrings, nameStr)
		}
	}

	// Resolve employees
	assigneeResult, err := we.module.resolveMultipleAssignees(nameStrings)
	if err != nil {
		we.module.api.LogError("Failed to resolve employees", "error", err.Error())
		// Continue without assignment
		workflow.CompletedData["assigned_to_employees"] = []AssignedEmployee{}
		we.markStepCompleted(workflow, step)
		return we.moveToNextStepAndExecute(ctx, workflow)
	}

	if !assigneeResult.RequiresDisambiguation {
		// All employees resolved
		workflow.CompletedData["assigned_to_employees"] = assigneeResult.ResolvedEmployees
		we.markStepCompleted(workflow, step)
		return we.moveToNextStepAndExecute(ctx, workflow)
	}

	// Store disambiguation data in step
	step.Data["unresolved_matches"] = assigneeResult.UnresolvedEmployeeMatches
	step.Data["resolved_employees"] = assigneeResult.ResolvedEmployees

	// Generate disambiguation message
	message, err := we.module.generateMultiEmployeeDisambiguationMessage(ctx, assigneeResult)
	if err != nil {
		return nil, fmt.Errorf("failed to generate employee disambiguation message: %w", err)
	}

	return &erp_modules.ModuleResponse{
		Success:     true,
		Message:     message,
		ActionTaken: "workflow_employee_disambiguation",
		Data: map[string]interface{}{
			"workflow_active":  true,
			"step_type":        string(step.Type),
			"step_id":          step.ID,
			"unresolved_count": len(assigneeResult.UnresolvedEmployeeMatches),
			"resolved_count":   len(assigneeResult.ResolvedEmployees),
		},
	}, nil
}

// executeProjectSelectionStep executes project selection step
func (we *WorkflowEngine) executeProjectSelectionStep(
	ctx *erp_modules.ModuleContext,
	workflow *WorkflowState,
	step *WorkflowStep,
) (*erp_modules.ModuleResponse, error) {

	projectName, ok := workflow.CompletedData["project"].(string)
	if !ok || projectName == "" {
		// No project specified
		we.markStepCompleted(workflow, step)
		return we.moveToNextStepAndExecute(ctx, workflow)
	}

	// Search for matching projects
	projects, err := we.module.erpClient.SearchProjectsByName(projectName)
	if err != nil {
		we.module.api.LogError("Failed to search projects", "error", err.Error())
		// Continue without project
		workflow.CompletedData["project"] = ""
		we.markStepCompleted(workflow, step)
		return we.moveToNextStepAndExecute(ctx, workflow)
	}

	// Filter high confidence matches
	var highConfidenceProjects []Project
	for _, project := range projects {
		if project.MatchConfidence >= 0.85 {
			highConfidenceProjects = append(highConfidenceProjects, project)
		}
	}

	if len(highConfidenceProjects) == 0 {
		// No matches, proceed without project
		workflow.CompletedData["project"] = ""
		we.markStepCompleted(workflow, step)
		return we.moveToNextStepAndExecute(ctx, workflow)
	}

	if len(highConfidenceProjects) == 1 {
		// Single match, use it
		workflow.CompletedData["project"] = highConfidenceProjects[0].Name
		we.markStepCompleted(workflow, step)
		return we.moveToNextStepAndExecute(ctx, workflow)
	}

	// Multiple matches, need selection
	step.Data["matching_projects"] = highConfidenceProjects

	// Generate selection message
	message, err := we.module.generateProjectTaskDisambiguationMessage(ctx, highConfidenceProjects)
	if err != nil {
		return nil, fmt.Errorf("failed to generate project selection message: %w", err)
	}

	return &erp_modules.ModuleResponse{
		Success:     true,
		Message:     message,
		ActionTaken: "workflow_project_selection",
		Data: map[string]interface{}{
			"workflow_active": true,
			"step_type":       string(step.Type),
			"step_id":         step.ID,
			"project_count":   len(highConfidenceProjects),
		},
	}, nil
}

// executeConfirmationStep executes final confirmation step
func (we *WorkflowEngine) executeConfirmationStep(
	ctx *erp_modules.ModuleContext,
	workflow *WorkflowState,
	step *WorkflowStep,
) (*erp_modules.ModuleResponse, error) {

	// Generate confirmation message
	message, err := we.module.generateConfirmationMessage(ctx, workflow.EntityType, workflow.CompletedData)
	if err != nil {
		return nil, fmt.Errorf("failed to generate confirmation message: %w", err)
	}

	return &erp_modules.ModuleResponse{
		Success:     true,
		Message:     message,
		ActionTaken: "workflow_confirmation",
		Data: map[string]interface{}{
			"workflow_active": true,
			"step_type":       string(step.Type),
			"step_id":         step.ID,
		},
	}, nil
}

// processCurrentStep processes user response for current step
func (we *WorkflowEngine) processCurrentStep(
	ctx *erp_modules.ModuleContext,
	workflow *WorkflowState,
	message string,
) (*erp_modules.ModuleResponse, error) {

	if workflow.CurrentStepIndex >= len(workflow.Steps) {
		return we.executeFinalAction(ctx, workflow)
	}

	currentStep := &workflow.Steps[workflow.CurrentStepIndex]

	switch currentStep.Type {
	case StepTypeExistingEntitySelection:
		return we.processExistingEntityResponse(ctx, workflow, currentStep, message)
	case StepTypeEmployeeDisambiguation:
		return we.processEmployeeDisambiguationResponse(ctx, workflow, currentStep, message)
	case StepTypeProjectSelection:
		return we.processProjectSelectionResponse(ctx, workflow, currentStep, message)
	case StepTypeConfirmation:
		return we.processConfirmationResponse(ctx, workflow, currentStep, message)
	}

	return nil, fmt.Errorf("unknown step type: %s", currentStep.Type)
}

// processExistingEntityResponse processes user response to existing entity selection
func (we *WorkflowEngine) processExistingEntityResponse(
	ctx *erp_modules.ModuleContext,
	workflow *WorkflowState,
	step *WorkflowStep,
	message string,
) (*erp_modules.ModuleResponse, error) {

	entities, ok := step.Data["existing_entities"].([]interface{})
	if !ok {
		return nil, fmt.Errorf("missing existing entities data")
	}

	// Parse response using LLM
	response, err := we.module.parseExistingEntityDisambiguationResponse(ctx, message, workflow.EntityType, len(entities))
	if err != nil {
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

	switch response.Intent {
	case "create_new":
		// Continue with new creation
		we.markStepCompleted(workflow, step)
		return we.moveToNextStepAndExecute(ctx, workflow)

	case "use_existing":
		// Use existing entity for assignment
		if response.SelectedIndex < 1 || response.SelectedIndex > len(entities) {
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

		// Get selected entity and assign employees to it
		selectedEntity := entities[response.SelectedIndex-1]
		return we.assignToExistingEntity(ctx, workflow, selectedEntity)

	case "cancel":
		we.cancelWorkflow(ctx.User.Id)
		isVietnamese := detectUserLanguage(ctx.User)
		cancelMsg := "✅ Đã hủy bỏ yêu cầu."
		if !isVietnamese {
			cancelMsg = "✅ Request cancelled."
		}
		return &erp_modules.ModuleResponse{
			Success:     true,
			Message:     cancelMsg,
			ActionTaken: "workflow_cancelled",
		}, nil
	}

	return nil, fmt.Errorf("unhandled intent: %s", response.Intent)
}

// processEmployeeDisambiguationResponse processes employee disambiguation response
func (we *WorkflowEngine) processEmployeeDisambiguationResponse(
	ctx *erp_modules.ModuleContext,
	workflow *WorkflowState,
	step *WorkflowStep,
	message string,
) (*erp_modules.ModuleResponse, error) {

	unresolvedMatches, ok := step.Data["unresolved_matches"].([]UnresolvedEmployeeMatch)
	if !ok {
		return nil, fmt.Errorf("missing unresolved matches data")
	}

	resolvedEmployees, ok := step.Data["resolved_employees"].([]AssignedEmployee)
	if !ok {
		resolvedEmployees = []AssignedEmployee{}
	}

	// Parse disambiguation response using LLM
	response, err := we.module.parseMultiEmployeeDisambiguationResponse(ctx, message, unresolvedMatches)
	if err != nil {
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

	switch response.Intent {
	case "index_selection":
		if len(response.SelectedIndexes) == 0 {
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

		// Resolve selected employees
		selectedEmployees, err := we.module.resolveDisambiguatedEmployees(unresolvedMatches, response.SelectedIndexes)
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
		allResolvedEmployees := append(resolvedEmployees, selectedEmployees...)
		workflow.CompletedData["assigned_to_employees"] = allResolvedEmployees

		// Mark step completed and continue
		we.markStepCompleted(workflow, step)
		return we.moveToNextStepAndExecute(ctx, workflow)

	case "cancel":
		we.cancelWorkflow(ctx.User.Id)
		isVietnamese := detectUserLanguage(ctx.User)
		cancelMsg := "✅ Đã hủy bỏ yêu cầu."
		if !isVietnamese {
			cancelMsg = "✅ Request cancelled."
		}
		return &erp_modules.ModuleResponse{
			Success:     true,
			Message:     cancelMsg,
			ActionTaken: "workflow_cancelled",
		}, nil
	}

	return nil, fmt.Errorf("unhandled intent: %s", response.Intent)
}

// processProjectSelectionResponse processes project selection response
func (we *WorkflowEngine) processProjectSelectionResponse(
	ctx *erp_modules.ModuleContext,
	workflow *WorkflowState,
	step *WorkflowStep,
	message string,
) (*erp_modules.ModuleResponse, error) {

	matchingProjects, ok := step.Data["matching_projects"].([]Project)
	if !ok {
		return nil, fmt.Errorf("missing matching projects data")
	}

	// Parse selection response using LLM
	response, err := we.module.parseProjectSelectionResponse(ctx, message, matchingProjects)
	if err != nil {
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

	switch response.Intent {
	case "select_project":
		if response.SelectedIndex < 1 || response.SelectedIndex > len(matchingProjects) {
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
		selectedProject := matchingProjects[response.SelectedIndex-1]
		workflow.CompletedData["project"] = selectedProject.Name

		// Mark step completed and continue
		we.markStepCompleted(workflow, step)
		return we.moveToNextStepAndExecute(ctx, workflow)

	case "no_project":
		// Clear project and proceed
		workflow.CompletedData["project"] = ""
		we.markStepCompleted(workflow, step)
		return we.moveToNextStepAndExecute(ctx, workflow)

	case "cancel":
		we.cancelWorkflow(ctx.User.Id)
		isVietnamese := detectUserLanguage(ctx.User)
		cancelMsg := "✅ Đã hủy bỏ yêu cầu."
		if !isVietnamese {
			cancelMsg = "✅ Request cancelled."
		}
		return &erp_modules.ModuleResponse{
			Success:     true,
			Message:     cancelMsg,
			ActionTaken: "workflow_cancelled",
		}, nil
	}

	return nil, fmt.Errorf("unhandled intent: %s", response.Intent)
}

// processConfirmationResponse processes final confirmation response
func (we *WorkflowEngine) processConfirmationResponse(
	ctx *erp_modules.ModuleContext,
	workflow *WorkflowState,
	step *WorkflowStep,
	message string,
) (*erp_modules.ModuleResponse, error) {

	// Parse confirmation response using LLM
	response, err := we.module.parseConfirmationResponse(ctx, message, workflow.CompletedData)
	if err != nil {
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

	switch response.Intent {
	case "confirm":
		// Execute final creation
		we.markStepCompleted(workflow, step)
		return we.executeFinalAction(ctx, workflow)

	case "modify":
		// Apply modifications and rebuild workflow
		for key, value := range response.Modifications {
			workflow.CompletedData[key] = value
		}

		// Rebuild workflow steps
		if err := we.buildWorkflowSteps(workflow); err != nil {
			return nil, fmt.Errorf("failed to rebuild workflow after modification: %w", err)
		}

		// Find first incomplete step
		workflow.CurrentStepIndex = we.findFirstIncompleteStep(workflow)
		workflow.LastModified = time.Now().UnixMilli()

		// Execute current step
		return we.executeCurrentStep(ctx, workflow)

	case "cancel":
		we.cancelWorkflow(ctx.User.Id)
		isVietnamese := detectUserLanguage(ctx.User)
		cancelMsg := "✅ Đã hủy bỏ yêu cầu."
		if !isVietnamese {
			cancelMsg = "✅ Request cancelled."
		}
		return &erp_modules.ModuleResponse{
			Success:     true,
			Message:     cancelMsg,
			ActionTaken: "workflow_cancelled",
		}, nil
	}

	return nil, fmt.Errorf("unhandled intent: %s", response.Intent)
}

// executeFinalAction executes the final creation action
func (we *WorkflowEngine) executeFinalAction(
	ctx *erp_modules.ModuleContext,
	workflow *WorkflowState,
) (*erp_modules.ModuleResponse, error) {

	defer we.cancelWorkflow(ctx.User.Id) // Clean up workflow

	// Execute based on workflow type
	switch workflow.WorkflowType {
	case "create_project":
		return we.executeProjectCreation(ctx, workflow)
	case "create_task":
		return we.executeTaskCreation(ctx, workflow)
	}

	return nil, fmt.Errorf("unknown workflow type: %s", workflow.WorkflowType)
}

// executeProjectCreation executes project creation
func (we *WorkflowEngine) executeProjectCreation(
	ctx *erp_modules.ModuleContext,
	workflow *WorkflowState,
) (*erp_modules.ModuleResponse, error) {

	var projectRequest ProjectCreationRequest
	dataBytes, _ := json.Marshal(workflow.CompletedData)
	json.Unmarshal(dataBytes, &projectRequest)

	// Create project
	projectID, err := we.module.erpClient.CreateProject(projectRequest, workflow.EmployeeID)
	if err != nil {
		isVietnamese := detectUserLanguage(ctx.User)
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
		err := we.module.erpClient.AssignProjectToEmployee(projectID, workflow.CreatorEmail, assignedEmployee.Email, projectRequest.Priority)
		if err != nil {
			we.module.api.LogWarn("Project created but assignment failed",
				"project_id", projectID,
				"assignee", assignedEmployee.EmployeeName,
				"error", err.Error())
			assignmentErrors = append(assignmentErrors, assignedEmployee.EmployeeName)
		}
	}

	isVietnamese := detectUserLanguage(ctx.User)
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
		ActionTaken: "project_created",
		Data: map[string]interface{}{
			"project_id":         projectID,
			"project_name":       projectRequest.ProjectName,
			"assigned_employees": projectRequest.AssignedToEmployees,
			"assignment_errors":  assignmentErrors,
		},
	}, nil
}

// executeTaskCreation executes task creation
func (we *WorkflowEngine) executeTaskCreation(
	ctx *erp_modules.ModuleContext,
	workflow *WorkflowState,
) (*erp_modules.ModuleResponse, error) {

	var taskRequest TaskCreationRequest
	dataBytes, _ := json.Marshal(workflow.CompletedData)
	json.Unmarshal(dataBytes, &taskRequest)

	// Create task
	taskID, err := we.module.erpClient.CreateTask(taskRequest, workflow.EmployeeID)
	if err != nil {
		isVietnamese := detectUserLanguage(ctx.User)
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
		err := we.module.erpClient.AssignTaskToEmployee(taskID, workflow.CreatorEmail, assignedEmployee.Email, taskRequest.Priority)
		if err != nil {
			we.module.api.LogWarn("Task created but assignment failed",
				"task_id", taskID,
				"assignee", assignedEmployee.EmployeeName,
				"error", err.Error())
			assignmentErrors = append(assignmentErrors, assignedEmployee.EmployeeName)
		}
	}

	isVietnamese := detectUserLanguage(ctx.User)
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
		ActionTaken: "task_created",
		Data: map[string]interface{}{
			"task_id":            taskID,
			"task_name":          taskRequest.Subject,
			"assigned_employees": taskRequest.AssignedToEmployees,
			"assignment_errors":  assignmentErrors,
			"project":            taskRequest.Project,
		},
	}, nil
}

// assignToExistingEntity assigns employees to existing project/task
func (we *WorkflowEngine) assignToExistingEntity(
	ctx *erp_modules.ModuleContext,
	workflow *WorkflowState,
	selectedEntity interface{},
) (*erp_modules.ModuleResponse, error) {

	defer we.cancelWorkflow(ctx.User.Id) // Clean up workflow

	// Extract assigned employees
	var assignedEmployees []AssignedEmployee
	if assignedToEmployees, ok := workflow.CompletedData["assigned_to_employees"].([]interface{}); ok {
		for _, emp := range assignedToEmployees {
			if empBytes, err := json.Marshal(emp); err == nil {
				var assignedEmp AssignedEmployee
				if json.Unmarshal(empBytes, &assignedEmp) == nil {
					assignedEmployees = append(assignedEmployees, assignedEmp)
				}
			}
		}
	}

	// Get entity ID and name
	var entityID, entityName string
	if workflow.EntityType == "project" {
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

	if len(assignedEmployees) == 0 {
		isVietnamese := detectUserLanguage(ctx.User)
		var successMsg string
		if isVietnamese {
			if workflow.EntityType == "project" {
				successMsg = fmt.Sprintf("✅ Đã chọn dự án hiện có: **%s**. Tuy nhiên, không có nhân viên nào được chỉ định để phân công.", entityName)
			} else {
				successMsg = fmt.Sprintf("✅ Đã chọn task hiện có: **%s**. Tuy nhiên, không có nhân viên nào được chỉ định để phân công.", entityName)
			}
		} else {
			if workflow.EntityType == "project" {
				successMsg = fmt.Sprintf("✅ Selected existing project: **%s**. However, no employees were specified for assignment.", entityName)
			} else {
				successMsg = fmt.Sprintf("✅ Selected existing task: **%s**. However, no employees were specified for assignment.", entityName)
			}
		}

		return &erp_modules.ModuleResponse{
			Success:     true,
			Message:     successMsg,
			ActionTaken: "existing_entity_selected_no_assignment",
			Data: map[string]interface{}{
				"entity_type": workflow.EntityType,
				"entity_id":   entityID,
				"entity_name": entityName,
			},
		}, nil
	}

	// Perform assignments
	var assignmentErrors []string
	priority := "Medium"
	if originalPriority, ok := workflow.CompletedData["priority"].(string); ok && originalPriority != "" {
		priority = originalPriority
	}

	for _, assignedEmployee := range assignedEmployees {
		var err error
		if workflow.EntityType == "project" {
			err = we.module.erpClient.AssignProjectToEmployee(entityID, workflow.CreatorEmail, assignedEmployee.Email, priority)
		} else {
			err = we.module.erpClient.AssignTaskToEmployee(entityID, workflow.CreatorEmail, assignedEmployee.Email, priority)
		}

		if err != nil {
			we.module.api.LogWarn("Assignment to existing entity failed",
				"entity_type", workflow.EntityType,
				"entity_id", entityID,
				"assignee", assignedEmployee.EmployeeName,
				"error", err.Error())
			assignmentErrors = append(assignmentErrors, assignedEmployee.EmployeeName)
		}
	}

	isVietnamese := detectUserLanguage(ctx.User)
	assigneeNames := make([]string, len(assignedEmployees))
	for i, emp := range assignedEmployees {
		assigneeNames[i] = emp.EmployeeName
	}
	assigneesStr := strings.Join(assigneeNames, ", ")

	var successMsg string
	if len(assignmentErrors) == 0 {
		if isVietnamese {
			if workflow.EntityType == "project" {
				successMsg = fmt.Sprintf("✅ Đã phân công dự án hiện có **%s** cho **%s**!", entityName, assigneesStr)
			} else {
				successMsg = fmt.Sprintf("✅ Đã phân công task hiện có **%s** cho **%s**!", entityName, assigneesStr)
			}
		} else {
			if workflow.EntityType == "project" {
				successMsg = fmt.Sprintf("✅ Successfully assigned existing project **%s** to **%s**!", entityName, assigneesStr)
			} else {
				successMsg = fmt.Sprintf("✅ Successfully assigned existing task **%s** to **%s**!", entityName, assigneesStr)
			}
		}
	} else {
		if isVietnamese {
			if workflow.EntityType == "project" {
				successMsg = fmt.Sprintf("✅ Đã phân công dự án hiện có **%s**. Thành công: **%s**. Lỗi: **%s**.",
					entityName, assigneesStr, strings.Join(assignmentErrors, ", "))
			} else {
				successMsg = fmt.Sprintf("✅ Đã phân công task hiện có **%s**. Thành công: **%s**. Lỗi: **%s**.",
					entityName, assigneesStr, strings.Join(assignmentErrors, ", "))
			}
		} else {
			if workflow.EntityType == "project" {
				successMsg = fmt.Sprintf("✅ Assigned existing project **%s**. Successful: **%s**. Failed: **%s**.",
					entityName, assigneesStr, strings.Join(assignmentErrors, ", "))
			} else {
				successMsg = fmt.Sprintf("✅ Assigned existing task **%s**. Successful: **%s**. Failed: **%s**.",
					entityName, assigneesStr, strings.Join(assignmentErrors, ", "))
			}
		}
	}

	return &erp_modules.ModuleResponse{
		Success:     true,
		Message:     successMsg,
		ActionTaken: "existing_entity_assigned",
		Data: map[string]interface{}{
			"entity_type":        workflow.EntityType,
			"entity_id":          entityID,
			"entity_name":        entityName,
			"assigned_employees": assignedEmployees,
			"assignment_errors":  assignmentErrors,
		},
	}, nil
}

// Helper methods

// cancelWorkflow cancels and cleans up a workflow
func (we *WorkflowEngine) cancelWorkflow(userID string) {
	delete(we.activeWorkflows, userID)
}

// markStepCompleted marks a step as completed
func (we *WorkflowEngine) markStepCompleted(workflow *WorkflowState, step *WorkflowStep) {
	step.Completed = true
	workflow.LastModified = time.Now().UnixMilli()
}

// markCurrentStepCompleted marks current step as completed
func (we *WorkflowEngine) markCurrentStepCompleted(workflow *WorkflowState) {
	if workflow.CurrentStepIndex < len(workflow.Steps) {
		workflow.Steps[workflow.CurrentStepIndex].Completed = true
		workflow.LastModified = time.Now().UnixMilli()
	}
}

// moveToNextStep moves to the next step
func (we *WorkflowEngine) moveToNextStep(workflow *WorkflowState) {
	workflow.CurrentStepIndex++
}

// moveToNextStepAndExecute moves to next step and executes it
func (we *WorkflowEngine) moveToNextStepAndExecute(
	ctx *erp_modules.ModuleContext,
	workflow *WorkflowState,
) (*erp_modules.ModuleResponse, error) {
	we.moveToNextStep(workflow)
	return we.executeCurrentStep(ctx, workflow)
}

// findFirstIncompleteStep finds the first incomplete step
func (we *WorkflowEngine) findFirstIncompleteStep(workflow *WorkflowState) int {
	for i, step := range workflow.Steps {
		if !step.Completed {
			return i
		}
	}
	return len(workflow.Steps) // All completed
}

// findNextAvailableStep finds next step with met dependencies
func (we *WorkflowEngine) findNextAvailableStep(workflow *WorkflowState) int {
	for i := workflow.CurrentStepIndex; i < len(workflow.Steps); i++ {
		step := &workflow.Steps[i]
		if !step.Completed && we.areDependenciesMet(workflow, step) {
			return i
		}
	}
	return len(workflow.Steps) // No available step
}

// areDependenciesMet checks if step dependencies are met
func (we *WorkflowEngine) areDependenciesMet(workflow *WorkflowState, step *WorkflowStep) bool {
	for _, depID := range step.Dependencies {
		found := false
		for _, s := range workflow.Steps {
			if s.ID == depID && s.Completed {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

// canSkipCurrentStep checks if current step can be skipped
func (we *WorkflowEngine) canSkipCurrentStep(workflow *WorkflowState) bool {
	if workflow.CurrentStepIndex >= len(workflow.Steps) {
		return false
	}
	step := &workflow.Steps[workflow.CurrentStepIndex]
	return !step.Required
}

// resetIncompleteSteps resets all incomplete steps
func (we *WorkflowEngine) resetIncompleteSteps(workflow *WorkflowState) {
	for i := range workflow.Steps {
		workflow.Steps[i].Completed = false
		workflow.Steps[i].Data = make(map[string]interface{})
	}
	workflow.LastModified = time.Now().UnixMilli()
}

// getAllPreviousStepIDs gets all previous step IDs for dependencies
func (we *WorkflowEngine) getAllPreviousStepIDs(workflow *WorkflowState) []string {
	var deps []string
	for _, step := range workflow.Steps {
		if step.ID != "final_confirmation" {
			deps = append(deps, step.ID)
		}
	}
	return deps
}

// parseModificationRequest parses user modification request using LLM
func (we *WorkflowEngine) parseModificationRequest(
	ctx *erp_modules.ModuleContext,
	workflow *WorkflowState,
	message string,
) (map[string]interface{}, error) {

	// Use existing LLM parsing logic with current data
	response, err := we.module.parseConfirmationResponse(ctx, message, workflow.CompletedData)
	if err != nil || response.Intent != "modify" {
		return nil, err
	}

	return response.Modifications, nil
}

// HasActiveWorkflow checks if user has active workflow
func (we *WorkflowEngine) HasActiveWorkflow(userID string) bool {
	_, exists := we.activeWorkflows[userID]
	return exists
}

// GetActiveWorkflow gets active workflow for user
func (we *WorkflowEngine) GetActiveWorkflow(userID string) (*WorkflowState, bool) {
	workflow, exists := we.activeWorkflows[userID]
	return workflow, exists
}
