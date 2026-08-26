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

// A task, and the operations that change one.

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

// Overdue reports whether the task is open and past its due date.
func (t *Task) Overdue(now time.Time) bool {
	return t.Status == StatusOpen && t.Due != nil && t.Due.Before(truncate(now))
}

func truncate(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, t.Location())
}

const (
	ViewToday     View = "today"
	ViewOverdue   View = "overdue"
	ViewUpcoming  View = "upcoming"
	ViewAnytime   View = "anytime"
	ViewDelegated View = "delegated"
	ViewLogbook   View = "logbook"
	ViewAll       View = "all"
)

const (
	SortManual   Sort = "manual"
	SortDue      Sort = "due"
	SortCreated  Sort = "created"
	SortTopic    Sort = "topic"
	SortAssignee Sort = "assignee"
	SortPriority Sort = "priority"
)

// Get returns one task by id.
func (s *Store) Get(id int64) (*Task, error) {
	t, err := scanTask(s.db.QueryRow(selectCols+" WHERE id = ?", id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return t, err
}

// Capture describes where a batch of tasks came from.
type Capture struct {
	Source string // web, cli, table, copilot
	Title  string // the call or meeting it was taken in, when there was one
}

// CreateBatch stores a parsed paste as one undoable unit. New tasks land above
// everything already in the list, keeping the order they were pasted in.
func (s *Store) CreateBatch(tasks []*parse.Task, c Capture, now time.Time) (int64, error) {
	if len(tasks) == 0 {
		return 0, errors.New("nothing to add")
	}
	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	stamp := now.Format(time.RFC3339)
	if c.Source == "" {
		c.Source = "web"
	}
	res, err := tx.Exec("INSERT INTO batches (created_at, source, title) VALUES (?, ?, ?)",
		stamp, c.Source, strings.TrimSpace(c.Title))
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

// ── Sessions ────────────────────────────────────────────────────────────────

// Open reports how much of the session is still outstanding.
func (s Session) Open() int { return s.Total - s.Done }

// ── Grouping ────────────────────────────────────────────────────────────────
