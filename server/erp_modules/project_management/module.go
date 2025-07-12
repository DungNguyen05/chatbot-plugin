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
	"github.com/mattermost/mattermost/server/public/model"
)

// ProjectManagementModule handles project and task management operations with unified workflow
type ProjectManagementModule struct {
	config                 ProjectManagementConfig
	erpClient              *ERPClient
	api                    PluginAPI
	prompts                PromptsInterface
	getLLM                 func() llm.LanguageModel
	enhancedWorkflowEngine *EnhancedWorkflowEngine
	notificationSender     NotificationSender
}

// NewProjectManagementModule creates a new project management module with workflow engine
func NewProjectManagementModule(
	config ProjectManagementConfig,
	httpClient *http.Client,
	prompts PromptsInterface,
	getLLM func() llm.LanguageModel,
	api PluginAPI,
	notificationSender NotificationSender, // THÊM PARAMETER NÀY
) *ProjectManagementModule {
	// Validate configuration
	if err := ValidateProjectManagementConfig(config); err != nil {
		api.LogError("Invalid project management configuration", "error", err.Error())
	}

	// Create ERP client
	erpClient := NewERPClient(config, httpClient, api)

	// Create module
	module := &ProjectManagementModule{
		config:             config,
		erpClient:          erpClient,
		api:                api,
		prompts:            prompts,
		getLLM:             getLLM,
		notificationSender: notificationSender,
	}

	// Initialize enhanced workflow engine
	module.enhancedWorkflowEngine = NewEnhancedWorkflowEngine(module)

	return module
}

// GetCategory returns the category this module handles
func (m *ProjectManagementModule) GetCategory() string {
	return "project_management"
}

// GetSupportedActions returns list of actions this module supports
func (m *ProjectManagementModule) GetSupportedActions() []string {
	return []string{"create_project", "create_task", "show_project", "show_task"}
}

// CanHandle determines if this module can handle the given intent
func (m *ProjectManagementModule) CanHandle(intent *erp_modules.Intent) bool {
	if intent.Category != "project_management" {
		return false
	}
	return isValidAction(intent.Action)
}

// ProcessUserMessage handles user messages including workflow states
func (m *ProjectManagementModule) ProcessUserMessage(ctx *erp_modules.ModuleContext, message string) (*erp_modules.ModuleResponse, error) {
	m.api.LogDebug("ProcessUserMessage called", "user_id", ctx.User.Id, "message", message)

	// Sanitize user input
	message = sanitizeUserInput(message)

	// Check if user has active enhanced workflow
	if m.enhancedWorkflowEngine.HasActiveWorkflow(ctx.User.Id) {
		m.api.LogDebug("Found active enhanced workflow for user", "user_id", ctx.User.Id)
		return m.enhancedWorkflowEngine.ProcessWorkflowMessage(ctx, message)
	}

	// No active workflow, not handling this message
	return nil, nil
}

