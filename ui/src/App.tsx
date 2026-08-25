import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { AnimatePresence, motion } from "motion/react";
import { api, type Task } from "./api";
import { useRoute } from "./lib/route";
import { usePrefs } from "./lib/prefs";
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

type Overlay = "capture" | "palette" | "settings" | "shortcuts" | null;

const JUMPS: Record<string, string> = {
  t: "today",
  u: "upcoming",
  a: "anytime",
  d: "delegated",
  l: "logbook",
};

export function App() {
  const { route, go } = useRoute();
  const { prefs, update, setField } = usePrefs();
  const qc = useQueryClient();

  const [overlay, setOverlay] = useState<Overlay>(null);
  const [detail, setDetail] = useState<Task>();
  const [cursor, setCursor] = useState(0);
  const [undo, setUndo] = useState<{ batchId: number; added: number }>();
  const [searching, setSearching] = useState(false);
  const searchRef = useRef<HTMLInputElement>(null);
  const chord = useRef<string | null>(null);

  const isWeek = route.kind === "week";
  const filters = route.filters;

  const list = useQuery({
    queryKey: ["list", filters],
    queryFn: () => api.list(filters),
    enabled: !isWeek,
  });
  const week = useQuery({
    queryKey: ["week", filters],
    queryFn: () => api.week(filters),
    enabled: isWeek,
  });

  const meta = (isWeek ? week.data?.meta : list.data?.meta) ?? undefined;
  const flat = useMemo(
    () => list.data?.sections.flatMap((s) => s.tasks) ?? [],
    [list.data],
  );
  const invalidate = useCallback(() => {
    void qc.invalidateQueries({ queryKey: ["list"] });
    void qc.invalidateQueries({ queryKey: ["week"] });
  }, [qc]);

  const toggle = useMutation({
    mutationFn: (t: Task) => api.toggle(t.id),
    onSuccess: invalidate,
  });
  const schedule = useMutation({
    mutationFn: ({ id, date }: { id: number; date: string }) => api.schedule(id, date),
    onSuccess: invalidate,
  });
  const move = useMutation({
    mutationFn: ({ id, above, below }: { id: number; above: number; below: number }) =>
      api.move(id, above, below),
    onSuccess: invalidate,
  });

  useEffect(() => setCursor(0), [route.kind, filters.view, filters.topic, filters.assignee, filters.tag, filters.q]);

  const cursorTask = flat[cursor];

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
        case "a":
          e.preventDefault();
          go("list", { view: "all" });
          break;
        case "w":
          e.preventDefault();
          go("week", {});
          break;
        case "[":
          e.preventDefault();
          update({ sidebar: !prefs.sidebar });
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
          setCursor((c) => Math.min(flat.length - 1, c + 1));
          break;
        case "k":
          e.preventDefault();
          setCursor((c) => Math.max(0, c - 1));
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
  }, [overlay, detail, searching, filters, flat.length, cursorTask, undo, prefs.sidebar, go, update, toggle, invalidate]);

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

  const heading = isWeek
    ? "week"
    : filters.topic || (filters.tag && `#${filters.tag}`) || filters.assignee || filters.view || "today";
  const filtered = Boolean(filters.topic || filters.tag || filters.assignee || filters.q);

  return (
    <div className="flex h-full">
      {prefs.sidebar && (
        <Sidebar
          meta={meta}
          filters={filters}
          isWeek={isWeek}
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
            {!isWeek && (
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

        <main className="min-h-0 flex-1 overflow-y-auto px-6 pb-16">
          {isWeek
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
        </main>

        <StatusBar
          view={heading}
          position={!isWeek && flat.length ? cursor + 1 : null}
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
  );
}
