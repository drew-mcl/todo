package parse

import (
	"strings"
	"testing"
	"time"
)

// Tuesday 25 August 2026, so weekday and end-of-week maths is checkable by hand.
var now = time.Date(2026, time.August, 25, 14, 30, 0, 0, time.UTC)

func ymd(t *testing.T, s string) *time.Time {
	t.Helper()
	d, err := time.ParseInLocation("2006-01-02", s, time.UTC)
	if err != nil {
		t.Fatalf("bad date literal %q: %v", s, err)
	}
	return &d
}

func one(t *testing.T, line string) *Task {
	t.Helper()
	r := Parse(line, now)
	if len(r.Tasks) != 1 {
		t.Fatalf("Parse(%q) produced %d tasks, want 1", line, len(r.Tasks))
	}
	return r.Tasks[0]
}

func dueStr(d *time.Time) string {
	if d == nil {
		return "<none>"
	}
	return d.Format("2006-01-02")
}

// TestBriefExamples pins the exact lines from the original brief, including the
// one-pipe form that has no due date.
func TestBriefExamples(t *testing.T) {
	tests := []struct {
		line     string
		topic    string
		title    string
		due      string
		assignee string
	}{
		{
			"personal | create todo system to better organize self | today",
			"personal", "create todo system to better organize self", "2026-08-25", "",
		},
		{
			"admin | quaterly training | end of week",
			"admin", "quaterly training", "2026-08-28", "",
		},
		{
			"some tool | need to update to show correct value @someone",
			"some tool", "need to update to show correct value", "<none>", "someone",
		},
		{
			"prod issue | somethin went wrong find out why | today @someone",
			"prod issue", "somethin went wrong find out why", "2026-08-25", "someone",
		},
	}
	for _, tc := range tests {
		got := one(t, tc.line)
		if got.Topic != tc.topic || got.Title != tc.title ||
			dueStr(got.Due) != tc.due || got.Assignee != tc.assignee {
			t.Errorf("Parse(%q) =\n  topic=%q title=%q due=%s assignee=%q\nwant\n  topic=%q title=%q due=%s assignee=%q",
				tc.line, got.Topic, got.Title, dueStr(got.Due), got.Assignee,
				tc.topic, tc.title, tc.due, tc.assignee)
		}
	}
}

func TestTokensInAnyPosition(t *testing.T) {
	want := func(tk *Task) bool {
		return tk.Topic == "prod issue" && tk.Title == "find out why alerts fired" &&
			tk.Assignee == "sam" && tk.Priority == PriUrgent && dueStr(tk.Due) == "2026-08-25"
	}
	for _, line := range []string{
		"prod issue | find out why alerts fired | today @sam !!",
		"prod issue | @sam find out why alerts fired | today !!",
		"prod issue | find out why alerts fired !! | today @sam",
		"prod issue !! | find out why alerts fired @sam | today",
	} {
		if got := one(t, line); !want(got) {
			t.Errorf("Parse(%q) = topic=%q title=%q who=%q pri=%d due=%s; tokens not position-independent",
				line, got.Topic, got.Title, got.Assignee, got.Priority, dueStr(got.Due))
		}
	}
}

func TestTokenGuards(t *testing.T) {
	tests := []struct {
		name, line, title string
		assignee          string
		tags              []string
		pri               Priority
	}{
		{"email is not an assignee", "admin | chase bob@corp.com for the form",
			"chase bob@corp.com for the form", "", nil, PriNormal},
		{"issue number is not a tag", "admin | look at ticket #1234",
			"look at ticket #1234", "", nil, PriNormal},
		{"trailing bang is not a priority", "admin | ship it!",
			"ship it!", "", nil, PriNormal},
		{"standalone bangs are a priority", "admin | ship it !!!",
			"ship it", "", nil, PriCritical},
		{"more than three bangs caps out", "admin | ship it !!!!!",
			"ship it", "", nil, PriCritical},
		{"tags are collected and lowercased", "admin | training #Compliance #q3",
			"training", "", []string{"compliance", "q3"}, PriNormal},
		{"trailing punctuation is trimmed off tokens", "admin | ask @sam, then wait",
			"ask then wait", "sam", nil, PriNormal},
		{"dotted handles survive", "admin | ask @sam.smith",
			"ask", "sam.smith", nil, PriNormal},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := one(t, tc.line)
			if got.Title != tc.title {
				t.Errorf("title = %q, want %q", got.Title, tc.title)
			}
			if got.Assignee != tc.assignee {
				t.Errorf("assignee = %q, want %q", got.Assignee, tc.assignee)
			}
			if got.Priority != tc.pri {
				t.Errorf("priority = %v, want %v", got.Priority, tc.pri)
			}
			if strings.Join(got.Tags, ",") != strings.Join(tc.tags, ",") {
				t.Errorf("tags = %v, want %v", got.Tags, tc.tags)
			}
		})
	}
}

