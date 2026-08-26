import type { Fields, Prefs, Theme, Density } from "../lib/prefs";
import { Dialog } from "./Dialog";
import { Segmented, Toggle } from "./Field";

const FIELD_LABELS: { key: keyof Fields; label: string }[] = [
  { key: "topic", label: "topic" },
  { key: "due", label: "due date" },
  { key: "assignee", label: "assignee" },
  { key: "tags", label: "tags" },
  { key: "priority", label: "priority" },
  { key: "note", label: "notes" },
  { key: "captured", label: "when it was captured" },
];

export function Settings({
  open,
  onClose,
  prefs,
  update,
  setField,
}: {
  open: boolean;
  onClose: () => void;
  prefs: Prefs;
  update: (p: Partial<Prefs>) => void;
  setField: (k: keyof Fields, v: boolean) => void;
}) {
  return (
    <Dialog open={open} onClose={onClose} label="Settings" width={480}>
      <header className="flex items-center justify-between border-b border-line-soft px-4 py-3">
        <h2 className="text-base text-ink">settings</h2>
        <button
          onClick={onClose}
          className="grid h-6 w-6 place-items-center rounded-md text-ink-4 hover:bg-sunk hover:text-ink"
          aria-label="Close"
        >
          ✕
        </button>
      </header>

      <div className="max-h-[60vh] space-y-5 overflow-y-auto p-4">
        <section>
          <h3 className="mb-2 font-mono text-2xs tracking-[0.14em] text-ink-4 uppercase">
            appearance
          </h3>
          <Segmented<Theme>
            value={prefs.theme}
            onChange={(theme) => update({ theme })}
            options={[
              { value: "system", label: "auto" },
              { value: "light", label: "light" },
              { value: "dark", label: "dark" },
            ]}
          />
        </section>

        <section>
          <h3 className="mb-2 font-mono text-2xs tracking-[0.14em] text-ink-4 uppercase">
            density
          </h3>
          <Segmented<Density>
            value={prefs.density}
            onChange={(density) => update({ density })}
            options={[
              { value: "comfortable", label: "comfortable" },
              { value: "compact", label: "compact" },
            ]}
          />
        </section>

        <section>
          <h3 className="mb-1 font-mono text-2xs tracking-[0.14em] text-ink-4 uppercase">
            your data
          </h3>
          <p className="mb-2 text-sm text-ink-4">
            Everything, as plain text: every capture, what was said and when.
          </p>
          <a
            href="/api/export"
            className="inline-block rounded-md border border-line px-3 py-1.5 font-mono text-base text-ink-2 hover:border-ink-4 hover:text-ink"
          >
            export everything
          </a>
        </section>

        <section>
          <h3 className="mb-1 font-mono text-2xs tracking-[0.14em] text-ink-4 uppercase">
            show on each task
          </h3>
          <p className="mb-2 text-sm text-ink-4">
            Turn off what you don't need. The list is yours to quieten.
          </p>
          {FIELD_LABELS.map((f) => (
            <Toggle
              key={f.key}
              label={f.label}
              checked={prefs.fields[f.key]}
              onChange={(v) => setField(f.key, v)}
            />
          ))}
        </section>
      </div>
    </Dialog>
  );
}
