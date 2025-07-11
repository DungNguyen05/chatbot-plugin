// Enhanced workflow engine that can handle complex multi-component requests
// This replaces the existing workflow_engine.go

package project_management

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/mattermost/mattermost-plugin-ai/server/erp_modules"
	"github.com/mattermost/mattermost-plugin-ai/server/llm"
)

// ComponentType represents different types of components that need resolution
type ComponentType string

const (
	ComponentTypeTask     ComponentType = "task"
	ComponentTypeEmployee ComponentType = "employee"
	ComponentTypeProject  ComponentType = "project"
)

// ComponentStatus represents the resolution status of a component
type ComponentStatus string

const (
	StatusPending   ComponentStatus = "pending"
	StatusResolving ComponentStatus = "resolving"
	StatusResolved  ComponentStatus = "resolved"
	StatusSkipped   ComponentStatus = "skipped"
)

// ComponentAnalysis represents the analysis of what needs to be clarified
type ComponentAnalysis struct {
	TaskComponent     *TaskComponentInfo     `json:"task_component,omitempty"`
	EmployeeComponent *EmployeeComponentInfo `json:"employee_component,omitempty"`
	ProjectComponent  *ProjectComponentInfo  `json:"project_component,omitempty"`
}

// TaskComponentInfo holds information about task resolution needs
type TaskComponentInfo struct {
	Status              ComponentStatus        `json:"status"`
	Action              string                 `json:"action"`         // "create_new", "use_existing", "needs_clarification"
	Name                string                 `json:"name,omitempty"` // extracted task name
	ExistingTasks       []Task                 `json:"existing_tasks,omitempty"`
	NeedsDisambiguation bool                   `json:"needs_disambiguation"`
	ResolutionData      map[string]interface{} `json:"resolution_data,omitempty"`
}

// EmployeeComponentInfo holds information about employee resolution needs
type EmployeeComponentInfo struct {
	Status              ComponentStatus           `json:"status"`
	Action              string                    `json:"action"` // "assign_to_existing", "needs_clarification"
	Names               []string                  `json:"names,omitempty"`
	ResolvedEmployees   []AssignedEmployee        `json:"resolved_employees,omitempty"`
	UnresolvedMatches   []UnresolvedEmployeeMatch `json:"unresolved_matches,omitempty"`
	NeedsDisambiguation bool                      `json:"needs_disambiguation"`
	ResolutionData      map[string]interface{}    `json:"resolution_data,omitempty"`
}

// ProjectComponentInfo holds information about project resolution needs
type ProjectComponentInfo struct {
	Status              ComponentStatus        `json:"status"`
	Action              string                 `json:"action"`         // "assign_to_existing", "no_project", "needs_clarification"
	Name                string                 `json:"name,omitempty"` // extracted project name
	MatchingProjects    []Project              `json:"matching_projects,omitempty"`
	SelectedProject     string                 `json:"selected_project,omitempty"`
	NeedsDisambiguation bool                   `json:"needs_disambiguation"`
	ResolutionData      map[string]interface{} `json:"resolution_data,omitempty"`
}

// EnhancedWorkflowState represents the complete state with component analysis
type EnhancedWorkflowState struct {
	UserID            string                 `json:"user_id"`
	WorkflowType      string                 `json:"workflow_type"` // "create_project", "create_task"
	EntityType        string                 `json:"entity_type"`   // "project", "task"
	OriginalRequest   map[string]interface{} `json:"original_request"`
	ComponentAnalysis *ComponentAnalysis     `json:"component_analysis"`
	CurrentComponent  ComponentType          `json:"current_component,omitempty"`
	CreatedAt         int64                  `json:"created_at"`
	LastModified      int64                  `json:"last_modified"`
	EmployeeID        string                 `json:"employee_id"`
	CreatorEmail      string                 `json:"creator_email"`
	CompletedData     map[string]interface{} `json:"completed_data"`
	Phase             WorkflowPhase          `json:"phase"` // New field to track phase
}

// WorkflowPhase represents the current phase of the workflow
type WorkflowPhase string

const (
	PhaseAnalysis     WorkflowPhase = "analysis"     // Analyzing components
	PhaseResolution   WorkflowPhase = "resolution"   // Resolving components
	PhaseConfirmation WorkflowPhase = "confirmation" // Final confirmation
	PhaseExecution    WorkflowPhase = "execution"    // Creating entities
)

// EnhancedWorkflowEngine manages the enhanced workflow process
type EnhancedWorkflowEngine struct {
	module          *ProjectManagementModule
	activeWorkflows map[string]*EnhancedWorkflowState
}

// NewEnhancedWorkflowEngine creates a new enhanced workflow engine
func NewEnhancedWorkflowEngine(module *ProjectManagementModule) *EnhancedWorkflowEngine {
	return &EnhancedWorkflowEngine{
		module:          module,
		activeWorkflows: make(map[string]*EnhancedWorkflowState),
	}
}

// StartWorkflow starts a new enhanced workflow
func (we *EnhancedWorkflowEngine) StartWorkflow(
	ctx *erp_modules.ModuleContext,
	workflowType, entityType string,
	originalRequest map[string]interface{},
	employeeID, creatorEmail string,
) (*erp_modules.ModuleResponse, error) {

	// Create enhanced workflow state
	workflow := &EnhancedWorkflowState{
		UserID:          ctx.User.Id,
		WorkflowType:    workflowType,
		EntityType:      entityType,
		OriginalRequest: originalRequest,
		CreatedAt:       time.Now().UnixMilli(),
		LastModified:    time.Now().UnixMilli(),
		EmployeeID:      employeeID,
		CreatorEmail:    creatorEmail,
		CompletedData:   make(map[string]interface{}),
		Phase:           PhaseAnalysis,
	}

	// Copy original request to completed data
	for k, v := range originalRequest {
		workflow.CompletedData[k] = v
	}

	// Store workflow
	we.activeWorkflows[ctx.User.Id] = workflow

	// Start with comprehensive analysis
	return we.performComprehensiveAnalysis(ctx, workflow)
}

