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
