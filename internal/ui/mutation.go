package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/mhuggins7278/gh-projects-tui/internal/github"
)

const mutationRequestTimeout = 45 * time.Second

type boardMutationKind uint8

const (
	boardMutationMoveLane boardMutationKind = iota
	boardMutationReorder
)

type boardMutationIntent struct {
	kind        boardMutationKind
	itemID      string
	direction   int
	description string
}

type boardMutationPlan struct {
	intent        boardMutationIntent
	projectID     string
	beforeItems   []github.Item
	field         github.Field
	fieldValue    github.FieldValueInput
	hasField      bool
	clearField    bool
	hasPosition   bool
	positionAfter *string
	noOp          bool
}

type boardMutationSession struct {
	id             uint64
	owner          github.Owner
	projectNumber  int
	projectID      string
	view           github.View
	canonicalItems []github.Item
	intents        []boardMutationIntent
	plan           *boardMutationPlan
	attempt        *mutationAttemptResult
	blocked        bool
	readID         uint64
	writeID        uint64
	readbackState  string
	readbackCount  int
	notice         string
	lastSuccess    string
}

type mutationAttemptResult struct {
	err       error
	ambiguous bool
}

func (m *Model) enqueueBoardMutation(intent boardMutationIntent) tea.Cmd {
	if _, ok := m.source.(ItemMutationSource); !ok {
		m.status = "Mutations are unavailable for this client"
		return nil
	}
	if _, ok := m.source.(ItemsSource); !ok {
		m.status = "Mutations require project readback, which is unavailable for this client"
		return nil
	}
	if m.selectedOwner == nil || m.selectedProject == nil || m.view == nil {
		m.status = "Mutations require a selected project and view"
		return nil
	}

	if m.mutationSession == nil {
		m.nextMutationSession++
		view := *m.view
		m.mutationSession = &boardMutationSession{
			id:             m.nextMutationSession,
			owner:          *m.selectedOwner,
			projectNumber:  m.selectedProject.Number,
			projectID:      m.view.ProjectID,
			view:           view,
			canonicalItems: cloneItems(m.items),
		}
	} else {
		session := m.mutationSession
		if !sameMutationProject(session, *m.selectedOwner, m.selectedProject.Number) ||
			session.projectID != m.view.ProjectID || session.view.Number != m.view.Number {
			m.status = "A save from another project or view is still being reconciled; finish it before mutating here"
			return nil
		}
		if session.blocked {
			m.status = "A previous save has an unknown outcome; press r to reconcile before making another change"
			return nil
		}
	}

	session := m.mutationSession
	session.intents = append(session.intents, intent)
	m.boardFocusID = intent.itemID
	m.projectPendingMutations()
	if len(session.intents) > 1 {
		m.status = fmt.Sprintf("Saving moves (%d queued)...", len(session.intents)-1)
	} else {
		m.status = intent.description + ": checking current project order..."
	}
	if len(session.intents) == 1 && session.plan == nil && session.attempt == nil && session.readID == 0 {
		return m.startMutationReadback()
	}
	return nil
}

func (m *Model) retryMutationReadback() tea.Cmd {
	if m.mutationSession == nil || !m.mutationSession.blocked {
		return nil
	}
	m.setMutationStatus(m.mutationSession, "Reconciling the saved project state...")
	return m.startMutationReadback()
}

func (m *Model) startMutationReadback() tea.Cmd {
	session := m.mutationSession
	if session == nil {
		return nil
	}
	session.blocked = false
	session.readID++
	readID := session.readID
	sessionID := session.id
	owner := session.owner
	projectNumber := session.projectNumber
	source := m.source
	ctx, cancel := context.WithTimeout(context.Background(), mutationRequestTimeout)
	m.mutationLoading = true
	return func() tea.Msg {
		defer cancel()
		loader, ok := source.(ItemsSource)
		if !ok {
			return mutationReadbackMsg{sessionID: sessionID, readID: readID, err: fmt.Errorf("project item readback is unavailable")}
		}
		items, err := readCanonicalProjectItems(ctx, loader, owner, projectNumber)
		return mutationReadbackMsg{sessionID: sessionID, readID: readID, items: items, err: err}
	}
}

