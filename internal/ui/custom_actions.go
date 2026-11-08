package ui

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"text/template"
	"text/template/parse"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/mhuggins7278/gh-projects-tui/internal/config"
	"github.com/mhuggins7278/gh-projects-tui/internal/github"
)

type customActionDoneMsg struct {
	scope        boardReadScope
	itemID, name string
	err          error
}
type customActionPreparedMsg struct {
	id                uint64
	scope             boardReadScope
	itemID, contentID string
	action            config.Action
	values            map[string]string
	repoPath          string
	err               error
}

// SetActions installs previously validated user configuration.
func (m *Model) SetActions(actions []config.Action) {
	m.customActions = append([]config.Action(nil), actions...)
}

func (m *Model) SetActionConfig(cfg config.ActionsConfig) {
	m.SetActions(cfg.Actions)
	m.repoPaths = make(map[string]string, len(cfg.RepoPaths))
	for key, path := range cfg.RepoPaths {
		m.repoPaths[key] = path
	}
	m.repoSearchRoots = append([]string(nil), cfg.RepoSearchRoots...)
}

func (m Model) customActionContext() (*github.Content, string, bool) {
	if m.detailVisible {
		if m.detail == nil || m.detail.Content == nil {
			return nil, "", false
		}
		for _, item := range m.tableItems() {
			if item.ID == m.detailItemID {
				if item.OutsideProject {
					return m.detail.Content, "", true
				}
				return m.detail.Content, item.ID, true
			}
		}
		return m.detail.Content, "", true
	}
	item, ok := m.selectedItem()
	if !ok || item.Content == nil {
		return nil, "", false
	}
	if item.OutsideProject {
		return item.Content, "", true
	}
	return item.Content, item.ID, true
}

func (m Model) customActionCommand(action config.Action) (*exec.Cmd, string, error) {
	values, itemID, err := m.customActionValues(action)
	if err != nil {
		return nil, "", err
	}
	command, err := buildCustomActionCommand(action, values, m.host)
	return command, itemID, err
}

func (m Model) customActionValues(action config.Action) (map[string]string, string, error) {
	content, itemID, ok := m.customActionContext()
	if !ok || content == nil {
		return nil, "", fmt.Errorf("no accessible item is selected")
	}
	kind := ""
	switch content.Kind {
	case "Issue":
		kind = "issue"
	case "PullRequest":
		kind = "pr"
	}
	if kind == "" || kind != action.On {
		return nil, "", fmt.Errorf("%s requires a selected %s", action.Name, action.On)
	}
	if content.Number <= 0 || content.RepositoryFullName == "" || content.ID == "" {
		return nil, "", fmt.Errorf("issue or pull request identity is unavailable; refresh and try again")
	}
	if m.selectedOwner == nil || m.selectedProject == nil || m.view == nil {
		return nil, "", fmt.Errorf("project context is unavailable")
	}
	values := map[string]string{
		"RepoName":      content.RepositoryFullName,
		"Host":          m.host,
		"Number":        strconv.Itoa(content.Number),
		"URL":           content.URL,
		"Title":         content.Title,
		"ProjectOwner":  m.selectedOwner.Login,
		"ProjectNumber": strconv.Itoa(m.selectedProject.Number),
		"ProjectID":     m.view.ProjectID,
		"ContentID":     content.ID,
	}
	if kind == "issue" {
		values["IssueNumber"] = strconv.Itoa(content.Number)
		values["IssueTitle"] = content.Title
	} else {
		values["PRNumber"] = strconv.Itoa(content.Number)
		values["PrNumber"] = strconv.Itoa(content.Number)
	}
	if itemID != "" {
		values["ProjectItemID"] = itemID
	}
	return values, itemID, nil
}

