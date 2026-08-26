package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/drew-mcl/todo/internal/palette"
	"github.com/drew-mcl/todo/internal/parse"
	"github.com/drew-mcl/todo/internal/store"
)

const (
	gutter  = "  "
	bullet  = "●"
	hollow  = "○"
	bar     = "▌"
	quoteIn = "│"
)

func (m *Model) View() string {
	if m.width == 0 {
		return ""
	}
	switch m.mode {
	case modeCapture:
		return m.viewCapture()
	case modeEdit:
		return m.viewEdit()
	case modeHelp:
		return m.viewHelp()
	default:
		return m.viewList()
	}
}

// rule draws the hairline that separates the chrome from the content.
func (m *Model) rule() string {
	return styRule.Render(strings.Repeat("─", max(0, m.width-4)))
}

// dot renders a topic in its own colour.
func (m *Model) dot(topic string) string {
	hue, ok := m.hues[topic]
	if !ok {
		hue = palette.TopicHue(topic)
	}
	return hueStyle(hue).Render(bullet)
}

// ── the list ────────────────────────────────────────────────────────────────

func (m *Model) viewList() string {
	var b strings.Builder
	b.WriteString(m.header())
	b.WriteString("\n")

	// header is two lines, the footer three; the rest is list.
	room := max(1, m.height-6)
	lines, at := m.listLines()
	lines = window(lines, at, room)

	b.WriteString(strings.Join(lines, "\n"))
	if pad := room - len(lines); pad > 0 {
		b.WriteString(strings.Repeat("\n", pad))
	}

	b.WriteString("\n" + gutter + m.rule() + "\n")
	b.WriteString(m.status())
	return b.String()
}

// window keeps the selected line on screen, scrolling only as far as it must.
func window(lines []string, at, room int) []string {
	if len(lines) <= room {
		return lines
	}
	start := 0
	// A margin, so the selection is never pinned to the very edge.
	const margin = 3
	if at > room-margin {
		start = at - room + margin
	}
	if start > len(lines)-room {
		start = len(lines) - room
	}
	if start < 0 {
		start = 0
	}
	return lines[start : start+room]
}

func (m *Model) header() string {
	now := m.now()
	left := styBrand.Render("todo")

	open := m.counts[store.ViewToday]
	right := styFaint.Render(strings.ToLower(now.Format("Mon 2 January")))
	if m.view == store.ViewToday && open+m.doneToday > 0 {
		right = styFaint.Render(fmt.Sprintf("%s · %d of %d done",
			strings.ToLower(now.Format("Mon 2 January")), m.doneToday, open+m.doneToday))
	}

	gap := m.width - 4 - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		gap = 1
	}
	return gutter + left + strings.Repeat(" ", gap) + right + "\n" + gutter + m.rule()
}

// listLines draws the whole list and reports which line the cursor sits on, so
// the caller can scroll to it.
func (m *Model) listLines() ([]string, int) {
	if len(m.flat) == 0 {
		line, hint := m.emptyWords()
		return []string{"", gutter + styDim.Render(line), gutter + styFaint.Render(hint)}, 0
	}

	var out []string
	at, index := 0, 0

	for _, sec := range m.sections {
		out = append(out, "")
		if sec.label != "" {
			out = append(out, gutter+styHeading.Render(strings.ToUpper(sec.label)))
		}
		for _, t := range sec.tasks {
			if index == m.cursor {
				at = len(out)
			}
			out = append(out, m.taskLines(t, index == m.cursor)...)
			// A blank line between tasks: at this density the blocks run into
			// one another without it.
			out = append(out, "")
			index++
		}
	}
	return out, at
}

func (m *Model) taskLines(t *store.Task, selected bool) []string {
	var out []string

	// The cursor is a mark in the gutter, not a highlight across the row.
	lead := "  "
	if selected {
		lead = styCursor.Render(bar) + " "
	}

	box := styFaint.Render(hollow)
	title := styTitle.Render(truncate(t.Title, m.width-12))
	if t.Done() {
		box = styDim.Render(bullet)
		title = styDone.Render(truncate(t.Title, m.width-12))
	}

	row := lead + box + " " + title
	if t.Priority > 0 {
		marks := styDanger.Render(t.Priority.Marks())
		if gap := m.width - 4 - lipgloss.Width(row) - lipgloss.Width(marks); gap > 1 {
			row += strings.Repeat(" ", gap) + marks
		}
	}
	out = append(out, gutter+row)

	if meta := m.metaLine(t); meta != "" {
		out = append(out, gutter+"    "+meta)
	}
	for _, n := range strings.Split(t.Note, "\n") {
		if n == "" {
			continue
		}
		out = append(out, gutter+"    "+styRule.Render(quoteIn)+" "+
			styDim.Render(truncate(n, m.width-12)))
	}
	return out
}

