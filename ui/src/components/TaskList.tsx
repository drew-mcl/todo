import {
  DndContext,
  PointerSensor,
  useSensor,
  useSensors,
  type DragEndEvent,
} from "@dnd-kit/core";
import { restrictToVerticalAxis } from "@dnd-kit/modifiers";
import { SortableContext, verticalListSortingStrategy } from "@dnd-kit/sortable";
import clsx from "clsx";
import { AnimatePresence, motion } from "motion/react";
import type { ListResponse, Task } from "../api";
import type { Fields } from "../lib/prefs";
import { TaskRow } from "./TaskRow";

const EMPTY: Record<string, [string, string]> = {
  today: ["nothing due today.", "press n and paste what came up on the call."],
  upcoming: ["nothing scheduled ahead.", "give a task a day with | eow or | +3d."],
  anytime: ["no undated tasks.", "everything you're carrying has a day against it."],
  delegated: ["you're not waiting on anyone.", "add @name to a line to track what someone owes you."],
  logbook: ["nothing finished yet.", "completed tasks collect here by the day you closed them."],
  all: ["the list is empty.", "press n and paste your notes."],
};

export function TaskList({
  data,
  fields,
  compact,
  cursorId,
  onToggle,
  onOpen,
  onMove,
  onFilter,
  filtered,
  onClearFilter,
}: {
  data: ListResponse;
  fields: Fields;
  compact: boolean;
  cursorId: number | null;
  onToggle: (t: Task) => void;
  onOpen: (t: Task) => void;
  onMove: (id: number, above: number, below: number) => void;
  onFilter: (kind: "topic" | "assignee" | "tag", value: string) => void;
  filtered: boolean;
  onClearFilter: () => void;
}) {
  const sensors = useSensors(useSensor(PointerSensor, { activationConstraint: { distance: 4 } }));
  const flat = data.sections.flatMap((s) => s.tasks);

  if (flat.length === 0) {
    // Clearing the day is the best thing that happens in here, and it used to
    // say the same flat sentence as a day that never had anything on it.
    const cleared = data.view === "today" && !filtered && (data.meta.doneToday ?? 0) > 0;
    const [line, hint] = filtered
      ? ["nothing matches that filter.", "clear it to see the whole list."]
      : cleared
        ? ["that is today done.", `${data.meta.doneToday} closed. nothing else is owed today.`]
        : (EMPTY[data.view] ?? EMPTY.all);

    return (
      <motion.div
        initial={{ opacity: 0, y: 4 }}
        animate={{ opacity: 1, y: 0 }}
        transition={{ duration: 0.25, ease: [0.22, 1, 0.36, 1] }}
        className="pt-16"
      >
        <p className={clsx("text-lg", cleared ? "text-ink" : "text-ink-2")}>
          {cleared && <span className="mr-2 text-accent">✓</span>}
          {line}
        </p>
        <p className="mt-1.5 font-mono text-base text-ink-4">
          {hint}{" "}
          {filtered && (
            <button onClick={onClearFilter} className="text-accent underline underline-offset-2">
              clear
            </button>
          )}
        </p>
      </motion.div>
    );
  }

  function onDragEnd(e: DragEndEvent) {
    const { active, over } = e;
    if (!over || active.id === over.id) return;
    const ids = flat.map((t) => t.id);
    const from = ids.indexOf(Number(active.id));
    const to = ids.indexOf(Number(over.id));
    if (from < 0 || to < 0) return;

    const reordered = [...ids];
    reordered.splice(to, 0, reordered.splice(from, 1)[0]);
    const at = reordered.indexOf(Number(active.id));
    onMove(Number(active.id), reordered[at - 1] ?? 0, reordered[at + 1] ?? 0);
  }

  return (
    <DndContext
      sensors={sensors}
      modifiers={[restrictToVerticalAxis]}
      onDragEnd={data.canDrag ? onDragEnd : undefined}
    >
      <SortableContext items={flat.map((t) => t.id)} strategy={verticalListSortingStrategy}>
        {data.sections.map((section, i) => (
          <section key={section.label || i} className={i > 0 ? "mt-6" : undefined}>
            {section.label && (
              <h2 className="pb-1.5 font-mono text-2xs tracking-[0.14em] text-ink-4 uppercase">
                {section.label}
              </h2>
            )}
            <ul>
              <AnimatePresence initial={false}>
              {section.tasks.map((task) => (
                <TaskRow
                  key={task.id}
                  task={task}
                  fields={fields}
                  compact={compact}
                  cursor={cursorId === task.id}
                  draggable={data.canDrag}
                  onToggle={() => onToggle(task)}
                  onOpen={() => onOpen(task)}
                  onFilter={onFilter}
                />
              ))}
              </AnimatePresence>
            </ul>
          </section>
        ))}
      </SortableContext>
    </DndContext>
  );
}
