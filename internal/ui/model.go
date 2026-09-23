package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/lipgloss"
	"github.com/mhuggins7278/gh-projects-tui/internal/config"
	"github.com/mhuggins7278/gh-projects-tui/internal/github"
)

type DiscoverySource interface {
	Discover(context.Context) (github.Discovery, error)
}

type DirectOwnerSource interface {
	ResolveOwner(context.Context, string) (github.Owner, []github.Project, error)
}

type ViewSource interface {
	OpenView(context.Context, github.Owner, int, int) (github.View, error)
}

type ViewsSource interface {
	ListViews(context.Context, github.Owner, int) ([]github.ViewSummary, error)
}

type ItemsSource interface {
	PageItems(context.Context, github.Owner, int, string, string) (github.ItemsPage, error)
}

type ItemDetailSource interface {
	LoadItemDetail(context.Context, github.Owner, int, string) (github.ItemDetail, error)
}

type ItemMutationSource interface {
	UpdateItemFieldValue(context.Context, github.ItemFieldValueUpdate) error
	ClearItemFieldValue(context.Context, github.ItemFieldValueClear) error
	UpdateItemPosition(context.Context, github.ItemPositionUpdate) error
}

type Selection struct {
	OwnerLogin    string
	ProjectNumber int
	ViewNumber    int
}

type screen int

const (
	screenLoading screen = iota
	screenOwnerPicker
	screenProjectPicker
	screenViewPicker
	screenBoard
)

// Model drives the picker and board. Unsupported mutation paths remain
// read-only until their reconciliation and safety rules are implemented.
type Model struct {
	source     DiscoverySource
	ctx        context.Context
	cancel     context.CancelFunc
	generation uint64
	selection  Selection
	loading    bool
	discovery  github.Discovery
	view       *github.View
	err        error

	host string

	screen               screen
	cursor               int
	filter               string
	filtering            bool
	showHelp             bool
	selectedOwner        *github.Owner
	selectedProject      *github.Project
	views                []github.ViewSummary
	viewsErr             error
	loadingViews         bool
	boardViewReasons     map[int]string
	pendingRejectedBoard *github.View
	viewPickerNotice     string
	loadingDetail        bool
	status               string
	items                []github.Item
	itemsLoading         bool
	itemsHasNext         bool
	itemsCursor          string
	itemsErr             error
	itemsLoadFrame       int
	itemsLanePending     int
	itemsLoadingLanes    map[string]bool
	itemsFailedLanes     map[string]bool
	boardLane            int
	boardCard            int
	boardFocusID         string
	width                int
	height               int
	detailVisible        bool
	detailLoading        bool
	detailItemID         string
	detailRequestID      uint64
	detail               *github.ItemDetail
	detailCache          map[string]github.ItemDetail
	detailErr            error
	detailOffset         int
	detailShowAll        bool
	mutationLoading      bool
	mutationSession      *boardMutationSession
	nextMutationSession  uint64
}

func NewModel(source DiscoverySource) Model {
	return NewModelWithSelection(source, Selection{})
}

func NewModelWithSelection(source DiscoverySource, selection Selection) Model {
	ctx, cancel := context.WithCancel(context.Background())
	return Model{
		source:      source,
		ctx:         ctx,
		cancel:      cancel,
		selection:   selection,
		loading:     true,
		screen:      screenLoading,
		host:        config.DefaultHost(),
		detailCache: make(map[string]github.ItemDetail),
	}
}

// NewModelWithHost supplies the host displayed in the TUI header.
func NewModelWithHost(source DiscoverySource, selection Selection, host string) Model {
	m := NewModelWithSelection(source, selection)
	if host != "" {
		m.host = host
	}
	return m
}

func (m Model) Init() tea.Cmd {
	if m.selection.OwnerLogin != "" {
		return resolveSelection(m.source, m.ctx, m.selection, m.generation)
	}
	return discover(m.source, m.ctx, m.generation)
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		return m.updateKey(msg)
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil
	case itemsLoadingTickMsg:
		if msg.generation != m.generation || !m.itemsLoading || m.screen != screenBoard {
			return m, nil
		}
		m.itemsLoadFrame = (m.itemsLoadFrame + 1) % len(itemsLoadingFrames)
		return m, itemsLoadingTick(m.generation)
	case discoveryMsg:
		return m.updateDiscovery(msg)
	case viewsMsg:
		if msg.generation != m.generation {
			return m, nil
		}
		m.loadingViews = false
		m.views = msg.views
		m.viewsErr = msg.err
		if msg.err != nil {
			m.err = msg.err
			m.status = fmt.Sprintf("View list failed: %v", msg.err)
			return m, nil
		}
		m.err = nil
		m.status = ""
		m.screen = screenViewPicker
		m.cursor = 0
		m.filter = ""
		m.filtering = false
		m.boardViewReasons = make(map[int]string)
		if m.pendingRejectedBoard != nil {
			if m.pendingRejectedBoard.Layout == github.BoardLayout {
				if compatibility := evaluateViewCompatibility(*m.pendingRejectedBoard); !compatibility.supported() {
					m.boardViewReasons[m.pendingRejectedBoard.Number] = compatibility.summary()
				}
			}
			m.pendingRejectedBoard = nil
		}
		return m, nil
	case viewDetailMsg:
		if msg.generation != m.generation {
			return m, nil
		}
		m.loadingDetail = false
		m.loading = false
		if msg.err != nil {
			m.err = msg.err
			m.status = fmt.Sprintf("View load failed: %v", msg.err)
			return m, nil
		}
		if msg.view == nil {
			m.status = "View load returned no view"
			return m, nil
		}
		if compatibility := evaluateViewCompatibility(*msg.view); !compatibility.supported() {
			if msg.view.Layout == github.BoardLayout {
				if m.boardViewReasons == nil {
					m.boardViewReasons = make(map[int]string)
				}
				m.boardViewReasons[msg.view.Number] = compatibility.summary()
			}
			m.rejectView(*msg.view, compatibility)
			return m, nil
		}
		m.view = msg.view
		m.err = nil
		m.status = ""
		m.screen = screenBoard
		m.cursor = 0
		return m, m.startItemsLoadWithSpinner()
	case itemsPageMsg:
		return m.updateItems(msg)
	case laneItemsMsg:
		return m.updateLaneItems(msg)
	case itemDetailMsg:
		if msg.generation != m.generation || m.screen != screenBoard || !m.detailVisible || msg.itemID != m.detailItemID || msg.requestID != m.detailRequestID {
			return m, nil
		}
		m.detailLoading = false
		m.detailErr = msg.err
		if msg.err == nil && msg.detail != nil {
			m.detail = msg.detail
			if m.detailCache == nil {
				m.detailCache = make(map[string]github.ItemDetail)
			}
			m.detailCache[msg.itemID] = *msg.detail
		}
		return m, nil
	case itemDetailDebounceMsg:
		if msg.generation != m.generation || m.screen != screenBoard || !m.detailVisible || msg.itemID != m.detailItemID || msg.requestID != m.detailRequestID {
			return m, nil
		}
		return m, m.loadItemDetailCmd(msg.itemID, msg.requestID)
	case boardMutationMsg:
		return m.updateBoardMutation(msg)
	case mutationReadbackMsg:
		return m.updateMutationReadback(msg)
	}
	return m, nil
}