// performComprehensiveAnalysis analyzes all components that need resolution
func (we *EnhancedWorkflowEngine) performComprehensiveAnalysis(
	ctx *erp_modules.ModuleContext,
	workflow *EnhancedWorkflowState,
) (*erp_modules.ModuleResponse, error) {

	workflow.Phase = PhaseAnalysis

	// Create component analysis
	analysis := &ComponentAnalysis{}

	// Analyze task component (for existing task checks)
	if err := we.analyzeTaskComponent(workflow, analysis); err != nil {
		we.module.api.LogError("Failed to analyze task component", "error", err.Error())
	}

	// Analyze employee component (for assignments)
	if err := we.analyzeEmployeeComponent(workflow, analysis); err != nil {
		we.module.api.LogError("Failed to analyze employee component", "error", err.Error())
	}

	// Analyze project component (for project assignment)
	if err := we.analyzeProjectComponent(workflow, analysis); err != nil {
		we.module.api.LogError("Failed to analyze project component", "error", err.Error())
	}

	workflow.ComponentAnalysis = analysis
	workflow.LastModified = time.Now().UnixMilli()

	// Determine what needs resolution first
	return we.proceedWithResolution(ctx, workflow)
}

// analyzeTaskComponent analyzes if task needs clarification
func (we *EnhancedWorkflowEngine) analyzeTaskComponent(
	workflow *EnhancedWorkflowState,
	analysis *ComponentAnalysis,
) error {

	var taskName string
	if workflow.EntityType == "task" {
		if subject, ok := workflow.CompletedData["subject"].(string); ok && subject != "" {
			taskName = subject
		}
	} else if workflow.EntityType == "project" {
		if projectName, ok := workflow.CompletedData["project_name"].(string); ok && projectName != "" {
			taskName = projectName
		}
	}

	if taskName == "" {
		return nil // No task component to analyze
	}

	taskInfo := &TaskComponentInfo{
		Status: StatusPending,
		Name:   taskName,
	}

	// Search for existing tasks/projects
	if workflow.EntityType == "task" {
		tasks, err := we.module.erpClient.SearchTasksByName(taskName)
		if err == nil {
			var highConfidenceTasks []Task
			for _, task := range tasks {
				if task.MatchConfidence >= 0.85 {
					highConfidenceTasks = append(highConfidenceTasks, task)
				}
			}
			if len(highConfidenceTasks) > 0 {
				taskInfo.ExistingTasks = highConfidenceTasks
				taskInfo.NeedsDisambiguation = true
				taskInfo.Action = "needs_clarification"
			} else {
				taskInfo.Action = "create_new"
			}
		} else {
			taskInfo.Action = "create_new"
		}
	} else {
		// For projects, similar logic
		projects, err := we.module.erpClient.SearchProjectsByName(taskName)
		if err == nil {
			var highConfidenceProjects []Project
			for _, project := range projects {
				if project.MatchConfidence >= 0.85 {
					highConfidenceProjects = append(highConfidenceProjects, project)
				}
			}
			if len(highConfidenceProjects) > 0 {
				// Convert to Task format for consistent handling
				for _, project := range highConfidenceProjects {
					task := Task{
						Name:            project.Name,
						Subject:         project.ProjectName,
						Status:          project.Status,
						Priority:        project.Priority,
						Department:      project.Department,
						MatchConfidence: project.MatchConfidence,
					}
					taskInfo.ExistingTasks = append(taskInfo.ExistingTasks, task)
				}
				taskInfo.NeedsDisambiguation = true
				taskInfo.Action = "needs_clarification"
			} else {
				taskInfo.Action = "create_new"
			}
		} else {
			taskInfo.Action = "create_new"
		}
	}

	analysis.TaskComponent = taskInfo
	return nil
}

// analyzeEmployeeComponent analyzes employee assignment needs
func (we *EnhancedWorkflowEngine) analyzeEmployeeComponent(
	workflow *EnhancedWorkflowState,
	analysis *ComponentAnalysis,
) error {

	assigneeNames, ok := workflow.CompletedData["assigned_to_names"].([]interface{})
	if !ok || len(assigneeNames) == 0 {
		return nil // No employee assignment
	}

	// Convert to string slice
	var nameStrings []string
	for _, name := range assigneeNames {
		if nameStr, ok := name.(string); ok {
			nameStrings = append(nameStrings, nameStr)
		}
	}

	if len(nameStrings) == 0 {
		return nil
	}

	employeeInfo := &EmployeeComponentInfo{
		Status: StatusPending,
		Names:  nameStrings,
		Action: "assign_to_existing",
	}

	// Resolve employees
	assigneeResult, err := we.module.resolveMultipleAssignees(nameStrings)
	if err != nil {
		employeeInfo.Action = "needs_clarification"
		employeeInfo.NeedsDisambiguation = true
	} else {
		if assigneeResult.RequiresDisambiguation {
			employeeInfo.UnresolvedMatches = assigneeResult.UnresolvedEmployeeMatches
			employeeInfo.ResolvedEmployees = assigneeResult.ResolvedEmployees
			employeeInfo.NeedsDisambiguation = true
			employeeInfo.Action = "needs_clarification"
		} else {
			employeeInfo.ResolvedEmployees = assigneeResult.ResolvedEmployees
			employeeInfo.Status = StatusResolved
		}
	}

	analysis.EmployeeComponent = employeeInfo
	return nil
}

