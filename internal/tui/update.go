package tui

import (
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/drew-mcl/todo/internal/parse"
	"github.com/drew-mcl/todo/internal/store"
)

// jumps are the g-chords, matching the browser so the two share muscle memory.
var jumps = map[string]store.View{
	"o": store.ViewOverdue,
	"u": store.ViewUpcoming,
	"y": store.ViewAnytime,
	"d": store.ViewDelegated,
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.layout()
		return m, nil

	case weekLoaded:
		if msg.err != nil {
			m.failed(msg.err)
			return m, nil
		}
		m.plan = msg.plan
		if n := len(m.weekFlat()); m.cursor >= n {
			m.cursor = max(0, n-1)
		}
		return m, nil

	case reloaded:
		if msg.err != nil {
			m.failed(msg.err)
			return m, nil
		}
		m.sections, m.flat = msg.sections, msg.flat
		m.hues, m.counts = msg.hues, msg.counts
		if msg.doneToday != m.doneToday {
			// Start the meter travelling from where it currently sits.
			m.meter, m.meterAt = m.meterNow(), m.now()
			m.doneToday = msg.doneToday
		}
		if m.cursor >= len(m.flat) {
			m.cursor = max(0, len(m.flat)-1)
		}
		return m, tick()

	case settledMsg:
		m.leavingID = 0
		if m.mode == modeWeek {
			return m, m.loadWeek
		}
		return m, m.reload

	case tickMsg:
		// Runs only while something is moving, and stops on its own.
		if m.animating() {
			return m, tick()
		}
		return m, nil

	case tea.KeyMsg:
		switch m.mode {
		case modeCapture:
			return m.updateCapture(msg)
		case modeEdit:
			return m.updateEdit(msg)
		case modeWeek:
			return m.updateWeek(msg)
		case modeHelp:
			m.mode = modeList
			return m, nil
		default:
			return m.updateList(msg)
		}
	}
	return m, nil
}

// layout sizes the panes to the terminal.
func (m *Model) layout() {
	m.draft.SetWidth(max(20, m.width-4))
	// Half the screen to write in, the rest to watch it become tasks.
	m.draft.SetHeight(max(3, m.height/2-6))
	// Leaves room for "capture" on the left and the key hints on the right.
	m.title.Width = max(12, m.width-40)
	m.edit.Width = max(20, m.width-6)
	m.search.Width = max(10, m.width-8)
}

func (m *Model) updateList(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.onSearch {
		switch msg.String() {
		case "enter":
			// Already filtered; enter just puts the keys back on the list.
			m.onSearch = false
			m.search.Blur()
			return m, nil
		case "esc":
			m.onSearch = false
			m.search.Blur()
			m.search.SetValue("")
			m.q = ""
			return m, m.reload
		}

		var cmd tea.Cmd
		m.search, cmd = m.search.Update(msg)
		// Narrow as you type. Waiting for enter makes a list feel like a form.
		if q := m.search.Value(); q != m.q {
			m.q, m.cursor = q, 0
			return m, tea.Batch(cmd, m.reload)
		}
		return m, cmd
	}

	key := msg.String()

	// g is a prefix; d doubles for delete.
	if m.chord != "" {
		chord := m.chord
		m.chord = ""
		switch chord {
		case "g":
			if v, ok := jumps[key]; ok {
				m.view, m.cursor = v, 0
				return m, m.reload
			}
			if key == "g" {
				m.cursor = 0
			}
			return m, nil
		case "d":
			if key == "d" {
				return m, m.deleteCurrent()
			}
			return m, nil
		}
	}

	switch key {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "?":
		m.mode = modeHelp
	case "g":
		m.chord = "g"
	case "d":
		m.chord = "d"
	case "G":
		m.cursor = max(0, len(m.flat)-1)
	case "j", "down":
		if m.cursor < len(m.flat)-1 {
			m.cursor++
		}
	case "k", "up":
		if m.cursor > 0 {
			m.cursor--
		}
	case "t":
		m.view, m.cursor = store.ViewToday, 0
		return m, m.reload
	case "a":
		m.view, m.cursor = store.ViewAll, 0
		return m, m.reload
	case "l":
		m.view, m.cursor = store.ViewLogbook, 0
		return m, m.reload
	case "/":
		m.onSearch = true
		m.search.Focus()
		return m, nil
	case "n":
		m.mode, m.cameFrom = modeCapture, modeList
		m.onTitle = false
		m.draft.Focus()
		m.layout()
		m.preview = parse.Parse(m.draft.Value(), m.now())
		return m, tea.Batch(textarea.Blink, tick())
	case "e":
		if t := m.current(); t != nil {
			m.mode, m.editing = modeEdit, t
			m.edit.SetValue(rawOf(t))
			m.edit.Focus()
			m.edit.CursorEnd()
		}
	case "x", " ":
		return m, m.toggleCurrent()
	case "u":
		return m, m.undoLast()
	case "w":
		m.mode, m.cursor = modeWeek, 0
		m.weekStart = store.WeekStart(m.now())
		return m, m.loadWeek
	}
	return m, nil
}

