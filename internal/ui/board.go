package ui

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/lipgloss"
	"github.com/mhuggins7278/gh-projects-tui/internal/github"
)

type boardLane struct {
	Key     string
	Name    string
	RowKey  string
	RowName string
	Items   []github.Item
	Loading bool
	Failed  bool
}

type laneDefinition struct {
	key  string
	name string
}

type laneItemsRequest struct {
	key    string
	name   string
	filter string
}

var itemsLoadingFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

func (m Model) boardLanes() []boardLane {
	lanes := lanesForView(m.view, m.boardItems())
	for index := range lanes {
		if m.itemsLoading && len(m.itemsLoadingLanes) == 0 {
			lanes[index].Loading = true
		} else {
			lanes[index].Loading = m.itemsLoadingLanes[lanes[index].Key]
		}
		lanes[index].Failed = m.itemsFailedLanes[lanes[index].Key]
	}
	return lanes
}

func (m Model) boardItems() []github.Item {
	items := sortedItemsForView(m.view, m.items)
	if strings.TrimSpace(m.filter) == "" {
		return items
	}
	filtered := make([]github.Item, 0, len(items))
	for _, item := range items {
		if itemMatchesBoardSearch(item, m.filter) {
			filtered = append(filtered, item)
		}
	}
	return filtered
}

func itemMatchesBoardSearch(item github.Item, query string) bool {
	query = strings.TrimSpace(query)
	if query == "" {
		return true
	}
	var searchText strings.Builder
	if item.Content != nil {
		fmt.Fprintf(&searchText, "%s %s %d %s %s %s ", item.Content.Kind, item.Content.Title, item.Content.Number, item.Content.Repository, item.Content.State, item.Content.URL)
	}
	for _, value := range item.FieldValues {
		searchText.WriteString(value.FieldName)
		searchText.WriteByte(' ')
		searchText.WriteString(value.Value)
		searchText.WriteByte(' ')
		if !value.Available {
			searchText.WriteString("unavailable ")
		}
	}
	return matchesFilter(searchText.String(), query)
}

func (m Model) itemsLoadingSpinner() string {
	return itemsLoadingFrames[m.itemsLoadFrame%len(itemsLoadingFrames)]
}

type itemSortValue struct {
	present bool
	number  float64
	text    string
	byText  bool
}

func sortedItemsForView(view *github.View, items []github.Item) []github.Item {
	ordered := append([]github.Item(nil), items...)
	if view == nil || positionOnly(view) || !sortFieldsSupported(view) {
		return ordered
	}
	sort.SliceStable(ordered, func(left, right int) bool {
		for _, field := range view.SortByFields {
			comparison := compareSortField(field.Field, ordered[left], ordered[right], strings.EqualFold(field.Direction, "DESC"))
			if comparison == 0 {
				continue
			}
			return comparison < 0
		}
		return false
	})
	return ordered
}

func sortFieldsSupported(view *github.View) bool {
	if view == nil {
		return true
	}
	for _, sortField := range view.SortByFields {
		if !supportedSortDirection(sortField.Direction) || !supportedSortField(sortField.Field) {
			return false
		}
	}
	return true
}

func compareSortField(field github.Field, left, right github.Item, descending bool) int {
	leftValue := itemSortValueFor(field, left)
	rightValue := itemSortValueFor(field, right)
	if !leftValue.present && !rightValue.present {
		return 0
	}
	// GitHub keeps unset values together after populated values in a board
	// sort; stable sorting preserves their existing project-position order.
	if !leftValue.present {
		return 1
	}
	if !rightValue.present {
		return -1
	}
	comparison := 0
	if leftValue.byText || rightValue.byText {
		comparison = strings.Compare(strings.ToLower(leftValue.text), strings.ToLower(rightValue.text))
	} else if leftValue.number < rightValue.number {
		comparison = -1
	} else if leftValue.number > rightValue.number {
		comparison = 1
	}
	if descending {
		return -comparison
	}
	return comparison
}

func itemSortValueFor(field github.Field, item github.Item) itemSortValue {
	if field.Name == "Title" || strings.EqualFold(field.DataType, "TITLE") {
		if item.Content == nil || strings.TrimSpace(item.Content.Title) == "" {
			return itemSortValue{}
		}
		return itemSortValue{present: true, text: item.Content.Title, byText: true}
	}
	if strings.EqualFold(field.DataType, "POSITION") {
		return itemSortValue{}
	}
	value, ok := itemFieldValue(field, item)
	if !ok || !value.Available || (value.Value == "" && value.OptionID == "" && value.IterationID == "") {
		return itemSortValue{}
	}

	if strings.EqualFold(field.DataType, "SINGLE_SELECT") {
		for index, option := range field.Options {
			if option.ID == value.OptionID {
				return itemSortValue{present: true, number: float64(index)}
			}
		}
	}
	if strings.EqualFold(field.DataType, "ITERATION") {
		for index, iteration := range field.Iterations {
			if iteration.ID == value.IterationID {
				return itemSortValue{present: true, number: float64(index)}
			}
		}
	}
	if strings.EqualFold(field.DataType, "NUMBER") {
		if number, err := strconv.ParseFloat(value.Value, 64); err == nil {
			return itemSortValue{present: true, number: number}
		}
	}
	return itemSortValue{present: true, text: value.Value, byText: true}
}

