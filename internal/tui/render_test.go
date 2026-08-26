package tui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/drew-mcl/todo/internal/parse"
	"github.com/drew-mcl/todo/internal/store"
)

var now = time.Date(2026, time.August, 26, 9, 0, 0, 0, time.UTC)

const seed = `prod issue | chase the vendor about the patch build | today @sam !!
> they have missed two dates now
           | write the postmortem | eow #board
admin | quarterly compliance training | today #compliance
personal | book the dentist
platform | decide on the ingest lag fix properly | today !!!`

// screen drives the model the way the terminal would and hands back what it drew.
func screen(t *testing.T, keys ...string) (*Model, string) {
	t.Helper()
	st, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	if _, err := st.CreateBatch(parse.Parse(seed, now).Tasks,
		store.Capture{Source: "test", Title: "Platform sync"}, now); err != nil {
		t.Fatalf("seed: %v", err)
	}

	m := New(st, func() time.Time { return now })
	m.Update(tea.WindowSizeMsg{Width: 92, Height: 28})
	if msg := m.reload(); msg != nil {
		m.Update(msg)
	}
	for _, k := range keys {
		var msg tea.Msg
		switch k {
		case "esc", "enter", "tab", "up", "down", "ctrl+s", " ":
			msg = tea.KeyMsg{Type: keyType(k)}
		default:
			msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)}
		}
		m.run(msg)
	}
	return m, m.View()
}

// run feeds a message in and follows every command it produces, the way the
// bubbletea runtime would. Batches have to be unwrapped by hand here: a test
// that skips them quietly never runs the reload inside.
func (m *Model) run(msg tea.Msg) {
	_, cmd := m.Update(msg)
	m.follow(cmd)
}

func (m *Model) follow(cmd tea.Cmd) { m.followN(cmd, 0) }

func (m *Model) followN(cmd tea.Cmd, depth int) {
	// Anything on a timer is left to the test. The animation tick reschedules
	// itself for as long as something is moving, which with a frozen clock is
	// forever; the linger would otherwise fire the instant it is scheduled and
	// hide the very state it exists to show.
	if cmd == nil || depth > 8 {
		return
	}
	out := cmd()
	switch v := out.(type) {
	case nil, tickMsg, settledMsg:
	case tea.BatchMsg:
		for _, c := range v {
			m.followN(c, depth+1)
		}
	default:
		_, next := m.Update(out)
		m.followN(next, depth+1)
	}
}

func keyType(k string) tea.KeyType {
	switch k {
	case "esc":
		return tea.KeyEsc
	case "enter":
		return tea.KeyEnter
	case "tab":
		return tea.KeyTab
	case "up":
		return tea.KeyUp
	case "down":
		return tea.KeyDown
	case "ctrl+s":
		return tea.KeyCtrlS
	case " ":
		return tea.KeySpace
	}
	return tea.KeyRunes
}

// plain strips the colour so the layout can be asserted on.
func plain(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == 0x1b {
			for i < len(s) && s[i] != 'm' {
				i++
			}
			continue
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

func TestListDraws(t *testing.T) {
	_, out := screen(t)
	text := plain(out)

	for _, want := range []string{
		"todo",
		"wed 26 august",
		"chase the vendor about the patch build",
		"prod issue",
		"they have missed two dates now", // the note travels with the task
		"today",
		"n capture",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("the list is missing %q:\n%s", want, text)
		}
	}
	// Today is exactly today, so the overdue view holds what slipped.
	if strings.Contains(text, "book the dentist") {
		t.Error("an undated task appeared in Today")
	}
	if lines := strings.Count(text, "\n"); lines > 28 {
		t.Errorf("drew %d lines into a 28-row terminal", lines)
	}
}

func TestCursorAndToggle(t *testing.T) {
	m, _ := screen(t, "j")
	if m.cursor != 1 {
		t.Fatalf("cursor = %d after j, want 1", m.cursor)
	}
	before := m.current().Title

	// Completing holds the row on screen, struck through, before it goes: the
	// point is that finishing something looks like finishing something.
	m2, out := screen(t, "j", "x")
	if !strings.Contains(plain(out), before) {
		t.Errorf("%q vanished instantly instead of being marked done:\n%s", before, plain(out))
	}
	if m2.leavingID == 0 {
		t.Error("nothing was marked as on its way out")
	}
	if got, err := m2.store.Get(m2.leavingID); err != nil || !got.Done() {
		t.Errorf("the task was not actually completed: %v", err)
	}

	// Once its moment is over, the list reloads without it.
	m2.run(settledMsg{})
	if strings.Contains(plain(m2.View()), before) {
		t.Errorf("%q is still in Today after it settled:\n%s", before, plain(m2.View()))
	}
}

// TestCompletionStopsAnimating guards the tick loop that drives the linger and
// the meter: it has to stop when nothing is moving.
func TestCompletionStopsAnimating(t *testing.T) {
	clock := now
	st, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	st.CreateBatch(parse.Parse(seed, now).Tasks, store.Capture{Source: "test"}, now)

	m := New(st, func() time.Time { return clock })
	m.Update(tea.WindowSizeMsg{Width: 92, Height: 28})
	if msg := m.reload(); msg != nil {
		m.Update(msg)
	}
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})

	if !m.animating() {
		t.Error("nothing is animating right after completing something")
	}
	clock = clock.Add(fade + time.Second)
	m.leavingID = 0
	if m.animating() {
		t.Error("still animating long after everything settled")
	}
}

