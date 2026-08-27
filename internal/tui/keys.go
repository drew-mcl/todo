package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/drew-mcl/todo/internal/store"
)

// The keys, and what they do, in one table.
//
// The help screen is drawn from this, so a key cannot be added without being
// documented and cannot be documented without existing. They were two lists
// before, and had already begun to disagree.

type binding struct {
	keys []string // what to press; the first is the one shown
	help string   // what it does, or "" to leave it out of the help
	run  func(*Model) tea.Cmd
}

// label renders a binding's keys for the help screen.
func (b binding) label() string {
	out := b.keys[0]
	for _, k := range b.keys[1:] {
		out += " / " + k
	}
	return out
}

func view(v store.View) func(*Model) tea.Cmd {
	return func(m *Model) tea.Cmd {
		m.mode, m.view, m.cursor = modeList, v, 0
		return m.reload
	}
}

// listBindings apply on the list. The chords -- g and dd -- are handled before
// this table is consulted, because a prefix is not a key.
var listBindings = []binding{
	{keys: []string{"n"}, help: "capture", run: (*Model).openCapture},
	{keys: []string{"j", "down"}, help: "move down", run: func(m *Model) tea.Cmd {
		if m.cursor < len(m.flat)-1 {
			m.cursor++
		}
		return nil
	}},
	{keys: []string{"k", "up"}, help: "move up", run: func(m *Model) tea.Cmd {
		if m.cursor > 0 {
			m.cursor--
		}
		return nil
	}},
	{keys: []string{"G"}, help: "last", run: func(m *Model) tea.Cmd {
		m.cursor = max(0, len(m.flat)-1)
		return nil
	}},
	{keys: []string{"x", " "}, help: "complete", run: (*Model).toggleCurrent},
	{keys: []string{"e"}, help: "edit the line", run: (*Model).openEdit},
	{keys: []string{"u"}, help: "take back the last capture", run: (*Model).undoLast},
	{keys: []string{"/"}, help: "search", run: func(m *Model) tea.Cmd {
		m.onSearch = true
		m.search.Focus()
		return nil
	}},
	{keys: []string{"t"}, help: "today", run: view(store.ViewToday)},
	{keys: []string{"w"}, help: "plan the week", run: (*Model).openWeek},
	{keys: []string{"a"}, help: "everything open", run: view(store.ViewAll)},
	{keys: []string{"l"}, help: "logbook", run: view(store.ViewLogbook)},
	{keys: []string{"?"}, help: "these keys", run: func(m *Model) tea.Cmd {
		m.mode = modeHelp
		return nil
	}},
	{keys: []string{"q", "ctrl+c"}, help: "quit", run: func(*Model) tea.Cmd { return tea.Quit }},
}

// chordBindings are the two-key sequences, listed for the help screen. They are
// dispatched by hand because a prefix has to be remembered between keystrokes.
var chordBindings = []binding{
	{keys: []string{"g g"}, help: "first"},
	{keys: []string{"d d"}, help: "delete"},
	{keys: []string{"g o"}, help: "overdue"},
	{keys: []string{"g u"}, help: "upcoming"},
	{keys: []string{"g y"}, help: "anytime"},
	{keys: []string{"g d"}, help: "delegated"},
}

// captureBindings are the keys in the capture box. Closing is filing there, so
// they had better be written down: they were the one set this table did not
// cover, and the box is where the surprises were.
var captureBindings = []binding{
	{keys: []string{"esc", "⌃s"}, help: "file what is there and close"},
	{keys: []string{"⌃x"}, help: "scrap the draft"},
	{keys: []string{"tab"}, help: "name the call, and back"},
	{keys: []string{"u"}, help: "take it back, words and all"},
}

// weekBindings only mean something on the planner.
var weekBindings = []binding{
	{keys: []string{"1 – 7"}, help: "put it on that day"},
	{keys: []string{"0"}, help: "take it off the calendar"},
	{keys: []string{"[", "]"}, help: "a day earlier / later"},
	{keys: []string{"<", ">"}, help: "previous / next week"},
	{keys: []string{"."}, help: "back to this week"},
	{keys: []string{"w", "esc"}, help: "back to the list"},
}

// lookup finds the binding for a key, if there is one.
func lookup(bindings []binding, key string) (binding, bool) {
	for _, b := range bindings {
		for _, k := range b.keys {
			if k == key {
				return b, true
			}
		}
	}
	return binding{}, false
}

// jumps are the views the g-chord reaches.
var jumps = map[string]store.View{
	"o": store.ViewOverdue,
	"u": store.ViewUpcoming,
	"y": store.ViewAnytime,
	"d": store.ViewDelegated,
}