func (m Model) selectedBoardItem() (github.Item, bool) {
	lanes := m.boardLanes()
	if m.boardLane < 0 || m.boardLane >= len(lanes) {
		return github.Item{}, false
	}
	items := lanes[m.boardLane].Items
	if m.boardCard < 0 || m.boardCard >= len(items) {
		return github.Item{}, false
	}
	return items[m.boardCard], true
}

func boardGroupField(view *github.View) (github.Field, string, bool) {
	if view == nil {
		return github.Field{}, "", false
	}
	if len(view.GroupByFields) > 0 {
		return view.GroupByFields[0], "column", true
	}
	if len(view.VerticalGroupBy) > 0 {
		return view.VerticalGroupBy[0], "vertical", true
	}
	return github.Field{}, "", false
}

func boardCombinedFields(view *github.View) (github.Field, github.Field, bool) {
	if view == nil || len(view.GroupByFields) != 1 || len(view.VerticalGroupBy) != 1 {
		return github.Field{}, github.Field{}, false
	}
	return view.GroupByFields[0], view.VerticalGroupBy[0], true
}

func parallelLaneItemsRequests(view *github.View) ([]laneItemsRequest, bool) {
	field, _, grouped := boardGroupField(view)
	if !grouped || !strings.EqualFold(field.Name, "Status") || (field.Kind != "ProjectV2SingleSelectField" && field.DataType != "SINGLE_SELECT") {
		return nil, false
	}
	// Keep concurrency bounded to avoid trading latency for secondary-rate-limit
	// failures on unusually large status configurations.
	if len(field.Options) == 0 || len(field.Options)+2 > 8 {
		return nil, false
	}
	base := strings.TrimSpace(view.Filter)
	combine := func(filter string) string {
		if base == "" {
			return filter
		}
		return base + " " + filter
	}
	requests := make([]laneItemsRequest, 0, len(field.Options)+2)
	otherFilters := make([]string, 0, len(field.Options)+1)
	for _, option := range field.Options {
		statusFilter := "status:" + strconv.Quote(option.Name)
		requests = append(requests, laneItemsRequest{
			key:    "option:" + option.ID,
			name:   option.Name,
			filter: combine(statusFilter),
		})
		otherFilters = append(otherFilters, "-"+statusFilter)
	}
	requests = append(requests, laneItemsRequest{key: "no-value", name: "No value", filter: combine("no:status")})
	otherFilters = append(otherFilters, "-no:status")
	requests = append(requests, laneItemsRequest{key: "other", name: "Other", filter: combine(strings.Join(otherFilters, " "))})
	return requests, true
}

func lanesForView(view *github.View, items []github.Item) []boardLane {
	if view == nil {
		return nil
	}
	if columnField, verticalField, combined := boardCombinedFields(view); combined {
		return combinedLanesForView(columnField, verticalField, items)
	}
	field, _, hasGroup := boardGroupField(view)
	if !hasGroup {
		return []boardLane{{Key: "all", Name: "All items", Items: append([]github.Item(nil), items...)}}
	}

	definitions, supported := laneDefinitions(field)
	lanes := make([]boardLane, 0, len(definitions)+1)
	indexes := make(map[string]int, len(definitions))
	for _, definition := range definitions {
		indexes[definition.key] = len(lanes)
		lanes = append(lanes, boardLane{Key: definition.key, Name: definition.name})
	}
	if !supported {
		return []boardLane{{Key: "all", Name: "All items", Items: append([]github.Item(nil), items...)}}
	}

	for _, item := range items {
		key, label := itemGroupKey(field, item)
		index, ok := indexes[key]
		if !ok {
			// Keep an item visible when GitHub returns a value not present in
			// the saved view metadata.
			if key == "no-value" {
				index = indexes[key]
			} else {
				if len(lanes) == 0 || lanes[len(lanes)-1].Key != "other" {
					lanes = append(lanes, boardLane{Key: "other", Name: "Other"})
				}
				index = len(lanes) - 1
			}
			// The card remains in one stable fallback lane; its actual
			// value is still shown in the card fields below.
			_ = label
		}
		lanes[index].Items = append(lanes[index].Items, item)
	}
	return lanes
}

func combinedLanesForView(columnField, verticalField github.Field, items []github.Item) []boardLane {
	columns, columnsSupported := laneDefinitions(columnField)
	rows, rowsSupported := laneDefinitions(verticalField)
	if !columnsSupported || !rowsSupported {
		return []boardLane{{Key: "all", Name: "All items", Items: append([]github.Item(nil), items...)}}
	}

	columnKeys := make(map[string]bool, len(columns))
	rowKeys := make(map[string]bool, len(rows))
	needsOtherColumn := false
	needsOtherRow := false
	for _, item := range items {
		columnKey, _ := itemGroupKey(columnField, item)
		if !containsLaneKey(columns, columnKey) && columnKey != "no-value" {
			needsOtherColumn = true
		}
		rowKey, _ := itemGroupKey(verticalField, item)
		if !containsLaneKey(rows, rowKey) && rowKey != "no-value" {
			needsOtherRow = true
		}
	}
	if needsOtherColumn {
		columns = append(columns, laneDefinition{key: "other", name: "Other"})
	}
	if needsOtherRow {
		rows = append(rows, laneDefinition{key: "other", name: "Other"})
	}

	lanes := make([]boardLane, 0, len(columns)*len(rows))
	indexes := make(map[string]int, len(columns)*len(rows))
	for _, row := range rows {
		rowKeys[row.key] = true
		for _, column := range columns {
			columnKeys[column.key] = true
			key := combinedLaneKey(row.key, column.key)
			indexes[key] = len(lanes)
			lanes = append(lanes, boardLane{
				Key:     key,
				Name:    column.name,
				RowKey:  row.key,
				RowName: row.name,
			})
		}
	}

	for _, item := range items {
		columnKey, _ := itemGroupKey(columnField, item)
		if !columnKeys[columnKey] {
			columnKey = "other"
		}
		rowKey, _ := itemGroupKey(verticalField, item)
		if !rowKeys[rowKey] {
			rowKey = "other"
		}
		if index, ok := indexes[combinedLaneKey(rowKey, columnKey)]; ok {
			lanes[index].Items = append(lanes[index].Items, item)
		}
	}
	return lanes
}

