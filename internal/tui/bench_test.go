package tui

// What a frame costs.
//
// Both screens used to draw every row and then throw away the ones that did not
// fit, so a long list -- or a long paste -- cost a page of colouring to show a
// screenful, on every frame. These are how that was found and how it stays
// found:
//
//	go test ./internal/tui -run xxx -bench . -benchtime 40x

import (
	"fmt"
	"github.com/drew-mcl/todo/internal/parse"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/drew-mcl/todo/internal/store"
)

func parseDraft(s string) []*parse.Task { return parse.Parse(s, now).Tasks }

func bigDraft(lines int) string {
	var b strings.Builder
	for i := range lines {
		fmt.Fprintf(&b, "topic %d | something that needs doing about it | today @sam !! #tag\n", i%12)
	}
	return b.String()
}

func benchCapture(b *testing.B, lines int) {
	st, _ := store.Open(":memory:")
	b.Cleanup(func() { st.Close() })
	m := New(st, func() time.Time { return now })
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(bigDraft(lines)), Paste: true})

	b.ResetTimer()
	for b.Loop() {
		_ = m.View()
	}
}

func BenchmarkCaptureView20(b *testing.B)   { benchCapture(b, 20) }
func BenchmarkCaptureView200(b *testing.B)  { benchCapture(b, 200) }
func BenchmarkCaptureView1000(b *testing.B) { benchCapture(b, 1000) }

// A keystroke: the parse plus the redraw, which is what typing actually costs.
func BenchmarkCaptureKeystroke200(b *testing.B) {
	st, _ := store.Open(":memory:")
	b.Cleanup(func() { st.Close() })
	m := New(st, func() time.Time { return now })
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(bigDraft(200)), Paste: true})

	b.ResetTimer()
	for b.Loop() {
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
		_ = m.View()
	}
}

func benchList(b *testing.B, tasks int) {
	st, _ := store.Open(":memory:")
	b.Cleanup(func() { st.Close() })
	var draft strings.Builder
	for i := range tasks {
		fmt.Fprintf(&draft, "topic %d | something that needs doing | today @sam !! #tag\n", i%12)
	}
	if _, err := st.CreateBatch(parseDraft(draft.String()), store.Capture{Source: "bench"}, now); err != nil {
		b.Fatal(err)
	}
	m := New(st, func() time.Time { return now })
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	m.follow(m.Init())

	b.ResetTimer()
	for b.Loop() {
		_ = m.View()
	}
}

func BenchmarkListView50(b *testing.B)  { benchList(b, 50) }
func BenchmarkListView500(b *testing.B) { benchList(b, 500) }

func BenchmarkReload500(b *testing.B) {
	st, _ := store.Open(":memory:")
	b.Cleanup(func() { st.Close() })
	var draft strings.Builder
	for i := range 500 {
		fmt.Fprintf(&draft, "topic %d | something that needs doing | today @sam !! #tag\n", i%12)
	}
	st.CreateBatch(parseDraft(draft.String()), store.Capture{Source: "bench"}, now)
	m := New(st, func() time.Time { return now })
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})

	b.ResetTimer()
	for b.Loop() {
		m.reload()
	}
}

func BenchmarkParse200(b *testing.B) {
	draft := bigDraft(200)
	b.ResetTimer()
	for b.Loop() {
		parse.Parse(draft, now)
	}
}

func BenchmarkParse1000(b *testing.B) {
	draft := bigDraft(1000)
	b.ResetTimer()
	for b.Loop() {
		parse.Parse(draft, now)
	}
}

func BenchmarkWeekView(b *testing.B) {
	st, _ := store.Open(":memory:")
	b.Cleanup(func() { st.Close() })
	var draft strings.Builder
	for i := range 400 {
		fmt.Fprintf(&draft, "topic %d | something that needs doing | today @sam\n", i%12)
	}
	st.CreateBatch(parseDraft(draft.String()), store.Capture{Source: "bench"}, now)
	m := New(st, func() time.Time { return now })
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	m.follow(m.Init())
	m.run(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("w")})

	b.ResetTimer()
	for b.Loop() {
		_ = m.View()
	}
}
