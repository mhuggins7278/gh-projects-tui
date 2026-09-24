package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/mhuggins7278/gh-projects-tui/internal/config"
	"github.com/mhuggins7278/gh-projects-tui/internal/github"
	"github.com/mhuggins7278/gh-projects-tui/internal/ui"
)

func main() {
	owner := flag.String("owner", "", "owner login")
	project := flag.Int("project", 0, "owner-scoped project number")
	view := flag.Int("view", 0, "project-scoped view number")
	debug := flag.Bool("debug", false, "show API telemetry in the header and enable the A inspector overlay (or GH_PROJECTS_TUI_DEBUG=1)")
	flag.Parse()
	host := config.DefaultHost()
	if err := validateFlags(*owner, *project, *view); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}

	client, err := github.NewClient()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	selection := ui.Selection{OwnerLogin: *owner, ProjectNumber: *project, ViewNumber: *view}
	model := ui.NewModelWithHost(client, selection, host)
	model.SetDebug(*debug || debugEnv())
	if _, err := tea.NewProgram(model).Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func debugEnv() bool {
	value := strings.TrimSpace(strings.ToLower(os.Getenv("GH_PROJECTS_TUI_DEBUG")))
	return value == "1" || value == "true" || value == "yes"
}

func validateFlags(owner string, project, view int) error {
	if project < 0 || view < 0 {
		return fmt.Errorf("--project and --view must be positive")
	}
	if project != 0 && owner == "" {
		return fmt.Errorf("--project requires --owner")
	}
	if view != 0 && project == 0 {
		return fmt.Errorf("--view requires --project")
	}
	if view != 0 && owner == "" {
		return fmt.Errorf("--view requires --owner")
	}
	return nil
}
