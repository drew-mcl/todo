package store

import (
	"fmt"
	"time"
)

// ErrNotFound is returned when a task id does not exist.

// The week planner.

// Day is one column of the week planner.
type Day struct {
	Date  time.Time
	Tasks []*Task
}

// Plan is a week's worth of columns plus the two trays you plan out of: what is
// already late, and what has no date at all.
type Plan struct {
	Start       time.Time
	Days        []Day
	Overdue     []*Task
	Unscheduled []*Task
}

// Prev and Next give the weeks either side, for navigation.
func (p Plan) Prev() string { return p.Start.AddDate(0, 0, -7).Format(dateLayout) }

func (p Plan) Next() string { return p.Start.AddDate(0, 0, 7).Format(dateLayout) }

// End is the last day shown.
func (p Plan) End() time.Time { return p.Start.AddDate(0, 0, len(p.Days)-1) }

// WeekStart returns the Monday on or before t.
func WeekStart(t time.Time) time.Time {
	t = truncate(t)
	// Go weeks start on Sunday; planning weeks start on Monday.
	back := (int(t.Weekday()) + 6) % 7
	return t.AddDate(0, 0, -back)
}

// Week builds a planning board: seven day columns from start, plus everything
// overdue and everything undated, so a week can be laid out by dragging.
//
// One rule keeps it honest: anything late lives in the overdue tray and never in
// a column. Browse back to a finished week and its unfinished work is all in the
// tray, waiting to be dragged somewhere real.
func (s *Store) Week(start time.Time, now time.Time, q Query) (*Plan, error) {
	start = truncate(start)
	plan := &Plan{Start: start, Days: make([]Day, 7)}
	for i := range plan.Days {
		plan.Days[i] = Day{Date: start.AddDate(0, 0, i)}
	}

	today := truncate(now).Format(dateLayout)
	last := start.AddDate(0, 0, 6).Format(dateLayout)

	filter, fargs := q.planFilter()

	// Columns hold what is still ahead of you; anything late is pulled out below.
	scheduled, err := s.list(`WHERE status = 'open' AND due_date IS NOT NULL
		AND due_date >= ? AND due_date >= ? AND due_date <= ?`+filter+`
		ORDER BY position ASC`,
		append([]any{start.Format(dateLayout), today, last}, fargs...)...)
	if err != nil {
		return nil, fmt.Errorf("loading the week: %w", err)
	}

	byDate := make(map[string]int, 7)
	for i, d := range plan.Days {
		byDate[d.Date.Format(dateLayout)] = i
	}
	for _, t := range scheduled {
		if i, ok := byDate[t.Due.Format(dateLayout)]; ok {
			plan.Days[i].Tasks = append(plan.Days[i].Tasks, t)
		}
	}

	// Everything unfinished and late, whatever week it came from, so browsing
	// back to a past week shows exactly what still needs a new date.
	if plan.Overdue, err = s.list(`WHERE status = 'open' AND due_date IS NOT NULL
		AND due_date < ?`+filter+` ORDER BY due_date ASC`,
		append([]any{today}, fargs...)...); err != nil {
		return nil, err
	}
	if plan.Unscheduled, err = s.list(`WHERE status = 'open' AND due_date IS NULL`+
		filter+` ORDER BY position ASC`, fargs...); err != nil {
		return nil, err
	}
	return plan, nil
}

// Schedule sets or clears a task's due date. An empty date unschedules it.
func (s *Store) Schedule(id int64, date string) (*Task, error) {
	if date == "" {
		return s.Update(id, Patch{ClearDue: true})
	}
	d, err := time.ParseInLocation(dateLayout, date, time.Local)
	if err != nil {
		return nil, fmt.Errorf("%q is not a date", date)
	}
	return s.Update(id, Patch{Due: &d})
}
