package store

import (
	"testing"
	"time"

	"github.com/drew-mcl/todo/internal/parse"
)

var now = time.Date(2026, time.August, 25, 9, 0, 0, 0, time.UTC)

func open(t *testing.T) *Store {
	t.Helper()
	s, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

// seed captures a blob through the real parser, exercising the same path the web
// handler uses.
func seed(t *testing.T, s *Store, blob string) int64 {
	t.Helper()
	id, err := s.CreateBatch(parse.Parse(blob, now).Tasks, Capture{Source: "test"}, now)
	if err != nil {
		t.Fatalf("CreateBatch: %v", err)
	}
	return id
}

func titles(tasks []*Task) []string {
	out := make([]string, len(tasks))
	for i, t := range tasks {
		out[i] = t.Title
	}
	return out
}

func eq(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

const blob = `prod issue | find out why alerts fired | today @sam !!
> 3am spike
admin | quarterly training | eow #compliance
some tool | update the stale value @jo
personal | book the dentist`

func TestCaptureRoundTrip(t *testing.T) {
	s := open(t)
	seed(t, s, blob)

	all, err := s.List(Query{View: ViewAll, Sort: SortManual}, now)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(all) != 4 {
		t.Fatalf("got %d tasks, want 4", len(all))
	}
	// Pasted order is preserved, and the batch sits above anything older.
	eq(t, titles(all), []string{
		"find out why alerts fired", "quarterly training",
		"update the stale value", "book the dentist",
	})

	first := all[0]
	if first.Note != "3am spike" || first.Assignee != "sam" || first.Priority != parse.PriUrgent {
		t.Errorf("round trip lost detail: note=%q assignee=%q pri=%v",
			first.Note, first.Assignee, first.Priority)
	}
	if first.Due == nil || first.Due.Format(dateLayout) != "2026-08-25" {
		t.Errorf("due = %v, want 2026-08-25", first.Due)
	}
	if len(all[1].Tags) != 1 || all[1].Tags[0] != "compliance" {
		t.Errorf("tags = %v, want [compliance]", all[1].Tags)
	}
}

func TestNewCaptureLandsOnTop(t *testing.T) {
	s := open(t)
	seed(t, s, "admin | first thing")
	seed(t, s, "admin | second thing")

	all, _ := s.List(Query{View: ViewAll, Sort: SortManual}, now)
	eq(t, titles(all), []string{"second thing", "first thing"})
}

func TestViews(t *testing.T) {
	s := open(t)
	seed(t, s, blob+"\nadmin | overdue thing | 20/8/2026")

	for _, tc := range []struct {
		view View
		want []string
	}{
		// Today is exactly today; what slipped has its own list.
		{ViewToday, []string{"find out why alerts fired"}},
		{ViewOverdue, []string{"overdue thing"}},
		{ViewUpcoming, []string{"quarterly training"}},
		{ViewAnytime, []string{"update the stale value", "book the dentist"}},
		// Delegated is a filter, not a silo: these also appear above.
		{ViewDelegated, []string{"find out why alerts fired", "update the stale value"}},
		{ViewLogbook, nil},
	} {
		got, err := s.List(Query{View: tc.view, Sort: SortDue}, now)
		if err != nil {
			t.Fatalf("List(%s): %v", tc.view, err)
		}
		eq(t, titles(got), tc.want)
	}
}

func TestCounts(t *testing.T) {
	s := open(t)
	seed(t, s, blob)
	c, err := s.Counts(now)
	if err != nil {
		t.Fatalf("Counts: %v", err)
	}
	if c[ViewToday] != 1 || c[ViewUpcoming] != 1 || c[ViewAnytime] != 2 || c[ViewDelegated] != 2 {
		t.Errorf("counts = %v; want today=1 upcoming=1 anytime=2 delegated=2", c)
	}
}

func TestToggleAndLogbook(t *testing.T) {
	s := open(t)
	seed(t, s, blob)
	all, _ := s.List(Query{View: ViewAll}, now)

	done, err := s.Toggle(all[0].ID, now)
	if err != nil {
		t.Fatalf("Toggle: %v", err)
	}
	if !done.Done() || done.CompletedAt == nil {
		t.Fatalf("task not completed: status=%s completed=%v", done.Status, done.CompletedAt)
	}
	log, _ := s.List(Query{View: ViewLogbook}, now)
	eq(t, titles(log), []string{"find out why alerts fired"})

	reopened, err := s.Toggle(all[0].ID, now)
	if err != nil {
		t.Fatalf("Toggle back: %v", err)
	}
	if reopened.Done() || reopened.CompletedAt != nil {
		t.Errorf("undo left the task done: status=%s completed=%v", reopened.Status, reopened.CompletedAt)
	}
}

func TestSorts(t *testing.T) {
	s := open(t)
	seed(t, s, blob)

	byPri, _ := s.List(Query{View: ViewAll, Sort: SortPriority}, now)
	if byPri[0].Title != "find out why alerts fired" {
		t.Errorf("priority sort put %q first", byPri[0].Title)
	}
	byTopic, _ := s.List(Query{View: ViewAll, Sort: SortTopic}, now)
	eq(t, titles(byTopic), []string{
		"quarterly training", "book the dentist",
		"find out why alerts fired", "update the stale value",
	})
	byDue, _ := s.List(Query{View: ViewAll, Sort: SortDue}, now)
	if byDue[len(byDue)-1].Due != nil {
		t.Error("due sort should push undated tasks to the end")
	}
}

func TestFilters(t *testing.T) {
	s := open(t)
	seed(t, s, blob)

	byTopic, _ := s.List(Query{View: ViewAll, Topic: "admin"}, now)
	eq(t, titles(byTopic), []string{"quarterly training"})

	byTag, _ := s.List(Query{View: ViewAll, Tag: "compliance"}, now)
	eq(t, titles(byTag), []string{"quarterly training"})

	found, _ := s.List(Query{View: ViewAll, Search: "dentist"}, now)
	eq(t, titles(found), []string{"book the dentist"})

	// Search covers notes too, not just titles.
	inNote, _ := s.List(Query{View: ViewAll, Search: "3am"}, now)
	eq(t, titles(inNote), []string{"find out why alerts fired"})
}

func TestTopicsAndTags(t *testing.T) {
	s := open(t)
	seed(t, s, blob+"\nadmin | another admin thing #compliance")

	topics, err := s.Topics()
	if err != nil {
		t.Fatalf("Topics: %v", err)
	}
	if topics[0].Name != "admin" || topics[0].Count != 2 {
		t.Errorf("topics[0] = %+v, want admin with 2", topics[0])
	}
	tags, err := s.Tags()
	if err != nil {
		t.Fatalf("Tags: %v", err)
	}
	if len(tags) != 1 || tags[0].Name != "compliance" || tags[0].Count != 2 {
		t.Errorf("tags = %+v, want compliance with 2", tags)
	}
}

func TestMove(t *testing.T) {
	s := open(t)
	seed(t, s, "admin | a\nadmin | b\nadmin | c\nadmin | d")
	ids := func() []*Task {
		l, _ := s.List(Query{View: ViewAll, Sort: SortManual}, now)
		return l
	}

	l := ids()
	// Drag "d" up between "a" and "b".
	if err := s.Move(l[3].ID, l[0].ID, l[1].ID); err != nil {
		t.Fatalf("Move: %v", err)
	}
	eq(t, titles(ids()), []string{"a", "d", "b", "c"})

	l = ids()
	// Drag "c" to the very top: no task above it.
	if err := s.Move(l[3].ID, 0, l[0].ID); err != nil {
		t.Fatalf("Move to top: %v", err)
	}
	eq(t, titles(ids()), []string{"c", "a", "d", "b"})

	l = ids()
	// Drag "c" to the very bottom: no task below it.
	if err := s.Move(l[0].ID, l[3].ID, 0); err != nil {
		t.Fatalf("Move to bottom: %v", err)
	}
	eq(t, titles(ids()), []string{"a", "d", "b", "c"})
}

// TestMoveRebalances forces the float gap shut and checks the renumbering keeps
// the list intact rather than collapsing rows onto one position.
func TestMoveRebalances(t *testing.T) {
	s := open(t)
	seed(t, s, "admin | a\nadmin | b\nadmin | c")
	l, _ := s.List(Query{View: ViewAll, Sort: SortManual}, now)

	if _, err := s.db.Exec("UPDATE tasks SET position = 0 WHERE id = ?", l[0].ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec("UPDATE tasks SET position = 1e-9 WHERE id = ?", l[1].ID); err != nil {
		t.Fatal(err)
	}
	if err := s.Move(l[2].ID, l[0].ID, l[1].ID); err != nil {
		t.Fatalf("Move across a closed gap: %v", err)
	}
	eq(t, titles(ids(s)), []string{"a", "c", "b"})

	var distinct int
	if err := s.db.QueryRow("SELECT count(DISTINCT position) FROM tasks").Scan(&distinct); err != nil {
		t.Fatal(err)
	}
	if distinct != 3 {
		t.Errorf("got %d distinct positions after rebalance, want 3", distinct)
	}
}

func ids(s *Store) []*Task {
	l, _ := s.List(Query{View: ViewAll, Sort: SortManual}, now)
	return l
}

func TestUpdate(t *testing.T) {
	s := open(t)
	seed(t, s, blob)
	l, _ := s.List(Query{View: ViewAll}, now)
	id := l[0].ID

	title, who := "find the actual root cause", ""
	tags := []string{"postmortem", "sev1"}
	got, err := s.Update(id, Patch{Title: &title, Assignee: &who, ClearDue: true, Tags: &tags})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if got.Title != title {
		t.Errorf("title = %q, want %q", got.Title, title)
	}
	if got.Assignee != "" {
		t.Errorf("assignee = %q, want it cleared", got.Assignee)
	}
	if got.Due != nil {
		t.Errorf("due = %v, want it cleared", got.Due)
	}
	if len(got.Tags) != 2 {
		t.Errorf("tags = %v, want 2", got.Tags)
	}
}

func TestUndoBatch(t *testing.T) {
	s := open(t)
	keep := seed(t, s, "admin | keep me")
	drop := seed(t, s, blob)

	n, err := s.UndoBatch(drop)
	if err != nil {
		t.Fatalf("UndoBatch: %v", err)
	}
	if n != 4 {
		t.Errorf("undid %d tasks, want 4", n)
	}
	eq(t, titles(ids(s)), []string{"keep me"})
	if keep == drop {
		t.Error("batches should get distinct ids")
	}
}

func TestMissingTask(t *testing.T) {
	s := open(t)
	if _, err := s.Get(999); err != ErrNotFound {
		t.Errorf("Get(999) = %v, want ErrNotFound", err)
	}
	if err := s.Delete(999); err != ErrNotFound {
		t.Errorf("Delete(999) = %v, want ErrNotFound", err)
	}
}

func TestMigrationIsIdempotent(t *testing.T) {
	s := open(t)
	if err := s.migrate(); err != nil {
		t.Fatalf("re-running migrate: %v", err)
	}
	var v int
	if err := s.db.QueryRow("PRAGMA user_version").Scan(&v); err != nil {
		t.Fatal(err)
	}
	if v != len(migrations) {
		t.Errorf("user_version = %d, want %d", v, len(migrations))
	}
}

// TestUpcomingIsChronological guards the grouping: day headings only make sense
// if the list actually runs in date order, whatever sort is asked for.
func TestUpcomingIsChronological(t *testing.T) {
	s := open(t)
	seed(t, s, `admin | later thing | +2w
admin | sooner thing | tomorrow
admin | middle thing | +3d
admin | another sooner thing | tomorrow`)

	for _, sort := range []Sort{SortManual, SortCreated, SortPriority, SortTopic} {
		got, err := s.List(Query{View: ViewUpcoming, Sort: sort}, now)
		if err != nil {
			t.Fatalf("List(%s): %v", sort, err)
		}
		eq(t, titles(got), []string{
			"sooner thing", "another sooner thing", "middle thing", "later thing",
		})
	}
}

// TestAssignees covers the sidebar's people list, which is discovered from what
// you have typed rather than from any roster.
func TestAssignees(t *testing.T) {
	s := open(t)
	seed(t, s, blob+"\nadmin | another thing for sam @sam")

	people, err := s.Assignees()
	if err != nil {
		t.Fatalf("Assignees: %v", err)
	}
	if len(people) != 2 {
		t.Fatalf("got %d people, want 2 (sam and jo)", len(people))
	}
	if people[0].Name != "sam" || people[0].Count != 2 {
		t.Errorf("people[0] = %+v, want sam with 2", people[0])
	}

	mine, err := s.List(Query{View: ViewAll, Assignee: "jo"}, now)
	if err != nil {
		t.Fatalf("List by assignee: %v", err)
	}
	eq(t, titles(mine), []string{"update the stale value"})
}

// TestWeekPlan covers the planning board, including the rule that stops a late
// task appearing both in its own column and in the overdue tray.
func TestWeekPlan(t *testing.T) {
	s := open(t)
	// now is Tuesday 25 Aug 2026, so the week runs Mon 24 to Sun 30.
	seed(t, s, `admin | slipped on monday | 24/8/2026
admin | due today | today
admin | due friday | eow
admin | long overdue | 1/8/2026
admin | no date at all
admin | next month | +5w`)

	plan, err := s.Week(WeekStart(now), now, Query{})
	if err != nil {
		t.Fatalf("Week: %v", err)
	}
	if got := plan.Start.Format(dateLayout); got != "2026-08-24" {
		t.Errorf("week starts %s, want the Monday 2026-08-24", got)
	}
	if len(plan.Days) != 7 {
		t.Fatalf("got %d days, want 7", len(plan.Days))
	}

	// Monday has already been and gone, so its unfinished task is in the tray.
	eq(t, titles(plan.Days[0].Tasks), nil)
	eq(t, titles(plan.Days[1].Tasks), []string{"due today"})
	eq(t, titles(plan.Days[4].Tasks), []string{"due friday"})

	// Everything late, whatever week it slipped from, ready to be re-dragged.
	eq(t, titles(plan.Overdue), []string{"long overdue", "slipped on monday"})
	eq(t, titles(plan.Unscheduled), []string{"no date at all"})

	// And nothing outside the window leaked into a column.
	for _, d := range plan.Days {
		for _, task := range d.Tasks {
			if task.Title == "next month" {
				t.Error("a task beyond the week appeared in a column")
			}
		}
	}
}

func TestSchedule(t *testing.T) {
	s := open(t)
	seed(t, s, "admin | move me around")
	l := ids(s)

	got, err := s.Schedule(l[0].ID, "2026-09-03")
	if err != nil {
		t.Fatalf("Schedule: %v", err)
	}
	if got.Due == nil || got.Due.Format(dateLayout) != "2026-09-03" {
		t.Errorf("due = %v, want 2026-09-03", got.Due)
	}

	// Dropping it back in the unscheduled tray clears the date.
	if got, err = s.Schedule(l[0].ID, ""); err != nil {
		t.Fatalf("Schedule to unscheduled: %v", err)
	}
	if got.Due != nil {
		t.Errorf("due = %v, want it cleared", got.Due)
	}
	if _, err := s.Schedule(l[0].ID, "not-a-date"); err == nil {
		t.Error("a junk date was accepted")
	}
}

func TestWeekStart(t *testing.T) {
	for _, tc := range []struct{ day, want string }{
		{"2026-08-24", "2026-08-24"}, // Monday itself
		{"2026-08-25", "2026-08-24"},
		{"2026-08-30", "2026-08-24"}, // Sunday belongs to the week just gone
		{"2026-08-31", "2026-08-31"},
	} {
		d, _ := time.ParseInLocation(dateLayout, tc.day, time.UTC)
		if got := WeekStart(d).Format(dateLayout); got != tc.want {
			t.Errorf("WeekStart(%s) = %s, want %s", tc.day, got, tc.want)
		}
	}
}

// TestWeekOfAPastWeekShowsItsUnfinishedWork is the point of the overdue tray:
// browse back and everything still open is there asking for a new date.
func TestWeekOfAPastWeekShowsItsUnfinishedWork(t *testing.T) {
	s := open(t)
	seed(t, s, "admin | never got done | 12/8/2026\nadmin | also not done | 13/8/2026")

	past := WeekStart(time.Date(2026, time.August, 12, 0, 0, 0, 0, time.UTC))
	plan, err := s.Week(past, now, Query{})
	if err != nil {
		t.Fatalf("Week: %v", err)
	}
	for _, d := range plan.Days {
		if len(d.Tasks) != 0 {
			t.Errorf("%s still holds %v; late work belongs in the tray",
				d.Date.Format(dateLayout), titles(d.Tasks))
		}
	}
	eq(t, titles(plan.Overdue), []string{"never got done", "also not done"})
}

func TestWeekFilters(t *testing.T) {
	s := open(t)
	seed(t, s, `admin | admin thing | eow
prod issue | prod thing | eow @sam
personal | personal thing
admin | tagged thing | eow #board`)

	byTopic, err := s.Week(WeekStart(now), now, Query{Topic: "admin"})
	if err != nil {
		t.Fatalf("Week filtered: %v", err)
	}
	eq(t, titles(byTopic.Days[4].Tasks), []string{"admin thing", "tagged thing"})
	eq(t, titles(byTopic.Unscheduled), nil)

	byPerson, _ := s.Week(WeekStart(now), now, Query{Assignee: "sam"})
	eq(t, titles(byPerson.Days[4].Tasks), []string{"prod thing"})

	byTag, _ := s.Week(WeekStart(now), now, Query{Tag: "board"})
	eq(t, titles(byTag.Days[4].Tasks), []string{"tagged thing"})
}

// TestWeekFiltersMatchListFilters: the planner used to build its own WHERE
// clause and had fallen behind the list's by three filters.
func TestWeekFiltersMatchListFilters(t *testing.T) {
	s := open(t)
	batch := seed(t, s, "admin | from this call | eow")
	seed(t, s, "admin | from another call | eow")

	plan, err := s.Week(WeekStart(now), now, Query{Batch: batch})
	if err != nil {
		t.Fatalf("Week: %v", err)
	}
	var got []string
	for _, d := range plan.Days {
		got = append(got, titles(d.Tasks)...)
	}
	eq(t, got, []string{"from this call"})

	// And the captured-date window, which it also ignored.
	none, err := s.Week(WeekStart(now), now, Query{From: "2027-01-01"})
	if err != nil {
		t.Fatalf("Week: %v", err)
	}
	for _, d := range none.Days {
		if len(d.Tasks) != 0 {
			t.Errorf("a task captured in the past survived a future window: %v", titles(d.Tasks))
		}
	}
}

// Filing twice for the same call is easy to do -- someone says one more thing
// after you thought you were finished -- and leaves the record in two pieces.
func TestMergePutsACallBackTogether(t *testing.T) {
	st := open(t)
	now := time.Date(2026, time.August, 25, 9, 0, 0, 0, time.UTC)

	first, err := st.CreateBatch(parse.Parse("prod issue | chase the vendor | today", now).Tasks,
		Capture{Source: "test", Title: "Platform sync"}, now)
	if err != nil {
		t.Fatal(err)
	}
	second, err := st.CreateBatch(parse.Parse("prod issue | and the postmortem | eow", now).Tasks,
		Capture{Source: "test"}, now)
	if err != nil {
		t.Fatal(err)
	}

	moved, err := st.Merge(first, []int64{second})
	if err != nil {
		t.Fatalf("Merge: %v", err)
	}
	if moved != 1 {
		t.Errorf("moved %d tasks, want 1", moved)
	}

	sessions, err := st.Sessions(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 1 {
		t.Fatalf("%d captures left, want 1", len(sessions))
	}
	if sessions[0].ID != first || sessions[0].Total != 2 {
		t.Errorf("kept %d with %d tasks", sessions[0].ID, sessions[0].Total)
	}
	if sessions[0].Title != "Platform sync" {
		t.Errorf("the name became %q", sessions[0].Title)
	}

	// Undo still reaches everything, because it is all one capture now.
	n, err := st.UndoBatch(first)
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Errorf("undo took back %d, want 2", n)
	}
}

// A capture with no name takes one from what it swallowed, rather than staying
// "the 9am on Tuesday" when one of the pieces was named.
func TestMergeTakesAName(t *testing.T) {
	st := open(t)
	now := time.Date(2026, time.August, 25, 9, 0, 0, 0, time.UTC)

	blank, _ := st.CreateBatch(parse.Parse("a | one", now).Tasks, Capture{Source: "test"}, now)
	named, _ := st.CreateBatch(parse.Parse("a | two", now).Tasks,
		Capture{Source: "test", Title: "Vendor call"}, now)

	if _, err := st.Merge(blank, []int64{named}); err != nil {
		t.Fatal(err)
	}
	got, err := st.Session(blank)
	if err != nil {
		t.Fatal(err)
	}
	if got.Title != "Vendor call" {
		t.Errorf("the merged capture is called %q", got.Title)
	}
}

// Merging into something that is not there would orphan every task into a
// batch id that does not exist.
func TestMergeRefusesAMissingCapture(t *testing.T) {
	st := open(t)
	now := time.Date(2026, time.August, 25, 9, 0, 0, 0, time.UTC)
	only, _ := st.CreateBatch(parse.Parse("a | one", now).Tasks, Capture{Source: "test"}, now)

	if _, err := st.Merge(9999, []int64{only}); err == nil {
		t.Error("merged into a capture that does not exist")
	}
	if _, err := st.Merge(only, []int64{only}); err != nil {
		t.Errorf("merging something into itself should be a no-op, got %v", err)
	}
	if sessions, _ := st.Sessions(10); len(sessions) != 1 {
		t.Errorf("%d captures left", len(sessions))
	}
}
