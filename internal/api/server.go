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

	"github.com/drew-mcl/todo/internal/store"
)

// Server routes the JSON API and, behind it, the built client.
type Server struct {
	store   *store.Store
	now     func() time.Time
	mux     *http.ServeMux
	handler http.Handler
}

// New builds a server over st. now is injectable so tests can freeze the clock.
// client serves the built UI; pass nil to run the API alone.
func New(st *store.Store, now func() time.Time, client http.Handler) *Server {
	if now == nil {
		now = time.Now
	}
	s := &Server{store: st, now: now, mux: http.NewServeMux()}
	s.routes(client)
	s.handler = guard(s.mux)
	return s
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.handler.ServeHTTP(w, r)
}

func (s *Server) routes(client http.Handler) {
	s.mux.HandleFunc("GET /api/list", s.handleList)
	s.mux.HandleFunc("GET /api/week", s.handleWeek)
	s.mux.HandleFunc("POST /api/preview", s.handlePreview)
	s.mux.HandleFunc("POST /api/table/preview", s.handleTablePreview)
	s.mux.HandleFunc("POST /api/table/capture", s.handleTableCapture)
	s.mux.HandleFunc("GET /api/sessions", s.handleSessions)
	s.mux.HandleFunc("GET /api/export", s.handleExportAll)
	s.mux.HandleFunc("POST /api/sessions/{id}/rename", s.handleRenameSession)
	s.mux.HandleFunc("GET /api/sessions/{id}/export", s.handleExport)
	s.mux.HandleFunc("POST /api/capture", s.handleCapture)
	s.mux.HandleFunc("POST /api/batches/{id}/undo", s.handleUndo)
	s.mux.HandleFunc("POST /api/tasks/{id}/toggle", s.handleToggle)
	s.mux.HandleFunc("POST /api/tasks/{id}/move", s.handleMove)
	s.mux.HandleFunc("POST /api/tasks/{id}/schedule", s.handleSchedule)
	s.mux.HandleFunc("PATCH /api/tasks/{id}", s.handleUpdate)
	s.mux.HandleFunc("DELETE /api/tasks/{id}", s.handleDelete)

	s.mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		s.errorf(w, http.StatusNotFound, "There is no %s %s.", r.Method, r.URL.Path)
	})

	if client != nil {
		s.mux.Handle("/", client)
	}
}

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

// fieldError names the input that was refused, so the interface can mark it
// rather than showing a sentence next to a form with nothing highlighted.
func (s *Server) fieldError(w http.ResponseWriter, field, format string, args ...any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusUnprocessableEntity)
	json.NewEncoder(w).Encode(map[string]string{
		"error": fmt.Sprintf(format, args...),
		"field": field,
	})
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
	out := store.Query{
		View:     store.View(q.Get("view")),
		Sort:     store.Sort(q.Get("sort")),
		Topic:    q.Get("topic"),
		Tag:      q.Get("tag"),
		Assignee: q.Get("assignee"),
		Search:   strings.TrimSpace(q.Get("q")),
	}
	if b, err := strconv.ParseInt(q.Get("batch"), 10, 64); err == nil && b > 0 {
		out.Batch = b
	}
	out.From, out.To = whenRange(q.Get("when"), s.now())
	return out
}

// whenRange turns a named period into a captured-date range. Named periods
// rather than a date picker: mid-week you think "last week", not "1 to 7 Sep".
func whenRange(when string, now time.Time) (from, to string) {
	day := func(t time.Time) string { return t.Format("2006-01-02") }
	today := truncate(now)
	switch when {
	case "today":
		return day(today), day(today)
	case "yesterday":
		y := today.AddDate(0, 0, -1)
		return day(y), day(y)
	case "week":
		return day(store.WeekStart(today)), day(today)
	case "lastweek":
		start := store.WeekStart(today).AddDate(0, 0, -7)
		return day(start), day(start.AddDate(0, 0, 6))
	case "month":
		return day(time.Date(today.Year(), today.Month(), 1, 0, 0, 0, 0, today.Location())), day(today)
	default:
		return "", ""
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
	if m.DoneToday, err = s.store.DoneOn(now); err != nil {
		return m, err
	}
	m.TodayLabel = strings.ToLower(now.Format("Monday 2 January"))
	m.Topics, m.People, m.Tags = groupDTOs(topics), groupDTOs(people), groupDTOs(tags)
	return m, nil
}

// maxPerCapture is a sanity limit, not a quota. A paste this large is a mistake
// -- a whole document instead of the meeting notes -- and silently creating
// thousands of tasks is a worse outcome than refusing.
const maxPerCapture = 500

func checkBatchSize(n int) error {
	if n > maxPerCapture {
		return fmt.Errorf(
			"that would create %d tasks, which looks like the wrong paste (the limit is %d)",
			n, maxPerCapture)
	}
	return nil
}

// listLimit caps a single response. The count sent alongside is the real one.
const listLimit = 500
