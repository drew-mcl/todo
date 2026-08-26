package api

import (
	"net/http"
	"strings"
	"time"

	"github.com/drew-mcl/todo/internal/parse"
	"github.com/drew-mcl/todo/internal/store"
)

// Reading and changing tasks.

// ── plumbing ────────────────────────────────────────────────────────────────

// ── handlers ────────────────────────────────────────────────────────────────

func (s *Server) handleList(w http.ResponseWriter, r *http.Request) {
	now := s.now()
	q := s.query(r)
	if q.View == "" {
		q.View = store.ViewToday
	}
	if !q.View.Valid() {
		s.errorf(w, http.StatusNotFound, "There is no %q view.", q.View)
		return
	}
	if q.Sort == "" {
		q.Sort = store.SortManual
	}

	q.Limit = listLimit
	tasks, err := s.store.List(q, now)
	if err != nil {
		s.fail(w, err, "loading the list")
		return
	}
	total, err := s.store.Count(q, now)
	if err != nil {
		s.fail(w, err, "counting the list")
		return
	}
	meta, err := s.meta(now)
	if err != nil {
		s.fail(w, err, "loading the sidebar")
		return
	}

	res := ListResponse{
		View: string(q.View), Sort: string(q.Sort),
		Sections:  sections(tasks, q.View, q.Sort, now),
		Total:     total,
		Shown:     len(tasks),
		Truncated: total > len(tasks),
		CanDrag: q.Sort == store.SortManual && q.View != store.ViewUpcoming &&
			q.View != store.ViewOverdue && q.View != store.ViewLogbook,
		Meta: meta,
	}
	s.json(w, res)
}

func (s *Server) handleWeek(w http.ResponseWriter, r *http.Request) {
	now := s.now()
	start := store.WeekStart(now)
	if v := r.URL.Query().Get("start"); v != "" {
		if d, err := time.ParseInLocation("2006-01-02", v, time.Local); err == nil {
			start = store.WeekStart(d)
		}
	}

	plan, err := s.store.Week(start, now, s.query(r))
	if err != nil {
		s.fail(w, err, "loading the week")
		return
	}
	meta, err := s.meta(now)
	if err != nil {
		s.fail(w, err, "loading the sidebar")
		return
	}

	today := truncate(now)
	res := WeekResponse{
		Start: plan.Start.Format("2006-01-02"),
		Prev:  plan.Prev(), Next: plan.Next(),
		Range:       weekRange(plan),
		Days:        make([]Day, 0, len(plan.Days)),
		Overdue:     taskDTOs(plan.Overdue, now),
		Unscheduled: taskDTOs(plan.Unscheduled, now),
		Meta:        meta,
	}
	for _, d := range plan.Days {
		res.Days = append(res.Days, Day{
			Date:    d.Date.Format("2006-01-02"),
			Weekday: d.Date.Format("Mon"),
			Label:   d.Date.Format("2 Jan"),
			IsToday: d.Date.Equal(today),
			IsPast:  d.Date.Before(today),
			Tasks:   taskDTOs(d.Tasks, now),
		})
	}
	s.json(w, res)
}

func (s *Server) handlePreview(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Draft string `json:"draft"`
	}
	if err := decode(r, &body); err != nil {
		s.errorf(w, http.StatusBadRequest, "Could not read the draft.")
		return
	}
	s.json(w, previewDTO(parse.Parse(body.Draft, s.now()), s.now()))
}

