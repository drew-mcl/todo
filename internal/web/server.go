// Package web serves the task list and the capture sheet.
package web

import (
	"embed"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/drew-mcl/todo/internal/store"
)

//go:embed templates/*.html
var templateFS embed.FS

//go:embed static
var staticFS embed.FS

// Server handles every route.
type Server struct {
	store *store.Store
	tmpl  *template.Template
	now   func() time.Time
	mux   *http.ServeMux
}

// New builds a server over st. now is injectable so tests can freeze the clock.
func New(st *store.Store, now func() time.Time) (*Server, error) {
	if now == nil {
		now = time.Now
	}
	fm := funcs(now)
	for k, v := range weekFuncs(now) {
		fm[k] = v
	}
	tmpl, err := template.New("").Funcs(fm).ParseFS(templateFS, "templates/*.html")
	if err != nil {
		return nil, fmt.Errorf("parsing templates: %w", err)
	}

	s := &Server{store: st, tmpl: tmpl, now: now, mux: http.NewServeMux()}
	s.routes()
	return s, nil
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) { s.mux.ServeHTTP(w, r) }

func (s *Server) routes() {
	s.mux.Handle("GET /static/", http.FileServerFS(staticFS))

	s.mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/view/today", http.StatusFound)
	})
	s.mux.HandleFunc("GET /view/{name}", s.handleList)
	s.mux.HandleFunc("GET /week", s.handleWeek)

	s.mux.HandleFunc("POST /capture/preview", s.handlePreview)
	s.mux.HandleFunc("POST /capture", s.handleCapture)
	s.mux.HandleFunc("POST /batches/{id}/undo", s.handleUndoBatch)

	s.mux.HandleFunc("POST /tasks/{id}/toggle", s.handleToggle)
	s.mux.HandleFunc("POST /tasks/{id}/move", s.handleMove)
	s.mux.HandleFunc("POST /tasks/{id}/schedule", s.handleSchedule)
	s.mux.HandleFunc("POST /tasks/{id}", s.handleUpdate)
	s.mux.HandleFunc("GET /tasks/{id}", s.handleDetail)
	s.mux.HandleFunc("POST /tasks/{id}/delete", s.handleDelete)
}

// render writes one named template, buffering first so a mid-render failure does
// not emit half a page.
func (s *Server) render(w http.ResponseWriter, name string, data any) {
	var buf strings.Builder
	if err := s.tmpl.ExecuteTemplate(&buf, name, data); err != nil {
		log.Printf("render %s: %v", name, err)
		http.Error(w, "Something went wrong rendering the page.", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, buf.String())
}

// pathID reads the {id} path value.
func pathID(r *http.Request) (int64, error) {
	return strconv.ParseInt(r.PathValue("id"), 10, 64)
}

// fail maps a store error onto a response.
func (s *Server) fail(w http.ResponseWriter, err error, msg string) {
	if err == store.ErrNotFound {
		http.Error(w, "That task no longer exists.", http.StatusNotFound)
		return
	}
	log.Printf("%s: %v", msg, err)
	http.Error(w, msg, http.StatusInternalServerError)
}
