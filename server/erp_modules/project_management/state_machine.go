// server/erp_modules/project_management/state_machine.go

package project_management

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/mattermost/mattermost-plugin-ai/server/erp_modules"
)

// MultiStepProcessor handles complex multi-step workflows
type MultiStepProcessor struct {
	module *ProjectManagementModule
}

// NewMultiStepProcessor creates a new multi-step processor
func NewMultiStepProcessor(module *ProjectManagementModule) *MultiStepProcessor {
	return &MultiStepProcessor{
		module: module,
	}
}

// ProcessTaskCreation processes task creation with full workflow
func (p *MultiStepProcessor) ProcessTaskCreation(employeeID string, ctx *erp_modules.ModuleContext, intent *erp_modules.Intent) (*erp_modules.ModuleResponse, error) {
	// Parse initial request
	taskRequest, err := p.module.analyzeTaskCreation(ctx, intent.RawMessage)
	if err != nil {
		return p.handleAnalysisError(ctx, err, "task")
	}

	// Validate required fields
	if taskRequest.Subject == "" {
		return p.handleMissingSubject(ctx)
	}

	// Convert to processing state
	state := &ProcessingState{
		UserID:         ctx.User.Id,
		Step:           StepAnalyzeExistingTask,
		Type:           "task",
		Action:         "create_task",
		EmployeeID:     employeeID,
		CreatedAt:      time.Now().UnixMilli(),
		CompletedSteps: make(map[ProcessingStep]bool),
	}

	// Convert request to map
	requestBytes, _ := json.Marshal(taskRequest)
	json.Unmarshal(requestBytes, &state.OriginalRequest)

	// Get creator email
	creatorEmail, err := p.module.getEmployeeEmailFromUser(ctx.User)
	if err != nil {
		p.module.api.LogWarn("Failed to get creator email", "error", err.Error())
		creatorEmail = "demo@example.com"
	}
	state.CreatorEmail = creatorEmail

	// Start processing from step 1
	return p.ProcessNextStep(ctx, state)
}

// ProcessProjectCreation processes project creation with full workflow
func (p *MultiStepProcessor) ProcessProjectCreation(employeeID string, ctx *erp_modules.ModuleContext, intent *erp_modules.Intent) (*erp_modules.ModuleResponse, error) {
	// Parse initial request
	projectRequest, err := p.module.analyzeProjectCreation(ctx, intent.RawMessage)
	if err != nil {
		return p.handleAnalysisError(ctx, err, "project")
	}

	// Validate required fields
	if projectRequest.ProjectName == "" {
		return p.handleMissingProjectName(ctx)
	}

	// Convert to processing state
	state := &ProcessingState{
		UserID:         ctx.User.Id,
		Step:           StepAnalyzeExistingProject,
		Type:           "project",
		Action:         "create_project",
		EmployeeID:     employeeID,
		CreatedAt:      time.Now().UnixMilli(),
		CompletedSteps: make(map[ProcessingStep]bool),
	}

	// Convert request to map
	requestBytes, _ := json.Marshal(projectRequest)
	json.Unmarshal(requestBytes, &state.OriginalRequest)

	// Get creator email
	creatorEmail, err := p.module.getEmployeeEmailFromUser(ctx.User)
	if err != nil {
		p.module.api.LogWarn("Failed to get creator email", "error", err.Error())
		creatorEmail = "demo@example.com"
	}
	state.CreatorEmail = creatorEmail

	// Start processing from step 1
	return p.ProcessNextStep(ctx, state)
}

// ProcessNextStep processes the next step in the workflow
func (p *MultiStepProcessor) ProcessNextStep(ctx *erp_modules.ModuleContext, state *ProcessingState) (*erp_modules.ModuleResponse, error) {
	p.module.api.LogDebug("Processing step", "step", state.Step, "user_id", state.UserID)

	switch state.Step {
	case StepAnalyzeExistingTask:
		return p.handleAnalyzeExistingTask(ctx, state)
	case StepAnalyzeExistingProject:
		return p.handleAnalyzeExistingProject(ctx, state)
	case StepResolveEmployees:
		return p.handleResolveEmployees(ctx, state)
	case StepResolveProject:
		return p.handleResolveProject(ctx, state)
	case StepFinalConfirmation:
		return p.handleFinalConfirmation(ctx, state)
	case StepComplete:
		return p.handleCompletion(ctx, state)
	default:
		return nil, fmt.Errorf("unknown processing step: %s", state.Step)
	}
}