func (m Model) updateDiscovery(msg discoveryMsg) (tea.Model, tea.Cmd) {
	if msg.generation != m.generation {
		return m, nil
	}
	m.loading = false
	m.discovery = msg.discovery
	m.view = msg.view
	m.err = msg.err
	if msg.err != nil && len(msg.discovery.Owners) == 0 {
		m.screen = screenOwnerPicker
		m.cursor = 0
		return m, nil
	}
	// Direct-selection path (explicit --owner/--project/--view).
	if m.selection.OwnerLogin != "" {
		if m.view != nil {
			if compatibility := evaluateViewCompatibility(*m.view); !compatibility.supported() {
				m.setDiscoverySelection()
				rejected := *m.view
				m.rejectView(rejected, compatibility)
				notice := m.status
				m.views = nil
				cmd := m.startViewsLoad()
				m.viewPickerNotice = notice
				m.pendingRejectedBoard = &rejected
				return m, cmd
			}
			m.enterBoardFromDiscovery()
			return m, m.startItemsLoadWithSpinner()
		}
		if m.err != nil {
			m.screen = screenOwnerPicker
			return m, nil
		}
		owner := findOwner(m.discovery, m.selection.OwnerLogin)
		if owner == nil {
			m.screen = screenOwnerPicker
			return m, nil
		}
		m.selectedOwner = owner
		if m.selection.ProjectNumber == 0 {
			m.screen = screenProjectPicker
			m.cursor = 0
			return m, nil
		}
		if project := findProject(m.discovery, owner.Login, m.selection.ProjectNumber); project != nil {
			m.selectedProject = project
			cmd := (&m).startViewsLoad()
			return m, cmd
		}
		m.screen = screenProjectPicker
		return m, nil
	}
	m.screen = screenOwnerPicker
	m.cursor = 0
	return m, nil
}

func (m *Model) enterBoardFromDiscovery() {
	m.screen = screenBoard
	m.cursor = 0
	m.setDiscoverySelection()
}

func (m *Model) setDiscoverySelection() {
	for _, owner := range m.discovery.Owners {
		if strings.EqualFold(owner.Login, m.selection.OwnerLogin) {
			ownerCopy := owner
			m.selectedOwner = &ownerCopy
			for _, project := range m.discovery.Projects[owner.Login] {
				if project.Number == m.selection.ProjectNumber {
					projectCopy := project
					m.selectedProject = &projectCopy
					break
				}
			}
			break
		}
	}
}

func (m *Model) rejectView(view github.View, compatibility viewCompatibility) {
	m.view = nil
	m.loading = false
	m.loadingDetail = false
	m.items = nil
	m.itemsLoading = false
	m.itemsHasNext = false
	m.itemsCursor = ""
	m.itemsErr = nil
	m.itemsLanePending = 0
	m.itemsLoadingLanes = nil
	m.itemsFailedLanes = nil
	m.detailVisible = false
	m.detailLoading = false
	m.detail = nil
	m.detailCache = nil
	m.detailErr = nil
	m.screen = screenViewPicker
	m.cursor = 0
	m.viewsErr = nil
	m.loadingViews = false
	m.status = unsupportedViewStatus(view, compatibility)
	m.viewPickerNotice = m.status

	for _, summary := range m.views {
		if summary.Number == view.Number {
			return
		}
	}
	m.views = []github.ViewSummary{{Number: view.Number, Name: view.Name, Layout: view.Layout}}
}