func containsLaneKey(definitions []laneDefinition, key string) bool {
	for _, definition := range definitions {
		if definition.key == key {
			return true
		}
	}
	return false
}

func combinedLaneKey(rowKey, columnKey string) string {
	return rowKey + "\x00" + columnKey
}

func laneDefinitions(field github.Field) ([]laneDefinition, bool) {
	switch {
	case field.Kind == "ProjectV2SingleSelectField" || field.DataType == "SINGLE_SELECT":
		definitions := []laneDefinition{{key: "no-value", name: noValueLaneName(field)}}
		for _, option := range field.Options {
			definitions = append(definitions, laneDefinition{key: "option:" + option.ID, name: option.Name})
		}
		return definitions, true
	case field.Kind == "ProjectV2IterationField" || field.DataType == "ITERATION":
		definitions := []laneDefinition{{key: "no-value", name: noValueLaneName(field)}}
		for _, iteration := range field.Iterations {
			definitions = append(definitions, laneDefinition{key: "iteration:" + iteration.ID, name: iteration.Title})
		}
		return definitions, true
	default:
		return nil, false
	}
}

func noValueLaneName(field github.Field) string {
	if strings.EqualFold(field.Name, "Status") {
		return "No Status"
	}
	return "No value"
}

func itemGroupKey(field github.Field, item github.Item) (string, string) {
	value, ok := itemFieldValue(field, item)
	if !ok || !value.Available {
		return "no-value", ""
	}
	if field.Kind == "ProjectV2IterationField" || field.DataType == "ITERATION" {
		if value.IterationID == "" {
			return "no-value", value.Value
		}
		return "iteration:" + value.IterationID, value.Value
	}
	if value.OptionID == "" {
		if value.Value == "" {
			return "no-value", ""
		}
		return "value:" + value.Value, value.Value
	}
	return "option:" + value.OptionID, value.Value
}

func itemFieldValue(field github.Field, item github.Item) (github.FieldValue, bool) {
	for _, value := range item.FieldValues {
		if value.FieldID == field.ID || (field.ID == "" && value.FieldName == field.Name) {
			return value, true
		}
	}
	return github.FieldValue{}, false
}

func (m Model) boardMutationUnavailable() string {
	if m.view == nil {
		return "Mutations require a loaded board view"
	}
	if !m.view.ViewerCanUpdate {
		return "This project is read-only"
	}
	if m.view.ProjectID == "" {
		return "Project identity is unavailable; refresh the view"
	}
	if m.itemsLoading {
		return "Wait for the board to finish loading"
	}
	if m.itemsErr != nil {
		return "Refresh before mutating an incomplete board"
	}
	if strings.TrimSpace(m.view.Filter) != "" {
		return "Saved-filtered views are read-only until hidden anchors are handled"
	}
	if m.filtering || strings.TrimSpace(m.filter) != "" {
		return "Clear local search before mutating the board"
	}
	if len(m.view.GroupByFields) > 1 || len(m.view.VerticalGroupBy) > 1 {
		return "Mutations require one grouping field"
	}
	if len(m.view.GroupByFields) > 0 && len(m.view.VerticalGroupBy) > 0 {
		return "Combined swimlane views are read-only"
	}
	field, grouped := mutationGroupingField(m.view)
	if !grouped {
		return "Mutations require one grouping field"
	}
	if !isSingleSelectField(field) && !isIterationField(field) {
		return "This grouping field cannot be mutated yet"
	}
	if _, ok := m.source.(ItemMutationSource); !ok {
		return "Mutations are unavailable for this client"
	}
	return ""
}

func mutationGroupingField(view *github.View) (github.Field, bool) {
	if view == nil {
		return github.Field{}, false
	}
	if len(view.GroupByFields) == 1 && len(view.VerticalGroupBy) == 0 {
		return view.GroupByFields[0], true
	}
	if len(view.GroupByFields) == 0 && len(view.VerticalGroupBy) == 1 {
		return view.VerticalGroupBy[0], true
	}
	return github.Field{}, false
}

func fieldValueInputForLane(field github.Field, lane boardLane) (github.FieldValueInput, bool, error) {
	if lane.Key == "no-value" {
		return github.FieldValueInput{}, true, nil
	}
	if isSingleSelectField(field) && strings.HasPrefix(lane.Key, "option:") {
		optionID := strings.TrimPrefix(lane.Key, "option:")
		if optionID == "" {
			return github.FieldValueInput{}, false, fmt.Errorf("destination lane has no option ID")
		}
		return github.FieldValueInput{SingleSelectOptionID: &optionID}, false, nil
	}
	if isIterationField(field) && strings.HasPrefix(lane.Key, "iteration:") {
		iterationID := strings.TrimPrefix(lane.Key, "iteration:")
		if iterationID == "" {
			return github.FieldValueInput{}, false, fmt.Errorf("destination lane has no iteration ID")
		}
		return github.FieldValueInput{IterationID: &iterationID}, false, nil
	}
	return github.FieldValueInput{}, false, fmt.Errorf("destination lane is not a writable value")
}

