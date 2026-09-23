package ui

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/mhuggins7278/gh-projects-tui/internal/github"
)

func runMutationCommands(t *testing.T, model Model, cmd tea.Cmd) Model {
	t.Helper()
	for step := 0; cmd != nil; step++ {
		if step >= 50 {
			t.Fatal("mutation command pipeline did not settle")
		}
		message := cmd()
		updated, next := model.Update(message)
		model = updated.(Model)
		cmd = next
	}
	return model
}

func writableStatusView() github.View {
	return github.View{
		Number: 1, ProjectID: "project", ViewerCanUpdate: true,
		GroupByFields: []github.Field{{
			ID: "status", Name: "Status", Kind: "ProjectV2SingleSelectField", DataType: "SINGLE_SELECT",
			Options: []github.FieldOption{{ID: "a", Name: "A"}, {ID: "b", Name: "B"}, {ID: "c", Name: "C"}},
		}},
	}
}

func mutationModel(source fakePickerSource, view github.View, items []github.Item) Model {
	model := newPickerModel(source)
	model.screen = screenBoard
	model.view = &view
	model.selectedOwner = &github.Owner{Login: "org", Kind: github.OrganizationOwner}
	model.selectedProject = &github.Project{Number: 1}
	model.items = cloneItems(items)
	return model
}

func TestQueuedLaneIntentsResolveSeriallyAndContinueAfterDefinitiveFailure(t *testing.T) {
	for _, failFirst := range []bool{false, true} {
		name := "success"
		if failFirst {
			name = "first operation rejected"
		}
		t.Run(name, func(t *testing.T) {
			view := writableStatusView()
			mutations := []string{}
			item := github.Item{ID: "item", FieldValues: []github.FieldValue{{FieldID: "status", OptionID: "a", Available: true}}}
			canonical := []github.Item{item}
			var mutationErrors []error
			if failFirst {
				mutationErrors = []error{&github.MutationError{Err: errors.New("permission denied"), Ambiguous: false}}
			}
			source := fakePickerSource{mutations: &mutations, mutationErrors: &mutationErrors, canonicalItems: &canonical}
			model := mutationModel(source, view, canonical)
			model.boardLane = 1

			updated, first := model.Update(keyPress("L"))
			model = updated.(Model)
			updated, second := model.Update(keyPress("L"))
			model = updated.(Model)
			if second != nil || len(model.mutationSession.intents) != 2 || model.boardLane != 3 {
				t.Fatalf("queued lane intent state = session:%#v lane:%d cmd nil:%v", model.mutationSession, model.boardLane, second == nil)
			}
			if len(mutations) != 0 {
				t.Fatalf("write started before canonical readback: %v", mutations)
			}
			model = runMutationCommands(t, model, first)
			if model.mutationSession != nil || model.mutationLoading {
				t.Fatalf("queue did not settle: session=%#v loading=%v", model.mutationSession, model.mutationLoading)
			}
			wantLane := "c"
			wantCalls := []string{"field:b", "field:c"}
			if failFirst {
				wantLane = "b"
				wantCalls = []string{"field:b", "field:b"}
			}
			if got := canonical[0].FieldValues[0].OptionID; got != wantLane {
				t.Fatalf("final canonical lane = %q, want %s", got, wantLane)
			}
			if !reflect.DeepEqual(mutations, wantCalls) {
				t.Fatalf("serialized mutation calls = %#v", mutations)
			}
			if failFirst && !strings.Contains(model.status, "Move card failed") {
				t.Fatalf("first failure was not reported: %q", model.status)
			}
		})
	}
}