func (m *Model) metaLine(t *store.Task) string {
	parts := []string{m.dot(t.Topic) + " " + styDim.Render(t.Topic)}

	if t.Due != nil {
		label := strings.ToLower(parse.FormatDue(*t.Due, m.now()))
		if t.Overdue(m.now()) {
			parts = append(parts, styDanger.Render(label))
		} else {
			parts = append(parts, styDim.Render(label))
		}
	}
	if t.Assignee != "" {
		parts = append(parts, styDim.Render(t.Assignee))
	}
	for _, tag := range t.Tags {
		parts = append(parts, styDim.Render("#"+tag))
	}
	return strings.Join(parts, styFaint.Render(" · "))
}

func (m *Model) emptyWords() (string, string) {
	switch m.view {
	case store.ViewToday:
		return "nothing due today.", "press n and paste what came up on the call."
	case store.ViewOverdue:
		return "nothing has slipped.", "everything with a date is still ahead of you."
	case store.ViewUpcoming:
		return "nothing scheduled ahead.", "give a task a day with | eow or | +3d."
	case store.ViewAnytime:
		return "no undated tasks.", "everything you are carrying has a day against it."
	case store.ViewDelegated:
		return "you are not waiting on anyone.", "add @name to a line to track what someone owes you."
	case store.ViewLogbook:
		return "nothing finished yet.", "completed tasks collect here."
	default:
		return "the list is empty.", "press n and paste your notes."
	}
}

// ── status ──────────────────────────────────────────────────────────────────

func (m *Model) status() string {
	if m.onSearch {
		return gutter + m.search.View()
	}

	left := styKey.Render(string(m.view))
	if m.q != "" {
		left += styFaint.Render(" /" + m.q)
	}
	if len(m.flat) > 0 {
		left += "  " + styFaint.Render(fmt.Sprintf("%d/%d", m.cursor+1, len(m.flat)))
	}

	right := styFaint.Render("n capture · j/k · x done · ? keys · q quit")
	switch {
	case m.problem != "":
		right = styErr.Render(truncate(m.problem, m.width/2))
	case m.flash != "":
		right = styAccent.Render(m.flash)
	}

	gap := m.width - 4 - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		return gutter + left
	}
	return gutter + left + strings.Repeat(" ", gap) + right
}

// ── capture ─────────────────────────────────────────────────────────────────

func (m *Model) viewCapture() string {
	var b strings.Builder

	right := styFaint.Render("⌃s add · tab · esc")
	left := styBrand.Render("capture") + "  " + m.title.View()
	// The title input is sized in layout() to leave room for both ends; clip
	// anyway, because a header that wraps breaks every line beneath it.
	head := truncateStyled(left, max(10, m.width-4-lipgloss.Width(right)-2))
	gap := max(1, m.width-4-lipgloss.Width(head)-lipgloss.Width(right))
	b.WriteString(gutter + head + strings.Repeat(" ", gap) + right + "\n")
	b.WriteString(gutter + m.rule() + "\n")
	b.WriteString(m.draft.View() + "\n")
	b.WriteString(gutter + m.rule() + "\n")

	if m.preview == nil || len(m.preview.Lines) == 0 {
		b.WriteString("\n" + gutter + styFaint.Render("your lines appear here as they will be filed"))
		return b.String()
	}

	tasks, notes, skipped := m.preview.Counts()
	var summary []string
	if tasks > 0 {
		summary = append(summary, plural(tasks, "task", "tasks"))
	}
	if notes > 0 {
		summary = append(summary, plural(notes, "note", "notes"))
	}
	if skipped > 0 {
		summary = append(summary, fmt.Sprintf("%d skipped", skipped))
	}
	if len(summary) == 0 {
		summary = append(summary, "nothing yet")
	}
	b.WriteString(gutter + styHeading.Render(strings.ToUpper(strings.Join(summary, " · "))) + "\n")

	room := m.height - m.draft.Height() - 9
	shown := 0
	for _, line := range m.preview.Lines {
		if shown >= room {
			b.WriteString(gutter + styFaint.Render("…") + "\n")
			break
		}
		if line.Kind == parse.KindBlank {
			continue
		}
		b.WriteString(gutter + m.shorthand(line.Raw) + "\n")
		shown++

		switch {
		case line.Task != nil:
			b.WriteString(gutter + "  " + styTitle.Render(truncate(line.Task.Title, m.width-16)) +
				"  " + m.previewMeta(line.Task) + "\n")
			shown++
			if line.Task.Warning != "" {
				b.WriteString(gutter + "  " + styDanger.Render(line.Task.Warning) + "\n")
				shown++
			}
		case line.Kind == parse.KindNote:
			b.WriteString(gutter + "  " + styFaint.Render("attached") + "\n")
			shown++
		case line.Kind == parse.KindSkipped:
			b.WriteString(gutter + "  " + styFaint.Render(line.Reason) + "\n")
			shown++
		}
	}
	return b.String()
}