func (m *Model) startViewsLoad() tea.Cmd {
	if m.itemsLoading {
		m.cancelItemsLoad()
	} else {
		if m.cancel != nil {
			m.cancel()
		}
		m.ctx, m.cancel = context.WithCancel(context.Background())
		m.generation++
	}
	m.screen = screenViewPicker
	m.view = nil
	m.detailVisible = false
	m.detailLoading = false
	m.detail = nil
	m.detailCache = nil
	m.detailErr = nil
	m.items = nil
	m.itemsLoading = false
	m.itemsHasNext = false
	m.itemsCursor = ""
	m.itemsErr = nil
	m.itemsLanePending = 0
	m.itemsLoadingLanes = nil
	m.itemsFailedLanes = nil
	m.loadingViews = true
	m.views = nil
	m.viewsErr = nil
	m.boardViewReasons = nil
	m.pendingRejectedBoard = nil
	m.viewPickerNotice = ""
	m.cursor = 0
	m.filter = ""
	m.filtering = false
	owner := *m.selectedOwner
	project := m.selectedProject.Number
	generation := m.generation
	ctx := m.ctx
	source := m.source
	return func() tea.Msg {
		lister, ok := source.(ViewsSource)
		if !ok {
			return viewsMsg{err: fmt.Errorf("view listing is not supported by this client"), generation: generation}
		}
		if ctx == nil {
			ctx = context.Background()
		}
		views, err := lister.ListViews(ctx, owner, project)
		return viewsMsg{views: views, err: err, generation: generation}
	}
}

func (m *Model) loadViewsCmd() (tea.Model, tea.Cmd) {
	cmd := m.startViewsLoad()
	return *m, cmd
}

func (m *Model) openViewCmd(owner github.Owner, projectNumber, viewNumber int) tea.Cmd {
	generation := m.generation
	ctx := m.ctx
	source := m.source
	return func() tea.Msg {
		loader, ok := source.(ViewSource)
		if !ok {
			return viewDetailMsg{err: fmt.Errorf("view selection is not supported by this client"), generation: generation}
		}
		if ctx == nil {
			ctx = context.Background()
		}
		view, err := loader.OpenView(ctx, owner, projectNumber, viewNumber)
		if err != nil {
			return viewDetailMsg{err: err, generation: generation}
		}
		return viewDetailMsg{view: &view, generation: generation}
	}
}

func (m *Model) startItemsLoad() tea.Cmd {
	if m.selectedOwner == nil || m.selectedProject == nil || m.view == nil {
		return nil
	}
	if m.cancel != nil {
		m.cancel()
	}
	m.ctx, m.cancel = context.WithCancel(context.Background())
	m.generation++
	m.items = nil
	m.itemsLoading = true
	m.itemsHasNext = false
	m.itemsCursor = ""
	m.itemsErr = nil
	m.itemsLoadFrame = 0
	m.itemsLanePending = 0
	m.itemsLoadingLanes = nil
	m.itemsFailedLanes = nil
	m.boardLane = 0
	m.boardCard = 0
	m.boardFocusID = ""
	m.detailVisible = false
	m.detailLoading = false
	m.detailItemID = ""
	m.detail = nil
	m.detailCache = nil
	m.detailErr = nil
	return m.itemsPageCmd("", true)
}

func (m *Model) startItemsLoadWithSpinner() tea.Cmd {
	load := m.startItemsLoad()
	if load == nil {
		return nil
	}
	commands := []tea.Cmd{itemsLoadingTick(m.generation)}
	if requests, ok := parallelLaneItemsRequests(m.view); ok {
		m.itemsLanePending = len(requests)
		m.itemsLoadingLanes = make(map[string]bool, len(requests))
		m.itemsFailedLanes = make(map[string]bool)
		for _, request := range requests {
			m.itemsLoadingLanes[request.key] = true
			commands = append(commands, m.laneItemsCmd(request))
		}
	} else {
		commands = append(commands, load)
	}
	return tea.Batch(commands...)
}

func (m *Model) itemsPageCmd(after string, reset bool) tea.Cmd {
	if m.selectedOwner == nil || m.selectedProject == nil || m.view == nil {
		return nil
	}
	owner := *m.selectedOwner
	project := m.selectedProject.Number
	filter := strings.TrimSpace(m.view.Filter)
	generation := m.generation
	ctx := m.ctx
	source := m.source
	return func() tea.Msg {
		loader, ok := source.(ItemsSource)
		if !ok {
			return itemsPageMsg{after: after, reset: reset, err: fmt.Errorf("item loading is not supported by this client"), generation: generation}
		}
		if ctx == nil {
			ctx = context.Background()
		}
		page, err := loader.PageItems(ctx, owner, project, filter, after)
		return itemsPageMsg{page: page, after: after, reset: reset, err: err, generation: generation}
	}
}

func (m Model) updateItems(msg itemsPageMsg) (tea.Model, tea.Cmd) {
	if msg.generation != m.generation || m.screen != screenBoard {
		return m, nil
	}
	if msg.reset {
		m.items = nil
		m.boardLane = 0
		m.boardCard = 0
		m.boardFocusID = ""
	}
	if msg.err != nil {
		m.itemsLoading = false
		m.itemsErr = msg.err
		m.itemsHasNext = false
		return m, nil
	}
	m.items = append(m.items, msg.page.Items...)
	m.itemsHasNext = msg.page.HasNext
	m.itemsCursor = msg.page.EndCursor
	m.itemsErr = nil
	if msg.page.HasNext {
		m.itemsLoading = true
		m.clampBoardCursor()
		return m, m.itemsPageCmd(msg.page.EndCursor, false)
	}
	m.itemsLoading = false
	m.clampBoardCursor()
	return m, nil
}