// handleAnalyzeExistingTask checks for existing tasks with similar names
func (p *MultiStepProcessor) handleAnalyzeExistingTask(ctx *erp_modules.ModuleContext, state *ProcessingState) (*erp_modules.ModuleResponse, error) {
	subject := state.OriginalRequest["subject"].(string)

	// FIRST: Resolve employees if any are specified
	assigneeNamesInterface, hasAssignees := state.OriginalRequest["assigned_to_names"]
	if hasAssignees {
		var assigneeNames []string
		if names, ok := assigneeNamesInterface.([]interface{}); ok {
			for _, name := range names {
				if nameStr, ok := name.(string); ok {
					assigneeNames = append(assigneeNames, nameStr)
				}
			}
		}

		if len(assigneeNames) > 0 {
			// Try to resolve employees first
			assigneeResult, err := p.module.resolveMultipleAssignees(assigneeNames)
			if err != nil {
				p.module.api.LogError("Failed to resolve employees during existing task analysis", "error", err.Error())
			} else if assigneeResult.RequiresDisambiguation {
				// Store the result and request employee disambiguation
				state.PendingEmployeeResolution = assigneeResult
				state.ResolvedEmployees = assigneeResult.ResolvedEmployees
				state.Step = StepResolveEmployees
				p.module.confirmationManager.StoreProcessingState(ctx.User.Id, state)
				return p.requestEmployeeDisambiguation(ctx, state, assigneeResult)
			} else {
				// All employees resolved successfully
				state.ResolvedEmployees = assigneeResult.ResolvedEmployees
			}
		}
	}

	// SECOND: Check for existing tasks
	existingTasks, err := p.module.erpClient.SearchTasksByName(subject)
	if err != nil {
		p.module.api.LogError("Failed to search existing tasks", "error", err.Error())
		// Continue to next step even if search fails
		return p.moveToNextStep(ctx, state, p.getNextStepAfterExistingAnalysis(state))
	}

	// Filter high confidence matches
	var highConfidenceTasks []Task
	for _, task := range existingTasks {
		if task.MatchConfidence >= 0.85 {
			highConfidenceTasks = append(highConfidenceTasks, task)
		}
	}

	if len(highConfidenceTasks) > 0 {
		// Store existing entities and request disambiguation
		var entities []interface{}
		for _, task := range highConfidenceTasks {
			entities = append(entities, task)
		}
		state.PendingExistingEntities = entities

		// Store state and request disambiguation
		p.module.confirmationManager.StoreProcessingState(ctx.User.Id, state)
		return p.requestExistingEntityDisambiguation(ctx, state, "task", entities)
	}

	// No existing matches, move to next step
	state.CompletedSteps[StepAnalyzeExistingTask] = true
	return p.moveToNextStep(ctx, state, p.getNextStepAfterExistingAnalysis(state))
}

// handleAnalyzeExistingProject checks for existing projects with similar names
func (p *MultiStepProcessor) handleAnalyzeExistingProject(ctx *erp_modules.ModuleContext, state *ProcessingState) (*erp_modules.ModuleResponse, error) {
	projectName := state.OriginalRequest["project_name"].(string)

	// FIRST: Resolve employees if any are specified
	assigneeNamesInterface, hasAssignees := state.OriginalRequest["assigned_to_names"]
	if hasAssignees {
		var assigneeNames []string
		if names, ok := assigneeNamesInterface.([]interface{}); ok {
			for _, name := range names {
				if nameStr, ok := name.(string); ok {
					assigneeNames = append(assigneeNames, nameStr)
				}
			}
		}

		if len(assigneeNames) > 0 {
			// Try to resolve employees first
			assigneeResult, err := p.module.resolveMultipleAssignees(assigneeNames)
			if err != nil {
				p.module.api.LogError("Failed to resolve employees during existing project analysis", "error", err.Error())
			} else if assigneeResult.RequiresDisambiguation {
				// Store the result and request employee disambiguation
				state.PendingEmployeeResolution = assigneeResult
				state.ResolvedEmployees = assigneeResult.ResolvedEmployees
				state.Step = StepResolveEmployees
				p.module.confirmationManager.StoreProcessingState(ctx.User.Id, state)
				return p.requestEmployeeDisambiguation(ctx, state, assigneeResult)
			} else {
				// All employees resolved successfully
				state.ResolvedEmployees = assigneeResult.ResolvedEmployees
			}
		}
	}

	// SECOND: Check for existing projects
	existingProjects, err := p.module.erpClient.SearchProjectsByName(projectName)
	if err != nil {
		p.module.api.LogError("Failed to search existing projects", "error", err.Error())
		// Continue to next step even if search fails
		return p.moveToNextStep(ctx, state, p.getNextStepAfterExistingAnalysis(state))
	}

	// Filter high confidence matches
	var highConfidenceProjects []Project
	for _, project := range existingProjects {
		if project.MatchConfidence >= 0.85 {
			highConfidenceProjects = append(highConfidenceProjects, project)
		}
	}

	if len(highConfidenceProjects) > 0 {
		// Store existing entities and request disambiguation
		var entities []interface{}
		for _, project := range highConfidenceProjects {
			entities = append(entities, project)
		}
		state.PendingExistingEntities = entities

		// Store state and request disambiguation
		p.module.confirmationManager.StoreProcessingState(ctx.User.Id, state)
		return p.requestExistingEntityDisambiguation(ctx, state, "project", entities)
	}

	// No existing matches, move to next step
	state.CompletedSteps[StepAnalyzeExistingProject] = true
	return p.moveToNextStep(ctx, state, p.getNextStepAfterExistingAnalysis(state))
}