// shorthand paints a raw line with its grammar coloured in, using the same
// tokens the browser uses.
func (m *Model) shorthand(raw string) string {
	var b strings.Builder
	for _, tok := range parse.Highlight(raw, m.now()) {
		b.WriteString(tokenStyle(tok.Kind).Render(tok.Text))
	}
	return truncateStyled(b.String(), m.width-4)
}

func (m *Model) previewMeta(t *parse.Task) string {
	parts := []string{m.dot(t.Topic) + " " + styDim.Render(t.Topic)}
	if t.Due != nil {
		parts = append(parts, styAccent.Render(strings.ToLower(parse.FormatDue(*t.Due, m.now()))))
	}
	if t.Assignee != "" {
		parts = append(parts, styDim.Render(t.Assignee))
	}
	if t.Priority > 0 {
		parts = append(parts, styDanger.Render(t.Priority.Marks()))
	}
	for _, tag := range t.Tags {
		parts = append(parts, styDim.Render("#"+tag))
	}
	return strings.Join(parts, styFaint.Render(" · "))
}

// ── edit and help ───────────────────────────────────────────────────────────

func (m *Model) viewEdit() string {
	var b strings.Builder
	b.WriteString(m.header() + "\n")
	b.WriteString("\n" + gutter + styHeading.Render("EDIT THE LINE") + "\n\n")
	b.WriteString(gutter + m.edit.View() + "\n\n")
	b.WriteString(gutter + m.shorthand(m.edit.Value()) + "\n")
	b.WriteString("\n" + gutter + styFaint.Render("enter save · esc cancel"))
	return b.String()
}

var helpKeys = [][2]string{
	{"n", "capture"},
	{"⌃s", "add what you have typed"},
	{"j / k", "move"},
	{"g g / G", "top / bottom"},
	{"x", "complete"},
	{"e", "edit the line"},
	{"d d", "delete"},
	{"u", "undo the last capture"},
	{"/", "search"},
	{"t", "today"},
	{"a", "everything open"},
	{"l", "logbook"},
	{"g o / u / y / d", "overdue / upcoming / anytime / delegated"},
	{"q", "quit"},
}

func (m *Model) viewHelp() string {
	var b strings.Builder
	b.WriteString(m.header() + "\n")
	b.WriteString("\n" + gutter + styHeading.Render("KEYS") + "\n\n")
	for _, k := range helpKeys {
		b.WriteString(gutter + styKey.Render(pad(k[0], 16)) + styDim.Render(k[1]) + "\n")
	}
	b.WriteString("\n" + gutter + styHeading.Render("SHORTHAND") + "\n\n")
	b.WriteString(gutter + m.shorthand("topic | what needs doing | today @who !! #tag > note") + "\n")
	b.WriteString(gutter + m.shorthand("      | repeats the topic above") + "\n")
	b.WriteString("\n" + gutter + styFaint.Render("lines without a | are left alone"))
	b.WriteString("\n\n" + gutter + styFaint.Render("any key to go back"))
	return b.String()
}

// ── text helpers ────────────────────────────────────────────────────────────

func pad(s string, n int) string {
	for lipgloss.Width(s) < n {
		s += " "
	}
	return s
}

func truncate(s string, n int) string {
	if n < 4 {
		return ""
	}
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}

// truncateStyled cuts a line that already carries colour, which cannot be
// sliced by rune without splitting an escape sequence.
func truncateStyled(s string, n int) string {
	if lipgloss.Width(s) <= n {
		return s
	}
	return lipgloss.NewStyle().MaxWidth(n).Render(s)
}

func plural(n int, one, many string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, one)
	}
	return fmt.Sprintf("%d %s", n, many)
}
