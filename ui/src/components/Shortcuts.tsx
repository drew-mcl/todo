import { Dialog } from "./Dialog";

const KEYS: { keys: string[]; label: string }[][] = [
  [
    { keys: ["n"], label: "capture" },
    { keys: ["⌘", "↵"], label: "add what you've typed" },
    { keys: ["esc"], label: "close anything" },
    { keys: ["⌘", "k"], label: "command palette" },
    { keys: ["/"], label: "search" },
    { keys: ["f"], label: "filter by who, when, what" },
    { keys: ["b"], label: "hide the sidebar" },
    { keys: ["?"], label: "this list" },
  ],
  [
    { keys: ["j"], label: "down" },
    { keys: ["k"], label: "up" },
    { keys: ["x"], label: "complete" },
    { keys: ["e"], label: "edit" },
    { keys: ["d", "d"], label: "delete" },
    { keys: ["u"], label: "undo the last paste" },
  ],
];

const JUMPS: { keys: string[]; label: string }[] = [
  { keys: ["t"], label: "today" },
  { keys: ["c"], label: "calls" },
  { keys: ["w"], label: "plan the week" },
  { keys: ["a"], label: "everything open" },
  { keys: ["l"], label: "logbook" },
  { keys: ["g", "o"], label: "overdue" },
  { keys: ["g", "u"], label: "upcoming" },
  { keys: ["g", "a"], label: "anytime" },
  { keys: ["g", "d"], label: "delegated" },
];

function Key({ k }: { k: string }) {
  return (
    <kbd className="rounded-xs border border-line bg-sunk px-1.5 py-0.5 font-mono text-2xs text-ink-3">
      {k}
    </kbd>
  );
}

function Row({ keys, label }: { keys: string[]; label: string }) {
  return (
    <div className="flex items-center gap-2 py-1">
      <span className="flex w-[70px] shrink-0 gap-0.5">
        {keys.map((k, i) => (
          <Key key={i} k={k} />
        ))}
      </span>
      <span className="text-base text-ink-3">{label}</span>
    </div>
  );
}

export function Shortcuts({ open, onClose }: { open: boolean; onClose: () => void }) {
  return (
    <Dialog open={open} onClose={onClose} label="Keyboard shortcuts" width={560}>
      <header className="flex items-center justify-between border-b border-line-soft px-4 py-3">
        <h2 className="text-base text-ink">keyboard</h2>
        <button
          onClick={onClose}
          className="grid h-6 w-6 place-items-center rounded-md text-ink-4 hover:bg-sunk hover:text-ink"
          aria-label="Close"
        >
          ✕
        </button>
      </header>
      <div className="max-h-[60vh] overflow-y-auto p-4">
        <div className="grid gap-x-8 sm:grid-cols-2">
          {KEYS.map((col, i) => (
            <div key={i}>
              {col.map((r) => (
                <Row key={r.label} {...r} />
              ))}
            </div>
          ))}
        </div>
        <div className="mt-3 border-t border-line-soft pt-3">
          <h3 className="mb-1 font-mono text-2xs tracking-[0.14em] text-ink-4 uppercase">jump</h3>
          <div className="grid gap-x-8 sm:grid-cols-2">
            {JUMPS.map((r) => (
              <Row key={r.label} {...r} />
            ))}
          </div>
        </div>
        <div className="mt-4 border-t border-line-soft pt-3">
          <h3 className="mb-2 font-mono text-2xs tracking-[0.14em] text-ink-4 uppercase">
            shorthand
          </h3>
          <div className="space-y-1.5 font-mono text-xs">
            <p className="text-ink-3">
              <span className="font-medium text-ink">topic</span>
              <span className="text-ink-4"> | </span>what needs doing
              <span className="text-ink-4"> | </span>
              <span className="text-accent">today</span> <span className="text-t0">@who</span>{" "}
              <span className="text-danger">!!</span> <span className="text-t2">#tag</span>{" "}
              <span className="text-ink-3 italic">&gt; note</span>
            </p>
            <p className="text-ink-4">
              <span className="font-medium text-ink-2">|</span> at the start of a line repeats the
              topic above · a line with no <span className="font-medium text-ink-2">|</span> is
              left alone
            </p>
          </div>

          <dl className="mt-3 grid grid-cols-[auto_1fr] gap-x-4 gap-y-1 font-mono text-xs">
            <dt className="text-ink-3">dates</dt>
            <dd className="m-0 text-ink-4">
              today · tomorrow · eow · eom · fri · next fri · +3d · +2w · 25/12 · 25 dec ·
              2026-12-25 · someday
            </dd>
            <dt className="text-ink-3">priority</dt>
            <dd className="m-0 text-ink-4">! high · !! urgent · !!! critical</dd>
            <dt className="text-ink-3">tables</dt>
            <dd className="m-0 text-ink-4">
              paste a tracker or a Copilot summary — columns are read from the headers and can be
              remapped
            </dd>
          </dl>
        </div>
      </div>
    </Dialog>
  );
}
