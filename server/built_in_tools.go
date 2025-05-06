// Copyright (c) 2023-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"errors"

	"github.com/andygrunwald/go-jira"
	"github.com/google/go-github/v41/github"
	"github.com/mattermost/mattermost-plugin-ai/server/llm"
	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/pluginapi"
)

type LookupMattermostUserArgs struct {
	Username string `jsonschema_description:"The username of the user to lookup without a leading '@'. Example: 'firstname.lastname'"`
}

func (p *Plugin) toolResolveLookupMattermostUser(context *llm.Context, argsGetter llm.ToolArgumentGetter) (string, error) {
	var args LookupMattermostUserArgs
	err := argsGetter(&args)
	if err != nil {
		return "invalid parameters to function", fmt.Errorf("failed to get arguments for tool LookupMattermostUser: %w", err)
	}

	if !model.IsValidUsername(args.Username) {
		return "invalid username", errors.New("invalid username")
	}

	// Fail for guests.
	if !p.pluginAPI.User.HasPermissionTo(context.RequestingUser.Id, model.PermissionViewMembers) {
		return "user doesn't have permissions", errors.New("user doesn't have permission to lookup users")
	}

	user, err := p.pluginAPI.User.GetByUsername(args.Username)
	if err != nil {
		if errors.Is(err, pluginapi.ErrNotFound) {
			return "user not found", nil
		}
		return "failed to lookup user", fmt.Errorf("failed to lookup user: %w", err)
	}

	userStatus, err := p.pluginAPI.User.GetStatus(user.Id)
	if err != nil {
		return "failed to lookup user", fmt.Errorf("failed to get user status: %w", err)
	}

	result := fmt.Sprintf("Username: %s", user.Username)
	if p.pluginAPI.Configuration.GetConfig().PrivacySettings.ShowFullName != nil && *p.pluginAPI.Configuration.GetConfig().PrivacySettings.ShowFullName {
		if user.FirstName != "" || user.LastName != "" {
			result += fmt.Sprintf("\nFull Name: %s %s", user.FirstName, user.LastName)
		}
	}
	if p.pluginAPI.Configuration.GetConfig().PrivacySettings.ShowEmailAddress != nil && *p.pluginAPI.Configuration.GetConfig().PrivacySettings.ShowEmailAddress {
		result += fmt.Sprintf("\nEmail: %s", user.Email)
	}
	if user.Nickname != "" {
		result += fmt.Sprintf("\nNickname: %s", user.Nickname)
	}
	if user.Position != "" {
		result += fmt.Sprintf("\nPosition: %s", user.Position)
	}
	if user.Locale != "" {
		result += fmt.Sprintf("\nLocale: %s", user.Locale)
	}
	result += fmt.Sprintf("\nTimezone: %s", model.GetPreferredTimezone(user.Timezone))
	result += fmt.Sprintf("\nLast Activity: %s", model.GetTimeForMillis(userStatus.LastActivityAt).Format("2006-01-02 15:04:05 MST"))
	// Exclude manual statuses because they could be prompt injections
	if userStatus.Status != "" && !userStatus.Manual {
		result += fmt.Sprintf("\nStatus: %s", userStatus.Status)
	}

	return result, nil
}

type GetGithubIssueArgs struct {
	RepoOwner string `jsonschema_description:"The owner of the repository to get issues from. Example: 'mattermost'"`
	RepoName  string `jsonschema_description:"The name of the repository to get issues from. Example: 'mattermost-plugin-ai'"`
	Number    int    `jsonschema_description:"The issue number to get. Example: '1'"`
}

func formatGithubIssue(issue *github.Issue) string {
	return fmt.Sprintf("Title: %s\nNumber: %d\nState: %s\nSubmitter: %s\nIs Pull Request: %v\nBody: %s", issue.GetTitle(), issue.GetNumber(), issue.GetState(), issue.User.GetLogin(), issue.IsPullRequest(), issue.GetBody())
}

var validGithubRepoName = regexp.MustCompile(`^[a-zA-Z0-9_.-]+$`)

