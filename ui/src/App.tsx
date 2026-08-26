import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { AnimatePresence, motion } from "motion/react";
import { api, type Task } from "./api";
import { useRoute } from "./lib/route";
import { usePrefs } from "./lib/prefs";
import { HueProvider } from "./lib/hues";
import { useTaskMutations } from "./lib/mutations";
import { useKeyboard } from "./lib/keys";
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

/** How long a completed task stays struck through before it folds away. */
const LINGER = 700;

export function App() {
  const { route, go } = useRoute();
  const { prefs, update, setField } = usePrefs();

  const [overlay, setOverlay] = useState<Overlay>(null);
  const [detail, setDetail] = useState<Task>();
  const [cursorId, setCursorId] = useState<number | null>(null);
  const [undo, setUndo] = useState<{ batchId: number; added: number }>();
  const [problem, setProblem] = useState<string>();
  const [searching, setSearching] = useState(false);
  const [filterOpen, setFilterOpen] = useState(false);
  const [draftQuery, setDraftQuery] = useState(route.filters.q ?? "");
  const searchRef = useRef<HTMLInputElement>(null);

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
  const onFailed = useCallback(
    (e: unknown) => setProblem(e instanceof Error ? e.message : "That did not work."),
    [],
  );
  const { toggle, schedule, move, remove, undoBatch, invalidateAll } = useTaskMutations({
    filters,
    isWeek,
    linger: LINGER,
    onFailed,
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

  useKeyboard({
    go,
    filters,
    cursorTask,
    overlayOpen: overlay !== null,
    detailOpen: detail !== undefined,
    searching,
    canUndo: undo !== undefined,
    openOverlay: setOverlay,
    closeOverlay: () => setOverlay(null),
    closeDetail: () => setDetail(undefined),
    openDetail: setDetail,
    moveCursor,
    toggle: (t) => toggle.mutate(t),
    remove,
    undo: () => undo && void undoBatch(undo.batchId).then(() => setUndo(undefined)),
    startSearch: () => {
      setSearching(true);
      setTimeout(() => searchRef.current?.focus(), 0);
    },
    stopSearch: () => {
      setSearching(false);
      setDraftQuery("");
    },
    toggleSidebar: () => update({ sidebar: !prefs.sidebar }),
    toggleFilters: () => setFilterOpen((o) => !o),
  });

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
    if (draftQuery === (filters.q ?? "")) return;
    const id = setTimeout(() => go(route.kind, { ...filters, q: draftQuery }), 160);
    return () => clearTimeout(id);
  }, [draftQuery, filters, route.kind, go]);

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
                value={draftQuery}
                placeholder="search"
                onChange={(e) => setDraftQuery(e.target.value)}
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
          onSaved={invalidateAll}
          onDeleted={invalidateAll}
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
                onClick={() => void undoBatch(undo.batchId).then(() => setUndo(undefined))}
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
          invalidateAll();
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
