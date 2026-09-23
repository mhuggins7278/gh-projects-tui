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

func writableCombinedView() github.View {
	priority := github.Field{ID: "priority", Name: "Priority", Kind: "ProjectV2SingleSelectField", DataType: "SINGLE_SELECT", Options: []github.FieldOption{{ID: "p0", Name: "P0"}, {ID: "p1", Name: "P1"}}}
	status := github.Field{ID: "status", Name: "Status", Kind: "ProjectV2SingleSelectField", DataType: "SINGLE_SELECT", Options: []github.FieldOption{{ID: "todo", Name: "Todo"}, {ID: "done", Name: "Done"}}}
	return github.View{Number: 1, ProjectID: "project", Layout: github.BoardLayout, ViewerCanUpdate: true, GroupByFields: []github.Field{priority}, VerticalGroupBy: []github.Field{status}}
}

func combinedItem(id, priority, status string) github.Item {
	item := github.Item{ID: id}
	if priority != "" {
		item.FieldValues = append(item.FieldValues, github.FieldValue{FieldID: "priority", OptionID: priority, Available: true})
	}
	if status != "" {
		item.FieldValues = append(item.FieldValues, github.FieldValue{FieldID: "status", OptionID: status, Available: true})
	}
	return item
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
			// Two different cards so rapid moves do not collapse into a final
			// placement: this exercises serial queue execution.
			canonical := []github.Item{
				{ID: "first", FieldValues: []github.FieldValue{{FieldID: "status", OptionID: "a", Available: true}}},
				{ID: "second", FieldValues: []github.FieldValue{{FieldID: "status", OptionID: "a", Available: true}}},
			}
			var mutationErrors []error
			if failFirst {
				mutationErrors = []error{&github.MutationError{Err: errors.New("permission denied"), Ambiguous: false}}
			}
			source := fakePickerSource{mutations: &mutations, mutationErrors: &mutationErrors, canonicalItems: &canonical}
			model := mutationModel(source, view, canonical)
			model.boardLane = 1
			model.boardCard = 0

			updated, first := model.Update(keyPress("L"))
			model = updated.(Model)
			// Select the second card (still in the source lane) before queueing.
			model.boardLane = 1
			model.boardCard = 0
			model.boardFocusID = "second"
			// Ensure the cursor resolves to the second card: lane a now holds
			// only second after first was optimistically moved to b.
			model.clampBoardCursor()
			updated, second := model.Update(keyPress("L"))
			model = updated.(Model)
			if second != nil || len(model.mutationSession.intents) != 2 {
				t.Fatalf("queued lane intent state = session:%#v lane:%d card:%d focus:%q cmd nil:%v", model.mutationSession, model.boardLane, model.boardCard, model.boardFocusID, second == nil)
			}
			if len(mutations) != 0 {
				t.Fatalf("write started before canonical readback: %v", mutations)
			}
			model = runMutationCommands(t, model, first)
			if model.mutationSession != nil || model.mutationLoading {
				t.Fatalf("queue did not settle: session=%#v loading=%v", model.mutationSession, model.mutationLoading)
			}
			wantCalls := []string{"field:b", "field:b"}
			wantFirst := "b"
			if failFirst {
				// Definitive failure leaves the first card in place, the
				// queue continues, and the failure is reported.
				wantFirst = "a"
			}
			if got := canonical[0].FieldValues[0].OptionID; got != wantFirst {
				t.Fatalf("first canonical lane = %q, want %s", got, wantFirst)
			}
			wantSecond := "b"
			if got := canonical[1].FieldValues[0].OptionID; got != wantSecond {
				t.Fatalf("second canonical lane = %q, want %s", got, wantSecond)
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

func TestRapidSameCardMovesCollapseToFinalPlacement(t *testing.T) {
	view := writableStatusView()
	mutations := []string{}
	item := github.Item{ID: "item", FieldValues: []github.FieldValue{{FieldID: "status", OptionID: "a", Available: true}}}
	canonical := []github.Item{item}
	source := fakePickerSource{mutations: &mutations, canonicalItems: &canonical}
	model := mutationModel(source, view, canonical)
	model.boardLane = 1

	updated, first := model.Update(keyPress("L"))
	model = updated.(Model)
	updated, second := model.Update(keyPress("L"))
	model = updated.(Model)
	if second != nil || len(model.mutationSession.intents) != 1 || model.boardLane != 3 {
		t.Fatalf("collapsed lane intent state = session:%#v lane:%d cmd nil:%v", model.mutationSession, model.boardLane, second == nil)
	}
	model = runMutationCommands(t, model, first)
	if model.mutationSession != nil || model.mutationLoading {
		t.Fatalf("collapsed queue did not settle: session=%#v loading=%v", model.mutationSession, model.mutationLoading)
	}
	if got := canonical[0].FieldValues[0].OptionID; got != "c" {
		t.Fatalf("final canonical lane = %q, want c", got)
	}
	if want := []string{"field:c"}; !reflect.DeepEqual(mutations, want) {
		t.Fatalf("collapsed mutation calls = %#v, want %#v", mutations, want)
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
	if second != nil || len(model.mutationSession.intents) != 1 || model.mutationSession.intents[0].direction != 2 {
		t.Fatalf("reorder intents should collapse to net +2, got %#v; second command nil=%v", model.mutationSession, second == nil)
	}
	model = runMutationCommands(t, model, first)
	if !reflect.DeepEqual(mutations, []string{"position:c"}) {
		t.Fatalf("collapsed reorder used wrong anchor: %#v", mutations)
	}
	got := make([]string, len(canonical))
	for index, item := range canonical {
		got[index] = item.ID
	}
	if want := []string{"x", "b", "y", "c", "a"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("canonical project order = %v, want %v", got, want)
	}
}

func TestRapidReorderWiggleCollapsesToNoWrites(t *testing.T) {
	view := writableStatusView()
	mutations := []string{}
	canonical := []github.Item{
		{ID: "a", FieldValues: []github.FieldValue{{FieldID: "status", OptionID: "a", Available: true}}},
		{ID: "b", FieldValues: []github.FieldValue{{FieldID: "status", OptionID: "a", Available: true}}},
		{ID: "c", FieldValues: []github.FieldValue{{FieldID: "status", OptionID: "a", Available: true}}},
	}
	model := mutationModel(fakePickerSource{mutations: &mutations, canonicalItems: &canonical}, view, canonical)
	model.boardLane = 1
	model.boardCard = 1 // middle card b

	var cmds []tea.Cmd
	for i := 0; i < 10; i++ {
		key := "J"
		if i%2 == 1 {
			key = "K"
		}
		updated, cmd := model.Update(keyPress(key))
		model = updated.(Model)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
	}
	if len(model.mutationSession.intents) != 0 {
		t.Fatalf("wiggle should collapse to zero intents, got %#v", model.mutationSession.intents)
	}
	for _, cmd := range cmds {
		model = runMutationCommands(t, model, cmd)
	}
	if len(mutations) != 0 {
		t.Fatalf("wiggle consumed API writes: %#v", mutations)
	}
	if model.mutationSession != nil {
		t.Fatalf("wiggle session should settle, got %#v", model.mutationSession)
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
	settleMessage := mutationCmd()
	updated, preflightCmd := model.Update(settleMessage)
	model = updated.(Model)
	if preflightCmd == nil {
		t.Fatal("settle did not start pre-read")
	}
	preflightMessage := preflightCmd()
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
	canonical := []github.Item{
		{ID: "first", FieldValues: []github.FieldValue{{FieldID: "status", OptionID: "a", Available: true}}},
		{ID: "second", FieldValues: []github.FieldValue{{FieldID: "status", OptionID: "a", Available: true}}},
	}
	mutations := []string{}
	mutationErrors := []error{context.DeadlineExceeded}
	source := fakePickerSource{mutations: &mutations, mutationErrors: &mutationErrors, canonicalItems: &canonical}
	model := mutationModel(source, view, canonical)
	model.boardLane = 1
	model.boardCard = 0
	updated, first := model.Update(keyPress("L"))
	model = updated.(Model)
	model.boardLane = 1
	model.boardCard = 0
	model.boardFocusID = "second"
	model.clampBoardCursor()
	updated, _ = model.Update(keyPress("L"))
	model = updated.(Model)
	if len(model.mutationSession.intents) != 2 {
		t.Fatalf("expected 2 queued intents for different cards, got %#v", model.mutationSession)
	}
	model = runMutationCommands(t, model, first)
	if model.mutationSession == nil || !model.mutationSession.blocked || !strings.Contains(model.status, "press r to") {
		t.Fatalf("ambiguous result did not block queue: session=%#v status=%q", model.mutationSession, model.status)
	}
	if !reflect.DeepEqual(mutations, []string{"field:b"}) {
		t.Fatalf("dependent write ran before reconciliation: %v", mutations)
	}

	// Simulate the timed-out write eventually applying on the server.
	canonical[0].FieldValues[0].OptionID = "b"
	updated, retry := model.Update(keyPress("r"))
	model = updated.(Model)
	model = runMutationCommands(t, model, retry)
	if model.mutationSession != nil {
		t.Fatalf("confirmed ambiguous write did not unblock: session=%#v", model.mutationSession)
	}
	if got := canonical[1].FieldValues[0].OptionID; got != "b" {
		t.Fatalf("queued intent did not resolve after confirmation: %#v", canonical)
	}
	if !reflect.DeepEqual(mutations, []string{"field:b", "field:b"}) {
		t.Fatalf("mutation calls after confirmation = %v", mutations)
	}
}

func TestRepeatedUnchangedReadbackKeepsAmbiguousWriteBlocked(t *testing.T) {
	view := writableStatusView()
	canonical := []github.Item{{ID: "item", FieldValues: []github.FieldValue{{FieldID: "status", OptionID: "a", Available: true}}}}
	mutations := []string{}
	mutationErrors := []error{context.DeadlineExceeded}
	source := fakePickerSource{mutations: &mutations, mutationErrors: &mutationErrors, canonicalItems: &canonical}
	model := mutationModel(source, view, canonical)
	model.boardLane = 1
	updated, first := model.Update(keyPress("L"))
	model = updated.(Model)
	model = runMutationCommands(t, model, first)
	if model.mutationSession == nil || !model.mutationSession.blocked || len(mutations) != 1 {
		t.Fatalf("first unchanged readback did not pause the queue: session=%#v calls=%v", model.mutationSession, mutations)
	}

	// Repeated unchanged readbacks alone do not prove a timed-out write
	// cannot still finish, so dependent work stays blocked.
	updated, retry := model.Update(keyPress("r"))
	model = updated.(Model)
	model = runMutationCommands(t, model, retry)
	if model.mutationSession == nil || !model.mutationSession.blocked || len(mutations) != 1 {
		t.Fatalf("repeated unchanged readback should stay blocked: session=%#v calls=%v", model.mutationSession, mutations)
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
	settleMessage := preflight()
	updated, preRead := model.Update(settleMessage)
	model = updated.(Model)
	if preRead == nil {
		t.Fatal("settle did not start pre-read")
	}
	readbackMessage := preRead()
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

func TestCombinedMoveIntoEmptyCellKeepsProjectPosition(t *testing.T) {
	view := writableCombinedView()
	canonical := []github.Item{combinedItem("item", "p0", "todo"), combinedItem("other-row", "p0", "done")}
	mutations := []string{}
	model := mutationModel(fakePickerSource{mutations: &mutations, canonicalItems: &canonical}, view, canonical)
	model.boardLane = 4 // Todo/P0.
	updated, cmd := model.Update(keyPress("L"))
	model = updated.(Model)
	model = runMutationCommands(t, model, cmd)
	priority, _ := itemFieldValue(view.GroupByFields[0], canonical[0])
	status, _ := itemFieldValue(view.VerticalGroupBy[0], canonical[0])
	if priority.OptionID != "p1" || status.OptionID != "todo" {
		t.Fatalf("empty-cell move values = priority:%#v status:%#v", priority, status)
	}
	if !reflect.DeepEqual(mutations, []string{"field:p1"}) || canonical[0].ID != "item" {
		t.Fatalf("empty-cell move changed project position: calls=%v items=%#v", mutations, canonical)
	}
}

func TestCombinedMoveToNoValueClearsOnlyColumnField(t *testing.T) {
	view := writableCombinedView()
	canonical := []github.Item{combinedItem("item", "p0", "todo"), combinedItem("anchor", "", "todo")}
	mutations := []string{}
	model := mutationModel(fakePickerSource{mutations: &mutations, canonicalItems: &canonical}, view, canonical)
	model.boardLane = 4 // Todo/P0.
	updated, cmd := model.Update(keyPress("H"))
	model = updated.(Model)
	model = runMutationCommands(t, model, cmd)
	var moved github.Item
	for _, item := range canonical {
		if item.ID == "item" {
			moved = item
			break
		}
	}
	if _, found := itemFieldValue(view.GroupByFields[0], moved); found {
		t.Fatalf("column grouping value was not cleared: %#v", moved.FieldValues)
	}
	status, found := itemFieldValue(view.VerticalGroupBy[0], moved)
	if !found || status.OptionID != "todo" {
		t.Fatalf("swimlane field changed during column clear: %#v", moved.FieldValues)
	}
	if !reflect.DeepEqual(mutations, []string{"clear", "position:anchor"}) {
		t.Fatalf("No value move mutations = %v", mutations)
	}
}

func TestCombinedSortedMoveUpdatesColumnWithoutPositionMutation(t *testing.T) {
	view := writableCombinedView()
	points := github.Field{ID: "points", Name: "Points", DataType: "NUMBER"}
	view.SortByFields = []github.SortField{{Direction: "DESC", Field: points}}
	canonical := []github.Item{combinedItem("item", "p0", "todo"), combinedItem("anchor", "p1", "todo")}
	mutations := []string{}
	model := mutationModel(fakePickerSource{mutations: &mutations, canonicalItems: &canonical}, view, canonical)
	model.boardLane = 4
	updated, cmd := model.Update(keyPress("L"))
	model = updated.(Model)
	model = runMutationCommands(t, model, cmd)
	if !reflect.DeepEqual(mutations, []string{"field:p1"}) {
		t.Fatalf("sorted combined move changed project position: %v", mutations)
	}
	status, _ := itemFieldValue(view.VerticalGroupBy[0], canonical[0])
	if status.OptionID != "todo" {
		t.Fatalf("sorted move changed swimlane: %#v", status)
	}
}

func TestCombinedMoveRefreshShowsCanonicalCell(t *testing.T) {
	view := writableCombinedView()
	canonical := []github.Item{combinedItem("item", "p0", "todo"), combinedItem("anchor", "p1", "todo")}
	mutations := []string{}
	source := fakePickerSource{mutations: &mutations, canonicalItems: &canonical, view: view}
	model := mutationModel(source, view, canonical)
	model.boardLane = 4
	updated, cmd := model.Update(keyPress("L"))
	model = updated.(Model)
	model = runMutationCommands(t, model, cmd)

	updated, refresh := model.Update(keyPress("r"))
	model = updated.(Model)
	if refresh == nil {
		t.Fatal("board refresh did not start items reload")
	}
	batch, ok := refresh().(tea.BatchMsg)
	if !ok || len(batch) < 2 {
		t.Fatalf("refresh item commands = %#v", batch)
	}
	updated, next := model.Update(batch[len(batch)-1]())
	model = updated.(Model)
	model = runMutationCommands(t, model, next)
	var refreshed github.Item
	for _, item := range model.items {
		if item.ID == "item" {
			refreshed = item
			break
		}
	}
	value, _ := itemFieldValue(view.GroupByFields[0], refreshed)
	row, _ := itemFieldValue(view.VerticalGroupBy[0], refreshed)
	if refreshed.ID != "item" || value.OptionID != "p1" || row.OptionID != "todo" {
		t.Fatalf("refreshed combined card = %#v", refreshed)
	}
}

func TestCombinedMoveRespectsReadOnlyPermission(t *testing.T) {
	view := writableCombinedView()
	view.ViewerCanUpdate = false
	model := mutationModel(fakePickerSource{}, view, []github.Item{combinedItem("item", "p0", "todo")})
	model.boardLane = 4
	updated, cmd := model.Update(keyPress("L"))
	model = updated.(Model)
	if cmd != nil || model.mutationSession != nil || !strings.Contains(model.status, "read-only") {
		t.Fatalf("read-only combined board allowed mutation: status=%q session=%#v", model.status, model.mutationSession)
	}
}

func TestCombinedMoveDoesNotWrapIntoAnotherSwimlane(t *testing.T) {
	view := writableCombinedView()
	canonical := []github.Item{combinedItem("item", "p1", "todo")}
	mutations := []string{}
	model := mutationModel(fakePickerSource{mutations: &mutations, canonicalItems: &canonical}, view, canonical)
	model.boardLane = 5 // Last column in the Todo swimlane.
	updated, cmd := model.Update(keyPress("L"))
	model = updated.(Model)
	if cmd != nil || model.mutationSession != nil || !strings.Contains(model.status, "No destination lane") || len(mutations) != 0 {
		t.Fatalf("combined lane move wrapped into another swimlane: status=%q session=%#v mutations=%v", model.status, model.mutationSession, mutations)
	}
}

func TestCombinedMoveRejectsUnsupportedSwimlaneField(t *testing.T) {
	view := writableCombinedView()
	view.VerticalGroupBy = []github.Field{{ID: "team", Name: "Team", Kind: "ProjectV2Field", DataType: "TEXT"}}
	model := mutationModel(fakePickerSource{}, view, []github.Item{combinedItem("item", "p0", "")})
	model.boardLane = 4
	updated, cmd := model.Update(keyPress("L"))
	model = updated.(Model)
	if cmd != nil || model.mutationSession != nil || !strings.Contains(model.status, "both axes") {
		t.Fatalf("unsupported combined axes allowed mutation: status=%q session=%#v", model.status, model.mutationSession)
	}
}

func TestAmbiguousSecondStepStaysBlockedUntilResolved(t *testing.T) {
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
	// Stable partial readbacks do not prove the timed-out position write
	// finished, so the session stays blocked instead of reporting partial.
	if model.mutationSession == nil || !model.mutationSession.blocked {
		t.Fatalf("ambiguous partial should stay blocked: session=%#v status=%q", model.mutationSession, model.status)
	}
	if !reflect.DeepEqual(mutations, []string{"field:b", "position:b"}) {
		t.Fatalf("ambiguous position mutation was resubmitted: %v", mutations)
	}
}