func readCanonicalProjectItems(ctx context.Context, source ItemsSource, owner github.Owner, projectNumber int) ([]github.Item, error) {
	items := make([]github.Item, 0)
	after := ""
	for {
		page, err := source.PageItems(ctx, owner, projectNumber, "", after)
		if err != nil {
			return nil, err
		}
		items = append(items, page.Items...)
		if !page.HasNext {
			return items, nil
		}
		if page.EndCursor == "" || page.EndCursor == after {
			return nil, fmt.Errorf("project item readback returned a repeated pagination cursor")
		}
		after = page.EndCursor
	}
}

func (m *Model) updateBoardMutation(msg boardMutationMsg) (tea.Model, tea.Cmd) {
	session := m.mutationSession
	if session == nil || session.id != msg.sessionID || session.writeID != msg.writeID || session.plan == nil || session.attempt != nil {
		return *m, nil
	}
	result := msg.result
	session.attempt = &result
	return *m, m.startMutationReadback()
}

func (m *Model) updateMutationReadback(msg mutationReadbackMsg) (tea.Model, tea.Cmd) {
	session := m.mutationSession
	if session == nil || session.id != msg.sessionID || session.readID != msg.readID {
		return *m, nil
	}
	m.mutationLoading = false
	if msg.err != nil {
		session.blocked = true
		if session.attempt != nil {
			m.setMutationStatus(session, fmt.Sprintf("Outcome of %s is unknown: readback failed: %v; press r to reconcile, queued writes are blocked", session.plan.intent.description, msg.err))
		} else {
			m.setMutationStatus(session, fmt.Sprintf("Could not read current project order: %v; press r to retry before saving", msg.err))
		}
		return *m, nil
	}

	session.canonicalItems = cloneItems(msg.items)
	if session.attempt == nil {
		return *m, m.dispatchNextMutation()
	}

	verification := verifyMutationPlan(*session.plan, session.canonicalItems)
	attempt := *session.attempt
	intent := session.plan.intent
	if verification.complete {
		session.lastSuccess = intent.description + " saved"
		delete(m.detailCache, intent.itemID)
		session.intents = session.intents[1:]
		session.plan = nil
		session.attempt = nil
		return *m, m.dispatchNextMutation()
	}
	if attempt.err != nil && attempt.ambiguous && verification.known && (verification.unchanged || verification.stablePartial) {
		observation, known := mutationPlanObservation(*session.plan, session.canonicalItems)
		if known {
			if observation == session.readbackState {
				session.readbackCount++
			} else {
				session.readbackState = observation
				session.readbackCount = 1
			}
			if session.readbackCount >= 2 {
				if session.notice == "" {
					if verification.stablePartial {
						session.notice = fmt.Sprintf("%s partially saved: repeated GitHub readbacks confirm the partial state", intent.description)
					} else {
						session.notice = fmt.Sprintf("%s failed: repeated GitHub readbacks confirm no change", intent.description)
					}
				}
				delete(m.detailCache, intent.itemID)
				session.intents = session.intents[1:]
				session.plan = nil
				session.attempt = nil
				session.readbackState = ""
				session.readbackCount = 0
				return *m, m.dispatchNextMutation()
			}
			session.blocked = true
			m.setMutationStatus(session, fmt.Sprintf("Readback has not yet confirmed the outcome of %s; press r to check again before queued writes continue", intent.description))
			return *m, m.showReconciledItems(session)
		}
	}
	session.readbackState = ""
	session.readbackCount = 0

	if attempt.err == nil || attempt.ambiguous || !verification.known {
		session.blocked = true
		m.setMutationStatus(session, fmt.Sprintf("Outcome of %s is not confirmed by GitHub readback; press r to reconcile again. Queued writes are blocked", intent.description))
		return *m, m.showReconciledItems(session)
	}

	if verification.appliedAny {
		if session.notice == "" {
			session.notice = fmt.Sprintf("%s partially saved: %v; the board reflects GitHub's state", intent.description, attempt.err)
		}
	} else if session.notice == "" {
		session.notice = fmt.Sprintf("%s failed: %v; the board reflects GitHub's state", intent.description, attempt.err)
	}
	delete(m.detailCache, intent.itemID)
	session.intents = session.intents[1:]
	session.plan = nil
	session.attempt = nil
	return *m, m.dispatchNextMutation()
}