func (s *Server) handleCapture(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Draft string `json:"draft"`
		Title string `json:"title"`
	}
	if err := decode(r, &body); err != nil {
		s.errorf(w, http.StatusBadRequest, "Could not read the draft.")
		return
	}
	now := s.now()
	res := parse.Parse(body.Draft, now)
	if len(res.Tasks) == 0 {
		s.errorf(w, http.StatusUnprocessableEntity,
			"No line contained a '|', so nothing was read as a task")
		return
	}
	if err := checkBatchSize(len(res.Tasks)); err != nil {
		s.errorf(w, http.StatusUnprocessableEntity, "%s", err.Error())
		return
	}
	batch, err := s.store.CreateBatch(res.Tasks, store.Capture{Source: "web", Title: body.Title}, now)
	if err != nil {
		s.fail(w, err, "saving the tasks")
		return
	}
	s.json(w, map[string]any{"batchId": batch, "added": len(res.Tasks)})
}

func (s *Server) handleUndo(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		s.errorf(w, http.StatusBadRequest, "Bad batch id.")
		return
	}
	n, err := s.store.UndoBatch(id)
	if err != nil {
		s.fail(w, err, "undoing the paste")
		return
	}
	s.json(w, map[string]int{"removed": n})
}

func (s *Server) handleToggle(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		s.errorf(w, http.StatusBadRequest, "Bad task id.")
		return
	}
	t, err := s.store.Toggle(id, s.now())
	if err != nil {
		s.fail(w, err, "updating the task")
		return
	}
	s.json(w, taskDTO(t, s.now()))
}

func (s *Server) handleMove(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		s.errorf(w, http.StatusBadRequest, "Bad task id.")
		return
	}
	var body struct{ Above, Below int64 }
	if err := decode(r, &body); err != nil {
		s.errorf(w, http.StatusBadRequest, "Could not read the move.")
		return
	}
	if err := s.store.Move(id, body.Above, body.Below); err != nil {
		s.fail(w, err, "reordering the list")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleSchedule(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		s.errorf(w, http.StatusBadRequest, "Bad task id.")
		return
	}
	var body struct {
		Date string `json:"date"`
	}
	if err := decode(r, &body); err != nil {
		s.errorf(w, http.StatusBadRequest, "Could not read the drop.")
		return
	}
	t, err := s.store.Schedule(id, body.Date)
	if err != nil {
		s.fail(w, err, "rescheduling the task")
		return
	}
	s.json(w, taskDTO(t, s.now()))
}

// handleUpdate applies a partial edit. Only fields present in the body are
// touched, so a control can post on its own without wiping its neighbours.
func (s *Server) handleUpdate(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		s.errorf(w, http.StatusBadRequest, "Bad task id.")
		return
	}
	var body struct {
		Title    *string   `json:"title"`
		Topic    *string   `json:"topic"`
		Note     *string   `json:"note"`
		Due      *string   `json:"due"`
		Assignee *string   `json:"assignee"`
		Priority *int      `json:"priority"`
		Tags     *[]string `json:"tags"`
	}
	if err := decode(r, &body); err != nil {
		s.errorf(w, http.StatusBadRequest, "Could not read the edit.")
		return
	}

	var p store.Patch
	p.Title, p.Topic, p.Note, p.Assignee, p.Tags = body.Title, body.Topic, body.Note, body.Assignee, body.Tags
	if body.Priority != nil {
		pr := parse.Priority(*body.Priority)
		p.Priority = &pr
	}
	if body.Due != nil {
		// The date field takes the same shorthand as the capture box.
		due := parse.ParseDue(*body.Due, s.now())
		switch {
		case strings.TrimSpace(*body.Due) == "", due.Recognised && !due.Explicit:
			p.ClearDue = true
		case due.Recognised:
			p.Due = &due.Date
		default:
			s.fieldError(w, "due", "%q is not a date I understand.", *body.Due)
			return
		}
	}

	t, err := s.store.Update(id, p)
	if err != nil {
		s.fail(w, err, "saving the change")
		return
	}
	s.json(w, taskDTO(t, s.now()))
}

func (s *Server) handleDelete(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		s.errorf(w, http.StatusBadRequest, "Bad task id.")
		return
	}
	if err := s.store.Delete(id); err != nil {
		s.fail(w, err, "deleting the task")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