func (m *Model) laneItemsCmd(request laneItemsRequest) tea.Cmd {
	owner := *m.selectedOwner
	project := m.selectedProject.Number
	generation := m.generation
	ctx := m.ctx
	source := m.source
	return func() tea.Msg {
		loader, ok := source.(ItemsSource)
		if !ok {
			return laneItemsMsg{request: request, err: fmt.Errorf("item loading is not supported by this client"), generation: generation}
		}
		if ctx == nil {
			ctx = context.Background()
		}
		items := make([]github.Item, 0)
		after := ""
		for {
			page, err := loader.PageItems(ctx, owner, project, request.filter, after)
			if err != nil {
				return laneItemsMsg{request: request, items: items, err: err, generation: generation}
			}
			items = append(items, page.Items...)
			if !page.HasNext {
				return laneItemsMsg{request: request, items: items, generation: generation}
			}
			after = page.EndCursor
		}
	}
}

func (m Model) updateLaneItems(msg laneItemsMsg) (tea.Model, tea.Cmd) {
	if msg.generation != m.generation || m.screen != screenBoard || !m.itemsLoadingLanes[msg.request.key] {
		return m, nil
	}
	m.items = appendUniqueItems(m.items, msg.items)
	delete(m.itemsLoadingLanes, msg.request.key)
	m.itemsLanePending--
	if msg.err != nil && m.itemsErr == nil {
		m.itemsErr = fmt.Errorf("%s: %w", msg.request.name, msg.err)
	}
	if msg.err != nil {
		m.itemsFailedLanes[msg.request.key] = true
	}
	if m.itemsLanePending > 0 {
		m.clampBoardCursor()
		return m, nil
	}
	m.itemsLoading = false
	m.itemsHasNext = false
	m.itemsLoadingLanes = nil
	m.clampBoardCursor()
	return m, nil
}

func (m *Model) cancelItemsLoad() {
	if !m.itemsLoading {
		return
	}
	if m.cancel != nil {
		m.cancel()
	}
	m.ctx, m.cancel = context.WithCancel(context.Background())
	m.generation++
	m.itemsLoading = false
	m.itemsLanePending = 0
	m.itemsLoadingLanes = nil
}

func appendUniqueItems(items, additions []github.Item) []github.Item {
	seen := make(map[string]bool, len(items)+len(additions))
	for _, item := range items {
		if item.ID != "" {
			seen[item.ID] = true
		}
	}
	for _, item := range additions {
		if item.ID != "" && seen[item.ID] {
			continue
		}
		items = append(items, item)
		if item.ID != "" {
			seen[item.ID] = true
		}
	}
	return items
}

func (m *Model) openItemDetailCmd() tea.Cmd {
	if m.detailLoading {
		return nil
	}
	item, ok := m.selectedBoardItem()
	if !ok || m.selectedOwner == nil || m.selectedProject == nil {
		return nil
	}
	m.detailVisible = true
	m.detailItemID = item.ID
	m.detailRequestID++
	m.boardFocusID = item.ID
	m.detail = nil
	m.detailErr = nil
	m.detailOffset = 0
	m.detailShowAll = false
	if cached, ok := m.detailCache[item.ID]; ok {
		m.detail = &cached
		m.detailLoading = false
		return nil
	}
	m.detailLoading = true
	requestID := m.detailRequestID
	generation := m.generation
	return tea.Tick(150*time.Millisecond, func(time.Time) tea.Msg {
		return itemDetailDebounceMsg{itemID: item.ID, requestID: requestID, generation: generation}
	})
}

func (m *Model) loadItemDetailCmd(itemID string, requestID uint64) tea.Cmd {
	owner := *m.selectedOwner
	project := m.selectedProject.Number
	generation := m.generation
	ctx := m.ctx
	source := m.source
	return func() tea.Msg {
		loader, ok := source.(ItemDetailSource)
		if !ok {
			return itemDetailMsg{itemID: itemID, requestID: requestID, err: fmt.Errorf("item detail is not supported by this client"), generation: generation}
		}
		if ctx == nil {
			ctx = context.Background()
		}
		detail, err := loader.LoadItemDetail(ctx, owner, project, itemID)
		return itemDetailMsg{itemID: itemID, requestID: requestID, detail: &detail, err: err, generation: generation}
	}
}

func (m *Model) clampBoardCursor() {
	lanes := m.boardLanes()
	if len(lanes) == 0 {
		m.boardLane = 0
		m.boardCard = 0
		return
	}
	if m.boardFocusID != "" {
		for laneIndex, lane := range lanes {
			for cardIndex, item := range lane.Items {
				if item.ID == m.boardFocusID {
					m.boardLane = laneIndex
					m.boardCard = cardIndex
					return
				}
			}
		}
	}
	if m.boardLane >= len(lanes) {
		m.boardLane = len(lanes) - 1
	}
	if m.boardLane < 0 {
		m.boardLane = 0
	}
	if len(lanes[m.boardLane].Items) == 0 {
		for laneIndex, lane := range lanes {
			if len(lane.Items) > 0 {
				m.boardLane = laneIndex
				m.boardCard = 0
				m.boardFocusID = lane.Items[0].ID
				return
			}
		}
		m.boardCard = 0
		return
	}
	if m.boardCard >= len(lanes[m.boardLane].Items) {
		m.boardCard = len(lanes[m.boardLane].Items) - 1
	}
	if m.boardCard < 0 {
		m.boardCard = 0
	}
	if item := lanes[m.boardLane].Items[m.boardCard]; item.ID != "" {
		m.boardFocusID = item.ID
	}
}

