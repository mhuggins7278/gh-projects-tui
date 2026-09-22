package ui

import (
	"errors"
	"testing"

	"github.com/mhuggins7278/gh-projects-tui/internal/github"
)

func TestQueuedMovesSerializeAndRollbackOnlyUnconfirmedMoves(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(map[bool]string{false: "success", true: "failure"}[fail], func(t *testing.T) {
			calls := []string{}
			m := newPickerModel(fakePickerSource{mutations: &calls})
			m.screen = screenBoard
			m.view = &github.View{ProjectID: "project", ViewerCanUpdate: true, GroupByFields: []github.Field{{
				ID: "status", Name: "Status", DataType: "SINGLE_SELECT",
				Options: []github.FieldOption{{ID: "a", Name: "A"}, {ID: "b", Name: "B"}, {ID: "c", Name: "C"}},
			}}}
			m.items = []github.Item{{ID: "item"}}
			updated, first := m.Update(keyPress("L"))
			m = updated.(Model)
			for range 2 {
				updated, cmd := m.Update(keyPress("L"))
				m = updated.(Model)
				if cmd != nil {
					t.Fatal("queued request started early")
				}
			}
			if len(calls) != 0 || len(m.mutationQueue) != 2 || m.boardLane != 3 {
				t.Fatalf("moves not optimistic/queued: calls=%v queue=%d lane=%d", calls, len(m.mutationQueue), m.boardLane)
			}
			updated, _ = m.Update(keyPress("h"))
			m = updated.(Model)
			if m.boardLane != 2 {
				t.Fatal("navigation blocked while saving")
			}
			updated, second := m.Update(first())
			m = updated.(Model)
			if second == nil || len(calls) != 1 {
				t.Fatal("next move was not dispatched serially")
			}
			message := second().(boardMutationMsg)
			if fail {
				message.err = errors.New("save failed")
			}
			updated, third := m.Update(message)
			m = updated.(Model)
			if fail {
				value, _ := itemFieldValue(m.view.GroupByFields[0], m.items[0])
				if third != nil || m.mutationLoading || len(m.mutationQueue) != 0 || value.OptionID != "a" {
					t.Fatalf("failed queue did not preserve first save: value=%v queue=%d", value, len(m.mutationQueue))
				}
			} else {
				if third == nil {
					t.Fatal("third move missing")
				}
				updated, next := m.Update(third())
				m = updated.(Model)
				if next != nil || m.mutationLoading || len(calls) != 3 {
					t.Fatal("queue did not drain without refresh")
				}
			}
		})
	}
}