func buildCustomActionCommand(action config.Action, values map[string]string, host string) (*exec.Cmd, error) {
	args := make([]string, 0, len(action.Run))
	for _, argument := range action.Run {
		t, err := template.New("argument").Option("missingkey=error").Parse(argument)
		if err != nil {
			return nil, fmt.Errorf("invalid action template: %w", err)
		}
		var out bytes.Buffer
		if err := t.Execute(&out, values); err != nil {
			return nil, fmt.Errorf("action %s: %w", action.Name, err)
		}
		args = append(args, out.String())
	}
	if len(args) == 0 || strings.TrimSpace(args[0]) == "" {
		return nil, fmt.Errorf("action has no executable")
	}
	if strings.HasPrefix(args[0], "~/") || (filepath.Separator == '\\' && strings.HasPrefix(args[0], `~\`)) {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("resolve action executable: %w", err)
		}
		args[0] = filepath.Join(home, args[0][2:])
	}
	command := exec.Command(args[0], args[1:]...)
	// Commands using gh inherit the same selected host as the TUI.
	for _, entry := range os.Environ() {
		if !strings.HasPrefix(strings.ToUpper(entry), "GH_HOST=") {
			command.Env = append(command.Env, entry)
		}
	}
	command.Env = append(command.Env, "GH_HOST="+host)
	return command, nil
}

func actionNeedsRepoPath(action config.Action) bool {
	var contains func(parse.Node) bool
	contains = func(node parse.Node) bool {
		switch n := node.(type) {
		case *parse.ListNode:
			if n != nil {
				for _, child := range n.Nodes {
					if contains(child) {
						return true
					}
				}
			}
		case *parse.ActionNode:
			return contains(n.Pipe)
		case *parse.PipeNode:
			if n != nil {
				for _, child := range n.Cmds {
					if contains(child) {
						return true
					}
				}
			}
		case *parse.CommandNode:
			if len(n.Args) >= 3 {
				if name, ok := n.Args[0].(*parse.IdentifierNode); ok && name.Ident == "index" {
					if key, ok := n.Args[2].(*parse.StringNode); !ok || key.Text == "RepoPath" {
						return true
					}
				}
			}
			for _, child := range n.Args {
				if contains(child) {
					return true
				}
			}
		case *parse.FieldNode:
			return len(n.Ident) > 0 && n.Ident[0] == "RepoPath"
		case *parse.ChainNode:
			for _, field := range n.Field {
				if field == "RepoPath" {
					return true
				}
			}
			return contains(n.Node)
		case *parse.VariableNode:
			if len(n.Ident) > 1 {
				for _, field := range n.Ident[1:] {
					if field == "RepoPath" {
						return true
					}
				}
			}
		case *parse.IfNode:
			return contains(n.Pipe) || contains(n.List) || contains(n.ElseList)
		case *parse.RangeNode:
			return contains(n.Pipe) || contains(n.List) || contains(n.ElseList)
		case *parse.WithNode:
			return contains(n.Pipe) || contains(n.List) || contains(n.ElseList)
		case *parse.TemplateNode:
			return contains(n.Pipe)
		case *parse.StringNode:
			return n.Text == "RepoPath"
		}
		return false
	}
	for _, argument := range action.Run {
		t, err := template.New("argument").Parse(argument)
		if err != nil {
			continue
		}
		for _, parsed := range t.Templates() {
			if parsed.Tree != nil && contains(parsed.Tree.Root) {
				return true
			}
		}
	}
	return false
}

func (m *Model) startCustomAction(action config.Action) tea.Cmd {
	if m.isTimeline() {
		m.status = "Actions are unavailable in read-only timelines"
		return nil
	}
	if m.itemsLoading || m.loadingDetail || m.detailLoading || m.mutationLoading || m.mutationSession != nil || m.issueActionPending != nil || m.tableAction != nil || m.actionRunning {
		m.status = "Wait for pending reads and writes before running an action"
		return nil
	}
	values, itemID, err := m.customActionValues(action)
	if err != nil {
		m.status = err.Error()
		return nil
	}
	if m.detailVisible {
		itemID = m.detailItemID
	}
	scope := m.currentBoardReadScope()
	if actionNeedsRepoPath(action) {
		cwd, _ := os.Getwd()
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		m.actionPrepareID++
		id := m.actionPrepareID
		m.actionRunning, m.actionPreparing, m.actionCancel = true, true, cancel
		m.status = "Finding checkout for " + values["RepoName"] + "... · esc cancels"
		paths, roots := m.repoPaths, m.repoSearchRoots
		return func() tea.Msg {
			defer cancel()
			path, err := config.ResolveRepoPath(ctx, scope.host, values["RepoName"], cwd, paths, roots)
			return customActionPreparedMsg{id: id, scope: scope, itemID: itemID, contentID: values["ContentID"], action: action, values: values, repoPath: path, err: err}
		}
	}
	command, err := buildCustomActionCommand(action, values, m.host)
	if err != nil {
		m.status = err.Error()
		return nil
	}
	m.actionRunning = true
	m.status = "Running " + action.Name + "..."
	return tea.ExecProcess(command, func(err error) tea.Msg {
		return customActionDoneMsg{scope: scope, itemID: itemID, name: action.Name, err: err}
	})
}

func (m Model) updateCustomActionPrepared(msg customActionPreparedMsg) (tea.Model, tea.Cmd) {
	if !m.actionPreparing || msg.id != m.actionPrepareID {
		return m, nil
	}
	m.actionPreparing, m.actionCancel = false, nil
	if msg.err != nil {
		m.actionRunning = false
		m.status = "Cannot run " + msg.action.Name + ": " + msg.err.Error()
		return m, nil
	}
	content, _, ok := m.customActionContext()
	if !msg.scope.matches(m) || !ok || content.ID != msg.contentID || m.itemsLoading || m.loadingDetail || m.detailLoading || m.mutationLoading || m.mutationSession != nil || m.issueActionPending != nil || m.tableAction != nil {
		m.actionRunning = false
		m.status = "Selection changed; choose the action again"
		return m, nil
	}
	msg.values["RepoPath"] = msg.repoPath
	command, err := buildCustomActionCommand(msg.action, msg.values, msg.scope.host)
	if err != nil {
		m.actionRunning = false
		m.status = err.Error()
		return m, nil
	}
	m.status = "Running " + msg.action.Name + "..."
	return m, tea.ExecProcess(command, func(err error) tea.Msg {
		return customActionDoneMsg{scope: msg.scope, itemID: msg.itemID, name: msg.action.Name, err: err}
	})
}

func (m Model) updateCustomActionDone(msg customActionDoneMsg) (tea.Model, tea.Cmd) {
	m.actionRunning = false
	if msg.err != nil {
		m.status = fmt.Sprintf("%s failed: %v", msg.name, msg.err)
	} else {
		m.status = msg.name + " finished"
	}
	if invalidator, ok := m.source.(interface{ InvalidateReads() }); ok {
		invalidator.InvalidateReads()
	}
	m.detailCache = make(map[string]github.ItemDetail)
	if !msg.scope.matches(m) {
		return m, nil
	}
	if m.detailVisible && m.detailItemID == msg.itemID {
		scope := m.currentBoardReadScope()
		m.pendingIssueReload = &scope
		m.detailLoading = true
		m.detailErr = nil
		m.detailRequestID++
		return m, m.loadItemDetailCmd(msg.itemID, m.detailRequestID, scope)
	}
	if m.itemsLoading {
		scope := m.currentBoardReadScope()
		m.pendingMutationReload = &scope
		return m, nil
	}
	return m, m.startItemsLoadPreservingFocus()
}

func (m Model) customActionHelp() string {
	if len(m.customActions) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("\nCustom actions:\n")
	for _, action := range m.customActions {
		fmt.Fprintf(&b, "%s  %s (%s)\n", action.Key, action.Name, action.On)
	}
	return strings.TrimRight(b.String(), "\n")
}
