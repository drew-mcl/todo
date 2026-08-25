package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/drew-mcl/todo/internal/parse"
	"github.com/drew-mcl/todo/internal/store"
)

// Server routes the JSON API and, behind it, the built client.
type Server struct {
	store *store.Store
	now   func() time.Time
	mux   *http.ServeMux
}

// New builds a server over st. now is injectable so tests can freeze the clock.
// client serves the built UI; pass nil to run the API alone.
func New(st *store.Store, now func() time.Time, client http.Handler) *Server {
	if now == nil {
		now = time.Now
	}
	s := &Server{store: st, now: now, mux: http.NewServeMux()}
	s.routes(client)
	return s
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) { s.mux.ServeHTTP(w, r) }

func (s *Server) routes(client http.Handler) {
	s.mux.HandleFunc("GET /api/list", s.handleList)
	s.mux.HandleFunc("GET /api/week", s.handleWeek)
	s.mux.HandleFunc("POST /api/preview", s.handlePreview)
	s.mux.HandleFunc("POST /api/capture", s.handleCapture)
	s.mux.HandleFunc("POST /api/batches/{id}/undo", s.handleUndo)
	s.mux.HandleFunc("POST /api/tasks/{id}/toggle", s.handleToggle)
	s.mux.HandleFunc("POST /api/tasks/{id}/move", s.handleMove)
	s.mux.HandleFunc("POST /api/tasks/{id}/schedule", s.handleSchedule)
	s.mux.HandleFunc("PATCH /api/tasks/{id}", s.handleUpdate)
	s.mux.HandleFunc("DELETE /api/tasks/{id}", s.handleDelete)

	if client != nil {
		s.mux.Handle("/", client)
	}
}

// ── plumbing ────────────────────────────────────────────────────────────────

func (s *Server) json(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("encoding response: %v", err)
	}
}

func (s *Server) fail(w http.ResponseWriter, err error, msg string) {
	if errors.Is(err, store.ErrNotFound) {
		s.errorf(w, http.StatusNotFound, "That task no longer exists.")
		return
	}
	log.Printf("%s: %v", msg, err)
	s.errorf(w, http.StatusInternalServerError, "%s.", msg)
}

func (s *Server) errorf(w http.ResponseWriter, code int, format string, args ...any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": fmt.Sprintf(format, args...)})
}

func pathID(r *http.Request) (int64, error) {
	return strconv.ParseInt(r.PathValue("id"), 10, 64)
}

// decode reads a JSON body into v.
func decode(r *http.Request, v any) error {
	defer r.Body.Close()
	return json.NewDecoder(r.Body).Decode(v)
}

func (s *Server) query(r *http.Request) store.Query {
	q := r.URL.Query()
	return store.Query{
		View:     store.View(q.Get("view")),
		Sort:     store.Sort(q.Get("sort")),
		Topic:    q.Get("topic"),
		Tag:      q.Get("tag"),
		Assignee: q.Get("assignee"),
		Search:   strings.TrimSpace(q.Get("q")),
	}
}

func (s *Server) meta(now time.Time) (Meta, error) {
	m := Meta{Today: now.Format("2006-01-02")}
	counts, err := s.store.Counts(now)
	if err != nil {
		return m, err
	}
	m.Counts = make(map[string]int, len(counts))
	for k, v := range counts {
		m.Counts[string(k)] = v
	}
	topics, err := s.store.Topics()
	if err != nil {
		return m, err
	}
	people, err := s.store.Assignees()
	if err != nil {
		return m, err
	}
	tags, err := s.store.Tags()
	if err != nil {
		return m, err
	}
	m.Topics, m.People, m.Tags = groupDTOs(topics), groupDTOs(people), groupDTOs(tags)
	return m, nil
}

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

	tasks, err := s.store.List(q, now)
	if err != nil {
		s.fail(w, err, "loading the list")
		return
	}
	meta, err := s.meta(now)
	if err != nil {
		s.fail(w, err, "loading the sidebar")
		return
	}

	res := ListResponse{
		View: string(q.View), Sort: string(q.Sort),
		Sections: sections(tasks, q.View, q.Sort, now),
		Total:    len(tasks),
		CanDrag:  q.Sort == store.SortManual && q.View != store.ViewUpcoming && q.View != store.ViewLogbook,
		Meta:     meta,
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
	batch, err := s.store.CreateBatch(res.Tasks, "web", now)
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
			s.errorf(w, http.StatusUnprocessableEntity, "%q is not a date I understand.", *body.Due)
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
