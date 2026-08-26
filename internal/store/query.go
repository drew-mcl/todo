package store

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// ErrNotFound is returned when a task id does not exist.

// Which tasks, in what order.
//
// One place builds the filters, so a list, a count and the planner can never
// disagree about what matches.

// View names a list of tasks in the sidebar.
type View string

// Views is the sidebar order: the time-based lists, then who you are waiting on,
// then everything open at once, then what is finished.
var Views = []View{
	ViewToday, ViewOverdue, ViewUpcoming, ViewAnytime, ViewDelegated, ViewAll, ViewLogbook,
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
		// Exactly today. Late work has its own list and its own week-board tray,
		// so it never squats on top of the day you actually planned.
		return "status = 'open' AND due_date = ?", []any{today}
	case ViewOverdue:
		return "status = 'open' AND due_date IS NOT NULL AND due_date < ?", []any{today}
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

// Sorts is the order the sort menu offers.
var Sorts = []Sort{SortManual, SortDue, SortCreated, SortTopic, SortAssignee, SortPriority}

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
	Batch    int64  // narrow to one capture session
	From     string // ISO date: captured on or after
	To       string // ISO date: captured on or before
	Limit    int    // 0 means no limit
}

// narrow appends the filters shared by List and Count.
func (q Query) narrow(clauses []string, args []any) ([]string, []any) {
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
	if q.Batch > 0 {
		clauses = append(clauses, "batch_id = ?")
		args = append(args, q.Batch)
	}
	// When something was captured, which is how you find your way back to the
	// call it was said on.
	if q.From != "" {
		clauses = append(clauses, "date(created_at) >= ?")
		args = append(args, q.From)
	}
	if q.To != "" {
		clauses = append(clauses, "date(created_at) <= ?")
		args = append(args, q.To)
	}
	if q.Search != "" {
		clauses = append(clauses, "(title LIKE ? OR note LIKE ? OR topic LIKE ?)")
		like := "%" + q.Search + "%"
		args = append(args, like, like, like)
	}
	return clauses, args
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

	clauses, args = q.narrow(clauses, args)

	order := q.Sort.orderBy()
	switch q.View {
	case ViewUpcoming, ViewOverdue:
		order = "due_date ASC, position ASC"
	case ViewLogbook:
		order = "completed_at DESC"
	}

	sql := fmt.Sprintf("%s WHERE %s ORDER BY %s", selectCols, strings.Join(clauses, " AND "), order)
	if q.Limit > 0 {
		sql += fmt.Sprintf(" LIMIT %d", q.Limit)
	}
	rows, err := s.db.Query(sql, args...)
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

// Count is how many tasks match, whatever the limit returned.
func (s *Store) Count(q Query, now time.Time) (int, error) {
	where, args := q.View.where(truncate(now).Format(dateLayout))
	clauses := []string{where}
	clauses, args = q.narrow(clauses, args)

	var n int
	err := s.db.QueryRow("SELECT count(*) FROM tasks WHERE "+strings.Join(clauses, " AND "), args...).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("counting tasks: %w", err)
	}
	return n, nil
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

// planFilter is the same narrowing as a list, in the form the planner's queries
// want it. It used to be a second implementation, and had quietly fallen three
// filters behind the one it was copied from.
func (q Query) planFilter() (string, []any) {
	clauses, args := q.narrow(nil, nil)
	if len(clauses) == 0 {
		return "", nil
	}
	return " AND " + strings.Join(clauses, " AND "), args
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
