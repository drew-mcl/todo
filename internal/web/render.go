package web

import (
	"fmt"
	"html/template"
	"strings"
	"time"

	"github.com/drew-mcl/todo/internal/parse"
	"github.com/drew-mcl/todo/internal/store"
)

// section is a labelled run of tasks in the list. An empty Label means the
// section is unheaded and the tasks run straight down the page.
type section struct {
	Label string
	Tasks []*store.Task
}

// pageData is everything a list page or fragment needs.
type pageData struct {
	View     store.View
	Sort     store.Sort
	Heading  string
	Sections []section
	Count    int
	Counts   map[store.View]int
	Topics   []store.Group
	People   []store.Group
	Tags     []store.Group
	Topic    string
	Tag      string
	Assignee string
	Search   string
	Now      time.Time
	Plan     *store.Plan // set on the week planner
	Undo     *undo
	Draft    string
	Open     bool // capture sheet starts open
	OOB      bool // render as an htmx out-of-band swap
}

// undo backs the toast shown after a paste.
type undo struct {
	BatchID int64
	Count   int
}

// Empty reports whether the current list has nothing in it.
func (p pageData) Empty() bool { return p.Count == 0 }

// OnWeek reports whether the planner is what is on screen, so the sidebar does
// not light up the All row underneath it.
func (p pageData) OnWeek() bool { return p.Plan != nil }

// CanDrag reports whether manual order is the order on screen. Dragging is only
// offered when it is, since in every other order a drop has nothing to mean.
func (p pageData) CanDrag() bool {
	return p.Sort == store.SortManual && p.View != store.ViewUpcoming && p.View != store.ViewLogbook
}

// Filtered reports whether a topic, tag or search is narrowing the list.
func (p pageData) Filtered() bool {
	return p.Topic != "" || p.Tag != "" || p.Assignee != "" || p.Search != ""
}

// QueryString rebuilds the current filters as a query string so links and form
// posts keep the user where they were.
func (p pageData) QueryString() template.URL {
	var parts []string
	if p.Sort != "" && p.Sort != store.SortManual {
		parts = append(parts, "sort="+string(p.Sort))
	}
	if p.Topic != "" {
		parts = append(parts, "topic="+template.URLQueryEscaper(p.Topic))
	}
	if p.Tag != "" {
		parts = append(parts, "tag="+template.URLQueryEscaper(p.Tag))
	}
	if p.Assignee != "" {
		parts = append(parts, "assignee="+template.URLQueryEscaper(p.Assignee))
	}
	if p.Search != "" {
		parts = append(parts, "q="+template.URLQueryEscaper(p.Search))
	}
	if len(parts) == 0 {
		return ""
	}
	return template.URL("?" + strings.Join(parts, "&"))
}

// dayLabel and dayNum head the planner's columns.
func weekFuncs(now func() time.Time) template.FuncMap {
	return template.FuncMap{
		"dayName": func(t time.Time) string { return t.Format("Mon") },
		"dayNum":  func(t time.Time) string { return t.Format("2 Jan") },
		"isoDate": func(t time.Time) string { return t.Format("2006-01-02") },
		"isToday": func(t time.Time) bool {
			n := now()
			y1, m1, d1 := t.Date()
			y2, m2, d2 := n.Date()
			return y1 == y2 && m1 == m2 && d1 == d2
		},
		"isPast": func(t time.Time) bool {
			n := now()
			return t.Before(time.Date(n.Year(), n.Month(), n.Day(), 0, 0, 0, 0, n.Location()))
		},
		"weekRange": func(p *store.Plan) string {
			if p.Start.Month() == p.End().Month() {
				return p.Start.Format("2") + "–" + p.End().Format("2 Jan 2006")
			}
			return p.Start.Format("2 Jan") + " – " + p.End().Format("2 Jan 2006")
		},
	}
}