func (m *Model) dispatchNextMutation() tea.Cmd {
	session := m.mutationSession
	if session == nil {
		m.mutationLoading = false
		return nil
	}
	for len(session.intents) > 0 {
		intent := session.intents[0]
		plan, err := resolveBoardMutationPlan(session.view, session.canonicalItems, intent)
		if err != nil {
			if session.notice == "" {
				session.notice = fmt.Sprintf("%s was not saved: %v; project state was refreshed", intent.description, err)
			}
			session.intents = session.intents[1:]
			continue
		}
		if plan.noOp {
			session.lastSuccess = intent.description + " already satisfied"
			session.intents = session.intents[1:]
			continue
		}
		plan.beforeItems = session.canonicalItems
		session.plan = &plan
		session.attempt = nil
		session.blocked = false
		session.readbackState = ""
		session.readbackCount = 0
		session.writeID++
		m.mutationLoading = true
		m.projectPendingMutations()
		m.setMutationStatus(session, fmt.Sprintf("Saving moves (%d queued)...", len(session.intents)-1))
		return m.applyMutationPlanCmd(session.id, session.writeID, plan)
	}

	refresh := m.showReconciledItems(session)
	status := session.notice
	if status == "" {
		status = session.lastSuccess
	}
	m.mutationSession = nil
	m.mutationLoading = false
	if status != "" && sameMutationProjectByModel(session, m) {
		m.status = status
	}
	return refresh
}

func (m *Model) applyMutationPlanCmd(sessionID, writeID uint64, plan boardMutationPlan) tea.Cmd {
	source := m.source
	ctx, cancel := context.WithTimeout(context.Background(), mutationRequestTimeout)
	return func() tea.Msg {
		defer cancel()
		mutator, ok := source.(ItemMutationSource)
		if !ok {
			return boardMutationMsg{sessionID: sessionID, writeID: writeID, result: mutationAttemptResult{err: fmt.Errorf("mutations are unavailable for this client")}}
		}
		result := mutationAttemptResult{}
		if plan.hasField {
			if plan.clearField {
				result.err = mutator.ClearItemFieldValue(ctx, github.ItemFieldValueClear{ProjectID: plan.projectID, ItemID: plan.intent.itemID, FieldID: plan.field.ID})
			} else {
				result.err = mutator.UpdateItemFieldValue(ctx, github.ItemFieldValueUpdate{ProjectID: plan.projectID, ItemID: plan.intent.itemID, FieldID: plan.field.ID, Value: plan.fieldValue})
			}
			if result.err != nil {
				result.ambiguous = github.IsAmbiguousMutationError(result.err)
				return boardMutationMsg{sessionID: sessionID, writeID: writeID, result: result}
			}
		}
		if plan.hasPosition {
			result.err = mutator.UpdateItemPosition(ctx, github.ItemPositionUpdate{ProjectID: plan.projectID, ItemID: plan.intent.itemID, AfterID: plan.positionAfter})
			if result.err != nil {
				result.ambiguous = github.IsAmbiguousMutationError(result.err)
				return boardMutationMsg{sessionID: sessionID, writeID: writeID, result: result}
			}
		}
		return boardMutationMsg{sessionID: sessionID, writeID: writeID, result: result}
	}
}

func (m *Model) projectPendingMutations() {
	session := m.mutationSession
	if session == nil || session.blocked || !sameMutationProjectByModel(session, m) || m.screen != screenBoard || m.view == nil || strings.TrimSpace(m.view.Filter) != "" || len(session.intents) == 0 {
		return
	}
	items := cloneItems(session.canonicalItems)
	for _, intent := range session.intents {
		plan, err := resolveBoardMutationPlan(session.view, items, intent)
		if err == nil && !plan.noOp {
			applyBoardMutationPlan(&items, plan)
		}
	}
	m.items = items
	m.boardFocusID = session.intents[len(session.intents)-1].itemID
	m.clampBoardCursor()
}

