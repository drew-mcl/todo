package parse

import (
	"encoding/csv"
	"fmt"
	"strings"
	"time"
)

// Tables are the other way work arrives: someone pastes a tracker, or Teams
// Copilot summarises a call into Action / Owner / Deadline / Notes. Rather than
// a separate importer per source, one engine reads the grid and a mapping says
// what each column means. Everything downstream is the same Task the shorthand
// produces, so the two inputs cannot drift apart.

// Role is what a column means.
type Role string

const (
	RoleIgnore   Role = "ignore"
	RoleTitle    Role = "title"
	RoleTopic    Role = "topic"
	RoleOwner    Role = "owner"
	RoleDue      Role = "due"
	RoleNote     Role = "note"
	RolePriority Role = "priority"
	RoleTags     Role = "tags"
	RoleStatus   Role = "status"
)

// Roles is the order the mapping control offers.
var Roles = []Role{
	RoleTitle, RoleTopic, RoleOwner, RoleDue, RolePriority, RoleTags, RoleNote,
	RoleStatus, RoleIgnore,
}

// headerHints maps a lowercased column header to the role it probably plays.
// Longer, more specific phrases are tried first.
var headerHints = []struct {
	match []string
	role  Role
}{
	{[]string{"action item", "action items", "actionitem"}, RoleTitle},
	{[]string{"due date", "duedate", "target date", "deadline", "due by", "due"}, RoleDue},
	{[]string{"action", "task", "item", "work", "activity", "issue", "plan",
		"description", "summary", "what", "deliverable", "next step", "next steps"}, RoleTitle},
	{[]string{"owner", "assignee", "assigned to", "assigned", "who", "responsible",
		"accountable", "dri", "lead", "person"}, RoleOwner},
	{[]string{"when", "by", "date", "timeline", "eta", "target"}, RoleDue},
	{[]string{"topic", "area", "category", "project", "workstream", "theme",
		"system", "component", "team", "epic"}, RoleTopic},
	{[]string{"priority", "severity", "urgency", "importance", "sev", "prio"}, RolePriority},
	{[]string{"tag", "tags", "label", "labels"}, RoleTags},
	{[]string{"note", "notes", "comment", "comments", "detail", "details",
		"context", "background"}, RoleNote},
	{[]string{"status", "state", "progress"}, RoleStatus},
}

// guessRole reads a header. An unrecognised column is ignored rather than
// guessed at, so a stray column never becomes a task title.
func guessRole(header string) Role {
	h := strings.ToLower(strings.TrimSpace(header))
	h = strings.Trim(h, "*_#:")
	for _, hint := range headerHints {
		for _, m := range hint.match {
			if h == m {
				return hint.role
			}
		}
	}
	for _, hint := range headerHints {
		for _, m := range hint.match {
			if strings.Contains(h, m) {
				return hint.role
			}
		}
	}
	return RoleIgnore
}

// Column is one column of a pasted grid.
type Column struct {
	Index  int    `json:"index"`
	Header string `json:"header"`
	Role   Role   `json:"role"`
}

// Table is a parsed grid, before any of it becomes a task.
type Table struct {
	Columns []Column   `json:"columns"`
	Rows    [][]string `json:"rows"`
	Format  string     `json:"format"` // markdown, tsv or csv
	Preset  string     `json:"preset"` // copilot, when the shape is recognised
}

// ParseTable reads a pasted grid. It accepts what people actually paste: a
// markdown table, a spreadsheet's tab-separated text, or CSV.
func ParseTable(input string) (*Table, error) {
	lines := splitLines(input)
	if len(lines) == 0 {
		return nil, fmt.Errorf("there is nothing here to read")
	}

	var grid [][]string
	var format string
	switch {
	case looksMarkdown(lines):
		grid, format = readMarkdown(lines), "markdown"
	case strings.Contains(input, "\t"):
		grid, format = readDelimited(lines, '\t'), "tsv"
	default:
		rows, err := csv.NewReader(strings.NewReader(input)).ReadAll()
		if err != nil {
			return nil, fmt.Errorf("this does not look like a table: %w", err)
		}
		grid, format = rows, "csv"
	}

	grid = dropEmptyRows(grid)
	if len(grid) < 2 {
		return nil, fmt.Errorf("a table needs a header row and at least one row under it")
	}

	t := &Table{Rows: grid[1:], Format: format}
	for i, h := range grid[0] {
		t.Columns = append(t.Columns, Column{Index: i, Header: strings.TrimSpace(h), Role: guessRole(h)})
	}
	// Nothing read as the task itself: fall back to the first unclaimed column,
	// which is almost always what a tracker leads with.
	if !t.has(RoleTitle) {
		for i := range t.Columns {
			if t.Columns[i].Role == RoleIgnore {
				t.Columns[i].Role = RoleTitle
				break
			}
		}
	}
	t.Preset = detectPreset(t)
	return t, nil
}

