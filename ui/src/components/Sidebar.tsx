import clsx from "clsx";
import type { Filters, Meta } from "../api";
import { TopicDot } from "./TaskRow";

const VIEWS = [
  { id: "today", label: "today" },
  { id: "upcoming", label: "upcoming" },
  { id: "anytime", label: "anytime" },
  { id: "delegated", label: "delegated" },
  { id: "logbook", label: "logbook" },
];

function Row({
  label,
  count,
  hint,
  active,
  dot,
  onClick,
}: {
  label: string;
  count?: number;
  hint?: string;
  active?: boolean;
  dot?: string;
  onClick: () => void;
}) {
  return (
    <button
      onClick={onClick}
      className={clsx(
        "flex w-full items-center gap-2 rounded-md px-2 py-[5px] text-left font-mono text-base transition-colors",
        active ? "bg-sunk text-ink" : "text-ink-3 hover:bg-sunk/60 hover:text-ink-2",
      )}
    >
      {dot !== undefined && <TopicDot topic={dot} />}
      <span className="flex-1 truncate">{label}</span>
      {hint && (
        <kbd className="rounded-xs border border-line px-1 text-2xs text-ink-4">{hint}</kbd>
      )}
      {count !== undefined && count > 0 && (
        <span className="text-xs tabular-nums text-ink-4">{count}</span>
      )}
    </button>
  );
}

function Group({
  title,
  items,
  active,
  dots,
  onPick,
}: {
  title: string;
  items: { name: string; count: number }[];
  active?: string;
  dots?: boolean;
  onPick: (name: string) => void;
}) {
  if (items.length === 0) return null;
  return (
    <section className="mt-5">
      <h2 className="px-2 pb-1 font-mono text-2xs tracking-[0.14em] text-ink-4 uppercase">
        {title}
      </h2>
      {items.map((it) => (
        <Row
          key={it.name}
          label={it.name}
          count={it.count}
          active={active === it.name}
          dot={dots ? it.name : undefined}
          onClick={() => onPick(it.name)}
        />
      ))}
    </section>
  );
}

export function Sidebar({
  meta,
  filters,
  isWeek,
  onGo,
  onCapture,
  onSettings,
  onShortcuts,
  onCollapse,
}: {
  meta?: Meta;
  filters: Filters;
  isWeek: boolean;
  onGo: (kind: "list" | "week", f: Filters) => void;
  onCapture: () => void;
  onSettings: () => void;
  onShortcuts: () => void;
  onCollapse: () => void;
}) {
  const filtered = filters.topic || filters.assignee || filters.tag;

  return (
    <aside className="flex h-full w-[228px] shrink-0 flex-col border-r border-line bg-sunk/40">
      <div className="flex items-center gap-2 px-3 pt-3 pb-2">
        <button
          onClick={() => onGo("list", { view: "all" })}
          className="flex flex-1 items-center gap-2"
          title="Everything open"
        >
          <svg viewBox="0 0 24 24" className="h-[18px] w-[18px]">
            <rect x="1" y="3" width="2.2" height="18" rx="1.1" fill="var(--color-accent)" />
            <rect x="8" y="4.6" width="14" height="2.2" rx="1.1" fill="currentColor" opacity=".85" />
            <rect x="8" y="10.9" width="10" height="2.2" rx="1.1" fill="currentColor" opacity=".5" />
            <rect x="8" y="17.2" width="14" height="2.2" rx="1.1" fill="currentColor" opacity=".25" />
          </svg>
          <span className="font-mono text-base font-medium">todo</span>
        </button>
        <button
          onClick={onCollapse}
          title="Hide sidebar  ["
          className="grid h-6 w-6 place-items-center rounded-md text-ink-4 hover:bg-sunk hover:text-ink-2"
        >
          <svg viewBox="0 0 16 16" className="h-[15px] w-[15px]" fill="none" stroke="currentColor" strokeWidth="1.3">
            <rect x="1.5" y="2.5" width="13" height="11" rx="2" />
            <path d="M6 2.5v11" />
          </svg>
        </button>
      </div>

      <div className="min-h-0 flex-1 overflow-y-auto px-2 pb-3">
        <button
          onClick={onCapture}
          className="mb-2 flex w-full items-center gap-2 rounded-md px-2 py-[5px] text-left font-mono text-base font-medium text-ink hover:bg-sunk"
        >
          <span className="flex-1">capture</span>
          <kbd className="rounded-xs border border-line px-1 text-2xs text-ink-4">n</kbd>
        </button>

        <Row
          label="all"
          hint="a"
          active={!isWeek && filters.view === "all" && !filtered}
          onClick={() => onGo("list", { view: "all" })}
        />
        <Row label="week" hint="w" active={isWeek} onClick={() => onGo("week", {})} />

        <div className="my-2 border-t border-line" />

        {VIEWS.map((v) => (
          <Row
            key={v.id}
            label={v.label}
            count={meta?.counts[v.id]}
            active={!isWeek && filters.view === v.id && !filtered}
            onClick={() => onGo("list", { view: v.id })}
          />
        ))}

        <Group
          title="topics"
          items={meta?.topics ?? []}
          active={filters.topic}
          dots
          onPick={(name) => onGo(isWeek ? "week" : "list", { ...filters, view: "all", topic: name, tag: "", assignee: "" })}
        />
        <Group
          title="people"
          items={meta?.people ?? []}
          active={filters.assignee}
          onPick={(name) => onGo(isWeek ? "week" : "list", { ...filters, view: "all", assignee: name, topic: "", tag: "" })}
        />
        <Group
          title="tags"
          items={(meta?.tags ?? []).map((t) => ({ ...t, name: `#${t.name}` }))}
          active={filters.tag ? `#${filters.tag}` : undefined}
          onPick={(name) =>
            onGo(isWeek ? "week" : "list", {
              ...filters, view: "all", tag: name.replace(/^#/, ""), topic: "", assignee: "",
            })
          }
        />
      </div>

      <div className="flex items-center gap-1 border-t border-line px-3 py-2">
        <button
          onClick={onShortcuts}
          className="flex-1 text-left font-mono text-xs text-ink-4 hover:text-ink-2"
        >
          shortcuts <kbd className="ml-1 rounded-xs border border-line px-1 text-2xs">?</kbd>
        </button>
        <button
          onClick={onSettings}
          title="Settings"
          className="grid h-6 w-6 place-items-center rounded-md text-ink-4 hover:bg-sunk hover:text-ink-2"
        >
          <svg viewBox="0 0 20 20" className="h-4 w-4" fill="none" stroke="currentColor" strokeWidth="1.4" strokeLinecap="round">
            <circle cx="10" cy="10" r="2.5" />
            <path d="M10 2v2.2M10 15.8V18M18 10h-2.2M4.2 10H2M15.7 4.3l-1.6 1.6M5.9 14.1l-1.6 1.6M15.7 15.7l-1.6-1.6M5.9 5.9L4.3 4.3" />
          </svg>
        </button>
      </div>
    </aside>
  );
}