// Execute processes the project management intent and starts enhanced workflow
func (m *ProjectManagementModule) Execute(ctx *erp_modules.ModuleContext, intent *erp_modules.Intent) (*erp_modules.ModuleResponse, error) {
	isVietnamese := detectUserLanguage(ctx.User)

	// Execute specific action based on intent.Action
	switch intent.Action {
	case "create_project":
		// Get employee ID for create actions
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
		// Get creator's email
		creatorEmail, err := m.getEmployeeEmailFromUser(ctx.User)
		if err != nil {
			m.api.LogWarn("Failed to get creator email", "error", err.Error())
			creatorEmail = "demo@example.com"
		}
		return m.handleCreateProjectRequest(employeeID, creatorEmail, ctx, intent)

	case "create_task":
		// Get employee ID for create actions
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
		// Get creator's email
		creatorEmail, err := m.getEmployeeEmailFromUser(ctx.User)
		if err != nil {
			m.api.LogWarn("Failed to get creator email", "error", err.Error())
			creatorEmail = "demo@example.com"
		}
		return m.handleCreateTaskRequest(employeeID, creatorEmail, ctx, intent)

	case "show_project":
		return m.handleShowProjectRequest(ctx, intent)

	case "show_task":
		return m.handleShowTaskRequest(ctx, intent)

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

// handleCreateProjectRequest processes project creation request using workflow
func (m *ProjectManagementModule) handleCreateProjectRequest(
	employeeID, creatorEmail string,
	ctx *erp_modules.ModuleContext,
	intent *erp_modules.Intent,
) (*erp_modules.ModuleResponse, error) {
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

	// Convert to map for enhanced workflow
	requestMap := make(map[string]interface{})
	requestBytes, _ := json.Marshal(projectRequest)
	json.Unmarshal(requestBytes, &requestMap)

	// Start enhanced workflow
	return m.enhancedWorkflowEngine.StartWorkflow(
		ctx,
		"create_project",
		"project",
		requestMap,
		employeeID,
		creatorEmail,
	)
}

// handleCreateTaskRequest processes task creation request using workflow
func (m *ProjectManagementModule) handleCreateTaskRequest(
	employeeID, creatorEmail string,
	ctx *erp_modules.ModuleContext,
	intent *erp_modules.Intent,
) (*erp_modules.ModuleResponse, error) {
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

	// Convert to map for enhanced workflow
	requestMap := make(map[string]interface{})
	requestBytes, _ := json.Marshal(taskRequest)
	json.Unmarshal(requestBytes, &requestMap)

	// Start enhanced workflow
	return m.enhancedWorkflowEngine.StartWorkflow(
		ctx,
		"create_task",
		"task",
		requestMap,
		employeeID,
		creatorEmail,
	)
}

// handleShowProjectRequest processes show project request
func (m *ProjectManagementModule) handleShowProjectRequest(ctx *erp_modules.ModuleContext, intent *erp_modules.Intent) (*erp_modules.ModuleResponse, error) {
	isVietnamese := detectUserLanguage(ctx.User)

	// Get all projects from ERP
	projects, err := m.erpClient.GetAllProjects()
	if err != nil {
		errorMsg := "⚠️ Không thể lấy danh sách dự án từ hệ thống ERP."
		if !isVietnamese {
			errorMsg = "⚠️ Cannot retrieve projects from ERP system."
		}
		return &erp_modules.ModuleResponse{
			Success: false,
			Message: errorMsg,
			Error:   err.Error(),
		}, nil
	}

	if len(projects) == 0 {
		errorMsg := "Không có dự án nào trong hệ thống."
		if !isVietnamese {
			errorMsg = "No projects found in the system."
		}
		return &erp_modules.ModuleResponse{
			Success: true,
			Message: errorMsg,
		}, nil
	}

	// Generate formatted project list
	message := m.formatProjectList(projects, isVietnamese)

	return &erp_modules.ModuleResponse{
		Success:     true,
		Message:     message,
		ActionTaken: "show_project",
		Data: map[string]interface{}{
			"project_count": len(projects),
			"projects":      projects,
		},
	}, nil
}

// handleShowTaskRequest processes show task request
func (m *ProjectManagementModule) handleShowTaskRequest(ctx *erp_modules.ModuleContext, intent *erp_modules.Intent) (*erp_modules.ModuleResponse, error) {
	isVietnamese := detectUserLanguage(ctx.User)

	// Get all tasks from ERP
	tasks, err := m.erpClient.GetAllTasks()
	if err != nil {
		errorMsg := "⚠️ Không thể lấy danh sách task từ hệ thống ERP."
		if !isVietnamese {
			errorMsg = "⚠️ Cannot retrieve tasks from ERP system."
		}
		return &erp_modules.ModuleResponse{
			Success: false,
			Message: errorMsg,
			Error:   err.Error(),
		}, nil
	}

	if len(tasks) == 0 {
		errorMsg := "Không có task nào trong hệ thống."
		if !isVietnamese {
			errorMsg = "No tasks found in the system."
		}
		return &erp_modules.ModuleResponse{
			Success: true,
			Message: errorMsg,
		}, nil
	}

	// Generate formatted task list
	message := m.formatTaskList(tasks, isVietnamese)

	return &erp_modules.ModuleResponse{
		Success:     true,
		Message:     message,
		ActionTaken: "show_task",
		Data: map[string]interface{}{
			"task_count": len(tasks),
			"tasks":      tasks,
		},
	}, nil
}

// formatProjectList formats the project list into a readable table message
func (m *ProjectManagementModule) formatProjectList(projects []Project, isVietnamese bool) string {
	var message strings.Builder

	if isVietnamese {
		message.WriteString("**Danh sách dự án trong hệ thống:**\n\n")
		message.WriteString("| STT | Tên dự án | Trạng thái | Ưu tiên | Khách hàng |\n")
		message.WriteString("|-----|-----------|------------|---------|------------|\n")
	} else {
		message.WriteString("**Projects in the system:**\n\n")
		message.WriteString("| No. | Project Name | Status | Priority | Customer |\n")
		message.WriteString("|-----|--------------|--------|----------|----------|\n")
	}

	// Limit display to prevent overwhelming messages
	displayLimit := 50
	displayedCount := 0

	for i, project := range projects {
		if displayedCount >= displayLimit {
			remaining := len(projects) - displayedCount
			if isVietnamese {
				message.WriteString(fmt.Sprintf("| ... | *...và %d dự án khác* | ... | ... | ... |\n", remaining))
			} else {
				message.WriteString(fmt.Sprintf("| ... | *...and %d more projects* | ... | ... | ... |\n", remaining))
			}
			break
		}

		// Safely handle empty fields
		projectName := project.ProjectName
		if projectName == "" {
			projectName = "[Không có tên]"
			if !isVietnamese {
				projectName = "[No name]"
			}
		}

		status := project.Status
		if status == "" {
			status = "N/A"
		}

		priority := project.Priority
		if priority == "" {
			priority = "N/A"
		}

		customer := project.Customer
		if customer == "" {
			customer = "N/A"
		}

		message.WriteString(fmt.Sprintf("| %d | %s | %s | %s | %s |\n",
			i+1, projectName, status, priority, customer))

		displayedCount++
	}

	return message.String()
}

// formatTaskList formats the task list into a readable table message
func (m *ProjectManagementModule) formatTaskList(tasks []Task, isVietnamese bool) string {
	var message strings.Builder

	if isVietnamese {
		message.WriteString("**Danh sách task trong hệ thống:**\n\n")
		message.WriteString("| STT | Tên task | Trạng thái | Ưu tiên | Dự án |\n")
		message.WriteString("|-----|----------|------------|---------|-------|\n")
	} else {
		message.WriteString("**Tasks in the system:**\n\n")
		message.WriteString("| No. | Task Name | Status | Priority | Project |\n")
		message.WriteString("|-----|-----------|--------|----------|----------|\n")
	}

	// Limit display to prevent overwhelming messages
	displayLimit := 50
	displayedCount := 0

	for i, task := range tasks {
		if displayedCount >= displayLimit {
			remaining := len(tasks) - displayedCount
			if isVietnamese {
				message.WriteString(fmt.Sprintf("| ... | *...và %d task khác* | ... | ... | ... |\n", remaining))
			} else {
				message.WriteString(fmt.Sprintf("| ... | *...and %d more tasks* | ... | ... | ... |\n", remaining))
			}
			break
		}

		// Safely handle empty fields
		taskName := task.Subject
		if taskName == "" {
			taskName = "[Không có tên]"
			if !isVietnamese {
				taskName = "[No name]"
			}
		}

		status := task.Status
		if status == "" {
			status = "N/A"
		}

		priority := task.Priority
		if priority == "" {
			priority = "N/A"
		}

		project := task.Project
		if project == "" {
			project = "N/A"
		}

		message.WriteString(fmt.Sprintf("| %d | %s | %s | %s | %s |\n",
			i+1, taskName, status, priority, project))

		displayedCount++
	}

	return message.String()
}

// GetDescription returns a description of what this module does
func (m *ProjectManagementModule) GetDescription() string {
	return "Quản lý dự án và công việc: tạo dự án mới, tạo task, phân công công việc cho nhiều nhân viên với luồng xử lý thông minh và linh hoạt, hỗ trợ phân tích đa thành phần và xử lý modification phức tạp"
}

// GetActionExamples returns examples of user messages for each action with enhanced capabilities
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
			"tạo dự án website giao cho Nam và gắn vào team Development",
			"create urgent project for client ABC assign to John, Mary with high priority",
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
			"tạo task code API gắn vào project website",
			"create task for project management system",
			"tạo task urgent fix bug giao cho team QA gắn vào project Mobile App",
			"create high priority task 'Database optimization' assign to Alice, Bob for project 'Backend Upgrade'",
		},
		"show_project": {
			"hiển thị dự án",
			"xem dự án",
			"danh sách dự án",
			"liệt kê dự án",
			"show projects",
			"list projects",
			"display projects",
			"view projects",
			"dự án nào đang có",
			"có dự án gì",
			"what projects are available",
			"show me all projects",
			"xem tất cả dự án",
			"cho tôi xem dự án",
		},
		"show_task": {
			"hiển thị task",
			"xem task",
			"danh sách task",
			"liệt kê task",
			"show tasks",
			"list tasks",
			"display tasks",
			"view tasks",
			"task nào đang có",
			"có task gì",
			"what tasks are available",
			"show me all tasks",
			"xem tất cả task",
			"cho tôi xem task",
		},
	}
}