func (t *Table) has(r Role) bool {
	for _, c := range t.Columns {
		if c.Role == r {
			return true
		}
	}
	return false
}

// detectPreset names a recognised shape. Teams Copilot summarises a call as
// action / owner / deadline, which is worth calling out by name so it is
// obvious the paste was understood.
func detectPreset(t *Table) string {
	var roles []Role
	for _, c := range t.Columns {
		roles = append(roles, c.Role)
	}
	seen := func(r Role) bool {
		for _, x := range roles {
			if x == r {
				return true
			}
		}
		return false
	}
	if len(t.Columns) >= 3 && seen(RoleTitle) && seen(RoleOwner) && seen(RoleDue) {
		for _, c := range t.Columns {
			h := strings.ToLower(c.Header)
			if strings.Contains(h, "action") || strings.Contains(h, "deadline") {
				return "copilot"
			}
		}
	}
	return ""
}

// Apply overrides the guessed mapping with one chosen in the interface.
func (t *Table) Apply(mapping []Role) {
	for i := range t.Columns {
		if i < len(mapping) && mapping[i] != "" {
			t.Columns[i].Role = mapping[i]
		}
	}
	t.Preset = detectPreset(t)
}

// TableRow is one row's outcome, kept alongside the task so the preview can
// explain a row it declined to import.
type TableRow struct {
	N      int      `json:"n"`
	Cells  []string `json:"cells"`
	Task   *Task    `json:"-"`
	Reason string   `json:"reason,omitempty"`
}

// Tasks turns the grid into tasks. defaultTopic fills in when no column carries
// one -- typically the title of the call the table came from.
func (t *Table) Tasks(now time.Time, defaultTopic string) []TableRow {
	col := map[Role]int{}
	for _, c := range t.Columns {
		if c.Role != RoleIgnore && c.Role != "" {
			if _, ok := col[c.Role]; !ok || c.Role == RoleTags {
				col[c.Role] = c.Index
			}
		}
	}

	cell := func(row []string, r Role) string {
		i, ok := col[r]
		if !ok || i >= len(row) {
			return ""
		}
		return strings.TrimSpace(row[i])
	}

	out := make([]TableRow, 0, len(t.Rows))
	for n, row := range t.Rows {
		entry := TableRow{N: n + 1, Cells: row}

		title := cleanCell(cell(row, RoleTitle))
		if title == "" {
			entry.Reason = "no task text in the mapped column"
			out = append(out, entry)
			continue
		}
		if s := strings.ToLower(cell(row, RoleStatus)); isDoneStatus(s) {
			entry.Reason = "already marked " + s
			out = append(out, entry)
			continue
		}

		task := &Task{
			Title: title,
			Topic: DefaultTopic,
			Raw:   strings.Join(row, " | "),
		}
		if topic := strings.ToLower(cleanCell(cell(row, RoleTopic))); topic != "" {
			task.Topic = topic
		} else if defaultTopic != "" {
			task.Topic = strings.ToLower(defaultTopic)
		}
		if owner := cleanOwner(cell(row, RoleOwner)); owner != "" {
			task.Assignee = owner
		}
		task.Note = cleanCell(cell(row, RoleNote))
		task.Priority = parsePriorityWord(cell(row, RolePriority))

		if due := cleanCell(cell(row, RoleDue)); due != "" {
			if d := ParseDue(due, now); d.Recognised && d.Explicit {
				date := d.Date
				task.Due = &date
			} else if !d.Recognised {
				// Keep what it said rather than dropping it on the floor.
				task.Warning = fmt.Sprintf("could not read %q as a date", due)
				task.Note = appendLine(task.Note, "due: "+due)
			}
		}
		for _, tag := range splitTags(cell(row, RoleTags)) {
			task.Tags = append(task.Tags, tag)
		}

		entry.Task = task
		out = append(out, entry)
	}
	return out
}

