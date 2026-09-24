package ui

import (
	"context"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/mhuggins7278/gh-projects-tui/internal/github"
)

type apiStatusSource struct {
	fakePickerSource
	status github.APIStatus
	ledger map[string]int
}

func (s apiStatusSource) Discover(ctx context.Context) (github.Discovery, error) {
	return s.fakePickerSource.Discover(ctx)
}

func (s apiStatusSource) APIStatus() github.APIStatus { return s.status }

func (s apiStatusSource) Ledger() map[string]int { return s.ledger }

func TestAPIToggleShowsLedgerBreakdown(t *testing.T) {
	source := apiStatusSource{
		fakePickerSource: fakePickerSource{discovery: testDiscovery()},
		status:           github.APIStatus{Requests: 700, Remaining: 4200},
		ledger: map[string]int{
			"query OrganizationProjectItems": 400,
			"query UserProjectViews":         2,
		},
	}
	model := feedDiscovery(t, newPickerModel(source), source.fakePickerSource)
	model.SetDebug(true)
	updated, _ := model.Update(keyPress("A"))
	result := updated.(Model)
	if !result.showAPI {
		t.Fatal("A did not open the API inspector")
	}
	content := ansi.Strip(result.View().Content)
	if !strings.Contains(content, "API requests this run: 700") {
		t.Fatalf("inspector missing totals: %q", content)
	}
	if !strings.Contains(content, "400  query OrganizationProjectItems") {
		t.Fatalf("inspector missing breakdown: %q", content)
	}
	updated, _ = result.Update(keyPress("A"))
	if updated.(Model).showAPI {
		t.Fatal("second A did not close the inspector")
	}
}

func TestAPIInspectorWithoutTelemetry(t *testing.T) {
	model := feedDiscovery(t, newPickerModel(fakePickerSource{discovery: testDiscovery()}), fakePickerSource{discovery: testDiscovery()})
	model.SetDebug(true)
	updated, _ := model.Update(keyPress("A"))
	content := ansi.Strip(updated.(Model).View().Content)
	if !strings.Contains(content, "telemetry is unavailable") {
		t.Fatalf("expected unavailable message, got: %q", content)
	}
}

func TestAPIToggleIgnoredWithoutDebug(t *testing.T) {
	source := apiStatusSource{
		fakePickerSource: fakePickerSource{discovery: testDiscovery()},
		status:           github.APIStatus{Requests: 700, Remaining: 4200},
	}
	model := feedDiscovery(t, newPickerModel(source), source.fakePickerSource)
	updated, _ := model.Update(keyPress("A"))
	result := updated.(Model)
	if result.showAPI {
		t.Fatal("A should do nothing without --debug")
	}
	if strings.Contains(ansi.Strip(result.View().Content), "API requests this run") {
		t.Fatal("inspector leaked without --debug")
	}
}

func TestAPIToggleIgnoredWhileFiltering(t *testing.T) {
	source := fakePickerSource{discovery: testDiscovery()}
	model := feedDiscovery(t, newPickerModel(source), source)
	updated, _ := model.Update(keyPress("/"))
	model = updated.(Model)
	updated, _ = model.Update(keyPress("A"))
	// While filtering, "A" is filter text, not the inspector toggle.
	if updated.(Model).showAPI {
		t.Fatal("A should not toggle the inspector while filtering")
	}
}