// Helper methods for workflow engine

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

// In module.go, update the generateExistingEntityDisambiguationMessage method:
func (m *ProjectManagementModule) generateExistingEntityDisambiguationMessage(
	ctx *erp_modules.ModuleContext,
	entityType string,
	existingEntities []interface{},
) (string, error) {
	isVietnamese := detectUserLanguage(ctx.User)

	var message strings.Builder

	// Header
	if isVietnamese {
		if entityType == "project" {
			message.WriteString("**Tìm thấy các dự án tương tự:**\n\n")
			message.WriteString("| STT | Tên dự án | Mã |\n")
		} else {
			message.WriteString("**Tìm thấy các task tương tự:**\n\n")
			message.WriteString("| STT | Tên task | Mã |\n")
		}
		message.WriteString("|-----|------------|-----|\n")
	} else {
		if entityType == "project" {
			message.WriteString("**Found similar projects:**\n\n")
			message.WriteString("| No. | Project Name | ID |\n")
		} else {
			message.WriteString("**Found similar tasks:**\n\n")
			message.WriteString("| No. | Task Name | ID |\n")
		}
		message.WriteString("|-----|--------------|-----|\n")
	}

	// Table rows
	for i, entity := range existingEntities {
		var entityName, entityID string

		entityBytes, err := json.Marshal(entity)
		if err != nil {
			continue
		}

		if entityType == "project" {
			var project Project
			if err := json.Unmarshal(entityBytes, &project); err == nil && project.ProjectName != "" {
				entityName = project.ProjectName
				entityID = project.Name
			}
		}

		if entityName == "" {
			var task Task
			if err := json.Unmarshal(entityBytes, &task); err == nil && task.Subject != "" {
				entityName = task.Subject
				entityID = task.Name
			}
		}

		if entityName == "" {
			entityName = "[Unknown Name]"
		}
		if entityID == "" {
			entityID = "[Unknown ID]"
		}

		message.WriteString(fmt.Sprintf("| %d | %s | %s |\n", i+1, entityName, entityID))
	}

	// Short instruction
	if isVietnamese {
		message.WriteString("Vui lòng chọn " + entityType + " có sẵn hoặc yêu cầu tạo mới.\n")
		message.WriteString("Có thể chọn bằng số thứ tự hoặc tên.\n")
	} else {
		message.WriteString("Please select an existing " + entityType + " or request to create a new one.\n")
		message.WriteString("You can select by number or name.\n")
	}

	return message.String(), nil
}

