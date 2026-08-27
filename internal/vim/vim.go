// Package vim is the small set of normal-mode keys the capture boxes answer to.
//
// The behaviour cannot live here: an editor cannot ask another process what a
// keystroke means and still feel like an editor. But what the keys are is
// written down once, and both front ends are served this list -- so the two
// implementations, and the two reference sheets they show, cannot quietly
// disagree about what `dd` does or whether `e` exists.
//
// Each side has a test that it handles everything on this list.
package vim

// Key is one binding: what to press, and what it does.
type Key struct {
	Press string `json:"press"`
	Does  string `json:"does"`
}

// Group is a run of related keys, in the order the reference shows them.
type Group struct {
	Name string `json:"name"`
	Keys []Key  `json:"keys"`
}

// Reference is every key the capture boxes answer to in normal mode, and the
// two that get you there and back.
//
// Deliberately small. This is a box you paste notes into, not somewhere to live:
// enough to move about and fix a line without reaching for the mouse, and
// nothing that needs a manual.
func Reference() []Group {
	return []Group{
		{Name: "modes", Keys: []Key{
			{"esc", "stop typing"},
			{"i  a", "type again, here or after"},
			{"I  A", "type at the start or end of the line"},
			{"o  O", "open a line below or above"},
		}},
		{Name: "moving", Keys: []Key{
			{"h  j  k  l", "left, down, up, right"},
			{"0  $", "start and end of the line"},
			{"^", "first thing on the line"},
			{"gg  G", "top and bottom"},
		}},
		{Name: "words", Keys: []Key{
			{"w", "on to the next word"},
			{"b", "back a word"},
			{"e", "end of this word"},
		}},
		{Name: "changing", Keys: []Key{
			{"x", "delete a character"},
			{"dd", "delete the line"},
			{"D", "delete to the end of the line"},
			{"cc", "clear the line and type"},
			{"u", "undo"},
		}},
		{Name: "leaving", Keys: []Key{
			{"esc", "file what is there and close"},
			{"return", "the same, once you have stopped typing"},
			{"ZZ", "and the same"},
			{"ZQ", "scrap it and close"},
			{"?", "this list"},
		}},
	}
}

// Presses is every key on the reference, flattened -- what a front end has to
// be able to answer. The multi-key entries are split, because "h  j  k  l" is
// four bindings written economically.
func Presses() []string {
	var out []string
	for _, g := range Reference() {
		for _, k := range g.Keys {
			out = append(out, fields(k.Press)...)
		}
	}
	return out
}

// fields splits on runs of spaces without pulling in strings for one use.
func fields(s string) []string {
	var out []string
	start := -1
	for i := 0; i <= len(s); i++ {
		if i == len(s) || s[i] == ' ' {
			if start >= 0 {
				out = append(out, s[start:i])
				start = -1
			}
			continue
		}
		if start < 0 {
			start = i
		}
	}
	return out
}