func TestQueuedReordersUseFreshProjectAnchorsAcrossInterleavedItems(t *testing.T) {
	view := writableStatusView()
	mutations := []string{}
	canonical := []github.Item{
		{ID: "a", FieldValues: []github.FieldValue{{FieldID: "status", OptionID: "a", Available: true}}},
		{ID: "x", FieldValues: []github.FieldValue{{FieldID: "status", OptionID: "b", Available: true}}},
		{ID: "b", FieldValues: []github.FieldValue{{FieldID: "status", OptionID: "a", Available: true}}},
		{ID: "y", FieldValues: []github.FieldValue{{FieldID: "status", OptionID: "b", Available: true}}},
		{ID: "c", FieldValues: []github.FieldValue{{FieldID: "status", OptionID: "a", Available: true}}},
	}
	model := mutationModel(fakePickerSource{mutations: &mutations, canonicalItems: &canonical}, view, canonical)
	model.boardLane = 1
	model.boardCard = 0

	updated, first := model.Update(keyPress("J"))
	model = updated.(Model)
	updated, second := model.Update(keyPress("J"))
	model = updated.(Model)
	if second != nil || len(model.mutationSession.intents) != 2 {
		t.Fatalf("reorder intents = %#v; second command nil=%v", model.mutationSession, second == nil)
	}
	model = runMutationCommands(t, model, first)
	if !reflect.DeepEqual(mutations, []string{"position:b", "position:c"}) {
		t.Fatalf("reorders used stale or wrong anchors: %#v", mutations)
	}
	got := make([]string, len(canonical))
	for index, item := range canonical {
		got[index] = item.ID
	}
	if want := []string{"x", "b", "y", "c", "a"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("canonical project order = %v, want %v", got, want)
	}
}

func TestViewSwitchDoesNotCancelOrStaleAnInFlightMutation(t *testing.T) {
	view := writableStatusView()
	canonical := []github.Item{{ID: "item", FieldValues: []github.FieldValue{{FieldID: "status", OptionID: "a", Available: true}}}}
	mutations := []string{}
	gate := make(chan struct{})
	var writeContextErr error
	source := fakePickerSource{mutations: &mutations, canonicalItems: &canonical, mutationWait: gate, mutationContextErr: &writeContextErr, views: []github.ViewSummary{{Number: 2, Name: "Other", Layout: github.BoardLayout}}}
	model := mutationModel(source, view, canonical)
	model.boardLane = 1
	updated, mutationCmd := model.Update(keyPress("L"))
	model = updated.(Model)
	if mutationCmd == nil || model.mutationSession == nil {
		t.Fatal("mutation did not start before the view switch")
	}
	preflightMessage := mutationCmd()
	updated, saveCmd := model.Update(preflightMessage)
	model = updated.(Model)
	if saveCmd == nil || model.mutationSession.plan == nil {
		t.Fatal("preflight did not dispatch the submitted mutation")
	}
	saveResult := make(chan tea.Msg, 1)
	go func() { saveResult <- saveCmd() }()

	updated, viewsCmd := model.Update(keyPress("v"))
	model = updated.(Model)
	if viewsCmd == nil || model.screen != screenViewPicker || model.mutationSession == nil {
		t.Fatalf("view switch stranded the write: screen=%v session=%#v", model.screen, model.mutationSession)
	}
	model = runMutationCommands(t, model, viewsCmd)
	close(gate)
	updated, readbackCmd := model.Update(<-saveResult)
	model = updated.(Model)
	model = runMutationCommands(t, model, readbackCmd)
	if model.screen != screenViewPicker || model.view != nil || model.mutationSession != nil || canonical[0].FieldValues[0].OptionID != "b" || writeContextErr != nil {
		t.Fatalf("late mutation result overwrote picker state or failed to save: screen=%v view=%#v session=%#v items=%#v", model.screen, model.view, model.mutationSession, canonical)
	}
}