// analyzeProjectComponent analyzes project assignment needs (for tasks)
func (we *EnhancedWorkflowEngine) analyzeProjectComponent(
	workflow *EnhancedWorkflowState,
	analysis *ComponentAnalysis,
) error {

	// Only for tasks
	if workflow.EntityType != "task" {
		return nil
	}

	projectName, ok := workflow.CompletedData["project"].(string)
	if !ok || projectName == "" {
		return nil // No project assignment
	}

	projectInfo := &ProjectComponentInfo{
		Status: StatusPending,
		Name:   projectName,
	}

	// Search for matching projects
	projects, err := we.module.erpClient.SearchProjectsByName(projectName)
	if err != nil {
		projectInfo.Action = "no_project"
	} else {
		var highConfidenceProjects []Project
		for _, project := range projects {
			if project.MatchConfidence >= 0.85 {
				highConfidenceProjects = append(highConfidenceProjects, project)
			}
		}

		if len(highConfidenceProjects) == 0 {
			projectInfo.Action = "no_project"
		} else if len(highConfidenceProjects) == 1 {
			projectInfo.SelectedProject = highConfidenceProjects[0].Name
			projectInfo.Status = StatusResolved
			projectInfo.Action = "assign_to_existing"
		} else {
			projectInfo.MatchingProjects = highConfidenceProjects
			projectInfo.NeedsDisambiguation = true
			projectInfo.Action = "needs_clarification"
		}
	}

	analysis.ProjectComponent = projectInfo
	return nil
}

// proceedWithResolution determines next step and proceeds
func (we *EnhancedWorkflowEngine) proceedWithResolution(
	ctx *erp_modules.ModuleContext,
	workflow *EnhancedWorkflowState,
) (*erp_modules.ModuleResponse, error) {

	workflow.Phase = PhaseResolution

	// Find first component that needs disambiguation
	if workflow.ComponentAnalysis.TaskComponent != nil && workflow.ComponentAnalysis.TaskComponent.NeedsDisambiguation {
		workflow.CurrentComponent = ComponentTypeTask
		return we.handleTaskDisambiguation(ctx, workflow)
	}

	if workflow.ComponentAnalysis.EmployeeComponent != nil && workflow.ComponentAnalysis.EmployeeComponent.NeedsDisambiguation {
		workflow.CurrentComponent = ComponentTypeEmployee
		return we.handleEmployeeDisambiguation(ctx, workflow)
	}

	if workflow.ComponentAnalysis.ProjectComponent != nil && workflow.ComponentAnalysis.ProjectComponent.NeedsDisambiguation {
		workflow.CurrentComponent = ComponentTypeProject
		return we.handleProjectDisambiguation(ctx, workflow)
	}

	// All components resolved, proceed to confirmation
	return we.proceedToConfirmation(ctx, workflow)
}

// ProcessWorkflowMessage processes user messages in active workflow
func (we *EnhancedWorkflowEngine) ProcessWorkflowMessage(
	ctx *erp_modules.ModuleContext,
	message string,
) (*erp_modules.ModuleResponse, error) {

	workflow, exists := we.activeWorkflows[ctx.User.Id]
	if !exists {
		return nil, nil
	}

	// Handle special commands
	if we.handleSpecialCommands(ctx, workflow, message) {
		return we.proceedWithResolution(ctx, workflow)
	}

	// Check if this is a comprehensive modification request
	if workflow.Phase == PhaseConfirmation {
		if modificationResponse := we.handleComprehensiveModification(ctx, workflow, message); modificationResponse != nil {
			return modificationResponse, nil
		}
	}

	// Handle current component resolution
	switch workflow.CurrentComponent {
	case ComponentTypeTask:
		return we.processTaskDisambiguationResponse(ctx, workflow, message)
	case ComponentTypeEmployee:
		return we.processEmployeeDisambiguationResponse(ctx, workflow, message)
	case ComponentTypeProject:
		return we.processProjectDisambiguationResponse(ctx, workflow, message)
	default:
		// In confirmation phase
		return we.processConfirmationResponse(ctx, workflow, message)
	}
}

// handleComprehensiveModification handles complex modification requests
func (we *EnhancedWorkflowEngine) handleComprehensiveModification(
	ctx *erp_modules.ModuleContext,
	workflow *EnhancedWorkflowState,
	message string,
) *erp_modules.ModuleResponse {

	// Parse comprehensive modification using enhanced LLM prompt
	modifications, err := we.parseComprehensiveModification(ctx, workflow, message)
	if err != nil || len(modifications) == 0 {
		return nil // Not a modification request
	}

	// Apply all modifications to completed data
	for key, value := range modifications {
		workflow.CompletedData[key] = value
	}

	// Re-run comprehensive analysis with new data
	response, err := we.performComprehensiveAnalysis(ctx, workflow)
	if err != nil {
		we.module.api.LogError("Failed to re-analyze after modification", "error", err.Error())
		return nil
	}

	return response
}

