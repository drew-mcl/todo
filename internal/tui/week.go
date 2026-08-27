package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/drew-mcl/todo/internal/store"
)

// The browser plans a week by dragging cards between seven columns. Seven
// columns do not fit a terminal -- at ninety characters that is twelve per day,
// which is not a task -- and there is nothing to drag with anyway.
//
// So the week stacks instead of spreading, and scheduling is a keystroke: 1-7
// puts the selected task on that day, [ and ] nudge it either side, 0 takes it
// off the calendar. It is faster than dragging, which is the usual consolation
// of doing something the terminal way.

type weekLoaded struct {
	plan *store.Plan
	err  error
}

func (m *Model) loadWeek() tea.Msg {
	plan, err := m.store.Week(m.weekStart, m.now(), store.Query{Search: m.q})
	if err != nil {
		return weekLoaded{err: err}
	}
	return weekLoaded{plan: plan}
}

// weekFlat is every task on the board, in the order the eye reads them, so one
// cursor can move through days and trays alike.
func (m *Model) weekFlat() []*store.Task {
	if m.plan == nil {
		return nil
	}
	var out []*store.Task
	for _, d := range m.plan.Days {
		out = append(out, d.Tasks...)
	}
	out = append(out, m.plan.Overdue...)
	out = append(out, m.plan.Unscheduled...)
	return out
}

func (m *Model) weekCurrent() *store.Task {
	flat := m.weekFlat()
	if m.cursor < 0 || m.cursor >= len(flat) {
		return nil
	}
	return flat[m.cursor]
}

func (m *Model) updateWeek(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	if m.chord == "d" {
		m.chord = ""
		if key == "d" {
			if t := m.weekCurrent(); t != nil {
				if err := m.store.Delete(t.ID); err != nil {
					m.failed(err)
					return m, nil
				}
				m.say("deleted %q", t.Title)
			}
			return m, m.loadWeek
		}
		return m, nil
	}

	// 1-7 schedule onto a day of the week on screen.
	if len(key) == 1 && key[0] >= '1' && key[0] <= '7' {
		return m, m.schedule(m.weekStart.AddDate(0, 0, int(key[0]-'1')))
	}

	switch key {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "esc", "w":
		m.mode, m.cursor = modeList, 0
		return m, m.reload
	case "?":
		m.mode = modeHelp
	case "d":
		m.chord = "d"
	case "j", "down":
		if m.cursor < len(m.weekFlat())-1 {
			m.cursor++
		}
	case "k", "up":
		if m.cursor > 0 {
			m.cursor--
		}
	case "G":
		m.cursor = max(0, len(m.weekFlat())-1)
	case "0":
		return m, m.unschedule()
	case "]", "L":
		return m, m.shift(1)
	case "[", "H":
		return m, m.shift(-1)
	case ">":
		m.weekStart, m.cursor = m.weekStart.AddDate(0, 0, 7), 0
		return m, m.loadWeek
	case "<":
		m.weekStart, m.cursor = m.weekStart.AddDate(0, 0, -7), 0
		return m, m.loadWeek
	case "n":
		// n is capture everywhere else, and a key that means two things is
		// worse than a week you have to reach with one more press.
		return m, m.openCapture()
	case ".":
		m.weekStart, m.cursor = store.WeekStart(m.now()), 0
		return m, m.loadWeek
	case "x", " ":
		if t := m.weekCurrent(); t != nil {
			if _, err := m.store.Toggle(t.ID, m.now()); err != nil {
				m.failed(err)
				return m, nil
			}
		}
		return m, m.loadWeek
	case "e":
		return m, m.editTask(m.weekCurrent())
	case "t":
		m.mode, m.view, m.cursor = modeList, store.ViewToday, 0
		return m, m.reload
	case "a":
		m.mode, m.view, m.cursor = modeList, store.ViewAll, 0
		return m, m.reload
	}
	return m, nil
}

func (m *Model) schedule(day time.Time) tea.Cmd {
	t := m.weekCurrent()
	if t == nil {
		return nil
	}
	if _, err := m.store.Schedule(t.ID, day.Format("2006-01-02")); err != nil {
		m.failed(err)
		return nil
	}
	m.say("%s → %s", truncate(t.Title, 28), strings.ToLower(day.Format("Mon 2 Jan")))
	return m.loadWeek
}

