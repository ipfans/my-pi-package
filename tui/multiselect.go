package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
)

// PickItem is one row in a multi-select list.
type PickItem struct {
	ID          string
	Title       string
	Description string
}

// MultiSelectOpts configures RunMultiSelect.
type MultiSelectOpts struct {
	// DefaultAll pre-selects every item when true.
	DefaultAll bool
	// MinSelected is the minimum number of items required on confirm (default 1).
	MinSelected int
}

// PickResult is the outcome of a multi-select session.
type PickResult struct {
	Aborted  bool
	Selected []string
}

type multiModel struct {
	title    string
	items    []PickItem
	cursor   int
	selected map[string]struct{}
	minSel   int
	width    int
	errMsg   string
	result   PickResult
	done     bool
}

// RunMultiSelect opens a checklist TUI. Caller should only invoke when IsInteractive().
func RunMultiSelect(title string, items []PickItem, opts MultiSelectOpts) (PickResult, error) {
	if len(items) == 0 {
		return PickResult{}, fmt.Errorf("no items to select")
	}
	minSel := opts.MinSelected
	if minSel <= 0 {
		minSel = 1
	}
	sel := make(map[string]struct{})
	if opts.DefaultAll {
		for _, it := range items {
			sel[it.ID] = struct{}{}
		}
	}
	m := multiModel{
		title:    title,
		items:    items,
		selected: sel,
		minSel:   minSel,
		width:    80,
	}
	p := tea.NewProgram(m)
	final, err := p.Run()
	if err != nil {
		return PickResult{Aborted: true}, err
	}
	fm, ok := final.(multiModel)
	if !ok {
		return PickResult{Aborted: true}, fmt.Errorf("unexpected model type")
	}
	return fm.result, nil
}

func (m multiModel) Init() tea.Cmd { return nil }

func (m multiModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		return m, nil
	case tea.KeyPressMsg:
		m.errMsg = ""
		switch msg.String() {
		case "ctrl+c", "q", "esc":
			m.result = PickResult{Aborted: true}
			m.done = true
			return m, tea.Quit
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.items)-1 {
				m.cursor++
			}
		case "space", "x":
			id := m.items[m.cursor].ID
			if _, ok := m.selected[id]; ok {
				delete(m.selected, id)
			} else {
				m.selected[id] = struct{}{}
			}
		case "a":
			if len(m.selected) == len(m.items) {
				m.selected = map[string]struct{}{}
			} else {
				m.selected = map[string]struct{}{}
				for _, it := range m.items {
					m.selected[it.ID] = struct{}{}
				}
			}
		case "n":
			m.selected = map[string]struct{}{}
		case "enter":
			if len(m.selected) < m.minSel {
				m.errMsg = fmt.Sprintf("select at least %d item(s)", m.minSel)
				return m, nil
			}
			// preserve list order
			var out []string
			for _, it := range m.items {
				if _, ok := m.selected[it.ID]; ok {
					out = append(out, it.ID)
				}
			}
			m.result = PickResult{Selected: out}
			m.done = true
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m multiModel) View() tea.View {
	var b strings.Builder
	b.WriteString("\n  " + m.title + "\n\n")
	for i, it := range m.items {
		cursor := "  "
		if i == m.cursor {
			cursor = "▸ "
		}
		mark := " "
		if _, ok := m.selected[it.ID]; ok {
			mark = "x"
		}
		line := it.Title
		if it.Description != "" {
			line = fmt.Sprintf("%-16s %s", it.Title, it.Description)
		}
		b.WriteString(fmt.Sprintf("  %s[%s] %s\n", cursor, mark, line))
	}
	b.WriteString(fmt.Sprintf("\n  selected %d/%d · space toggle · a all · n none · enter confirm · q quit\n",
		len(m.selected), len(m.items)))
	if m.errMsg != "" {
		b.WriteString("  ! " + m.errMsg + "\n")
	}
	return tea.NewView(b.String())
}