func TestStrayPipeStaysInTitle(t *testing.T) {
	got := one(t, "some tool | update the a | b toggle")
	if got.Title != "update the a | b toggle" {
		t.Errorf("title = %q, want the pipe kept as literal text", got.Title)
	}
	if got.Due != nil {
		t.Errorf("due = %s, want none", dueStr(got.Due))
	}
}

func TestTwoSegmentDateStaysTitle(t *testing.T) {
	// Minimal form is topic|title, so a lone second segment is never a date.
	if got := one(t, "admin | eow"); got.Title != "eow" || got.Due != nil {
		t.Errorf("got title=%q due=%s, want title=%q and no due", got.Title, dueStr(got.Due), "eow")
	}
}

func TestNotes(t *testing.T) {
	in := `prod issue | find out why alerts fired | today @sam > 3am spike, check ingest lag
> asked sam to pull the dashboard
> needs a real fix not a restart`

	r := Parse(in, now)
	if len(r.Tasks) != 1 {
		t.Fatalf("got %d tasks, want 1", len(r.Tasks))
	}
	want := "3am spike, check ingest lag\nasked sam to pull the dashboard\nneeds a real fix not a restart"
	if r.Tasks[0].Note != want {
		t.Errorf("note =\n%q\nwant\n%q", r.Tasks[0].Note, want)
	}
	if r.Tasks[0].Title != "find out why alerts fired" {
		t.Errorf("title = %q, note leaked into it", r.Tasks[0].Title)
	}
}

func TestIndentedLineBecomesNote(t *testing.T) {
	r := Parse("admin | quarterly training\n    covers the new starter policy", now)
	if len(r.Tasks) != 1 || r.Tasks[0].Note != "covers the new starter policy" {
		t.Errorf("indented line did not attach as a note: %+v", r.Tasks[0])
	}
}

func TestOrphanNoteIsSkipped(t *testing.T) {
	r := Parse("> nothing to attach to", now)
	if len(r.Tasks) != 0 || r.Lines[0].Kind != KindSkipped {
		t.Errorf("orphan note = %v, want skipped with no task", r.Lines[0].Kind)
	}
}

// TestRawBlobPaste is the behaviour the whole tool rests on: a messy paste yields
// only the action lines.
func TestRawBlobPaste(t *testing.T) {
	in := `Weekly sync 25 Aug
attendees: sam, jo, me

discussed the roadmap, finance want numbers by Q3
prod issue | find out why alerts fired | today @sam !!
> 3am spike
admin | quarterly training | eow #compliance

some tool | need to update to show correct value @jo
that's all folks`

	r := Parse(in, now)
	if len(r.Tasks) != 3 {
		t.Fatalf("got %d tasks, want 3", len(r.Tasks))
	}
	tasks, notes, skipped := r.Counts()
	if tasks != 3 || notes != 1 || skipped != 4 {
		t.Errorf("counts = %d tasks, %d notes, %d skipped; want 3, 1, 4", tasks, notes, skipped)
	}
	for _, l := range r.Lines {
		if l.Kind == KindSkipped && l.Reason == "" {
			t.Errorf("line %d skipped without a reason to show the user", l.N)
		}
	}
}

func TestSalvagedLines(t *testing.T) {
	t.Run("missing topic", func(t *testing.T) {
		got := one(t, "| write the postmortem | today")
		if got.Topic != DefaultTopic || got.Title != "write the postmortem" || got.Warning == "" {
			t.Errorf("got topic=%q title=%q warning=%q; want it filed under %s with a warning",
				got.Topic, got.Title, got.Warning, DefaultTopic)
		}
	})
	t.Run("missing title", func(t *testing.T) {
		got := one(t, "call the bank |")
		if got.Title != "call the bank" || got.Topic != DefaultTopic || got.Warning == "" {
			t.Errorf("got topic=%q title=%q warning=%q; want the text kept as the task",
				got.Topic, got.Title, got.Warning)
		}
	})
}