func (m *Model) rememberBoardFocus() {
	if item, ok := m.selectedBoardItem(); ok && item.ID != "" {
		m.boardFocusID = item.ID
	}
}

func (m Model) updateKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	// Global keys.
	switch key {
	case "q", "ctrl+c":
		if m.cancel != nil {
			m.cancel()
		}
		return m, tea.Quit
	case "?":
		if !m.filtering {
			m.showHelp = !m.showHelp
			return m, nil
		}
	}
	if key == "r" && m.mutationSession != nil && m.mutationSession.blocked {
		return m, m.retryMutationReadback()
	}

	if m.filtering {
		if m.screen == screenBoard {
			return m.updateBoardSearchKey(key)
		}
		switch key {
		case "enter":
			m.filtering = false
			m.cursor = 0
			return m, nil
		case "esc":
			m.filtering = false
			m.filter = ""
			m.cursor = 0
			return m, nil
		case "backspace":
			if len(m.filter) > 0 {
				m.filter = m.filter[:len(m.filter)-1]
				m.cursor = 0
			}
			return m, nil
		case "ctrl+u":
			m.filter = ""
			m.cursor = 0
			return m, nil
		default:
			// Append printable single characters.
			if len(key) == 1 {
				m.filter += key
				m.cursor = 0
				return m, nil
			}
			// Multi-char keys (up/down) still navigate while filtering.
			switch key {
			case "up", "k":
				m.moveCursor(-1)
				return m, nil
			case "down", "j":
				m.moveCursor(1)
				return m, nil
			}
			return m, nil
		}
	}

	if m.screen == screenBoard {
		if m.detailVisible {
			switch key {
			case "esc":
				m.detailVisible = false
				m.detailLoading = false
				m.detailRequestID++
				m.detail = nil
				m.detailErr = nil
				return m, nil
			case "j", "down", "J":
				m.moveDetail(1)
				return m, nil
			case "k", "up", "K":
				m.moveDetail(-1)
				return m, nil
			case "g":
				m.detailOffset = 0
				return m, nil
			case "f", "F":
				m.detailShowAll = !m.detailShowAll
				m.detailOffset = 0
				return m, nil
			}
			return m, nil
		}
		switch key {
		case "H":
			return m, m.moveBoardLaneMutation(-1)
		case "L":
			return m, m.moveBoardLaneMutation(1)
		case "J":
			return m, m.reorderBoardItem(1)
		case "K":
			return m, m.reorderBoardItem(-1)
		case "h", "left":
			m.moveBoardLane(-1)
			return m, nil
		case "l", "right":
			m.moveBoardLane(1)
			return m, nil
		case "j", "down":
			m.moveBoardCard(1)
			return m, nil
		case "k", "up":
			m.moveBoardCard(-1)
			return m, nil
		case "enter":
			return m, m.openItemDetailCmd()
		}
	}

	switch key {
	case "/":
		m.filtering = true
		if m.screen == screenBoard {
			m.filter = ""
			m.clampBoardCursor()
		}
		return m, nil
	case "esc":
		return m, m.goBack()
	case "backspace":
		return m, m.goBack()
	case "up", "k":
		m.moveCursor(-1)
		return m, nil
	case "down", "j":
		m.moveCursor(1)
		return m, nil
	case "enter":
		return m, m.selectCurrent()
	case "p":
		m.cancelItemsLoad()
		if m.selectedOwner != nil && m.screen != screenProjectPicker {
			m.screen = screenProjectPicker
			m.cursor = 0
			m.filter = ""
			m.view = nil
			m.detailVisible = false
			m.detailLoading = false
			m.detail = nil
			m.detailErr = nil
		} else if m.screen == screenBoard || m.screen == screenViewPicker {
			m.screen = screenProjectPicker
			m.cursor = 0
			m.filter = ""
			m.view = nil
			m.detailVisible = false
			m.detailLoading = false
			m.detail = nil
			m.detailErr = nil
		}
		return m, nil
	case "v":
		if m.selectedProject != nil && m.selectedOwner != nil {
			cmd := (&m).startViewsLoad()
			return m, cmd
		}
		return m, nil
	case "r":
		return m, m.refresh()
	case "o":
		m.status = m.browserURL()
		return m, nil
	}
	// Board navigation reserves h/l for lanes; ignore in pickers.
	return m, nil
}

func (m *Model) updateBoardSearchKey(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "enter":
		m.filtering = false
		m.clampBoardCursor()
	case "esc":
		m.filtering = false
		m.filter = ""
		m.clampBoardCursor()
	case "backspace":
		runes := []rune(m.filter)
		if len(runes) > 0 {
			m.filter = string(runes[:len(runes)-1])
			m.clampBoardCursor()
		}
	case "ctrl+u":
		m.filter = ""
		m.clampBoardCursor()
	default:
		if len([]rune(key)) == 1 {
			m.filter += key
			m.clampBoardCursor()
		}
	}
	return *m, nil
}

func (m *Model) moveCursor(delta int) {
	n := m.currentListLen()
	if n == 0 {
		m.cursor = 0
		return
	}
	m.cursor += delta
	if m.cursor < 0 {
		m.cursor = 0
	}
	if m.cursor >= n {
		m.cursor = n - 1
	}
}

