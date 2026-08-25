package web

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/drew-mcl/todo/internal/parse"
	"github.com/drew-mcl/todo/internal/store"
)

// page assembles the shared shell data: sidebar counts, topics and tags.
func (s *Server) page(r *http.Request, view store.View) (pageData, error) {
	q := r.URL.Query()
	sort := store.Sort(q.Get("sort"))
	if sort == "" {
		sort = store.SortManual
	}

	d := pageData{
		View:     view,
		Sort:     sort,
		Topic:    q.Get("topic"),
		Tag:      q.Get("tag"),
		Assignee: q.Get("assignee"),
		Search:   strings.TrimSpace(q.Get("q")),
		Now:      s.now(),
	}
	d.Heading = view.Title()
	switch {
	case d.Topic != "":
		d.Heading = d.Topic
	case d.Tag != "":
		d.Heading = "#" + d.Tag
	case d.Assignee != "":
		d.Heading = strings.ToUpper(d.Assignee[:1]) + d.Assignee[1:]
	}

	tasks, err := s.store.List(store.Query{
		View: view, Sort: sort, Topic: d.Topic, Tag: d.Tag,
		Assignee: d.Assignee, Search: d.Search,
	}, d.Now)
	if err != nil {
		return d, err
	}
	d.Count = len(tasks)
	d.Sections = group(tasks, view, sort, d.Now)

	if d.Counts, err = s.store.Counts(d.Now); err != nil {
		return d, err
	}
	if d.Topics, err = s.store.Topics(); err != nil {
		return d, err
	}
	if d.People, err = s.store.Assignees(); err != nil {
		return d, err
	}
	if d.Tags, err = s.store.Tags(); err != nil {
		return d, err
	}
	return d, nil
}

func (s *Server) handleList(w http.ResponseWriter, r *http.Request) {
	view := store.View(r.PathValue("name"))
	if !view.Valid() {
		http.NotFound(w, r)
		return
	}
	d, err := s.page(r, view)
	if err != nil {
		s.fail(w, err, "loading the list")
		return
	}

	// The undo toast survives the redirect after a capture as query parameters,
	// which keeps the server stateless.
	if id, err := strconv.ParseInt(r.URL.Query().Get("undo"), 10, 64); err == nil && id > 0 {
		n, _ := strconv.Atoi(r.URL.Query().Get("n"))
		d.Undo = &undo{BatchID: id, Count: n}
	}
	d.Open = r.URL.Query().Has("capture")

	s.render(w, "page", d)
}

// handleWeek renders the planning board: seven columns you drag tasks between,
// which is the same gesture as setting a due date.
func (s *Server) handleWeek(w http.ResponseWriter, r *http.Request) {
	now := s.now()
	start := store.WeekStart(now)
	if q := r.URL.Query().Get("start"); q != "" {
		if d, err := time.ParseInLocation("2006-01-02", q, time.Local); err == nil {
			start = store.WeekStart(d)
		}
	}

	d, err := s.page(r, store.ViewAll)
	if err != nil {
		s.fail(w, err, "loading the week")
		return
	}
	if !d.Filtered() {
		d.Heading = "Week"
	}
	d.Sections = nil
	plan := store.Query{Topic: d.Topic, Tag: d.Tag, Assignee: d.Assignee, Search: d.Search}
	if d.Plan, err = s.store.Week(start, now, plan); err != nil {
		s.fail(w, err, "loading the week")
		return
	}
	s.render(w, "page", d)
}

// handleSchedule moves a task onto a day, or off the calendar entirely.
func (s *Server) handleSchedule(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		http.Error(w, "Bad task id.", http.StatusBadRequest)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Could not read the drop.", http.StatusBadRequest)
		return
	}
	t, err := s.store.Schedule(id, r.FormValue("date"))
	if err != nil {
		s.fail(w, err, "rescheduling the task")
		return
	}
	s.render(w, "card", rowData{Task: t, View: store.ViewAll})
}

// handlePreview re-parses the draft on every keystroke. The preview is rendered
// server side on purpose: there is one parser, and this is it.
func (s *Server) handlePreview(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Could not read the draft.", http.StatusBadRequest)
		return
	}
	s.render(w, "preview", newPreview(parse.Parse(r.FormValue("draft"), s.now())))
}

// previewData backs the capture preview fragment. The tallies are worked out here
// rather than in the template, which cannot unpack three return values.
type previewData struct {
	Lines   []parse.Line
	Any     bool
	Summary string
}

func newPreview(res *parse.Result) previewData {
	tasks, notes, skipped := res.Counts()

	var parts []string
	if tasks > 0 {
		parts = append(parts, count(tasks, "task", "tasks"))
	}
	if notes > 0 {
		parts = append(parts, count(notes, "note", "notes"))
	}
	if skipped > 0 {
		parts = append(parts, fmt.Sprintf("%d skipped", skipped))
	}
	if len(parts) == 0 {
		parts = append(parts, "nothing yet")
	}
	return previewData{
		Lines:   res.Lines,
		Any:     tasks+notes+skipped > 0,
		Summary: strings.Join(parts, " · "),
	}
}

func count(n int, one, many string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, one)
	}
	return fmt.Sprintf("%d %s", n, many)
}

