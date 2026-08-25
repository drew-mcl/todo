package store

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/drew-mcl/todo/internal/parse"
)

// ErrNotFound is returned when a task id does not exist.
var ErrNotFound = errors.New("task not found")

const dateLayout = "2006-01-02"

// Status values.
const (
	StatusOpen = "open"
	StatusDone = "done"
)

// Task is a stored task.
type Task struct {
	ID          int64
	Topic       string
	Title       string
	Note        string
	Due         *time.Time
	Assignee    string
	Priority    parse.Priority
	Status      string
	Position    float64
	BatchID     *int64
	Raw         string
	CreatedAt   time.Time
	CompletedAt *time.Time
	Tags        []string
}

// Done reports whether the task has been completed.
func (t *Task) Done() bool { return t.Status == StatusDone }

// Delegated reports whether the task is waiting on someone else.
func (t *Task) Delegated() bool { return t.Assignee != "" }

// Overdue reports whether the task is open and past its due date.
func (t *Task) Overdue(now time.Time) bool {
	return t.Status == StatusOpen && t.Due != nil && t.Due.Before(truncate(now))
}

func truncate(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, t.Location())
}

// View names a list of tasks in the sidebar.
type View string

const (
	ViewToday     View = "today"
	ViewUpcoming  View = "upcoming"
	ViewAnytime   View = "anytime"
	ViewDelegated View = "delegated"
	ViewLogbook   View = "logbook"
	ViewAll       View = "all"
)

// Views is the sidebar order: the time-based lists, then who you are waiting on,
// then everything open at once, then what is finished.
var Views = []View{ViewToday, ViewUpcoming, ViewAnytime, ViewDelegated, ViewAll, ViewLogbook}

// Title renders a view name for display.
func (v View) Title() string {
	if v == "" {
		return ""
	}
	return strings.ToUpper(string(v[:1])) + string(v[1:])
}

// Valid reports whether v is a known view.
func (v View) Valid() bool {
	for _, k := range Views {
		if k == v {
			return true
		}
	}
	return false
}

// where builds the view's filter clause. Delegated is a filter, not a silo: its
// tasks also appear in the dated views, so nothing hides from you there.
func (v View) where(today string) (string, []any) {
	switch v {
	case ViewToday:
		return "status = 'open' AND due_date IS NOT NULL AND due_date <= ?", []any{today}
	case ViewUpcoming:
		return "status = 'open' AND due_date > ?", []any{today}
	case ViewAnytime:
		return "status = 'open' AND due_date IS NULL", nil
	case ViewDelegated:
		return "status = 'open' AND assignee IS NOT NULL AND assignee <> ''", nil
	case ViewLogbook:
		return "status = 'done'", nil
	default:
		return "status = 'open'", nil
	}
}

// Sort orders a list. Manual is the drag order; the rest are read-only views onto
// it, which is why dragging is disabled while one of them is active.
type Sort string

const (
	SortManual   Sort = "manual"
	SortDue      Sort = "due"
	SortCreated  Sort = "created"
	SortTopic    Sort = "topic"
	SortAssignee Sort = "assignee"
	SortPriority Sort = "priority"
)

// Sorts is the order the sort menu offers.
var Sorts = []Sort{SortManual, SortDue, SortCreated, SortTopic, SortAssignee, SortPriority}

// Title renders a sort name for display.
func (s Sort) Title() string {
	if s == "" {
		return ""
	}
	return strings.ToUpper(string(s[:1])) + string(s[1:])
}

func (s Sort) orderBy() string {
	switch s {
	case SortDue:
		return "due_date IS NULL, due_date ASC, position ASC"
	case SortCreated:
		return "created_at DESC, position ASC"
	case SortTopic:
		return "topic ASC, position ASC"
	case SortAssignee:
		return "assignee IS NULL, assignee ASC, position ASC"
	case SortPriority:
		return "priority DESC, position ASC"
	default:
		return "position ASC"
	}
}

// Query selects and orders a list of tasks.
type Query struct {
	View     View
	Sort     Sort
	Topic    string
	Tag      string
	Assignee string
	Search   string
}

