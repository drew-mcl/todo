package web

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
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

	srv, err := New(st, func() time.Time { return now })
	if err != nil {
		t.Fatalf("web.New: %v", err)
	}
	return srv, st
}

func seeded(t *testing.T) (*Server, *store.Store) {
	t.Helper()
	srv, st := newServer(t)
	if _, err := st.CreateBatch(parse.Parse(blob, now).Tasks, "test", now); err != nil {
		t.Fatalf("CreateBatch: %v", err)
	}
	return srv, st
}

func get(t *testing.T, srv *Server, path string) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
	return w
}

func post(t *testing.T, srv *Server, path string, form url.Values) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(http.MethodPost, path, strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, r)
	return w
}

func mustContain(t *testing.T, body, want, why string) {
	t.Helper()
	if !strings.Contains(body, want) {
		t.Errorf("%s: response is missing %q", why, want)
	}
}

// TestEveryViewRenders is the guard against template execution errors, which the
// compiler cannot catch.
func TestEveryViewRenders(t *testing.T) {
	srv, _ := seeded(t)
	for _, v := range store.Views {
		w := get(t, srv, "/view/"+string(v))
		if w.Code != http.StatusOK {
			t.Fatalf("GET /view/%s = %d\n%s", v, w.Code, w.Body.String())
		}
		mustContain(t, w.Body.String(), "</html>", "view "+string(v))
	}
}

func TestEveryEmptyViewRenders(t *testing.T) {
	srv, _ := newServer(t)
	for _, v := range store.Views {
		w := get(t, srv, "/view/"+string(v))
		if w.Code != http.StatusOK {
			t.Fatalf("GET empty /view/%s = %d\n%s", v, w.Code, w.Body.String())
		}
		mustContain(t, w.Body.String(), "empty", "empty state for "+string(v))
	}
}

func TestEverySortRenders(t *testing.T) {
	srv, _ := seeded(t)
	for _, s := range store.Sorts {
		w := get(t, srv, "/view/all?sort="+string(s))
		if w.Code != http.StatusOK {
			t.Fatalf("sort %s = %d\n%s", s, w.Code, w.Body.String())
		}
	}
}

func TestTodayShowsWhatMatters(t *testing.T) {
	srv, _ := seeded(t)
	body := get(t, srv, "/view/today").Body.String()

	mustContain(t, body, "find out why alerts fired", "today list")
	mustContain(t, body, `assignee=sam`, "delegated tasks stay inline, not in a silo")
	mustContain(t, body, "3am spike", "the note travels with the task")
	if strings.Contains(body, "book the dentist") {
		t.Error("an undated task appeared in Today")
	}
}

func TestUnknownViewIs404(t *testing.T) {
	srv, _ := seeded(t)
	if w := get(t, srv, "/view/nonsense"); w.Code != http.StatusNotFound {
		t.Errorf("GET /view/nonsense = %d, want 404", w.Code)
	}
}

// TestPreviewIsServerRendered pins the reason htmx is here: the preview shows
// the real parse, highlighted with the real token rules.
func TestPreviewIsServerRendered(t *testing.T) {
	srv, _ := newServer(t)
	w := post(t, srv, "/capture/preview", url.Values{"draft": {blob + "\nprose from the call"}})
	if w.Code != http.StatusOK {
		t.Fatalf("preview = %d\n%s", w.Code, w.Body.String())
	}
	body := w.Body.String()

	mustContain(t, body, "4 tasks", "summary counts tasks")
	mustContain(t, body, "1 note", "summary counts notes")
	mustContain(t, body, "1 skipped", "summary counts skipped lines")
	mustContain(t, body, `class="sg-due"`, "dates are highlighted")
	mustContain(t, body, `class="sg-who"`, "assignees are highlighted")
	mustContain(t, body, `class="sg-pri"`, "priority marks are highlighted")
	mustContain(t, body, `class="sg-tag"`, "tags are highlighted")
	mustContain(t, body, "no topic separator", "skipped lines say why")
	mustContain(t, body, "data-promote", "skipped lines can be promoted")
	mustContain(t, body, "Today", "the parsed date is rendered for reading")
}

func TestPreviewOfNothing(t *testing.T) {
	srv, _ := newServer(t)
	w := post(t, srv, "/capture/preview", url.Values{"draft": {""}})
	if w.Code != http.StatusOK {
		t.Fatalf("empty preview = %d", w.Code)
	}
	mustContain(t, w.Body.String(), "appear here", "empty preview invites a paste")
}

