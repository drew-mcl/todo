package api

import (
	"strings"
	"time"

	"github.com/drew-mcl/todo/internal/parse"
	"github.com/drew-mcl/todo/internal/store"
)

func truncate(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, t.Location())
}

// sections splits tasks into display groups. Grouping follows the active sort,
// so the headings always explain the order on screen.
func sections(tasks []*store.Task, v store.View, s store.Sort, now time.Time) []Section {
	if len(tasks) == 0 {
		return []Section{}
	}
	switch {
	case s == store.SortTopic:
		return by(tasks, now, func(t *store.Task) string { return t.Topic })
	case s == store.SortAssignee:
		return by(tasks, now, func(t *store.Task) string {
			if t.Assignee == "" {
				return "me"
			}
			return t.Assignee
		})
	case v == store.ViewUpcoming:
		return by(tasks, now, func(t *store.Task) string { return parse.FormatDue(*t.Due, now) })
	case v == store.ViewLogbook:
		return by(tasks, now, func(t *store.Task) string {
			if t.CompletedAt == nil {
				return "done"
			}
			return parse.FormatDue(*t.CompletedAt, now)
		})
	case v == store.ViewToday:
		return splitOverdue(tasks, now)
	}
	return []Section{{Label: "", Tasks: taskDTOs(tasks, now)}}
}

// by groups consecutive tasks sharing a label, preserving the incoming order.
func by(tasks []*store.Task, now time.Time, label func(*store.Task) string) []Section {
	var out []Section
	var run []*store.Task
	flush := func(l string) {
		if len(run) > 0 {
			out = append(out, Section{Label: l, Tasks: taskDTOs(run, now)})
		}
	}
	current := ""
	for i, t := range tasks {
		l := label(t)
		if i == 0 {
			current = l
		}
		if l != current {
			flush(current)
			current, run = l, nil
		}
		run = append(run, t)
	}
	flush(current)
	return out
}

// splitOverdue lifts anything already late to the top of Today, which is the
// whole point of the view.
func splitOverdue(tasks []*store.Task, now time.Time) []Section {
	var late, due []*store.Task
	for _, t := range tasks {
		if t.Overdue(now) {
			late = append(late, t)
		} else {
			due = append(due, t)
		}
	}
	if len(late) == 0 {
		return []Section{{Label: "", Tasks: taskDTOs(due, now)}}
	}
	out := []Section{{Label: "overdue", Tasks: taskDTOs(late, now)}}
	if len(due) > 0 {
		out = append(out, Section{Label: "today", Tasks: taskDTOs(due, now)})
	}
	return out
}

// weekRange names the span a planner page covers.
func weekRange(p *store.Plan) string {
	end := p.End()
	if p.Start.Month() == end.Month() {
		return strings.ToLower(p.Start.Format("2") + "–" + end.Format("2 Jan 2006"))
	}
	return strings.ToLower(p.Start.Format("2 Jan") + " – " + end.Format("2 Jan 2006"))
}
