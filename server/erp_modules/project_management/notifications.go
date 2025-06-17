// Copyright (c) 2023-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package project_management

import (
	"bytes"
	"fmt"
	"html/template"
	"strings"
	"time"

	"github.com/mattermost/mattermost-plugin-ai/server/llm"
	"github.com/mattermost/mattermost/server/public/model"
)

// NotificationManager handles all project management related notifications
type NotificationManager struct {
	config    ProjectManagementConfig
	i18n      I18nBundle
	prompts   PromptsInterface
	getLLM    func() llm.LanguageModel
	api       PluginAPI
	botUserID string
}

// NewNotificationManager creates a new notification manager
func NewNotificationManager(
	config ProjectManagementConfig,
	i18n I18nBundle,
	prompts PromptsInterface,
	getLLM func() llm.LanguageModel,
	api PluginAPI,
	botUserID string,
) *NotificationManager {
	return &NotificationManager{
		config:    config,
		i18n:      i18n,
		prompts:   prompts,
		getLLM:    getLLM,
		api:       api,
		botUserID: botUserID,
	}
}

// SendNotification sends a notification about project management events
func (n *NotificationManager) SendNotification(userID, userName string, eventType ProjectManagementEventType, itemName string, details string) error {
	n.api.LogDebug("Sending project management notification",
		"user_id", userID,
		"event_type", string(eventType),
		"item_name", itemName)

	// Check if notifications are enabled
	if !n.config.Enabled {
		n.api.LogDebug("Project management notifications are disabled, skipping notification")
		return nil
	}

	// Get configured notification channels
	notifyChannelIDs := n.config.NotifyChannels
	if len(notifyChannelIDs) == 0 {
		n.api.LogDebug("No notification channels configured for project management")
		return nil
	}

	// Get the user's details for more personalized messages
	user, err := n.api.GetUser(userID)
	if err != nil {
		return err
	}

	// Create notification message based on user's locale
	defaultLocale := *n.api.GetConfig().LocalizationSettings.DefaultServerLocale
	message := n.createNotificationMessage(eventType, userName, itemName, details, defaultLocale)

	// Send to configured notification channels
	n.sendToChannels(notifyChannelIDs, message)

	// Send personalized message to the user via LLM
	if err := n.sendPersonalizedProjectMessage(user, eventType, itemName, details); err != nil {
		n.api.LogError("Failed to send personalized project management message",
			"user_id", userID,
			"error", err.Error())
	}

	return nil
}

// createNotificationMessage creates the appropriate notification message
func (n *NotificationManager) createNotificationMessage(eventType ProjectManagementEventType, userName, itemName, details, locale string) string {
	switch eventType {
	case ProjectManagementEventProjectCreated:
		return n.i18n.Localize("project.notification.created", "**%s** has created a new project: **%s**", locale, userName, itemName)
	case ProjectManagementEventTaskCreated:
		return n.i18n.Localize("project.notification.task_created", "**%s** has created a new task: **%s**", locale, userName, itemName)
	case ProjectManagementEventTaskAssigned:
		return n.i18n.Localize("project.notification.task_assigned", "**%s** has been assigned a task: **%s**", locale, userName, itemName)
	default:
		return fmt.Sprintf("**%s** - %s: %s", userName, string(eventType), itemName)
	}
}

// sendToChannels sends notifications to all configured channels
func (n *NotificationManager) sendToChannels(channelIDs []string, message string) {
	var successCount, errorCount int

	for _, channelID := range channelIDs {
		if err := n.sendToChannel(channelID, message); err != nil {
			n.api.LogError("Failed to send project management notification",
				"channel_id", channelID,
				"error", err.Error())
			errorCount++
		} else {
			successCount++
		}
	}

	n.api.LogInfo("Project management notification summary",
		"total_channels", len(channelIDs),
		"successful", successCount,
		"failed", errorCount)
}

