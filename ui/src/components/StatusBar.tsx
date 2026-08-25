import type { Filters } from "../api";

/**
 * A status line, the way an editor has one. It answers "what am I looking at,
 * how much of it is there, and what can I press" without occupying the page.
 */
export function StatusBar({
  view,
  position,
  total,
  filters,
  hint,
}: {
  view: string;
  position: number | null;
  total: number;
  filters: Filters;
  hint?: string;
}) {
  const chips = [
    filters.topic && `topic:${filters.topic}`,
    filters.assignee && `@${filters.assignee}`,
    filters.tag && `#${filters.tag}`,
    filters.q && `/${filters.q}`,
  ].filter(Boolean) as string[];

  return (
    <footer className="flex h-7 shrink-0 items-center gap-3 border-t border-line bg-sunk/60 px-3 font-mono text-xs text-ink-4">
      <span className="text-ink-2">{view}</span>
      <span className="tabular-nums">
        {position !== null ? `${position}/${total}` : `${total}`}
      </span>
      {chips.map((c) => (
        <span key={c} className="rounded-xs bg-accent-soft px-1.5 text-accent">
          {c}
        </span>
      ))}
      <span className="ml-auto hidden items-center gap-3 sm:flex">
        {hint && <span className="text-ink-3">{hint}</span>}
        <span>⌘K palette</span>
        <span>? keys</span>
      </span>
    </footer>
  );
}
