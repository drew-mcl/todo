// Command todo captures meeting-note shorthand into a local task list.
//
//	todo serve                 start the web app on 127.0.0.1
//	todo add "topic | task"    capture from the terminal
//	cat notes.txt | todo add   capture a whole file
package main

import (
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/drew-mcl/todo/internal/api"
	"github.com/drew-mcl/todo/internal/parse"
	"github.com/drew-mcl/todo/internal/store"
	"github.com/drew-mcl/todo/internal/ui"
)

const usage = `todo -- capture meeting notes as tasks

Usage:
  todo serve [--port 8765] [--open]      start the web app
  todo add   [text...]                   capture from the terminal or stdin

Shorthand:
  topic | task text [| due] [@who] [!priority] [#tags] [> note]

Flags:
  --db PATH    database file (default %s, or $TODO_DB)
`

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "todo:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		fmt.Printf(usage, store.DefaultPath())
		return nil
	}
	switch args[0] {
	case "serve":
		return serve(args[1:])
	case "add":
		return add(args[1:])
	case "-h", "--help", "help":
		fmt.Printf(usage, store.DefaultPath())
		return nil
	default:
		return fmt.Errorf("unknown command %q; try 'todo help'", args[0])
	}
}

func serve(args []string) error {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	port := fs.Int("port", 8765, "port to listen on")
	dbPath := fs.String("db", store.DefaultPath(), "database file")
	openBrowser := fs.Bool("open", false, "open the app in a browser")
	if err := fs.Parse(args); err != nil {
		return err
	}

	st, err := store.Open(*dbPath)
	if err != nil {
		return err
	}
	defer st.Close()

	srv := api.New(st, time.Now, ui.Handler())

	// Loopback only. This is a personal list with no authentication in front of
	// it, so it must never be reachable from the network.
	addr := fmt.Sprintf("127.0.0.1:%d", *port)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("listening on %s: %w", addr, err)
	}

	url := "http://" + addr
	fmt.Printf("todo is at %s\n%s\n", url, *dbPath)
	if *openBrowser {
		launch(url)
	}
	return http.Serve(ln, srv)
}

// launch opens the default browser, and stays quiet if it cannot.
func launch(url string) {
	var cmd string
	switch runtime.GOOS {
	case "darwin":
		cmd = "open"
	case "windows":
		cmd = "explorer"
	default:
		cmd = "xdg-open"
	}
	_ = exec.Command(cmd, url).Start()
}

func add(args []string) error {
	fs := flag.NewFlagSet("add", flag.ExitOnError)
	dbPath := fs.String("db", store.DefaultPath(), "database file")
	if err := fs.Parse(args); err != nil {
		return err
	}

	text := strings.Join(fs.Args(), " ")
	if text == "" {
		piped, err := io.ReadAll(os.Stdin)
		if err != nil {
			return fmt.Errorf("reading stdin: %w", err)
		}
		text = string(piped)
	}
	if strings.TrimSpace(text) == "" {
		return fmt.Errorf("nothing to add; pass a line or pipe one in")
	}

	now := time.Now()
	res := parse.Parse(text, now)
	if len(res.Tasks) == 0 {
		return fmt.Errorf("no line contained a '|', so nothing was read as a task")
	}

	st, err := store.Open(*dbPath)
	if err != nil {
		return err
	}
	defer st.Close()

	if _, err := st.CreateBatch(res.Tasks, "cli", now); err != nil {
		return err
	}

	for _, t := range res.Tasks {
		fmt.Printf("%-14s %s%s%s%s\n",
			t.Topic, t.Title,
			field(t.Due != nil, "  "+dueText(t, now)),
			field(t.Assignee != "", "  @"+t.Assignee),
			field(t.Priority > 0, "  "+t.Priority.Marks()))
	}
	if _, _, skipped := res.Counts(); skipped > 0 {
		fmt.Printf("(%d line(s) skipped)\n", skipped)
	}
	return nil
}

func dueText(t *parse.Task, now time.Time) string {
	if t.Due == nil {
		return ""
	}
	return parse.FormatDue(*t.Due, now)
}

func field(ok bool, s string) string {
	if ok {
		return s
	}
	return ""
}
