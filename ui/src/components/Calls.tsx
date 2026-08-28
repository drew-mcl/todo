import { useState } from "react";
import clsx from "clsx";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, type Session } from "../api";
import { Dialog } from "./Dialog";
import { Toggle } from "./Field";

/**
 * Every capture, newest first. The list is a record of what was said and when,
 * so a commitment can be traced back to the call it was made on -- and sent
 * back out afterwards.
 */
export function Calls({
  onOpenSession,
}: {
  onOpenSession: (id: number) => void;
}) {
  const qc = useQueryClient();
  const [exporting, setExporting] = useState<Session>();
  const [editing, setEditing] = useState<number>();
  const [draftTitle, setDraftTitle] = useState("");
  // Filing twice for the same call is easy to do -- someone says one more thing
  // after you thought you were finished. Picking the pieces puts them back.
  const [picked, setPicked] = useState<number[]>([]);
  const [problem, setProblem] = useState<string>();

  const { data } = useQuery({ queryKey: ["sessions"], queryFn: api.sessions });
  const rename = useMutation({
    mutationFn: ({ id, title }: { id: number; title: string }) => api.renameSession(id, title),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["sessions"] }),
  });

  /** The oldest of the picked ones is kept: it is where the call started. */
  const merge = useMutation({
    mutationFn: () => {
      const order = (data ?? []).filter((s) => picked.includes(s.id)).map((s) => s.id);
      const into = order[order.length - 1];
      return api.mergeSessions(into, order.slice(0, -1));
    },
    onSuccess: () => {
      setPicked([]);
      qc.invalidateQueries({ queryKey: ["sessions"] });
      qc.invalidateQueries({ queryKey: ["list"] });
    },
    onError: (e: unknown) =>
      setProblem(e instanceof Error ? e.message : "Those would not merge."),
  });

  const pick = (id: number) =>
    setPicked((p) => (p.includes(id) ? p.filter((x) => x !== id) : [...p, id]));

  if (data && data.length === 0) {
    return (
      <div className="pt-16">
        <p className="text-lg text-ink-2">no captures yet.</p>
        <p className="mt-1.5 font-mono text-base text-ink-4">
          press n, name the call, and paste your notes.
        </p>
      </div>
    );
  }

  return (
    <>
      <ul className="max-w-[900px]">
        {data?.map((s) => (
          <li
            key={s.id}
            className={clsx(
              "group/row flex items-start gap-3 border-b border-line-soft py-3",
              picked.includes(s.id) && "bg-accent/8",
            )}
          >
            <input
              type="checkbox"
              checked={picked.includes(s.id)}
              onChange={() => pick(s.id)}
              aria-label={`Pick ${s.title} to merge`}
              className={clsx(
                "mt-1 shrink-0 accent-accent transition-opacity",
                picked.length > 0
                  ? "opacity-100"
                  : "opacity-0 group-hover/row:opacity-100 focus:opacity-100",
              )}
            />
            <div className="min-w-0 flex-1">
              {editing === s.id ? (
                <input
                  autoFocus
                  value={draftTitle}
                  onChange={(e) => setDraftTitle(e.target.value)}
                  onBlur={() => {
                    rename.mutate({ id: s.id, title: draftTitle });
                    setEditing(undefined);
                  }}
                  onKeyDown={(e) => {
                    if (e.key === "Enter") (e.target as HTMLInputElement).blur();
                    if (e.key === "Escape") setEditing(undefined);
                  }}
                  className="w-full rounded-sm bg-sunk px-1 font-mono text-md text-ink outline-none"
                />
              ) : (
                <button
                  onClick={() => onOpenSession(s.id)}
                  className="block max-w-full truncate text-left font-mono text-md text-ink hover:underline hover:underline-offset-2"
                >
                  {s.title}
                </button>
              )}
              <p className="mt-0.5 flex flex-wrap items-center gap-x-1.5 font-mono text-xs text-ink-3">
                <span>{s.when.toLowerCase()}</span>
                <span className="text-ink-4">·</span>
                <span>
                  {s.open} open of {s.total}
                </span>
                {s.source !== "web" && (
                  <>
                    <span className="text-ink-4">·</span>
                    <span className="text-t0">{s.source}</span>
                  </>
                )}
                {s.preview && (
                  <>
                    <span className="text-ink-4">·</span>
                    <span className="truncate text-ink-4">{s.preview}</span>
                  </>
                )}
              </p>
            </div>

            {/* Progress for the call, so a glance says whether it is settled. */}
            <div className="mt-2 hidden h-[3px] w-24 shrink-0 overflow-hidden rounded-full bg-line sm:block">
              <div
                className="h-full rounded-full bg-accent"
                style={{ width: `${s.total ? (s.done / s.total) * 100 : 0}%` }}
              />
            </div>

            <div className="flex shrink-0 gap-1 opacity-0 transition-opacity group-hover/row:opacity-100 focus-within:opacity-100">
              <button
                onClick={() => {
                  setEditing(s.id);
                  setDraftTitle(s.title);
                }}
                className="rounded-md border border-line px-2 py-1 font-mono text-xs text-ink-3 hover:border-ink-4 hover:text-ink"
              >
                rename
              </button>
              <button
                onClick={() => setExporting(s)}
                className="rounded-md border border-line px-2 py-1 font-mono text-xs text-ink-3 hover:border-ink-4 hover:text-ink"
              >
                share
              </button>
            </div>
          </li>
        ))}
      </ul>

      {picked.length > 0 && (
        <div className="sticky bottom-4 mt-4 flex max-w-[900px] flex-wrap items-center gap-3 rounded-md border border-line bg-raised px-4 py-3 shadow-[var(--shadow-pop)]">
          <span className="font-mono text-base text-ink-2">
            {picked.length} picked
          </span>
          <span className="font-mono text-xs text-ink-4">
            merging keeps the oldest and folds the rest into it
          </span>
          {problem && <span className="font-mono text-xs text-danger">{problem}</span>}
          <div className="ml-auto flex gap-2">
            <button
              onClick={() => {
                setPicked([]);
                setProblem(undefined);
              }}
              className="rounded-md border border-line px-3 py-1.5 text-base text-ink-3 hover:border-ink-4 hover:text-ink"
            >
              cancel
            </button>
            <button
              onClick={() => merge.mutate()}
              disabled={picked.length < 2 || merge.isPending}
              className="rounded-md bg-ink px-3.5 py-1.5 text-base font-medium text-bg transition-opacity hover:opacity-85 disabled:opacity-35"
            >
              merge {picked.length}
            </button>
          </div>
        </div>
      )}

      <ExportDialog session={exporting} onClose={() => setExporting(undefined)} />
    </>
  );
}

