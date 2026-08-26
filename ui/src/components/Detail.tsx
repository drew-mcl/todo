import { useEffect, useState } from "react";
import { AnimatePresence, motion } from "motion/react";
import { api, type Task } from "../api";
import { Field, Segmented, inputClass } from "./Field";
import { TopicDot } from "./TaskRow";

export function Detail({
  task,
  onClose,
  onSaved,
  onDeleted,
}: {
  task?: Task;
  onClose: () => void;
  onSaved: (t: Task) => void;
  onDeleted: (id: number) => void;
}) {
  const [draft, setDraft] = useState<Task | undefined>(task);
  const [dueText, setDueText] = useState("");
  const [tagText, setTagText] = useState("");
  const [error, setError] = useState<string>();

  useEffect(() => {
    setDraft(task);
    setDueText(task?.dueLabel ?? "");
    setTagText(task?.tags.join(", ") ?? "");
    setError(undefined);
  }, [task]);

  async function save() {
    if (!draft) return;
    try {
      const saved = await api.update(draft.id, {
        title: draft.title,
        topic: draft.topic,
        note: draft.note,
        assignee: draft.assignee,
        priority: draft.priority,
        due: dueText,
        tags: tagText.split(/[,\s#]+/).filter(Boolean),
      });
      onSaved(saved);
      onClose();
    } catch (e) {
      setError(e instanceof Error ? e.message : "Could not save that.");
    }
  }

  return (
    <AnimatePresence>
      {draft && (
        <motion.aside
          initial={{ x: 24, opacity: 0 }}
          animate={{ x: 0, opacity: 1 }}
          exit={{ x: 24, opacity: 0 }}
          transition={{ duration: 0.18, ease: [0.22, 1, 0.36, 1] }}
          className="absolute inset-y-0 right-0 z-30 flex w-[380px] flex-col border-l border-line bg-surface shadow-[var(--shadow-panel)]"
          role="dialog"
          aria-label="Task detail"
        >
          <header className="flex items-center justify-between border-b border-line-soft px-4 py-2.5">
            <span className="flex items-center gap-2 font-mono text-xs text-ink-3">
              <TopicDot topic={draft.topic} />
              {draft.topic}
            </span>
            <button
              onClick={onClose}
              className="grid h-6 w-6 place-items-center rounded-md text-ink-4 hover:bg-sunk hover:text-ink"
              aria-label="Close"
            >
              ✕
            </button>
          </header>

          <div className="min-h-0 flex-1 space-y-3 overflow-y-auto p-4">
            <Field label="task">
              <textarea
                rows={2}
                value={draft.title}
                onChange={(e) => setDraft({ ...draft, title: e.target.value })}
                className={`${inputClass} resize-none text-md`}
              />
            </Field>

            <div className="grid grid-cols-2 gap-2.5">
              <Field label="topic">
                <input
                  value={draft.topic}
                  onChange={(e) => setDraft({ ...draft, topic: e.target.value })}
                  className={inputClass}
                />
              </Field>
              <Field label="due">
                <input
                  value={dueText}
                  placeholder="today, eow, 25/12"
                  onChange={(e) => setDueText(e.target.value)}
                  className={inputClass}
                />
              </Field>
            </div>

            <Field label="waiting on">
              <input
                value={draft.assignee}
                placeholder="nobody"
                onChange={(e) => setDraft({ ...draft, assignee: e.target.value })}
                className={inputClass}
              />
            </Field>

            <Field label="priority">
              <Segmented
                value={draft.priority}
                onChange={(priority) => setDraft({ ...draft, priority })}
                options={[
                  { value: 0, label: "normal" },
                  { value: 1, label: "!" },
                  { value: 2, label: "!!" },
                  { value: 3, label: "!!!", tone: "danger" },
                ]}
              />
            </Field>

            <Field label="tags">
              <input
                value={tagText}
                placeholder="compliance, q3"
                onChange={(e) => setTagText(e.target.value)}
                className={inputClass}
              />
            </Field>

            <Field label="notes">
              <textarea
                rows={6}
                value={draft.note}
                placeholder="Context, links, what was actually said."
                onChange={(e) => setDraft({ ...draft, note: e.target.value })}
                className={`${inputClass} resize-y text-base leading-relaxed`}
              />
            </Field>

            {error && <p className="font-mono text-xs text-danger">{error}</p>}

            {/* Provenance: this list is also a record of what was committed to,
                and when it was said. */}
            <div className="space-y-1.5 border-t border-line-soft pt-3">
              <p className="font-mono text-2xs tracking-[0.14em] text-ink-4 uppercase">
                captured {draft.capturedWhen.toLowerCase()}
              </p>
              <code className="block font-mono text-xs break-words text-ink-3">{draft.raw}</code>
            </div>
          </div>

          <footer className="flex items-center gap-2 border-t border-line-soft px-4 py-3">
            <button
              onClick={save}
              className="rounded-md bg-ink px-3.5 py-1.5 text-base font-medium text-bg hover:opacity-85"
            >
              save
            </button>
            <button
              onClick={async () => {
                await api.remove(draft.id);
                onDeleted(draft.id);
                onClose();
              }}
              className="ml-auto rounded-md border border-line px-3 py-1.5 text-base text-danger hover:border-danger"
            >
              delete
            </button>
          </footer>
        </motion.aside>
      )}
    </AnimatePresence>
  );
}
