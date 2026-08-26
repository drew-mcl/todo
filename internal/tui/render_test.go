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
		_, cmd := m.Update(msg)
		// Commands here are synchronous reloads; run them so the view is current.
		for cmd != nil {
			out := cmd()
			if out == nil {
				break
			}
			_, cmd = m.Update(out)
		}
	}
	return m, m.View()
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

	m2, out := screen(t, "j", "x")
	text := plain(out)
	if strings.Contains(text, before) && m2.view == store.ViewToday {
		// Completing removes it from Today.
		t.Errorf("%q is still in Today after x:\n%s", before, text)
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
