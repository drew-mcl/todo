import { useEffect, useRef, useState } from "react";
import clsx from "clsx";
import { AnimatePresence, motion } from "motion/react";
import { api, type PreviewResponse, type Token, type TokenKind } from "../api";
import { draftStore } from "../lib/prefs";
import { TopicDot } from "./TaskRow";
import { TableInput } from "./TableInput";

type Mode = "shorthand" | "table" | "copilot";

const MODES: { id: Mode; label: string; hint: string }[] = [
  { id: "shorthand", label: "shorthand", hint: "type or paste your notes" },
  { id: "table", label: "table", hint: "paste a tracker or spreadsheet" },
  { id: "copilot", label: "copilot", hint: "paste the summary from Teams" },
];

/** Colour is spent here because here it is teaching the grammar. */
const TOKEN: Record<TokenKind, string> = {
  text: "text-ink-2",
  pipe: "text-ink-4",
  topic: "text-ink font-medium",
  due: "text-accent",
  who: "text-t0",
  pri: "text-danger font-semibold",
  tag: "text-t2",
  note: "text-ink-3 italic",
};

function Shorthand({ tokens, dim }: { tokens?: Token[]; dim?: boolean }) {
  if (!tokens) return null;
  return (
    <code className={clsx("font-mono text-base break-words whitespace-pre-wrap", dim && "opacity-45")}>
      {tokens.map((t, i) => (
        <span key={i} className={TOKEN[t.kind]}>
          {t.text}
        </span>
      ))}
    </code>
  );
}

const BANGS = ["", "!", "!!", "!!!"];

function Preview({
  data,
  onPromote,
}: {
  data?: PreviewResponse;
  onPromote: (line: number) => void;
}) {
  if (!data || data.lines.length === 0) {
    return (
      <p className="font-mono text-xs tracking-wide text-ink-4 uppercase">
        your lines appear here as they'll be filed
      </p>
    );
  }

  const summary = [
    data.tasks && `${data.tasks} task${data.tasks === 1 ? "" : "s"}`,
    data.notes && `${data.notes} note${data.notes === 1 ? "" : "s"}`,
    data.skipped && `${data.skipped} skipped`,
  ]
    .filter(Boolean)
    .join(" · ");

  return (
    <>
      <p className="mb-2 font-mono text-xs tracking-wide text-ink-4 uppercase">{summary}</p>
      <ol className="space-y-px">
        {data.lines.map((line) => {
          if (line.kind === "task" && line.task) {
            return (
              <li key={line.n} className="border-t border-line-soft py-1.5 first:border-t-0">
                <Shorthand tokens={line.tokens} />
                <div className="mt-1 flex flex-wrap items-baseline gap-x-2 border-l border-line pl-3">
                  <span className="text-md text-ink">{line.task.title}</span>
                  <span className="flex flex-wrap items-center gap-x-1.5 font-mono text-xs text-ink-3">
                    <span className="inline-flex items-center gap-1.5">
                      <TopicDot topic={line.task.topic} />
                      {line.task.topic}
                    </span>
                    {line.task.dueLabel && <span className="text-accent">· {line.task.dueLabel.toLowerCase()}</span>}
                    {line.task.assignee && <span className="text-t0">· {line.task.assignee}</span>}
                    {line.task.priority > 0 && (
                      <span className="text-danger">· {BANGS[line.task.priority]}</span>
                    )}
                    {line.task.tags.map((t) => (
                      <span key={t} className="text-t2">· #{t}</span>
                    ))}
                  </span>
                  {line.task.note && (
                    <p className="basis-full text-sm text-ink-3">{line.task.note}</p>
                  )}
                </div>
                {line.task.warning && (
                  <p className="mt-1 pl-3 font-mono text-xs text-danger">{line.task.warning}</p>
                )}
              </li>
            );
          }
          if (line.kind === "note") {
            return (
              <li key={line.n} className="flex items-baseline gap-2 border-t border-line-soft py-1.5">
                <Shorthand tokens={line.tokens} />
                <span className="font-mono text-xs text-ink-4">↳ attached</span>
              </li>
            );
          }
          return (
            <li key={line.n} className="flex flex-wrap items-baseline gap-2 border-t border-line-soft py-1.5">
              <span className="flex-1 line-through decoration-line">
                <Shorthand tokens={line.tokens} dim />
              </span>
              <span className="font-mono text-xs text-ink-4">{line.reason}</span>
              <button
                onClick={() => onPromote(line.n)}
                className="rounded-full border border-line px-2 py-px font-mono text-xs text-ink-3 hover:border-ink-4 hover:text-ink"
              >
                make a task
              </button>
            </li>
          );
        })}
      </ol>
    </>
  );
}

