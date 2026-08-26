import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { AnimatePresence, motion } from "motion/react";
import { api, type ListResponse, type Task } from "./api";
import { useRoute } from "./lib/route";
import { usePrefs } from "./lib/prefs";
import { HueProvider } from "./lib/hues";
import { Sidebar } from "./components/Sidebar";
import { StatusBar } from "./components/StatusBar";
import { TaskList } from "./components/TaskList";
import { Week } from "./components/Week";
import { Capture } from "./components/Capture";
import { Detail } from "./components/Detail";
import { Palette } from "./components/Palette";
import { Settings } from "./components/Settings";
import { Shortcuts } from "./components/Shortcuts";
import { Select } from "./components/Select";
import { FilterButton, FilterRow, DayHeader, OverdueNudge } from "./components/FilterBar";
import { Calls } from "./components/Calls";

type Overlay = "capture" | "palette" | "settings" | "shortcuts" | null;

// g-chords reach the narrowing filters; the four places you live have a plain
// key of their own.
const JUMPS: Record<string, string> = {
  o: "overdue",
  u: "upcoming",
  a: "anytime",
  d: "delegated",
};

export function App() {
  const { route, go } = useRoute();
  const { prefs, update, setField } = usePrefs();
  const qc = useQueryClient();

  const [overlay, setOverlay] = useState<Overlay>(null);
  const [detail, setDetail] = useState<Task>();
  const [cursorId, setCursorId] = useState<number | null>(null);
  const [undo, setUndo] = useState<{ batchId: number; added: number }>();
  const [problem, setProblem] = useState<string>();
  const [searching, setSearching] = useState(false);
  const [filterOpen, setFilterOpen] = useState(false);
  const searchRef = useRef<HTMLInputElement>(null);
  const chord = useRef<string | null>(null);

  const isWeek = route.kind === "week";
  const isCalls = route.kind === "calls";
  const filters = route.filters;

  const list = useQuery({
    queryKey: ["list", filters],
    queryFn: () => api.list(filters),
    enabled: !isWeek && !isCalls,
  });
  const week = useQuery({
    queryKey: ["week", filters],
    queryFn: () => api.week(filters),
    enabled: isWeek,
  });

  const meta = (isWeek ? week.data?.meta : list.data?.meta) ?? undefined;
  const overdueCount = meta?.counts.overdue ?? 0;
  const flat = useMemo(
    () => (isWeek || isCalls ? [] : (list.data?.sections.flatMap((s) => s.tasks) ?? [])),
    [list.data, isWeek, isCalls],
  );
  // Everything, for a capture or an undo that can change any list.
  const invalidate = useCallback(() => {
    void qc.invalidateQueries({ queryKey: ["list"] });
    void qc.invalidateQueries({ queryKey: ["week"] });
    void qc.invalidateQueries({ queryKey: ["sessions"] });
  }, [qc]);

  // Just the board you are looking at, for edits to a single task.
  const refresh = useCallback(() => {
    void qc.invalidateQueries({ queryKey: [isWeek ? "week" : "list"] });
  }, [qc, isWeek]);

  const onFailed = useCallback(
    (e: unknown) => setProblem(e instanceof Error ? e.message : "That did not work."),
    [],
  );

  const toggle = useMutation({
    mutationFn: (t: Task) => api.toggle(t.id),
    // Tick immediately; the list catches up behind it.
    onMutate: async (t: Task) => {
      await qc.cancelQueries({ queryKey: ["list", filters] });
      const previous = qc.getQueryData<ListResponse>(["list", filters]);
      qc.setQueryData<ListResponse>(["list", filters], (old) =>
        old
          ? {
              ...old,
              sections: old.sections.map((s) => ({
                ...s,
                tasks: s.tasks.map((x) => (x.id === t.id ? { ...x, done: !x.done } : x)),
              })),
            }
          : old,
      );
      return { previous };
    },
    onError: (e, _t, ctx) => {
      if (ctx?.previous) qc.setQueryData(["list", filters], ctx.previous);
      onFailed(e);
    },
    onSettled: refresh,
  });
  const schedule = useMutation({
    mutationFn: ({ id, date }: { id: number; date: string }) => api.schedule(id, date),
    onSuccess: refresh,
    onError: onFailed,
  });
  const move = useMutation({
    mutationFn: ({ id, above, below }: { id: number; above: number; below: number }) =>
      api.move(id, above, below),
    onSuccess: refresh,
    onError: onFailed,
  });

  useEffect(() => setCursorId(null), [
    route.kind, filters.view, filters.topic, filters.assignee, filters.tag, filters.q,
  ]);

  // Held by id rather than by index: completing a task used to leave the cursor
  // pointing at whatever slid up into that row.
  const cursorIndex = cursorId === null ? 0 : Math.max(0, flat.findIndex((t) => t.id === cursorId));
  const cursorTask = flat[cursorIndex];

  const moveCursor = useCallback(
    (delta: number) => {
      if (flat.length === 0) return;
      const next = Math.min(flat.length - 1, Math.max(0, cursorIndex + delta));
      setCursorId(flat[next].id);
    },
    [flat, cursorIndex],
  );

  const setFilter = useCallback(
    (kind: "topic" | "assignee" | "tag", value: string) =>
      go("list", { view: "all", topic: "", assignee: "", tag: "", [kind]: value }),
    [go],
  );

  // ── Keyboard ──────────────────────────────────────────────────────────────
  useEffect(() => {
    function onKey(e: KeyboardEvent) {
      const target = e.target as HTMLElement | null;
      const typing =
        target &&
        (target.tagName === "INPUT" ||
          target.tagName === "TEXTAREA" ||
          target.isContentEditable);

      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === "k") {
        e.preventDefault();
        setOverlay((o) => (o === "palette" ? null : "palette"));
        return;
      }
      if (e.key === "Escape") {
        if (overlay) return setOverlay(null);
        if (detail) return setDetail(undefined);
        if (searching) {
          setSearching(false);
          if (filters.q) go("list", { ...filters, q: "" });
          return;
        }
        if (typing) (target as HTMLElement).blur();
        return;
      }
      if (typing || e.metaKey || e.ctrlKey || e.altKey || overlay) return;

      if (chord.current === "g") {
        chord.current = null;
        const view = JUMPS[e.key];
        if (view) {
          e.preventDefault();
          go("list", { view });
        }
        return;
      }

      switch (e.key) {
        case "g":
          chord.current = "g";
          setTimeout(() => (chord.current = null), 800);
          break;
        case "n":
          e.preventDefault();
          setOverlay("capture");
          break;
        case "t":
          e.preventDefault();
          go("list", { view: "today" });
          break;
        case "a":
          e.preventDefault();
          go("list", { view: "all" });
          break;
        case "l":
          e.preventDefault();
          go("list", { view: "logbook" });
          break;
        case "c":
          e.preventDefault();
          go("calls", {});
          break;
        case "w":
          e.preventDefault();
          go("week", {});
          break;
        case "b":
        case "[":
          e.preventDefault();
          update({ sidebar: !prefs.sidebar });
          break;
        case "f":
          e.preventDefault();
          setFilterOpen((o) => !o);
          break;
        case "?":
          e.preventDefault();
          setOverlay("shortcuts");
          break;
        case "/":
          e.preventDefault();
          setSearching(true);
          setTimeout(() => searchRef.current?.focus(), 0);
          break;
        case "j":
          e.preventDefault();
          moveCursor(cursorId === null ? 0 : 1);
          break;
        case "k":
          e.preventDefault();
          moveCursor(cursorId === null ? 0 : -1);
          break;
        case "x":
          if (cursorTask) {
            e.preventDefault();
            toggle.mutate(cursorTask);
          }
          break;
        case "e":
          if (cursorTask) {
            e.preventDefault();
            setDetail(cursorTask);
          }
          break;
        case "d":
          if (chord.current === "d") {
            chord.current = null;
            if (cursorTask) {
              e.preventDefault();
              void api.remove(cursorTask.id).then(invalidate);
            }
          } else {
            chord.current = "d";
            setTimeout(() => (chord.current = null), 600);
          }
          break;
        case "u":
          if (undo) {
            e.preventDefault();
            void api.undoBatch(undo.batchId).then(() => {
              setUndo(undefined);
              invalidate();
            });
          }
          break;
      }
    }
    addEventListener("keydown", onKey);
    return () => removeEventListener("keydown", onKey);
  }, [overlay, detail, searching, filters, flat.length, cursorTask, cursorId, moveCursor, undo, prefs.sidebar, go, update, toggle, invalidate]);

  // Keep the keyboard cursor in view as it moves.
  useEffect(() => {
    if (!cursorTask) return;
    document
      .querySelector(`[data-task-id="${cursorTask.id}"]`)
      ?.scrollIntoView({ block: "nearest" });
  }, [cursorTask]);

  useEffect(() => {
    if (!undo) return;
    const id = setTimeout(() => setUndo(undefined), 12_000);
    return () => clearTimeout(id);
  }, [undo]);

  useEffect(() => {
    if (!problem) return;
    const id = setTimeout(() => setProblem(undefined), 6_000);
    return () => clearTimeout(id);
  }, [problem]);

  const heading = isWeek
    ? "week"
    : isCalls
      ? "calls"
      : filters.topic ||
        (filters.tag && `#${filters.tag}`) ||
        filters.assignee ||
        filters.view ||
        "today";
  const filtered = Boolean(
    filters.topic || filters.tag || filters.assignee || filters.q || filters.when,
  );

  return (
    <HueProvider topics={(meta?.topics ?? []).map((t) => t.name)}>
      <div className="flex h-full">
      {prefs.sidebar && (
        <Sidebar
          meta={meta}
          filters={filters}
          kind={route.kind}
          onGo={go}
          onCapture={() => setOverlay("capture")}
          onSettings={() => setOverlay("settings")}
          onShortcuts={() => setOverlay("shortcuts")}
          onCollapse={() => update({ sidebar: false })}
        />
      )}

      <div className="relative flex min-w-0 flex-1 flex-col">
        <header className="flex shrink-0 items-center gap-3 px-6 pt-5 pb-3">
          {!prefs.sidebar && (
            <button
              onClick={() => update({ sidebar: true })}
              title="Show sidebar  ["
              className="grid h-7 w-7 place-items-center rounded-md text-ink-4 hover:bg-sunk hover:text-ink"
            >
              <svg viewBox="0 0 16 16" className="h-4 w-4" fill="none" stroke="currentColor" strokeWidth="1.3">
                <rect x="1.5" y="2.5" width="13" height="11" rx="2" />
                <path d="M6 2.5v11" />
              </svg>
            </button>
          )}
          <h1 className="text-xl text-ink">{heading}</h1>

          <div className="ml-auto flex items-center gap-2">
            {!isCalls && (
              <FilterButton
                open={filterOpen}
                onToggle={() => setFilterOpen((o) => !o)}
                filters={filters}
              />
            )}
            {(searching || filters.q) && (
              <input
                ref={searchRef}
                defaultValue={filters.q}
                placeholder="search"
                onKeyDown={(e) => {
                  if (e.key === "Enter")
                    go(route.kind, { ...filters, q: (e.target as HTMLInputElement).value });
                }}
                className="w-52 rounded-md border border-line bg-surface px-2.5 py-1.5 font-mono text-base outline-none focus:border-accent"
              />
            )}
            <button
              onClick={() => {
                setSearching(true);
                setTimeout(() => searchRef.current?.focus(), 0);
              }}
              title="Search  /"
              className="grid h-8 w-8 place-items-center rounded-md border border-line text-ink-3 hover:border-ink-4 hover:text-ink"
            >
              <svg viewBox="0 0 16 16" className="h-[15px] w-[15px]" fill="none" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round">
                <circle cx="7" cy="7" r="4.6" />
                <path d="M10.4 10.4L14 14" />
              </svg>
            </button>
            {!isWeek && !isCalls && (
              <Select
                label="Sort by"
                value={(list.data?.sort ?? "manual") as string}
                onChange={(sort) => go("list", { ...filters, sort })}
                options={["manual", "due", "created", "topic", "assignee", "priority"].map((s) => ({
                  value: s,
                  label: s,
                }))}
              />
            )}
          </div>
        </header>

        {!isCalls && (
          <FilterRow
            open={filterOpen}
            meta={meta}
            filters={filters}
            onChange={(f) => go(route.kind, { ...f, view: f.view ?? "all" })}
          />
        )}

        <main className="min-h-0 flex-1 overflow-y-auto px-6 pb-16">
          {!isWeek && !isCalls && filters.view === "today" && !filtered && (
            <>
              <DayHeader meta={meta} />
              <OverdueNudge
                count={overdueCount}
                onGo={() => go("list", { view: "overdue" })}
              />
            </>
          )}
          {isCalls ? (
            <Calls
              onOpenSession={(id) =>
                go("list", {
                  view: "all", batch: String(id), q: "", topic: "", assignee: "", tag: "",
                })
              }
            />
          ) : isWeek
            ? week.data && (
                <Week
                  data={week.data}
                  fields={prefs.fields}
                  onSchedule={(id, date) => schedule.mutate({ id, date })}
                  onToggle={(t) => toggle.mutate(t)}
                  onOpen={setDetail}
                  onNavigate={(start) => go("week", { ...filters, start })}
                />
              )
            : list.data && (
                <TaskList
                  data={list.data}
                  fields={prefs.fields}
                  compact={prefs.density === "compact"}
                  cursorId={cursorTask?.id ?? null}
                  onToggle={(t) => toggle.mutate(t)}
                  onOpen={setDetail}
                  onMove={(id, above, below) => move.mutate({ id, above, below })}
                  onFilter={setFilter}
                  filtered={filtered}
                  onClearFilter={() => go("list", { view: filters.view ?? "today" })}
                />
              )}
          {list.data?.truncated && (
            <p className="mt-4 font-mono text-xs text-ink-4">
              showing the first {list.data.shown} of {list.data.total} — narrow it with a filter
            </p>
          )}
        </main>

        <StatusBar
          view={heading}
          position={!isWeek && !isCalls && flat.length ? cursorIndex + 1 : null}
          total={
            isWeek
              ? (week.data
                  ? week.data.days.reduce((n, d) => n + d.tasks.length, 0) +
                    week.data.overdue.length +
                    week.data.unscheduled.length
                  : 0)
              : (list.data?.total ?? 0)
          }
          filters={filters}
          hint={undo ? "u to undo" : isWeek ? "drag to schedule" : undefined}
        />

        <Detail
          task={detail}
          onClose={() => setDetail(undefined)}
          onSaved={invalidate}
          onDeleted={invalidate}
        />

        <AnimatePresence>
          {problem && (
            <motion.div
              initial={{ y: 8, opacity: 0 }}
              animate={{ y: 0, opacity: 1 }}
              exit={{ y: 8, opacity: 0 }}
              className="absolute bottom-10 left-1/2 z-20 flex -translate-x-1/2 items-center gap-3 rounded-full border border-danger/30 bg-danger-soft px-4 py-2 font-mono text-base text-danger backdrop-blur"
            >
              <span>{problem}</span>
              <button onClick={() => setProblem(undefined)} className="opacity-60 hover:opacity-100">
                ✕
              </button>
            </motion.div>
          )}
        </AnimatePresence>

        <AnimatePresence>
          {undo && (
            <motion.div
              initial={{ y: 8, opacity: 0 }}
              animate={{ y: 0, opacity: 1 }}
              exit={{ y: 8, opacity: 0 }}
              className="absolute bottom-10 left-1/2 z-20 flex -translate-x-1/2 items-center gap-3 rounded-full bg-ink px-4 py-2 font-mono text-base text-bg shadow-[var(--shadow-pop)]"
            >
              <span>
                added {undo.added} task{undo.added === 1 ? "" : "s"}
              </span>
              <button
                onClick={() =>
                  void api.undoBatch(undo.batchId).then(() => {
                    setUndo(undefined);
                    invalidate();
                  })
                }
                className="rounded-full bg-bg/15 px-2.5 py-0.5 hover:bg-bg/25"
              >
                undo
              </button>
            </motion.div>
          )}
        </AnimatePresence>
      </div>

      <Capture
        open={overlay === "capture"}
        onClose={() => setOverlay(null)}
        onAdded={(batchId, added) => {
          setUndo({ batchId, added });
          invalidate();
        }}
      />
      <Palette
        open={overlay === "palette"}
        onClose={() => setOverlay(null)}
        meta={meta}
        onGo={go}
        onCapture={() => setOverlay("capture")}
        onSettings={() => setOverlay("settings")}
        onShortcuts={() => setOverlay("shortcuts")}
      />
      <Settings
        open={overlay === "settings"}
        onClose={() => setOverlay(null)}
        prefs={prefs}
        update={update}
        setField={setField}
      />
      <Shortcuts open={overlay === "shortcuts"} onClose={() => setOverlay(null)} />
      </div>
    </HueProvider>
  );
}
