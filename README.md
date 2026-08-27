# todo

Paste your call notes. The action lines become tasks. Everything else is left alone.

```
git clone git@github.com:drew-mcl/todo.git && cd todo
make && ./todo
```

`todo` opens the terminal app. `todo serve --open` opens the web one. `make bar`
adds a macOS capture window on a global hotkey. All three read the same database,
so it does not matter which is in front of you.

The built client is committed, so a clone needs **Go and nothing else** — no Node,
no Swift. `make ui` rebuilds the web client if you change it; `make bar` builds
the optional capture window.

## shorthand

```
topic | task text [| due] [@who] [!priority] [#tags] [> note]
      | repeats the topic above
> continuation note line
```

```
prod issue | chase the vendor about the patch | today @sam !!
> they have missed two dates now
           | write the postmortem | eow #board
admin | quarterly training | eow #compliance
some tool | overview tab shows the stale value @jo
```

Lines without a `|` are skipped, so a whole page of meeting notes can go in at
once. `@who`, `!` and `#tag` work in any position. Start a line with `|` and it
repeats the topic above — `^`, `,,` and `~` do the same if you prefer a mark.

Dates: `today` `tomorrow` `eow` `eom` `next week` `fri` `next fri` `+3d` `+2w`
`25/12` `25 dec` `2026-12-25` `someday`

Priority: `!` high, `!!` urgent, `!!!` critical.

## the other ways in

Not everything arrives as shorthand. Capture has three modes:

| mode | for |
|---|---|
| shorthand | typing or pasting your own notes |
| table | a tracker, spreadsheet, markdown table or CSV |
| copilot | the action-item table Teams Copilot writes after a call |

Table and copilot read the header row and guess what each column means — action,
owner, deadline, notes, topic, priority, status — and show the guess as a set of
dropdowns you can correct. Rows already marked done are skipped. Dates go through
the same reader as the shorthand, so `Friday` and `1 Sep` and `+3d` all work.

## calls

Name a capture and it becomes a call you can find again. `calls` lists them
newest first with how much is still open, so a commitment can be traced back to
the conversation it was made in.

**share** renders a call as plain text for a chat message — grouped by who owes
what, so each person can find their own name:

```
Platform sync — Tue 25 Aug
3 actions

Jo
  • Pull the headcount numbers (Tue 1 Sep)

Sam
  • Chase the vendor about the patch build (Fri 28 Aug)

Me
  • Write the postmortem (Fri 28 Aug)
```

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

`today` means today. Anything that slipped lives in `overdue` and in the week
board's overdue tray until you give it a new day — it never squats on top of the
day you actually planned.

## keys

`t` today · `w` week · `a` everything · `l` logbook · `c` calls · `n` capture
`⌘K` palette · `/` search · `f` filter · `b` hide the sidebar · `?` all of them
`j`/`k` move · `x` complete · `e` edit · `dd` delete · `u` undo
`g` then `o` `u` `a` `d` to reach the narrower filters

## ⌥Space, from anywhere

```
make bar-install
```

A capture window on a system-wide hotkey, so a commitment made on a call can be
written down without leaving the call. It floats over whatever is in front of
you — including full-screen apps, and on whatever Space you happen to be on —
and puts you back where you were when it closes.

It is the same box as the one in the terminal: your shorthand on top, what it
becomes underneath, re-read on every keystroke. `⌘↵` files it, `esc` puts it
away, `⌘Z` takes back what you just added.

Clicking away does not close it and does not empty it. Going to fetch the thing
you were about to paste should not cost you what you have already written.

```
mac/Sources   the window, the hotkey, the pipe
mac/Tests     what can be checked without a screen
```

The window holds no grammar, no dates and no colours of its own. It runs
`todo bridge` — a line of JSON in, a line of JSON out — so it renders what it is
told, the same way the web client does. `make test-bar` checks the two agree.

The hotkey is `⌥Space` unless you say otherwise:

```
defaults write com.drew-mcl.todo.capture hotkey "ctrl+opt+t"
```

`make bar` builds it without installing; `make bar-uninstall` takes it back off.
Nothing about it is required — the terminal app and the web one do not know it
is there.

## the terminal app

```
todo            # or todo tui
```

Same lists, same grammar, same colours — a topic is the same dot in the terminal
as it is in the browser, because both sides run the hash in
`internal/palette`. It talks to SQLite directly, so it works whether or not the
server is up.

`n` opens the capture box, and every keystroke re-reads what you have written:
your shorthand on one line, what it becomes on the next. The preview follows the
line you are on and marks it, so a page-long paste shows you the part you are
actually looking at.

```
  2 TASKS · 1 NOTE · 1 SKIPPED
  prod issue | chase the vendor about the patch | today @sam !!
    chase the vendor about the patch  ● prod issue · today · sam · !!
  > they have missed two dates
    attached
             | write the postmortem | eow #board
    write the postmortem  ● prod issue · fri · #board
  finance want the numbers before friday
    no topic separator
```

`e` reopens a task as the line you originally wrote, rather than a form of
separate fields — correct the shorthand and it is re-read.

`w` plans the week. Seven columns do not fit a terminal, so the days stack and
scheduling is a keystroke instead of a drag — which turns out to be faster:

```
  WED 26 AUG · TODAY  3
    ▌ ○ chase the vendor about the patch build                      !!
        ● prod issue · sam

  FRI 28 AUG  1
      ○ write the postmortem
        ● prod issue
```

`1`–`7` puts the selected task on that day · `0` takes it off the calendar
`[` `]` a day either way · `<` `>` another week · `.` back to this one

`n` capture · `⌃s` add · `j`/`k` move · `x` complete · `e` edit · `dd` delete
`u` undo · `/` search · `t` today · `w` week · `a` all · `l` logbook · `?` keys

In the capture box, `tab` names the call and `esc` steps back out of the name
before it closes the box. Which of the two has the keys is drawn, not implied —
a stray tab used to swallow a whole page of notes into a one-line title.
Closing keeps the draft.

Search narrows as you type. Completing something holds it on screen, struck
through, before it folds away — and when the last thing today goes, the day
says so rather than reading like a day that never had anything on it.

## shape

```
main.go              tui, serve, add
internal/parse       the shorthand grammar, dates, tables, highlighting
internal/store       SQLite via modernc — no CGO
internal/palette     the colours, and which one a topic gets
internal/api         JSON over the top of both
internal/tui         the terminal app
internal/ui          the built client, embedded
ui/                  React + Vite + Tailwind
mac/                 the optional capture bar (Swift, AppKit)
```

There is one parser and it is in Go. The capture preview, the list, the CLI and
the macOS window all go through it, so they cannot disagree about what a line
means — every client renders what it is told and derives nothing.

Lines arrive with the characters their source gave them, so the parser rewrites
the look-alikes first: a fullwidth or box-drawing bar copied out of a rendered
table is read as the separator it plainly is, rather than skipped as a line with
no separator at all.

Database at `~/.local/share/todo/todo.db`, or set `TODO_DB`.

It binds to 127.0.0.1 and refuses any request that was not addressed to loopback
or that names another site as its origin. That matters more than it sounds: a
server on localhost is reachable by every page you visit, and a POST with a
plain content type is never preflighted, so without the check any website could
write to your list. Settings has **export everything** — it is your data, in
plain text, whenever you want it.