func (m *Model) goBack() tea.Cmd {
	m.filter = ""
	m.filtering = false
	m.cursor = 0
	m.status = ""
	switch m.screen {
	case screenViewPicker:
		m.screen = screenProjectPicker
		m.views = nil
	case screenProjectPicker:
		m.screen = screenOwnerPicker
		m.selectedOwner = nil
		m.selectedProject = nil
	case screenBoard:
		m.cancelItemsLoad()
		if m.selectedProject != nil {
			m.screen = screenViewPicker
			m.view = nil
			m.detailVisible = false
			m.detailLoading = false
			m.detail = nil
			m.detailErr = nil
			if len(m.views) == 0 {
				return m.loadViewsCmdFunc()
			}
		}
	}
	return nil
}

func (m *Model) loadViewsCmdFunc() tea.Cmd {
	return m.startViewsLoad()
}

func (m *Model) selectCurrent() tea.Cmd {
	m.status = ""
	switch m.screen {
	case screenOwnerPicker:
		owners := m.filteredOwners()
		if len(owners) == 0 {
			return nil
		}
		selected := owners[m.cursor]
		m.selectedOwner = &selected
		m.selectedProject = nil
		m.screen = screenProjectPicker
		m.cursor = 0
		m.filter = ""
		return nil
	case screenProjectPicker:
		projects := m.filteredProjects()
		if len(projects) == 0 || m.selectedOwner == nil {
			return nil
		}
		selected := projects[m.cursor]
		m.selectedProject = &selected
		return m.loadViewsCmdFunc()
	case screenViewPicker:
		views := m.filteredViews()
		if len(views) == 0 || m.selectedOwner == nil || m.selectedProject == nil {
			return nil
		}
		selected := views[m.cursor]
		summaryView := github.View{Number: selected.Number, Name: selected.Name, Layout: selected.Layout}
		if compatibility := evaluateViewCompatibility(summaryView); !compatibility.supported() {
			m.status = unsupportedViewStatus(summaryView, compatibility)
			return nil
		}
		m.loadingDetail = true
		m.loading = false
		m.status = fmt.Sprintf("Loading view #%d...", selected.Number)
		owner := *m.selectedOwner
		project := m.selectedProject.Number
		return m.openViewCmd(owner, project, selected.Number)
	case screenBoard:
		return nil
	}
	return nil
}

func (m *Model) refresh() tea.Cmd {
	if m.cancel != nil {
		m.cancel()
	}
	m.ctx, m.cancel = context.WithCancel(context.Background())
	m.generation++
	m.itemsLoading = false
	m.itemsLanePending = 0
	m.itemsLoadingLanes = nil
	m.itemsFailedLanes = nil
	m.loading = m.screen == screenLoading
	m.err = nil
	m.viewsErr = nil
	m.status = ""
	switch m.screen {
	case screenViewPicker:
		if m.selectedOwner != nil && m.selectedProject != nil {
			m.loadingViews = true
			owner := *m.selectedOwner
			project := m.selectedProject.Number
			generation := m.generation
			ctx := m.ctx
			source := m.source
			return func() tea.Msg {
				lister, ok := source.(ViewsSource)
				if !ok {
					return viewsMsg{err: fmt.Errorf("view listing is not supported by this client"), generation: generation}
				}
				views, err := lister.ListViews(ctx, owner, project)
				return viewsMsg{views: views, err: err, generation: generation}
			}
		}
		m.loading = true
		m.screen = screenLoading
		if m.selection.OwnerLogin != "" {
			return resolveSelection(m.source, m.ctx, m.selection, m.generation)
		}
		return discover(m.source, m.ctx, m.generation)
	case screenBoard:
		if m.view != nil && m.selectedOwner != nil && m.selectedProject != nil {
			m.loadingDetail = true
			owner := *m.selectedOwner
			return m.openViewCmd(owner, m.selectedProject.Number, m.view.Number)
		}
		m.loading = true
		m.screen = screenLoading
		return discover(m.source, m.ctx, m.generation)
	default:
		m.loading = true
		m.screen = screenLoading
		m.view = nil
		if m.selection.OwnerLogin != "" {
			return resolveSelection(m.source, m.ctx, m.selection, m.generation)
		}
		return discover(m.source, m.ctx, m.generation)
	}
}

func (m Model) browserURL() string {
	if m.selectedProject != nil && m.selectedProject.URL != "" {
		return "Open in browser: " + m.selectedProject.URL
	}
	if m.selectedOwner != nil {
		return fmt.Sprintf("No browser URL for %s yet.", m.selectedOwner.Login)
	}
	return "Nothing to open yet."
}

func findOwner(discovery github.Discovery, login string) *github.Owner {
	for _, owner := range discovery.Owners {
		if strings.EqualFold(owner.Login, login) {
			ownerCopy := owner
			return &ownerCopy
		}
	}
	return nil
}

func findProject(discovery github.Discovery, ownerLogin string, number int) *github.Project {
	for _, project := range discovery.Projects[ownerLogin] {
		if project.Number == number {
			projectCopy := project
			return &projectCopy
		}
	}
	return nil
}

func (m Model) currentListLen() int {
	switch m.screen {
	case screenOwnerPicker:
		return len(m.filteredOwners())
	case screenProjectPicker:
		return len(m.filteredProjects())
	case screenViewPicker:
		return len(m.filteredViews())
	default:
		return 0
	}
}

func matchesFilter(haystack, needle string) bool {
	if needle == "" {
		return true
	}
	return strings.Contains(strings.ToLower(haystack), strings.ToLower(needle))
}

func (m Model) filteredOwners() []github.Owner {
	var out []github.Owner
	for _, owner := range m.discovery.Owners {
		if matchesFilter(owner.Login, m.filter) {
			out = append(out, owner)
		}
	}
	return out
}

