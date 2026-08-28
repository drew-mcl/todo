package api

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/drew-mcl/todo/internal/palette"
	"github.com/drew-mcl/todo/internal/parse"
	"github.com/drew-mcl/todo/internal/store"
	"github.com/drew-mcl/todo/internal/vim"
)

// The capture bar is a native window, so it cannot be served a page and it
// should not have to run a web server to be talked to. It gets the same answers
// over a pipe instead: one JSON request per line in, one JSON reply per line
// out, in the same shapes the browser is sent.
//
// The rule is the one the whole project runs on. There is one parser, one set
// of dates and one set of colours, and they live in Go -- so the bar renders
// what it is told and derives nothing, exactly like the client does.

// BridgeRequest is one line of input.
type BridgeRequest struct {
	ID    int    `json:"id"`
	Op    string `json:"op"`
	Draft string `json:"draft,omitempty"`
	Title string `json:"title,omitempty"`
	Batch int64  `json:"batch,omitempty"`
	Task  int64  `json:"task,omitempty"`
}

// BridgeReply is one line of output. Exactly one of the result fields is set.
type BridgeReply struct {
	ID      int              `json:"id"`
	Error   string           `json:"error,omitempty"`
	Hello   *BridgeHello     `json:"hello,omitempty"`
	Preview *PreviewResponse `json:"preview,omitempty"`
	// Hues accompanies a preview: which colour each topic in it gets. The rule
	// is Assign's, not a hash, because no two topics on one screen may share a
	// dot -- and working that out is not the bar's job.
	Hues   map[string]int `json:"hues,omitempty"`
	Added  *BridgeAdded   `json:"added,omitempty"`
	Undone *int           `json:"undone,omitempty"`
	Day    *BridgeDay     `json:"day,omitempty"`
}

// BridgeHello is what the bar needs before it can draw anything: the version it
// is talking to and every colour it is allowed to use.
type BridgeHello struct {
	Version string         `json:"version"`
	Palette BridgePalette  `json:"palette"`
	Counts  map[string]int `json:"counts"`
	// The keys the capture box answers to, so the window's reference sheet and
	// the browser's are the same sheet.
	Keys []vim.Group `json:"keys"`
}

// BridgePalette is internal/palette on the wire. The bar holds no colours of
// its own, so a topic is the same dot in the capture window as it is in the
// terminal and the browser.
type BridgePalette struct {
	Topic  []BridgeColour           `json:"topic"`
	Scheme map[string]*BridgeColour `json:"scheme"`
}

// BridgeColour is one adaptive colour: what to use in each appearance.
type BridgeColour struct {
	Light string `json:"light"`
	Dark  string `json:"dark"`
}

// BridgeAdded is the result of a capture, with enough of the day to say
// something afterwards.
type BridgeAdded struct {
	BatchID int64 `json:"batchId"`
	Added   int   `json:"added"`
	Today   int   `json:"today"`
}

// BridgeDay is today, for the window that shows it: what is due, what has
// slipped, and how much of it is already done.
type BridgeDay struct {
	Label   string         `json:"label"`
	Tasks   []Task         `json:"tasks"`
	Overdue []Task         `json:"overdue"`
	Done    int            `json:"done"`
	Hues    map[string]int `json:"hues"`
}

// bridgeVersion changes when the wire shapes do, so a stale bar can say so
// rather than mis-drawing.
const bridgeVersion = "1"

// Bridge answers requests on in until it is closed. It is the whole of the
// capture bar's contact with this program.
func Bridge(st *store.Store, now func() time.Time, in io.Reader, out io.Writer) error {
	if now == nil {
		now = time.Now
	}
	dec := json.NewDecoder(bufio.NewReaderSize(in, maxBody))
	enc := json.NewEncoder(out)

	for {
		var req BridgeRequest
		if err := dec.Decode(&req); err != nil {
			if err == io.EOF {
				return nil
			}
			return fmt.Errorf("reading a request: %w", err)
		}
		if err := enc.Encode(answer(st, now, req)); err != nil {
			return fmt.Errorf("writing a reply: %w", err)
		}
		if f, ok := out.(interface{ Flush() error }); ok {
			f.Flush()
		}
	}
}