// sendToChannel sends a notification to a specific channel
func (n *NotificationManager) sendToChannel(channelID, message string) error {
	// Validate channel exists and bot has access
	channel, err := n.api.GetChannel(channelID)
	if err != nil {
		return fmt.Errorf("failed to get channel: %w", err)
	}

	// Check if bot is a member of the channel
	_, err = n.api.GetChannelMember(channelID, n.botUserID)
	if err != nil {
		n.api.LogWarn("Bot is not a member of notification channel, attempting to add bot",
			"channel_id", channelID,
			"bot_id", n.botUserID,
			"channel_name", channel.Name)

		// Try to add the bot to the channel
		if _, addErr := n.api.AddChannelMember(channelID, n.botUserID); addErr != nil {
			return fmt.Errorf("failed to add bot to channel: %w", addErr)
		}

		n.api.LogInfo("Successfully added bot to notification channel",
			"channel_id", channelID,
			"bot_id", n.botUserID,
			"channel_name", channel.Name)
	}

	// Create and send the post
	post := &model.Post{
		ChannelId: channelID,
		Message:   message,
		UserId:    n.botUserID,
	}

	return n.api.CreatePost(post)
}

// sendPersonalizedProjectMessage sends a personalized message to the user using the LLM
func (n *NotificationManager) sendPersonalizedProjectMessage(user *model.User, eventType ProjectManagementEventType, itemName, details string) error {
	// Set up context for LLM
	context := &llm.Context{
		RequestingUser: user,
	}

	// Current time info for more context
	vietTime, err := GetVietnamTime()
	if err != nil {
		vietTime = time.Now()
	}

	timeOfDay := getTimeOfDay(vietTime)
	dayOfWeek := vietTime.Weekday().String()

	// Determine language for prompts based on user locale
	isVietnamese := strings.HasPrefix(user.Locale, "vi")

	// Get Vietnamese day of week if needed
	vietnameseDayOfWeek := dayOfWeek
	if isVietnamese {
		vietnameseDayOfWeek = getVietnameseDayOfWeek(vietTime.Weekday())
	}

	// Build parameters for LLM
	context.Parameters = map[string]any{
		"EventType":           string(eventType),
		"ItemName":            itemName,
		"Details":             details,
		"TimeOfDay":           timeOfDay,
		"DayOfWeek":           dayOfWeek,
		"VietnameseDayOfWeek": vietnameseDayOfWeek,
		"UserName":            getUserDisplayName(user),
		"IsProjectCreated":    eventType == ProjectManagementEventProjectCreated,
		"IsTaskCreated":       eventType == ProjectManagementEventTaskCreated,
		"IsTaskAssigned":      eventType == ProjectManagementEventTaskAssigned,
		"IsVietnamese":        isVietnamese,
	}

	// Define the prompt based on event type and language
	promptText := n.buildPromptText(eventType, isVietnamese)

	// Replace template variables in the prompt
	processedPrompt, err := processTemplate(promptText, context.Parameters)
	if err != nil {
		return fmt.Errorf("failed to process template: %w", err)
	}

	// Use the LLM to generate the personalized message
	messageRequest := llm.CompletionRequest{
		Posts: []llm.Post{
			{
				Role:    llm.PostRoleSystem,
				Message: processedPrompt,
			},
		},
		Context: context,
	}

	result, err := n.getLLM().ChatCompletionNoStream(messageRequest)
	if err != nil {
		return fmt.Errorf("failed to generate personalized message: %w", err)
	}

	// Send the message to the user
	post := &model.Post{
		Message: result,
	}

	if err := n.api.BotDMNonResponse(n.botUserID, user.Id, post); err != nil {
		return fmt.Errorf("failed to send personalized DM: %w", err)
	}

	return nil
}