func TestCaptureCommits(t *testing.T) {
	srv, st := newServer(t)
	w := post(t, srv, "/capture", url.Values{"draft": {blob}, "view": {"today"}})
	if w.Code != http.StatusSeeOther {
		t.Fatalf("capture = %d\n%s", w.Code, w.Body.String())
	}

	loc := w.Header().Get("Location")
	if !strings.Contains(loc, "undo=") || !strings.Contains(loc, "n=4") {
		t.Errorf("redirect = %q, want an undo token and a count", loc)
	}
	tasks, _ := st.List(store.Query{View: store.ViewAll}, now)
	if len(tasks) != 4 {
		t.Fatalf("stored %d tasks, want 4", len(tasks))
	}
	// And the toast that offers the undo actually renders.
	mustContain(t, get(t, srv, loc).Body.String(), "Undo", "undo toast")
}

func TestCaptureOfUnreadableTextKeepsTheDraft(t *testing.T) {
	srv, _ := newServer(t)
	w := post(t, srv, "/capture", url.Values{
		"draft": {"just some prose\nand more prose"}, "view": {"today"},
	})
	if w.Code != http.StatusOK {
		t.Fatalf("capture of prose = %d, want the sheet back", w.Code)
	}
	mustContain(t, w.Body.String(), "just some prose", "the draft survives")
	mustContain(t, w.Body.String(), "data-capture-open", "the sheet reopens")
}

func TestUndoBatch(t *testing.T) {
	srv, st := newServer(t)
	post(t, srv, "/capture", url.Values{"draft": {blob}, "view": {"today"}})

	tasks, _ := st.List(store.Query{View: store.ViewAll}, now)
	batch := *tasks[0].BatchID

	w := post(t, srv, "/batches/"+strconv.FormatInt(batch, 10)+"/undo", url.Values{"view": {"today"}})
	if w.Code != http.StatusNoContent {
		t.Fatalf("undo = %d", w.Code)
	}
	if w.Header().Get("HX-Redirect") == "" {
		t.Error("undo did not tell the client where to go")
	}
	if left, _ := st.List(store.Query{View: store.ViewAll}, now); len(left) != 0 {
		t.Errorf("%d tasks survived the undo", len(left))
	}
}