func TestExplicitNoDate(t *testing.T) {
	got := one(t, "admin | tidy the shared drive | someday")
	if got.Due != nil || got.Title != "tidy the shared drive" {
		t.Errorf("got title=%q due=%s, want the token consumed and no date", got.Title, dueStr(got.Due))
	}
}

func TestDueTokens(t *testing.T) {
	tests := []struct{ token, want string }{
		{"today", "2026-08-25"},
		{"tod", "2026-08-25"},
		{"tomorrow", "2026-08-26"},
		{"tmr", "2026-08-26"},
		{"eow", "2026-08-28"},
		{"end of week", "2026-08-28"},
		{"eom", "2026-08-31"},
		{"eoy", "2026-12-31"},
		{"next week", "2026-08-31"},
		{"tue", "2026-08-25"}, // today counts
		{"mon", "2026-08-31"}, // next occurrence
		{"friday", "2026-08-28"},
		{"next fri", "2026-09-04"}, // the Friday of next week
		{"next tue", "2026-09-01"},
		{"+3d", "2026-08-28"},
		{"+2w", "2026-09-08"},
		{"+1m", "2026-09-25"},
		{"2026-12-25", "2026-12-25"},
		{"25/12", "2026-12-25"},
		{"1/9/2027", "2027-09-01"},
		{"25 dec", "2026-12-25"},
		{"dec 25", "2026-12-25"},
		{"3rd sep", "2026-09-03"},
		{"1 jan", "2027-01-01"}, // already past this year, so next year
	}
	for _, tc := range tests {
		got := ParseDue(tc.token, now)
		if !got.Recognised || !got.Explicit {
			t.Errorf("ParseDue(%q) was not recognised as a date", tc.token)
			continue
		}
		if g := got.Date.Format("2006-01-02"); g != tc.want {
			t.Errorf("ParseDue(%q) = %s, want %s", tc.token, g, tc.want)
		}
	}
}

func TestNonDatesRejected(t *testing.T) {
	for _, s := range []string{
		"ship the thing", "review", "32/13", "31/02", "", "the end of week report", "+xd",
	} {
		if got := ParseDue(s, now); got.Recognised {
			t.Errorf("ParseDue(%q) was read as a date (%s), want rejected",
				s, got.Date.Format("2006-01-02"))
		}
	}
}

func TestFormatDue(t *testing.T) {
	tests := []struct{ date, want string }{
		{"2026-08-25", "Today"},
		{"2026-08-26", "Tomorrow"},
		{"2026-08-24", "Yesterday"},
		{"2026-08-20", "5 days overdue"},
		{"2026-08-28", "Fri"},
		{"2026-09-15", "Tue 15 Sep"},
		{"2027-01-04", "4 Jan 2027"},
	}
	for _, tc := range tests {
		if got := FormatDue(*ymd(t, tc.date), now); got != tc.want {
			t.Errorf("FormatDue(%s) = %q, want %q", tc.date, got, tc.want)
		}
	}
}

// TestDittoRepeatsTopic covers the ledger's own notation for "same as above",
// so a run of items under one heading is only named once.
func TestDittoRepeatsTopic(t *testing.T) {
	in := `prod issue | find out why the alerts fired | today @sam
" | write the postmortem | eow
^ | brief the team on friday
,, | chase the vendor
| and one more with nothing at all`

	r := Parse(in, now)
	if len(r.Tasks) != 5 {
		t.Fatalf("got %d tasks, want 5", len(r.Tasks))
	}
	for i, got := range r.Tasks {
		if got.Topic != "prod issue" {
			t.Errorf("task %d (%q) has topic %q, want it repeated from above",
				i, got.Title, got.Topic)
		}
		if got.Warning != "" {
			t.Errorf("task %d warned %q; repeating a topic is not a salvage", i, got.Warning)
		}
	}
	if r.Tasks[1].Title != "write the postmortem" {
		t.Errorf("title = %q, the mark leaked into it", r.Tasks[1].Title)
	}
	if due := dueStr(r.Tasks[1].Due); due != "2026-08-28" {
		t.Errorf("due = %s, want the rest of the line still parsed", due)
	}
}

