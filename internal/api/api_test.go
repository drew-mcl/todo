package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
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
	if _, err := st.CreateBatch(parse.Parse(blob, now).Tasks, store.Capture{Source: "test"}, now); err != nil {
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
	r.Host = "127.0.0.1:8765" // httptest defaults to example.com, which the guard refuses
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

// TestTodayIsExactlyToday pins the rule that keeps Today and the week board
// telling the same story: late work is its own list, never today's.
func TestTodayIsExactlyToday(t *testing.T) {
	srv, st := seeded(t)
	st.CreateBatch(parse.Parse("admin | late thing | 20/8/2026", now).Tasks,
		store.Capture{Source: "test"}, now)

	var today ListResponse
	do(t, srv, "GET", "/api/list?view=today", nil, &today)
	for _, got := range titles(today.Sections) {
		if got == "late thing" {
			t.Error("an overdue task appeared in Today; it belongs in overdue")
		}
	}
	if len(titles(today.Sections)) != 1 {
		t.Errorf("today = %v, want only what is due today", titles(today.Sections))
	}

	var overdue ListResponse
	do(t, srv, "GET", "/api/list?view=overdue", nil, &overdue)
	got := titles(overdue.Sections)
	if len(got) != 1 || got[0] != "late thing" {
		t.Errorf("overdue = %v, want the late task", got)
	}
	if !overdue.Sections[0].Tasks[0].Overdue {
		t.Error("the overdue task is not flagged overdue")
	}
	if overdue.CanDrag {
		t.Error("overdue is date-ordered, so manual drag has nothing to mean")
	}
	if overdue.Meta.Counts["overdue"] != 1 {
		t.Errorf("sidebar overdue count = %d, want 1", overdue.Meta.Counts["overdue"])
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
	st.CreateBatch(parse.Parse("admin | long overdue | 1/8/2026", now).Tasks, store.Capture{Source: "test"}, now)

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

// ── Sessions, export and tables ─────────────────────────────────────────────

func TestSessionsAndExport(t *testing.T) {
	srv, st := newServer(t)

	// Two captures, one named after the call it came from.
	var first struct {
		BatchID int64 `json:"batchId"`
	}
	do(t, srv, "POST", "/api/capture",
		map[string]string{"draft": blob, "title": "Platform sync"}, &first)
	do(t, srv, "POST", "/api/capture",
		map[string]string{"draft": "admin | something else"}, nil)

	var sessions []Session
	if code := do(t, srv, "GET", "/api/sessions", nil, &sessions); code != http.StatusOK {
		t.Fatalf("sessions = %d", code)
	}
	if len(sessions) != 2 {
		t.Fatalf("got %d sessions, want 2", len(sessions))
	}
	named := sessions[0]
	if named.ID != first.BatchID {
		named = sessions[1]
	}
	if named.Title != "Platform sync" {
		t.Errorf("title = %q", named.Title)
	}
	if named.Total != 4 || named.Open != 4 || named.Done != 0 {
		t.Errorf("counts = %+v, want 4 total and 4 open", named)
	}
	if named.When == "" || named.Preview == "" {
		t.Errorf("a session must be recognisable: %+v", named)
	}
	// One that was never named still reads as an event, not a blank.
	for _, s := range sessions {
		if s.Title == "" {
			t.Errorf("session %d has no fallback title", s.ID)
		}
	}

	// Complete one, then export what is left.
	tasks, _ := st.List(store.Query{View: store.ViewAll, Batch: first.BatchID}, now)
	do(t, srv, "POST", "/api/tasks/"+itoa(tasks[0].ID)+"/toggle", nil, nil)

	var export struct{ Text string }
	code := do(t, srv, "GET", "/api/sessions/"+itoa(first.BatchID)+"/export", nil, &export)
	if code != http.StatusOK {
		t.Fatalf("export = %d", code)
	}
	text := export.Text
	if !strings.HasPrefix(text, "Platform sync — ") {
		t.Errorf("export does not lead with the call:\n%s", text)
	}
	if !strings.Contains(text, "3 actions") {
		t.Errorf("export miscounts what is outstanding:\n%s", text)
	}
	if strings.Contains(text, tasks[0].Title) {
		t.Errorf("a completed action was exported:\n%s", text)
	}
	// Grouped by who owes it, with mine last.
	if !strings.Contains(text, "\nJo\n") || !strings.Contains(text, "\nMe\n") {
		t.Errorf("export is not grouped by owner:\n%s", text)
	}
	if strings.Index(text, "\nMe\n") < strings.Index(text, "\nJo\n") {
		t.Errorf("mine should come last:\n%s", text)
	}

	flat := struct{ Text string }{}
	do(t, srv, "GET", "/api/sessions/"+itoa(first.BatchID)+"/export?group=none", nil, &flat)
	if strings.Contains(flat.Text, "\nJo\n") {
		t.Errorf("group=none still grouped:\n%s", flat.Text)
	}

	// And it can be renamed after the fact.
	if code := do(t, srv, "POST", "/api/sessions/"+itoa(first.BatchID)+"/rename",
		map[string]string{"title": "Platform sync — week 35"}, nil); code != http.StatusNoContent {
		t.Fatalf("rename = %d", code)
	}
	do(t, srv, "GET", "/api/sessions", nil, &sessions)
	found := false
	for _, s := range sessions {
		if s.Title == "Platform sync — week 35" {
			found = true
		}
	}
	if !found {
		t.Error("the rename did not stick")
	}
}

func TestListFiltersBySession(t *testing.T) {
	srv, _ := newServer(t)
	var a, b struct {
		BatchID int64 `json:"batchId"`
	}
	do(t, srv, "POST", "/api/capture", map[string]string{"draft": "admin | from call a"}, &a)
	do(t, srv, "POST", "/api/capture", map[string]string{"draft": "admin | from call b"}, &b)

	var res ListResponse
	do(t, srv, "GET", "/api/list?view=all&batch="+itoa(a.BatchID), nil, &res)
	if got := titles(res.Sections); len(got) != 1 || got[0] != "from call a" {
		t.Errorf("session filter returned %v", got)
	}
}

const copilotPaste = `| Action | Owner | Deadline | Notes |
| --- | --- | --- | --- |
| Chase the vendor about the patch | Sam Okafor | Friday | Missed two dates |
| Pull the headcount numbers | Jo | 1 Sep | |
| Ship the thing | Priya | today | |`

func TestTablePreviewAndCapture(t *testing.T) {
	srv, st := newServer(t)

	var pv TablePreview
	if code := do(t, srv, "POST", "/api/table/preview",
		map[string]any{"input": copilotPaste, "topic": "platform sync"}, &pv); code != http.StatusOK {
		t.Fatalf("table preview = %d", code)
	}
	if pv.Preset != "copilot" {
		t.Errorf("preset = %q, want copilot", pv.Preset)
	}
	if pv.Format != "markdown" || pv.Tasks != 3 {
		t.Errorf("format=%q tasks=%d", pv.Format, pv.Tasks)
	}
	if len(pv.Columns) != 4 || pv.Columns[1].Role != "owner" {
		t.Errorf("columns = %+v", pv.Columns)
	}
	if len(pv.Roles) == 0 {
		t.Error("the client needs the role list to offer a remapping")
	}
	first := pv.Rows[0].Task
	if first == nil || first.Assignee != "sam" || first.Topic != "platform sync" {
		t.Errorf("first row = %+v", first)
	}
	if first.DueLabel == "" {
		t.Error("friday was not read as a date")
	}

	// A mapping sent from the interface overrides the guess.
	var remapped TablePreview
	do(t, srv, "POST", "/api/table/preview", map[string]any{
		"input":   copilotPaste,
		"mapping": []string{"title", "ignore", "due", "note"},
	}, &remapped)
	if remapped.Rows[0].Task.Assignee != "" {
		t.Error("ignoring the owner column had no effect")
	}

	var out struct {
		BatchID int64 `json:"batchId"`
		Added   int   `json:"added"`
	}
	if code := do(t, srv, "POST", "/api/table/capture", map[string]any{
		"input": copilotPaste, "topic": "platform sync",
		"title": "Platform sync", "source": "copilot",
	}, &out); code != http.StatusOK {
		t.Fatalf("table capture = %d", code)
	}
	if out.Added != 3 {
		t.Errorf("added %d, want 3", out.Added)
	}
	stored, _ := st.List(store.Query{View: store.ViewAll, Batch: out.BatchID}, now)
	if len(stored) != 3 {
		t.Fatalf("stored %d tasks", len(stored))
	}
	if stored[0].Raw == "" {
		t.Error("the source row was not kept")
	}

	// And a paste that is not a table says so rather than importing nonsense.
	if code := do(t, srv, "POST", "/api/table/preview",
		map[string]any{"input": "just some prose"}, nil); code != http.StatusUnprocessableEntity {
		t.Errorf("non-table = %d, want 422", code)
	}
}

func TestWhenFilter(t *testing.T) {
	srv, st := newServer(t)
	// now is Tue 25 Aug 2026, so this week began on Mon 24 Aug.
	seed := func(day string, title string) {
		when, _ := time.Parse("2006-01-02", day)
		st.CreateBatch(parse.Parse("admin | "+title, when).Tasks,
			store.Capture{Source: "test"}, when)
	}
	seed("2026-08-25", "said today")
	seed("2026-08-24", "said monday")
	seed("2026-08-19", "said last week")
	seed("2026-07-30", "said last month")

	for _, tc := range []struct {
		when string
		want []string
	}{
		{"today", []string{"said today"}},
		{"week", []string{"said today", "said monday"}},
		{"lastweek", []string{"said last week"}},
		{"month", []string{"said today", "said monday", "said last week"}},
		{"", []string{"said today", "said monday", "said last week", "said last month"}},
	} {
		var res ListResponse
		do(t, srv, "GET", "/api/list?view=all&sort=created&when="+tc.when, nil, &res)
		got := titles(res.Sections)
		if len(got) != len(tc.want) {
			t.Errorf("when=%q returned %v, want %v", tc.when, got, tc.want)
			continue
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Errorf("when=%q returned %v, want %v", tc.when, got, tc.want)
				break
			}
		}
	}
}

// ── The guard ───────────────────────────────────────────────────────────────

// raw sends a request without the test helper's loopback Host, so the guard is
// exercised the way a browser would exercise it.
func raw(t *testing.T, srv *Server, method, path, host, origin string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(method, path, strings.NewReader(`{"draft":"admin | snuck in"}`))
	r.Header.Set("Content-Type", "application/json")
	r.Host = host
	if origin != "" {
		r.Header.Set("Origin", origin)
	}
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, r)
	return w
}

// TestGuardRefusesOtherSites covers the reason a server on 127.0.0.1 still
// needs a door: the browser will happily carry another page's request to it.
func TestGuardRefusesOtherSites(t *testing.T) {
	srv, st := newServer(t)

	for _, tc := range []struct {
		name, host, origin string
		want               int
	}{
		{"same origin", "127.0.0.1:8765", "http://127.0.0.1:8765", http.StatusOK},
		{"localhost by name", "localhost:8765", "http://localhost:8765", http.StatusOK},
		{"no origin at all", "127.0.0.1:8765", "", http.StatusOK},
		{"another site", "127.0.0.1:8765", "https://evil.example", http.StatusForbidden},
		// A page whose origin looks local but was served on another port is
		// still not this app.
		{"another local port", "127.0.0.1:8765", "http://127.0.0.1:9999", http.StatusForbidden},
		// DNS rebinding: a name the attacker controls, resolved to loopback.
		{"rebound hostname", "evil.example:8765", "http://evil.example:8765", http.StatusForbidden},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := raw(t, srv, "POST", "/api/capture", tc.host, tc.origin)
			if w.Code != tc.want {
				t.Errorf("Host=%q Origin=%q gave %d, want %d", tc.host, tc.origin, w.Code, tc.want)
			}
		})
	}

	// Only the requests that were allowed should have written anything.
	tasks, _ := st.List(store.Query{View: store.ViewAll}, now)
	if len(tasks) != 3 {
		t.Errorf("%d tasks were created; only the three allowed requests should have written",
			len(tasks))
	}
}

