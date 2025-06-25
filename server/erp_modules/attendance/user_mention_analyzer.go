// Copyright (c) 2023-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package attendance

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/mattermost/mattermost-plugin-ai/server/llm"
	"github.com/mattermost/mattermost/server/public/model"
)

// UserMentionAnalysisResult represents the result of analyzing user mentions
type UserMentionAnalysisResult struct {
	MentionedUsers []string `json:"mentioned_users"`
	QueryType      string   `json:"query_type"` // "self", "others"
	Reasoning      string   `json:"reasoning"`
}

// UserMentionAnalyzer analyzes user mentions in attendance queries
type UserMentionAnalyzer struct {
	api     PluginAPI
	prompts PromptsInterface
	getLLM  func() llm.LanguageModel
}

// NewUserMentionAnalyzer creates a new user mention analyzer
func NewUserMentionAnalyzer(api PluginAPI, prompts PromptsInterface, getLLM func() llm.LanguageModel) *UserMentionAnalyzer {
	return &UserMentionAnalyzer{
		api:     api,
		prompts: prompts,
		getLLM:  getLLM,
	}
}

// AnalyzeUserMentions analyzes a message to determine which users' attendance data is being requested
func (a *UserMentionAnalyzer) AnalyzeUserMentions(message string, requestingUser *model.User) (*UserMentionAnalysisResult, error) {
	// Extract @mentions from the message
	mentionedUsernames := a.extractMentions(message)

	// Use LLM to analyze the intent
	analysisResult, err := a.analyzeMentionIntent(message, requestingUser, mentionedUsernames)
	if err != nil {
		return nil, fmt.Errorf("failed to analyze mention intent: %w", err)
	}

	// Convert usernames to user IDs
	mentionedUserIDs, err := a.convertUsernamesToIDs(analysisResult.MentionedUsers)
	if err != nil {
		a.api.LogWarn("Failed to convert some usernames to IDs", "error", err.Error())
		// Continue with the users we could resolve
	}

	analysisResult.MentionedUsers = mentionedUserIDs

	return analysisResult, nil
}

// extractMentions extracts @mentions from a message
func (a *UserMentionAnalyzer) extractMentions(message string) []string {
	// Regex to match @username patterns
	mentionRegex := regexp.MustCompile(`@([a-zA-Z0-9\.\-_]+)`)
	matches := mentionRegex.FindAllStringSubmatch(message, -1)

	var usernames []string
	for _, match := range matches {
		if len(match) > 1 {
			usernames = append(usernames, match[1])
		}
	}

	return usernames
}

// analyzeMentionIntent uses LLM to analyze the user's intent regarding mentions
func (a *UserMentionAnalyzer) analyzeMentionIntent(message string, requestingUser *model.User, mentionedUsernames []string) (*UserMentionAnalysisResult, error) {
	// Create LLM context
	llmContext := &llm.Context{
		RequestingUser: requestingUser,
	}

	// Detect user language
	isVietnamese := detectUserLanguage(requestingUser)

	llmContext.Parameters = map[string]interface{}{
		"UserMessage":        message,
		"MentionedUsernames": mentionedUsernames,
		"RequestingUsername": requestingUser.Username,
		"IsVietnamese":       isVietnamese,
	}

	// Format the mention analysis prompt
	systemPrompt, err := a.prompts.Format("attendance_mention_analysis", llmContext)
	if err != nil {
		return nil, fmt.Errorf("failed to format mention analysis prompt: %w", err)
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
	response, err := a.getLLM().ChatCompletionNoStream(completionRequest, llm.WithMaxGeneratedTokens(300))
	if err != nil {
		return nil, fmt.Errorf("failed to analyze mentions with LLM: %w", err)
	}

	// Parse JSON response
	var result UserMentionAnalysisResult
	response = strings.TrimSpace(response)
	start := strings.Index(response, "{")
	end := strings.LastIndex(response, "}") + 1

	if start == -1 || end <= start {
		return nil, fmt.Errorf("no valid JSON found in LLM response: %s", response)
	}

	jsonStr := response[start:end]
	if err := json.Unmarshal([]byte(jsonStr), &result); err != nil {
		return nil, fmt.Errorf("failed to parse LLM response as JSON: %w", err)
	}

	return &result, nil
}

// convertUsernamesToIDs converts usernames to user IDs
func (a *UserMentionAnalyzer) convertUsernamesToIDs(usernames []string) ([]string, error) {
	var userIDs []string
	var errors []string

	for _, username := range usernames {
		user, err := a.api.GetUserByUsername(username)
		if err != nil {
			errors = append(errors, fmt.Sprintf("failed to find user %s: %v", username, err))
			continue
		}
		userIDs = append(userIDs, user.Id)
	}

	if len(errors) > 0 {
		return userIDs, fmt.Errorf("some users could not be resolved: %s", strings.Join(errors, "; "))
	}

	return userIDs, nil
}

// GetUserDisplayName returns the best display name for a user
func GetUserDisplayName(user *model.User) string {
	if user.FirstName != "" {
		return user.FirstName
	}
	if user.Nickname != "" {
		return user.Nickname
	}
	return user.Username
}
