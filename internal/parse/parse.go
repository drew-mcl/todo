// Package parse turns pasted meeting-note shorthand into tasks.
//
// The grammar is deliberately forgiving: anything it cannot read is reported as
// skipped rather than guessed at, so a whole blob of raw call notes can be pasted
// and only the action lines become tasks.
//
//	topic | task text [| due] [@assignee] [!priority] [#tags] [> note]
//	> continuation note line
package parse

import (
	"strings"
	"time"
)

// Priority ranks a task. No marks means Normal; each extra "!" escalates.
type Priority int

const (
	PriNormal Priority = iota
	PriHigh
	PriUrgent
	PriCritical
)

// Marks renders the priority back into the shorthand that produced it.
func (p Priority) Marks() string { return strings.Repeat("!", int(p)) }

func (p Priority) String() string {
	switch p {
	case PriHigh:
		return "High"
	case PriUrgent:
		return "Urgent"
	case PriCritical:
		return "Critical"
	default:
		return "Normal"
	}
}

// Kind classifies what a single input line turned into.
type Kind string

const (
	KindTask    Kind = "task"
	KindNote    Kind = "note"
	KindSkipped Kind = "skipped"
	KindBlank   Kind = "blank"
)

// DefaultTopic catches task lines written without one, so content is never lost.
const DefaultTopic = "inbox"

// dittoMarks repeat the topic of the task above.
//
// The documented form is to simply leave the topic off -- a line that starts
// with "|" can only have meant "same as the one above", so there is no symbol
// to remember and the pipes line up down the page. "^" is the explicit
// alternative for when you want it visible; the rest are silent aliases so no
// reasonable guess is wrong mid-call.
var dittoMarks = map[string]bool{
	`"`:  true, // the ditto mark itself
	`”`:  true, // what a phone or a word processor will actually insert
	`''`: true,
	`^`:  true,
	",,": true,
	"~":  true,
	"":   true,
}

// lookalikes are the characters that arrive instead of the ones the grammar
// wants. A separator that is not U+007C reads as no separator at all: the line
// is skipped, and the grammar looks broken when the only thing wrong is where
// the text was copied from.
//
// Rendered tables give box-drawing bars and phones give the fullwidth one. The
// spaces and the invisibles are here for the same reason: a no-break space is
// whitespace to the parser but not to the highlighter, which is how the colours
// in the capture box come to disagree with what is about to be stored, and a
// byte order mark on the front of a paste makes a second copy of a topic you
// already had -- the same word, in a different colour.
var lookalikes = strings.NewReplacer(
	"\uff5c", "|", // fullwidth vertical line
	"\u00a6", "|", // broken bar
	"\u2502", "|", // box drawings light vertical
	"\u2503", "|", // box drawings heavy vertical
	"\u2551", "|", // box drawings double vertical
	"\u2223", "|", // divides
	"\u2758", "|", // light vertical bar
	"\u23d0", "|", // vertical line extension
	"\u00a0", " ", // no-break space
	"\u2007", " ", // figure space
	"\u202f", " ", // narrow no-break space
	"\u200b", "", // zero-width space
	"\ufeff", "", // byte order mark
	"\r\n", "\n",
)

// Normalise rewrites the characters a paste brings with it into the ones the
// grammar reads. Everything that reads a line goes through it, so the parser
// and the highlighting cannot disagree about where the pipes are.
func Normalise(s string) string { return lookalikes.Replace(s) }

// Task is one parsed action item.
type Task struct {
	Topic    string
	Title    string
	Note     string
	Due      *time.Time // nil means anytime
	Assignee string     // empty means me
	Priority Priority
	Tags     []string
	Raw      string
	Warning  string // set when the line was salvaged rather than cleanly read
}

// Delegated reports whether the task is waiting on someone else.
func (t *Task) Delegated() bool { return t.Assignee != "" }

// Line records what happened to one input line, so the preview can show the user
// exactly how their text was read -- including the parts that were ignored.
type Line struct {
	N      int
	Raw    string
	Kind   Kind
	Text   string // note body for KindNote
	Reason string // why a line was skipped
	Task   *Task  // set for KindTask
	Owner  *Task  // set for KindNote: the task the note attached to
}

// Result is the parse of a whole paste.
type Result struct {
	Lines []Line
	Tasks []*Task
}

// Counts summarises a result for the preview header.
func (r *Result) Counts() (tasks, notes, skipped int) {
	for _, l := range r.Lines {
		switch l.Kind {
		case KindTask:
			tasks++
		case KindNote:
			notes++
		case KindSkipped:
			skipped++
		}
	}
	return
}