func (s *Server) handleCapture(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Could not read the draft.", http.StatusBadRequest)
		return
	}
	view := store.View(r.FormValue("view"))
	if !view.Valid() {
		view = store.ViewToday
	}

	res := parse.Parse(r.FormValue("draft"), s.now())
	if len(res.Tasks) == 0 {
		// Nothing readable in the draft: reopen the sheet rather than silently
		// throwing the text away.
		d, err := s.page(r, view)
		if err != nil {
			s.fail(w, err, "loading the list")
			return
		}
		d.Draft, d.Open = r.FormValue("draft"), true
		s.render(w, "page", d)
		return
	}

	batch, err := s.store.CreateBatch(res.Tasks, "web", s.now())
	if err != nil {
		s.fail(w, err, "saving the tasks")
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/view/%s?undo=%d&n=%d", view, batch, len(res.Tasks)),
		http.StatusSeeOther)
}

func (s *Server) handleUndoBatch(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		http.Error(w, "Bad batch id.", http.StatusBadRequest)
		return
	}
	if _, err := s.store.UndoBatch(id); err != nil {
		s.fail(w, err, "undoing the paste")
		return
	}
	view := store.View(r.FormValue("view"))
	if !view.Valid() {
		view = store.ViewToday
	}
	w.Header().Set("HX-Redirect", "/view/"+string(view))
	w.WriteHeader(http.StatusNoContent)
}

// rowData backs a single task row, which needs to know its view to decide
// whether completing it takes it off this list.
type rowData struct {
	Task *store.Task
	View store.View
	Sort store.Sort
}

func (s *Server) handleToggle(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		http.Error(w, "Bad task id.", http.StatusBadRequest)
		return
	}
	t, err := s.store.Toggle(id, s.now())
	if err != nil {
		s.fail(w, err, "updating the task")
		return
	}
	view := store.View(r.FormValue("view"))
	if !view.Valid() {
		view = store.ViewToday
	}

	// Swap the row, and push fresh sidebar counts alongside it so the numbers
	// never drift from the list.
	s.render(w, "row", rowData{Task: t, View: view})
	if counts, err := s.store.Counts(s.now()); err == nil {
		s.render(w, "counts-oob", pageData{View: view, Counts: counts, OOB: true})
	}
}

func (s *Server) handleMove(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		http.Error(w, "Bad task id.", http.StatusBadRequest)
		return
	}
	above, _ := strconv.ParseInt(r.FormValue("above"), 10, 64)
	below, _ := strconv.ParseInt(r.FormValue("below"), 10, 64)
	if err := s.store.Move(id, above, below); err != nil {
		s.fail(w, err, "reordering the list")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleDetail(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		http.Error(w, "Bad task id.", http.StatusBadRequest)
		return
	}
	t, err := s.store.Get(id)
	if err != nil {
		s.fail(w, err, "loading the task")
		return
	}
	s.render(w, "detail", rowData{Task: t, View: store.View(r.URL.Query().Get("view"))})
}

// handleUpdate applies an inline edit. Only the fields present in the form are
// touched, so each control can post on its own.
func (s *Server) handleUpdate(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		http.Error(w, "Bad task id.", http.StatusBadRequest)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Could not read the edit.", http.StatusBadRequest)
		return
	}

	var p store.Patch
	if v, ok := formValue(r, "title"); ok && strings.TrimSpace(v) != "" {
		p.Title = &v
	}
	if v, ok := formValue(r, "note"); ok {
		p.Note = &v
	}
	if v, ok := formValue(r, "topic"); ok && strings.TrimSpace(v) != "" {
		p.Topic = &v
	}
	if v, ok := formValue(r, "assignee"); ok {
		p.Assignee = &v
	}
	if v, ok := formValue(r, "priority"); ok {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 && n <= 3 {
			pr := parse.Priority(n)
			p.Priority = &pr
		}
	}
	if v, ok := formValue(r, "due"); ok {
		// The date field takes the same shorthand as the capture box.
		if d := parse.ParseDue(v, s.now()); strings.TrimSpace(v) == "" || (d.Recognised && !d.Explicit) {
			p.ClearDue = true
		} else if d.Recognised {
			p.Due = &d.Date
		}
	}
	if v, ok := formValue(r, "tags"); ok {
		tags := strings.FieldsFunc(v, func(r rune) bool {
			return r == ',' || r == ' ' || r == '#'
		})
		p.Tags = &tags
	}

	t, err := s.store.Update(id, p)
	if err != nil {
		s.fail(w, err, "saving the change")
		return
	}
	view := store.View(r.FormValue("view"))
	if !view.Valid() {
		view = store.ViewToday
	}
	s.render(w, "row", rowData{Task: t, View: view})
}

func (s *Server) handleDelete(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		http.Error(w, "Bad task id.", http.StatusBadRequest)
		return
	}
	if err := s.store.Delete(id); err != nil {
		s.fail(w, err, "deleting the task")
		return
	}
	w.WriteHeader(http.StatusOK)
}

// formValue distinguishes a field that was submitted empty from one that was not
// submitted at all, so a partial form never wipes a field it did not include.
func formValue(r *http.Request, key string) (string, bool) {
	if _, ok := r.Form[key]; !ok {
		return "", false
	}
	return r.FormValue(key), true
}
