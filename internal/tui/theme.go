package tui

import (
	"github.com/charmbracelet/lipgloss"
	"github.com/drew-mcl/todo/internal/palette"
	"github.com/drew-mcl/todo/internal/parse"
)

// The same system as the browser, in a terminal. Colour is nearly absent so
// that the little that remains means something: red is lateness and priority,
// the accent is today and focus, and the topic dots carry the rest.

func adaptive(light, dark string) lipgloss.AdaptiveColor {
	return lipgloss.AdaptiveColor{Light: light, Dark: dark}
}

var (
	cInk    = adaptive(palette.InkLight, palette.InkDark)
	cInk2   = adaptive(palette.Ink2Light, palette.Ink2Dark)
	cInk3   = adaptive(palette.Ink3Light, palette.Ink3Dark)
	cInk4   = adaptive(palette.Ink4Light, palette.Ink4Dark)
	cLine   = adaptive(palette.LineLight, palette.LineDark)
	cAccent = adaptive(palette.AccentLight, palette.AccentDark)
	cDanger = adaptive(palette.DangerLight, palette.DangerDark)
)

var (
	styBrand   = lipgloss.NewStyle().Foreground(cInk).Bold(true)
	styDim     = lipgloss.NewStyle().Foreground(cInk3)
	styFaint   = lipgloss.NewStyle().Foreground(cInk4)
	styTitle   = lipgloss.NewStyle().Foreground(cInk)
	styDone    = lipgloss.NewStyle().Foreground(cInk4).Strikethrough(true)
	styAccent  = lipgloss.NewStyle().Foreground(cAccent)
	styDanger  = lipgloss.NewStyle().Foreground(cDanger)
	styRule    = lipgloss.NewStyle().Foreground(cLine)
	styNote    = lipgloss.NewStyle().Foreground(cInk3).Italic(true)
	styHeading = lipgloss.NewStyle().Foreground(cInk4).Bold(true)
	styCursor  = lipgloss.NewStyle().Foreground(cAccent)
	styKey     = lipgloss.NewStyle().Foreground(cInk2).Bold(true)
	styErr     = lipgloss.NewStyle().Foreground(cDanger)
)

// hueStyle colours a topic's dot. The assignment is worked out once per reload
// from the whole set of topics, so no two on screen share a colour.
func hueStyle(hue int) lipgloss.Style {
	return lipgloss.NewStyle().Foreground(adaptive(palette.TopicLight[hue], palette.TopicDark[hue]))
}

// tokenStyle maps a highlighted shorthand token to its colour. The capture
// preview is the one place the full palette is spent, because there it is
// teaching the grammar rather than decorating a list.
func tokenStyle(k parse.TokenKind) lipgloss.Style {
	switch k {
	case parse.TokTopic:
		return lipgloss.NewStyle().Foreground(cInk).Bold(true)
	case parse.TokPipe:
		return styFaint
	case parse.TokDue:
		return styAccent
	case parse.TokWho:
		return hueStyle(0)
	case parse.TokPri:
		return lipgloss.NewStyle().Foreground(cDanger).Bold(true)
	case parse.TokTag:
		return hueStyle(2)
	case parse.TokNote:
		return styNote
	default:
		return styDim
	}
}