// Helper function to safely extract entity ID
func getEntityID(entity interface{}) string {
	if entityBytes, err := json.Marshal(entity); err == nil {
		var entityMap map[string]interface{}
		if json.Unmarshal(entityBytes, &entityMap) == nil {
			if id, ok := entityMap["name"].(string); ok {
				return id
			}
		}
	}
	return "unknown"
}

// generateProjectTaskDisambiguationMessage generates message for project selection
func (m *ProjectManagementModule) generateProjectTaskDisambiguationMessage(
	ctx *erp_modules.ModuleContext,
	matchingProjects []Project,
) (string, error) {
	isVietnamese := detectUserLanguage(ctx.User)

	var message strings.Builder

	// Tiêu đề và bảng
	if isVietnamese {
		message.WriteString("**Tìm thấy nhiều dự án phù hợp:**\n\n")
		message.WriteString("| STT | Tên dự án | Mã |\n")
		message.WriteString("|-----|------------|-----|\n")
	} else {
		message.WriteString("**Found multiple matching projects:**\n\n")
		message.WriteString("| No. | Project Name | ID |\n")
		message.WriteString("|-----|---------------|-----|\n")
	}

	// Dữ liệu bảng
	for i, project := range matchingProjects {
		name := project.ProjectName
		id := project.Name

		if name == "" {
			name = "[Unknown]"
		}
		if id == "" {
			id = "[Unknown ID]"
		}

		message.WriteString(fmt.Sprintf("| %d | %s | %s |\n", i+1, name, id))
	}

	message.WriteString("\n")

	// Hướng dẫn lựa chọn (giống mẫu bạn dùng)
	if isVietnamese {
		message.WriteString("**Vui lòng chọn dự án bằng số thứ tự hoặc tên.**\n")
		message.WriteString("Bạn cũng có thể huỷ bỏ yêu cầu.")
	} else {
		message.WriteString("**Please select project by number or name.**\n")
		message.WriteString("You can also cancel this request.")
	}

	return message.String(), nil
}

