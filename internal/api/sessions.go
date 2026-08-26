package api

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/drew-mcl/todo/internal/parse"
	"github.com/drew-mcl/todo/internal/store"
)

// Session is one capture -- usually a call -- as the client sees it.
type Session struct {
	ID      int64  `json:"id"`
	Title   string `json:"title"`
	Source  string `json:"source"`
	When    string `json:"when"` // "Today", "Tue 25 Aug"
	Date    string `json:"date"` // ISO
	Total   int    `json:"total"`
	Open    int    `json:"open"`
	Done    int    `json:"done"`
	Preview string `json:"preview"` // the first task, to recognise it by
}

func (s *Server) handleSessions(w http.ResponseWriter, r *http.Request) {
	now := s.now()
	list, err := s.store.Sessions(200)
	if err != nil {
		s.fail(w, err, "loading your calls")
		return
	}

	out := make([]Session, 0, len(list))
	for _, ses := range list {
		dto := Session{
			ID: ses.ID, Title: ses.Title, Source: ses.Source,
			Total: ses.Total, Done: ses.Done, Open: ses.Open(),
			When: parse.FormatDue(ses.CreatedAt, now),
			Date: ses.CreatedAt.Format("2006-01-02"),
		}
		if dto.Title == "" {
			dto.Title = untitled(ses.CreatedAt)
		}
		if tasks, err := s.store.List(store.Query{View: store.ViewAll, Batch: ses.ID}, now); err == nil && len(tasks) > 0 {
			dto.Preview = tasks[0].Title
		}
		out = append(out, dto)
	}
	s.json(w, out)
}

// untitled names a capture that was never given one, so the list still reads as
// a sequence of events rather than a column of blanks.
func untitled(t time.Time) string {
	return "capture · " + strings.ToLower(t.Format("Mon 2 Jan, 15:04"))
}

func (s *Server) handleRenameSession(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		s.errorf(w, http.StatusBadRequest, "Bad session id.")
		return
	}
	var body struct {
		Title string `json:"title"`
	}
	if err := decode(r, &body); err != nil {
		s.errorf(w, http.StatusBadRequest, "Could not read the title.")
		return
	}
	if err := s.store.RenameSession(id, body.Title); err != nil {
		s.fail(w, err, "renaming the call")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleExport renders a capture as something you can paste straight into a
// chat. Plain text on purpose: it survives Teams, Slack and mail intact, which
// markdown tables and rich HTML do not.
func (s *Server) handleExport(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		s.errorf(w, http.StatusBadRequest, "Bad session id.")
		return
	}
	now := s.now()
	ses, err := s.store.Session(id)
	if err != nil {
		s.fail(w, err, "loading the call")
		return
	}
	tasks, err := s.store.List(store.Query{View: store.ViewAll, Batch: id}, now)
	if err != nil {
		s.fail(w, err, "loading the actions")
		return
	}

	q := r.URL.Query()
	opts := exportOptions{
		GroupByOwner: q.Get("group") != "none",
		IncludeDone:  q.Get("done") == "1",
		IncludeNotes: q.Get("notes") == "1",
	}
	s.json(w, map[string]string{"text": renderExport(ses, tasks, now, opts)})
}

type exportOptions struct {
	GroupByOwner bool
	IncludeDone  bool
	IncludeNotes bool
}

func renderExport(ses store.Session, tasks []*store.Task, now time.Time, o exportOptions) string {
	title := ses.Title
	if title == "" {
		title = untitled(ses.CreatedAt)
	}

	var keep []*store.Task
	for _, t := range tasks {
		if t.Done() && !o.IncludeDone {
			continue
		}
		keep = append(keep, t)
	}

	var b strings.Builder
	fmt.Fprintf(&b, "%s — %s\n", title, ses.CreatedAt.Format("Mon 2 Jan"))
	if len(keep) == 0 {
		b.WriteString("\nNothing outstanding.\n")
		return b.String()
	}
	fmt.Fprintf(&b, "%s\n", plural(len(keep), "action", "actions"))

	line := func(t *store.Task) {
		b.WriteString("  • " + t.Title)
		var bits []string
		if t.Due != nil {
			bits = append(bits, parse.FormatDue(*t.Due, now))
		}
		if t.Priority > 0 {
			bits = append(bits, strings.ToLower(t.Priority.String()))
		}
		if len(bits) > 0 {
			b.WriteString(" (" + strings.Join(bits, ", ") + ")")
		}
		b.WriteString("\n")
		if o.IncludeNotes && t.Note != "" {
			for _, n := range strings.Split(t.Note, "\n") {
				b.WriteString("      " + n + "\n")
			}
		}
	}

	if !o.GroupByOwner {
		b.WriteString("\n")
		for _, t := range keep {
			line(t)
		}
		return b.String()
	}

	// Grouped by who owes it, because the point of sending this is that each
	// person can find their own name.
	byOwner := map[string][]*store.Task{}
	for _, t := range keep {
		owner := t.Assignee
		if owner == "" {
			owner = "me"
		}
		byOwner[owner] = append(byOwner[owner], t)
	}
	owners := make([]string, 0, len(byOwner))
	for k := range byOwner {
		owners = append(owners, k)
	}
	sort.Slice(owners, func(i, j int) bool {
		if (owners[i] == "me") != (owners[j] == "me") {
			return owners[j] == "me" // mine last; everyone else needs to read theirs
		}
		return owners[i] < owners[j]
	})

	for _, owner := range owners {
		fmt.Fprintf(&b, "\n%s\n", title2(owner))
		for _, t := range byOwner[owner] {
			line(t)
		}
	}
	return b.String()
}

func title2(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

func plural(n int, one, many string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, one)
	}
	return fmt.Sprintf("%d %s", n, many)
}