func (m *Model) unschedule() tea.Cmd {
	t := m.weekCurrent()
	if t == nil {
		return nil
	}
	if _, err := m.store.Schedule(t.ID, ""); err != nil {
		m.failed(err)
		return nil
	}
	m.say("%s → unscheduled", truncate(t.Title, 28))
	return m.loadWeek
}

// shift nudges a task a day either way, which is most of what planning is.
func (m *Model) shift(days int) tea.Cmd {
	t := m.weekCurrent()
	if t == nil {
		return nil
	}
	from := truncateDay(m.now())
	if t.Due != nil {
		from = *t.Due
	}
	return m.schedule(from.AddDate(0, 0, days))
}

func truncateDay(t time.Time) time.Time {
	y, mo, d := t.Date()
	return time.Date(y, mo, d, 0, 0, 0, 0, t.Location())
}

// ── drawing ─────────────────────────────────────────────────────────────────

func (m *Model) viewWeek() string {
	var b strings.Builder

	b.WriteString(m.bar(
		styBrand.Render("todo")+styFaint.Render(" · week"),
		styFaint.Render(strings.ToLower(weekRange(m.plan)))) + "\n")
	b.WriteString(gutter + m.rule() + "\n")

	blocks, at := m.weekBlocks()
	room := max(1, m.height-6)
	lines, _, _ := paint(blocks, at, room)
	b.WriteString(strings.Join(lines, "\n"))
	if pad := room - len(lines); pad > 0 {
		b.WriteString(strings.Repeat("\n", pad))
	}

	b.WriteString("\n" + gutter + m.rule() + "\n")
	b.WriteString(m.weekStatus())
	return b.String()
}

// weekBlocks lays the board out as measurable pieces and reports which one the
// cursor sits on, so only the days on screen are drawn.
func (m *Model) weekBlocks() ([]block, int) {
	if m.plan == nil {
		rows := []string{"", gutter + styFaint.Render("loading…")}
		return []block{{height: len(rows), draw: func() []string { return rows }}}, 0
	}

	var blocks []block
	at, index := 0, 0
	today := truncateDay(m.now())

	day := func(heading string, tasks []*store.Task, tone string) {
		head := styHeading.Render(strings.ToUpper(heading))
		switch tone {
		case "today":
			head = styAccent.Render(strings.ToUpper(heading))
		case "late":
			head = styDanger.Render(strings.ToUpper(heading))
		}
		if len(tasks) > 0 {
			head += styFaint.Render(fmt.Sprintf("  %d", len(tasks)))
		}
		rows := []string{"", gutter + head}
		if len(tasks) == 0 {
			rows = append(rows, gutter+"    "+styFaint.Render("—"))
		}
		blocks = append(blocks, block{height: len(rows), draw: func() []string { return rows }})

		for _, t := range tasks {
			if index == m.cursor {
				at = len(blocks)
			}
			selected := index == m.cursor
			// The row and its meta line, which is what block draws.
			blocks = append(blocks, block{
				height: 2,
				draw:   func() []string { return m.weekTask(t, selected) },
			})
			index++
		}
	}

	for _, d := range m.plan.Days {
		tone := ""
		switch {
		case d.Date.Equal(today):
			tone = "today"
		case d.Date.Before(today):
			tone = "past"
		}
		heading := strings.ToLower(d.Date.Format("Mon 2 Jan"))
		if tone == "today" {
			heading += " · today"
		}
		day(heading, d.Tasks, tone)
	}

	day("overdue", m.plan.Overdue, "late")
	day("unscheduled", m.plan.Unscheduled, "")
	return blocks, at
}

func (m *Model) weekTask(t *store.Task, selected bool) []string {
	return m.block(t, selected, 2, weekMeta)
}

func (m *Model) weekStatus() string {
	return m.status(styKey.Render("week")+position(m.cursor, len(m.weekFlat())),
		"1-7 day · 0 off · [ ] shift · < > week · w list")
}

// weekRange names the span on screen.
func weekRange(p *store.Plan) string {
	if p == nil {
		return ""
	}
	end := p.End()
	if p.Start.Month() == end.Month() {
		return p.Start.Format("2") + "–" + end.Format("2 Jan 2006")
	}
	return p.Start.Format("2 Jan") + " – " + end.Format("2 Jan 2006")
}
