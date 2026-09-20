package timber

import (
	"context"
	"encoding/json/v2"
	"errors"
	"fmt"
	"strings"
)

const (
	agentTabLabel = "Agent"
	shellTabLabel = "Shell"

	parentDashboardTabLabel       = "Status"
	parentDashboardRefreshSeconds = 60
)

type herdrResource struct {
	WorkspaceID string `json:"workspace_id"`
	TabID       string `json:"tab_id"`
	PaneID      string `json:"pane_id"`
}

type herdrWorkspaceCreateResponse struct {
	Result struct {
		Workspace   herdrResource `json:"workspace"`
		Tab         herdrResource `json:"tab"`
		RootPane    herdrResource `json:"root_pane"`
		AlreadyOpen bool          `json:"already_open"`
	} `json:"result"`
}

type herdrWorktreeListResponse struct {
	Result struct {
		Source struct {
			RepoName          string  `json:"repo_name"`
			RepoRoot          string  `json:"repo_root"`
			SourceWorkspaceID *string `json:"source_workspace_id"`
		} `json:"source"`
	} `json:"result"`
}

type herdrWorkspaceGetResponse struct {
	Result struct {
		Workspace struct {
			WorkspaceID string `json:"workspace_id"`
			Label       string `json:"label"`
		} `json:"workspace"`
	} `json:"result"`
}

type herdrTabCreateResponse struct {
	Result struct {
		Tab      herdrResource `json:"tab"`
		RootPane herdrResource `json:"root_pane"`
	} `json:"result"`
}

type herdrPaneCurrentResponse struct {
	Result struct {
		Pane herdrResource `json:"pane"`
	} `json:"result"`
}

type herdrSpace struct {
	runtime        Runtime
	workspaceID    string
	worktreePath   string
	agentTabID     string
	agentPaneID    string
	agentPaneLabel string
	alreadyOpen    bool
}

func parseHerdrSpace(runtime Runtime, output []byte, worktreePath string, agentPaneLabel string) (herdrSpace, error) {
	var response herdrWorkspaceCreateResponse
	if err := json.Unmarshal(output, &response); err != nil {
		return herdrSpace{}, fmt.Errorf("decode herdr workspace create response: %w", err)
	}

	space := herdrSpace{
		runtime:        runtime,
		workspaceID:    response.Result.Workspace.WorkspaceID,
		worktreePath:   worktreePath,
		agentTabID:     response.Result.Tab.TabID,
		agentPaneID:    response.Result.RootPane.PaneID,
		agentPaneLabel: agentPaneLabel,
		alreadyOpen:    response.Result.AlreadyOpen,
	}
	return space, space.validateInitialResources()
}

// qualifiedWorktreeName formats the worktree label Herdr panes display.
func qualifiedWorktreeName(worktreeName string, repoName string) string {
	return worktreeName + "@" + repoName
}

func parseHerdrWorkspaceID(output []byte) (string, error) {
	var response herdrWorkspaceCreateResponse
	if err := json.Unmarshal(output, &response); err != nil {
		return "", fmt.Errorf("decode herdr workspace create response: %w", err)
	}
	if response.Result.Workspace.WorkspaceID == "" {
		return "", errors.New("herdr workspace create response has no workspace ID")
	}
	return response.Result.Workspace.WorkspaceID, nil
}

func parseHerdrWorktreeListParent(output []byte) (string, error) {
	var response herdrWorktreeListResponse
	if err := json.Unmarshal(output, &response); err != nil {
		return "", fmt.Errorf("decode herdr worktree list response: %w", err)
	}
	if response.Result.Source.SourceWorkspaceID == nil {
		return "", nil
	}
	return *response.Result.Source.SourceWorkspaceID, nil
}

func parseHerdrWorkspaceLabel(output []byte, workspaceID string) (string, error) {
	var response herdrWorkspaceGetResponse
	if err := json.Unmarshal(output, &response); err != nil {
		return "", fmt.Errorf("decode herdr workspace get response: %w", err)
	}
	if response.Result.Workspace.WorkspaceID == "" {
		return "", errors.New("herdr workspace get response has no workspace ID")
	}
	if response.Result.Workspace.WorkspaceID != workspaceID {
		return "", fmt.Errorf("herdr workspace get response has unexpected workspace ID %q", response.Result.Workspace.WorkspaceID)
	}
	return response.Result.Workspace.Label, nil
}