// getNextStepAfterExistingAnalysis determines next step after existing entity analysis
func (p *MultiStepProcessor) getNextStepAfterExistingAnalysis(state *ProcessingState) ProcessingStep {
	// If employees not resolved yet, go to resolve employees
	if len(state.ResolvedEmployees) == 0 && state.PendingEmployeeResolution == nil {
		assigneeNamesInterface, hasAssignees := state.OriginalRequest["assigned_to_names"]
		if hasAssignees {
			if names, ok := assigneeNamesInterface.([]interface{}); ok && len(names) > 0 {
				return StepResolveEmployees
			}
		}
	}

	// For tasks, check if project needs resolution
	if state.Type == "task" {
		if projectName, ok := state.OriginalRequest["project"].(string); ok && projectName != "" {
			return StepResolveProject
		}
	}

	return StepFinalConfirmation
}

// handleResolveEmployees resolves employee assignments
func (p *MultiStepProcessor) handleResolveEmployees(ctx *erp_modules.ModuleContext, state *ProcessingState) (*erp_modules.ModuleResponse, error) {
	assigneeNamesInterface, hasAssignees := state.OriginalRequest["assigned_to_names"]
	if !hasAssignees {
		// No employees to resolve, move to next step
		state.CompletedSteps[StepResolveEmployees] = true
		return p.moveToNextStep(ctx, state, p.getNextStepAfterEmployees(state))
	}

	// Convert to string slice
	var assigneeNames []string
	if names, ok := assigneeNamesInterface.([]interface{}); ok {
		for _, name := range names {
			if nameStr, ok := name.(string); ok {
				assigneeNames = append(assigneeNames, nameStr)
			}
		}
	}

	if len(assigneeNames) == 0 {
		// No employees to resolve, move to next step
		state.CompletedSteps[StepResolveEmployees] = true
		return p.moveToNextStep(ctx, state, p.getNextStepAfterEmployees(state))
	}

	// Try to resolve employees
	assigneeResult, err := p.module.resolveMultipleAssignees(assigneeNames)
	if err != nil {
		p.module.api.LogError("Failed to resolve employees", "error", err.Error())
		// Continue without employees
		state.CompletedSteps[StepResolveEmployees] = true
		return p.moveToNextStep(ctx, state, p.getNextStepAfterEmployees(state))
	}

	if assigneeResult.RequiresDisambiguation {
		// Store pending resolution and request disambiguation
		state.PendingEmployeeResolution = assigneeResult
		state.ResolvedEmployees = assigneeResult.ResolvedEmployees

		// Store state and request disambiguation
		p.module.confirmationManager.StoreProcessingState(ctx.User.Id, state)
		return p.requestEmployeeDisambiguation(ctx, state, assigneeResult)
	}

	// All employees resolved successfully
	state.ResolvedEmployees = assigneeResult.ResolvedEmployees
	state.CompletedSteps[StepResolveEmployees] = true
	return p.moveToNextStep(ctx, state, p.getNextStepAfterEmployees(state))
}

