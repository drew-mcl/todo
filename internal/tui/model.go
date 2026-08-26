// Package tui is todo in a terminal.
//
// It talks to SQLite directly rather than to the server, so it works whether or
// not anything else is running, and it shares the parser, the store and the
// palette with the web client -- a topic is the same colour in both.
package tui

import (
	"fmt"
	"time"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/drew-mcl/todo/internal/palette"
	"github.com/drew-mcl/todo/internal/parse"
	"github.com/drew-mcl/todo/internal/store"
)

type mode int

const (
	modeList mode = iota
	modeCapture
	modeEdit
	modeHelp
	modeWeek
)

// Model is the whole application.
type Model struct {
	store *store.Store
	now   func() time.Time

	mode     mode
	cameFrom mode // where capture should return to
	view     store.View
	sort     store.Sort
	q        string

	sections  []store.Section
	flat      []*store.Task
	hues      map[string]int
	counts    map[store.View]int
	doneToday int

	cursor        int
	width, height int

	draft   textarea.Model
	title   textinput.Model
	onTitle bool
	preview *parse.Result

	edit    textinput.Model
	editing *store.Task

	search   textinput.Model
	onSearch bool

	anim *anim

	plan      *store.Plan
	weekStart time.Time

	// A completed task is held on screen, struck through, before it goes.
	leavingID int64
	leavingAt time.Time

	// The day meter fills rather than jumping, and a message fades rather than
	// sitting there until something else replaces it.
	meterAt time.Time
	meter   float64
	flashAt time.Time

	lastBatch int64
	flash     string
	problem   string
	chord     string
}

// New builds the program's initial state.
func New(st *store.Store, now func() time.Time) *Model {
	if now == nil {
		now = time.Now
	}

	draft := textarea.New()
	draft.Placeholder = "topic | what needs doing | today @who !!"
	draft.ShowLineNumbers = false
	draft.CharLimit = 0
	// Line up with the two-space gutter the rest of the screen uses, instead of
	// the default bar hard against column zero.
	draft.Prompt = gutter
	draft.FocusedStyle.CursorLine = lipgloss.NewStyle()
	draft.FocusedStyle.Prompt = styFaint
	draft.BlurredStyle.Prompt = styFaint

	title := textinput.New()
	title.Placeholder = "name this call (optional)"
	title.Prompt = ""

	edit := textinput.New()
	edit.Prompt = ""

	search := textinput.New()
	search.Placeholder = "search"
	search.Prompt = "/"

	return &Model{
		store: st, now: now,
		view: store.ViewToday, sort: store.SortManual,
		draft: draft, title: title, edit: edit, search: search,
		anim: newAnim(), weekStart: store.WeekStart(now()),
	}
}

// Init loads the first list.
func (m *Model) Init() tea.Cmd { return m.reload }

// reloaded carries a refreshed list back into the model.
type reloaded struct {
	sections  []store.Section
	flat      []*store.Task
	hues      map[string]int
	counts    map[store.View]int
	doneToday int
	err       error
}

func (m *Model) reload() tea.Msg {
	now := m.now()
	tasks, err := m.store.List(store.Query{View: m.view, Sort: m.sort, Search: m.q}, now)
	if err != nil {
		return reloaded{err: err}
	}
	counts, err := m.store.Counts(now)
	if err != nil {
		return reloaded{err: err}
	}
	done, err := m.store.DoneOn(now)
	if err != nil {
		return reloaded{err: err}
	}
	topics, err := m.store.Topics()
	if err != nil {
		return reloaded{err: err}
	}

	names := make([]string, 0, len(topics))
	for _, t := range topics {
		names = append(names, t.Name)
	}
	// Every topic on the list gets a colour of its own.
	for _, t := range tasks {
		names = append(names, t.Topic)
	}

	return reloaded{
		sections:  store.Sections(tasks, m.view, m.sort, now),
		flat:      tasks,
		hues:      palette.Assign(names),
		counts:    counts,
		doneToday: done,
	}
}

func (m *Model) current() *store.Task {
	if m.cursor < 0 || m.cursor >= len(m.flat) {
		return nil
	}
	return m.flat[m.cursor]
}

func (m *Model) say(format string, args ...any) {
	m.flash = fmt.Sprintf(format, args...)
	m.flashAt = m.now()
	m.problem = ""
}

func (m *Model) failed(err error) {
	m.problem = err.Error()
	m.flash = ""
}
