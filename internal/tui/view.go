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
	case modeWeek:
		return m.viewWeek()
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
	blocks, at := m.listBlocks()
	lines, _, _ := paint(blocks, at, room)

	b.WriteString(strings.Join(lines, "\n"))
	if pad := room - len(lines); pad > 0 {
		b.WriteString(strings.Repeat("\n", pad))
	}

	b.WriteString("\n" + gutter + m.rule() + "\n")
	b.WriteString(m.statusLine())
	return b.String()
}

// fold is where a window of room rows starts, so that row at stays on screen.
func fold(total, at, room int) int {
	if total <= room {
		return 0
	}
	// A margin, so the selection is never pinned to the very edge.
	start := 0
	const margin = 3
	if at > room-margin {
		start = at - room + margin
	}
	if start > total-room {
		start = total - room
	}
	if start < 0 {
		start = 0
	}
	return start
}

// block is one thing on a screen that can be measured before it is drawn: a
// task and its detail, a heading, a parsed line and what it became.
type block struct {
	height int
	draw   func() []string
}

// paint draws the blocks around block at that fall inside room rows, and says
// how much is hidden either side.
//
// Only what fits is drawn at all. Measuring costs nothing next to colouring --
// a task is its row, its meta line and its notes -- so a list of five hundred,
// or a paste of five hundred, costs a screenful of lipgloss per frame rather
// than a page of it, twenty-two times a second.
func paint(blocks []block, at, room int) (rows []string, above, below int) {
	starts := make([]int, len(blocks))
	total := 0
	for i, b := range blocks {
		starts[i] = total
		total += b.height
	}

	focus := 0
	if at >= 0 && at < len(blocks) {
		// The end of the block, so the whole of what is selected stays on.
		focus = starts[at] + blocks[at].height - 1
	}
	start := fold(total, focus, room)

	from := -1
	for i, b := range blocks {
		if starts[i]+b.height <= start || starts[i] >= start+room {
			continue
		}
		if from < 0 {
			from = starts[i]
		}
		rows = append(rows, b.draw()...)
	}
	if from < 0 {
		from = start
	}
	// The first block drawn may begin above the fold, and the last may run past
	// the bottom.
	if cut := start - from; cut > 0 && cut < len(rows) {
		rows = rows[cut:]
	}
	if len(rows) > room {
		rows = rows[:room]
	}
	return rows, start, total - start - len(rows)
}

func (m *Model) header() string {
	now := m.now()
	right := styFaint.Render(strings.ToLower(now.Format("Mon 2 January")))

	if open := m.counts[store.ViewToday]; m.view == store.ViewToday && open+m.doneToday > 0 {
		// The date still earns its place; the meter joins it rather than
		// replacing it.
		right += "  " + m.meterBar(10) + "  " +
			styFaint.Render(fmt.Sprintf("%d of %d done", m.doneToday, open+m.doneToday))
	}
	return m.bar(styBrand.Render("todo"), right) + "\n" + gutter + m.rule()
}

// meterBar draws the day's progress. It travels rather than jumping, which is
// the difference between a number changing and something being finished.
func (m *Model) meterBar(width int) string {
	filled := int(m.meterNow()*float64(width) + 0.5)
	filled = min(max(filled, 0), width)
	return styAccent.Render(strings.Repeat("━", filled)) +
		styRule.Render(strings.Repeat("━", width-filled))
}

// listBlocks lays the list out as measurable pieces and reports which one the
// cursor sits on, so the caller can scroll to it and draw only what shows.
func (m *Model) listBlocks() ([]block, int) {
	if len(m.flat) == 0 {
		line, hint := m.emptyWords()
		lead := styDim.Render(line)
		if m.view == store.ViewToday && m.doneToday > 0 {
			lead = styAccent.Render("✓ ") + styTitle.Render(line)
		}
		rows := []string{"", gutter + lead, gutter + styFaint.Render(hint)}
		return []block{{height: len(rows), draw: func() []string { return rows }}}, 0
	}

	var blocks []block
	at, index := 0, 0

	for _, sec := range m.sections {
		head := []string{""}
		if sec.Label != "" {
			head = append(head, gutter+styHeading.Render(strings.ToUpper(sec.Label)))
		}
		blocks = append(blocks, block{height: len(head), draw: func() []string { return head }})

		for _, t := range sec.Tasks {
			if index == m.cursor {
				at = len(blocks)
			}
			selected := index == m.cursor
			blocks = append(blocks, block{
				// The row, its meta line, its notes, and the blank that keeps
				// one task from running into the next: at this density they do.
				height: 3 + noteRows(t.Note),
				draw:   func() []string { return append(m.taskLines(t, selected), "") },
			})
			index++
		}
	}
	return blocks, at
}