// parseComprehensiveModification parses complex modification requests
func (we *EnhancedWorkflowEngine) parseComprehensiveModification(
	ctx *erp_modules.ModuleContext,
	workflow *EnhancedWorkflowState,
	message string,
) (map[string]interface{}, error) {

	// Create enhanced LLM context for comprehensive modification analysis
	llmContext := &llm.Context{
		RequestingUser: ctx.User,
		Time:           time.Now().Format(time.RFC1123),
	}

	isVietnamese := detectUserLanguage(ctx.User)

	llmContext.Parameters = map[string]interface{}{
		"UserMessage":  message,
		"CurrentData":  workflow.CompletedData,
		"EntityType":   workflow.EntityType,
		"IsVietnamese": isVietnamese,
		"WorkflowType": workflow.WorkflowType,
	}

	// Use enhanced prompt for comprehensive modification analysis
	systemPrompt, err := we.module.prompts.Format("comprehensive_modification_analysis", llmContext)
	if err != nil {
		// Fallback to existing modification analysis
		response, fallbackErr := we.module.parseConfirmationResponse(ctx, message, workflow.CompletedData)
		if fallbackErr != nil {
			return nil, fallbackErr
		}
		if response.Intent == "modify" {
			return response.Modifications, nil
		}
		return nil, nil
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
	response, err := we.module.getLLM().ChatCompletionNoStream(completionRequest, llm.WithMaxGeneratedTokens(400))
	if err != nil {
		return nil, fmt.Errorf("failed to analyze comprehensive modification with LLM: %w", err)
	}

	// Parse JSON response
	var modificationResponse ConfirmationResponse
	response = strings.TrimSpace(response)
	start := strings.Index(response, "{")
	end := strings.LastIndex(response, "}") + 1

	if start == -1 || end <= start {
		return nil, fmt.Errorf("no valid JSON found in LLM response: %s", response)
	}

	jsonStr := response[start:end]
	if err := json.Unmarshal([]byte(jsonStr), &modificationResponse); err != nil {
		return nil, fmt.Errorf("failed to parse LLM comprehensive modification response as JSON: %w", err)
	}

	if modificationResponse.Intent == "modify" {
		return modificationResponse.Modifications, nil
	}

	return nil, nil
}

// handleTaskDisambiguation handles task disambiguation
func (we *EnhancedWorkflowEngine) handleTaskDisambiguation(
	ctx *erp_modules.ModuleContext,
	workflow *EnhancedWorkflowState,
) (*erp_modules.ModuleResponse, error) {

	taskInfo := workflow.ComponentAnalysis.TaskComponent
	if taskInfo == nil || !taskInfo.NeedsDisambiguation {
		return we.proceedToNextComponent(ctx, workflow)
	}

	// Convert Task to interface{} for generateExistingEntityDisambiguationMessage
	var existingEntities []interface{}
	for _, task := range taskInfo.ExistingTasks {
		existingEntities = append(existingEntities, task)
	}

	// Generate disambiguation message
	message, err := we.module.generateExistingEntityDisambiguationMessage(ctx, workflow.EntityType, existingEntities)
	if err != nil {
		return nil, fmt.Errorf("failed to generate task disambiguation message: %w", err)
	}

	return &erp_modules.ModuleResponse{
		Success:     true,
		Message:     message,
		ActionTaken: "workflow_task_disambiguation",
		Data: map[string]interface{}{
			"workflow_active": true,
			"component_type":  string(ComponentTypeTask),
			"entity_count":    len(existingEntities),
		},
	}, nil
}

// processTaskDisambiguationResponse processes task disambiguation response
func (we *EnhancedWorkflowEngine) processTaskDisambiguationResponse(
	ctx *erp_modules.ModuleContext,
	workflow *EnhancedWorkflowState,
	message string,
) (*erp_modules.ModuleResponse, error) {

	taskInfo := workflow.ComponentAnalysis.TaskComponent
	if taskInfo == nil {
		return nil, fmt.Errorf("missing task component info")
	}

	// Parse response
	response, err := we.module.parseExistingEntityDisambiguationResponse(ctx, message, workflow.EntityType, len(taskInfo.ExistingTasks))
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
		taskInfo.Status = StatusResolved
		taskInfo.Action = "create_new"
		return we.proceedToNextComponent(ctx, workflow)

	case "use_existing":
		if response.SelectedIndex < 1 || response.SelectedIndex > len(taskInfo.ExistingTasks) {
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

		// Store selected existing entity for later use
		selectedTask := taskInfo.ExistingTasks[response.SelectedIndex-1]
		taskInfo.ResolutionData = map[string]interface{}{
			"selected_existing_entity": selectedTask,
			"action":                   "use_existing",
		}
		taskInfo.Status = StatusResolved
		taskInfo.Action = "use_existing"

		return we.proceedToNextComponent(ctx, workflow)

	case "cancel":
		we.cancelWorkflow(ctx.User.Id)
		isVietnamese := detectUserLanguage(ctx.User)
		cancelMsg := "Đã hủy bỏ yêu cầu."
		if !isVietnamese {
			cancelMsg = "Request cancelled."
		}
		return &erp_modules.ModuleResponse{
			Success:     true,
			Message:     cancelMsg,
			ActionTaken: "workflow_cancelled",
		}, nil
	}

	return nil, fmt.Errorf("unhandled intent: %s", response.Intent)
}

// handleEmployeeDisambiguation handles employee disambiguation
func (we *EnhancedWorkflowEngine) handleEmployeeDisambiguation(
	ctx *erp_modules.ModuleContext,
	workflow *EnhancedWorkflowState,
) (*erp_modules.ModuleResponse, error) {

	employeeInfo := workflow.ComponentAnalysis.EmployeeComponent
	if employeeInfo == nil || !employeeInfo.NeedsDisambiguation {
		return we.proceedToNextComponent(ctx, workflow)
	}

	// Create assignee result for message generation
	assigneeResult := &AssigneeResolutionResult{
		ResolvedEmployees:         employeeInfo.ResolvedEmployees,
		UnresolvedEmployeeMatches: employeeInfo.UnresolvedMatches,
		RequiresDisambiguation:    true,
	}

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
			"component_type":   string(ComponentTypeEmployee),
			"unresolved_count": len(employeeInfo.UnresolvedMatches),
			"resolved_count":   len(employeeInfo.ResolvedEmployees),
		},
	}, nil
}

