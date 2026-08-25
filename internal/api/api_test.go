package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/drew-mcl/todo/internal/parse"
	"github.com/drew-mcl/todo/internal/store"
)

var now = time.Date(2026, time.August, 25, 9, 0, 0, 0, time.UTC)

const blob = `prod issue | find out why alerts fired | today @sam !!
> 3am spike, check the ingest lag
admin | quarterly training | eow #compliance
some tool | update the stale value @jo
personal | book the dentist`

func newServer(t *testing.T) (*Server, *store.Store) {
	t.Helper()
	st, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	return New(st, func() time.Time { return now }, nil), st
}

func seeded(t *testing.T) (*Server, *store.Store) {
	t.Helper()
	srv, st := newServer(t)
	if _, err := st.CreateBatch(parse.Parse(blob, now).Tasks, "test", now); err != nil {
		t.Fatalf("CreateBatch: %v", err)
	}
	return srv, st
}

// do runs a request and decodes the JSON body into out (which may be nil).
func do(t *testing.T, srv *Server, method, path string, body, out any) int {
	t.Helper()
	var rdr *bytes.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		rdr = bytes.NewReader(b)
	} else {
		rdr = bytes.NewReader(nil)
	}
	r := httptest.NewRequest(method, path, rdr)
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, r)

	if out != nil && w.Body.Len() > 0 {
		if err := json.Unmarshal(w.Body.Bytes(), out); err != nil {
			t.Fatalf("%s %s returned undecodable JSON: %v\n%s", method, path, err, w.Body.String())
		}
	}
	return w.Code
}

func titles(s []Section) []string {
	var out []string
	for _, sec := range s {
		for _, t := range sec.Tasks {
			out = append(out, t.Title)
		}
	}
	return out
}

func TestListShapesEveryView(t *testing.T) {
	srv, _ := seeded(t)
	for _, v := range store.Views {
		var res ListResponse
		if code := do(t, srv, "GET", "/api/list?view="+string(v), nil, &res); code != http.StatusOK {
			t.Fatalf("GET view=%s = %d", v, code)
		}
		if res.View != string(v) {
			t.Errorf("view = %q, want %q", res.View, v)
		}
		if res.Sections == nil {
			t.Errorf("view %s returned a null sections array; the client expects []", v)
		}
		if res.Meta.Counts == nil {
			t.Errorf("view %s returned no sidebar counts", v)
		}
	}
	if code := do(t, srv, "GET", "/api/list?view=nonsense", nil, nil); code != http.StatusNotFound {
		t.Errorf("unknown view = %d, want 404", code)
	}
}

func TestListCarriesEverythingTheRowNeeds(t *testing.T) {
	srv, _ := seeded(t)
	var res ListResponse
	do(t, srv, "GET", "/api/list?view=today", nil, &res)

	var found *Task
	for _, s := range res.Sections {
		for i := range s.Tasks {
			if s.Tasks[i].Title == "find out why alerts fired" {
				found = &s.Tasks[i]
			}
		}
	}
	if found == nil {
		t.Fatal("today is missing the dated task")
	}
	if found.DueLabel != "Today" {
		t.Errorf("dueLabel = %q; the client must not have to format dates", found.DueLabel)
	}
	if found.Assignee != "sam" || found.Priority != 2 || found.Note != "3am spike, check the ingest lag" {
		t.Errorf("row detail lost: %+v", found)
	}
	if found.CapturedWhen == "" || found.CapturedAt != "2026-08-25" {
		t.Errorf("provenance missing: capturedAt=%q capturedWhen=%q", found.CapturedAt, found.CapturedWhen)
	}
	if found.Tags == nil {
		t.Error("tags is null; the client expects an array")
	}
}

func TestListOverdueSection(t *testing.T) {
	srv, st := seeded(t)
	st.CreateBatch(parse.Parse("admin | late thing | 20/8/2026", now).Tasks, "test", now)

	var res ListResponse
	do(t, srv, "GET", "/api/list?view=today", nil, &res)
	if len(res.Sections) < 2 || res.Sections[0].Label != "overdue" {
		t.Fatalf("expected an overdue section first, got %+v", res.Sections)
	}
	if !res.Sections[0].Tasks[0].Overdue {
		t.Error("the overdue task is not flagged overdue")
	}
}