func (m *Model) moveBoardLaneMutation(delta int) tea.Cmd {
	if reason := m.boardMutationUnavailable(); reason != "" {
		m.status = reason
		return nil
	}
	lanes := m.boardLanes()
	if m.boardLane < 0 || m.boardLane >= len(lanes) {
		m.status = "No card lane is selected"
		return nil
	}
	targetLane := m.boardLane + delta
	if targetLane < 0 || targetLane >= len(lanes) {
		m.status = "No destination lane in that direction"
		return nil
	}
	item, ok := m.selectedBoardItem()
	if !ok {
		m.status = "No card is selected"
		return nil
	}
	field, _ := mutationGroupingField(m.view)
	value, clear, err := fieldValueInputForLane(field, lanes[targetLane])
	if err != nil {
		m.status = err.Error()
		return nil
	}
	projectID := m.view.ProjectID
	afterID := ""
	if positionOnly(m.view) && len(lanes[targetLane].Items) > 0 {
		afterID = lanes[targetLane].Items[len(lanes[targetLane].Items)-1].ID
	}
	action := func(ctx context.Context, source ItemMutationSource) (error, bool) {
		if clear {
			err := source.ClearItemFieldValue(ctx, github.ItemFieldValueClear{ProjectID: projectID, ItemID: item.ID, FieldID: field.ID})
			if err != nil {
				return err, false
			}
		} else {
			err := source.UpdateItemFieldValue(ctx, github.ItemFieldValueUpdate{ProjectID: projectID, ItemID: item.ID, FieldID: field.ID, Value: value})
			if err != nil {
				return err, false
			}
		}
		if afterID == "" {
			return nil, false
		}
		if err := source.UpdateItemPosition(ctx, github.ItemPositionUpdate{ProjectID: projectID, ItemID: item.ID, AfterID: &afterID}); err != nil {
			return err, true
		}
		return nil, false
	}
	return m.beginBoardMutation(item.ID, "Move card", action, func() {
		m.applyOptimisticLaneMove(item.ID, field, value, clear, afterID)
	})
}

func (m *Model) reorderBoardItem(delta int) tea.Cmd {
	if reason := m.boardMutationUnavailable(); reason != "" {
		m.status = reason
		return nil
	}
	if !positionOnly(m.view) {
		m.status = "Manual reorder is disabled when the view has a saved field sort"
		return nil
	}
	if strings.TrimSpace(m.view.Filter) != "" {
		m.status = "Manual reorder is disabled for filtered saved views"
		return nil
	}
	lanes := m.boardLanes()
	if m.boardLane < 0 || m.boardLane >= len(lanes) {
		m.status = "No card lane is selected"
		return nil
	}
	lane := lanes[m.boardLane]
	if m.boardCard < 0 || m.boardCard >= len(lane.Items) {
		m.status = "No card is selected"
		return nil
	}
	target := m.boardCard + delta
	if target < 0 || target >= len(lane.Items) {
		m.status = "No card in that direction"
		return nil
	}
	item := lane.Items[m.boardCard]
	var afterID *string
	if delta > 0 {
		anchor := lane.Items[target].ID
		afterID = &anchor
	} else if m.boardCard > 1 {
		anchor := lane.Items[m.boardCard-2].ID
		afterID = &anchor
	} else if anchor := previousLaneAnchor(lanes, m.boardLane); anchor != "" {
		afterID = &anchor
	}
	projectID := m.view.ProjectID
	action := func(ctx context.Context, source ItemMutationSource) (error, bool) {
		return source.UpdateItemPosition(ctx, github.ItemPositionUpdate{ProjectID: projectID, ItemID: item.ID, AfterID: afterID}), false
	}
	optimisticAfterID := ""
	if afterID != nil {
		optimisticAfterID = *afterID
	}
	return m.beginBoardMutation(item.ID, "Reorder card", action, func() {
		m.applyOptimisticPosition(item.ID, optimisticAfterID)
	})
}

func (m *Model) applyOptimisticLaneMove(itemID string, field github.Field, input github.FieldValueInput, clear bool, afterID string) {
	for index := range m.items {
		if m.items[index].ID != itemID {
			continue
		}
		if clear {
			m.items[index].FieldValues = removeFieldValue(m.items[index].FieldValues, field)
		} else {
			m.items[index].FieldValues = replaceFieldValue(m.items[index].FieldValues, field, input)
		}
		break
	}
	if afterID != "" {
		m.items = moveItemAfter(m.items, itemID, afterID)
	}
	m.boardFocusID = itemID
	m.clampBoardCursor()
}

func (m *Model) applyOptimisticPosition(itemID, afterID string) {
	if afterID == "" {
		m.items = moveItemToTop(m.items, itemID)
	} else {
		m.items = moveItemAfter(m.items, itemID, afterID)
	}
	m.boardFocusID = itemID
	m.clampBoardCursor()
}

func previousLaneAnchor(lanes []boardLane, laneIndex int) string {
	for index := laneIndex - 1; index >= 0; index-- {
		if len(lanes[index].Items) > 0 {
			return lanes[index].Items[len(lanes[index].Items)-1].ID
		}
	}
	return ""
}