// processEmployeeDisambiguationResponse processes employee disambiguation response
func (we *EnhancedWorkflowEngine) processEmployeeDisambiguationResponse(
	ctx *erp_modules.ModuleContext,
	workflow *EnhancedWorkflowState,
	message string,
) (*erp_modules.ModuleResponse, error) {

	employeeInfo := workflow.ComponentAnalysis.EmployeeComponent
	if employeeInfo == nil {
		return nil, fmt.Errorf("missing employee component info")
	}

	// Parse disambiguation response
	response, err := we.module.parseMultiEmployeeDisambiguationResponse(ctx, message, employeeInfo.UnresolvedMatches)
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
		selectedEmployees, err := we.module.resolveDisambiguatedEmployees(employeeInfo.UnresolvedMatches, response.SelectedIndexes)
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
		allResolvedEmployees := append(employeeInfo.ResolvedEmployees, selectedEmployees...)
		employeeInfo.ResolvedEmployees = allResolvedEmployees
		employeeInfo.Status = StatusResolved

		// Update completed data
		workflow.CompletedData["assigned_to_employees"] = allResolvedEmployees

		return we.proceedToNextComponent(ctx, workflow)

	case "cancel":
		we.cancelWorkflow(ctx.User.Id)
		isVietnamese := detectUserLanguage(ctx.User)
		cancelMsg := "Đã hủy bỏ yêu cầu."
		if !isVietnamese {
			cancelMsg = "Request cancelled."
		}
		return &erp_modules.ModuleResponse{
			Success:     true,
			Message:     cancelMsg,
			ActionTaken: "workflow_cancelled",
		}, nil
	}

	return nil, fmt.Errorf("unhandled intent: %s", response.Intent)
}

// handleProjectDisambiguation handles project disambiguation
func (we *EnhancedWorkflowEngine) handleProjectDisambiguation(
	ctx *erp_modules.ModuleContext,
	workflow *EnhancedWorkflowState,
) (*erp_modules.ModuleResponse, error) {

	projectInfo := workflow.ComponentAnalysis.ProjectComponent
	if projectInfo == nil || !projectInfo.NeedsDisambiguation {
		return we.proceedToNextComponent(ctx, workflow)
	}

	// Generate project selection message
	message, err := we.module.generateProjectTaskDisambiguationMessage(ctx, projectInfo.MatchingProjects)
	if err != nil {
		return nil, fmt.Errorf("failed to generate project disambiguation message: %w", err)
	}

	return &erp_modules.ModuleResponse{
		Success:     true,
		Message:     message,
		ActionTaken: "workflow_project_disambiguation",
		Data: map[string]interface{}{
			"workflow_active": true,
			"component_type":  string(ComponentTypeProject),
			"project_count":   len(projectInfo.MatchingProjects),
		},
	}, nil
}

// processProjectDisambiguationResponse processes project disambiguation response
func (we *EnhancedWorkflowEngine) processProjectDisambiguationResponse(
	ctx *erp_modules.ModuleContext,
	workflow *EnhancedWorkflowState,
	message string,
) (*erp_modules.ModuleResponse, error) {

	projectInfo := workflow.ComponentAnalysis.ProjectComponent
	if projectInfo == nil {
		return nil, fmt.Errorf("missing project component info")
	}

	// Parse selection response
	response, err := we.module.parseProjectSelectionResponse(ctx, message, projectInfo.MatchingProjects)
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
		if response.SelectedIndex < 1 || response.SelectedIndex > len(projectInfo.MatchingProjects) {
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
		selectedProject := projectInfo.MatchingProjects[response.SelectedIndex-1]
		projectInfo.SelectedProject = selectedProject.Name
		projectInfo.Status = StatusResolved
		workflow.CompletedData["project"] = selectedProject.Name

		return we.proceedToNextComponent(ctx, workflow)

	case "no_project":
		// Clear project and proceed
		projectInfo.Status = StatusResolved
		projectInfo.Action = "no_project"
		workflow.CompletedData["project"] = ""

		return we.proceedToNextComponent(ctx, workflow)

	case "cancel":
		we.cancelWorkflow(ctx.User.Id)
		isVietnamese := detectUserLanguage(ctx.User)
		cancelMsg := "Đã hủy bỏ yêu cầu."
		if !isVietnamese {
			cancelMsg = "Request cancelled."
		}
		return &erp_modules.ModuleResponse{
			Success:     true,
			Message:     cancelMsg,
			ActionTaken: "workflow_cancelled",
		}, nil
	}

	return nil, fmt.Errorf("unhandled intent: %s", response.Intent)
}

// proceedToNextComponent moves to next component or confirmation
func (we *EnhancedWorkflowEngine) proceedToNextComponent(
	ctx *erp_modules.ModuleContext,
	workflow *EnhancedWorkflowState,
) (*erp_modules.ModuleResponse, error) {

	// Check if there are more components that need disambiguation
	if workflow.ComponentAnalysis.TaskComponent != nil &&
		workflow.ComponentAnalysis.TaskComponent.NeedsDisambiguation &&
		workflow.ComponentAnalysis.TaskComponent.Status == StatusPending {
		workflow.CurrentComponent = ComponentTypeTask
		return we.handleTaskDisambiguation(ctx, workflow)
	}

	if workflow.ComponentAnalysis.EmployeeComponent != nil &&
		workflow.ComponentAnalysis.EmployeeComponent.NeedsDisambiguation &&
		workflow.ComponentAnalysis.EmployeeComponent.Status == StatusPending {
		workflow.CurrentComponent = ComponentTypeEmployee
		return we.handleEmployeeDisambiguation(ctx, workflow)
	}

	if workflow.ComponentAnalysis.ProjectComponent != nil &&
		workflow.ComponentAnalysis.ProjectComponent.NeedsDisambiguation &&
		workflow.ComponentAnalysis.ProjectComponent.Status == StatusPending {
		workflow.CurrentComponent = ComponentTypeProject
		return we.handleProjectDisambiguation(ctx, workflow)
	}

	// All components resolved, proceed to confirmation
	return we.proceedToConfirmation(ctx, workflow)
}

// proceedToConfirmation moves to confirmation phase
func (we *EnhancedWorkflowEngine) proceedToConfirmation(
	ctx *erp_modules.ModuleContext,
	workflow *EnhancedWorkflowState,
) (*erp_modules.ModuleResponse, error) {

	workflow.Phase = PhaseConfirmation
	workflow.CurrentComponent = ""

	// Apply any resolved component data to completed data
	we.applyResolvedComponentData(workflow)

	// Generate comprehensive confirmation message
	message, err := we.generateComprehensiveConfirmation(ctx, workflow)
	if err != nil {
		return nil, fmt.Errorf("failed to generate comprehensive confirmation: %w", err)
	}

	return &erp_modules.ModuleResponse{
		Success:     true,
		Message:     message,
		ActionTaken: "workflow_comprehensive_confirmation",
		Data: map[string]interface{}{
			"workflow_active": true,
			"phase":           string(PhaseConfirmation),
		},
	}, nil
}