func parseCurrentHerdrSpace(runtime Runtime, output []byte, worktreePath string, agentPaneLabel string) (herdrSpace, error) {
	var response herdrPaneCurrentResponse
	if err := json.Unmarshal(output, &response); err != nil {
		return herdrSpace{}, fmt.Errorf("decode herdr pane current response: %w", err)
	}

	space := herdrSpace{
		runtime:        runtime,
		workspaceID:    response.Result.Pane.WorkspaceID,
		worktreePath:   worktreePath,
		agentTabID:     response.Result.Pane.TabID,
		agentPaneID:    response.Result.Pane.PaneID,
		agentPaneLabel: agentPaneLabel,
	}
	if space.workspaceID == "" || space.agentTabID == "" || space.agentPaneID == "" {
		return herdrSpace{}, errors.New("herdr pane current response has incomplete pane resources")
	}
	return space, nil
}

func (x herdrSpace) validateInitialResources() error {
	if x.workspaceID == "" {
		return errors.New("herdr workspace create response has no workspace ID")
	}
	if x.agentTabID == "" {
		return errors.New("herdr workspace create response has no tab ID")
	}
	if x.agentPaneID == "" {
		return errors.New("herdr workspace create response has no root pane ID")
	}
	return nil
}

func (x herdrSpace) configure(ctx context.Context) error {
	if _, err := x.runtime.runHerdr(ctx, "tab", "rename", x.agentTabID, agentTabLabel); err != nil {
		return err
	}
	if _, err := x.runtime.runHerdr(ctx, "pane", "rename", x.agentPaneID, x.agentPaneLabel); err != nil {
		return err
	}
	if err := x.createTab(ctx, shellTabLabel); err != nil {
		return err
	}
	return x.startCommands(ctx)
}

func (x herdrSpace) createTab(ctx context.Context, label string) error {
	output, err := x.runtime.runHerdr(
		ctx,
		"tab", "create", "--workspace", x.workspaceID, "--cwd", x.worktreePath,
		"--label", label, "--no-focus",
	)
	if err != nil {
		return err
	}
	return parseHerdrTabCreateResponse(output)
}

func parseHerdrTabCreateResponse(output []byte) error {
	var response herdrTabCreateResponse
	if err := json.Unmarshal(output, &response); err != nil {
		return fmt.Errorf("decode herdr tab create response: %w", err)
	}
	if response.Result.Tab.TabID == "" || response.Result.RootPane.PaneID == "" {
		return errors.New("herdr tab create response has incomplete tab resources")
	}
	return nil
}

func (x herdrSpace) startCommands(ctx context.Context) error {
	_, err := x.runtime.runHerdr(ctx, "pane", "run", x.agentPaneID, "pi")
	return err
}

func (x herdrSpace) focus(ctx context.Context) error {
	if _, err := x.runtime.runHerdr(ctx, "workspace", "focus", x.workspaceID); err != nil {
		return err
	}
	_, err := x.runtime.runHerdr(ctx, "tab", "focus", x.agentTabID)
	return err
}

func (x herdrSpace) focusWorkspace(ctx context.Context) error {
	_, err := x.runtime.runHerdr(ctx, "workspace", "focus", x.workspaceID)
	return err
}

// parentDashboardCommand builds the refresh loop a parent workspace runs in
// its Status tab. Pull request status is included only when gh proved usable
// at setup; a later login takes effect the next time the parent is created.
func parentDashboardCommand(repoName string, withPullRequests bool) string {
	command := "timber list " + shellQuote("@"+repoName)
	if withPullRequests {
		command += " --pr"
	}
	return fmt.Sprintf("while :; do clear; %s; sleep %d; done", command, parentDashboardRefreshSeconds)
}

func shellQuote(value string) string {
	if value == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(value, "'", `'\''`) + "'"
}

func (x herdrSpace) close(ctx context.Context) error {
	_, err := x.runtime.runHerdr(ctx, "workspace", "close", x.workspaceID)
	return err
}