const selectCols = `SELECT id, topic, title, note, due_date, assignee, priority, status,
	position, batch_id, raw, created_at, completed_at,
	COALESCE((SELECT group_concat(tag, ',') FROM tags WHERE task_id = tasks.id), '')
	FROM tasks`

// List returns the tasks matching q. Two views override the sort, because their
// whole organising idea is a date: Upcoming always runs forwards in time, and
// Logbook always runs backwards from the last thing finished.
func (s *Store) List(q Query, now time.Time) ([]*Task, error) {
	where, args := q.View.where(truncate(now).Format(dateLayout))
	clauses := []string{where}

	if q.Topic != "" {
		clauses = append(clauses, "topic = ?")
		args = append(args, q.Topic)
	}
	if q.Tag != "" {
		clauses = append(clauses, "id IN (SELECT task_id FROM tags WHERE tag = ?)")
		args = append(args, q.Tag)
	}
	if q.Assignee != "" {
		clauses = append(clauses, "assignee = ?")
		args = append(args, q.Assignee)
	}
	if q.Search != "" {
		clauses = append(clauses, "(title LIKE ? OR note LIKE ? OR topic LIKE ?)")
		like := "%" + q.Search + "%"
		args = append(args, like, like, like)
	}

	order := q.Sort.orderBy()
	switch q.View {
	case ViewUpcoming:
		order = "due_date ASC, position ASC"
	case ViewLogbook:
		order = "completed_at DESC"
	}

	rows, err := s.db.Query(
		fmt.Sprintf("%s WHERE %s ORDER BY %s", selectCols, strings.Join(clauses, " AND "), order),
		args...)
	if err != nil {
		return nil, fmt.Errorf("listing tasks: %w", err)
	}
	defer rows.Close()

	var out []*Task
	for rows.Next() {
		t, err := scanTask(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

type scanner interface{ Scan(...any) error }

func scanTask(row scanner) (*Task, error) {
	var (
		t                              Task
		due, assignee, completed, tags sql.NullString
		created                        string
		batch                          sql.NullInt64
	)
	if err := row.Scan(&t.ID, &t.Topic, &t.Title, &t.Note, &due, &assignee, &t.Priority,
		&t.Status, &t.Position, &batch, &t.Raw, &created, &completed, &tags); err != nil {
		return nil, err
	}
	if due.Valid && due.String != "" {
		if d, err := time.ParseInLocation(dateLayout, due.String, time.Local); err == nil {
			t.Due = &d
		}
	}
	if completed.Valid && completed.String != "" {
		if c, err := time.Parse(time.RFC3339, completed.String); err == nil {
			t.CompletedAt = &c
		}
	}
	if c, err := time.Parse(time.RFC3339, created); err == nil {
		t.CreatedAt = c
	}
	t.Assignee = assignee.String
	if batch.Valid {
		t.BatchID = &batch.Int64
	}
	if tags.String != "" {
		t.Tags = strings.Split(tags.String, ",")
	}
	return &t, nil
}

// Get returns one task by id.
func (s *Store) Get(id int64) (*Task, error) {
	t, err := scanTask(s.db.QueryRow(selectCols+" WHERE id = ?", id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return t, err
}

// Counts returns the number of tasks behind each sidebar view.
func (s *Store) Counts(now time.Time) (map[View]int, error) {
	out := make(map[View]int, len(Views))
	today := truncate(now).Format(dateLayout)
	for _, v := range Views {
		if v == ViewLogbook {
			continue // a running total of everything ever done is just noise
		}
		where, args := v.where(today)
		var n int
		if err := s.db.QueryRow("SELECT count(*) FROM tasks WHERE "+where, args...).Scan(&n); err != nil {
			return nil, fmt.Errorf("counting %s: %w", v, err)
		}
		out[v] = n
	}
	return out, nil
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

// CreateBatch stores a parsed paste as one undoable unit. New tasks land above
// everything already in the list, keeping the order they were pasted in.
func (s *Store) CreateBatch(tasks []*parse.Task, source string, now time.Time) (int64, error) {
	if len(tasks) == 0 {
		return 0, errors.New("nothing to add")
	}
	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	stamp := now.Format(time.RFC3339)
	res, err := tx.Exec("INSERT INTO batches (created_at, source) VALUES (?, ?)", stamp, source)
	if err != nil {
		return 0, fmt.Errorf("creating batch: %w", err)
	}
	batchID, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}

	var minPos sql.NullFloat64
	if err := tx.QueryRow("SELECT min(position) FROM tasks WHERE status = 'open'").Scan(&minPos); err != nil {
		return 0, err
	}
	top := minPos.Float64

	for i, t := range tasks {
		var due any
		if t.Due != nil {
			due = t.Due.Format(dateLayout)
		}
		var assignee any
		if t.Assignee != "" {
			assignee = t.Assignee
		}
		pos := top - float64(len(tasks)-i)

		res, err := tx.Exec(`INSERT INTO tasks
			(topic, title, note, due_date, assignee, priority, status, position, batch_id, raw, created_at)
			VALUES (?, ?, ?, ?, ?, ?, 'open', ?, ?, ?, ?)`,
			t.Topic, t.Title, t.Note, due, assignee, int(t.Priority), pos, batchID, t.Raw, stamp)
		if err != nil {
			return 0, fmt.Errorf("inserting task %q: %w", t.Title, err)
		}
		id, err := res.LastInsertId()
		if err != nil {
			return 0, err
		}
		for _, tag := range t.Tags {
			if _, err := tx.Exec("INSERT OR IGNORE INTO tags (task_id, tag) VALUES (?, ?)", id, tag); err != nil {
				return 0, fmt.Errorf("tagging task %d: %w", id, err)
			}
		}
	}
	return batchID, tx.Commit()
}

// Toggle flips a task between open and done and returns its new state.
func (s *Store) Toggle(id int64, now time.Time) (*Task, error) {
	t, err := s.Get(id)
	if err != nil {
		return nil, err
	}
	if t.Done() {
		_, err = s.db.Exec("UPDATE tasks SET status = 'open', completed_at = NULL WHERE id = ?", id)
	} else {
		_, err = s.db.Exec("UPDATE tasks SET status = 'done', completed_at = ? WHERE id = ?",
			now.Format(time.RFC3339), id)
	}
	if err != nil {
		return nil, fmt.Errorf("toggling task %d: %w", id, err)
	}
	return s.Get(id)
}

// Patch holds the fields an edit changes. Nil fields are left alone.
type Patch struct {
	Topic    *string
	Title    *string
	Note     *string
	Due      *time.Time // set with ClearDue to distinguish "unchanged" from "cleared"
	ClearDue bool
	Assignee *string
	Priority *parse.Priority
	Tags     *[]string
}

// Update applies a patch to one task.
func (s *Store) Update(id int64, p Patch) (*Task, error) {
	sets, args := []string{}, []any{}
	add := func(col string, v any) { sets, args = append(sets, col+" = ?"), append(args, v) }

	if p.Topic != nil {
		add("topic", strings.ToLower(strings.TrimSpace(*p.Topic)))
	}
	if p.Title != nil {
		add("title", strings.TrimSpace(*p.Title))
	}
	if p.Note != nil {
		add("note", *p.Note)
	}
	if p.ClearDue {
		add("due_date", nil)
	} else if p.Due != nil {
		add("due_date", p.Due.Format(dateLayout))
	}
	if p.Assignee != nil {
		if a := strings.ToLower(strings.TrimSpace(*p.Assignee)); a == "" {
			add("assignee", nil)
		} else {
			add("assignee", a)
		}
	}
	if p.Priority != nil {
		add("priority", int(*p.Priority))
	}

	if len(sets) > 0 {
		args = append(args, id)
		res, err := s.db.Exec("UPDATE tasks SET "+strings.Join(sets, ", ")+" WHERE id = ?", args...)
		if err != nil {
			return nil, fmt.Errorf("updating task %d: %w", id, err)
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return nil, ErrNotFound
		}
	}

	if p.Tags != nil {
		tx, err := s.db.Begin()
		if err != nil {
			return nil, err
		}
		defer tx.Rollback()
		if _, err := tx.Exec("DELETE FROM tags WHERE task_id = ?", id); err != nil {
			return nil, err
		}
		for _, tag := range *p.Tags {
			if tag = strings.ToLower(strings.TrimSpace(tag)); tag != "" {
				if _, err := tx.Exec("INSERT OR IGNORE INTO tags (task_id, tag) VALUES (?, ?)", id, tag); err != nil {
					return nil, err
				}
			}
		}
		if err := tx.Commit(); err != nil {
			return nil, err
		}
	}
	return s.Get(id)
}

// rebalanceGap is the point at which float midpoints get too cramped to keep
// splitting, and the whole list is renumbered to integers.
const rebalanceGap = 1e-6

// Move places task id between the tasks above and below it in the list. Either
// neighbour may be 0, meaning the task was dropped at that end.
func (s *Store) Move(id, above, below int64) error {
	pos, err := s.midpoint(above, below)
	if err != nil {
		return err
	}
	if _, err := s.db.Exec("UPDATE tasks SET position = ? WHERE id = ?", pos, id); err != nil {
		return fmt.Errorf("moving task %d: %w", id, err)
	}
	return nil
}

func (s *Store) midpoint(above, below int64) (float64, error) {
	posOf := func(id int64) (float64, bool, error) {
		if id == 0 {
			return 0, false, nil
		}
		var p float64
		err := s.db.QueryRow("SELECT position FROM tasks WHERE id = ?", id).Scan(&p)
		if errors.Is(err, sql.ErrNoRows) {
			return 0, false, nil
		}
		return p, err == nil, err
	}

	hi, hasHi, err := posOf(above)
	if err != nil {
		return 0, err
	}
	lo, hasLo, err := posOf(below)
	if err != nil {
		return 0, err
	}

	switch {
	case hasHi && hasLo:
		if lo-hi < rebalanceGap {
			if err := s.rebalance(); err != nil {
				return 0, err
			}
			return s.midpoint(above, below)
		}
		return hi + (lo-hi)/2, nil
	case hasHi:
		return hi + 1, nil
	case hasLo:
		return lo - 1, nil
	default:
		return 0, nil
	}
}

// rebalance renumbers every task to consecutive integers, restoring room to
// insert between any two of them.
func (s *Store) rebalance() error {
	_, err := s.db.Exec(`WITH ranked AS (
		SELECT id, row_number() OVER (ORDER BY position ASC, id ASC) AS rn FROM tasks)
		UPDATE tasks SET position = (SELECT rn FROM ranked WHERE ranked.id = tasks.id)`)
	if err != nil {
		return fmt.Errorf("rebalancing positions: %w", err)
	}
	return nil
}

// UndoBatch deletes every task from one paste and reports how many went.
func (s *Store) UndoBatch(id int64) (int, error) {
	res, err := s.db.Exec("DELETE FROM tasks WHERE batch_id = ?", id)
	if err != nil {
		return 0, fmt.Errorf("undoing batch %d: %w", id, err)
	}
	n, _ := res.RowsAffected()
	if _, err := s.db.Exec("DELETE FROM batches WHERE id = ?", id); err != nil {
		return int(n), err
	}
	return int(n), nil
}

// Delete removes one task for good.
func (s *Store) Delete(id int64) error {
	res, err := s.db.Exec("DELETE FROM tasks WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("deleting task %d: %w", id, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// ── Week planning ───────────────────────────────────────────────────────────

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

// planFilter narrows the whole board to one topic, person or tag.
func (q Query) planFilter() (string, []any) {
	var clause string
	var args []any
	if q.Topic != "" {
		clause += " AND topic = ?"
		args = append(args, q.Topic)
	}
	if q.Assignee != "" {
		clause += " AND assignee = ?"
		args = append(args, q.Assignee)
	}
	if q.Tag != "" {
		clause += " AND id IN (SELECT task_id FROM tags WHERE tag = ?)"
		args = append(args, q.Tag)
	}
	if q.Search != "" {
		clause += " AND (title LIKE ? OR note LIKE ? OR topic LIKE ?)"
		like := "%" + q.Search + "%"
		args = append(args, like, like, like)
	}
	return clause, args
}

// list runs a task query with a raw trailing clause.
func (s *Store) list(clause string, args ...any) ([]*Task, error) {
	rows, err := s.db.Query(selectCols+" "+clause, args...)
	if err != nil {
		return nil, fmt.Errorf("listing tasks: %w", err)
	}
	defer rows.Close()
	var out []*Task
	for rows.Next() {
		t, err := scanTask(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
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