// buildPromptText builds the appropriate prompt based on event type and language
func (n *NotificationManager) buildPromptText(eventType ProjectManagementEventType, isVietnamese bool) string {
	switch eventType {
	case ProjectManagementEventProjectCreated:
		if isVietnamese {
			return `Bạn là trợ lý quản lý dự án thân thiện. Tạo một tin nhắn thông báo (1-2 câu) 
cho {{.UserName}} vừa tạo dự án mới "{{.ItemName}}". 
Hiện tại là {{.TimeOfDay}} vào {{.VietnameseDayOfWeek}}. 
Làm cho nó nghe chuyên nghiệp nhưng thân thiện. Khuyến khích họ về việc bắt đầu dự án mới. KHÔNG SỬ DỤNG QUÁ 2 CÂU. Sử dụng tiếng Việt.`
		} else {
			return `You are a friendly project management assistant. Generate a message to notice (1-2 sentences only) 
for {{.UserName}} who just created a new project "{{.ItemName}}". 
It's currently {{.TimeOfDay}} on {{.DayOfWeek}}. 
Make it sound professional but encouraging about starting the new project. DO NOT USE MORE THAN 2 SENTENCES. Use English.`
		}

	case ProjectManagementEventTaskCreated:
		if isVietnamese {
			return `Bạn là trợ lý quản lý dự án thân thiện. Tạo một tin nhắn thông báo (1-2 câu) 
cho {{.UserName}} vừa tạo task mới "{{.ItemName}}". 
Hiện tại là {{.TimeOfDay}} vào {{.VietnameseDayOfWeek}}. 
Khuyến khích về việc tổ chức công việc tốt. KHÔNG SỬ DỤNG QUÁ 2 CÂU. Sử dụng tiếng Việt.`
		} else {
			return `You are a friendly project management assistant. Generate a message to notice (1-2 sentences only) 
for {{.UserName}} who just created a new task "{{.ItemName}}". 
It's currently {{.TimeOfDay}} on {{.DayOfWeek}}. 
Encourage good task organization. DO NOT USE MORE THAN 2 SENTENCES. Use English.`
		}

	case ProjectManagementEventTaskAssigned:
		if isVietnamese {
			return `Bạn là trợ lý quản lý dự án thân thiện. Tạo một tin nhắn thông báo (1-2 câu) 
cho {{.UserName}} vừa được giao task "{{.ItemName}}". 
Hiện tại là {{.TimeOfDay}} vào {{.VietnameseDayOfWeek}}. 
Khuyến khích về việc nhận nhiệm vụ mới. KHÔNG SỬ DỤNG QUÁ 2 CÂU. Sử dụng tiếng Việt.`
		} else {
			return `You are a friendly project management assistant. Generate a message to notice (1-2 sentences only) 
for {{.UserName}} who was just assigned task "{{.ItemName}}". 
It's currently {{.TimeOfDay}} on {{.DayOfWeek}}. 
Encourage about the new assignment. DO NOT USE MORE THAN 2 SENTENCES. Use English.`
		}

	default:
		if isVietnamese {
			return `Tạo một tin nhắn ngắn gọn và thân thiện cho {{.UserName}} về dự án/task "{{.ItemName}}". Sử dụng tiếng Việt.`
		} else {
			return `Create a short and friendly message for {{.UserName}} about project/task "{{.ItemName}}". Use English.`
		}
	}
}

// processTemplate processes template text with given data
func processTemplate(templateText string, data map[string]any) (string, error) {
	tmpl, err := template.New("prompt").Parse(templateText)
	if err != nil {
		return "", fmt.Errorf("failed to parse template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("failed to execute template: %w", err)
	}

	return buf.String(), nil
}

// getTimeOfDay returns a string describing the time of day
func getTimeOfDay(t time.Time) string {
	hour := t.Hour()

	switch {
	case hour >= 5 && hour < 12:
		return "morning"
	case hour >= 12 && hour < 17:
		return "afternoon"
	case hour >= 17 && hour < 21:
		return "evening"
	default:
		return "night"
	}
}

// getVietnameseDayOfWeek returns Vietnamese day of week
func getVietnameseDayOfWeek(weekday time.Weekday) string {
	switch weekday {
	case time.Sunday:
		return "Chủ nhật"
	case time.Monday:
		return "Thứ hai"
	case time.Tuesday:
		return "Thứ ba"
	case time.Wednesday:
		return "Thứ tư"
	case time.Thursday:
		return "Thứ năm"
	case time.Friday:
		return "Thứ sáu"
	case time.Saturday:
		return "Thứ bảy"
	default:
		return "Chủ nhật"
	}
}

// getUserDisplayName returns the best display name for a user
func getUserDisplayName(user *model.User) string {
	if user.FirstName != "" {
		return user.FirstName
	}
	if user.Nickname != "" {
		return user.Nickname
	}
	return user.Username
}