func replaceFieldValue(values []github.FieldValue, field github.Field, input github.FieldValueInput) []github.FieldValue {
	updated := github.FieldValue{FieldID: field.ID, FieldName: field.Name, Available: true}
	if input.SingleSelectOptionID != nil {
		updated.OptionID = *input.SingleSelectOptionID
		for _, option := range field.Options {
			if option.ID == updated.OptionID {
				updated.Value = option.Name
				break
			}
		}
	}
	if input.IterationID != nil {
		updated.IterationID = *input.IterationID
		for _, iteration := range field.Iterations {
			if iteration.ID == updated.IterationID {
				updated.Value = iteration.Title
				break
			}
		}
	}
	for index, value := range values {
		if !matchesField(value, field) {
			continue
		}
		updated.Kind = value.Kind
		values[index] = updated
		return values
	}
	return append(values, updated)
}

func removeFieldValue(values []github.FieldValue, field github.Field) []github.FieldValue {
	filtered := values[:0]
	for _, value := range values {
		if !matchesField(value, field) {
			filtered = append(filtered, value)
		}
	}
	return filtered
}

func matchesField(value github.FieldValue, field github.Field) bool {
	if field.ID != "" {
		return value.FieldID == field.ID
	}
	return value.FieldName == field.Name
}

func moveItemAfter(items []github.Item, itemID, afterID string) []github.Item {
	if itemID == "" || afterID == "" || itemID == afterID {
		return items
	}
	itemIndex := -1
	for index, item := range items {
		if item.ID == itemID {
			itemIndex = index
			break
		}
	}
	if itemIndex < 0 {
		return items
	}
	item := items[itemIndex]
	without := append(append([]github.Item(nil), items[:itemIndex]...), items[itemIndex+1:]...)
	anchorIndex := -1
	for index, candidate := range without {
		if candidate.ID == afterID {
			anchorIndex = index
			break
		}
	}
	if anchorIndex < 0 {
		return items
	}
	result := make([]github.Item, 0, len(items))
	result = append(result, without[:anchorIndex+1]...)
	result = append(result, item)
	result = append(result, without[anchorIndex+1:]...)
	return result
}

func moveItemToTop(items []github.Item, itemID string) []github.Item {
	itemIndex := -1
	for index, item := range items {
		if item.ID == itemID {
			itemIndex = index
			break
		}
	}
	if itemIndex <= 0 {
		return items
	}
	item := items[itemIndex]
	result := make([]github.Item, 0, len(items))
	result = append(result, item)
	result = append(result, items[:itemIndex]...)
	result = append(result, items[itemIndex+1:]...)
	return result
}

func (m *Model) moveBoardLane(delta int) {
	lanes := m.boardLanes()
	if len(lanes) == 0 {
		return
	}
	m.boardLane += delta
	if m.boardLane < 0 {
		m.boardLane = 0
	}
	if m.boardLane >= len(lanes) {
		m.boardLane = len(lanes) - 1
	}
	m.boardCard = 0
	if len(lanes[m.boardLane].Items) > 0 {
		m.boardCard = minInt(m.boardCard, len(lanes[m.boardLane].Items)-1)
		m.rememberBoardFocus()
	} else {
		m.boardFocusID = ""
	}
}

func (m *Model) moveBoardCard(delta int) {
	lanes := m.boardLanes()
	if m.boardLane < 0 || m.boardLane >= len(lanes) || len(lanes[m.boardLane].Items) == 0 {
		return
	}
	m.boardCard += delta
	if m.boardCard < 0 {
		m.boardCard = 0
	}
	if m.boardCard >= len(lanes[m.boardLane].Items) {
		m.boardCard = len(lanes[m.boardLane].Items) - 1
	}
	m.rememberBoardFocus()
}

func minInt(left, right int) int {
	if left < right {
		return left
	}
	return right
}

func (m Model) renderBoard(b *strings.Builder) {
	board := &strings.Builder{}
	m.renderBoardContent(board)
	if !m.detailVisible {
		b.WriteString(board.String())
		return
	}
	b.WriteString(renderPopover(board.String(), m.renderDetailPanel(), m.frameContentWidth(), m.bodyCanvasHeight()))
}