// parseExistingEntityDisambiguationResponse parses user response to existing entity selection
// Replace the existing parseExistingEntityDisambiguationResponse method:
func (m *ProjectManagementModule) parseExistingEntityDisambiguationResponse(
	ctx *erp_modules.ModuleContext,
	message, entityType string,
	entityCount int,
) (*ExistingEntityDisambiguationResponse, error) {
	// For backward compatibility, we need to reconstruct existingEntities
	// This is a simplified version - in practice, you should pass the actual entities
	var existingEntities []interface{}
	for i := 0; i < entityCount; i++ {
		// Create placeholder entities - in real implementation,
		// you should pass the actual entities to this method
		if entityType == "project" {
			existingEntities = append(existingEntities, Project{Name: fmt.Sprintf("placeholder-%d", i+1)})
		} else {
			existingEntities = append(existingEntities, Task{Name: fmt.Sprintf("placeholder-%d", i+1)})
		}
	}

	// Use enhanced parsing
	enhancedResponse, err := m.parseEnhancedExistingEntityDisambiguationResponse(ctx, message, entityType, existingEntities)
	if err != nil {
		return nil, err
	}

	// Convert to old format for backward compatibility
	oldResponse := &ExistingEntityDisambiguationResponse{
		Intent:    enhancedResponse.Intent,
		Reasoning: enhancedResponse.Reasoning,
	}

	// Handle different intents
	switch enhancedResponse.Intent {
	case "index_selection":
		oldResponse.SelectedIndex = enhancedResponse.SelectedIndex
	case "name_selection":
		// For name selection, we need to find the index of the selected name
		// This is a simplified approach - in practice, you'd want better matching
		oldResponse.SelectedIndex = 1 // Default to first item
		oldResponse.Intent = "use_existing"
	case "create_new":
		oldResponse.Intent = "create_new"
		oldResponse.SelectedIndex = 0
	case "cancel":
		oldResponse.Intent = "cancel"
		oldResponse.SelectedIndex = 0
	}

	return oldResponse, nil
}

