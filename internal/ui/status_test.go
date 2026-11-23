package ui

import (
	"errors"
	"strings"
	"testing"

	"github.com/mhuggins7278/gh-projects-tui/internal/github"
)

func TestFieldEditsKeepStatusAreaQuiet(t *testing.T) {
	for _, layout := range []github.ViewLayout{github.BoardLayout, github.TableLayout} {
		t.Run(string(layout), func(t *testing.T) {
			field := github.Field{
				ID: "sprint", Name: "OTC Sprint", DataType: "ITERATION",
				Iterations: []github.Iteration{{ID: "sprint-10", Title: "Sprint 10"}},
			}
			m, source := editorModel(field)
			m.view.Layout = layout
			m.debug = true // Debug telemetry must not bring routine notices back.
			m.status = "Previous failure"
			updated, load := m.Update(keyPress("e"))
			m = updated.(Model)
			if load == nil || m.status != "" {
				t.Fatalf("loading fields left a status notice: %q", m.status)
			}
			updated, _ = m.Update(load())
			m = updated.(Model)
			if m.tableEditor == nil || m.status != "" {
				t.Fatalf("choosing a field left a status notice: %q", m.status)
			}
			for _, key := range []string{"enter", "down"} {
				m, _ = editorKey(m, key)
				if m.status != "" {
					t.Fatalf("editing a field left a status notice: %q", m.status)
				}
			}
			m, cmd := editorKey(m, "enter")
			for step := 0; cmd != nil; step++ {
				if step >= 50 {
					t.Fatal("field save did not settle")
				}
				if m.status != "" {
					t.Fatalf("saving a field left a status notice: %q", m.status)
				}
				updated, next := m.Update(cmd())
				m, cmd = updated.(Model), next
			}
			if len(source.writes) != 1 || source.writes[0].Value.IterationID == nil || *source.writes[0].Value.IterationID != "sprint-10" || m.mutationSession != nil || m.status != "" {
				t.Fatalf("field save did not finish quietly: writes=%v status=%q", source.writes, m.status)
			}
			if strings.Contains(m.View().Content, "Edit OTC Sprint saved") {
				t.Fatal("rendered the routine success notice")
			}
			m, _ = editorKey(m, "e")
			m, _ = editorKey(m, "esc")
			if m.tableEditor != nil || m.status != "" || len(source.writes) != 1 {
				t.Fatal("cancelling an edit left a status notice or wrote a field")
			}
		})
	}
}

func TestFieldEditFailuresStillRender(t *testing.T) {
	for _, layout := range []github.ViewLayout{github.BoardLayout, github.TableLayout} {
		for _, ambiguous := range []bool{false, true} {
			name := "rejected"
			want := "permission denied"
			if ambiguous {
				name, want = "uncertain", "press r to check again"
			}
			t.Run(string(layout)+"/"+name, func(t *testing.T) {
				m, source := editorModel(github.Field{ID: "notes", Name: "Notes", DataType: "TEXT"})
				m.view.Layout = layout
				source.err = &github.MutationError{Err: errors.New("permission denied"), Ambiguous: ambiguous}
				for _, key := range []string{"e", "enter", "ctrl+u", "n"} {
					m, _ = editorKey(m, key)
				}
				m, cmd := editorKey(m, "enter")
				m = runMembershipReads(t, m, cmd)
				if !strings.Contains(m.status, want) || !strings.Contains(m.View().Content, want) {
					t.Fatalf("lost actionable failure: %q", m.status)
				}
			})
		}
	}
}

func TestArchiveKeepsConfirmationButNotRoutineStatus(t *testing.T) {
	source := &fakeTableActionSource{}
	m := tableActionModel(source)
	m, _ = editorKey(m, "A")
	if !strings.Contains(m.View().Content, "Press enter to archive") {
		t.Fatal("archive confirmation is not visible")
	}
	m, cmd := editorKey(m, "enter")
	if m.status != "" || !strings.Contains(m.footerHints(), "saving") {
		t.Fatal("save should use the compact footer, not a status notice")
	}
	m = runMembershipReads(t, m, cmd)
	if len(source.archive) != 1 || m.tableAction != nil || m.status != "" {
		t.Fatalf("archive did not finish quietly: %q", m.status)
	}
}
