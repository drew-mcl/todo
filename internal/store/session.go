package store

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// ErrNotFound is returned when a task id does not exist.

// Captures. One batch of tasks taken in one go, usually a call.

// Session is one capture -- typically a call -- with how it has since gone.
type Session struct {
	ID        int64
	Title     string
	Source    string
	CreatedAt time.Time
	Total     int
	Done      int
}

// Sessions lists captures newest first, so a call can be found by when it
// happened. Empty ones (everything undone since deleted) are left out.
func (s *Store) Sessions(limit int) ([]Session, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.db.Query(`
		SELECT b.id, b.title, b.source, b.created_at,
		       count(t.id),
		       sum(CASE WHEN t.status = 'done' THEN 1 ELSE 0 END)
		FROM batches b
		JOIN tasks t ON t.batch_id = b.id
		GROUP BY b.id
		ORDER BY b.created_at DESC, b.id DESC
		LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("listing sessions: %w", err)
	}
	defer rows.Close()

	var out []Session
	for rows.Next() {
		var (
			ses     Session
			created string
			done    sql.NullInt64
		)
		if err := rows.Scan(&ses.ID, &ses.Title, &ses.Source, &created, &ses.Total, &done); err != nil {
			return nil, err
		}
		ses.Done = int(done.Int64)
		if t, err := time.Parse(time.RFC3339, created); err == nil {
			ses.CreatedAt = t
		}
		out = append(out, ses)
	}
	return out, rows.Err()
}

// Session returns one capture by id.
func (s *Store) Session(id int64) (Session, error) {
	all, err := s.Sessions(0)
	if err != nil {
		return Session{}, err
	}
	for _, ses := range all {
		if ses.ID == id {
			return ses, nil
		}
	}
	return Session{}, ErrNotFound
}

// RenameSession retitles a capture after the fact.
func (s *Store) RenameSession(id int64, title string) error {
	res, err := s.db.Exec("UPDATE batches SET title = ? WHERE id = ?", strings.TrimSpace(title), id)
	if err != nil {
		return fmt.Errorf("renaming session %d: %w", id, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// Merge folds other captures into one.
//
// Filing twice for the same call is easy to do -- someone says one more thing
// after you thought you were finished -- and leaves the record in two pieces.
// This puts it back together: the tasks move, the emptied captures go, and the
// one you kept takes a name if it did not have one.
//
// Returns how many tasks moved.
func (s *Store) Merge(into int64, others []int64) (int, error) {
	keep := make([]int64, 0, len(others))
	for _, id := range others {
		if id != into {
			keep = append(keep, id)
		}
	}
	if len(keep) == 0 {
		return 0, nil
	}

	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	// The one being kept has to exist, or this would orphan everything into a
	// batch id that is not there.
	var exists int
	if err := tx.QueryRow(`SELECT count(*) FROM batches WHERE id = ?`, into).Scan(&exists); err != nil {
		return 0, fmt.Errorf("looking up the capture: %w", err)
	}
	if exists == 0 {
		return 0, ErrNotFound
	}

	marks, args := placeholders(keep)

	// A capture with no name of its own takes the first one it can find among
	// the others, rather than staying "the 3.15 on Tuesday" for no reason.
	var title string
	if err := tx.QueryRow(`SELECT title FROM batches WHERE id = ?`, into).Scan(&title); err != nil {
		return 0, err
	}
	if title == "" {
		var found sql.NullString
		if err := tx.QueryRow(`SELECT title FROM batches
			WHERE id IN (`+marks+`) AND title <> '' ORDER BY id LIMIT 1`, args...).Scan(&found); err != nil &&
			err != sql.ErrNoRows {
			return 0, err
		}
		if found.Valid && found.String != "" {
			if _, err := tx.Exec(`UPDATE batches SET title = ? WHERE id = ?`, found.String, into); err != nil {
				return 0, err
			}
		}
	}

	res, err := tx.Exec(`UPDATE tasks SET batch_id = ? WHERE batch_id IN (`+marks+`)`,
		append([]any{into}, args...)...)
	if err != nil {
		return 0, fmt.Errorf("moving the tasks: %w", err)
	}
	moved, _ := res.RowsAffected()

	if _, err := tx.Exec(`DELETE FROM batches WHERE id IN (`+marks+`)`, args...); err != nil {
		return 0, fmt.Errorf("removing the emptied captures: %w", err)
	}
	return int(moved), tx.Commit()
}

// placeholders builds "?, ?, ?" and the arguments to go with it.
func placeholders(ids []int64) (string, []any) {
	marks := make([]string, len(ids))
	args := make([]any, len(ids))
	for i, id := range ids {
		marks[i], args[i] = "?", id
	}
	return strings.Join(marks, ", "), args
}