func (m *Model) showReconciledItems(session *boardMutationSession) tea.Cmd {
	if !sameMutationProjectByModel(session, m) || m.screen != screenBoard {
		return nil
	}
	if m.view != nil && strings.TrimSpace(m.view.Filter) != "" {
		if m.itemsLoading {
			m.cancelItemsLoad()
		}
		return m.startItemsLoadWithSpinner()
	}
	m.showCanonicalWithoutOptimism(session)
	return nil
}

func (m *Model) showCanonicalWithoutOptimism(session *boardMutationSession) {
	if !sameMutationProjectByModel(session, m) || m.screen != screenBoard {
		return
	}
	if m.itemsLoading {
		m.cancelItemsLoad()
	}
	m.items = cloneItems(session.canonicalItems)
	m.itemsHasNext = false
	m.itemsCursor = ""
	m.itemsErr = nil
	m.itemsLoadingLanes = nil
	m.itemsFailedLanes = nil
	m.itemsLanePending = 0
	m.clampBoardCursor()
}

func (m *Model) setMutationStatus(session *boardMutationSession, status string) {
	if sameMutationProjectByModel(session, m) {
		m.status = status
	}
}

func sameMutationProject(session *boardMutationSession, owner github.Owner, projectNumber int) bool {
	return strings.EqualFold(session.owner.Login, owner.Login) && session.owner.Kind == owner.Kind && session.projectNumber == projectNumber
}

func sameMutationProjectByModel(session *boardMutationSession, m *Model) bool {
	return m.selectedOwner != nil && m.selectedProject != nil && sameMutationProject(session, *m.selectedOwner, m.selectedProject.Number)
}

func resolveBoardMutationPlan(view github.View, items []github.Item, intent boardMutationIntent) (boardMutationPlan, error) {
	plan := boardMutationPlan{intent: intent, projectID: view.ProjectID}
	itemIndex := -1
	for index, item := range items {
		if item.ID == intent.itemID {
			itemIndex = index
			break
		}
	}
	if itemIndex < 0 {
		return boardMutationPlan{}, fmt.Errorf("card %q is no longer in the project", intent.itemID)
	}
	lanes := lanesForView(&view, sortedItemsForView(&view, items))
	switch intent.kind {
	case boardMutationMoveLane:
		field, grouped := mutationGroupingField(&view)
		if !grouped {
			return boardMutationPlan{}, fmt.Errorf("the saved grouping is no longer writable")
		}
		currentLane := -1
		for laneIndex, lane := range lanes {
			for _, laneItem := range lane.Items {
				if laneItem.ID == intent.itemID {
					currentLane = laneIndex
					break
				}
			}
			if currentLane >= 0 {
				break
			}
		}
		if currentLane < 0 {
			return boardMutationPlan{}, fmt.Errorf("card %q is no longer in a board lane", intent.itemID)
		}
		targetLane := currentLane + intent.direction
		if targetLane < 0 || targetLane >= len(lanes) {
			return boardMutationPlan{}, fmt.Errorf("card %q can no longer move in that direction", intent.itemID)
		}
		destination := &lanes[targetLane]
		currentKey, _ := itemGroupKey(field, items[itemIndex])
		if currentKey == destination.Key {
			plan.noOp = true
			return plan, nil
		}
		value, clear, err := fieldValueInputForLane(field, *destination)
		if err != nil {
			return boardMutationPlan{}, err
		}
		plan.field = field
		plan.fieldValue = value
		plan.hasField = true
		plan.clearField = clear
		if positionOnly(&view) && len(destination.Items) > 0 {
			after := destination.Items[len(destination.Items)-1].ID
			plan.hasPosition = true
			plan.positionAfter = &after
		}
		return plan, nil
	case boardMutationReorder:
		if !positionOnly(&view) || strings.TrimSpace(view.Filter) != "" {
			return boardMutationPlan{}, fmt.Errorf("manual reorder is not available for this saved view")
		}
		currentLane := -1
		currentCard := -1
		for laneIndex, lane := range lanes {
			for cardIndex, card := range lane.Items {
				if card.ID == intent.itemID {
					currentLane, currentCard = laneIndex, cardIndex
					break
				}
			}
			if currentLane >= 0 {
				break
			}
		}
		if currentLane < 0 {
			return boardMutationPlan{}, fmt.Errorf("card %q is no longer in a board lane", intent.itemID)
		}
		targetCard := currentCard + intent.direction
		if targetCard < 0 || targetCard >= len(lanes[currentLane].Items) {
			return boardMutationPlan{}, fmt.Errorf("card %q can no longer move in that direction", intent.itemID)
		}
		var after *string
		if intent.direction > 0 {
			anchor := lanes[currentLane].Items[targetCard].ID
			after = &anchor
		} else {
			targetID := lanes[currentLane].Items[targetCard].ID
			after = projectPredecessor(items, intent.itemID, targetID)
		}
		plan.hasPosition = true
		plan.positionAfter = after
		return plan, nil
	default:
		return boardMutationPlan{}, fmt.Errorf("unknown board movement intent")
	}
}