func TestListCanDrag(t *testing.T) {
	srv, _ := seeded(t)
	for _, tc := range []struct {
		path string
		want bool
	}{
		{"/api/list?view=all", true},
		{"/api/list?view=all&sort=due", false},
		{"/api/list?view=upcoming", false},
		{"/api/list?view=logbook", false},
	} {
		var res ListResponse
		do(t, srv, "GET", tc.path, nil, &res)
		if res.CanDrag != tc.want {
			t.Errorf("%s canDrag = %v, want %v", tc.path, res.CanDrag, tc.want)
		}
	}
}

// TestPreviewIsServerSide pins the rule the whole project runs on: there is one
// parser, it is in Go, and the client only renders what it is told.
func TestPreviewIsServerSide(t *testing.T) {
	srv, _ := newServer(t)
	var res PreviewResponse
	code := do(t, srv, "POST", "/api/preview",
		map[string]string{"draft": blob + "\nprose from the call\n\" | repeats the topic"}, &res)
	if code != http.StatusOK {
		t.Fatalf("preview = %d", code)
	}
	if res.Tasks != 5 || res.Notes != 1 || res.Skipped != 1 {
		t.Errorf("counts = %d tasks, %d notes, %d skipped; want 5, 1, 1",
			res.Tasks, res.Notes, res.Skipped)
	}

	kinds := map[string]bool{}
	var ditto *PreviewLine
	for i, l := range res.Lines {
		for _, tok := range l.Tokens {
			kinds[tok.Kind] = true
		}
		if l.Kind == "skipped" && l.Reason == "" {
			t.Errorf("line %d skipped without a reason to show", l.N)
		}
		if l.Task != nil && l.Task.Topic == "some tool" {
			_ = i
		}
	}
	for _, want := range []string{"topic", "pipe", "due", "who", "pri", "tag", "note"} {
		if !kinds[want] {
			t.Errorf("the highlighter never emitted a %q token", want)
		}
	}
	for i := range res.Lines {
		if res.Lines[i].Raw == `" | repeats the topic` {
			ditto = &res.Lines[i]
		}
	}
	if ditto == nil || ditto.Task == nil || ditto.Task.Topic != "personal" {
		t.Errorf("the ditto mark did not resolve to the topic above: %+v", ditto)
	}
}

func TestCaptureAndUndo(t *testing.T) {
	srv, st := newServer(t)
	var cap struct {
		BatchID int64 `json:"batchId"`
		Added   int   `json:"added"`
	}
	if code := do(t, srv, "POST", "/api/capture", map[string]string{"draft": blob}, &cap); code != http.StatusOK {
		t.Fatalf("capture = %d", code)
	}
	if cap.Added != 4 || cap.BatchID == 0 {
		t.Fatalf("capture returned %+v, want 4 added and a batch id", cap)
	}

	if code := do(t, srv, "POST", "/api/capture", map[string]string{"draft": "just prose"}, nil); code != http.StatusUnprocessableEntity {
		t.Errorf("capturing unreadable text = %d, want 422", code)
	}

	var undo struct{ Removed int }
	do(t, srv, "POST", "/api/batches/"+itoa(cap.BatchID)+"/undo", nil, &undo)
	if undo.Removed != 4 {
		t.Errorf("undo removed %d, want 4", undo.Removed)
	}
	if left, _ := st.List(store.Query{View: store.ViewAll}, now); len(left) != 0 {
		t.Errorf("%d tasks survived the undo", len(left))
	}
}

func TestToggleUpdateDelete(t *testing.T) {
	srv, st := seeded(t)
	all, _ := st.List(store.Query{View: store.ViewAll}, now)
	id := itoa(all[0].ID)

	var got Task
	do(t, srv, "POST", "/api/tasks/"+id+"/toggle", nil, &got)
	if !got.Done {
		t.Error("toggle did not complete the task")
	}
	do(t, srv, "POST", "/api/tasks/"+id+"/toggle", nil, &got)
	if got.Done {
		t.Error("toggling back did not reopen the task")
	}

	// A partial patch must not wipe the fields it omits.
	do(t, srv, "PATCH", "/api/tasks/"+id, map[string]any{"due": "eow"}, &got)
	if got.Due == nil || *got.Due != "2026-08-28" {
		t.Errorf("due = %v, want the shorthand read as 2026-08-28", got.Due)
	}
	if got.Assignee != "sam" {
		t.Errorf("assignee = %q; a partial patch wiped it", got.Assignee)
	}
	do(t, srv, "PATCH", "/api/tasks/"+id, map[string]any{"due": ""}, &got)
	if got.Due != nil {
		t.Errorf("due = %v, want it cleared", got.Due)
	}
	if code := do(t, srv, "PATCH", "/api/tasks/"+id, map[string]any{"due": "wednesbury"}, nil); code != http.StatusUnprocessableEntity {
		t.Errorf("an unreadable date = %d, want 422", code)
	}

	if code := do(t, srv, "DELETE", "/api/tasks/"+id, nil, nil); code != http.StatusNoContent {
		t.Errorf("delete = %d, want 204", code)
	}
	if code := do(t, srv, "DELETE", "/api/tasks/9999", nil, nil); code != http.StatusNotFound {
		t.Errorf("deleting a missing task = %d, want 404", code)
	}
}