// rawOf is the shorthand a task would be written as. The stored line is used
// when there is one, so editing shows you what you actually typed.
func rawOf(t *store.Task) string {
	if t.Raw != "" {
		return t.Raw
	}
	var b strings.Builder
	b.WriteString(t.Topic + " | " + t.Title)
	if t.Due != nil {
		b.WriteString(" | " + t.Due.Format("2006-01-02"))
	}
	if t.Assignee != "" {
		b.WriteString(" @" + t.Assignee)
	}
	if t.Priority > 0 {
		b.WriteString(" " + t.Priority.Marks())
	}
	for _, tag := range t.Tags {
		b.WriteString(" #" + tag)
	}
	return b.String()
}

// toggleCurrent completes a task and then leaves it alone for a moment. The row
// is already struck through; reloading at once would whip it off the screen
// before that registered as anything.
func (m *Model) toggleCurrent() tea.Cmd {
	t := m.current()
	if t == nil {
		return nil
	}
	updated, err := m.store.Toggle(t.ID, m.now())
	if err != nil {
		m.failed(err)
		return nil
	}
	// Swap the local copy so it draws as done without a round trip.
	for i, x := range m.flat {
		if x.ID == updated.ID {
			m.flat[i] = updated
		}
	}
	for si := range m.sections {
		for ti, x := range m.sections[si].Tasks {
			if x.ID == updated.ID {
				m.sections[si].Tasks[ti] = updated
			}
		}
	}
	m.leavingID, m.leavingAt = updated.ID, m.now()
	return tea.Batch(tick(), tea.Tick(linger, func(time.Time) tea.Msg { return settledMsg{} }))
}

// settledMsg arrives once a completed task has had its moment.
type settledMsg struct{}

func (m *Model) deleteCurrent() tea.Cmd {
	t := m.current()
	if t == nil {
		return nil
	}
	if err := m.store.Delete(t.ID); err != nil {
		m.failed(err)
		return nil
	}
	m.say("deleted %q", t.Title)
	return m.reload
}

func (m *Model) undoLast() tea.Cmd {
	if m.lastBatch == 0 {
		m.say("nothing to undo")
		return nil
	}
	n, err := m.store.UndoBatch(m.lastBatch)
	if err != nil {
		m.failed(err)
		return nil
	}
	m.lastBatch = 0
	m.say("undid %d", n)
	return m.reload
}

// ── capture ─────────────────────────────────────────────────────────────────

func (m *Model) updateCapture(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		// The draft stays in the box, so reopening picks up where you left off.
		m.mode = m.cameFrom
		m.draft.Blur()
		m.title.Blur()
		return m, nil

	case "ctrl+s":
		return m, m.commit()

	case "tab":
		m.onTitle = !m.onTitle
		if m.onTitle {
			m.draft.Blur()
			m.title.Focus()
		} else {
			m.title.Blur()
			m.draft.Focus()
		}
		return m, nil
	}

	var cmd tea.Cmd
	if m.onTitle {
		m.title, cmd = m.title.Update(msg)
	} else {
		m.draft, cmd = m.draft.Update(msg)
		// Re-read on every keystroke. The parse is cheap, and watching a line
		// become a task is the whole point of the box.
		m.preview = parse.Parse(m.draft.Value(), m.now())
		return m, tea.Batch(cmd, tick())
	}
	return m, cmd
}

func (m *Model) commit() tea.Cmd {
	res := parse.Parse(m.draft.Value(), m.now())
	if len(res.Tasks) == 0 {
		m.failed(errNothing{})
		return nil
	}
	batch, err := m.store.CreateBatch(res.Tasks,
		store.Capture{Source: "tui", Title: m.title.Value()}, m.now())
	if err != nil {
		m.failed(err)
		return nil
	}
	m.lastBatch = batch
	m.say("added %d · u to undo", len(res.Tasks))
	m.draft.SetValue("")
	m.title.SetValue("")
	m.preview = nil
	m.mode = m.cameFrom
	m.draft.Blur()
	if m.mode == modeWeek {
		return m.loadWeek
	}
	return m.reload
}

type errNothing struct{}

func (errNothing) Error() string { return "no line contained a '|', so nothing became a task" }

// ── edit ────────────────────────────────────────────────────────────────────

// The line is the thing you wrote, so the line is the thing you correct: edit
// re-opens the shorthand rather than a form of separate fields.
func (m *Model) updateEdit(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.mode, m.editing = modeList, nil
		m.edit.Blur()
		return m, nil

	case "enter":
		t := m.editing
		m.mode, m.editing = modeList, nil
		m.edit.Blur()
		if t == nil {
			return m, nil
		}
		res := parse.Parse(m.edit.Value(), m.now())
		if len(res.Tasks) == 0 {
			m.failed(errNothing{})
			return m, nil
		}
		p := res.Tasks[0]
		patch := store.Patch{
			Title: &p.Title, Topic: &p.Topic, Note: &p.Note,
			Assignee: &p.Assignee, Priority: &p.Priority, Tags: &p.Tags,
		}
		if p.Due != nil {
			patch.Due = p.Due
		} else {
			patch.ClearDue = true
		}
		if _, err := m.store.Update(t.ID, patch); err != nil {
			m.failed(err)
			return m, nil
		}
		return m, m.reload
	}

	var cmd tea.Cmd
	m.edit, cmd = m.edit.Update(msg)
	return m, cmd
}
