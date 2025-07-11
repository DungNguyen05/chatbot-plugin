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
}

// NewProjectManagementModule creates a new project management module with workflow engine
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

	// Create module
	module := &ProjectManagementModule{
		config:    config,
		erpClient: erpClient,
		api:       api,
		prompts:   prompts,
		getLLM:    getLLM,
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
	return []string{"create_project", "create_task"}
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

	// Get creator's email
	creatorEmail, err := m.getEmployeeEmailFromUser(ctx.User)
	if err != nil {
		m.api.LogWarn("Failed to get creator email", "error", err.Error())
		creatorEmail = "demo@example.com"
	}

	// Execute specific action by starting enhanced workflow
	switch intent.Action {
	case "create_project":
		return m.handleCreateProjectRequest(employeeID, creatorEmail, ctx, intent)
	case "create_task":
		return m.handleCreateTaskRequest(employeeID, creatorEmail, ctx, intent)
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

// generateExistingEntityDisambiguationMessage generates message for existing entity selection
func (m *ProjectManagementModule) generateExistingEntityDisambiguationMessage(
	ctx *erp_modules.ModuleContext,
	entityType string,
	existingEntities []interface{},
) (string, error) {
	isVietnamese := detectUserLanguage(ctx.User)

	var message strings.Builder

	if isVietnamese {
		if entityType == "project" {
			message.WriteString("**Tìm thấy dự án tương tự:**\n\n")
		} else {
			message.WriteString("**Tìm thấy task tương tự:**\n\n")
		}
	} else {
		if entityType == "project" {
			message.WriteString("**Found similar projects:**\n\n")
		} else {
			message.WriteString("**Found similar tasks:**\n\n")
		}
	}

	// List existing entities
	for i, entity := range existingEntities {
		if entityType == "project" {
			if projectBytes, err := json.Marshal(entity); err == nil {
				var project Project
				if json.Unmarshal(projectBytes, &project) == nil {
					message.WriteString(fmt.Sprintf("%d. **%s** (ID: %s, Status: %s)\n",
						i+1, project.ProjectName, project.Name, project.Status))
				}
			}
		} else {
			if taskBytes, err := json.Marshal(entity); err == nil {
				var task Task
				if json.Unmarshal(taskBytes, &task) == nil {
					message.WriteString(fmt.Sprintf("%d. **%s** (ID: %s, Status: %s)\n",
						i+1, task.Subject, task.Name, task.Status))
				}
			}
		}
	}

	message.WriteString("\n")

	if isVietnamese {
		if entityType == "project" {
			message.WriteString("Bạn muốn:\n")
			message.WriteString("• **Tạo mới** - Tạo dự án mới\n")
			message.WriteString("• **Chọn số** - Sử dụng dự án hiện có (ví dụ: '1')\n")
			message.WriteString("• **Hủy** - Hủy bỏ yêu cầu\n\n")
			message.WriteString("Vui lòng trả lời: 'tạo mới', số thứ tự, hoặc 'hủy'")
		} else {
			message.WriteString("Bạn muốn:\n")
			message.WriteString("• **Tạo mới** - Tạo task mới\n")
			message.WriteString("• **Chọn số** - Sử dụng task hiện có (ví dụ: '1')\n")
			message.WriteString("• **Hủy** - Hủy bỏ yêu cầu\n\n")
			message.WriteString("Vui lòng trả lời: 'tạo mới', số thứ tự, hoặc 'hủy'")
		}
	} else {
		if entityType == "project" {
			message.WriteString("Do you want to:\n")
			message.WriteString("• **Create new** - Create a new project\n")
			message.WriteString("• **Select number** - Use existing project (example: '1')\n")
			message.WriteString("• **Cancel** - Cancel the request\n\n")
			message.WriteString("Please reply: 'create new', number, or 'cancel'")
		} else {
			message.WriteString("Do you want to:\n")
			message.WriteString("• **Create new** - Create a new task\n")
			message.WriteString("• **Select number** - Use existing task (example: '1')\n")
			message.WriteString("• **Cancel** - Cancel the request\n\n")
			message.WriteString("Please reply: 'create new', number, or 'cancel'")
		}
	}

	return message.String(), nil
}

// generateProjectTaskDisambiguationMessage generates message for project selection
func (m *ProjectManagementModule) generateProjectTaskDisambiguationMessage(
	ctx *erp_modules.ModuleContext,
	matchingProjects []Project,
) (string, error) {
	isVietnamese := detectUserLanguage(ctx.User)

	var message strings.Builder

	if isVietnamese {
		message.WriteString("**Tìm thấy nhiều dự án phù hợp:**\n\n")
	} else {
		message.WriteString("**Found multiple matching projects:**\n\n")
	}

	// List matching projects
	for i, project := range matchingProjects {
		message.WriteString(fmt.Sprintf("%d. **%s** (ID: %s, Status: %s)\n",
			i+1, project.ProjectName, project.Name, project.Status))
	}

	message.WriteString("\n")

	if isVietnamese {
		message.WriteString("Bạn muốn:\n")
		message.WriteString("• **Chọn số** - Gán task vào dự án (ví dụ: '1')\n")
		message.WriteString("• **Không** - Tạo task không thuộc dự án nào\n")
		message.WriteString("• **Hủy** - Hủy bỏ yêu cầu\n\n")
		message.WriteString("Vui lòng trả lời: số thứ tự, 'không', hoặc 'hủy'")
	} else {
		message.WriteString("Do you want to:\n")
		message.WriteString("• **Select number** - Assign task to project (example: '1')\n")
		message.WriteString("• **No project** - Create task without project\n")
		message.WriteString("• **Cancel** - Cancel the request\n\n")
		message.WriteString("Please reply: number, 'no project', or 'cancel'")
	}

	return message.String(), nil
}

// parseExistingEntityDisambiguationResponse parses user response to existing entity selection
func (m *ProjectManagementModule) parseExistingEntityDisambiguationResponse(
	ctx *erp_modules.ModuleContext,
	message, entityType string,
	entityCount int,
) (*ExistingEntityDisambiguationResponse, error) {
	// Create LLM context
	llmContext := &llm.Context{
		RequestingUser: ctx.User,
		Time:           time.Now().Format(time.RFC1123),
	}

	isVietnamese := detectUserLanguage(ctx.User)

	llmContext.Parameters = map[string]interface{}{
		"UserMessage":  message,
		"EntityType":   entityType,
		"EntityCount":  entityCount,
		"IsVietnamese": isVietnamese,
	}

	// Format the disambiguation analysis prompt
	systemPrompt, err := m.prompts.Format("existing_entity_disambiguation_analysis", llmContext)
	if err != nil {
		return nil, fmt.Errorf("failed to format existing entity disambiguation prompt: %w", err)
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
		return nil, fmt.Errorf("failed to analyze existing entity disambiguation with LLM: %w", err)
	}

	// Parse JSON response
	var disambiguationResponse ExistingEntityDisambiguationResponse
	response = strings.TrimSpace(response)
	start := strings.Index(response, "{")
	end := strings.LastIndex(response, "}") + 1

	if start == -1 || end <= start {
		return nil, fmt.Errorf("no valid JSON found in LLM response: %s", response)
	}

	jsonStr := response[start:end]
	if err := json.Unmarshal([]byte(jsonStr), &disambiguationResponse); err != nil {
		return nil, fmt.Errorf("failed to parse LLM existing entity disambiguation response as JSON: %w", err)
	}

	return &disambiguationResponse, nil
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