// Parse reads a whole pasted blob. now anchors every relative date token.
func Parse(input string, now time.Time) *Result {
	res := &Result{}
	var prev *Task       // the task a following "> note" line attaches to
	var lastTopic string // survives blank lines, so ditto still reaches back

	for i, raw := range strings.Split(Normalise(input), "\n") {
		line := Line{N: i + 1, Raw: raw}
		trimmed := strings.TrimSpace(raw)

		switch {
		case trimmed == "":
			line.Kind = KindBlank
			prev = nil

		case strings.HasPrefix(trimmed, ">"):
			body := strings.TrimSpace(strings.TrimPrefix(trimmed, ">"))
			if prev == nil {
				line.Kind, line.Reason = KindSkipped, "note with nothing above it"
				break
			}
			line.Kind, line.Text, line.Owner = KindNote, body, prev
			appendNote(prev, body)

		case !strings.Contains(trimmed, "|"):
			// An indented line under a task reads as context for it; anything else
			// is prose from the meeting and is left alone.
			if prev != nil && (strings.HasPrefix(raw, " ") || strings.HasPrefix(raw, "\t")) {
				line.Kind, line.Text, line.Owner = KindNote, trimmed, prev
				appendNote(prev, trimmed)
				break
			}
			line.Kind, line.Reason = KindSkipped, "no topic separator"

		default:
			t := parseTask(trimmed, now, lastTopic)
			line.Kind, line.Task = KindTask, t
			res.Tasks = append(res.Tasks, t)
			lastTopic, prev = t.Topic, t
		}
		res.Lines = append(res.Lines, line)
	}
	return res
}

func appendNote(t *Task, body string) {
	if body == "" {
		return
	}
	if t.Note == "" {
		t.Note = body
		return
	}
	t.Note += "\n" + body
}

// parseTask reads a single task line. Tokens are lifted out before the pipes are
// split, so @assignee, !priority and #tags work in any position.
func parseTask(line string, now time.Time, lastTopic string) *Task {
	t := &Task{Raw: line, Topic: DefaultTopic}

	body := line
	if head, note, found := strings.Cut(line, " > "); found {
		body, t.Note = head, strings.TrimSpace(note)
	}

	body = extractTokens(body, t)

	segs := strings.Split(body, "|")
	for i := range segs {
		segs[i] = strings.TrimSpace(segs[i])
	}

	titleSegs := segs[1:]
	// A trailing segment is a due date only if the whole of it reads as one;
	// otherwise it is just more of the task text.
	if len(segs) >= 3 {
		if due := ParseDue(segs[len(segs)-1], now); due.Recognised {
			if due.Explicit {
				d := due.Date
				t.Due = &d
			}
			titleSegs = segs[1 : len(segs)-1]
		}
	}

	switch topic := strings.ToLower(segs[0]); {
	case !dittoMarks[topic]:
		t.Topic = topic
	case lastTopic != "":
		// Repeat the topic above, so a run of items under one heading only has
		// to be named once.
		t.Topic = lastTopic
	case topic == "":
		t.Warning = "no topic given, filed under " + DefaultTopic
	default:
		t.Warning = "nothing above to repeat, filed under " + DefaultTopic
	}

	t.Title = strings.TrimSpace(strings.Join(titleSegs, " | "))
	if t.Title == "" {
		t.Title = segs[0]
		t.Topic = DefaultTopic
		t.Warning = "no task text, used the topic as the task"
	}
	return t
}

// extractTokens pulls @assignee, !priority and #tags out of s and returns what is
// left. Tokens must stand as whole whitespace-delimited words, which is what keeps
// email addresses, "urgent!" and issue numbers like #1234 out of the results.
func extractTokens(s string, t *Task) string {
	seen := map[string]bool{}
	kept := make([]string, 0, 8)

	for _, w := range strings.Fields(s) {
		bare := strings.TrimRight(w, ",;:.)")
		switch {
		case t.Assignee == "" && isToken(bare, '@'):
			t.Assignee = strings.ToLower(bare[1:])
			continue
		case isToken(bare, '#'):
			if tag := strings.ToLower(bare[1:]); !seen[tag] {
				seen[tag] = true
				t.Tags = append(t.Tags, tag)
			}
			continue
		case isBangs(bare):
			if p := Priority(min(len(bare), 3)); p > t.Priority {
				t.Priority = p
			}
			continue
		}
		kept = append(kept, w)
	}
	return strings.Join(kept, " ")
}

// isToken reports whether w is a sigil followed by a name starting with a letter.
// Requiring a letter is what stops "#1234" from becoming a tag.
func isToken(w string, sigil byte) bool {
	if len(w) < 2 || w[0] != sigil {
		return false
	}
	if c := w[1]; !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z') {
		return false
	}
	for _, c := range w[1:] {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' ||
			c == '_' || c == '-' || c == '.') {
			return false
		}
	}
	return true
}

// isBangs reports whether w is nothing but "!" marks, so "fix it!" is left alone.
func isBangs(w string) bool {
	if w == "" {
		return false
	}
	return strings.Trim(w, "!") == ""
}