// TestClearingTheDayIsSaidSo: the best thing that happens in here should not
// read the same as a day that never had anything on it.
func TestClearingTheDayIsSaidSo(t *testing.T) {
	st, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	st.CreateBatch(parse.Parse("admin | the only thing | today", now).Tasks,
		store.Capture{Source: "test"}, now)

	m := New(st, func() time.Time { return now })
	m.Update(tea.WindowSizeMsg{Width: 92, Height: 28})
	if msg := m.reload(); msg != nil {
		m.Update(msg)
	}
	m.run(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	m.run(settledMsg{})

	text := plain(m.View())
	if !strings.Contains(text, "that is today done") {
		t.Errorf("clearing the day says nothing:\n%s", text)
	}
	if strings.Contains(text, "nothing due today") {
		t.Errorf("an emptied day reads like an empty one:\n%s", text)
	}
}

// TestCaptureShowsTheParse is the reason the box exists: what you type becomes
// tasks in front of you, coloured with the same grammar the browser uses.
func TestCaptureShowsTheParse(t *testing.T) {
	st, _ := store.Open(":memory:")
	defer st.Close()
	m := New(st, func() time.Time { return now })
	m.Update(tea.WindowSizeMsg{Width: 92, Height: 30})
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})

	for _, r := range "admin | pull the numbers | eow @jo !!" {
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	text := plain(m.View())

	for _, want := range []string{
		"capture",
		"1 TASK",
		"pull the numbers", // the rendered task
		"admin",
		"fri", // eow, read and shown back
		"jo",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("the capture preview is missing %q:\n%s", want, text)
		}
	}
	if !strings.Contains(text, "⌃s add") {
		t.Error("the capture box does not say how to commit")
	}
}

