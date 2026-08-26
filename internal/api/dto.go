// Package api serves the task list as JSON.
//
// Anything the client would otherwise have to re-derive is computed here and
// sent alongside the raw value: due labels, overdue-ness, section grouping and
// the shorthand highlighting. The rule is the same one the whole project runs
// on -- there is one parser and one set of rules, and they live in Go.
package api

import (
	"time"

	"github.com/drew-mcl/todo/internal/parse"
	"github.com/drew-mcl/todo/internal/store"
)

// Task is one task as the client sees it.
type Task struct {
	ID       int64    `json:"id"`
	Topic    string   `json:"topic"`
	Title    string   `json:"title"`
	Note     string   `json:"note"`
	Due      *string  `json:"due"`      // ISO date, null when undated
	DueLabel string   `json:"dueLabel"` // "Today", "5 days overdue"
	Overdue  bool     `json:"overdue"`
	Assignee string   `json:"assignee"`
	Priority int      `json:"priority"`
	Done     bool     `json:"done"`
	Tags     []string `json:"tags"`
	Raw      string   `json:"raw"`
	BatchID  *int64   `json:"batchId"`

	// Provenance. The list is also a record of what was committed to and when,
	// so "what day did Sam say they would do this" is answerable.
	CapturedAt   string `json:"capturedAt"`
	CapturedWhen string `json:"capturedWhen"`
	CompletedAt  string `json:"completedAt,omitempty"`
}

func taskDTO(t *store.Task, now time.Time) Task {
	d := Task{
		ID: t.ID, Topic: t.Topic, Title: t.Title, Note: t.Note,
		Assignee: t.Assignee, Priority: int(t.Priority),
		Done: t.Done(), Tags: t.Tags, Raw: t.Raw, BatchID: t.BatchID,
		Overdue: t.Overdue(now),
	}
	if d.Tags == nil {
		d.Tags = []string{}
	}
	if t.Due != nil {
		iso := t.Due.Format("2006-01-02")
		d.Due = &iso
		d.DueLabel = parse.FormatDue(*t.Due, now)
	}
	if !t.CreatedAt.IsZero() {
		d.CapturedAt = t.CreatedAt.Format("2006-01-02")
		d.CapturedWhen = parse.FormatDue(t.CreatedAt, now)
	}
	if t.CompletedAt != nil {
		d.CompletedAt = t.CompletedAt.Format("2006-01-02")
	}
	return d
}

func taskDTOs(in []*store.Task, now time.Time) []Task {
	out := make([]Task, 0, len(in))
	for _, t := range in {
		out = append(out, taskDTO(t, now))
	}
	return out
}

// Section is a labelled run of tasks. An empty label means the tasks run
// straight down the page with no heading.
type Section struct {
	Label string `json:"label"`
	Tasks []Task `json:"tasks"`
}

// Group is a topic, person or tag with its open-task count.
type Group struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

func groupDTOs(in []store.Group) []Group {
	out := make([]Group, 0, len(in))
	for _, g := range in {
		out = append(out, Group{Name: g.Name, Count: g.Count})
	}
	return out
}

// Meta is everything the sidebar needs.
type Meta struct {
	Counts map[string]int `json:"counts"`
	Topics []Group        `json:"topics"`
	People []Group        `json:"people"`
	Tags   []Group        `json:"tags"`
	Today  string         `json:"today"`
	// Progress for the day, so Today can show what has been closed and not only
	// what is left.
	TodayLabel string `json:"todayLabel"`
	DoneToday  int    `json:"doneToday"`
}

// ListResponse backs a list view.
type ListResponse struct {
	View     string    `json:"view"`
	Sort     string    `json:"sort"`
	Sections []Section `json:"sections"`
	// Total is how many match; Shown is how many came back. They differ only
	// when a list is large enough that sending it all would be the bug.
	Total     int  `json:"total"`
	Shown     int  `json:"shown"`
	Truncated bool `json:"truncated"`
	CanDrag   bool `json:"canDrag"`
	Meta      Meta `json:"meta"`
}

// Day is one column of the week planner.
type Day struct {
	Date    string `json:"date"`
	Weekday string `json:"weekday"`
	Label   string `json:"label"`
	IsToday bool   `json:"isToday"`
	IsPast  bool   `json:"isPast"`
	Tasks   []Task `json:"tasks"`
}

// WeekResponse backs the planner.
type WeekResponse struct {
	Start       string `json:"start"`
	Prev        string `json:"prev"`
	Next        string `json:"next"`
	Range       string `json:"range"`
	Days        []Day  `json:"days"`
	Overdue     []Task `json:"overdue"`
	Unscheduled []Task `json:"unscheduled"`
	Meta        Meta   `json:"meta"`
}

// Token is one highlighted span of a shorthand line.
type Token struct {
	Kind string `json:"kind"`
	Text string `json:"text"`
}

// PreviewTask is a parsed-but-unsaved task.
type PreviewTask struct {
	Topic    string   `json:"topic"`
	Title    string   `json:"title"`
	Note     string   `json:"note"`
	DueLabel string   `json:"dueLabel"`
	Assignee string   `json:"assignee"`
	Priority int      `json:"priority"`
	Tags     []string `json:"tags"`
	Warning  string   `json:"warning"`
}

// PreviewLine is what one input line turned into.
type PreviewLine struct {
	N          int          `json:"n"`
	Raw        string       `json:"raw"`
	Kind       string       `json:"kind"`
	Reason     string       `json:"reason,omitempty"`
	Tokens     []Token      `json:"tokens,omitempty"`
	Task       *PreviewTask `json:"task,omitempty"`
	OwnerTitle string       `json:"ownerTitle,omitempty"`
}

// PreviewResponse is the live read of a draft.
type PreviewResponse struct {
	Lines   []PreviewLine `json:"lines"`
	Tasks   int           `json:"tasks"`
	Notes   int           `json:"notes"`
	Skipped int           `json:"skipped"`
}

func previewDTO(res *parse.Result, now time.Time) PreviewResponse {
	out := PreviewResponse{Lines: make([]PreviewLine, 0, len(res.Lines))}
	out.Tasks, out.Notes, out.Skipped = res.Counts()

	for _, l := range res.Lines {
		if l.Kind == parse.KindBlank {
			continue
		}
		line := PreviewLine{N: l.N, Raw: l.Raw, Kind: string(l.Kind), Reason: l.Reason}
		for _, tok := range parse.Highlight(l.Raw, now) {
			line.Tokens = append(line.Tokens, Token{Kind: string(tok.Kind), Text: tok.Text})
		}
		if l.Owner != nil {
			line.OwnerTitle = l.Owner.Title
		}
		if l.Task != nil {
			line.Task = previewTask(l.Task, now)
		}
		out.Lines = append(out.Lines, line)
	}
	return out
}

// previewTask renders a parsed-but-unsaved task, whether it came from the
// shorthand or from a pasted grid.
func previewTask(t *parse.Task, now time.Time) *PreviewTask {
	p := &PreviewTask{
		Topic: t.Topic, Title: t.Title, Note: t.Note,
		Assignee: t.Assignee, Priority: int(t.Priority),
		Tags: t.Tags, Warning: t.Warning,
	}
	if p.Tags == nil {
		p.Tags = []string{}
	}
	if t.Due != nil {
		p.DueLabel = parse.FormatDue(*t.Due, now)
	}
	return p
}
