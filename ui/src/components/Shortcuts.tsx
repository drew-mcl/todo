import { Dialog } from "./Dialog";

const KEYS: { keys: string[]; label: string }[][] = [
  [
    { keys: ["n"], label: "capture" },
    { keys: ["⌘", "↵"], label: "add what you've typed" },
    { keys: ["esc"], label: "close anything" },
    { keys: ["⌘", "k"], label: "command palette" },
    { keys: ["/"], label: "search" },
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
  { keys: ["a"], label: "everything open" },
  { keys: ["w"], label: "plan the week" },
  { keys: ["["], label: "hide the sidebar" },
  { keys: ["g", "t"], label: "today" },
  { keys: ["g", "u"], label: "upcoming" },
  { keys: ["g", "a"], label: "anytime" },
  { keys: ["g", "d"], label: "delegated" },
  { keys: ["g", "l"], label: "logbook" },
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
        <p className="mt-4 border-t border-line-soft pt-3 font-mono text-xs text-ink-4">
          <span className="font-medium text-ink-2">topic</span> | what needs doing |{" "}
          <span className="text-accent">today</span> <span className="text-t0">@who</span>{" "}
          <span className="text-danger">!!</span> <span className="text-t2">#tag</span>{" "}
          <span className="italic">&gt; note</span> — lines without a <b>|</b> are left alone.
        </p>
      </div>
    </Dialog>
  );
}