/**
 * What you paste into Teams after the call. Plain text on purpose: it survives
 * every chat client intact, which markdown tables do not.
 */
function ExportDialog({ session, onClose }: { session?: Session; onClose: () => void }) {
  const [byOwner, setByOwner] = useState(true);
  const [notes, setNotes] = useState(false);
  const [copied, setCopied] = useState(false);

  const { data } = useQuery({
    queryKey: ["export", session?.id, byOwner, notes],
    queryFn: () => api.exportSession(session!.id, byOwner, notes),
    enabled: Boolean(session),
  });

  async function copy() {
    if (!data) return;
    try {
      await navigator.clipboard.writeText(data.text);
      setCopied(true);
      setTimeout(() => setCopied(false), 1800);
    } catch {
      // Clipboard access can be refused; the text is on screen to copy by hand.
    }
  }

  return (
    <Dialog open={Boolean(session)} onClose={onClose} label="Share actions" width={620}>
      <header className="flex items-center justify-between border-b border-line-soft px-4 py-3">
        <h2 className="truncate text-base text-ink">share · {session?.title}</h2>
        <button
          onClick={onClose}
          className="grid h-6 w-6 shrink-0 place-items-center rounded-md text-ink-4 hover:bg-sunk hover:text-ink"
          aria-label="Close"
        >
          ✕
        </button>
      </header>

      <div className="max-h-[52vh] overflow-y-auto p-4">
        <pre className="rounded-md border border-line bg-sunk p-3 font-mono text-base leading-relaxed whitespace-pre-wrap text-ink-2">
          {data?.text ?? "…"}
        </pre>
      </div>

      <footer className="flex flex-wrap items-center gap-3 border-t border-line-soft px-4 py-3">
        <div className="flex-1">
          <Toggle checked={byOwner} onChange={setByOwner} label="group by owner" />
          <Toggle checked={notes} onChange={setNotes} label="include notes" />
        </div>
        <button
          onClick={copy}
          className={clsx(
            "rounded-md px-3.5 py-1.5 text-base font-medium transition-colors",
            copied ? "bg-accent text-bg" : "bg-ink text-bg hover:opacity-85",
          )}
        >
          {copied ? "copied" : "copy"}
        </button>
      </footer>
    </Dialog>
  );
}