func (m Model) renderBoardContent(b *strings.Builder) {
	if m.loadingDetail {
		b.WriteString("Loading view...\n")
		return
	}
	if m.view == nil {
		b.WriteString("No view loaded. Press v to pick a view.\n")
		return
	}
	if m.view.Layout == github.TableLayout {
		m.renderTableContent(b)
		return
	}

	fmt.Fprintf(b, "%s  %s  %s\n", titleStyle.Render(m.view.Name), mutedStyle.Render(fmt.Sprintf("#%d", m.view.Number)), layoutBadge(string(m.view.Layout)))
	if m.view.Filter != "" {
		fmt.Fprintf(b, "%s %s\n", mutedStyle.Render("Filter:"), m.view.Filter)
	}
	visibleItems := m.boardItems()
	if m.filtering || strings.TrimSpace(m.filter) != "" {
		search := m.filter
		if m.filtering {
			search += "_"
		}
		fmt.Fprintf(b, "%s %s (%d matches)\n", statusStyle.Render("Search:"), search, len(visibleItems))
	}
	columnField, verticalField, combined := boardCombinedFields(m.view)
	field, axis, hasGroup := boardGroupField(m.view)
	if combined {
		columnName := columnField.Name
		if columnName == "" {
			columnName = "unknown field"
		}
		verticalName := verticalField.Name
		if verticalName == "" {
			verticalName = "unknown field"
		}
		fmt.Fprintf(b, "%s %s columns / %s swimlanes\n", mutedStyle.Render("Grouping:"), columnName, verticalName)
	} else if !hasGroup {
		b.WriteString(mutedStyle.Render("Grouping: none") + "\n")
	} else {
		label := field.Name
		if label == "" {
			label = "unknown field"
		}
		if _, supported := laneDefinitions(field); supported {
			fmt.Fprintf(b, "%s %s (%s)\n", mutedStyle.Render("Grouping:"), label, axis)
		} else {
			fmt.Fprintf(b, "%s %s (%s; display fallback; field type is not supported yet)\n", statusStyle.Render("Grouping:"), label, axis)
		}
	}
	if len(m.view.SortByFields) > 0 && !positionOnly(m.view) {
		if sortFieldsSupported(m.view) {
			b.WriteString(statusStyle.Render("Sort: saved field sort applied") + "\n")
		} else {
			b.WriteString(statusStyle.Render("Sort: saved field sort is not applied yet; showing project position") + "\n")
		}
	}
	if strings.TrimSpace(m.filter) == "" {
		fmt.Fprintf(b, "%s %d", mutedStyle.Render("Items:"), len(m.items))
	} else {
		fmt.Fprintf(b, "%s %d/%d", mutedStyle.Render("Items:"), len(visibleItems), len(m.items))
	}
	if m.itemsLoading {
		b.WriteString("  " + statusStyle.Render(m.itemsLoadingSpinner()+" loading"))
	}
	b.WriteString("\n")

	if m.itemsErr != nil && len(m.items) == 0 && !m.itemsLoading {
		b.WriteString("\n" + errorStyle.Render("Items failed: "+m.itemsErr.Error()) + "\n")
		b.WriteString(mutedStyle.Render("Press r to retry.") + "\n")
		return
	}
	if len(m.items) == 0 && !m.itemsLoading {
		b.WriteString("\n" + mutedStyle.Render("No issues, pull requests, or drafts match this view.") + "\n")
		return
	}
	if len(visibleItems) == 0 && !m.itemsLoading {
		b.WriteString("\n" + mutedStyle.Render("No cards match the current search.") + "\n")
		return
	}

	lanes := m.boardLanes()
	b.WriteString("\n" + m.renderLaneGrid(lanes) + "\n")
	if m.itemsErr != nil {
		b.WriteString(errorStyle.Render("Lane loading failed: "+m.itemsErr.Error()) + "\n")
	}
}

func (m Model) renderLaneGrid(lanes []boardLane) string {
	if len(lanes) > 0 && lanes[0].RowKey != "" {
		return m.renderSwimlaneGrid(lanes)
	}
	columns, laneWidth := m.boardGrid(len(lanes))
	viewportHeight := m.boardViewportHeight()
	rowStart := 0
	rowEnd := len(lanes)
	if columns < len(lanes) {
		// Keep the active lane's row on screen on narrow terminals. h/l then
		// acts as a horizontal lane navigator without losing focus below the
		// visible viewport.
		rowStart = (m.boardLane / columns) * columns
		rowEnd = rowStart + columns
		if rowEnd > len(lanes) {
			rowEnd = len(lanes)
		}
	}
	blocks := make([]string, 0, (rowEnd-rowStart)*2-1)
	for laneIndex := rowStart; laneIndex < rowEnd; laneIndex++ {
		blocks = append(blocks, m.renderLaneColumn(lanes[laneIndex], laneIndex, laneWidth, viewportHeight))
		if laneIndex+1 < rowEnd {
			blocks = append(blocks, "  ")
		}
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, blocks...)
}

func (m Model) renderSwimlaneGrid(lanes []boardLane) string {
	rowCount := 0
	for start := 0; start < len(lanes); {
		rowCount++
		rowKey := lanes[start].RowKey
		start++
		for start < len(lanes) && lanes[start].RowKey == rowKey {
			start++
		}
	}
	rowHeight := m.swimlaneViewportHeight(rowCount)

	rows := make([]string, 0, rowCount)
	for start := 0; start < len(lanes); {
		end := start + 1
		for end < len(lanes) && lanes[end].RowKey == lanes[start].RowKey {
			end++
		}

		columns, laneWidth := m.boardGrid(end - start)
		visibleStart := start
		if m.boardLane >= start && m.boardLane < end {
			visibleStart = start + ((m.boardLane-start)/columns)*columns
		}
		visibleEnd := visibleStart + columns
		if visibleEnd > end {
			visibleEnd = end
		}
		blocks := make([]string, 0, (visibleEnd-visibleStart)*2-1)
		for laneIndex := visibleStart; laneIndex < visibleEnd; laneIndex++ {
			blocks = append(blocks, m.renderLaneColumn(lanes[laneIndex], laneIndex, laneWidth, rowHeight))
			if laneIndex+1 < visibleEnd {
				blocks = append(blocks, "  ")
			}
		}
		row := lipgloss.JoinHorizontal(lipgloss.Top, blocks...)
		label := lanes[start].RowName
		if label == "" {
			label = "Unnamed swimlane"
		}
		rows = append(rows, lipgloss.JoinVertical(lipgloss.Left, statusStyle.Render("Swimlane: "+label), row))
		start = end
	}
	return lipgloss.JoinVertical(lipgloss.Left, rows...)
}