// applyResolvedComponentData applies component resolution data to completed data
func (we *EnhancedWorkflowEngine) applyResolvedComponentData(workflow *EnhancedWorkflowState) {
	// Apply employee resolutions
	if workflow.ComponentAnalysis.EmployeeComponent != nil &&
		workflow.ComponentAnalysis.EmployeeComponent.Status == StatusResolved {
		workflow.CompletedData["assigned_to_employees"] = workflow.ComponentAnalysis.EmployeeComponent.ResolvedEmployees
	}

	// Apply project resolutions
	if workflow.ComponentAnalysis.ProjectComponent != nil &&
		workflow.ComponentAnalysis.ProjectComponent.Status == StatusResolved {
		if workflow.ComponentAnalysis.ProjectComponent.SelectedProject != "" {
			workflow.CompletedData["project"] = workflow.ComponentAnalysis.ProjectComponent.SelectedProject
		}
	}

	// Task component data is already in CompletedData, but check for use_existing action
	if workflow.ComponentAnalysis.TaskComponent != nil &&
		workflow.ComponentAnalysis.TaskComponent.Action == "use_existing" &&
		workflow.ComponentAnalysis.TaskComponent.ResolutionData != nil {
		// Store information about using existing entity for later execution
		workflow.CompletedData["_use_existing_entity"] = workflow.ComponentAnalysis.TaskComponent.ResolutionData["selected_existing_entity"]
		workflow.CompletedData["_use_existing_action"] = true
	}
}

// generateComprehensiveConfirmation generates comprehensive confirmation message
func (we *EnhancedWorkflowEngine) generateComprehensiveConfirmation(
	ctx *erp_modules.ModuleContext,
	workflow *EnhancedWorkflowState,
) (string, error) {

	isVietnamese := detectUserLanguage(ctx.User)
	var message strings.Builder

	// Check if using existing entity
	if useExisting, ok := workflow.CompletedData["_use_existing_action"].(bool); ok && useExisting {
		// This is assignment to existing entity
		if existingEntity, ok := workflow.CompletedData["_use_existing_entity"]; ok {
			return we.generateExistingEntityAssignmentConfirmation(ctx, workflow, existingEntity, isVietnamese)
		}
	}

	// Regular new entity creation confirmation
	if isVietnamese {
		if workflow.EntityType == "project" {
			message.WriteString("**Xác nhận thông tin dự án mới:**\n\n")
		} else {
			message.WriteString("**Xác nhận thông tin task mới:**\n\n")
		}
	} else {
		if workflow.EntityType == "project" {
			message.WriteString("**Confirm New Project Information:**\n\n")
		} else {
			message.WriteString("**Confirm New Task Information:**\n\n")
		}
	}

	// Display all confirmed information
	for key, value := range workflow.CompletedData {
		if strings.HasPrefix(key, "_") {
			continue // Skip internal fields
		}

		if value == nil || value == "" {
			continue
		}

		// Handle special fields
		switch key {
		case "assigned_to_employees":
			if employees, ok := value.([]AssignedEmployee); ok && len(employees) > 0 {
				var names []string
				for _, emp := range employees {
					names = append(names, emp.EmployeeName)
				}
				if isVietnamese {
					message.WriteString(fmt.Sprintf("• **Phân công cho:** %s\n", strings.Join(names, ", ")))
				} else {
					message.WriteString(fmt.Sprintf("• **Assigned to:** %s\n", strings.Join(names, ", ")))
				}
			}
		case "project_name":
			if isVietnamese {
				message.WriteString(fmt.Sprintf("• **Tên dự án:** %v\n", value))
			} else {
				message.WriteString(fmt.Sprintf("• **Project Name:** %v\n", value))
			}
		case "subject":
			if isVietnamese {
				message.WriteString(fmt.Sprintf("• **Tên task:** %v\n", value))
			} else {
				message.WriteString(fmt.Sprintf("• **Task Name:** %v\n", value))
			}
		case "project":
			if isVietnamese {
				message.WriteString(fmt.Sprintf("• **Thuộc dự án:** %v\n", value))
			} else {
				message.WriteString(fmt.Sprintf("• **Project:** %v\n", value))
			}
		case "priority":
			if isVietnamese {
				message.WriteString(fmt.Sprintf("• **Độ ưu tiên:** %v\n", value))
			} else {
				message.WriteString(fmt.Sprintf("• **Priority:** %v\n", value))
			}
		case "description":
			if isVietnamese {
				message.WriteString(fmt.Sprintf("• **Mô tả:** %v\n", value))
			} else {
				message.WriteString(fmt.Sprintf("• **Description:** %v\n", value))
			}
		case "expected_start_date", "exp_start_date":
			if isVietnamese {
				message.WriteString(fmt.Sprintf("• **Ngày bắt đầu:** %v\n", value))
			} else {
				message.WriteString(fmt.Sprintf("• **Start Date:** %v\n", value))
			}
		case "expected_end_date", "exp_end_date":
			if isVietnamese {
				message.WriteString(fmt.Sprintf("• **Ngày kết thúc:** %v\n", value))
			} else {
				message.WriteString(fmt.Sprintf("• **End Date:** %v\n", value))
			}
		default:
			// Format other fields generically
			if str, ok := value.(string); ok && str != "" {
				fieldName := strings.Title(strings.ReplaceAll(key, "_", " "))
				message.WriteString(fmt.Sprintf("• **%s:** %v\n", fieldName, value))
			}
		}
	}

	message.WriteString("\n")

	if isVietnamese {
		message.WriteString("Vui lòng xác nhận thông tin trên có chính xác không? Hoặc có thể yêu cầu chỉnh sửa thêm.\n")
	} else {
		message.WriteString("Please confirm if the above information is correct. You can request modifications if needed.\n")
	}

	return message.String(), nil
}