func projectPredecessor(items []github.Item, movingID, targetID string) *string {
	filtered := make([]github.Item, 0, len(items))
	for _, item := range items {
		if item.ID != movingID {
			filtered = append(filtered, item)
		}
	}
	for index, item := range filtered {
		if item.ID != targetID {
			continue
		}
		if index == 0 {
			return nil
		}
		predecessor := filtered[index-1].ID
		return &predecessor
	}
	return nil
}

func applyBoardMutationPlan(items *[]github.Item, plan boardMutationPlan) {
	if plan.hasField {
		for index := range *items {
			if (*items)[index].ID != plan.intent.itemID {
				continue
			}
			if plan.clearField {
				(*items)[index].FieldValues = removeFieldValue((*items)[index].FieldValues, plan.field)
			} else {
				(*items)[index].FieldValues = replaceFieldValue((*items)[index].FieldValues, plan.field, plan.fieldValue)
			}
			break
		}
	}
	if plan.hasPosition {
		if plan.positionAfter == nil {
			*items = moveItemToTop(*items, plan.intent.itemID)
		} else {
			*items = moveItemAfter(*items, plan.intent.itemID, *plan.positionAfter)
		}
	}
}

type mutationPlanVerification struct {
	complete      bool
	known         bool
	appliedAny    bool
	unchanged     bool
	stablePartial bool
}

func verifyMutationPlan(plan boardMutationPlan, items []github.Item) mutationPlanVerification {
	verification := mutationPlanVerification{known: true}
	itemIndex := -1
	for index, item := range items {
		if item.ID == plan.intent.itemID {
			itemIndex = index
			break
		}
	}
	if itemIndex < 0 {
		return mutationPlanVerification{}
	}
	complete := true
	allUnchanged := true
	partialState := true
	beforeIndex := projectItemIndex(plan.beforeItems, plan.intent.itemID)
	preconditionKnown := beforeIndex >= 0
	if plan.hasField {
		fieldSatisfied, known := mutationFieldSatisfied(plan, items[itemIndex])
		if !known {
			verification.known = false
		}
		fieldUnchanged := false
		if beforeIndex >= 0 {
			beforeValue, beforeKnown := mutationFieldState(plan.field, plan.beforeItems[beforeIndex])
			currentValue, currentKnown := mutationFieldState(plan.field, items[itemIndex])
			if !beforeKnown {
				preconditionKnown = false
			}
			fieldUnchanged = beforeKnown && currentKnown && beforeValue == currentValue
		}
		if fieldSatisfied {
			verification.appliedAny = true
		} else {
			complete = false
		}
		allUnchanged = allUnchanged && !fieldSatisfied && fieldUnchanged
		partialState = partialState && (fieldSatisfied || fieldUnchanged)
	}
	if plan.hasPosition {
		positionSatisfied, known := mutationPositionSatisfied(plan, itemIndex, items)
		if !known {
			verification.known = false
		}
		beforePredecessor, beforeKnown := projectPredecessorID(plan.beforeItems, plan.intent.itemID)
		currentPredecessor, currentKnown := projectPredecessorID(items, plan.intent.itemID)
		if !beforeKnown {
			preconditionKnown = false
		}
		positionUnchanged := beforeKnown && currentKnown && beforePredecessor == currentPredecessor
		if positionSatisfied {
			verification.appliedAny = true
		} else {
			complete = false
		}
		allUnchanged = allUnchanged && !positionSatisfied && positionUnchanged
		partialState = partialState && (positionSatisfied || positionUnchanged)
	}
	verification.complete = complete && verification.known
	verification.unchanged = verification.known && preconditionKnown && allUnchanged
	verification.stablePartial = verification.known && preconditionKnown && !verification.complete && verification.appliedAny && partialState
	return verification
}

