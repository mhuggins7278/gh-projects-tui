package ui

import (
	"context"
	"errors"
	"reflect"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/mhuggins7278/gh-projects-tui/internal/github"
)

type issueMembershipSource struct {
	fakePickerSource
	closed, ambiguous bool
	writes            int
	filters, cursors  []string
}

func (s *issueMembershipSource) SetIssueClosed(_ context.Context, _ string, closed bool) error {
	s.writes++
	s.closed = closed
	if s.ambiguous {
		return &github.MutationError{Err: errors.New("timeout"), Ambiguous: true}
	}
	return nil
}
func (s *issueMembershipSource) IssueComment(context.Context, string, string) error { return nil }
func (s *issueMembershipSource) ReadIssueState(context.Context, string) (github.IssueState, error) {
	state := "OPEN"
	if s.closed {
		state = "CLOSED"
	}
	return github.IssueState{ID: "issue", State: state}, nil
}
func (s *issueMembershipSource) LoadItemDetail(context.Context, github.Owner, int, string) (github.ItemDetail, error) {
	state, _ := s.ReadIssueState(context.Background(), "issue")
	return github.ItemDetail{ID: "one", Content: &github.Content{ID: state.ID, Kind: "Issue", State: state.State, Body: "Keep reading"}}, nil
}
func (s *issueMembershipSource) PageItems(_ context.Context, _ github.Owner, _ int, filter, after string) (github.ItemsPage, error) {
	s.filters = append(s.filters, filter)
	s.cursors = append(s.cursors, after)
	// Both tested state transitions remove the acted-on issue. The replacement
	// is on the next page so the test also exercises the actual read lifecycle.
	if after == "" {
		return github.ItemsPage{HasNext: true, EndCursor: "last"}, nil
	}
	return github.ItemsPage{Items: []github.Item{{ID: "survivor", Content: &github.Content{Kind: "Issue", Title: "Remaining issue"}}}}, nil
}

func runMembershipReads(t *testing.T, model Model, cmd tea.Cmd) Model {
	t.Helper()
	queue := []tea.Cmd{cmd}
	for steps := 0; len(queue) > 0; steps++ {
		if steps > 30 {
			t.Fatal("read pipeline did not settle")
		}
		current := queue[0]
		queue = queue[1:]
		if current == nil {
			continue
		}
		msg := current()
		if batch, ok := msg.(tea.BatchMsg); ok {
			queue = append(queue, batch...)
			continue
		}
		if _, ok := msg.(itemsLoadingTickMsg); ok {
			continue
		}
		updated, next := model.Update(msg)
		model = updated.(Model)
		if next != nil {
			queue = append(queue, next)
		}
	}
	return model
}

func TestIssueStateChangeRefreshesSavedMembershipAfterDetailCloses(t *testing.T) {
	for _, filter := range []string{"is:open", "is:closed"} {
		for _, ambiguous := range []bool{false, true} {
			t.Run(filter+map[bool]string{false: "/success", true: "/readback"}[ambiguous], func(t *testing.T) {
				source := &issueMembershipSource{closed: filter == "is:closed", ambiguous: ambiguous}
				view := writableStatusView()
				view.Filter = filter
				state := "OPEN"
				if source.closed {
					state = "CLOSED"
				}
				m := mutationModel(source.fakePickerSource, view, []github.Item{{ID: "one", Content: &github.Content{ID: "issue", Kind: "Issue", State: state}}})
				m.source = source
				m.detailVisible, m.detailItemID = true, "one"
				m.detail = &github.ItemDetail{ID: "one", Content: &github.Content{ID: "issue", Kind: "Issue", State: state, Body: "Keep reading"}}
				updated, _ := m.Update(keyPress("x"))
				m = updated.(Model)
				updated, write := m.Update(keyPress("enter"))
				m = updated.(Model)
				m = runMembershipReads(t, m, write)
				if !m.detailVisible || m.detail.Content.Body != "Keep reading" || source.writes != 1 || len(source.filters) != 0 {
					t.Fatal("state change discarded detail or prematurely refreshed membership")
				}
				updated, reload := m.Update(keyPress("esc"))
				m = updated.(Model)
				if reload == nil {
					t.Fatal("closing details did not refresh saved membership")
				}
				m = runMembershipReads(t, m, reload)
				if m.itemsLoading || len(m.items) != 1 || m.items[0].ID != "survivor" || source.writes != 1 || !reflect.DeepEqual(source.filters, []string{filter, filter}) || !reflect.DeepEqual(source.cursors, []string{"", "last"}) {
					t.Fatalf("stale membership or wrong read/write pipeline: items=%v filters=%v cursors=%v writes=%d", m.items, source.filters, source.cursors, source.writes)
				}
			})
		}
	}
}