// funcs are the helpers available to every template.
func funcs(now func() time.Time) template.FuncMap {
	return template.FuncMap{
		// highlight colours a raw shorthand line using the parser's own rules.
		"highlight": func(raw string) []parse.Token { return parse.Highlight(raw, now()) },

		// due renders a date the way the list shows it.
		"due": func(d *time.Time) string {
			if d == nil {
				return ""
			}
			return parse.FormatDue(*d, now())
		},

		// dayLabel heads a date-grouped section.
		"dayLabel": func(d *time.Time) string {
			if d == nil {
				return "No date"
			}
			return parse.FormatDue(*d, now())
		},

		"bangs": func(p parse.Priority) string { return p.Marks() },

		"priName": func(p parse.Priority) string { return p.String() },

		// who renders an assignee for reading rather than for typing.
		"who": func(s string) string {
			if s == "" {
				return ""
			}
			return strings.ToUpper(s[:1]) + s[1:]
		},

		"overdue": func(t *store.Task) bool { return t.Overdue(now()) },

		// leftView reports whether toggling a task takes it out of this view, so
		// the row can be shown struck through and then collapse away.
		"leftView": func(t *store.Task, v store.View) bool {
			return t.Done() != (v == store.ViewLogbook)
		},

		"lines": func(s string) []string {
			if s == "" {
				return nil
			}
			return strings.Split(s, "\n")
		},

		"plural": func(n int, one, many string) string {
			if n == 1 {
				return fmt.Sprintf("%d %s", n, one)
			}
			return fmt.Sprintf("%d %s", n, many)
		},

		// dict builds an argument bag, since a template can only take one value.
		"dict": func(kv ...any) (map[string]any, error) {
			if len(kv)%2 != 0 {
				return nil, fmt.Errorf("dict needs an even number of arguments, got %d", len(kv))
			}
			m := make(map[string]any, len(kv)/2)
			for i := 0; i < len(kv); i += 2 {
				k, ok := kv[i].(string)
				if !ok {
					return nil, fmt.Errorf("dict key %d is not a string", i)
				}
				m[k] = kv[i+1]
			}
			return m, nil
		},

		"views": func() []store.View { return store.Views },

		// viewAll gives templates a typed key for the counts map: index will not
		// convert an untyped string to store.View the way eq does.
		"viewAll": func() store.View { return store.ViewAll },
		"sorts":   func() []store.Sort { return store.Sorts },
	}
}

// group splits tasks into display sections. Grouping follows the active sort so
// the headings always explain the order the user is looking at.
func group(tasks []*store.Task, v store.View, s store.Sort, now time.Time) []section {
	if len(tasks) == 0 {
		return nil
	}
	switch {
	case s == store.SortTopic:
		return by(tasks, func(t *store.Task) string { return t.Topic })
	case s == store.SortAssignee:
		return by(tasks, func(t *store.Task) string {
			if t.Assignee == "" {
				return "Me"
			}
			return strings.ToUpper(t.Assignee[:1]) + t.Assignee[1:]
		})
	case v == store.ViewUpcoming:
		return by(tasks, func(t *store.Task) string { return parse.FormatDue(*t.Due, now) })
	case v == store.ViewLogbook:
		return by(tasks, func(t *store.Task) string {
			if t.CompletedAt == nil {
				return "Done"
			}
			return parse.FormatDue(*t.CompletedAt, now)
		})
	case v == store.ViewToday:
		return splitOverdue(tasks, now)
	}
	return []section{{Tasks: tasks}}
}

// by groups consecutive tasks sharing a label, preserving the incoming order.
func by(tasks []*store.Task, label func(*store.Task) string) []section {
	var out []section
	for _, t := range tasks {
		l := label(t)
		if n := len(out); n > 0 && out[n-1].Label == l {
			out[n-1].Tasks = append(out[n-1].Tasks, t)
			continue
		}
		out = append(out, section{Label: l, Tasks: []*store.Task{t}})
	}
	return out
}

// splitOverdue lifts anything already late to the top of Today, since that is
// the whole point of the view.
func splitOverdue(tasks []*store.Task, now time.Time) []section {
	var late, due []*store.Task
	for _, t := range tasks {
		if t.Overdue(now) {
			late = append(late, t)
		} else {
			due = append(due, t)
		}
	}
	if len(late) == 0 {
		return []section{{Tasks: due}}
	}
	out := []section{{Label: "Overdue", Tasks: late}}
	if len(due) > 0 {
		out = append(out, section{Label: "Today", Tasks: due})
	}
	return out
}
