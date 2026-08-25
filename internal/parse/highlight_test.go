package parse

import (
	"strings"
	"testing"
)

// render collapses tokens into a compact form so expectations stay readable.
func render(toks []Token) string {
	var b strings.Builder
	for _, t := range toks {
		if t.Kind == TokText {
			b.WriteString(t.Text)
			continue
		}
		b.WriteString("[" + string(t.Kind) + ":" + t.Text + "]")
	}
	return b.String()
}

func TestHighlight(t *testing.T) {
	tests := []struct{ in, want string }{
		{
			"prod issue | find out why | today @sam !!",
			"[topic:prod issue] [pipe:|] find out why [pipe:|] [due:today] [who:@sam] [pri:!!]",
		},
		{
			// Only one pipe, so nothing is a date and it all stays task text.
			"some tool | update the value @jo",
			"[topic:some tool] [pipe:|] update the value [who:@jo]",
		},
		{
			// The trailing segment is not a date, so it must not colour as one.
			"some tool | update the a | b toggle",
			"[topic:some tool] [pipe:|] update the a [pipe:|] b toggle",
		},
		{
			"admin | training | eow #compliance > covers new starters",
			"[topic:admin] [pipe:|] training [pipe:|] [due:eow] [tag:#compliance][note: > covers new starters]",
		},
		{"> a continuation line", "[note:> a continuation line]"},
		{"just some prose from the call", "just some prose from the call"},
		{"admin | chase bob@corp.com", "[topic:admin] [pipe:|] chase bob@corp.com"},
	}
	for _, tc := range tests {
		if got := render(Highlight(tc.in, now)); got != tc.want {
			t.Errorf("Highlight(%q) =\n  %s\nwant\n  %s", tc.in, got, tc.want)
		}
	}
}

// TestHighlightIsLossless guards the property that matters: the spans always
// reassemble into the exact line the user typed.
func TestHighlightIsLossless(t *testing.T) {
	for _, in := range []string{
		"prod issue | find out why | today @sam !!",
		"  admin  |  spaced   out  |  eow  ",
		"> note",
		"prose",
		"a|b|c",
	} {
		var b strings.Builder
		for _, tok := range Highlight(in, now) {
			b.WriteString(tok.Text)
		}
		if got := b.String(); got != in {
			t.Errorf("Highlight(%q) reassembled to %q", in, got)
		}
	}
}