// noteRows is how many rows a note takes. taskLines has to agree with it, and a
// test says so: a block that lies about its height scrolls to the wrong place.
func noteRows(note string) int {
	n := 0
	for _, line := range strings.Split(note, "\n") {
		if line != "" {
			n++
		}
	}
	return n
}

func (m *Model) taskLines(t *store.Task, selected bool) []string {
	out := m.block(t, selected, 0, listMeta)
	for _, n := range strings.Split(t.Note, "\n") {
		if n == "" {
			continue
		}
		out = append(out, gutter+"    "+styRule.Render(quoteIn)+" "+
			styDim.Render(truncate(n, m.width-12)))
	}
	return out
}

func (m *Model) emptyWords() (string, string) {
	switch m.view {
	case store.ViewToday:
		// The best thing that happens in here used to read the same as a day
		// that never had anything on it.
		if m.doneToday > 0 {
			return "that is today done.",
				fmt.Sprintf("%d closed. nothing else is owed today.", m.doneToday)
		}
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

func (m *Model) statusLine() string {
	if m.onSearch {
		return gutter + m.search.View()
	}
	left := styKey.Render(string(m.view))
	if m.q != "" {
		left += styFaint.Render(" /" + m.q)
	}
	return m.status(left+position(m.cursor, len(m.flat)),
		"n capture · j/k · x done · ? keys · q quit")
}

// ── capture ─────────────────────────────────────────────────────────────────

func (m *Model) viewCapture() string {
	var b strings.Builder

	// Which of the two fields has the keys has to be unmistakable. Tab used to
	// move the cursor into the one-line title without saying so, and everything
	// typed after it went in there -- which reads as the box having closed and
	// the grammar having stopped working.
	brand, mark := styBrand.Render("capture"), " "
	hint := "esc file · ⌃x scrap · tab name it"
	if m.onTitle {
		brand, mark = styFaint.Render("capture"), styCursor.Render(bar)
		hint = "naming the call · tab the notes · esc back"
	}
	right := styFaint.Render(hint)
	if strings.Contains(m.title.Value(), "|") {
		// A task line in the title box is the mistake tab used to invite, so
		// say which box it is in rather than leaving it to look like a parser
		// that has stopped reading pipes.
		right = styDanger.Render("that is a task line, not a title")
	}
	left := truncateStyled(brand+"  "+mark+m.title.View(),
		max(10, m.width-6-lipgloss.Width(right)))

	b.WriteString(m.bar(left, right) + "\n")
	b.WriteString(gutter + m.rule() + "\n")
	b.WriteString(m.draft.View() + "\n")
	b.WriteString(gutter + m.rule() + "\n")

	if m.preview == nil || len(m.preview.Lines) == 0 {
		b.WriteString("\n" + gutter + styFaint.Render("your lines appear here as they will be filed"))
		return b.String()
	}

	blocks, at := m.previewBlocks()

	// Header, two rules and the summary; the rest is preview.
	room := max(1, m.height-m.draft.Height()-5)
	shown, above, below := paint(blocks, at, room)

	b.WriteString(m.bar(styHeading.Render(m.previewSummary()), scrollMark(above, below)) + "\n")
	b.WriteString(strings.Join(shown, "\n"))
	return b.String()
}

// previewSummary is the count line: what the draft currently amounts to.
func (m *Model) previewSummary() string {
	tasks, notes, skipped := m.preview.Counts()
	var parts []string
	if tasks > 0 {
		parts = append(parts, count(tasks, "task", "tasks"))
	}
	if notes > 0 {
		parts = append(parts, count(notes, "note", "notes"))
	}
	if skipped > 0 {
		parts = append(parts, fmt.Sprintf("%d skipped", skipped))
	}
	if len(parts) == 0 {
		parts = append(parts, "nothing yet")
	}
	return strings.ToUpper(strings.Join(parts, " · "))
}

// scrollMark admits what is off the top and the bottom of the preview, so a
// draft longer than the screen does not look like a preview that has stopped.
func scrollMark(above, below int) string {
	var parts []string
	if above > 0 {
		parts = append(parts, "↑"+itoa(above))
	}
	if below > 0 {
		parts = append(parts, "↓"+itoa(below))
	}
	if len(parts) == 0 {
		return ""
	}
	return styFaint.Render(strings.Join(parts, " "))
}

// previewBlocks lays the parse out as measurable pieces and reports which one
// belongs to the line the cursor is on.
//
// How tall a line's block will be is known without drawing it -- the line, what
// it became, and a warning if there is one -- so the colouring is spent on the
// dozen or so the window actually shows. The preview follows the draft: typing
// on the tenth line used to leave it showing the first six, which reads as a
// preview that has stopped working rather than one looking somewhere else.
func (m *Model) previewBlocks() ([]block, int) {
	now := m.now()
	cursor := m.draft.Line() + 1

	var blocks []block
	at := 0
	for _, line := range m.preview.Lines {
		if line.Kind == parse.KindBlank {
			continue
		}
		// The nearest block at or above the cursor: a blank line has nothing of
		// its own to show, so it holds the preview where it was.
		if line.N <= cursor {
			at = len(blocks)
		}

		height := 2
		switch {
		case line.Task != nil && line.Task.Warning != "":
			height = 3
		case line.Task == nil && line.Kind != parse.KindNote && line.Kind != parse.KindSkipped:
			height = 1
		}
		// Settling is remembered for every line, drawn or not, so scrolling back
		// to one does not start it over.
		p := m.anim.note(previewKey(line), now)
		blocks = append(blocks, block{
			height: height,
			draw:   func() []string { return m.previewRows(line, p, line.N == cursor) },
		})
	}
	m.anim.sweep()
	return blocks, at
}

// previewRows draws one input line and what the parser made of it.
func (m *Model) previewRows(line parse.Line, p float64, live bool) []string {
	// The line being typed carries the same bar the list cursor uses, so which
	// of a page of lines is yours is never in doubt.
	lead := func(indent int) string {
		if live {
			return styCursor.Render(bar) + strings.Repeat(" ", indent+1)
		}
		return gutter + strings.Repeat(" ", indent)
	}

	// The raw line is what you are typing, so it is never animated -- only what
	// the parser made of it settles in underneath.
	rows := []string{lead(0) + m.shorthand(line.Raw)}
	switch {
	case line.Task != nil:
		rows = append(rows, lead(2)+
			ramp(p).Render(truncate(line.Task.Title, m.width-16))+
			"  "+m.previewMeta(line.Task, p))
		if line.Task.Warning != "" {
			rows = append(rows, lead(2)+styDanger.Render(line.Task.Warning))
		}
	case line.Kind == parse.KindNote:
		rows = append(rows, lead(2)+dimmed(p).Render("attached"))
	case line.Kind == parse.KindSkipped:
		rows = append(rows, lead(2)+dimmed(p).Render(line.Reason))
	}
	return rows
}

// previewKey identifies a parsed line by what it says rather than where it sits,
// so typing on line four does not restart the lines above it.
func previewKey(l parse.Line) string {
	if l.Task != nil {
		return "t:" + l.Task.Topic + "|" + l.Task.Title + "|" + l.Task.Assignee
	}
	return string(l.Kind) + ":" + l.Raw
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

// previewMeta draws the parsed detail. p is how settled the line is: the topic
// dot and the colours land last, so the line looks assembled rather than faded.
func (m *Model) previewMeta(t *parse.Task, p float64) string {
	dot := styFaint.Render(hollow)
	if visible(p) {
		dot = m.dot(t.Topic)
	}
	parts := []string{dot + " " + dimmed(p).Render(t.Topic)}

	if t.Due != nil {
		label := strings.ToLower(parse.FormatDue(*t.Due, m.now()))
		if visible(p) {
			parts = append(parts, styAccent.Render(label))
		} else {
			parts = append(parts, dimmed(p).Render(label))
		}
	}
	if t.Assignee != "" {
		parts = append(parts, dimmed(p).Render(t.Assignee))
	}
	if t.Priority > 0 {
		marks := t.Priority.Marks()
		if visible(p) {
			parts = append(parts, styDanger.Render(marks))
		} else {
			parts = append(parts, dimmed(p).Render(marks))
		}
	}
	for _, tag := range t.Tags {
		parts = append(parts, dimmed(p).Render("#"+tag))
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

func (m *Model) viewHelp() string {
	var b strings.Builder
	b.WriteString(m.header() + "\n")

	section := func(title string, bindings []binding) {
		b.WriteString("\n" + gutter + styHeading.Render(title) + "\n\n")
		for _, k := range bindings {
			if k.help == "" {
				continue
			}
			b.WriteString(gutter + styKey.Render(pad(k.label(), 16)) + styDim.Render(k.help) + "\n")
		}
	}

	section("KEYS", listBindings)
	section("AND TWO-KEY", chordBindings)
	section("IN CAPTURE", captureBindings)
	section("ON THE WEEK", weekBindings)

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

// count renders "1 task" or "3 tasks".
func count(n int, one, many string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, one)
	}
	return fmt.Sprintf("%d %s", n, many)
}
