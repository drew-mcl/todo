package api

import (
	"strings"
	"time"

	"github.com/drew-mcl/todo/internal/store"
)

func truncate(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, t.Location())
}

// sections renders the shared grouping as DTOs. The rules themselves live in
// the store, so the browser and the terminal group identically.
func sections(tasks []*store.Task, v store.View, s store.Sort, now time.Time) []Section {
	groups := store.Sections(tasks, v, s, now)
	out := make([]Section, 0, len(groups))
	for _, g := range groups {
		out = append(out, Section{Label: g.Label, Tasks: taskDTOs(g.Tasks, now)})
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