func TestGuardCapsTheBody(t *testing.T) {
	srv, _ := newServer(t)
	huge := strings.Repeat("admin | a very long line indeed\n", 400_000) // ~12 MiB
	if code := do(t, srv, "POST", "/api/capture", map[string]string{"draft": huge}, nil); code == http.StatusOK {
		t.Error("a body far past the limit was accepted")
	}
}

func TestCaptureRefusesAnAbsurdNumberOfTasks(t *testing.T) {
	srv, st := newServer(t)
	var lines []string
	for i := 0; i < maxPerCapture+1; i++ {
		lines = append(lines, "admin | task")
	}
	code := do(t, srv, "POST", "/api/capture", map[string]string{"draft": strings.Join(lines, "\n")}, nil)
	if code != http.StatusUnprocessableEntity {
		t.Errorf("capture of %d tasks = %d, want 422", maxPerCapture+1, code)
	}
	if tasks, _ := st.List(store.Query{View: store.ViewAll}, now); len(tasks) != 0 {
		t.Errorf("%d tasks were written despite the refusal", len(tasks))
	}
}

func TestListIsCapped(t *testing.T) {
	srv, st := newServer(t)
	for range 3 {
		var lines []string
		for i := 0; i < 250; i++ {
			lines = append(lines, "admin | task")
		}
		st.CreateBatch(parse.Parse(strings.Join(lines, "\n"), now).Tasks,
			store.Capture{Source: "test"}, now)
	}

	var res ListResponse
	do(t, srv, "GET", "/api/list?view=all", nil, &res)
	if res.Total != 750 {
		t.Errorf("total = %d, want the real count of 750", res.Total)
	}
	if res.Shown != listLimit || !res.Truncated {
		t.Errorf("shown = %d truncated = %v, want %d and true", res.Shown, res.Truncated, listLimit)
	}
	if got := len(titles(res.Sections)); got != listLimit {
		t.Errorf("%d tasks came back, want the response capped at %d", got, listLimit)
	}
}

