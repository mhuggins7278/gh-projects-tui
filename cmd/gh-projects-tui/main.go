package main

import (
	"flag"
	"fmt"
	"os"

	tea "charm.land/bubbletea/v2"
	"github.com/mhuggins7278/gh-projects-tui/internal/config"
	"github.com/mhuggins7278/gh-projects-tui/internal/github"
	"github.com/mhuggins7278/gh-projects-tui/internal/ui"
)

func main() {
	owner := flag.String("owner", "", "owner login")
	project := flag.Int("project", 0, "owner-scoped project number")
	view := flag.Int("view", 0, "project-scoped view number")
	flag.Parse()
	if *project < 0 || *view < 0 {
		fmt.Fprintln(os.Stderr, "--project and --view must be positive")
		os.Exit(2)
	}
	host := config.DefaultHost()
	remembered, _ := config.Load(host)
	explicit := config.Selection{Owner: *owner, Project: *project, View: *view}
	// A project needs an explicit or remembered owner; a view needs an
	// explicit or remembered project. Ancestor changes discard descendants.
	if explicit.Project != 0 && explicit.Owner == "" && remembered.Owner == "" {
		fmt.Fprintln(os.Stderr, "--project requires --owner (or a remembered owner)")
		os.Exit(2)
	}
	if explicit.View != 0 && explicit.Project == 0 && remembered.Project == 0 {
		fmt.Fprintln(os.Stderr, "--view requires --project (or a remembered project)")
		os.Exit(2)
	}
	if explicit.View != 0 && explicit.Owner == "" && remembered.Owner == "" {
		fmt.Fprintln(os.Stderr, "--view requires --owner (or a remembered owner)")
		os.Exit(2)
	}

	client, err := github.NewClient()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	model := ui.NewModelWithHost(client, ui.Selection{OwnerLogin: *owner, ProjectNumber: *project, ViewNumber: *view}, host)
	if _, err := tea.NewProgram(model).Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