func (m Model) swimlaneViewportHeight(rowCount int) int {
	viewport := m.boardViewportHeight()
	if rowCount <= 1 {
		return viewport
	}
	rowHeight := (viewport - (rowCount * 2) + 1) / rowCount
	if rowHeight < 4 {
		return 4
	}
	return rowHeight
}

func (m Model) boardGrid(laneCount int) (int, int) {
	available := m.frameContentWidth() - 2
	if available < 20 {
		available = 20
	}
	columns := available / 29
	if columns < 1 {
		columns = 1
	}
	if columns > laneCount {
		columns = laneCount
	}
	if columns < 1 {
		columns = 1
	}
	totalWidth := (available - (columns-1)*2) / columns
	contentWidth := totalWidth - 4
	if contentWidth < 12 {
		contentWidth = 12
	}
	return columns, contentWidth
}

func (m Model) renderLaneColumn(lane boardLane, laneIndex, width, viewportHeight int) string {
	active := laneIndex == m.boardLane
	header := m.renderLaneHeader(lane, laneIndex, active)
	lines := []string{header}
	if len(lane.Items) == 0 {
		if lane.Failed {
			lines = append(lines, errorStyle.Render("Load failed; press r"))
		} else if lane.Loading {
			lines = append(lines, mutedStyle.Render("Loading cards..."))
		} else {
			lines = append(lines, mutedStyle.Render("No cards"))
		}
		return laneFrameStyle(laneIndex).Width(width).Render(strings.Join(lines, "\n"))
	}

	start, end := m.boardWindowForLane(lane, m.boardCard, width, viewportHeight, active)
	showEarlier := start > 0
	showLater := end < len(lane.Items)
	column := m.renderLaneWindow(lane, laneIndex, width, start, end, showEarlier, showLater, active)
	for lipgloss.Height(column) > viewportHeight {
		if showLater {
			showLater = false
		} else if showEarlier {
			showEarlier = false
		} else if end-start > 1 {
			if active && m.boardCard == end-1 {
				start++
			} else {
				end--
			}
		} else {
			break
		}
		column = m.renderLaneWindow(lane, laneIndex, width, start, end, showEarlier, showLater, active)
	}
	return column
}

func (m Model) renderLaneWindow(lane boardLane, laneIndex, width, start, end int, showEarlier, showLater, active bool) string {
	lines := []string{m.renderLaneHeader(lane, laneIndex, active)}
	if showEarlier {
		lines = append(lines, mutedStyle.Render(fmt.Sprintf("... %d earlier", start)))
	}
	for itemIndex := start; itemIndex < end; itemIndex++ {
		lines = append(lines, m.renderLaneCard(lane.Items[itemIndex], itemIndex == m.boardCard && active, width))
	}
	if showLater {
		lines = append(lines, mutedStyle.Render(fmt.Sprintf("... %d more", len(lane.Items)-end)))
	}
	if lane.Failed {
		lines = append(lines, errorStyle.Render("... incomplete; press r"))
	}
	return laneFrameStyle(laneIndex).Width(width).Render(strings.Join(lines, "\n"))
}

func (m Model) renderLaneHeader(lane boardLane, laneIndex int, active bool) string {
	marker := "  "
	if active {
		marker = "> "
	}
	label := fmt.Sprintf("%s[%s]", marker, lane.Name)
	if lane.Loading {
		label += "  " + m.itemsLoadingSpinner()
	} else {
		label += "  " + itemCountLabel(len(lane.Items))
	}
	if lane.Failed {
		label += " !"
	}
	return laneTitleStyle(laneIndex).Render(label)
}

func (m Model) renderLaneCard(item github.Item, selected bool, width int) string {
	cardWidth := width - 4
	if cardWidth < 8 {
		cardWidth = 8
	}
	if item.Content == nil {
		return cardStyle(selected).Width(cardWidth).Render(mutedStyle.Render("content unavailable"))
	}
	identity := contentCardIdentity(item.Content)
	icon := contentIcon(item.Content)
	iconStyle := itemKindStyle(contentKind(item.Content))
	if state := contentState(item.Content); state != "" {
		iconStyle = itemStateStyle(state)
	}
	identityWidth := cardWidth - 2
	prefix := ""
	if selected {
		prefix = "> "
	}
	identityAvailable := identityWidth - lipgloss.Width(prefix) - lipgloss.Width(icon) - 1
	if identityAvailable > 0 {
		identity = truncateText(identity, identityAvailable)
	} else {
		identity = ""
	}
	identityLine := prefix + iconStyle.Render(icon)
	if identity != "" {
		identityLine += " " + mutedStyle.Render(identity)
	}
	title := strings.TrimSpace(item.Content.Title)
	if title == "" {
		title = "(untitled)"
	}
	titleLines := wrapText(title, cardWidth-2)
	if len(titleLines) > 2 {
		titleLines = titleLines[:2]
		titleLines[1] = truncateText(titleLines[1]+"...", cardWidth-2)
	}
	lines := []string{identityLine}
	for _, line := range titleLines {
		lines = append(lines, titleStyle.Render(line))
	}
	if summary := cardFieldSummary(item, m.view); summary != "" {
		lines = append(lines, mutedStyle.Render(truncateText(summary, cardWidth-2)))
	}
	if summary := subIssueSummary(item.Content); summary != "" {
		lines = append(lines, mutedStyle.Render(truncateText(summary, cardWidth-2)))
	}
	return cardStyle(selected).Width(cardWidth).Render(strings.Join(lines, "\n"))
}