func TestIssueMembershipReloadWaitsForPaginationAndDropsChangedScopes(t *testing.T) {
	for _, scenario := range []string{"paging", "navigate", "new view", "new host"} {
		t.Run(scenario, func(t *testing.T) {
			source := &issueMembershipSource{}
			view := writableStatusView()
			view.Filter = "is:open"
			m := mutationModel(source.fakePickerSource, view, nil)
			m.source = source
			m.detailVisible = true
			m.detailItemID = "one"
			m.detail = &github.ItemDetail{Content: &github.Content{Kind: "Issue", State: "CLOSED"}}
			m.itemsLoading = true
			// A confirmed state read arrives while original items are still paging.
			m.issueActionPending = &issueActionState{id: 1, readID: 1, itemID: "one", issueID: "issue", closed: true, scope: m.currentBoardReadScope()}
			m.mutationLoading = true
			updated, cmd := m.Update(issueReadbackMsg{actionID: 1, readID: 1, state: github.IssueState{ID: "issue", State: "CLOSED"}})
			m = updated.(Model)
			if cmd != nil || !m.detailVisible {
				t.Fatal("readback interrupted details/paging")
			}
			switch scenario {
			case "navigate":
				m.abandonReads()
				m.selectedProject = &github.Project{Number: 2}
			case "new view":
				m.view.Number++
			case "new host":
				m.host = "other.example"
			}
			updated, cmd = m.Update(keyPress("esc"))
			m = updated.(Model)
			if cmd != nil {
				t.Fatal("dismiss interrupted an active page or refreshed a changed scope")
			}
			if scenario != "paging" {
				if m.pendingIssueReload != nil || m.pendingMutationReload != nil || len(source.filters) != 0 {
					t.Fatal("reload leaked across navigation")
				}
				return
			}
			if m.pendingMutationReload == nil {
				t.Fatal("refresh was lost before original pagination ended")
			}
			updated, cmd = m.Update(itemsPageMsg{generation: m.generation, scope: m.currentBoardReadScope(), page: github.ItemsPage{}})
			m = runMembershipReads(t, updated.(Model), cmd)
			if len(m.items) != 1 || m.items[0].ID != "survivor" || source.writes != 0 || !reflect.DeepEqual(source.filters, []string{"is:open", "is:open"}) {
				t.Fatal("final page failed to start an exact-filter reload")
			}
		})
	}
}

func TestUnresolvedOrFailedIssueWriteDoesNotRefreshMembership(t *testing.T) {
	for _, ambiguous := range []bool{false, true} {
		view := writableStatusView()
		view.Filter = "is:open"
		m := mutationModel(fakePickerSource{}, view, nil)
		m.detailVisible, m.detailItemID = true, "one"
		m.detail = &github.ItemDetail{Content: &github.Content{ID: "issue", Kind: "Issue", State: "OPEN"}}
		m.issueActionPending = &issueActionState{id: 1, itemID: "one", issueID: "issue", scope: m.currentBoardReadScope(), closed: true}
		m.mutationLoading = true
		var err error = errors.New("permission denied")
		if ambiguous {
			err = &github.MutationError{Err: errors.New("timeout"), Ambiguous: true}
		}
		updated, _ := m.Update(issueActionMsg{actionID: 1, itemID: "one", err: err})
		m = updated.(Model)
		if m.pendingIssueReload != nil || m.pendingMutationReload != nil {
			t.Fatal("failed/unconfirmed write scheduled membership reload")
		}
		if ambiguous && (m.issueActionPending == nil || !m.mutationLoading) {
			t.Fatal("uncertain write unlocked another write")
		}
	}
}

func TestConfirmedIssueReadbackAfterDismissingDetailReloadsMembership(t *testing.T) {
	source := &issueMembershipSource{closed: true}
	view := writableStatusView()
	view.Filter = "is:open"
	m := mutationModel(source.fakePickerSource, view, []github.Item{{ID: "one"}})
	m.source = source
	m.detailVisible, m.detailItemID = true, "one"
	m.mutationLoading = true
	m.issueActionPending = &issueActionState{id: 1, readID: 1, itemID: "one", issueID: "issue", closed: true, blocked: true, scope: m.currentBoardReadScope()}
	updated, cmd := m.Update(keyPress("esc"))
	m = updated.(Model)
	if cmd != nil || m.detailVisible || m.issueActionPending == nil {
		t.Fatal("dismissing blocked detail lost the unresolved issue action")
	}
	updated, cmd = m.Update(issueReadbackMsg{actionID: 1, readID: 1, state: github.IssueState{ID: "issue", State: "CLOSED"}})
	if cmd == nil {
		t.Fatal("confirmed outcome failed to refresh the visible board")
	}
	m = runMembershipReads(t, updated.(Model), cmd)
	if m.issueActionPending != nil || m.mutationLoading || len(m.items) != 1 || m.items[0].ID != "survivor" || source.writes != 0 || !reflect.DeepEqual(source.filters, []string{"is:open", "is:open"}) {
		t.Fatal("confirmed readback retained stale membership or replayed the write")
	}
}