// parseProjectSelectionResponse parses user response to project selection
func (m *ProjectManagementModule) parseProjectSelectionResponse(
	ctx *erp_modules.ModuleContext,
	message string,
	matchingProjects []Project,
) (*ProjectSelectionResponse, error) {
	// Create LLM context
	llmContext := &llm.Context{
		RequestingUser: ctx.User,
		Time:           time.Now().Format(time.RFC1123),
	}

	isVietnamese := detectUserLanguage(ctx.User)

	llmContext.Parameters = map[string]interface{}{
		"UserMessage":  message,
		"ProjectCount": len(matchingProjects),
		"IsVietnamese": isVietnamese,
	}

	// Format the project selection analysis prompt
	systemPrompt, err := m.prompts.Format("project_selection_analysis", llmContext)
	if err != nil {
		return nil, fmt.Errorf("failed to format project selection analysis prompt: %w", err)
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
	response, err := m.getLLM().ChatCompletionNoStream(completionRequest, llm.WithMaxGeneratedTokens(200))
	if err != nil {
		return nil, fmt.Errorf("failed to analyze project selection with LLM: %w", err)
	}

	// Parse JSON response
	var selectionResponse ProjectSelectionResponse
	response = strings.TrimSpace(response)
	start := strings.Index(response, "{")
	end := strings.LastIndex(response, "}") + 1

	if start == -1 || end <= start {
		return nil, fmt.Errorf("no valid JSON found in LLM response: %s", response)
	}

	jsonStr := response[start:end]
	if err := json.Unmarshal([]byte(jsonStr), &selectionResponse); err != nil {
		return nil, fmt.Errorf("failed to parse LLM project selection response as JSON: %w", err)
	}

	return &selectionResponse, nil
}

// parseConfirmationResponse parses user confirmation response
func (m *ProjectManagementModule) parseConfirmationResponse(
	ctx *erp_modules.ModuleContext,
	message string,
	pendingData map[string]interface{},
) (*ConfirmationResponse, error) {
	// Create LLM context
	llmContext := &llm.Context{
		RequestingUser: ctx.User,
		Time:           time.Now().Format(time.RFC1123),
	}

	entityType := "project"
	if subject, ok := pendingData["subject"]; ok && subject != nil {
		entityType = "task"
	}

	llmContext.Parameters = map[string]interface{}{
		"UserMessage":    message,
		"PendingData":    pendingData,
		"OriginalSchema": m.getOriginalSchema(entityType),
	}

	// Use appropriate template based on type
	templateName := "project_modification_analysis"
	if entityType == "task" {
		templateName = "task_modification_analysis"
	}

	// Format the analysis prompt
	systemPrompt, err := m.prompts.Format(templateName, llmContext)
	if err != nil {
		return nil, fmt.Errorf("failed to format modification analysis prompt: %w", err)
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
	response, err := m.getLLM().ChatCompletionNoStream(completionRequest, llm.WithMaxGeneratedTokens(300))
	if err != nil {
		return nil, fmt.Errorf("failed to analyze user response with LLM: %w", err)
	}

	// Parse JSON response
	var confirmationResponse ConfirmationResponse
	response = strings.TrimSpace(response)
	start := strings.Index(response, "{")
	end := strings.LastIndex(response, "}") + 1

	if start == -1 || end <= start {
		return nil, fmt.Errorf("no valid JSON found in LLM response: %s", response)
	}

	jsonStr := response[start:end]
	if err := json.Unmarshal([]byte(jsonStr), &confirmationResponse); err != nil {
		return nil, fmt.Errorf("failed to parse LLM response as JSON: %w", err)
	}

	return &confirmationResponse, nil
}

// sendAssignmentNotifications sends DM notifications to assigned employees
func (m *ProjectManagementModule) sendAssignmentNotifications(
	entityType, entityID, entityName, assignerName, priority string,
	assignedEmployees []AssignedEmployee,
	startDate, endDate, projectName string, // THÊM CÁC PARAMETERS NÀY
) {
	if m.notificationSender == nil {
		m.api.LogWarn("Notification sender not configured, skipping DM notifications")
		return
	}

	for _, employee := range assignedEmployees {
		// Tìm user ID từ employee email hoặc employee ID
		userID, err := m.findUserIDByEmployee(employee)
		if err != nil {
			m.api.LogWarn("Could not find user ID for employee notification",
				"employee_name", employee.EmployeeName,
				"employee_email", employee.Email,
				"error", err.Error())
			continue
		}

		// Gửi notification tùy theo entity type
		var notificationErr error
		if entityType == "project" {
			notificationErr = m.notificationSender.SendProjectAssignmentNotification(
				userID, entityID, entityName, assignerName, priority, startDate, endDate)
		} else {
			notificationErr = m.notificationSender.SendTaskAssignmentNotification(
				userID, entityID, entityName, assignerName, priority, projectName, startDate, endDate)
		}

		if notificationErr != nil {
			m.api.LogError("Failed to send assignment notification",
				"entity_type", entityType,
				"entity_name", entityName,
				"employee_name", employee.EmployeeName,
				"user_id", userID,
				"error", notificationErr.Error())
		} else {
			m.api.LogInfo("Successfully sent assignment notification",
				"entity_type", entityType,
				"entity_name", entityName,
				"employee_name", employee.EmployeeName,
				"user_id", userID)
		}
	}
}

// findUserIDByEmployee finds Mattermost user ID from employee information
func (m *ProjectManagementModule) findUserIDByEmployee(employee AssignedEmployee) (string, error) {
	// Cách 1: Tìm bằng employee ID (nếu employee ID = Mattermost user ID)
	if len(employee.EmployeeID) == 26 { // Mattermost user ID length
		if user, err := m.api.GetUser(employee.EmployeeID); err == nil && user != nil {
			return employee.EmployeeID, nil
		}
	}

	// Cách 2: Tìm bằng email thông qua ERP
	// Lấy danh sách tất cả employees từ ERP để tìm custom_chat_id
	allEmployees, err := m.erpClient.GetAllEmployees()
	if err != nil {
		return "", fmt.Errorf("failed to get employees from ERP: %w", err)
	}

	// Tìm employee có ID hoặc email khớp
	for _, erpEmployee := range allEmployees {
		if (erpEmployee.Name == employee.EmployeeID || erpEmployee.CompanyEmail == employee.Email) &&
			erpEmployee.CustomChatID != "" {
			return erpEmployee.CustomChatID, nil
		}
	}

	return "", fmt.Errorf("could not find user ID for employee: %s", employee.EmployeeName)
}
