// Copyright (c) 2023-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package project_management

// ExistingEntityDisambiguationResponse represents parsed user response to existing entity selection
type ExistingEntityDisambiguationResponse struct {
	Intent        string `json:"intent"`         // "create_new", "use_existing", "cancel"
	SelectedIndex int    `json:"selected_index"` // 1-based index for use_existing
	Reasoning     string `json:"reasoning"`      // LLM reasoning
}

// ProjectSelectionResponse represents parsed user response to project selection
type ProjectSelectionResponse struct {
	Intent        string `json:"intent"`         // "select_project", "no_project", "cancel"
	SelectedIndex int    `json:"selected_index"` // 1-based index for select_project
	Reasoning     string `json:"reasoning"`      // LLM reasoning
}

// ConfirmationResponse represents parsed user response to confirmation
type ConfirmationResponse struct {
	Intent        string                 `json:"intent"`        // "confirm", "modify", "cancel"
	Modifications map[string]interface{} `json:"modifications"` // Fields to modify
	Reasoning     string                 `json:"reasoning"`     // LLM reasoning
}
