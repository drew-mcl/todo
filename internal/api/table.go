package api

import (
	"net/http"

	"github.com/drew-mcl/todo/internal/parse"
	"github.com/drew-mcl/todo/internal/store"
)

// A pasted grid goes through the same two-step as the shorthand: read it, show
// exactly what it became, then commit. The mapping is the only extra, and it is
// guessed from the headers so most pastes need no adjustment.

type tableBody struct {
	Input   string       `json:"input"`
	Mapping []parse.Role `json:"mapping"`
	Topic   string       `json:"topic"`
	Title   string       `json:"title"`
	Source  string       `json:"source"`
}

// TableColumn is one column and what it was read as.
type TableColumn struct {
	Index  int    `json:"index"`
	Header string `json:"header"`
	Role   string `json:"role"`
}

// TableRow is one row's outcome.
type TableRow struct {
	N      int          `json:"n"`
	Cells  []string     `json:"cells"`
	Task   *PreviewTask `json:"task,omitempty"`
	Reason string       `json:"reason,omitempty"`
}

// TablePreview is the read of a pasted grid.
type TablePreview struct {
	Columns []TableColumn `json:"columns"`
	Rows    []TableRow    `json:"rows"`
	Format  string        `json:"format"`
	Preset  string        `json:"preset"`
	Roles   []string      `json:"roles"`
	Tasks   int           `json:"tasks"`
	Skipped int           `json:"skipped"`
}

func (s *Server) readTable(body tableBody) (*parse.Table, []parse.TableRow, error) {
	t, err := parse.ParseTable(body.Input)
	if err != nil {
		return nil, nil, err
	}
	if len(body.Mapping) > 0 {
		t.Apply(body.Mapping)
	}
	return t, t.Tasks(s.now(), body.Topic), nil
}

func (s *Server) handleTablePreview(w http.ResponseWriter, r *http.Request) {
	var body tableBody
	if err := decode(r, &body); err != nil {
		s.errorf(w, http.StatusBadRequest, "Could not read what you pasted.")
		return
	}
	table, rows, err := s.readTable(body)
	if err != nil {
		s.errorf(w, http.StatusUnprocessableEntity, "%s", err.Error())
		return
	}

	out := TablePreview{Format: table.Format, Preset: table.Preset}
	for _, r := range parse.Roles {
		out.Roles = append(out.Roles, string(r))
	}
	for _, c := range table.Columns {
		out.Columns = append(out.Columns, TableColumn{Index: c.Index, Header: c.Header, Role: string(c.Role)})
	}
	now := s.now()
	for _, row := range rows {
		dto := TableRow{N: row.N, Cells: row.Cells, Reason: row.Reason}
		if row.Task != nil {
			dto.Task = previewTask(row.Task, now)
			out.Tasks++
		} else {
			out.Skipped++
		}
		out.Rows = append(out.Rows, dto)
	}
	s.json(w, out)
}

func (s *Server) handleTableCapture(w http.ResponseWriter, r *http.Request) {
	var body tableBody
	if err := decode(r, &body); err != nil {
		s.errorf(w, http.StatusBadRequest, "Could not read what you pasted.")
		return
	}
	_, rows, err := s.readTable(body)
	if err != nil {
		s.errorf(w, http.StatusUnprocessableEntity, "%s", err.Error())
		return
	}

	var tasks []*parse.Task
	for _, row := range rows {
		if row.Task != nil {
			tasks = append(tasks, row.Task)
		}
	}
	if len(tasks) == 0 {
		s.errorf(w, http.StatusUnprocessableEntity,
			"No row had anything in the column mapped to the task")
		return
	}

	source := body.Source
	if source == "" {
		source = "table"
	}
	batch, err := s.store.CreateBatch(tasks, store.Capture{Source: source, Title: body.Title}, s.now())
	if err != nil {
		s.fail(w, err, "saving the tasks")
		return
	}
	s.json(w, map[string]any{"batchId": batch, "added": len(tasks)})
}