// generateExistingEntityAssignmentConfirmation generates confirmation for existing entity assignment
func (we *EnhancedWorkflowEngine) generateExistingEntityAssignmentConfirmation(
	ctx *erp_modules.ModuleContext,
	workflow *EnhancedWorkflowState,
	existingEntity interface{},
	isVietnamese bool,
) (string, error) {

	var message strings.Builder
	var entityName, entityID string

	// Extract entity information
	if workflow.EntityType == "project" {
		if entityBytes, err := json.Marshal(existingEntity); err == nil {
			var project Project
			if json.Unmarshal(entityBytes, &project) == nil {
				entityName = project.ProjectName
				entityID = project.Name
			}
		}
	} else {
		if entityBytes, err := json.Marshal(existingEntity); err == nil {
			var task Task
			if json.Unmarshal(entityBytes, &task) == nil {
				entityName = task.Subject
				entityID = task.Name
			}
		}
	}

	if isVietnamese {
		if workflow.EntityType == "project" {
			message.WriteString("**Xác nhận phân công dự án có sẵn:**\n\n")
			message.WriteString(fmt.Sprintf("• **Dự án:** %s (ID: %s)\n", entityName, entityID))
		} else {
			message.WriteString("**Xác nhận phân công task có sẵn:**\n\n")
			message.WriteString(fmt.Sprintf("• **Task:** %s (ID: %s)\n", entityName, entityID))
		}
	} else {
		if workflow.EntityType == "project" {
			message.WriteString("**Confirm Existing Project Assignment:**\n\n")
			message.WriteString(fmt.Sprintf("• **Project:** %s (ID: %s)\n", entityName, entityID))
		} else {
			message.WriteString("**Confirm Existing Task Assignment:**\n\n")
			message.WriteString(fmt.Sprintf("• **Task:** %s (ID: %s)\n", entityName, entityID))
		}
	}

	// Show assigned employees
	if employees, ok := workflow.CompletedData["assigned_to_employees"].([]AssignedEmployee); ok && len(employees) > 0 {
		var names []string
		for _, emp := range employees {
			names = append(names, emp.EmployeeName)
		}
		if isVietnamese {
			message.WriteString(fmt.Sprintf("• **Phân công cho:** %s\n", strings.Join(names, ", ")))
		} else {
			message.WriteString(fmt.Sprintf("• **Assigned to:** %s\n", strings.Join(names, ", ")))
		}
	}

	message.WriteString("\n")

	if isVietnamese {
		message.WriteString("Xác nhận phân công này?\n")
	} else {
		message.WriteString("Confirm this assignment?\n")
	}

	return message.String(), nil
}

// processConfirmationResponse processes final confirmation response
func (we *EnhancedWorkflowEngine) processConfirmationResponse(
	ctx *erp_modules.ModuleContext,
	workflow *EnhancedWorkflowState,
	message string,
) (*erp_modules.ModuleResponse, error) {

	// Parse confirmation response
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
		// Execute final action
		return we.executeFinalAction(ctx, workflow)

	case "modify":
		// Apply modifications and re-analyze
		for key, value := range response.Modifications {
			workflow.CompletedData[key] = value
		}

		// Re-run comprehensive analysis
		return we.performComprehensiveAnalysis(ctx, workflow)

	case "cancel":
		we.cancelWorkflow(ctx.User.Id)
		isVietnamese := detectUserLanguage(ctx.User)
		cancelMsg := "Đã hủy bỏ yêu cầu."
		if !isVietnamese {
			cancelMsg = "Request cancelled."
		}
		return &erp_modules.ModuleResponse{
			Success:     true,
			Message:     cancelMsg,
			ActionTaken: "workflow_cancelled",
		}, nil
	}

	return nil, fmt.Errorf("unhandled intent: %s", response.Intent)
}

// executeFinalAction executes the final creation or assignment action
func (we *EnhancedWorkflowEngine) executeFinalAction(
	ctx *erp_modules.ModuleContext,
	workflow *EnhancedWorkflowState,
) (*erp_modules.ModuleResponse, error) {

	defer we.cancelWorkflow(ctx.User.Id)

	workflow.Phase = PhaseExecution

	// Check if this is assignment to existing entity
	if useExisting, ok := workflow.CompletedData["_use_existing_action"].(bool); ok && useExisting {
		if existingEntity, ok := workflow.CompletedData["_use_existing_entity"]; ok {
			return we.executeExistingEntityAssignment(ctx, workflow, existingEntity)
		}
	}

	// Regular new entity creation
	switch workflow.WorkflowType {
	case "create_project":
		return we.executeProjectCreation(ctx, workflow)
	case "create_task":
		return we.executeTaskCreation(ctx, workflow)
	}

	return nil, fmt.Errorf("unknown workflow type: %s", workflow.WorkflowType)
}