// handleResolveProject resolves project selection for tasks
func (p *MultiStepProcessor) handleResolveProject(ctx *erp_modules.ModuleContext, state *ProcessingState) (*erp_modules.ModuleResponse, error) {
	projectInterface, hasProject := state.OriginalRequest["project"]
	if !hasProject {
		// No project specified, move to final confirmation
		state.CompletedSteps[StepResolveProject] = true
		return p.moveToNextStep(ctx, state, StepFinalConfirmation)
	}

	projectName, ok := projectInterface.(string)
	if !ok || projectName == "" {
		// No project specified, move to final confirmation
		state.CompletedSteps[StepResolveProject] = true
		return p.moveToNextStep(ctx, state, StepFinalConfirmation)
	}

	// Search for matching projects
	matchingProjects, err := p.module.erpClient.SearchProjectsByName(projectName)
	if err != nil {
		p.module.api.LogError("Failed to search projects", "error", err.Error())
		// Continue without project
		state.OriginalRequest["project"] = ""
		state.CompletedSteps[StepResolveProject] = true
		return p.moveToNextStep(ctx, state, StepFinalConfirmation)
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
		state.OriginalRequest["project"] = ""
		state.CompletedSteps[StepResolveProject] = true
		return p.moveToNextStep(ctx, state, StepFinalConfirmation)
	}

	if len(highConfidenceProjects) == 1 {
		// Single match, use it
		state.SelectedProject = highConfidenceProjects[0].Name
		state.OriginalRequest["project"] = highConfidenceProjects[0].Name
		state.CompletedSteps[StepResolveProject] = true
		return p.moveToNextStep(ctx, state, StepFinalConfirmation)
	}

	// Multiple matches, need disambiguation
	state.PendingProjectSelection = highConfidenceProjects

	// Store state and request disambiguation
	p.module.confirmationManager.StoreProcessingState(ctx.User.Id, state)
	return p.requestProjectSelectionDisambiguation(ctx, state, highConfidenceProjects)
}

// handleFinalConfirmation shows final confirmation with all resolved data
func (p *MultiStepProcessor) handleFinalConfirmation(ctx *erp_modules.ModuleContext, state *ProcessingState) (*erp_modules.ModuleResponse, error) {
	// Update request with all resolved data
	state.OriginalRequest["assigned_to_employees"] = state.ResolvedEmployees

	// Generate final confirmation message
	confirmationMsg, err := p.module.generateConfirmationMessage(ctx, state.Type, state.OriginalRequest)
	if err != nil {
		isVietnamese := detectUserLanguage(ctx.User)
		errorMsg := "⚠️ Có lỗi xảy ra khi tạo tin nhắn xác nhận cuối cùng."
		if !isVietnamese {
			errorMsg = "⚠️ An error occurred while creating final confirmation message."
		}
		return &erp_modules.ModuleResponse{
			Success: false,
			Message: errorMsg,
			Error:   err.Error(),
		}, nil
	}

	// Store final confirmation state
	finalConfirmation := &ProjectManagementConfirmation{
		UserID:       state.UserID,
		Type:         state.Type,
		Action:       state.Action,
		Data:         state.OriginalRequest,
		CreatedAt:    time.Now().UnixMilli(),
		EmployeeID:   state.EmployeeID,
		CreatorEmail: state.CreatorEmail,
	}

	p.module.confirmationManager.StorePendingConfirmation(ctx.User.Id, finalConfirmation)

	return &erp_modules.ModuleResponse{
		Success:     true,
		Message:     confirmationMsg,
		ActionTaken: "request_final_confirmation",
		Data: map[string]interface{}{
			"awaiting_confirmation": true,
			"type":                  state.Type,
			"all_steps_completed":   true,
		},
	}, nil
}

// handleCompletion executes the final creation
func (p *MultiStepProcessor) handleCompletion(ctx *erp_modules.ModuleContext, state *ProcessingState) (*erp_modules.ModuleResponse, error) {
	switch state.Action {
	case "create_task":
		return p.module.handleCreateTask(state.EmployeeID, ctx.User.Id, state.OriginalRequest, state.CreatorEmail)
	case "create_project":
		return p.module.handleCreateProject(state.EmployeeID, ctx.User.Id, state.OriginalRequest, state.CreatorEmail)
	default:
		return nil, fmt.Errorf("unknown action: %s", state.Action)
	}
}

// Helper methods for step management
func (p *MultiStepProcessor) moveToNextStep(ctx *erp_modules.ModuleContext, state *ProcessingState, nextStep ProcessingStep) (*erp_modules.ModuleResponse, error) {
	state.Step = nextStep
	return p.ProcessNextStep(ctx, state)
}

