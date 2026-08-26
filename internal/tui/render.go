package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/drew-mcl/todo/internal/parse"
	"github.com/drew-mcl/todo/internal/store"
)

// The pieces every screen is built from. The list and the planner draw the same
// task and the same status bar; before this they each had their own copy, and
// the two had already started to differ.

// bar lays a line out with something at each end. Every header and status line
// in the app is one of these.
func (m *Model) bar(left, right string) string {
	gap := m.width - 4 - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		return gutter + left
	}
	return gutter + left + strings.Repeat(" ", gap) + right
}

// status is the line along the bottom: where you are, how far through, and
// whatever the app last had to say for itself.
func (m *Model) status(left, keys string) string {
	right := styFaint.Render(keys)
	if msg, ok := m.message(); msg != "" {
		style := styErr
		if ok {
			style = styAccent
		}
		right = style.Render(truncate(msg, m.width/2))
	}
	return m.bar(left, right)
}

// position renders "3/12" when there is a list to be somewhere in.
func position(cursor, total int) string {
	if total == 0 {
		return ""
	}
	return "  " + styFaint.Render(itoa(cursor+1)+"/"+itoa(total))
}

// row draws a task: the mark, the title, and the priority pushed to the edge.
// indent is how far in the block sits, which is all that differs between the
// list and the planner.
func (m *Model) row(t *store.Task, selected bool, indent int) string {
	pad := strings.Repeat(" ", indent)
	lead := pad + "  "
	if selected {
		lead = pad + styCursor.Render(bar) + " "
	}

	mark, title := styFaint.Render(hollow), styTitle.Render(truncate(t.Title, m.width-12-indent))
	if t.Done() {
		// Accent, not grey: a finished thing should look finished for the
		// moment it is still on screen.
		mark = styAccent.Render(bullet)
		title = styDone.Render(truncate(t.Title, m.width-12-indent))
	}

	out := lead + mark + " " + title
	if t.Priority > 0 {
		marks := styDanger.Render(t.Priority.Marks())
		if gap := m.width - 4 - lipgloss.Width(out) - lipgloss.Width(marks); gap > 1 {
			out += strings.Repeat(" ", gap) + marks
		}
	}
	return gutter + out
}

// meta is the quiet line under a task: topic, when, who, tags. what selects
// which of those are worth the space on a given screen.
type meta struct {
	due, assignee, tags, lateOnly bool
}

var (
	listMeta = meta{due: true, assignee: true, tags: true}
	weekMeta = meta{assignee: true, lateOnly: true}
)

func (m *Model) meta(t *store.Task, what meta, indent int) string {
	parts := []string{m.dot(t.Topic) + " " + styDim.Render(t.Topic)}

	if t.Due != nil {
		late := t.Overdue(m.now())
		if what.due || (what.lateOnly && late) {
			label := strings.ToLower(parse.FormatDue(*t.Due, m.now()))
			if late {
				parts = append(parts, styDanger.Render(label))
			} else {
				parts = append(parts, styDim.Render(label))
			}
		}
	}
	if what.assignee && t.Assignee != "" {
		parts = append(parts, styDim.Render(t.Assignee))
	}
	if what.tags {
		for _, tag := range t.Tags {
			parts = append(parts, styDim.Render("#"+tag))
		}
	}
	return gutter + strings.Repeat(" ", indent+4) + strings.Join(parts, styFaint.Render(" · "))
}

// block is a task and its meta line, which is how both screens draw one.
func (m *Model) block(t *store.Task, selected bool, indent int, what meta) []string {
	return []string{m.row(t, selected, indent), m.meta(t, what, indent)}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}
