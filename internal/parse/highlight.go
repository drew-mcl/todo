package parse

import (
	"strings"
	"time"
)

// TokenKind labels a span of a shorthand line for display.
type TokenKind string

const (
	TokText  TokenKind = "text"
	TokPipe  TokenKind = "pipe"
	TokTopic TokenKind = "topic"
	TokDue   TokenKind = "due"
	TokWho   TokenKind = "who"
	TokPri   TokenKind = "pri"
	TokTag   TokenKind = "tag"
	TokNote  TokenKind = "note"
)

// Token is one highlighted span of raw input.
type Token struct {
	Kind TokenKind
	Text string
}

// Highlight breaks a raw shorthand line into display spans using the same rules
// the parser applies, so the colours in the capture box always agree with what
// actually gets stored.
func Highlight(raw string, now time.Time) []Token {
	// The same rewrite the parser applies, so a pasted bar is coloured as the
	// separator it is about to be read as rather than as plain text.
	raw = Normalise(raw)
	trimmed := strings.TrimSpace(raw)
	switch {
	case trimmed == "":
		return nil
	case strings.HasPrefix(trimmed, ">"):
		return []Token{{TokNote, raw}}
	case !strings.Contains(raw, "|"):
		return []Token{{TokText, raw}}
	}

	head, note, hasNote := strings.Cut(raw, " > ")

	// Work out which segment, if any, the parser will read as a date. Stripping
	// tokens never removes a pipe, so segment indices still line up with head.
	segs := strings.Split(head, "|")
	dateSeg := -1
	if probe := strings.Split(extractTokens(head, &Task{}), "|"); len(probe) >= 3 {
		if ParseDue(strings.TrimSpace(probe[len(probe)-1]), now).Recognised {
			dateSeg = len(probe) - 1
		}
	}

	var out []Token
	for i, seg := range segs {
		if i > 0 {
			out = append(out, Token{TokPipe, "|"})
		}
		plain := TokText
		if i == 0 {
			plain = TokTopic
		} else if i == dateSeg {
			plain = TokDue
		}
		out = append(out, highlightWords(seg, plain)...)
	}
	if hasNote {
		out = append(out, Token{TokNote, " > " + note})
	}
	return merge(out)
}

// highlightWords classifies each whitespace-delimited word, leaving the spacing
// between them untouched so the line renders exactly as typed.
func highlightWords(s string, plain TokenKind) []Token {
	type item struct {
		space bool
		text  string
		kind  TokenKind
	}
	var items []item
	for i := 0; i < len(s); {
		j := i
		for j < len(s) && (s[j] == ' ' || s[j] == '\t') {
			j++
		}
		if j > i {
			items = append(items, item{space: true, text: s[i:j]})
			i = j
		}
		for j < len(s) && s[j] != ' ' && s[j] != '\t' {
			j++
		}
		if j > i {
			items = append(items, item{text: s[i:j], kind: classify(s[i:j], plain)})
			i = j
		}
	}

	out := make([]Token, 0, len(items))
	for n, it := range items {
		kind := it.kind
		if it.space {
			// A gap between two words of the same kind belongs to that run, so
			// multi-word topics stay one span; anything else stays neutral.
			kind = TokText
			if n > 0 && n+1 < len(items) && items[n-1].kind == items[n+1].kind {
				kind = items[n-1].kind
			}
		}
		out = append(out, Token{kind, it.text})
	}
	return out
}

func classify(word string, plain TokenKind) TokenKind {
	bare := strings.TrimRight(word, ",;:.)")
	switch {
	case isToken(bare, '@'):
		return TokWho
	case isToken(bare, '#'):
		return TokTag
	case isBangs(bare):
		return TokPri
	}
	return plain
}

// merge joins neighbouring spans of the same kind so the markup stays compact.
func merge(in []Token) []Token {
	out := in[:0]
	for _, t := range in {
		if n := len(out); n > 0 && out[n-1].Kind == t.Kind {
			out[n-1].Text += t.Text
			continue
		}
		out = append(out, t)
	}
	return out
}