func TestDittoReachesAcrossBlankLines(t *testing.T) {
	r := Parse("admin | first thing\n\n\" | second thing", now)
	if len(r.Tasks) != 2 || r.Tasks[1].Topic != "admin" {
		t.Errorf("a blank line broke the repeat: %+v", r.Tasks[1])
	}
}

func TestDittoWithNothingAbove(t *testing.T) {
	got := one(t, `" | nothing came before this`)
	if got.Topic != DefaultTopic || got.Warning == "" {
		t.Errorf("got topic=%q warning=%q; want it filed under %s with a warning",
			got.Topic, got.Warning, DefaultTopic)
	}
}

func TestDittoDoesNotSwallowRealTopics(t *testing.T) {
	r := Parse("admin | first\nplatform | second", now)
	if r.Tasks[1].Topic != "platform" {
		t.Errorf("topic = %q, want the explicit one to win", r.Tasks[1].Topic)
	}
}

// A line copied out of a rendered table, a chat client or a word processor
// brings look-alike characters with it. A separator that is not U+007C reads as
// no separator at all: the line is skipped, and the grammar looks broken when
// the only thing wrong is where the text came from.
func TestPasteBringsLookalikes(t *testing.T) {
	cases := []struct{ name, line string }{
		{"fullwidth bar", "prod issue ｜ chase the vendor ｜ today"},
		{"broken bar", "prod issue ¦ chase the vendor ¦ today"},
		{"box drawing bar", "prod issue │ chase the vendor │ today"},
		{"heavy box bar", "prod issue ┃ chase the vendor ┃ today"},
		{"double box bar", "prod issue ║ chase the vendor ║ today"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			res := Parse(c.line, now)
			if len(res.Tasks) != 1 {
				t.Fatalf("%q was not read as a task: %s", c.line, res.Lines[0].Reason)
			}
			got := res.Tasks[0]
			if got.Topic != "prod issue" || got.Title != "chase the vendor" {
				t.Errorf("read as topic %q title %q", got.Topic, got.Title)
			}
			if got.Due == nil {
				t.Error("the date in the last segment was not read")
			}
		})
	}
}

// A byte order mark on the front of a paste is invisible, and used to make a
// second copy of a topic you already had -- same word, different colour.
func TestPasteBringsInvisibleCharacters(t *testing.T) {
	got := Parse("\ufeffprod issue | chase the vendor @sam", now)
	if len(got.Tasks) != 1 {
		t.Fatal("the line was not read as a task")
	}
	if got.Tasks[0].Topic != "prod issue" {
		t.Errorf("a byte order mark ended up in the topic: %q", got.Tasks[0].Topic)
	}
	if got.Tasks[0].Assignee != "sam" {
		t.Errorf("assignee is %q", got.Tasks[0].Assignee)
	}
}

// The highlighter splits on plain spaces only, so a no-break space in front of
// a token used to colour it as part of the word before it -- the capture box
// disagreeing with what was about to be stored.
func TestNoBreakSpacesColourTheSame(t *testing.T) {
	var who int
	for _, tok := range Highlight("prod issue | chase the vendor @sam", now) {
		if tok.Kind == TokWho {
			who++
		}
	}
	if who != 1 {
		t.Error("the assignee behind a no-break space was not coloured as one")
	}
}

// The highlighting has to agree with the parse about where the pipes are, or a
// rewritten line would be coloured as if it were prose.
func TestHighlightSeesTheSameBars(t *testing.T) {
	var kinds []TokenKind
	for _, tok := range Highlight("prod issue ｜ chase the vendor ｜ today", now) {
		kinds = append(kinds, tok.Kind)
	}
	var pipes, dues int
	for _, k := range kinds {
		switch k {
		case TokPipe:
			pipes++
		case TokDue:
			dues++
		}
	}
	if pipes != 2 {
		t.Errorf("coloured %d separators, want 2 (%v)", pipes, kinds)
	}
	if dues == 0 {
		t.Errorf("the date was not coloured as one (%v)", kinds)
	}
}
