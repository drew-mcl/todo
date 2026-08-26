import { useEffect, useRef, useState } from "react";
import clsx from "clsx";
import { AnimatePresence, motion } from "motion/react";
import { WHEN, type Filters, type Meta } from "../api";
import { Select } from "./Select";

/**
 * Who, when and what -- the three questions asked of a list of commitments.
 * Folded away until wanted, and identical on every view so the answer is always
 * in the same place.
 */
function activeCount(f: Filters) {
  return [f.topic, f.assignee, f.tag, f.when].filter(Boolean).length;
}

/** The trigger. Lives with the other header controls. */
export function FilterButton({
  open,
  onToggle,
  filters,
}: {
  open: boolean;
  onToggle: () => void;
  filters: Filters;
}) {
  const active = activeCount(filters);

  return (
    <>
      <button
        onClick={onToggle}
        title="Filter"
        className={clsx(
          "relative grid h-8 w-8 place-items-center rounded-md border transition-colors",
          open || active
            ? "border-ink-4 text-ink"
            : "border-line text-ink-3 hover:border-ink-4 hover:text-ink",
        )}
      >
        <svg viewBox="0 0 16 16" className="h-[15px] w-[15px]" fill="none" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round">
          <path d="M2 4h12M4.5 8h7M7 12h2" />
        </svg>
        {active > 0 && (
          <span className="absolute -top-1 -right-1 grid h-[15px] min-w-[15px] place-items-center rounded-full bg-accent px-1 font-mono text-2xs text-bg">
            {active}
          </span>
        )}
      </button>

    </>
  );
}

/**
 * The row itself, opened below the header so the controls above it never move.
 */
export function FilterRow({
  open,
  meta,
  filters,
  onChange,
}: {
  open: boolean;
  meta?: Meta;
  filters: Filters;
  onChange: (f: Filters) => void;
}) {
  const active = activeCount(filters);

  return (
    <>
      <AnimatePresence initial={false}>
        {open && (
          <motion.div
            initial={{ height: 0, opacity: 0 }}
            animate={{ height: "auto", opacity: 1 }}
            exit={{ height: 0, opacity: 0 }}
            transition={{ duration: 0.16, ease: [0.22, 1, 0.36, 1] }}
            className="overflow-hidden"
          >
            <div className="flex flex-wrap items-center gap-2 border-b border-line-soft px-6 pt-1 pb-3">
              <Select
                label="Topic"
                value={filters.topic ?? ""}
                onChange={(topic) => onChange({ ...filters, topic })}
                options={[
                  { value: "", label: "any topic" },
                  ...(meta?.topics ?? []).map((t) => ({ value: t.name, label: t.name })),
                ]}
              />
              <Select
                label="Person"
                value={filters.assignee ?? ""}
                onChange={(assignee) => onChange({ ...filters, assignee })}
                options={[
                  { value: "", label: "anyone" },
                  ...(meta?.people ?? []).map((p) => ({ value: p.name, label: p.name })),
                ]}
              />
              <Select
                label="Tag"
                value={filters.tag ?? ""}
                onChange={(tag) => onChange({ ...filters, tag })}
                options={[
                  { value: "", label: "any tag" },
                  ...(meta?.tags ?? []).map((t) => ({ value: t.name, label: `#${t.name}` })),
                ]}
              />
              <Select
                label="Captured"
                value={filters.when ?? ""}
                onChange={(when) => onChange({ ...filters, when })}
                options={WHEN}
              />
              {active > 0 && (
                <button
                  onClick={() =>
                    onChange({ ...filters, topic: "", assignee: "", tag: "", when: "" })
                  }
                  className="font-mono text-base text-accent underline underline-offset-2"
                >
                  clear
                </button>
              )}
            </div>
          </motion.div>
        )}
      </AnimatePresence>
    </>
  );
}

/**
 * Counts up rather than jumping. Closing something is the moment worth marking,
 * and a number that ticks over says it happened.
 */
function useCountUp(target: number, ms = 400) {
  const [n, setN] = useState(target);
  const from = useRef(target);

  useEffect(() => {
    const start = performance.now();
    const a = from.current;
    if (a === target) return;
    let raf = 0;
    const step = (t: number) => {
      const p = Math.min(1, (t - start) / ms);
      // Same ease as everything else that moves.
      const eased = 1 - Math.pow(1 - p, 3);
      setN(Math.round(a + (target - a) * eased));
      if (p < 1) raf = requestAnimationFrame(step);
      else from.current = target;
    };
    raf = requestAnimationFrame(step);
    return () => cancelAnimationFrame(raf);
  }, [target, ms]);

  return n;
}

/** The day's own header: the date, and how much of it you have closed. */
export function DayHeader({ meta }: { meta?: Meta }) {
  const left = meta?.counts.today ?? 0;
  const done = meta?.doneToday ?? 0;
  const total = left + done;
  const shown = useCountUp(done);
  if (!meta) return null;
  const pct = total === 0 ? 0 : Math.round((done / total) * 100);

  return (
    <div className="mb-4 flex flex-wrap items-center gap-x-4 gap-y-2 border-b border-line-soft pb-3">
      <p className="font-mono text-base text-ink-2">{meta.todayLabel}</p>
      <div className="flex flex-1 items-center gap-3">
        <div className="h-[3px] min-w-24 flex-1 overflow-hidden rounded-full bg-line">
          <motion.div
            className="h-full rounded-full bg-accent"
            initial={false}
            animate={{ width: `${pct}%` }}
            transition={{ duration: 0.45, ease: [0.22, 1, 0.36, 1] }}
          />
        </div>
        <p className="font-mono text-xs whitespace-nowrap text-ink-4">
          {total === 0 ? "nothing due" : `${shown} of ${total} done`}
        </p>
      </div>
    </div>
  );
}

/** A quiet nudge when work has slipped, instead of it squatting on Today. */
export function OverdueNudge({ count, onGo }: { count: number; onGo: () => void }) {
  if (count === 0) return null;
  return (
    <button
      onClick={onGo}
      className="mb-3 flex w-full items-center gap-2 rounded-md border border-danger/25 bg-danger-soft px-3 py-2 text-left font-mono text-base text-danger transition-colors hover:border-danger/50"
    >
      <span className="flex-1">
        {count} {count === 1 ? "task has" : "tasks have"} slipped
      </span>
      <span className="text-xs opacity-70">reschedule →</span>
    </button>
  );
}