func TestCaptureCommits(t *testing.T) {
	st, _ := store.Open(":memory:")
	defer st.Close()
	m := New(st, func() time.Time { return now })
	m.Update(tea.WindowSizeMsg{Width: 92, Height: 30})
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	for _, r := range "admin | a real task | today" {
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	if cmd != nil {
		if msg := cmd(); msg != nil {
			m.Update(msg)
		}
	}

	if m.mode != modeList {
		t.Error("capture did not close after committing")
	}
	tasks, _ := st.List(store.Query{View: store.ViewAll}, now)
	if len(tasks) != 1 || tasks[0].Title != "a real task" {
		t.Fatalf("stored %v", tasks)
	}
	if m.lastBatch == 0 {
		t.Error("the batch was not remembered, so u cannot undo it")
	}
}

// TestEditReopensTheLine covers the nicest thing about a terminal: you correct
// the shorthand you wrote, not a form of separate fields.
func TestEditReopensTheLine(t *testing.T) {
	m, _ := screen(t, "e")
	if m.mode != modeEdit {
		t.Fatal("e did not open the editor")
	}
	if !strings.Contains(m.edit.Value(), "chase the vendor") {
		t.Errorf("the editor was seeded with %q, not the line as typed", m.edit.Value())
	}
	if !strings.Contains(m.edit.Value(), "|") {
		t.Error("the editor should hold shorthand, not a rendered title")
	}
}

func TestViewsAndHelp(t *testing.T) {
	if m, _ := screen(t, "l"); m.view != store.ViewLogbook {
		t.Errorf("l went to %q", m.view)
	}
	if m, _ := screen(t, "a"); m.view != store.ViewAll {
		t.Errorf("a went to %q", m.view)
	}
	if m, _ := screen(t, "g", "o"); m.view != store.ViewOverdue {
		t.Errorf("g o went to %q", m.view)
	}
	_, out := screen(t, "?")
	if !strings.Contains(plain(out), "SHORTHAND") {
		t.Error("help does not explain the grammar")
	}
}

func TestNarrowTerminal(t *testing.T) {
	st, _ := store.Open(":memory:")
	defer st.Close()
	st.CreateBatch(parse.Parse(seed, now).Tasks, store.Capture{Source: "test"}, now)
	m := New(st, func() time.Time { return now })
	m.Update(tea.WindowSizeMsg{Width: 46, Height: 16})
	if msg := m.reload(); msg != nil {
		m.Update(msg)
	}

	for _, line := range strings.Split(plain(m.View()), "\n") {
		if len([]rune(line)) > 46 {
			t.Errorf("a line ran past a 46-column terminal (%d):\n%s", len([]rune(line)), line)
		}
	}
}

// TestScrollsWithTheCursor covers the thing a long list needs: the selection
// stays on screen and the status line never gets pushed off the bottom.
func TestScrollsWithTheCursor(t *testing.T) {
	st, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	var lines []string
	for i := range 40 {
		lines = append(lines, fmt.Sprintf("admin | task number %d | today", i))
	}
	if _, err := st.CreateBatch(parse.Parse(strings.Join(lines, "\n"), now).Tasks,
		store.Capture{Source: "test"}, now); err != nil {
		t.Fatal(err)
	}

	const rows = 24
	m := New(st, func() time.Time { return now })
	m.Update(tea.WindowSizeMsg{Width: 90, Height: rows})
	if msg := m.reload(); msg != nil {
		m.Update(msg)
	}

	for step := range 39 {
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
		out := plain(m.View())

		if got := strings.Count(out, "\n") + 1; got > rows {
			t.Fatalf("step %d drew %d lines into %d rows", step, got, rows)
		}
		// The status line must survive at the bottom.
		if !strings.Contains(out, "n capture") {
			t.Fatalf("step %d pushed the status line off screen:\n%s", step, out)
		}
		// And the selected task must be somewhere on it.
		want := m.current().Title
		if !strings.Contains(out, want) {
			t.Fatalf("step %d scrolled past the selection %q:\n%s", step, want, out)
		}
	}
}

// TestPreviewSettles covers the one moving part: a parsed line arrives dim and
// reaches full weight, rather than snapping into place mid-keystroke.
func TestPreviewSettles(t *testing.T) {
	st, _ := store.Open(":memory:")
	defer st.Close()

	clock := now
	m := New(st, func() time.Time { return clock })
	m.Update(tea.WindowSizeMsg{Width: 92, Height: 30})
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	for _, r := range "admin | pull the numbers | eow" {
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}

	first := m.View()
	if !strings.Contains(plain(first), "pull the numbers") {
		t.Fatal("the parsed line never appeared")
	}
	if !m.anim.running(clock) {
		t.Error("nothing is settling, so the line arrived fully formed")
	}
	// The dot lands last, so at birth it is still a hollow placeholder.
	if !strings.Contains(plain(first), hollow) {
		t.Error("the topic dot should arrive after the text, not with it")
	}

	clock = clock.Add(settle + time.Millisecond)
	settled := m.View()
	if m.anim.running(clock) {
		t.Error("the line never finished settling")
	}
	if first == settled {
		t.Error("the line looks identical settled and unsettled; nothing animated")
	}
	if !strings.Contains(plain(settled), bullet) {
		t.Error("the topic dot never arrived")
	}
}

// TestSettleStopsTicking guards the thing that would make this a bad idea:
// a tick loop that never stops.
func TestSettleStopsTicking(t *testing.T) {
	st, _ := store.Open(":memory:")
	defer st.Close()

	clock := now
	m := New(st, func() time.Time { return clock })
	m.Update(tea.WindowSizeMsg{Width: 92, Height: 30})
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	for _, r := range "admin | a task" {
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	m.View()

	if _, cmd := m.Update(tickMsg(clock)); cmd == nil {
		t.Error("stopped ticking while the line was still settling")
	}
	clock = clock.Add(settle * 2)
	m.View()
	if _, cmd := m.Update(tickMsg(clock)); cmd != nil {
		t.Error("still ticking after everything settled")
	}
}

// ── the week planner ────────────────────────────────────────────────────────

func weekScreen(t *testing.T, keys ...string) (*Model, string) {
	t.Helper()
	m, _ := screen(t)
	step := m.run
	step(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("w")})
	for _, k := range keys {
		if k == "esc" {
			step(tea.KeyMsg{Type: tea.KeyEsc})
			continue
		}
		step(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)})
	}
	return m, m.View()
}