func (p *MultiStepProcessor) getNextStepAfterEmployees(state *ProcessingState) ProcessingStep {
	if state.Type == "task" {
		// For tasks, check if project needs resolution
		if projectName, ok := state.OriginalRequest["project"].(string); ok && projectName != "" {
			return StepResolveProject
		}
	}
	return StepFinalConfirmation
}

// Request methods for different types of disambiguation
func (p *MultiStepProcessor) requestExistingEntityDisambiguation(ctx *erp_modules.ModuleContext, state *ProcessingState, entityType string, entities []interface{}) (*erp_modules.ModuleResponse, error) {
	// Generate disambiguation message
	disambiguationMsg, err := p.module.generateExistingEntityDisambiguationMessage(ctx, &ExistingEntityDisambiguationConfirmation{
		EntityType:       entityType,
		ExistingEntities: entities,
	})
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
			"matches_count":           len(entities),
		},
	}, nil
}

func (p *MultiStepProcessor) requestEmployeeDisambiguation(ctx *erp_modules.ModuleContext, state *ProcessingState, assigneeResult *AssigneeResolutionResult) (*erp_modules.ModuleResponse, error) {
	// Generate disambiguation message
	disambiguationMsg, err := p.module.generateMultiEmployeeDisambiguationMessage(ctx, assigneeResult)
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
		ActionTaken: "request_employee_disambiguation",
		Data: map[string]interface{}{
			"awaiting_disambiguation": true,
			"type":                    "employee_selection",
			"unresolved_count":        len(assigneeResult.UnresolvedEmployeeMatches),
			"resolved_count":          len(assigneeResult.ResolvedEmployees),
		},
	}, nil
}

func (p *MultiStepProcessor) requestProjectSelectionDisambiguation(ctx *erp_modules.ModuleContext, state *ProcessingState, projects []Project) (*erp_modules.ModuleResponse, error) {
	// Generate disambiguation message
	disambiguationMsg, err := p.module.generateProjectTaskDisambiguationMessage(ctx, &ProjectTaskDisambiguationConfirmation{
		MatchingProjects: projects,
	})
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
		ActionTaken: "request_project_selection",
		Data: map[string]interface{}{
			"awaiting_disambiguation": true,
			"type":                    "project_selection",
			"projects_count":          len(projects),
		},
	}, nil
}

// Error handling methods
func (p *MultiStepProcessor) handleAnalysisError(ctx *erp_modules.ModuleContext, err error, entityType string) (*erp_modules.ModuleResponse, error) {
	isVietnamese := detectUserLanguage(ctx.User)
	var errorMsg string
	if entityType == "task" {
		errorMsg = "⚠️ Không thể hiểu được yêu cầu tạo task. Vui lòng thử lại với thông tin rõ ràng hơn."
		if !isVietnamese {
			errorMsg = "⚠️ Cannot understand the task creation request. Please try again with clearer information."
		}
	} else {
		errorMsg = "⚠️ Không thể hiểu được yêu cầu tạo dự án. Vui lòng thử lại với thông tin rõ ràng hơn."
		if !isVietnamese {
			errorMsg = "⚠️ Cannot understand the project creation request. Please try again with clearer information."
		}
	}

	return &erp_modules.ModuleResponse{
		Success: false,
		Message: errorMsg,
		Error:   err.Error(),
	}, nil
}

func (p *MultiStepProcessor) handleMissingSubject(ctx *erp_modules.ModuleContext) (*erp_modules.ModuleResponse, error) {
	isVietnamese := detectUserLanguage(ctx.User)
	errorMsg := "📝 Vui lòng cung cấp tên công việc. Ví dụ: 'Tạo task thiết kế giao diện trang chủ'"
	if !isVietnamese {
		errorMsg = "📝 Please provide task name. Example: 'Create homepage design task'"
	}

	return &erp_modules.ModuleResponse{
		Success: false,
		Message: errorMsg,
	}, nil
}

func (p *MultiStepProcessor) handleMissingProjectName(ctx *erp_modules.ModuleContext) (*erp_modules.ModuleResponse, error) {
	isVietnamese := detectUserLanguage(ctx.User)
	errorMsg := "📝 Vui lòng cung cấp tên dự án. Ví dụ: 'Tạo dự án phát triển website bán hàng'"
	if !isVietnamese {
		errorMsg = "📝 Please provide project name. Example: 'Create e-commerce website development project'"
	}

	return &erp_modules.ModuleResponse{
		Success: false,
		Message: errorMsg,
	}, nil
}
