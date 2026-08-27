package store

import (
	"fmt"
	"strings"
	"time"

	"github.com/drew-mcl/todo/internal/parse"
)

// ErrNotFound is returned when a task id does not exist.

// The counts and groupings the sidebar is made of.

// Counts returns the number of tasks behind each sidebar view.
//
// One pass over the table rather than one per view -- the sidebar is drawn on
// every list, and six round trips to answer six counts is five more than the
// question needs. The clauses are still the views' own: a second copy of what
// "overdue" means is exactly how a sidebar starts disagreeing with the list it
// is sitting next to.
func (s *Store) Counts(now time.Time) (map[View]int, error) {
	today := truncate(now).Format(dateLayout)

	var (
		counted []View
		columns []string
		args    []any
	)
	for _, v := range Views {
		if v == ViewLogbook {
			continue // a running total of everything ever done is just noise
		}
		where, a := v.where(today)
		counted = append(counted, v)
		columns = append(columns, "count(*) FILTER (WHERE "+where+")")
		args = append(args, a...)
	}

	found := make([]int, len(counted))
	into := make([]any, len(counted))
	for i := range found {
		into[i] = &found[i]
	}
	if err := s.db.QueryRow("SELECT "+strings.Join(columns, ", ")+" FROM tasks", args...).
		Scan(into...); err != nil {
		return nil, fmt.Errorf("counting the views: %w", err)
	}

	out := make(map[View]int, len(counted))
	for i, v := range counted {
		out[v] = found[i]
	}
	return out, nil
}

// DoneOn counts what was completed on a given day, so Today can show progress
// rather than only what is left.
func (s *Store) DoneOn(day time.Time) (int, error) {
	var n int
	err := s.db.QueryRow(
		`SELECT count(*) FROM tasks WHERE status = 'done' AND date(completed_at) = ?`,
		truncate(day).Format(dateLayout)).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("counting what is done: %w", err)
	}
	return n, nil
}

// Group is a topic or tag with its open-task count, for the sidebar.
type Group struct {
	Name  string
	Count int
}

// Topics lists topics with open tasks, busiest first.
func (s *Store) Topics() ([]Group, error) {
	return s.groups(`SELECT topic, count(*) FROM tasks WHERE status = 'open'
		GROUP BY topic ORDER BY count(*) DESC, topic ASC`)
}

// Assignees lists the people with open tasks against them, busiest first. These
// are discovered from what you have typed -- there is no roster to maintain.
func (s *Store) Assignees() ([]Group, error) {
	return s.groups(`SELECT assignee, count(*) FROM tasks
		WHERE status = 'open' AND assignee IS NOT NULL AND assignee <> ''
		GROUP BY assignee ORDER BY count(*) DESC, assignee ASC`)
}

// Tags lists tags with open tasks, busiest first.
func (s *Store) Tags() ([]Group, error) {
	return s.groups(`SELECT tag, count(*) FROM tags
		JOIN tasks ON tasks.id = tags.task_id WHERE tasks.status = 'open'
		GROUP BY tag ORDER BY count(*) DESC, tag ASC`)
}

func (s *Store) groups(query string) ([]Group, error) {
	rows, err := s.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Group
	for rows.Next() {
		var g Group
		if err := rows.Scan(&g.Name, &g.Count); err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

// Section is a labelled run of tasks. An empty label means the tasks run
// straight down the page with no heading.
type Section struct {
	Label string
	Tasks []*Task
}

// Sections splits a list into display groups. Grouping follows the active sort,
// so the headings always explain the order on screen.
//
// Here rather than in each front end: the browser and the terminal have to tell
// the same story about what you are looking at, and two copies of these rules
// would eventually stop agreeing.
func Sections(tasks []*Task, v View, s Sort, now time.Time) []Section {
	if len(tasks) == 0 {
		return nil
	}

	label := func(*Task) string { return "" }
	switch {
	case s == SortTopic:
		label = func(t *Task) string { return t.Topic }
	case s == SortAssignee:
		label = func(t *Task) string {
			if t.Assignee == "" {
				return "me"
			}
			return t.Assignee
		}
	case v == ViewUpcoming, v == ViewOverdue:
		label = func(t *Task) string { return strings.ToLower(parse.FormatDue(*t.Due, now)) }
	case v == ViewLogbook:
		label = func(t *Task) string {
			if t.CompletedAt == nil {
				return "done"
			}
			return strings.ToLower(parse.FormatDue(*t.CompletedAt, now))
		}
	}

	var out []Section
	for _, t := range tasks {
		l := label(t)
		if n := len(out); n > 0 && out[n-1].Label == l {
			out[n-1].Tasks = append(out[n-1].Tasks, t)
			continue
		}
		out = append(out, Section{Label: l, Tasks: []*Task{t}})
	}
	return out
}
