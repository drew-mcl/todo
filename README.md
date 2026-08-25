# todo

Paste your call notes. The action lines become tasks. Everything else is left alone.

```
go build -o todo . && ./todo serve --open
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
"          | write the postmortem | eow
admin | quarterly training | eow #compliance
some tool | overview tab shows the stale value @jo
```

`"` repeats the topic from the line above, so a run of items under one heading is
only named once. `^`, `,,`, `~` and a bare leading `|` all do the same thing.

Lines without a `|` are skipped, so you can paste a whole meeting's notes and only
the action items land. `@who`, `!`, and `#tag` work in any position.

Dates: `today` `tomorrow` `eow` `eom` `next week` `fri` `next fri` `+3d` `+2w` `25/12`
`25 dec` `2026-12-25` `someday`

Priority: `!` high, `!!` urgent, `!!!` critical.

## capture from the terminal

```
todo add "prod issue | check the logs | today @sam"
cat notes.txt | todo add
```

## week planning

```
todo serve   →   press w
```

Seven day columns. Drag a task onto a day to schedule it; drag it to a tray to
unschedule it. Anything late sits in **Overdue** until you give it a new day, so
browsing back to a past week shows exactly what still needs rescuing. Filter the
whole board by topic, person or tag.

## keys

`n` capture · `a` everything · `w` week · `j`/`k` move · `x` complete · `e` edit
`/` search · `u` undo · `[` hide the sidebar · `?` all of them
`g` then `t` `u` `a` `d` `l` to jump between filters

## settings

Five colour schemes (graphite, ink, paper, obsidian, amber) and light/dark/auto,
under Settings in the sidebar. Stored per browser.

## where things live

Database at `~/.local/share/todo/todo.db`, or set `TODO_DB`. Serves on 127.0.0.1 only —
no auth, so keep it off the network. One binary, no CGO, assets embedded.
