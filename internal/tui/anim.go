package tui

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/drew-mcl/todo/internal/store"
)

// The capture box re-reads the draft on every keystroke, which means a finished
// line resolves into a task the instant you type the last character of its
// date. Arriving fully formed reads as a flicker; arriving over a fifth of a
// second reads as the line becoming something.
//
// So each parsed line remembers when its content first appeared, and is drawn
// dimmer than final until it has settled. Nothing moves -- only the weight of
// the ink changes, which is the most a terminal can do tastefully and, as it
// happens, all this needs.

const (
	settle   = 220 * time.Millisecond
	tickRate = 45 * time.Millisecond
)

type tickMsg time.Time

func tick() tea.Cmd {
	return tea.Tick(tickRate, func(t time.Time) tea.Msg { return tickMsg(t) })
}

// anim remembers when each line last changed. Keyed by content, so a line that
// is untouched while you type below it does not restart.
type anim struct {
	born map[string]time.Time
	seen map[string]bool
}

func newAnim() *anim {
	return &anim{born: map[string]time.Time{}, seen: map[string]bool{}}
}

// note records that a line is on screen and returns how settled it is, from 0
// (just appeared) to 1 (finished).
func (a *anim) note(key string, now time.Time) float64 {
	a.seen[key] = true
	born, ok := a.born[key]
	if !ok {
		a.born[key] = now
		return 0
	}
	if d := now.Sub(born); d < settle {
		return float64(d) / float64(settle)
	}
	return 1
}

// sweep forgets lines that are no longer on screen, so an edited line animates
// again rather than reappearing fully formed.
func (a *anim) sweep() {
	for k := range a.born {
		if !a.seen[k] {
			delete(a.born, k)
		}
	}
	a.seen = map[string]bool{}
}

// running reports whether anything is still settling, and therefore whether the
// tick loop needs to keep going. Idle typing costs nothing.
func (a *anim) running(now time.Time) bool {
	for _, born := range a.born {
		if now.Sub(born) < settle {
			return true
		}
	}
	return false
}

// ramp is the ink a line is drawn in on its way to full weight.
func ramp(p float64) lipgloss.Style {
	switch {
	case p < 0.35:
		return styFaint
	case p < 0.7:
		return styDim
	default:
		return styTitle
	}
}

// dimmed is the same ramp for text that ends up dim rather than full.
func dimmed(p float64) lipgloss.Style {
	if p < 0.5 {
		return styFaint
	}
	return styDim
}

// visible reports whether a decoration has arrived yet. The topic dot lands
// last, which is what makes the line look assembled rather than faded.
func visible(p float64) bool { return p > 0.55 }

// linger is how long a completed task stays on screen, struck through, before
// it folds away. Long enough to register, short enough not to be in the way.
const linger = 650 * time.Millisecond

// fade is how long a message sits in the status bar before it goes quiet.
const fade = 6 * time.Second

// meterFill is how long the day meter takes to travel.
const meterFill = 450 * time.Millisecond

// ease is the same curve everything else moves on.
func ease(p float64) float64 {
	if p >= 1 {
		return 1
	}
	if p <= 0 {
		return 0
	}
	q := 1 - p
	return 1 - q*q*q
}

// meterNow is where the day meter has travelled to. It eases from wherever it
// was when the count last changed towards where it is going.
func (m *Model) meterNow() float64 {
	target := m.meterTarget()
	if m.meterAt.IsZero() {
		return target
	}
	p := ease(float64(m.now().Sub(m.meterAt)) / float64(meterFill))
	return m.meter + (target-m.meter)*p
}

func (m *Model) meterTarget() float64 {
	total := m.counts[store.ViewToday] + m.doneToday
	if total == 0 {
		return 0
	}
	return float64(m.doneToday) / float64(total)
}

// animating reports whether anything on screen is still moving, and so whether
// the tick loop needs to keep running. Nothing moving costs nothing.
func (m *Model) animating() bool {
	now := m.now()
	if m.mode == modeCapture && m.anim.running(now) {
		return true
	}
	if m.leavingID != 0 && now.Sub(m.leavingAt) < linger {
		return true
	}
	if !m.meterAt.IsZero() && now.Sub(m.meterAt) < meterFill {
		return true
	}
	if m.flash != "" && now.Sub(m.flashAt) < fade {
		return true
	}
	return false
}

// message is what the status bar has to say, if anything, having gone quiet
// once it has been read.
func (m *Model) message() (string, bool) {
	if m.problem != "" {
		return m.problem, false
	}
	if m.flash != "" && m.now().Sub(m.flashAt) < fade {
		return m.flash, true
	}
	return "", true
}
