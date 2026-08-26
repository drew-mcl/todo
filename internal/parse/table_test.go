package parse

import (
	"strings"
	"testing"
)

// The shape Teams Copilot produces when it summarises a call.
const copilotTable = `| Action | Owner | Deadline | Notes |
| --- | --- | --- | --- |
| Chase the vendor about the patch build | Sam Okafor | Friday | They have missed two dates |
| Pull the headcount numbers | Jo | 1 Sep | For the board pack |
| Write the postmortem | Priya (Platform) | next week | |`

func TestCopilotTable(t *testing.T) {
	tbl, err := ParseTable(copilotTable)
	if err != nil {
		t.Fatalf("ParseTable: %v", err)
	}
	if tbl.Format != "markdown" {
		t.Errorf("format = %q, want markdown", tbl.Format)
	}
	if tbl.Preset != "copilot" {
		t.Errorf("preset = %q, want it recognised as copilot", tbl.Preset)
	}
	want := []Role{RoleTitle, RoleOwner, RoleDue, RoleNote}
	for i, w := range want {
		if tbl.Columns[i].Role != w {
			t.Errorf("column %d (%q) mapped to %q, want %q",
				i, tbl.Columns[i].Header, tbl.Columns[i].Role, w)
		}
	}

	rows := tbl.Tasks(now, "platform sync")
	if len(rows) != 3 {
		t.Fatalf("got %d rows, want 3", len(rows))
	}
	first := rows[0].Task
	if first == nil {
		t.Fatal("the first row produced no task")
	}
	if first.Title != "Chase the vendor about the patch build" {
		t.Errorf("title = %q", first.Title)
	}
	if first.Assignee != "sam" {
		t.Errorf("assignee = %q, want the full name reduced to a handle", first.Assignee)
	}
	if first.Topic != "platform sync" {
		t.Errorf("topic = %q, want the call title when no column carries one", first.Topic)
	}
	if first.Due == nil || first.Due.Format("2006-01-02") != "2026-08-28" {
		t.Errorf("due = %v, want friday read as 2026-08-28", first.Due)
	}
	if first.Note != "They have missed two dates" {
		t.Errorf("note = %q", first.Note)
	}
	// "Priya (Platform)" must not keep the parenthetical.
	if rows[2].Task.Assignee != "priya" {
		t.Errorf("assignee = %q, want priya", rows[2].Task.Assignee)
	}
}

func TestTsvFromASpreadsheet(t *testing.T) {
	in := "Issue\tPlan\tOwner\tDue date\tPriority\n" +
		"Ingest lag\tDecide on the fix properly\tsam\t+3d\tHigh\n" +
		"Stale dashboard\tUpdate the overview tab\tjo\teow\tLow\n"

	tbl, err := ParseTable(in)
	if err != nil {
		t.Fatalf("ParseTable: %v", err)
	}
	if tbl.Format != "tsv" {
		t.Errorf("format = %q, want tsv", tbl.Format)
	}
	// "Issue" and "Plan" both read as task-ish; the first wins and the mapping
	// is the user's to correct.
	if tbl.Columns[2].Role != RoleOwner || tbl.Columns[3].Role != RoleDue {
		t.Errorf("owner/due columns mis-mapped: %+v", tbl.Columns)
	}
	if tbl.Columns[4].Role != RolePriority {
		t.Errorf("priority column mapped to %q", tbl.Columns[4].Role)
	}

	// Remap so the plan column carries the task and the issue becomes the topic.
	tbl.Apply([]Role{RoleTopic, RoleTitle, RoleOwner, RoleDue, RolePriority})
	rows := tbl.Tasks(now, "")
	if len(rows) != 2 || rows[0].Task == nil {
		t.Fatalf("rows = %+v", rows)
	}
	if rows[0].Task.Title != "Decide on the fix properly" || rows[0].Task.Topic != "ingest lag" {
		t.Errorf("remapping ignored: %+v", rows[0].Task)
	}
	if rows[0].Task.Priority != PriUrgent {
		t.Errorf("priority = %v, want High read as urgent", rows[0].Task.Priority)
	}
}

func TestTableSkipsFinishedAndEmptyRows(t *testing.T) {
	in := `Task,Owner,Status
Ship the thing,sam,Done
Still to do,jo,In progress
,priya,Open`

	tbl, err := ParseTable(in)
	if err != nil {
		t.Fatalf("ParseTable: %v", err)
	}
	if tbl.Format != "csv" {
		t.Errorf("format = %q, want csv", tbl.Format)
	}
	rows := tbl.Tasks(now, "")
	if len(rows) != 3 {
		t.Fatalf("got %d rows, want 3", len(rows))
	}
	if rows[0].Task != nil || !strings.Contains(rows[0].Reason, "done") {
		t.Errorf("a finished row was imported: %+v", rows[0])
	}
	if rows[1].Task == nil {
		t.Error("the open row should have been imported")
	}
	if rows[2].Task != nil || rows[2].Reason == "" {
		t.Errorf("a row with no task text was imported: %+v", rows[2])
	}
}

func TestTableKeepsAnUnreadableDate(t *testing.T) {
	tbl, _ := ParseTable("Action,Deadline\nDo the thing,end of sprint 4")
	rows := tbl.Tasks(now, "")
	task := rows[0].Task
	if task == nil {
		t.Fatal("row not imported")
	}
	if task.Due != nil {
		t.Errorf("due = %v, want none", task.Due)
	}
	if task.Warning == "" {
		t.Error("an unreadable date should say so")
	}
	if !strings.Contains(task.Note, "end of sprint 4") {
		t.Errorf("note = %q; the date text must not be dropped", task.Note)
	}
}

func TestTableRejectsNonTables(t *testing.T) {
	for _, in := range []string{"", "just one line of prose"} {
		if _, err := ParseTable(in); err == nil {
			t.Errorf("ParseTable(%q) succeeded, want an error", in)
		}
	}
}

func TestGuessRole(t *testing.T) {
	for _, tc := range []struct {
		header string
		want   Role
	}{
		{"Action Item", RoleTitle},
		{"action", RoleTitle},
		{"Task", RoleTitle},
		{"Owner", RoleOwner},
		{"Assigned To", RoleOwner},
		{"DRI", RoleOwner},
		{"Deadline", RoleDue},
		{"Due Date", RoleDue},
		{"Target date", RoleDue},
		{"Workstream", RoleTopic},
		{"Severity", RolePriority},
		{"Notes", RoleNote},
		{"Status", RoleStatus},
		{"Sprint velocity", RoleIgnore},
	} {
		if got := guessRole(tc.header); got != tc.want {
			t.Errorf("guessRole(%q) = %q, want %q", tc.header, got, tc.want)
		}
	}
}