// executeExistingEntityAssignment assigns to existing entity
func (we *EnhancedWorkflowEngine) executeExistingEntityAssignment(
	ctx *erp_modules.ModuleContext,
	workflow *EnhancedWorkflowState,
	existingEntity interface{},
) (*erp_modules.ModuleResponse, error) {

	// Extract assigned employees
	var assignedEmployees []AssignedEmployee
	if employees, ok := workflow.CompletedData["assigned_to_employees"].([]AssignedEmployee); ok {
		assignedEmployees = employees
	}

	// Get entity information
	var entityID, entityName string
	if workflow.EntityType == "project" {
		if entityBytes, err := json.Marshal(existingEntity); err == nil {
			var project Project
			if json.Unmarshal(entityBytes, &project) == nil {
				entityID = project.Name
				entityName = project.ProjectName
			}
		}
	} else {
		if entityBytes, err := json.Marshal(existingEntity); err == nil {
			var task Task
			if json.Unmarshal(entityBytes, &task) == nil {
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
				successMsg = fmt.Sprintf("Đã chọn dự án có sẵn: **%s**. Tuy nhiên, không có nhân viên nào được chỉ định để phân công.", entityName)
			} else {
				successMsg = fmt.Sprintf("Đã chọn task có sẵn: **%s**. Tuy nhiên, không có nhân viên nào được chỉ định để phân công.", entityName)
			}
		} else {
			if workflow.EntityType == "project" {
				successMsg = fmt.Sprintf("Selected existing project: **%s**. However, no employees were specified for assignment.", entityName)
			} else {
				successMsg = fmt.Sprintf("Selected existing task: **%s**. However, no employees were specified for assignment.", entityName)
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
				successMsg = fmt.Sprintf("Đã phân công dự án có sẵn **%s** cho **%s**!", entityName, assigneesStr)
			} else {
				successMsg = fmt.Sprintf("Đã phân công task có sẵn **%s** cho **%s**!", entityName, assigneesStr)
			}
		} else {
			if workflow.EntityType == "project" {
				successMsg = fmt.Sprintf("Successfully assigned existing project **%s** to **%s**!", entityName, assigneesStr)
			} else {
				successMsg = fmt.Sprintf("Successfully assigned existing task **%s** to **%s**!", entityName, assigneesStr)
			}
		}
	} else {
		if isVietnamese {
			if workflow.EntityType == "project" {
				successMsg = fmt.Sprintf("Đã phân công dự án có sẵn **%s**. Thành công: **%s**. Lỗi: **%s**.",
					entityName, assigneesStr, strings.Join(assignmentErrors, ", "))
			} else {
				successMsg = fmt.Sprintf("Đã phân công task có sẵn **%s**. Thành công: **%s**. Lỗi: **%s**.",
					entityName, assigneesStr, strings.Join(assignmentErrors, ", "))
			}
		} else {
			if workflow.EntityType == "project" {
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

// executeProjectCreation executes new project creation
func (we *EnhancedWorkflowEngine) executeProjectCreation(
	ctx *erp_modules.ModuleContext,
	workflow *EnhancedWorkflowState,
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
				successMsg = fmt.Sprintf("Đã tạo dự án thành công: **%s** và phân công cho **%s**!", projectID, assigneesStr)
			} else {
				successMsg = fmt.Sprintf("Successfully created project: **%s** and assigned to **%s**!", projectID, assigneesStr)
			}
		} else {
			if isVietnamese {
				successMsg = fmt.Sprintf("Đã tạo dự án: **%s**. Phân công thành công cho **%s**. Lỗi phân công: **%s**.",
					projectID, assigneesStr, strings.Join(assignmentErrors, ", "))
			} else {
				successMsg = fmt.Sprintf("Created project: **%s**. Successfully assigned to **%s**. Assignment failed for: **%s**.",
					projectID, assigneesStr, strings.Join(assignmentErrors, ", "))
			}
		}
	} else {
		if isVietnamese {
			successMsg = fmt.Sprintf("Đã tạo dự án thành công: **%s**!", projectID)
		} else {
			successMsg = fmt.Sprintf("Successfully created project: **%s**!", projectID)
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

// executeTaskCreation executes new task creation
func (we *EnhancedWorkflowEngine) executeTaskCreation(
	ctx *erp_modules.ModuleContext,
	workflow *EnhancedWorkflowState,
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
				successMsg = fmt.Sprintf("Đã tạo task thành công: **%s** và phân công cho **%s**!", taskID, assigneesStr)
			} else {
				successMsg = fmt.Sprintf("Successfully created task: **%s** and assigned to **%s**!", taskID, assigneesStr)
			}
		} else {
			if isVietnamese {
				successMsg = fmt.Sprintf("Đã tạo task: **%s**. Phân công thành công cho **%s**. Lỗi phân công: **%s**.",
					taskID, assigneesStr, strings.Join(assignmentErrors, ", "))
			} else {
				successMsg = fmt.Sprintf("Created task: **%s**. Successfully assigned to **%s**. Assignment failed for: **%s**.",
					taskID, assigneesStr, strings.Join(assignmentErrors, ", "))
			}
		}
	} else {
		if isVietnamese {
			successMsg = fmt.Sprintf("Đã tạo task thành công: **%s**!", taskID)
		} else {
			successMsg = fmt.Sprintf("Successfully created task: **%s**!", taskID)
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

// Helper methods

// cancelWorkflow cancels and cleans up a workflow
func (we *EnhancedWorkflowEngine) cancelWorkflow(userID string) {
	delete(we.activeWorkflows, userID)
}

// handleSpecialCommands handles special workflow commands
func (we *EnhancedWorkflowEngine) handleSpecialCommands(
	ctx *erp_modules.ModuleContext,
	workflow *EnhancedWorkflowState,
	message string,
) bool {
	messageLower := strings.ToLower(strings.TrimSpace(message))

	switch messageLower {
	case "cancel", "hủy", "stop", "dừng":
		we.cancelWorkflow(ctx.User.Id)
		return true
	case "restart", "bắt đầu lại", "làm lại":
		// Reset workflow to analysis phase
		workflow.Phase = PhaseAnalysis
		workflow.CurrentComponent = ""
		workflow.ComponentAnalysis = nil
		return true
	}

	return false
}

// HasActiveWorkflow checks if user has active workflow
func (we *EnhancedWorkflowEngine) HasActiveWorkflow(userID string) bool {
	_, exists := we.activeWorkflows[userID]
	return exists
}

// GetActiveWorkflow gets active workflow for user
func (we *EnhancedWorkflowEngine) GetActiveWorkflow(userID string) (*EnhancedWorkflowState, bool) {
	workflow, exists := we.activeWorkflows[userID]
	return workflow, exists
}