func TestAmbiguousMutationBlocksQueuedWritesUntilReadbackConfirmsOutcome(t *testing.T) {
	view := writableStatusView()
	canonical := []github.Item{{ID: "item", FieldValues: []github.FieldValue{{FieldID: "status", OptionID: "a", Available: true}}}}
	mutations := []string{}
	mutationErrors := []error{context.DeadlineExceeded}
	source := fakePickerSource{mutations: &mutations, mutationErrors: &mutationErrors, canonicalItems: &canonical}
	model := mutationModel(source, view, canonical)
	model.boardLane = 1
	updated, first := model.Update(keyPress("L"))
	model = updated.(Model)
	updated, _ = model.Update(keyPress("L"))
	model = updated.(Model)
	model = runMutationCommands(t, model, first)
	if model.mutationSession == nil || !model.mutationSession.blocked || !strings.Contains(model.status, "press r to") {
		t.Fatalf("ambiguous result did not block queue: session=%#v status=%q", model.mutationSession, model.status)
	}
	if !reflect.DeepEqual(mutations, []string{"field:b"}) {
		t.Fatalf("dependent write ran before reconciliation: %v", mutations)
	}

	canonical[0].FieldValues[0].OptionID = "b"
	updated, retry := model.Update(keyPress("r"))
	model = updated.(Model)
	model = runMutationCommands(t, model, retry)
	if model.mutationSession != nil || canonical[0].FieldValues[0].OptionID != "c" {
		t.Fatalf("confirmed ambiguous write did not unblock and resolve the queued intent: session=%#v items=%#v", model.mutationSession, canonical)
	}
	if !reflect.DeepEqual(mutations, []string{"field:b", "field:c"}) {
		t.Fatalf("mutation calls after confirmation = %v", mutations)
	}
}

func TestRepeatedUnchangedReadbackResolvesAmbiguousWriteBeforeContinuing(t *testing.T) {
	view := writableStatusView()
	canonical := []github.Item{{ID: "item", FieldValues: []github.FieldValue{{FieldID: "status", OptionID: "a", Available: true}}}}
	mutations := []string{}
	mutationErrors := []error{context.DeadlineExceeded}
	source := fakePickerSource{mutations: &mutations, mutationErrors: &mutationErrors, canonicalItems: &canonical}
	model := mutationModel(source, view, canonical)
	model.boardLane = 1
	updated, first := model.Update(keyPress("L"))
	model = updated.(Model)
	updated, _ = model.Update(keyPress("L"))
	model = updated.(Model)
	model = runMutationCommands(t, model, first)
	if model.mutationSession == nil || !model.mutationSession.blocked || len(mutations) != 1 {
		t.Fatalf("first unchanged readback did not pause the queue: session=%#v calls=%v", model.mutationSession, mutations)
	}

	updated, retry := model.Update(keyPress("r"))
	model = updated.(Model)
	model = runMutationCommands(t, model, retry)
	if model.mutationSession != nil || canonical[0].FieldValues[0].OptionID != "b" {
		t.Fatalf("stable unchanged readback did not resolve the timed-out write: session=%#v item=%#v", model.mutationSession, canonical[0])
	}
	if !reflect.DeepEqual(mutations, []string{"field:b", "field:b"}) {
		t.Fatalf("timed-out write was retried or queued intent lost: %v", mutations)
	}
}

func TestStaleReorderIntentIsDroppedBeforeSubmittingAnOldAnchor(t *testing.T) {
	view := writableStatusView()
	canonical := []github.Item{
		{ID: "a", FieldValues: []github.FieldValue{{FieldID: "status", OptionID: "a", Available: true}}},
		{ID: "b", FieldValues: []github.FieldValue{{FieldID: "status", OptionID: "a", Available: true}}},
		{ID: "c", FieldValues: []github.FieldValue{{FieldID: "status", OptionID: "a", Available: true}}},
	}
	mutations := []string{}
	model := mutationModel(fakePickerSource{mutations: &mutations, canonicalItems: &canonical}, view, canonical)
	model.boardLane = 1
	model.boardCard = 1
	updated, cmd := model.Update(keyPress("J"))
	model = updated.(Model)
	// The saved project changes after the keypress but before the preflight read.
	canonical = []github.Item{canonical[0], canonical[1]}
	model = runMutationCommands(t, model, cmd)
	if len(mutations) != 0 || model.mutationSession != nil || !strings.Contains(model.status, "no longer move") {
		t.Fatalf("stale reorder was submitted: calls=%v session=%#v status=%q", mutations, model.mutationSession, model.status)
	}
}