func (m Model) filteredProjectsWith(ownerLogin, filter string) []github.Project {
	var out []github.Project
	for _, project := range m.discovery.Projects[ownerLogin] {
		if matchesFilter(project.Title, filter) || matchesFilter(fmt.Sprintf("%d", project.Number), filter) {
			out = append(out, project)
		}
	}
	return out
}

func (m Model) filteredProjects() []github.Project {
	if m.selectedOwner == nil {
		return nil
	}
	return m.filteredProjectsWith(m.selectedOwner.Login, m.filter)
}

func (m Model) filteredViews() []github.ViewSummary {
	var out []github.ViewSummary
	for _, view := range m.views {
		if matchesFilter(view.Name, m.filter) || matchesFilter(fmt.Sprintf("%d", view.Number), m.filter) || matchesFilter(string(view.Layout), m.filter) {
			out = append(out, view)
		}
	}
	return out
}

func (m Model) View() tea.View {
	content := lipgloss.JoinVertical(lipgloss.Left, m.renderHeader(), m.renderScreen(), m.renderFooter())
	view := tea.NewView(content)
	view.AltScreen = true
	return view
}

func (m Model) breadcrumb() string {
	parts := []string{}
	if m.selectedOwner != nil {
		parts = append(parts, m.selectedOwner.Login)
	}
	if m.selectedProject != nil {
		parts = append(parts, fmt.Sprintf("#%d %s", m.selectedProject.Number, m.selectedProject.Title))
	}
	if m.view != nil {
		parts = append(parts, fmt.Sprintf("view #%d %s", m.view.Number, m.view.Name))
	} else if m.screen == screenViewPicker && m.selectedProject != nil {
		parts = append(parts, "views")
	}
	if len(parts) == 0 {
		return "owners"
	}
	return strings.Join(parts, " / ")
}

func (m Model) renderOwnerPicker(b *strings.Builder) {
	if m.err != nil && len(m.discovery.Owners) == 0 {
		b.WriteString(errorStyle.Render("Discovery failed: "+m.err.Error()) + "\n")
		b.WriteString(mutedStyle.Render("Press r to retry, q to quit.") + "\n")
		return
	}
	b.WriteString(m.pickerHeading("Owners", "select an owner"))
	b.WriteString("\n")
	b.WriteString(m.filterLine())
	owners := m.filteredOwners()
	if len(owners) == 0 {
		b.WriteString(mutedStyle.Render("No matching owners.") + "\n")
	} else {
		for i, owner := range owners {
			label := fmt.Sprintf("%-24s %d projects", owner.Login, len(m.discovery.Projects[owner.Login]))
			b.WriteString(m.pickerRow(i == m.cursor, label) + "\n")
		}
	}
	m.renderDiscoveryErrors(b)
}

func (m Model) renderProjectPicker(b *strings.Builder) {
	if m.selectedOwner == nil {
		b.WriteString(mutedStyle.Render("No owner selected. Press esc to go back.") + "\n")
		return
	}
	if m.err != nil {
		b.WriteString(errorStyle.Render("Error: "+m.err.Error()) + "\n")
	}
	b.WriteString(m.pickerHeading("Projects", "for "+m.selectedOwner.Login))
	b.WriteString("\n")
	b.WriteString(m.filterLine())
	projects := m.filteredProjects()
	if len(projects) == 0 {
		b.WriteString(mutedStyle.Render("No matching projects.") + "\n")
	} else {
		for i, project := range projects {
			title := project.Title
			if title == "" {
				title = "(untitled)"
			}
			label := fmt.Sprintf("#%-4d %s", project.Number, title)
			b.WriteString(m.pickerRow(i == m.cursor, label) + "\n")
		}
	}
}

func (m Model) renderViewPicker(b *strings.Builder) {
	if m.selectedOwner == nil || m.selectedProject == nil {
		b.WriteString(mutedStyle.Render("No project selected. Press esc to go back.") + "\n")
		return
	}
	if m.loadingViews {
		b.WriteString(sectionStyle.Render(fmt.Sprintf("Loading views for #%d...", m.selectedProject.Number)) + "\n")
		return
	}
	if m.viewsErr != nil {
		b.WriteString(errorStyle.Render("View list failed: "+m.viewsErr.Error()) + "\n")
		b.WriteString(mutedStyle.Render("Press r to retry, esc to go back.") + "\n")
		return
	}
	b.WriteString(m.pickerHeading("Saved views", fmt.Sprintf("#%d %s", m.selectedProject.Number, m.selectedProject.Title)))
	b.WriteString("\n")
	if m.viewPickerNotice != "" {
		b.WriteString(statusStyle.Render(m.viewPickerNotice) + "\n")
	}
	b.WriteString(m.filterLine())
	views := m.filteredViews()
	if len(views) == 0 {
		b.WriteString(mutedStyle.Render("No matching saved views.") + "\n")
		return
	}
	for i, view := range views {
		label := fmt.Sprintf("#%-4d %-24s", view.Number, view.Name)
		if reason := m.boardViewReasons[view.Number]; reason != "" {
			label += "  (unsupported: " + reason + ")"
		}
		b.WriteString(m.pickerRow(i == m.cursor, label+" "+layoutBadge(string(view.Layout))) + "\n")
	}
}

