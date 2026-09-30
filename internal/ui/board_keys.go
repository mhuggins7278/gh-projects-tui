package ui

import tea "charm.land/bubbletea/v2"

func (m *Model) handleBoardNavigationKey(key string) (tea.Cmd, bool) {
	switch key {
	case "H":
		return m.moveBoardLaneMutation(-1), true
	case "L":
		return m.moveBoardLaneMutation(1), true
	case "J":
		return m.reorderBoardItem(1), true
	case "K":
		return m.reorderBoardItem(-1), true
	case "h", "left":
		m.moveBoardLane(-1)
		return nil, true
	case "l", "right":
		m.moveBoardLane(1)
		return nil, true
	case "j", "down":
		m.moveBoardCard(1)
		return nil, true
	case "k", "up":
		m.moveBoardCard(-1)
		return nil, true
	case "enter":
		return m.openItemDetailCmd(), true
	}
	return nil, false
}