func TestAnchorDisappearingAfterPreflightBlocksDependentWrites(t *testing.T) {
	view := writableStatusView()
	canonical := []github.Item{
		{ID: "a", FieldValues: []github.FieldValue{{FieldID: "status", OptionID: "a", Available: true}}},
		{ID: "b", FieldValues: []github.FieldValue{{FieldID: "status", OptionID: "a", Available: true}}},
		{ID: "c", FieldValues: []github.FieldValue{{FieldID: "status", OptionID: "a", Available: true}}},
	}
	mutations := []string{}
	model := mutationModel(fakePickerSource{mutations: &mutations, canonicalItems: &canonical}, view, canonical)
	model.boardLane = 1
	model.boardCard = 1
	updated, preflight := model.Update(keyPress("J"))
	model = updated.(Model)
	readbackMessage := preflight()
	updated, write := model.Update(readbackMessage)
	model = updated.(Model)
	if write == nil {
		t.Fatal("preflight did not start the reorder")
	}

	// The anchor is concurrently removed after canonical planning but before submission.
	canonical = canonical[:2]
	writeMessage := write()
	updated, reconcile := model.Update(writeMessage)
	model = updated.(Model)
	model = runMutationCommands(t, model, reconcile)
	if model.mutationSession == nil || !model.mutationSession.blocked || !strings.Contains(model.status, "Outcome") {
		t.Fatalf("disappeared anchor did not block dependent writes: session=%#v status=%q", model.mutationSession, model.status)
	}
	if !reflect.DeepEqual(mutations, []string{"position:c"}) {
		t.Fatalf("stale anchor triggered unexpected mutations: %v", mutations)
	}
}

func TestLaneDirectionIsResolvedAgainstCanonicalCurrentLane(t *testing.T) {
	view := writableStatusView()
	canonical := []github.Item{{ID: "item", FieldValues: []github.FieldValue{{FieldID: "status", OptionID: "a", Available: true}}}}
	mutations := []string{}
	model := mutationModel(fakePickerSource{mutations: &mutations, canonicalItems: &canonical}, view, canonical)
	model.boardLane = 1
	updated, cmd := model.Update(keyPress("L"))
	model = updated.(Model)
	// Another client moves the card one lane before the preflight read starts.
	canonical[0].FieldValues[0].OptionID = "b"
	model = runMutationCommands(t, model, cmd)
	if canonical[0].FieldValues[0].OptionID != "c" || !reflect.DeepEqual(mutations, []string{"field:c"}) {
		t.Fatalf("lane direction used a stale destination: item=%#v mutations=%v", canonical[0], mutations)
	}
}

func TestReadbackFailurePreventsMutationUntilRetry(t *testing.T) {
	view := writableStatusView()
	canonical := []github.Item{{ID: "item", FieldValues: []github.FieldValue{{FieldID: "status", OptionID: "a", Available: true}}}}
	mutations := []string{}
	source := fakePickerSource{mutations: &mutations, canonicalItems: &canonical, canonicalErr: errors.New("read unavailable")}
	model := mutationModel(source, view, canonical)
	model.boardLane = 1
	updated, cmd := model.Update(keyPress("L"))
	model = updated.(Model)
	model = runMutationCommands(t, model, cmd)
	if model.mutationSession == nil || !model.mutationSession.blocked || len(mutations) != 0 {
		t.Fatalf("write was not held behind failed readback: session=%#v mutations=%v", model.mutationSession, mutations)
	}

	model.source = fakePickerSource{mutations: &mutations, canonicalItems: &canonical}
	updated, retry := model.Update(keyPress("r"))
	model = updated.(Model)
	model = runMutationCommands(t, model, retry)
	if model.mutationSession != nil || canonical[0].FieldValues[0].OptionID != "b" || !reflect.DeepEqual(mutations, []string{"field:b"}) {
		t.Fatalf("retry did not read canonical state before saving: session=%#v canonical=%#v mutations=%v", model.mutationSession, canonical, mutations)
	}
}