func TestToggleSwapsRowAndCounts(t *testing.T) {
	srv, st := seeded(t)
	tasks, _ := st.List(store.Query{View: store.ViewAll}, now)
	id := strconv.FormatInt(tasks[0].ID, 10)

	w := post(t, srv, "/tasks/"+id+"/toggle", url.Values{"view": {"today"}})
	if w.Code != http.StatusOK {
		t.Fatalf("toggle = %d\n%s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	mustContain(t, body, "is-done", "the row comes back completed")
	mustContain(t, body, `data-leaving="1"`, "the row knows it has left this view")
	mustContain(t, body, `hx-swap-oob="true"`, "sidebar counts refresh alongside it")

	if got, _ := st.Get(tasks[0].ID); !got.Done() {
		t.Error("the task was not actually completed")
	}
}

func TestToggleInLogbookDoesNotClaimToLeave(t *testing.T) {
	srv, st := seeded(t)
	tasks, _ := st.List(store.Query{View: store.ViewAll}, now)
	id := strconv.FormatInt(tasks[0].ID, 10)

	// Completing from Today leaves Today; completing again from the Logbook
	// (i.e. reopening) leaves the Logbook.
	post(t, srv, "/tasks/"+id+"/toggle", url.Values{"view": {"today"}})
	body := post(t, srv, "/tasks/"+id+"/toggle", url.Values{"view": {"logbook"}}).Body.String()
	mustContain(t, body, `data-leaving="1"`, "reopening removes it from the Logbook")
	if strings.Contains(body, "is-done") {
		t.Error("the reopened row still renders as done")
	}
}

func TestMove(t *testing.T) {
	srv, st := seeded(t)
	before, _ := st.List(store.Query{View: store.ViewAll, Sort: store.SortManual}, now)

	w := post(t, srv, "/tasks/"+strconv.FormatInt(before[3].ID, 10)+"/move", url.Values{
		"above": {strconv.FormatInt(before[0].ID, 10)},
		"below": {strconv.FormatInt(before[1].ID, 10)},
	})
	if w.Code != http.StatusNoContent {
		t.Fatalf("move = %d\n%s", w.Code, w.Body.String())
	}
	after, _ := st.List(store.Query{View: store.ViewAll, Sort: store.SortManual}, now)
	if after[1].ID != before[3].ID {
		t.Errorf("after the move the second row is %q, want %q", after[1].Title, before[3].Title)
	}
}

func TestDetailAndUpdate(t *testing.T) {
	srv, st := seeded(t)
	tasks, _ := st.List(store.Query{View: store.ViewAll}, now)
	id := strconv.FormatInt(tasks[0].ID, 10)

	body := get(t, srv, "/tasks/"+id+"?view=today").Body.String()
	mustContain(t, body, "find out why alerts fired", "detail shows the task")
	mustContain(t, body, "3am spike", "detail shows the note")
	mustContain(t, body, "Captured as", "detail shows the line as typed")

	w := post(t, srv, "/tasks/"+id, url.Values{
		"title": {"find the actual root cause"},
		"due":   {"eow"},
		"tags":  {"postmortem, sev1"},
		"note":  {"ingest lag again"},
		"view":  {"today"},
	})
	if w.Code != http.StatusOK {
		t.Fatalf("update = %d\n%s", w.Code, w.Body.String())
	}
	got, _ := st.Get(tasks[0].ID)
	if got.Title != "find the actual root cause" {
		t.Errorf("title = %q", got.Title)
	}
	if got.Due == nil || got.Due.Format("2006-01-02") != "2026-08-28" {
		t.Errorf("due = %v, want the shorthand 'eow' to have been read", got.Due)
	}
	if len(got.Tags) != 2 {
		t.Errorf("tags = %v, want 2", got.Tags)
	}
	if got.Assignee != "sam" {
		t.Errorf("assignee = %q; a form that omits a field must not wipe it", got.Assignee)
	}
}

func TestUpdateCanClearTheDueDate(t *testing.T) {
	srv, st := seeded(t)
	tasks, _ := st.List(store.Query{View: store.ViewAll}, now)
	post(t, srv, "/tasks/"+strconv.FormatInt(tasks[0].ID, 10), url.Values{
		"due": {""}, "view": {"today"},
	})
	if got, _ := st.Get(tasks[0].ID); got.Due != nil {
		t.Errorf("due = %v, want it cleared", got.Due)
	}
}

func TestDelete(t *testing.T) {
	srv, st := seeded(t)
	tasks, _ := st.List(store.Query{View: store.ViewAll}, now)
	id := tasks[0].ID

	if w := post(t, srv, "/tasks/"+strconv.FormatInt(id, 10)+"/delete", nil); w.Code != http.StatusOK {
		t.Fatalf("delete = %d", w.Code)
	}
	if _, err := st.Get(id); err != store.ErrNotFound {
		t.Errorf("Get after delete = %v, want ErrNotFound", err)
	}
	if w := post(t, srv, "/tasks/9999/delete", nil); w.Code != http.StatusNotFound {
		t.Errorf("deleting a missing task = %d, want 404", w.Code)
	}
}

func TestFiltersAndSearch(t *testing.T) {
	srv, _ := seeded(t)
	body := get(t, srv, "/view/all?topic=admin").Body.String()
	mustContain(t, body, "quarterly training", "topic filter")
	if strings.Contains(body, "book the dentist") {
		t.Error("the topic filter let another topic through")
	}

	body = get(t, srv, "/view/all?tag=compliance").Body.String()
	mustContain(t, body, "quarterly training", "tag filter")

	body = get(t, srv, "/view/all?q=dentist").Body.String()
	mustContain(t, body, "book the dentist", "search")
	mustContain(t, body, "Clear", "a filtered list offers a way out")
}

func TestStaticAssetsAreEmbedded(t *testing.T) {
	for _, path := range []string{"/static/app.css", "/static/app.js",
		"/static/htmx.min.js", "/static/sortable.min.js"} {
		srv, _ := newServer(t)
		w := get(t, srv, path)
		if w.Code != http.StatusOK || w.Body.Len() == 0 {
			t.Errorf("GET %s = %d, %d bytes", path, w.Code, w.Body.Len())
		}
	}
}

func TestRootRedirects(t *testing.T) {
	srv, _ := newServer(t)
	w := get(t, srv, "/")
	if w.Code != http.StatusFound || w.Header().Get("Location") != "/view/today" {
		t.Errorf("GET / = %d to %q, want 302 to /view/today", w.Code, w.Header().Get("Location"))
	}
}

func TestSidebarListsPeople(t *testing.T) {
	srv, _ := seeded(t)
	body := get(t, srv, "/view/today").Body.String()

	mustContain(t, body, "People", "the people group is headed")
	mustContain(t, body, "assignee=sam", "people are linked as filters")
	mustContain(t, body, `data-section="people"`, "the group is collapsible")
	mustContain(t, body, `data-section="topics"`, "topics are collapsible")
	mustContain(t, body, "nav-pinned", "All is pinned above the quick filters")
}

func TestFilterByAssignee(t *testing.T) {
	srv, _ := seeded(t)
	body := get(t, srv, "/view/all?assignee=jo").Body.String()

	mustContain(t, body, "update the stale value", "jo's task")
	mustContain(t, body, "<h1>Jo</h1>", "the heading names the person")
	if strings.Contains(body, "book the dentist") {
		t.Error("the assignee filter let an unassigned task through")
	}
}

func TestWeekPlannerRenders(t *testing.T) {
	srv, _ := seeded(t)
	w := get(t, srv, "/week")
	if w.Code != http.StatusOK {
		t.Fatalf("GET /week = %d\n%s", w.Code, w.Body.String())
	}
	body := w.Body.String()

	mustContain(t, body, `data-week`, "the planner renders")
	mustContain(t, body, "Unscheduled", "the unscheduled tray")
	mustContain(t, body, "Overdue", "the overdue tray")
	mustContain(t, body, `data-date="2026-08-25"`, "a drop target per day")
	mustContain(t, body, "24–30 Aug 2026", "the week is named")
	mustContain(t, body, "book the dentist", "undated tasks land in the tray")
	if strings.Contains(body, `class="controls"`) {
		t.Error("the search and sort controls have no meaning on the planner")
	}
}

func TestWeekNavigation(t *testing.T) {
	srv, _ := seeded(t)
	body := get(t, srv, "/week?start=2026-09-07").Body.String()
	mustContain(t, body, "7–13 Sep 2026", "an explicit week is honoured")
	// A mid-week date still resolves to that week's Monday.
	mustContain(t, get(t, srv, "/week?start=2026-09-10").Body.String(), "7–13 Sep 2026",
		"any day resolves to its Monday")
}

func TestScheduleByDrop(t *testing.T) {
	srv, st := seeded(t)
	tasks, _ := st.List(store.Query{View: store.ViewAnytime}, now)
	if len(tasks) == 0 {
		t.Fatal("need an undated task")
	}
	id := strconv.FormatInt(tasks[0].ID, 10)

	w := post(t, srv, "/tasks/"+id+"/schedule", url.Values{"date": {"2026-08-27"}})
	if w.Code != http.StatusOK {
		t.Fatalf("schedule = %d\n%s", w.Code, w.Body.String())
	}
	mustContain(t, w.Body.String(), `class="card`, "the moved card comes back")

	got, _ := st.Get(tasks[0].ID)
	if got.Due == nil || got.Due.Format("2006-01-02") != "2026-08-27" {
		t.Fatalf("due = %v, want 2026-08-27", got.Due)
	}

	// Dropping into a tray clears the date again.
	post(t, srv, "/tasks/"+id+"/schedule", url.Values{"date": {""}})
	if got, _ = st.Get(tasks[0].ID); got.Due != nil {
		t.Errorf("due = %v, want it cleared", got.Due)
	}
}

func TestWeekFilterUI(t *testing.T) {
	srv, _ := seeded(t)
	body := get(t, srv, "/week").Body.String()
	mustContain(t, body, "All topics", "the planner offers a topic filter")
	mustContain(t, body, "Anyone", "the planner offers a person filter")

	filtered := get(t, srv, "/week?topic=admin").Body.String()
	mustContain(t, filtered, "quarterly training", "the filtered task survives")
	if strings.Contains(filtered, "book the dentist") {
		t.Error("the week filter let another topic through")
	}
	mustContain(t, filtered, "Clear", "a filtered planner offers a way out")
}