func TestWeekBoard(t *testing.T) {
	srv, st := seeded(t)
	st.CreateBatch(parse.Parse("admin | long overdue | 1/8/2026", now).Tasks, "test", now)

	var res WeekResponse
	if code := do(t, srv, "GET", "/api/week", nil, &res); code != http.StatusOK {
		t.Fatalf("week = %d", code)
	}
	if res.Start != "2026-08-24" || len(res.Days) != 7 {
		t.Fatalf("week starts %s with %d days, want 2026-08-24 and 7", res.Start, len(res.Days))
	}
	if res.Range != "24–30 aug 2026" {
		t.Errorf("range = %q", res.Range)
	}
	if !res.Days[1].IsToday || res.Days[0].IsToday {
		t.Error("today is not marked on the right column")
	}
	if !res.Days[0].IsPast {
		t.Error("monday should be marked past")
	}
	if len(res.Overdue) != 1 || res.Overdue[0].Title != "long overdue" {
		t.Errorf("overdue tray = %v", res.Overdue)
	}
	if len(res.Unscheduled) != 2 {
		t.Errorf("unscheduled tray has %d, want 2", len(res.Unscheduled))
	}

	// Dropping a card onto a day is the same act as setting a due date.
	var moved Task
	do(t, srv, "POST", "/api/tasks/"+itoa(res.Unscheduled[0].ID)+"/schedule",
		map[string]string{"date": "2026-08-27"}, &moved)
	if moved.Due == nil || *moved.Due != "2026-08-27" {
		t.Errorf("schedule put it on %v, want 2026-08-27", moved.Due)
	}
	do(t, srv, "POST", "/api/tasks/"+itoa(moved.ID)+"/schedule", map[string]string{"date": ""}, &moved)
	if moved.Due != nil {
		t.Errorf("dropping into a tray left due = %v", moved.Due)
	}
}

func TestWeekFilter(t *testing.T) {
	srv, _ := seeded(t)
	var res WeekResponse
	do(t, srv, "GET", "/api/week?topic=personal", nil, &res)
	if len(res.Unscheduled) != 1 || res.Unscheduled[0].Title != "book the dentist" {
		t.Errorf("filtered planner = %v", res.Unscheduled)
	}
}

func TestMove(t *testing.T) {
	srv, st := seeded(t)
	before, _ := st.List(store.Query{View: store.ViewAll, Sort: store.SortManual}, now)
	code := do(t, srv, "POST", "/api/tasks/"+itoa(before[3].ID)+"/move",
		map[string]int64{"Above": before[0].ID, "Below": before[1].ID}, nil)
	if code != http.StatusNoContent {
		t.Fatalf("move = %d", code)
	}
	after, _ := st.List(store.Query{View: store.ViewAll, Sort: store.SortManual}, now)
	if after[1].ID != before[3].ID {
		t.Errorf("after the move the second row is %q", after[1].Title)
	}
}

func TestMetaFeedsTheSidebar(t *testing.T) {
	srv, _ := seeded(t)
	var res ListResponse
	do(t, srv, "GET", "/api/list?view=all", nil, &res)

	if res.Meta.Counts["today"] != 1 || res.Meta.Counts["delegated"] != 2 {
		t.Errorf("counts = %v", res.Meta.Counts)
	}
	if len(res.Meta.Topics) != 4 {
		t.Errorf("topics = %v, want 4", res.Meta.Topics)
	}
	// Equal counts, so the tie breaks alphabetically.
	if len(res.Meta.People) != 2 || res.Meta.People[0].Name != "jo" {
		t.Errorf("people = %v", res.Meta.People)
	}
	if res.Meta.Today != "2026-08-25" {
		t.Errorf("today = %q", res.Meta.Today)
	}
}

func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	neg := n < 0
	if neg {
		n = -n
	}
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	if neg {
		b = append([]byte{'-'}, b...)
	}
	return string(b)
}