func TestUnknownApiPathIsJson(t *testing.T) {
	srv, _ := newServer(t)
	for _, tc := range []struct{ method, path string }{
		{"GET", "/api/nope"},
		{"DELETE", "/api/list"},
	} {
		w := httptest.NewRecorder()
		r := httptest.NewRequest(tc.method, tc.path, nil)
		r.Host = "127.0.0.1:8765"
		srv.ServeHTTP(w, r)

		if w.Code != http.StatusNotFound {
			t.Errorf("%s %s = %d, want 404", tc.method, tc.path, w.Code)
		}
		if ct := w.Header().Get("Content-Type"); !strings.Contains(ct, "json") {
			t.Errorf("%s %s returned %s; an API path must never answer with the client",
				tc.method, tc.path, ct)
		}
	}
}

func TestBadDateNamesTheField(t *testing.T) {
	srv, st := seeded(t)
	all, _ := st.List(store.Query{View: store.ViewAll}, now)

	r := httptest.NewRequest("PATCH", "/api/tasks/"+itoa(all[0].ID),
		strings.NewReader(`{"title":"renamed","due":"wednesbury"}`))
	r.Host = "127.0.0.1:8765"
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, r)

	var body map[string]string
	json.Unmarshal(w.Body.Bytes(), &body)
	if body["field"] != "due" {
		t.Errorf("error did not name the offending field: %v", body)
	}
	// Nothing is applied, so the interface can keep the draft and let the user
	// correct one input rather than retype everything.
	if got, _ := st.Get(all[0].ID); got.Title == "renamed" {
		t.Error("a rejected patch applied part of itself")
	}
}