// ── helpers ─────────────────────────────────────────────────────────────────

func splitLines(s string) []string {
	var out []string
	for _, l := range strings.Split(strings.ReplaceAll(s, "\r\n", "\n"), "\n") {
		if strings.TrimSpace(l) != "" {
			out = append(out, l)
		}
	}
	return out
}

func looksMarkdown(lines []string) bool {
	if len(lines) < 2 {
		return false
	}
	for _, l := range lines[:min(4, len(lines))] {
		t := strings.TrimSpace(l)
		if strings.Count(t, "|") >= 2 && strings.Trim(t, "|-: \t") == "" {
			return true
		}
	}
	return strings.Count(lines[0], "|") >= 2
}

func readMarkdown(lines []string) [][]string {
	var out [][]string
	for _, l := range lines {
		t := strings.TrimSpace(l)
		if strings.Trim(t, "|-: \t") == "" {
			continue // the ---|--- rule under the header
		}
		t = strings.TrimPrefix(t, "|")
		t = strings.TrimSuffix(t, "|")
		cells := strings.Split(t, "|")
		for i := range cells {
			cells[i] = strings.TrimSpace(cells[i])
		}
		out = append(out, cells)
	}
	return out
}

func readDelimited(lines []string, sep rune) [][]string {
	out := make([][]string, 0, len(lines))
	for _, l := range lines {
		cells := strings.Split(l, string(sep))
		for i := range cells {
			cells[i] = strings.TrimSpace(cells[i])
		}
		out = append(out, cells)
	}
	return out
}

func dropEmptyRows(grid [][]string) [][]string {
	out := grid[:0]
	for _, row := range grid {
		for _, c := range row {
			if strings.TrimSpace(c) != "" {
				out = append(out, row)
				break
			}
		}
	}
	return out
}

// cleanCell strips the markdown and bullet decoration that survives a paste.
func cleanCell(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "- ")
	s = strings.TrimPrefix(s, "* ")
	s = strings.TrimPrefix(s, "• ")
	s = strings.ReplaceAll(s, "**", "")
	s = strings.ReplaceAll(s, "<br>", " ")
	return strings.TrimSpace(s)
}

// cleanOwner reduces "Sam Okafor (Platform)" or "@sam" to a handle.
func cleanOwner(s string) string {
	s = cleanCell(s)
	if i := strings.IndexAny(s, "(<"); i > 0 {
		s = s[:i]
	}
	if i := strings.IndexAny(s, ",;/&"); i > 0 {
		s = s[:i] // only the first owner; the rest stay in the text
	}
	s = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(s), "@"))
	if f := strings.Fields(s); len(f) > 0 {
		s = f[0]
	}
	if strings.EqualFold(s, "unassigned") || strings.EqualFold(s, "tbd") || strings.EqualFold(s, "n/a") {
		return ""
	}
	return strings.ToLower(s)
}

func parsePriorityWord(s string) Priority {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "critical", "blocker", "p0", "sev0", "sev1", "!!!", "highest":
		return PriCritical
	case "urgent", "high", "p1", "sev2", "!!":
		return PriUrgent
	case "medium", "med", "p2", "!", "moderate":
		return PriHigh
	default:
		return PriNormal
	}
}

func isDoneStatus(s string) bool {
	switch strings.TrimSpace(s) {
	case "done", "complete", "completed", "closed", "resolved", "shipped", "✅", "x":
		return true
	}
	return false
}

func splitTags(s string) []string {
	var out []string
	seen := map[string]bool{}
	for _, t := range strings.FieldsFunc(s, func(r rune) bool {
		return r == ',' || r == ';' || r == ' ' || r == '#' || r == '/'
	}) {
		t = strings.ToLower(strings.TrimSpace(t))
		if t != "" && !seen[t] {
			seen[t] = true
			out = append(out, t)
		}
	}
	return out
}

func appendLine(base, add string) string {
	if base == "" {
		return add
	}
	return base + "\n" + add
}