func TestCanonicalReadbackPaginatesTheWholeProject(t *testing.T) {
	first := github.ItemsPage{Items: []github.Item{{ID: "one"}}, HasNext: true, EndCursor: "cursor-1"}
	second := github.ItemsPage{Items: []github.Item{{ID: "two"}}}
	project := []github.Item{}
	source := fakePickerSource{
		canonicalItems: &project,
		canonicalPages: map[string]github.ItemsPage{"": first, "cursor-1": second},
	}
	items, err := readCanonicalProjectItems(context.Background(), source, github.Owner{Login: "org", Kind: github.OrganizationOwner}, 1)
	if err != nil {
		t.Fatalf("readCanonicalProjectItems() error = %v", err)
	}
	if got := []string{items[0].ID, items[1].ID}; !reflect.DeepEqual(got, []string{"one", "two"}) {
		t.Fatalf("canonical page items = %v", got)
	}
}

func TestSecondStepFailureReportsPartialMoveFromCanonicalReadback(t *testing.T) {
	view := writableStatusView()
	canonical := []github.Item{
		{ID: "a", FieldValues: []github.FieldValue{{FieldID: "status", OptionID: "a", Available: true}}},
		{ID: "b", FieldValues: []github.FieldValue{{FieldID: "status", OptionID: "b", Available: true}}},
	}
	mutations := []string{}
	source := fakePickerSource{
		mutations:           &mutations,
		canonicalItems:      &canonical,
		positionMutationErr: &github.MutationError{Err: errors.New("position rejected"), Ambiguous: false},
	}
	model := mutationModel(source, view, canonical)
	model.boardLane = 1
	updated, cmd := model.Update(keyPress("L"))
	model = updated.(Model)
	model = runMutationCommands(t, model, cmd)
	if model.mutationSession != nil || !strings.Contains(model.status, "partially saved") {
		t.Fatalf("partial second-step failure was not reconciled: session=%#v status=%q", model.mutationSession, model.status)
	}
	if canonical[0].FieldValues[0].OptionID != "b" || !reflect.DeepEqual(mutations, []string{"field:b", "position:b"}) {
		t.Fatalf("partial mutation state = items:%#v calls:%v", canonical, mutations)
	}
}

func TestAmbiguousSecondStepWaitsForStablePartialReadback(t *testing.T) {
	view := writableStatusView()
	canonical := []github.Item{
		{ID: "a", FieldValues: []github.FieldValue{{FieldID: "status", OptionID: "a", Available: true}}},
		{ID: "b", FieldValues: []github.FieldValue{{FieldID: "status", OptionID: "b", Available: true}}},
	}
	mutations := []string{}
	source := fakePickerSource{
		mutations:           &mutations,
		canonicalItems:      &canonical,
		positionMutationErr: context.DeadlineExceeded,
	}
	model := mutationModel(source, view, canonical)
	model.boardLane = 1
	updated, cmd := model.Update(keyPress("L"))
	model = updated.(Model)
	model = runMutationCommands(t, model, cmd)
	if model.mutationSession == nil || !model.mutationSession.blocked || canonical[0].FieldValues[0].OptionID != "b" {
		t.Fatalf("ambiguous second step was not held for readback: session=%#v items=%#v", model.mutationSession, canonical)
	}
	updated, retry := model.Update(keyPress("r"))
	model = updated.(Model)
	model = runMutationCommands(t, model, retry)
	if model.mutationSession != nil || !strings.Contains(model.status, "partially saved") || canonical[0].FieldValues[0].OptionID != "b" {
		t.Fatalf("stable partial result was not reconciled: session=%#v status=%q items=%#v", model.mutationSession, model.status, canonical)
	}
	if !reflect.DeepEqual(mutations, []string{"field:b", "position:b"}) {
		t.Fatalf("ambiguous position mutation was resubmitted: %v", mutations)
	}
}