func projectItemIndex(items []github.Item, itemID string) int {
	for index, item := range items {
		if item.ID == itemID {
			return index
		}
	}
	return -1
}

func projectPredecessorID(items []github.Item, itemID string) (string, bool) {
	index := projectItemIndex(items, itemID)
	if index < 0 {
		return "", false
	}
	if index == 0 {
		return "", true
	}
	return items[index-1].ID, true
}

func mutationFieldState(field github.Field, item github.Item) (string, bool) {
	value, found := itemFieldValue(field, item)
	if found && !value.Available {
		return "", false
	}
	key, _ := itemGroupKey(field, item)
	return key, true
}

func mutationPlanObservation(plan boardMutationPlan, items []github.Item) (string, bool) {
	var observation strings.Builder
	if plan.hasField {
		index := projectItemIndex(items, plan.intent.itemID)
		if index < 0 {
			return "", false
		}
		value, known := mutationFieldState(plan.field, items[index])
		if !known {
			return "", false
		}
		observation.WriteString("field=")
		observation.WriteString(value)
	}
	if plan.hasPosition {
		predecessor, known := projectPredecessorID(items, plan.intent.itemID)
		if !known {
			return "", false
		}
		observation.WriteString(";predecessor=")
		if predecessor == "" {
			observation.WriteString("<top>")
		} else {
			observation.WriteString(predecessor)
		}
	}
	return observation.String(), true
}

func mutationFieldSatisfied(plan boardMutationPlan, item github.Item) (bool, bool) {
	value, found := itemFieldValue(plan.field, item)
	if found && !value.Available {
		return false, false
	}
	if plan.clearField {
		key, _ := itemGroupKey(plan.field, item)
		return key == "no-value", true
	}
	if !found {
		return false, true
	}
	if plan.fieldValue.SingleSelectOptionID != nil {
		return value.OptionID == *plan.fieldValue.SingleSelectOptionID, true
	}
	if plan.fieldValue.IterationID != nil {
		return value.IterationID == *plan.fieldValue.IterationID, true
	}
	return false, false
}

func mutationPositionSatisfied(plan boardMutationPlan, itemIndex int, items []github.Item) (bool, bool) {
	if plan.positionAfter == nil {
		return itemIndex == 0, true
	}
	anchorIndex := -1
	for index, item := range items {
		if item.ID == *plan.positionAfter {
			anchorIndex = index
			break
		}
	}
	if anchorIndex < 0 {
		return false, false
	}
	return itemIndex == anchorIndex+1, true
}

func cloneItems(items []github.Item) []github.Item {
	cloned := append([]github.Item(nil), items...)
	for index := range cloned {
		cloned[index].FieldValues = append([]github.FieldValue(nil), items[index].FieldValues...)
	}
	return cloned
}

type boardMutationMsg struct {
	sessionID uint64
	writeID   uint64
	result    mutationAttemptResult
}

type mutationReadbackMsg struct {
	sessionID uint64
	readID    uint64
	items     []github.Item
	err       error
}
