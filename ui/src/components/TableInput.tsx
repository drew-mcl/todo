import { useEffect, useState } from "react";
import clsx from "clsx";
import { api, type Role, type TablePreview } from "../api";
import { Select } from "./Select";
import { TopicDot } from "./TaskRow";

const BANGS = ["", "!", "!!", "!!!"];

const ROLE_LABELS: Record<Role, string> = {
  title: "task",
  topic: "topic",
  owner: "owner",
  due: "due",
  priority: "priority",
  tags: "tags",
  note: "notes",
  status: "status",
  ignore: "ignore",
};

/**
 * The other way work arrives: a tracker someone pasted, or Copilot's summary of
 * a call. The columns are guessed from the headers and shown as controls, so a
 * paste that guessed wrong is one dropdown away from right.
 */
export function TableInput({
  source,
  placeholder,
  topic,
  title,
  onCommitted,
}: {
  source: "table" | "copilot";
  placeholder: string;
  topic: string;
  title: string;
  onCommitted: (batchId: number, added: number) => void;
}) {
  const [input, setInput] = useState("");
  const [preview, setPreview] = useState<TablePreview>();
  const [mapping, setMapping] = useState<Role[]>([]);
  const [error, setError] = useState<string>();
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    if (!input.trim()) {
      setPreview(undefined);
      setError(undefined);
      return;
    }
    const id = setTimeout(() => {
      api
        .tablePreview(input, mapping.length ? mapping : undefined, topic)
        .then((p) => {
          setPreview(p);
          setError(undefined);
          if (!mapping.length) setMapping(p.columns.map((c) => c.role));
        })
        .catch((e) => {
          setPreview(undefined);
          setError(e instanceof Error ? e.message : "That does not look like a table.");
        });
    }, 150);
    return () => clearTimeout(id);
  }, [input, mapping, topic]);

  async function commit() {
    if (!preview?.tasks || busy) return;
    setBusy(true);
    try {
      const { batchId, added } = await api.tableCapture(input, mapping, topic, title, source);
      setInput("");
      setPreview(undefined);
      setMapping([]);
      onCommitted(batchId, added);
    } catch (e) {
      setError(e instanceof Error ? e.message : "Could not add those.");
    } finally {
      setBusy(false);
    }
  }

  return (
    <>
      <textarea
        value={input}
        spellCheck={false}
        onChange={(e) => {
          setInput(e.target.value);
          setMapping([]); // a new paste gets a fresh guess
        }}
        onKeyDown={(e) => {
          if (e.key === "Enter" && (e.metaKey || e.ctrlKey)) {
            e.preventDefault();
            void commit();
          }
        }}
        placeholder={placeholder}
        className="min-h-[140px] flex-1 resize-none bg-transparent px-6 py-4 font-mono text-base leading-relaxed text-ink outline-none placeholder:text-ink-4"
      />

      <div className="h-[clamp(160px,32vh,340px)] shrink-0 overflow-y-auto border-t border-line-soft bg-sunk px-6 py-3">
        {error && <p className="font-mono text-base text-danger">{error}</p>}

        {!error && !preview && (
          <p className="font-mono text-xs tracking-wide text-ink-4 uppercase">
            paste a table and its columns appear here
          </p>
        )}

        {preview && (
          <>
            <div className="mb-3 flex flex-wrap items-center gap-2">
              <p className="font-mono text-xs tracking-wide text-ink-4 uppercase">
                {preview.format}
                {preview.preset === "copilot" && " · copilot"} · {preview.tasks} task
                {preview.tasks === 1 ? "" : "s"}
                {preview.skipped > 0 && ` · ${preview.skipped} skipped`}
              </p>
            </div>

            {/* One control per column: what the header was read as. */}
            <div className="mb-3 flex flex-wrap gap-2">
              {preview.columns.map((c, i) => (
                <div key={c.index} className="flex items-center gap-1.5">
                  <span className="max-w-[140px] truncate font-mono text-xs text-ink-3">
                    {c.header || `column ${i + 1}`}
                  </span>
                  <span className="text-ink-4">→</span>
                  <Select
                    label={`Role for ${c.header}`}
                    value={mapping[i] ?? c.role}
                    onChange={(role) => {
                      const next = preview.columns.map((col, j) => mapping[j] ?? col.role);
                      next[i] = role;
                      setMapping(next);
                    }}
                    options={preview.roles.map((r) => ({ value: r, label: ROLE_LABELS[r] ?? r }))}
                  />
                </div>
              ))}
            </div>

            <ol className="space-y-px">
              {preview.rows.map((row) => (
                <li key={row.n} className="border-t border-line-soft py-1.5">
                  {row.task ? (
                    <div className="flex flex-wrap items-baseline gap-x-2">
                      <span className="text-md text-ink">{row.task.title}</span>
                      <span className="flex flex-wrap items-center gap-x-1.5 font-mono text-xs text-ink-3">
                        <span className="inline-flex items-center gap-1.5">
                          <TopicDot topic={row.task.topic} />
                          {row.task.topic}
                        </span>
                        {row.task.dueLabel && (
                          <span className="text-accent">· {row.task.dueLabel.toLowerCase()}</span>
                        )}
                        {row.task.assignee && <span className="text-t0">· {row.task.assignee}</span>}
                        {row.task.priority > 0 && (
                          <span className="text-danger">· {BANGS[row.task.priority]}</span>
                        )}
                        {row.task.tags.map((t) => (
                          <span key={t} className="text-t2">· #{t}</span>
                        ))}
                      </span>
                      {row.task.warning && (
                        <span className="basis-full font-mono text-xs text-danger">
                          {row.task.warning}
                        </span>
                      )}
                    </div>
                  ) : (
                    <div className="flex flex-wrap items-baseline gap-2">
                      <span className="font-mono text-base text-ink-4 line-through decoration-line">
                        {row.cells.filter(Boolean).join(" · ") || `row ${row.n}`}
                      </span>
                      <span className="font-mono text-xs text-ink-4">{row.reason}</span>
                    </div>
                  )}
                </li>
              ))}
            </ol>
          </>
        )}
      </div>

      <footer className="flex shrink-0 flex-wrap items-center gap-x-5 gap-y-2 border-t border-line-soft px-6 py-3">
        <p className="font-mono text-xs text-ink-4">
          columns are guessed from the headers — change any that read wrong
        </p>
        <button
          onClick={commit}
          disabled={!preview?.tasks || busy}
          className={clsx(
            "ml-auto rounded-md bg-ink px-3.5 py-1.5 text-base font-medium text-bg transition-opacity",
            "hover:opacity-85 disabled:opacity-35",
          )}
        >
          {preview?.tasks
            ? `add ${preview.tasks} task${preview.tasks === 1 ? "" : "s"}`
            : "add tasks"}
        </button>
      </footer>
    </>
  );
}