func TestWeekDraws(t *testing.T) {
	m, _ := weekScreen(t)
	if m.mode != modeWeek {
		t.Fatal("w did not open the planner")
	}
	// Tall enough to hold the whole board; at 28 rows it correctly scrolls and
	// the far ends are off screen.
	m.Update(tea.WindowSizeMsg{Width: 92, Height: 60})
	text := strings.ToLower(plain(m.View()))

	// Seven days stacked, because seven columns do not fit a terminal.
	for _, want := range []string{"mon 24 aug", "wed 26 aug · today", "sun 30 aug", "unscheduled"} {
		if !strings.Contains(text, want) {
			t.Errorf("the planner is missing %q:\n%s", want, text)
		}
	}
	if !strings.Contains(text, "1-7 day") {
		t.Error("the planner does not say how to schedule")
	}
	for _, line := range strings.Split(text, "\n") {
		if len([]rune(line)) > 92 {
			t.Errorf("a line ran past the terminal:\n%s", line)
		}
	}
}

// TestWeekSchedulesByKey is the whole reason the planner works here: dragging
// has no equivalent, so a day is a keystroke.
func TestWeekSchedulesByKey(t *testing.T) {
	m, _ := weekScreen(t)

	// Walk to something in the unscheduled tray.
	var target *store.Task
	for i, task := range m.weekFlat() {
		if task.Due == nil {
			m.cursor, target = i, task
			break
		}
	}
	if target == nil {
		t.Fatal("nothing unscheduled to place")
	}

	// 5 is the fifth day of the week on screen: Friday 28 August.
	m.run(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("5")})

	got, err := m.store.Get(target.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Due == nil || got.Due.Format("2006-01-02") != "2026-08-28" {
		t.Fatalf("due = %v, want friday 2026-08-28", got.Due)
	}

	// 0 takes it back off the calendar.
	for i, task := range m.weekFlat() {
		if task.ID == target.ID {
			m.cursor = i
		}
	}
	m.run(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("0")})
	if got, _ := m.store.Get(target.ID); got.Due != nil {
		t.Errorf("due = %v, want it cleared", got.Due)
	}
}

func TestWeekNavigatesAndReturns(t *testing.T) {
	m, out := weekScreen(t, ">")
	if !strings.Contains(strings.ToLower(plain(out)), "31 aug") {
		t.Errorf("> did not move to the following week:\n%s", plain(out))
	}
	m, _ = weekScreen(t, ">", ".")
	if !m.weekStart.Equal(store.WeekStart(now)) {
		t.Error(". did not come back to this week")
	}
	m, _ = weekScreen(t, "esc")
	if m.mode != modeList {
		t.Error("esc did not return to the list")
	}
}

// TestCaptureReturnsWhereItCameFrom: n means capture everywhere, and closing it
// should put you back on the board you were planning, not somewhere else.
func TestCaptureReturnsWhereItCameFrom(t *testing.T) {
	m, _ := weekScreen(t, "n")
	if m.mode != modeCapture {
		t.Fatal("n did not open capture from the planner")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if m.mode != modeWeek {
		t.Errorf("esc went to %v, want back to the planner", m.mode)
	}

	m2, _ := screen(t, "n")
	m2.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if m2.mode != modeList {
		t.Errorf("esc from the list went to %v, want the list", m2.mode)
	}
}

// TestSearchNarrowsAsYouType: waiting for enter makes a list feel like a form.
func TestSearchNarrowsAsYouType(t *testing.T) {
	m, _ := screen(t, "a") // everything, so there is something to narrow
	step := m.run

	before := len(m.flat)
	step(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	for _, r := range "dentist" {
		step(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}

	if len(m.flat) >= before {
		t.Errorf("the list did not narrow while typing: %d then %d", before, len(m.flat))
	}
	if len(m.flat) != 1 || !strings.Contains(m.flat[0].Title, "dentist") {
		t.Errorf("narrowed to %v", m.flat)
	}

	// Escape puts it all back.
	step(tea.KeyMsg{Type: tea.KeyEsc})
	if len(m.flat) != before {
		t.Errorf("esc left %d of %d", len(m.flat), before)
	}
}