func (p *Plugin) toolGetGithubIssue(context *llm.Context, argsGetter llm.ToolArgumentGetter) (string, error) {
	var args GetGithubIssueArgs
	err := argsGetter(&args)
	if err != nil {
		return "invalid parameters to function", fmt.Errorf("failed to get arguments for tool GetGithubIssues: %w", err)
	}

	// Fail for over length repo owner or name.
	if len(args.RepoOwner) > 39 || len(args.RepoName) > 100 {
		return "invalid parameters to function", errors.New("invalid repo owner or repo name")
	}

	// Fail if repo owner or repo name contain invalid characters.
	if !validGithubRepoName.MatchString(args.RepoOwner) || !validGithubRepoName.MatchString(args.RepoName) {
		return "invalid parameters to function", errors.New("invalid repo owner or repo name")
	}

	// Fail for bad issue numbers.
	if args.Number < 1 {
		return "invalid parameters to function", errors.New("invalid issue number")
	}

	req, err := http.NewRequest(http.MethodGet,
		fmt.Sprintf("/github/api/v1/issue?owner=%s&repo=%s&number=%d",
			url.QueryEscape(args.RepoOwner),
			url.QueryEscape(args.RepoName),
			args.Number,
		),
		nil,
	)
	if err != nil {
		return "internal failure", fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Mattermost-User-ID", context.RequestingUser.Id)

	resp := p.pluginAPI.Plugin.HTTP(req)
	if resp == nil {
		return "Error: unable to get issue, internal failure", errors.New("failed to get issue, response was nil")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		result, _ := io.ReadAll(resp.Body)
		return "Error: unable to get issue, internal failure", fmt.Errorf("failed to get issue, status code: %v\n body: %v", resp.Status, string(result))
	}

	var issue github.Issue
	err = json.NewDecoder(resp.Body).Decode(&issue)
	if err != nil {
		return "internal failure", fmt.Errorf("failed to decode response: %w", err)
	}

	return formatGithubIssue(&issue), nil
}

type GetJiraIssueArgs struct {
	InstanceURL string   `jsonschema_description:"The URL of the Jira instance to get the issue from. Example: 'https://mattermost.atlassian.net'"`
	IssueKeys   []string `jsonschema_description:"The issue keys of the Jira issues to get. Example: 'MM-1234'"`
}

var validJiraIssueKey = regexp.MustCompile(`^([[:alnum:]]+)-([[:digit:]]+)$`)

func formatJiraIssue(issue *jira.Issue) string {
	result := strings.Builder{}
	result.WriteString("Issue Key: ")
	result.WriteString(issue.Key)
	result.WriteRune('\n')

	if issue.Fields != nil {
		result.WriteString("Summary: ")
		result.WriteString(issue.Fields.Summary)
		result.WriteRune('\n')

		result.WriteString("Description: ")
		result.WriteString(issue.Fields.Description)
		result.WriteRune('\n')

		result.WriteString("Status: ")
		if issue.Fields.Status != nil {
			result.WriteString(issue.Fields.Status.Name)
		} else {
			result.WriteString("Unknown")
		}
		result.WriteRune('\n')

		result.WriteString("Assignee: ")
		if issue.Fields.Assignee != nil {
			result.WriteString(issue.Fields.Assignee.DisplayName)
		} else {
			result.WriteString("Unassigned")
		}
		result.WriteRune('\n')

		result.WriteString("Created: ")
		result.WriteString(time.Time(issue.Fields.Created).Format(time.RFC1123))
		result.WriteRune('\n')

		result.WriteString("Updated: ")
		result.WriteString(time.Time(issue.Fields.Updated).Format(time.RFC1123))
		result.WriteRune('\n')

		if issue.Fields.Type.Name != "" {
			result.WriteString("Type: ")
			result.WriteString(issue.Fields.Type.Name)
			result.WriteRune('\n')
		}

		if issue.Fields.Labels != nil {
			result.WriteString("Labels: ")
			result.WriteString(strings.Join(issue.Fields.Labels, ", "))
			result.WriteRune('\n')
		}

		if issue.Fields.Reporter != nil {
			result.WriteString("Reporter: ")
			result.WriteString(issue.Fields.Reporter.DisplayName)
			result.WriteRune('\n')
		} else if issue.Fields.Creator != nil {
			result.WriteString("Creator: ")
			result.WriteString(issue.Fields.Creator.DisplayName)
			result.WriteRune('\n')
		}

		if issue.Fields.Priority != nil {
			result.WriteString("Priority: ")
			result.WriteString(issue.Fields.Priority.Name)
			result.WriteRune('\n')
		}

		if !time.Time(issue.Fields.Duedate).IsZero() {
			result.WriteString("Due Date: ")
			result.WriteString(time.Time(issue.Fields.Duedate).Format(time.RFC1123))
			result.WriteRune('\n')
		}

		if issue.Fields.TimeTracking != nil {
			if issue.Fields.TimeTracking.OriginalEstimate != "" {
				result.WriteString("Original Estimate: ")
				result.WriteString(issue.Fields.TimeTracking.OriginalEstimate)
				result.WriteRune('\n')
			}
			if issue.Fields.TimeTracking.TimeSpent != "" {
				result.WriteString("Time Spent: ")
				result.WriteString(issue.Fields.TimeTracking.TimeSpent)
				result.WriteRune('\n')
			}
			if issue.Fields.TimeTracking.RemainingEstimate != "" {
				result.WriteString("Remaining Estimate: ")
				result.WriteString(issue.Fields.TimeTracking.RemainingEstimate)
				result.WriteRune('\n')
			}
		}

		if issue.Fields.Comments != nil {
			for _, comment := range issue.Fields.Comments.Comments {
				result.WriteString(fmt.Sprintf("Comment from %s at %s: %s\n", comment.Author.DisplayName, comment.Created, comment.Body))
			}
		}
	}

	return result.String()
}

var fetchedFields = []string{
	"summary",
	"description",
	"status",
	"assignee",
	"created",
	"updated",
	"issuetype",
	"labels",
	"reporter",
	"creator",
	"priority",
	"duedate",
	"timetracking",
	"comment",
}

func (p *Plugin) getPublicJiraIssues(instanceURL string, issueKeys []string) ([]jira.Issue, error) {
	httpClient := p.createExternalHTTPClient()
	client, err := jira.NewClient(httpClient, instanceURL)
	if err != nil {
		return nil, fmt.Errorf("failed to create Jira client: %w", err)
	}
	jql := fmt.Sprintf("key in (%s)", strings.Join(issueKeys, ","))
	issues, _, err := client.Issue.Search(jql, &jira.SearchOptions{Fields: fetchedFields})
	if err != nil {
		return nil, fmt.Errorf("failed to get issue: %w", err)
	}
	if issues == nil {
		return nil, fmt.Errorf("failed to get issue: issue not found")
	}

	return issues, nil
}

func (p *Plugin) toolGetJiraIssue(context *llm.Context, argsGetter llm.ToolArgumentGetter) (string, error) {
	var args GetJiraIssueArgs
	err := argsGetter(&args)
	if err != nil {
		return "invalid parameters to function", fmt.Errorf("failed to get arguments for tool GetJiraIssue: %w", err)
	}

	// Fail for over-length issue key. or doesn't look like an issue key
	for _, issueKey := range args.IssueKeys {
		if len(issueKey) > 50 || !validJiraIssueKey.MatchString(issueKey) {
			return "invalid parameters to function", errors.New("invalid issue key")
		}
	}

	issues, err := p.getPublicJiraIssues(args.InstanceURL, args.IssueKeys)
	if err != nil {
		return "internal failure", err
	}

	result := strings.Builder{}
	for i := range issues {
		result.WriteString(formatJiraIssue(&issues[i]))
		result.WriteString("------\n")
	}

	return result.String(), nil
}

// Removing the SearchServer tool since search functionality is removed in MySQL version

// getBuiltInTools returns the built-in tools that are available to all users.
// isDM is true if the response will be in a DM with the user. More tools are available in DMs because of security properties.

func (p *Plugin) getDefaultToolsStore(bot *Bot, isDM bool) *llm.ToolStore {
	if bot == nil || bot.cfg.DisableTools {
		return llm.NewNoTools()
	}
	store := llm.NewToolStore(&p.pluginAPI.Log, p.getConfiguration().EnableLLMTrace)
	store.AddTools(p.getBuiltInTools(isDM, bot))
	return store
}

type CreateTaskArgs struct {
	Title            string `jsonschema_description:"The title of the task"`
	Description      string `jsonschema_description:"The detailed description of the task"`
	AssigneeUsername string `jsonschema_description:"The username of the person to assign the task to"`
	Deadline         string `jsonschema_description:"The deadline for the task in format YYYY-MM-DD or relative terms like 'tomorrow', 'next week', etc."`
}

type UpdateTaskStatusArgs struct {
	TaskID string `jsonschema_description:"The ID of the task to update"`
	Status string `jsonschema_description:"The new status of the task (open, complete)"`
}

type StartRollCallArgs struct {
	Title string `jsonschema_description:"The title or purpose of the roll call"`
}

type RespondToRollCallArgs struct {
	Response string `jsonschema_description:"The response to the roll call, like 'present', 'here', or other status information"`
}

type EndRollCallArgs struct {
	ShowSummary bool `jsonschema_description:"Whether to show a summary of the roll call responses"`
}

func (p *Plugin) toolResolveCreateTask(context *llm.Context, argsGetter llm.ToolArgumentGetter) (string, error) {
	var args CreateTaskArgs
	err := argsGetter(&args)
	if err != nil {
		return "Invalid parameters to function", fmt.Errorf("failed to get arguments for tool CreateTask: %w", err)
	}

	// Parse deadline
	deadline := time.Now().Add(24 * time.Hour) // Default to 24 hours from now
	if args.Deadline != "" {
		parsedDeadline, err := parseHumanReadableDate(args.Deadline)
		if err == nil {
			deadline = parsedDeadline
		}
	}

	// Get assignee user
	assignee, err := p.pluginAPI.User.GetByUsername(args.AssigneeUsername)
	if err != nil {
		return fmt.Sprintf("User %s not found", args.AssigneeUsername), nil
	}

	// Get current channel
	channel := context.Channel
	if channel == nil {
		return "Cannot create task: no channel context", nil
	}

	// Create task
	task, err := p.CreateTask(
		args.Title,
		args.Description,
		assignee.Id,
		context.RequestingUser.Id,
		channel.Id,
		deadline.UnixMilli(),
	)

	if err != nil {
		return "Failed to create task", err
	}

	// Notify the assignee about the task
	p.sendTaskNotification(task, assignee)

	deadlineStr := deadline.Format("2006-01-02 15:04:05")
	return fmt.Sprintf("Task created and assigned to %s (ID: %s)\nTitle: %s\nDescription: %s\nDeadline: %s",
		assignee.Username, task.ID, task.Title, task.Description, deadlineStr), nil
}

func (p *Plugin) toolResolveUpdateTaskStatus(context *llm.Context, argsGetter llm.ToolArgumentGetter) (string, error) {
	var args UpdateTaskStatusArgs
	err := argsGetter(&args)
	if err != nil {
		return "Invalid parameters to function", fmt.Errorf("failed to get arguments for tool UpdateTaskStatus: %w", err)
	}

	var status TaskStatus
	switch args.Status {
	case "complete":
		status = TaskStatusComplete
	case "open":
		status = TaskStatusOpen
	default:
		return "Invalid status. Use 'open' or 'complete'.", nil
	}

	err = p.UpdateTaskStatus(args.TaskID, status)
	if err != nil {
		return "Failed to update task status", err
	}

	return fmt.Sprintf("Task status updated to %s", args.Status), nil
}

func (p *Plugin) toolResolveStartRollCall(context *llm.Context, argsGetter llm.ToolArgumentGetter) (string, error) {
	var args StartRollCallArgs
	err := argsGetter(&args)
	if err != nil {
		return "Invalid parameters to function", fmt.Errorf("failed to get arguments for tool StartRollCall: %w", err)
	}

	// Get current channel
	channel := context.Channel
	if channel == nil {
		return "Cannot start roll call: no channel context", nil
	}

	// Check if there's already an active roll call
	existingRollCall, err := p.GetActiveRollCall(channel.Id)
	if err != nil {
		return "Failed to check for active roll calls", err
	}

	if existingRollCall != nil {
		return "There is already an active roll call in this channel", nil
	}

	// Create roll call
	rollCall, err := p.CreateRollCall(channel.Id, context.RequestingUser.Id, args.Title)
	if err != nil {
		return "Failed to start roll call", err
	}

	// Post a message to the channel about the roll call
	err = p.postRollCallAnnouncement(rollCall, channel)
	if err != nil {
		return "Roll call started but failed to post announcement", err
	}

	return fmt.Sprintf("Roll call started: %s (ID: %s)\nRespond with the 'Respond to Roll Call' command.",
		rollCall.Title, rollCall.ID), nil
}

func (p *Plugin) toolResolveRespondToRollCall(context *llm.Context, argsGetter llm.ToolArgumentGetter) (string, error) {
	var args RespondToRollCallArgs
	err := argsGetter(&args)
	if err != nil {
		return "Invalid parameters to function", fmt.Errorf("failed to get arguments for tool RespondToRollCall: %w", err)
	}

	// Get current channel
	channel := context.Channel
	if channel == nil {
		return "Cannot respond to roll call: no channel context", nil
	}

	// Get active roll call
	rollCall, err := p.GetActiveRollCall(channel.Id)
	if err != nil {
		return "Failed to check for active roll calls", err
	}

	if rollCall == nil {
		return "There is no active roll call in this channel", nil
	}

	// Record response
	err = p.RecordRollCallResponse(rollCall.ID, context.RequestingUser.Id, args.Response)
	if err != nil {
		return "Failed to record roll call response", err
	}

	return "Your roll call response has been recorded", nil
}

func (p *Plugin) toolResolveEndRollCall(context *llm.Context, argsGetter llm.ToolArgumentGetter) (string, error) {
	var args EndRollCallArgs
	err := argsGetter(&args)
	if err != nil {
		return "Invalid parameters to function", fmt.Errorf("failed to get arguments for tool EndRollCall: %w", err)
	}

	// Get current channel
	channel := context.Channel
	if channel == nil {
		return "Cannot end roll call: no channel context", nil
	}

	// Get active roll call
	rollCall, err := p.GetActiveRollCall(channel.Id)
	if err != nil {
		return "Failed to check for active roll calls", err
	}

	if rollCall == nil {
		return "There is no active roll call in this channel", nil
	}

	// End roll call
	err = p.EndRollCall(rollCall.ID)
	if err != nil {
		return "Failed to end roll call", err
	}

	if !args.ShowSummary {
		return "Roll call ended", nil
	}

	// Get and format summary
	summary, err := p.formatRollCallSummary(rollCall)
	if err != nil {
		return "Roll call ended but failed to generate summary", err
	}

	return fmt.Sprintf("Roll call ended. Summary:\n\n%s", summary), nil
}

// Add these new tools to the getBuiltInTools function:
func (p *Plugin) getBuiltInTools(isDM bool, bot *Bot) []llm.Tool {
	builtInTools := []llm.Tool{}

	if isDM {
		builtInTools = append(builtInTools, llm.Tool{
			Name:        "LookupMattermostUser",
			Description: "Lookup a Mattermost user by their username. Available information includes: username, full name, email, nickname, position, locale, timezone, last activity, and status.",
			Schema:      LookupMattermostUserArgs{},
			Resolver:    p.toolResolveLookupMattermostUser,
		})

		// GitHub plugin tools
		status, err := p.pluginAPI.Plugin.GetPluginStatus("github")
		if err != nil && !errors.Is(err, pluginapi.ErrNotFound) {
			p.API.LogError("failed to get github plugin status", "error", err.Error())
		} else if status != nil && status.State == model.PluginStateRunning {
			builtInTools = append(builtInTools, llm.Tool{
				Name:        "GetGithubIssue",
				Description: "Retrieve a single GitHub issue by owner, repo, and issue number.",
				Schema:      GetGithubIssueArgs{},
				Resolver:    p.toolGetGithubIssue,
			})
		}

		// Jira plugin tools
		builtInTools = append(builtInTools, llm.Tool{
			Name:        "GetJiraIssue",
			Description: "Retrieve a single Jira issue by issue key.",
			Schema:      GetJiraIssueArgs{},
			Resolver:    p.toolGetJiraIssue,
		})
	}

	// Task management tools - available in all contexts
	builtInTools = append(builtInTools, llm.Tool{
		Name:        "CreateTask",
		Description: "Create a new task and assign it to a user with a deadline",
		Schema:      CreateTaskArgs{},
		Resolver:    p.toolResolveCreateTask,
	})

	builtInTools = append(builtInTools, llm.Tool{
		Name:        "UpdateTaskStatus",
		Description: "Update the status of a task (open, complete)",
		Schema:      UpdateTaskStatusArgs{},
		Resolver:    p.toolResolveUpdateTaskStatus,
	})

	builtInTools = append(builtInTools, llm.Tool{
		Name:        "StartRollCall",
		Description: "Start a roll call in the current channel to track who is present",
		Schema:      StartRollCallArgs{},
		Resolver:    p.toolResolveStartRollCall,
	})

	builtInTools = append(builtInTools, llm.Tool{
		Name:        "RespondToRollCall",
		Description: "Respond to an active roll call in the current channel",
		Schema:      RespondToRollCallArgs{},
		Resolver:    p.toolResolveRespondToRollCall,
	})

	builtInTools = append(builtInTools, llm.Tool{
		Name:        "EndRollCall",
		Description: "End an active roll call in the current channel",
		Schema:      EndRollCallArgs{},
		Resolver:    p.toolResolveEndRollCall,
	})

	builtInTools = append(builtInTools, llm.Tool{
		Name:        "GenerateRollup",
		Description: "Generate a daily or weekly rollup report of tasks and activities",
		Schema:      RollupArgs{},
		Resolver:    p.toolResolveGenerateRollup,
	})

	return builtInTools
}
