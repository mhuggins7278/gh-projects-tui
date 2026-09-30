package ui

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/mhuggins7278/gh-projects-tui/internal/github"
)

type membershipReadSource struct {
	fakeTableActionSource
	pages     map[string]github.ItemsPage
	failAfter string
	fullReads int
	cursors   []string
	owner     github.Owner
	project   int
}

func (s *membershipReadSource) PageItems(context.Context, github.Owner, int, string, string) (github.ItemsPage, error) {
	s.fullReads++
	return github.ItemsPage{}, errors.New("full reader should not be used")
}
func (s *membershipReadSource) PageMutationItems(ctx context.Context, owner github.Owner, project int, after string, fields []github.Field) (github.ItemsPage, error) {
	if err := ctx.Err(); err != nil {
		return github.ItemsPage{}, err
	}
	if _, ok := ctx.Deadline(); !ok {
		return github.ItemsPage{}, errors.New("readback has no deadline")
	}
	s.cursors = append(s.cursors, after)
	if owner != s.owner || project != s.project || len(fields) != 0 {
		return github.ItemsPage{}, errors.New("wrong scope or unused field selection")
	}
	if after == s.failAfter {
		return github.ItemsPage{}, errors.New("page inaccessible")
	}
	return s.pages[after], nil
}

func TestTableReadbackUsesIDsAndRequiresCompleteOriginScan(t *testing.T) {
	for _, lastPage := range []string{"present", "absent", "error"} {
		t.Run(lastPage, func(t *testing.T) {
			source := &membershipReadSource{failAfter: "never", pages: map[string]github.ItemsPage{
				"":     {Items: []github.Item{{ID: "other"}}, HasNext: true, EndCursor: "last"},
				"last": {},
			}}
			m := tableActionModel(source)
			source.owner, source.project = *m.selectedOwner, m.selectedProject.Number
			m.tableAction = &tableActionState{scope: m.currentBoardReadScope(), itemID: "one", phase: "blocked"}
			if lastPage == "present" {
				source.pages["last"] = github.ItemsPage{Items: []github.Item{{ID: "one"}}}
			}
			if lastPage == "error" {
				source.failAfter = "last"
			}
			read := m.startTableActionReadback()
			m.abandonReads()
			m.screen = screenProjectPicker
			m.selectedProject = &github.Project{Number: 2}
			updated, cmd := m.Update(read())
			m = updated.(Model)
			if cmd != nil || source.fullReads != 0 || !reflect.DeepEqual(source.cursors, []string{"", "last"}) {
				t.Fatalf("reads=%v full=%d", source.cursors, source.fullReads)
			}
			if lastPage == "absent" {
				if m.tableAction != nil {
					t.Fatal("full absence did not reconcile")
				}
			} else if m.tableAction == nil || m.tableAction.phase != "blocked" {
				t.Fatal("partial or present result unlocked writes")
			}
		})
	}
}

type stateReadSource struct {
	reviewIssueSource
	state     github.IssueState
	readErr   error
	readIDs   []string
	fullReads int
}

func (s *stateReadSource) ReadIssueState(ctx context.Context, id string) (github.IssueState, error) {
	if _, ok := ctx.Deadline(); !ok {
		return github.IssueState{}, errors.New("readback has no deadline")
	}
	s.readIDs = append(s.readIDs, id)
	return s.state, s.readErr
}
func (s *stateReadSource) LoadItemDetail(context.Context, github.Owner, int, string) (github.ItemDetail, error) {
	s.fullReads++
	return github.ItemDetail{}, errors.New("full detail reader should not be used")
}

func TestIssueReadbackPreservesDetailAndIgnoresStaleState(t *testing.T) {
	source := &stateReadSource{state: github.IssueState{ID: "issue", State: "CLOSED"}}
	m := mutationModel(source.fakePickerSource, writableStatusView(), []github.Item{{ID: "one", Content: &github.Content{ID: "issue", State: "OPEN"}}})
	m.source = source
	m.detailVisible = true
	m.detailItemID = "one"
	m.detail = &github.ItemDetail{ID: "one", Content: &github.Content{ID: "issue", Kind: "Issue", State: "OPEN", Body: "Keep this body"}, CommentsLoaded: true, Comments: []github.IssueComment{{ID: "comment", Body: "Keep this comment"}}}
	m.detailCache = map[string]github.ItemDetail{"one": *m.detail}
	m.issueActionPending = &issueActionState{id: 7, itemID: "one", issueID: "issue", scope: m.currentBoardReadScope(), closed: true, blocked: true}
	m.mutationLoading = true
	read := m.startIssueReadback()
	result := read().(issueReadbackMsg)
	for _, stale := range []issueReadbackMsg{{actionID: 6, readID: result.readID, state: result.state}, {actionID: 7, readID: result.readID - 1, state: result.state}} {
		updated, _ := m.Update(stale)
		m = updated.(Model)
		if m.issueActionPending == nil || !m.mutationLoading || !m.issueActionRefreshing {
			t.Fatal("stale state unlocked write")
		}
	}
	updated, cmd := m.Update(result)
	m = updated.(Model)
	if cmd != nil || m.issueActionPending != nil || m.mutationLoading || m.issueActionRefreshing {
		t.Fatal("confirmed state did not reconcile")
	}
	if source.fullReads != 0 || !reflect.DeepEqual(source.readIDs, []string{"issue"}) {
		t.Fatal("readback did not use issue-only read")
	}
	if m.detail.Content.State != "CLOSED" || m.items[0].Content.State != "CLOSED" || m.detail.Content.Body != "Keep this body" || m.detail.Comments[0].Body != "Keep this comment" {
		t.Fatal("state-only read discarded or failed to update detail")
	}
	if _, ok := m.detailCache["one"]; ok {
		t.Fatal("stale detail cache survived reconciliation")
	}
}

func TestIssueReadbackUsesOriginIssueAfterNavigation(t *testing.T) {
	source := &stateReadSource{state: github.IssueState{ID: "issue", State: "CLOSED"}}
	m := mutationModel(source.fakePickerSource, writableStatusView(), nil)
	m.source = source
	m.issueActionPending = &issueActionState{id: 1, itemID: "one", issueID: "issue", scope: m.currentBoardReadScope(), closed: true, blocked: true}
	m.mutationLoading = true
	read := m.startIssueReadback()
	m.abandonReads()
	m.selectedProject = &github.Project{Number: 2}
	m.items = []github.Item{{ID: "one", Content: &github.Content{ID: "different", State: "OPEN"}}}
	m.detailItemID = "one"
	m.detail = &github.ItemDetail{Content: &github.Content{ID: "different", State: "OPEN"}}
	updated, _ := m.Update(read())
	m = updated.(Model)
	if m.issueActionPending != nil || m.mutationLoading || !reflect.DeepEqual(source.readIDs, []string{"issue"}) {
		t.Fatal("origin state failed to reconcile")
	}
	if m.items[0].Content.State != "OPEN" || m.detail.Content.State != "OPEN" {
		t.Fatal("origin state changed another project")
	}
}
