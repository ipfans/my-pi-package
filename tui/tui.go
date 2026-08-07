// Package tui provides bubbletea/v2 interactive flows for my-pi-package.
package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/ipfans/my-pi-package/catalog"
)

// Phase is the interactive wizard step.
type Phase int

const (
	PhaseLazyOrPick Phase = iota
	PhasePicker
	PhaseDone
	PhaseAbort
)

// Result is what the TUI returns to the installer.
type Result struct {
	Aborted  bool
	Selected map[string]struct{}
}

type item struct {
	id       string
	label    string
	category string
	isGroup  bool
}

type model struct {
	cat      *catalog.Catalog
	phase    Phase
	cursor   int
	selected map[string]struct{}
	items    []item // flat list for picker (groups + packages)
	choice   int    // 0 = lazy, 1 = pick
	width    int
	result   Result
}

// RunInstallPicker opens the lazy-or-pick + multiselect flow.
// initial is the pre-selected id set (usually full catalog).
func RunInstallPicker(cat *catalog.Catalog, initial map[string]struct{}) (Result, error) {
	sel := make(map[string]struct{}, len(initial))
	for id := range initial {
		sel[id] = struct{}{}
	}
	m := model{
		cat:      cat,
		phase:    PhaseLazyOrPick,
		selected: sel,
		choice:   0,
		width:    80,
	}
	m.items = buildItems(cat)
	p := tea.NewProgram(m)
	final, err := p.Run()
	if err != nil {
		return Result{Aborted: true}, err
	}
	fm, ok := final.(model)
	if !ok {
		return Result{Aborted: true}, fmt.Errorf("unexpected model type")
	}
	return fm.result, nil
}

func buildItems(cat *catalog.Catalog) []item {
	var items []item
	for _, catName := range cat.Categories {
		items = append(items, item{id: catName, label: catName, category: catName, isGroup: true})
		for _, p := range cat.Packages {
			if p.Category != catName {
				continue
			}
			items = append(items, item{
				id:       p.ID,
				label:    fmt.Sprintf("%-18s %s", p.ID, p.Description),
				category: catName,
			})
		}
	}
	return items
}

func (m model) Init() tea.Cmd { return nil }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		return m, nil
	case tea.KeyPressMsg:
		switch msg.String() {
		case "ctrl+c", "q", "esc":
			m.phase = PhaseAbort
			m.result = Result{Aborted: true}
			return m, tea.Quit
		}
		switch m.phase {
		case PhaseLazyOrPick:
			return m.updateLazyOrPick(msg)
		case PhasePicker:
			return m.updatePicker(msg)
		}
	}
	return m, nil
}

func (m model) updateLazyOrPick(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		if m.choice > 0 {
			m.choice--
		}
	case "down", "j":
		if m.choice < 1 {
			m.choice++
		}
	case "enter":
		if m.choice == 0 {
			// install everything — keep initial selection
			m.phase = PhaseDone
			m.result = Result{Selected: m.selected}
			return m, tea.Quit
		}
		m.phase = PhasePicker
		m.cursor = 0
	}
	return m, nil
}

func (m model) updatePicker(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		if m.cursor < len(m.items)-1 {
			m.cursor++
		}
	case "space", "x":
		it := m.items[m.cursor]
		if it.isGroup {
			m.toggleCategory(it.category)
		} else {
			m.toggleID(it.id)
		}
	case "a":
		m.toggleAll()
	case "enter":
		m.phase = PhaseDone
		m.result = Result{Selected: m.selected}
		return m, tea.Quit
	}
	return m, nil
}

func (m *model) toggleID(id string) {
	if _, ok := m.selected[id]; ok {
		delete(m.selected, id)
	} else {
		m.selected[id] = struct{}{}
	}
}

func (m *model) toggleCategory(category string) {
	allOn := true
	for _, p := range m.cat.Packages {
		if p.Category != category {
			continue
		}
		if _, ok := m.selected[p.ID]; !ok {
			allOn = false
			break
		}
	}
	for _, p := range m.cat.Packages {
		if p.Category != category {
			continue
		}
		if allOn {
			delete(m.selected, p.ID)
		} else {
			m.selected[p.ID] = struct{}{}
		}
	}
}

func (m *model) toggleAll() {
	if len(m.selected) == len(m.cat.Packages) {
		m.selected = map[string]struct{}{}
		return
	}
	m.selected = map[string]struct{}{}
	for _, p := range m.cat.Packages {
		m.selected[p.ID] = struct{}{}
	}
}

func (m model) View() tea.View {
	var b strings.Builder
	b.WriteString(logo())
	b.WriteString("\n  my-pi-package — curated Pi setup\n\n")

	switch m.phase {
	case PhaseLazyOrPick:
		b.WriteString(fmt.Sprintf("  Install all %d Pi packages the lazy way, or pick them yourself?\n\n", len(m.cat.Packages)))
		opts := []string{
			fmt.Sprintf("Install everything  (all %d packages)", len(m.cat.Packages)),
			"Pick packages        (open a checklist)",
		}
		for i, opt := range opts {
			cursor := "  "
			if i == m.choice {
				cursor = "▸ "
			}
			b.WriteString("  " + cursor + opt + "\n")
		}
		b.WriteString("\n  ↑/↓ move · enter confirm · q quit\n")
	case PhasePicker:
		b.WriteString("  Pick packages to install\n\n")
		for i, it := range m.items {
			cursor := "  "
			if i == m.cursor {
				cursor = "▸ "
			}
			if it.isGroup {
				on := m.categoryAllSelected(it.category)
				mark := " "
				if on {
					mark = "x"
				}
				b.WriteString(fmt.Sprintf("  %s[%s] %s\n", cursor, mark, strings.ToUpper(it.label)))
				continue
			}
			mark := " "
			if _, ok := m.selected[it.id]; ok {
				mark = "x"
			}
			b.WriteString(fmt.Sprintf("  %s[%s] %s\n", cursor, mark, it.label))
		}
		b.WriteString(fmt.Sprintf("\n  selected %d/%d · space toggle · a all · enter confirm · q quit\n", len(m.selected), len(m.cat.Packages)))
	default:
		b.WriteString("  Done.\n")
	}
	return tea.NewView(b.String())
}

func (m model) categoryAllSelected(category string) bool {
	for _, p := range m.cat.Packages {
		if p.Category != category {
			continue
		}
		if _, ok := m.selected[p.ID]; !ok {
			return false
		}
	}
	return true
}

func logo() string {
	return `
                 z Z z
                z Z
               z
        ____
       |  _ \(_)
       | |_) | |
       |  __/| |
       |_|   |_|
`
}

// IsInteractive reports whether both stdin and stdout are terminals.
func IsInteractive() bool {
	return isTerminal(0) && isTerminal(1)
}