func contentCardIdentity(content *github.Content) string {
	identity := ""
	if content.Repository != "" {
		identity = content.Repository
	}
	if content.Number > 0 {
		if identity != "" {
			identity += " "
		}
		identity += fmt.Sprintf("#%d", content.Number)
	}
	if identity == "" {
		identity = contentKind(content)
	}
	return identity
}

func contentIcon(content *github.Content) string {
	switch content.Kind {
	case "Issue":
		if strings.EqualFold(content.State, "CLOSED") {
			return "✓"
		}
		return "○"
	case "PullRequest":
		if content.IsDraft {
			return "◌"
		}
		if content.Merged {
			return "↔"
		}
		if strings.EqualFold(content.State, "CLOSED") {
			return "×"
		}
		return "↗"
	case "DraftIssue":
		return "◇"
	default:
		return "•"
	}
}

func contentState(content *github.Content) string {
	if content == nil {
		return ""
	}
	if content.IsDraft {
		return "DRAFT"
	}
	if content.Merged {
		return "MERGED"
	}
	switch strings.ToUpper(content.State) {
	case "OPEN":
		return "OPEN"
	case "CLOSED":
		return "CLOSED"
	default:
		return ""
	}
}

func subIssueSummary(content *github.Content) string {
	if content == nil || content.SubIssueTotal <= 0 {
		return ""
	}
	percent := (content.SubIssueDone*100 + content.SubIssueTotal/2) / content.SubIssueTotal
	return fmt.Sprintf("Sub-issues: %d/%d (%d%%)", content.SubIssueDone, content.SubIssueTotal, percent)
}

func cardFieldSummary(item github.Item, view *github.View) string {
	if view == nil {
		return ""
	}
	parts := make([]string, 0, len(view.Fields))
	for _, field := range view.Fields {
		if strings.EqualFold(field.Name, "Title") || strings.EqualFold(field.DataType, "TITLE") {
			continue
		}
		value, ok := itemFieldValue(field, item)
		if !ok || !value.Available || strings.TrimSpace(value.Value) == "" {
			continue
		}
		name := field.Name
		if name == "" {
			name = value.FieldName
		}
		if name == "" {
			continue
		}
		parts = append(parts, name+": "+value.Value)
	}
	return strings.Join(parts, " · ")
}

func positionOnly(view *github.View) bool {
	if view == nil || len(view.SortByFields) == 0 {
		return true
	}
	for _, sort := range view.SortByFields {
		if sort.Field.DataType != "POSITION" && sort.Field.Name != "Position" {
			return false
		}
	}
	return true
}

func (m Model) boardViewportHeight() int {
	if m.height <= 0 {
		return 32
	}
	reserved := 16
	if m.view != nil {
		if m.view.Filter != "" {
			reserved++
		}
		if len(m.view.SortByFields) > 0 && !positionOnly(m.view) {
			reserved++
		}
	}
	if m.showHelp {
		reserved += 5
	}
	if m.status != "" {
		reserved++
	}
	viewport := m.height - reserved
	if viewport < 8 {
		return 8
	}
	return viewport
}

func (m Model) boardWindowForLane(lane boardLane, selected, width, viewportHeight int, active bool) (int, int) {
	if len(lane.Items) == 0 {
		return 0, 0
	}
	if selected < 0 {
		selected = 0
	}
	if selected >= len(lane.Items) {
		selected = len(lane.Items) - 1
	}

	heights := make([]int, len(lane.Items))
	for i, item := range lane.Items {
		heights[i] = lipgloss.Height(m.renderLaneCard(item, false, width))
		if heights[i] < 1 {
			heights[i] = 1
		}
	}

	// A lane frame consumes two border rows and one header row. Reserve
	// another row for each overflow marker so the selected card remains in the
	// actual terminal viewport, not just in the logical item window.
	capacity := viewportHeight - 3
	if active {
		if selected > 0 {
			capacity--
		}
		if selected < len(lane.Items)-1 {
			capacity--
		}
	} else if len(lane.Items) > 1 {
		capacity--
	}
	if capacity < 1 {
		capacity = 1
	}

	if !active {
		used := 0
		end := 0
		for end < len(heights) && (end == 0 || used+heights[end] <= capacity) {
			used += heights[end]
			end++
		}
		if end == 0 {
			end = 1
		}
		return 0, end
	}

	start, end := selected, selected+1
	used := heights[selected]
	for start > 0 || end < len(heights) {
		beforeHeight := 0
		if start > 0 {
			beforeHeight = heights[start-1]
		}
		afterHeight := 0
		if end < len(heights) {
			afterHeight = heights[end]
		}
		preferBefore := start > 0 && (end >= len(heights) || selected-start <= end-selected)
		if preferBefore && used+beforeHeight <= capacity {
			start--
			used += beforeHeight
			continue
		}
		if end < len(heights) && used+afterHeight <= capacity {
			end++
			used += afterHeight
			continue
		}
		if start > 0 && used+beforeHeight <= capacity {
			start--
			used += beforeHeight
			continue
		}
		break
	}
	return start, end
}

func contentKind(content *github.Content) string {
	switch content.Kind {
	case "Issue":
		return "Issue"
	case "PullRequest":
		return "PR"
	case "DraftIssue":
		return "Draft"
	default:
		if content.Kind == "" {
			return "Item"
		}
		return content.Kind
	}
}
