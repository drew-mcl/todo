package store

import (
	"database/sql"
	"errors"
	"fmt"
)

// ErrNotFound is returned when a task id does not exist.

// Manual order.
//
// Positions are floats, so a task can be dropped between any two others without
// renumbering the rest -- until the gap between two of them closes, at which
// point everything is renumbered once and the drop is retried.

// rebalanceGap is the point at which float midpoints get too cramped to keep
// splitting, and the whole list is renumbered to integers.
const rebalanceGap = 1e-6

// Move places task id between the tasks above and below it in the list. Either
// neighbour may be 0, meaning the task was dropped at that end.
func (s *Store) Move(id, above, below int64) error {
	pos, err := s.midpoint(above, below, 0)
	if err != nil {
		return err
	}
	if _, err := s.db.Exec("UPDATE tasks SET position = ? WHERE id = ?", pos, id); err != nil {
		return fmt.Errorf("moving task %d: %w", id, err)
	}
	return nil
}

func (s *Store) midpoint(above, below int64, attempt int) (float64, error) {
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
			if attempt > 0 {
				return 0, fmt.Errorf("positions %g and %g will not separate", hi, lo)
			}
			if err := s.rebalance(); err != nil {
				return 0, err
			}
			return s.midpoint(above, below, attempt+1)
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