export function Capture({
  open,
  onClose,
  onAdded,
}: {
  open: boolean;
  onClose: () => void;
  onAdded: (batchId: number, added: number) => void;
}) {
  const [mode, setMode] = useState<Mode>("shorthand");
  const [title, setTitle] = useState("");
  const [draft, setDraft] = useState(() => draftStore.read());
  const [preview, setPreview] = useState<PreviewResponse>();
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string>();
  const ref = useRef<HTMLTextAreaElement>(null);

  useEffect(() => {
    if (!open || mode !== "shorthand") return;
    const el = ref.current;
    el?.focus();
    el?.setSelectionRange(el.value.length, el.value.length);
  }, [open, mode]);

  // The parse is debounced but always server-side: one parser, and this is it.
  useEffect(() => {
    if (!open) return;
    draftStore.write(draft);
    if (!draft.trim()) {
      setPreview(undefined);
      return;
    }
    const id = setTimeout(() => {
      api.preview(draft).then(setPreview).catch(() => undefined);
    }, 120);
    return () => clearTimeout(id);
  }, [draft, open]);

  async function commit() {
    if (busy || !preview?.tasks) return;
    setBusy(true);
    setError(undefined);
    try {
      const { batchId, added } = await api.capture(draft, title);
      draftStore.clear();
      setDraft("");
      setPreview(undefined);
      onAdded(batchId, added);
      onClose();
    } catch (e) {
      setError(e instanceof Error ? e.message : "Could not add those.");
    } finally {
      setBusy(false);
    }
  }

  function promote(n: number) {
    const lines = draft.split("\n");
    if (!lines[n - 1]) return;
    lines[n - 1] = `inbox | ${lines[n - 1].trim()}`;
    setDraft(lines.join("\n"));
    ref.current?.focus();
  }

  return (
    <AnimatePresence>
      {open && (
        <motion.div
          initial={{ opacity: 0 }}
          animate={{ opacity: 1 }}
          exit={{ opacity: 0 }}
          transition={{ duration: 0.14 }}
          className="fixed inset-0 z-40 grid justify-items-center bg-ink/25 backdrop-blur-[3px]"
          onMouseDown={(e) => e.target === e.currentTarget && onClose()}
        >
          <motion.div
            initial={{ y: -10 }}
            animate={{ y: 0 }}
            exit={{ y: -10 }}
            transition={{ duration: 0.18, ease: [0.22, 1, 0.36, 1] }}
            className="flex h-full w-full max-w-[960px] flex-col border-x border-line bg-surface shadow-[var(--shadow-pop)]"
          >
            <header className="flex shrink-0 flex-wrap items-center gap-x-4 gap-y-2 border-b border-line-soft px-6 py-3">
              {/* Naming the call is what turns a pile of tasks into a record you
                  can come back to and send on afterwards. */}
              <input
                value={title}
                onChange={(e) => setTitle(e.target.value)}
                placeholder="name this call (optional)"
                className="min-w-0 flex-1 bg-transparent font-mono text-md text-ink outline-none placeholder:text-ink-4"
              />
              <div className="flex gap-0.5 rounded-md border border-line bg-sunk p-0.5">
                {MODES.map((m) => (
                  <button
                    key={m.id}
                    onClick={() => setMode(m.id)}
                    title={m.hint}
                    className={clsx(
                      "rounded-sm px-2.5 py-1 font-mono text-xs transition-colors",
                      mode === m.id ? "bg-raised text-ink" : "text-ink-3 hover:text-ink",
                    )}
                  >
                    {m.label}
                  </button>
                ))}
              </div>
              <p className="font-mono text-xs whitespace-nowrap text-ink-4">
                <kbd className="rounded-xs border border-line px-1">⌘</kbd>
                <kbd className="ml-px rounded-xs border border-line px-1">↵</kbd> add ·{" "}
                <kbd className="rounded-xs border border-line px-1">esc</kbd> close
              </p>
            </header>

            {mode !== "shorthand" ? (
              <TableInput
                key={mode}
                source={mode === "copilot" ? "copilot" : "table"}
                topic={title}
                title={title}
                placeholder={
                  mode === "copilot"
                    ? "Paste the action items table from Teams Copilot.\n\n| Action | Owner | Deadline | Notes |\n| --- | --- | --- | --- |\n| Chase the vendor | Sam | Friday | Missed two dates |"
                    : "Paste a table — from a spreadsheet, a markdown table, or CSV.\n\nIssue\tPlan\tOwner\tDue date\nIngest lag\tDecide on the fix\tsam\t+3d"
                }
                onCommitted={(batchId, added) => {
                  onAdded(batchId, added);
                  onClose();
                }}
              />
            ) : (
            <>

            <textarea
              ref={ref}
              value={draft}
              spellCheck={false}
              onChange={(e) => setDraft(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === "Enter" && (e.metaKey || e.ctrlKey)) {
                  e.preventDefault();
                  void commit();
                }
              }}
              placeholder={
                "Paste your notes. Lines with a | become tasks, everything else is left alone.\n\nprod issue | chase the vendor about the patch | today @sam !!\n> they have missed two dates now\n           | write the postmortem | eow #board"
              }
              className="min-h-[180px] flex-1 resize-none bg-transparent px-6 py-4 font-mono text-md leading-[1.8] text-ink outline-none placeholder:text-ink-4"
            />

            <div className="h-[clamp(150px,30vh,330px)] shrink-0 overflow-y-auto border-t border-line-soft bg-sunk px-6 py-3">
              <Preview data={preview} onPromote={promote} />
            </div>

            <footer className="flex shrink-0 flex-wrap items-center gap-x-5 gap-y-2 border-t border-line-soft px-6 py-3">
              <code className="font-mono text-xs text-ink-4">
                <span className="font-medium text-ink-2">topic</span>
                <span> | </span>what needs doing<span> | </span>
                <span className="text-accent">today</span> <span className="text-t0">@who</span>{" "}
                <span className="text-danger">!!</span> <span className="text-t2">#tag</span>{" "}
                <span className="italic">&gt; note</span>
              </code>
              <span className="font-mono text-xs text-ink-4">
                start a line with <span className="font-semibold text-ink-2">|</span> to repeat
                the topic above
              </span>
              {error && <span className="font-mono text-xs text-danger">{error}</span>}
              <div className="ml-auto flex items-center gap-2">
                <button
                  onClick={onClose}
                  className="rounded-md border border-line px-3 py-1.5 text-base text-ink-3 hover:border-ink-4 hover:text-ink"
                >
                  cancel
                </button>
                <button
                  onClick={commit}
                  disabled={!preview?.tasks || busy}
                  className="rounded-md bg-ink px-3.5 py-1.5 text-base font-medium text-bg transition-opacity hover:opacity-85 disabled:opacity-35"
                >
                  {preview?.tasks
                    ? `add ${preview.tasks} task${preview.tasks === 1 ? "" : "s"}`
                    : "add tasks"}
                </button>
              </div>
            </footer>
            </>
            )}
          </motion.div>
        </motion.div>
      )}
    </AnimatePresence>
  );
}
