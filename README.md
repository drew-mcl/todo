# todo

Paste your call notes. The action lines become tasks. Everything else is left alone.

```
make build && ./todo serve --open
```

## shorthand

```
topic | task text [| due] [@who] [!priority] [#tags] [> note]
"     | repeats the topic above
> continuation note line
```

```
prod issue | chase the vendor about the patch | today @sam !!
> they have missed two dates now
"          | write the postmortem | eow #board
admin | quarterly training | eow #compliance
some tool | overview tab shows the stale value @jo
```

Lines without a `|` are skipped, so a whole page of meeting notes can go in at
once. `@who`, `!` and `#tag` work in any position. `^`, `,,`, `~` and a bare
leading `|` all repeat the topic too.

Dates: `today` `tomorrow` `eow` `eom` `next week` `fri` `next fri` `+3d` `+2w`
`25/12` `25 dec` `2026-12-25` `someday`

Priority: `!` high, `!!` urgent, `!!!` critical.

## week planning

Press `w`. Seven day columns; drag a task onto a day to schedule it, or into a
tray to unschedule it. Anything late sits in **overdue** until you give it a new
day, so browsing back to a past week shows exactly what still needs rescuing.
Filter the whole board by topic, person or tag.

## capture from the terminal

```
todo add "prod issue | check the logs | today @sam"
cat notes.txt | todo add
```

## keys

`n` capture · `a` everything · `w` week · `⌘K` palette · `/` search · `?` all of them
`j`/`k` move · `x` complete · `e` edit · `dd` delete · `u` undo · `[` hide the sidebar
`g` then `t` `u` `a` `d` `l` to jump

## shape

```
main.go              serve, add
internal/parse       the shorthand grammar, dates, highlighting
internal/store       SQLite via modernc — no CGO
internal/api         JSON over the top of both
internal/ui          the built client, embedded
ui/                  React + Vite + Tailwind
```

There is one parser and it is in Go. The capture preview, the list and the CLI
all go through it, so they cannot disagree about what a line means — the client
renders what it is told and derives nothing.

Database at `~/.local/share/todo/todo.db`, or set `TODO_DB`. Serves on 127.0.0.1
only — there is no auth, so keep it off the network.
