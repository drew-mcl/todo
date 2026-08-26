import { Command } from "cmdk";
import type { Filters, Meta } from "../api";
import { Dialog } from "./Dialog";
import { TopicDot } from "./TaskRow";

const VIEWS = ["today", "upcoming", "anytime", "delegated", "logbook", "all"];

export function Palette({
  open,
  onClose,
  meta,
  onGo,
  onCapture,
  onSettings,
  onShortcuts,
}: {
  open: boolean;
  onClose: () => void;
  meta?: Meta;
  onGo: (kind: "list" | "week", f: Filters) => void;
  onCapture: () => void;
  onSettings: () => void;
  onShortcuts: () => void;
}) {
  const run = (fn: () => void) => () => {
    onClose();
    fn();
  };

  return (
    <Dialog open={open} onClose={onClose} label="Command palette" width={560}>
      <Command loop className="flex max-h-[70vh] flex-col">
        <Command.Input
          autoFocus
          placeholder="Jump to a view, topic or person…"
          className="w-full border-b border-line-soft bg-transparent px-4 py-3 font-mono text-md text-ink outline-none placeholder:text-ink-4"
        />
        <Command.List className="min-h-0 flex-1 overflow-y-auto p-1.5">
          <Command.Empty className="px-3 py-6 text-center font-mono text-xs text-ink-4">
            Nothing matches.
          </Command.Empty>

          <Group heading="go">
            <Item onSelect={run(() => onGo("week", {}))} label="week" hint="w" />
            {VIEWS.map((v) => (
              <Item key={v} onSelect={run(() => onGo("list", { view: v }))} label={v} />
            ))}
          </Group>

          <Group heading="do">
            <Item onSelect={run(onCapture)} label="capture notes" hint="n" />
            <Item onSelect={run(onShortcuts)} label="keyboard shortcuts" hint="?" />
            <Item onSelect={run(onSettings)} label="settings" />
          </Group>

          {meta && meta.topics.length > 0 && (
            <Group heading="topics">
              {meta.topics.map((t) => (
                <Item
                  key={t.name}
                  onSelect={run(() => onGo("list", { view: "all", topic: t.name }))}
                  label={t.name}
                  dot={t.name}
                  count={t.count}
                />
              ))}
            </Group>
          )}

          {meta && meta.people.length > 0 && (
            <Group heading="people">
              {meta.people.map((p) => (
                <Item
                  key={p.name}
                  onSelect={run(() => onGo("list", { view: "all", assignee: p.name }))}
                  label={p.name}
                  count={p.count}
                />
              ))}
            </Group>
          )}

          {meta && meta.tags.length > 0 && (
            <Group heading="tags">
              {meta.tags.map((t) => (
                <Item
                  key={t.name}
                  onSelect={run(() => onGo("list", { view: "all", tag: t.name }))}
                  label={`#${t.name}`}
                  count={t.count}
                />
              ))}
            </Group>
          )}
        </Command.List>
      </Command>
    </Dialog>
  );
}

function Group({ heading, children }: { heading: string; children: React.ReactNode }) {
  return (
    <Command.Group
      heading={heading}
      className="[&_[cmdk-group-heading]]:px-2.5 [&_[cmdk-group-heading]]:pt-2 [&_[cmdk-group-heading]]:pb-1 [&_[cmdk-group-heading]]:font-mono [&_[cmdk-group-heading]]:text-2xs [&_[cmdk-group-heading]]:tracking-[0.14em] [&_[cmdk-group-heading]]:text-ink-4 [&_[cmdk-group-heading]]:uppercase"
    >
      {children}
    </Command.Group>
  );
}

function Item({
  label,
  onSelect,
  hint,
  count,
  dot,
}: {
  label: string;
  onSelect: () => void;
  hint?: string;
  count?: number;
  dot?: string;
}) {
  return (
    <Command.Item
      value={label}
      onSelect={onSelect}
      className="flex cursor-pointer items-center gap-2 rounded-md px-2.5 py-1.5 font-mono text-base text-ink-2 data-[selected=true]:bg-sunk data-[selected=true]:text-ink"
    >
      {dot && <TopicDot topic={dot} />}
      <span className="flex-1 truncate">{label}</span>
      {count !== undefined && <span className="text-xs tabular-nums text-ink-4">{count}</span>}
      {hint && <kbd className="rounded-xs border border-line px-1 text-2xs text-ink-4">{hint}</kbd>}
    </Command.Item>
  );
}