func answer(st *store.Store, now func() time.Time, req BridgeRequest) BridgeReply {
	reply := BridgeReply{ID: req.ID}
	fail := func(format string, args ...any) BridgeReply {
		reply.Error = fmt.Sprintf(format, args...)
		return reply
	}

	switch req.Op {
	case "hello":
		counts, err := st.Counts(now())
		if err != nil {
			return fail("reading the list: %v", err)
		}
		reply.Hello = &BridgeHello{
			Version: bridgeVersion,
			Palette: wirePalette(),
			Counts:  countsDTO(counts),
			Keys:    vim.Reference(),
		}

	case "preview":
		t := now()
		res := parse.Parse(req.Draft, t)
		p := previewDTO(res, t)
		reply.Preview, reply.Hues = &p, previewHues(res)

	case "capture":
		t := now()
		res := parse.Parse(req.Draft, t)
		if len(res.Tasks) == 0 {
			return fail("No line contained a '|', so nothing was read as a task.")
		}
		if err := checkBatchSize(len(res.Tasks)); err != nil {
			return fail("%s", err.Error())
		}
		batch, err := st.CreateBatch(res.Tasks, store.Capture{Source: "bar", Title: req.Title}, t)
		if err != nil {
			return fail("saving the tasks: %v", err)
		}
		counts, err := st.Counts(t)
		if err != nil {
			return fail("reading the list: %v", err)
		}
		reply.Added = &BridgeAdded{
			BatchID: batch,
			Added:   len(res.Tasks),
			Today:   counts[store.ViewToday],
		}

	case "today":
		day, err := today(st, now())
		if err != nil {
			return fail("reading the day: %v", err)
		}
		reply.Day = day

	case "toggle":
		if _, err := st.Toggle(req.Task, now()); err != nil {
			return fail("closing that one: %v", err)
		}
		day, err := today(st, now())
		if err != nil {
			return fail("reading the day: %v", err)
		}
		reply.Day = day

	case "undo":
		n, err := st.UndoBatch(req.Batch)
		if err != nil {
			return fail("undoing that capture: %v", err)
		}
		reply.Undone = &n

	default:
		return fail("there is no %q operation", req.Op)
	}
	return reply
}

// today is the day as the window shows it: what is due, then what has slipped,
// which is the order you would want to be asked about them in.
func today(st *store.Store, now time.Time) (*BridgeDay, error) {
	due, err := st.List(store.Query{View: store.ViewToday, Sort: store.SortManual}, now)
	if err != nil {
		return nil, err
	}
	late, err := st.List(store.Query{View: store.ViewOverdue, Sort: store.SortManual}, now)
	if err != nil {
		return nil, err
	}
	done, err := st.DoneOn(now)
	if err != nil {
		return nil, err
	}

	names := make([]string, 0, len(due)+len(late))
	for _, t := range append(append([]*store.Task{}, due...), late...) {
		names = append(names, t.Topic)
	}
	return &BridgeDay{
		Label:   strings.ToLower(now.Format("Mon 2 January")),
		Tasks:   taskDTOs(due, now),
		Overdue: taskDTOs(late, now),
		Done:    done,
		Hues:    palette.Assign(names),
	}, nil
}

// previewHues gives every topic in a draft a colour of its own, by the same
// rule the list uses.
func previewHues(res *parse.Result) map[string]int {
	names := make([]string, 0, len(res.Tasks))
	for _, t := range res.Tasks {
		names = append(names, t.Topic)
	}
	return palette.Assign(names)
}

// wirePalette is internal/palette, flattened for a client that cannot import it.
func wirePalette() BridgePalette {
	p := BridgePalette{Scheme: map[string]*BridgeColour{
		"ink":    {palette.InkLight, palette.InkDark},
		"ink2":   {palette.Ink2Light, palette.Ink2Dark},
		"ink3":   {palette.Ink3Light, palette.Ink3Dark},
		"ink4":   {palette.Ink4Light, palette.Ink4Dark},
		"line":   {palette.LineLight, palette.LineDark},
		"sunk":   {palette.SunkLight, palette.SunkDark},
		"accent": {palette.AccentLight, palette.AccentDark},
		"danger": {palette.DangerLight, palette.DangerDark},
	}}
	for i := range palette.Hues {
		p.Topic = append(p.Topic, BridgeColour{palette.TopicLight[i], palette.TopicDark[i]})
	}
	return p
}

func countsDTO(in map[store.View]int) map[string]int {
	out := make(map[string]int, len(in))
	for v, n := range in {
		out[string(v)] = n
	}
	return out
}