func (m Model) renderDiscoveryErrors(b *strings.Builder) {
	if len(m.discovery.OwnerErrors) > 0 {
		b.WriteString("\n" + errorStyle.Render("Owner errors") + "\n")
		for _, failure := range m.discovery.OwnerErrors {
			b.WriteString(mutedStyle.Render(fmt.Sprintf("  %s: %v", failure.Owner.Login, failure.Err)) + "\n")
		}
	}
	if m.discovery.MembershipError != nil {
		b.WriteString("\n" + statusStyle.Render(fmt.Sprintf("Organization discovery: %v", m.discovery.MembershipError)) + "\n")
	}
	if m.err != nil && len(m.discovery.Owners) > 0 {
		b.WriteString("\n" + statusStyle.Render(fmt.Sprintf("Warning: %v", m.err)) + "\n")
	}
}

func (m Model) filterLine() string {
	if m.filtering {
		return statusStyle.Render(fmt.Sprintf("Filter: %s_", m.filter)) + "\n"
	}
	if m.filter != "" {
		return mutedStyle.Render(fmt.Sprintf("Filter: %s", m.filter)) + "\n"
	}
	return ""
}

func (m Model) footerHints() string {
	if m.screen == screenBoard && m.detailVisible {
		return "j/k scroll detail · f all fields · esc close · q quit"
	}
	switch m.screen {
	case screenOwnerPicker:
		return "j/k move · enter select · / filter · r refresh · ? help · q quit"
	case screenProjectPicker:
		return "j/k move · enter select · / filter · esc owners · r refresh · ? help · q quit"
	case screenViewPicker:
		return "j/k move · enter open · / filter · esc projects · p projects · r refresh · ? help · q quit"
	case screenBoard:
		return "h/l lanes · j/k cards · H/L move · J/K reorder · / search · v views · p projects · r refresh · o browser · ? help · q quit"
	default:
		return "Loading... q to quit"
	}
}

func (m Model) helpText() string {
	return "Keys: j/k or up/down move · enter select · / filter/search (enter done, esc clear) ·\n" +
		"esc/backspace back · p projects · v reload views · r refresh · o browser URL ·\n" +
		"? toggle help · q quit.\n" +
		"On the board, h/l changes lanes, j/k changes cards, / searches loaded cards, and enter opens detail.\n" +
		"H/L moves cards and J/K reorders cards when the view is writable and fully loaded.\n" +
		"In detail, j/k scrolls, f toggles all project fields, and esc returns to the board."
}

type discoveryMsg struct {
	discovery  github.Discovery
	view       *github.View
	err        error
	generation uint64
}

type viewsMsg struct {
	views      []github.ViewSummary
	err        error
	generation uint64
}

type viewDetailMsg struct {
	view       *github.View
	err        error
	generation uint64
}

type itemsPageMsg struct {
	page       github.ItemsPage
	after      string
	reset      bool
	err        error
	generation uint64
}

type itemsLoadingTickMsg struct {
	generation uint64
}

type laneItemsMsg struct {
	request    laneItemsRequest
	items      []github.Item
	err        error
	generation uint64
}

type itemDetailMsg struct {
	itemID     string
	requestID  uint64
	detail     *github.ItemDetail
	err        error
	generation uint64
}

type itemDetailDebounceMsg struct {
	itemID     string
	requestID  uint64
	generation uint64
}

func itemsLoadingTick(generation uint64) tea.Cmd {
	return tea.Tick(100*time.Millisecond, func(time.Time) tea.Msg {
		return itemsLoadingTickMsg{generation: generation}
	})
}

func discover(source DiscoverySource, ctx context.Context, generation uint64) tea.Cmd {
	return func() tea.Msg {
		if source == nil {
			return discoveryMsg{err: fmt.Errorf("GitHub client is not configured"), generation: generation}
		}
		if ctx == nil {
			ctx = context.Background()
		}
		discovery, err := source.Discover(ctx)
		return discoveryMsg{discovery: discovery, err: err, generation: generation}
	}
}

func resolveSelection(source DiscoverySource, ctx context.Context, selection Selection, generation uint64) tea.Cmd {
	return func() tea.Msg {
		resolver, ok := source.(DirectOwnerSource)
		if !ok {
			return discoveryMsg{err: fmt.Errorf("direct owner selection is not supported by this client"), generation: generation}
		}
		if ctx == nil {
			ctx = context.Background()
		}
		owner, projects, err := resolver.ResolveOwner(ctx, selection.OwnerLogin)
		if err != nil {
			return discoveryMsg{err: err, generation: generation}
		}
		discovery := github.Discovery{
			Viewer:   owner.Login,
			Owners:   []github.Owner{owner},
			Projects: map[string][]github.Project{owner.Login: projects},
		}
		if selection.ProjectNumber == 0 {
			return discoveryMsg{discovery: discovery, generation: generation}
		}
		projectFound := false
		for _, project := range projects {
			if project.Number == selection.ProjectNumber {
				projectFound = true
				break
			}
		}
		if !projectFound {
			return discoveryMsg{discovery: discovery, err: fmt.Errorf("project %d was not found for owner %q", selection.ProjectNumber, selection.OwnerLogin), generation: generation}
		}
		if selection.ViewNumber == 0 {
			return discoveryMsg{discovery: discovery, generation: generation}
		}
		loader, ok := source.(ViewSource)
		if !ok {
			return discoveryMsg{discovery: discovery, err: fmt.Errorf("view selection is not supported by this client"), generation: generation}
		}
		view, err := loader.OpenView(ctx, owner, selection.ProjectNumber, selection.ViewNumber)
		if err != nil {
			return discoveryMsg{discovery: discovery, err: err, generation: generation}
		}
		return discoveryMsg{discovery: discovery, view: &view, generation: generation}
	}
}
