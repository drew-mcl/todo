import clsx from "clsx";
import {
  DndContext,
  DragOverlay,
  PointerSensor,
  useDraggable,
  useDroppable,
  useSensor,
  useSensors,
  type DragEndEvent,
  type DragStartEvent,
} from "@dnd-kit/core";
import { useState } from "react";
import type { Task, WeekResponse } from "../api";
import type { Fields } from "../lib/prefs";
import { Check } from "./Check";
import { TopicDot } from "./TaskRow";

const BANGS = ["", "!", "!!", "!!!"];

function Card({
  task,
  fields,
  onToggle,
  onOpen,
  overlay,
}: {
  task: Task;
  fields: Fields;
  onToggle?: () => void;
  onOpen?: () => void;
  overlay?: boolean;
}) {
  const { attributes, listeners, setNodeRef, isDragging } = useDraggable({
    id: task.id,
    disabled: overlay,
  });

  return (
    <div
      ref={setNodeRef}
      {...listeners}
      {...attributes}
      onClick={onOpen}
      className={clsx(
        "flex cursor-grab items-start gap-2 rounded-md border bg-bg px-2 py-1.5 text-left transition-colors active:cursor-grabbing",
        overlay
          ? "border-ink-4 shadow-[var(--shadow-pop)]"
          : "border-line-soft hover:border-ink-4",
        isDragging && "opacity-30",
      )}
    >
      {onToggle && (
        <span className="mt-[2px]">
          <Check done={task.done} onToggle={onToggle} label={`Complete ${task.title}`} size={13} />
        </span>
      )}
      <div className="min-w-0 flex-1">
        <p className="text-sm leading-snug text-ink">{task.title}</p>
        <p className="mt-0.5 flex flex-wrap items-center gap-x-1.5 font-mono text-2xs text-ink-3">
          {fields.topic && (
            <span className="inline-flex items-center gap-1">
              <TopicDot topic={task.topic} />
              {task.topic}
            </span>
          )}
          {fields.assignee && task.assignee && <span>· {task.assignee}</span>}
          {fields.due && task.overdue && (
            <span className="text-danger">· {task.dueLabel.toLowerCase()}</span>
          )}
        </p>
      </div>
      {fields.priority && task.priority > 0 && (
        <span className="font-mono text-2xs font-semibold text-danger">{BANGS[task.priority]}</span>
      )}
    </div>
  );
}

function Drop({
  id,
  date,
  children,
  className,
}: {
  id: string;
  date: string;
  children: React.ReactNode;
  className?: string;
}) {
  const { setNodeRef, isOver } = useDroppable({ id, data: { date } });
  return (
    <div
      ref={setNodeRef}
      className={clsx(
        "flex-1 space-y-1 p-1.5 transition-colors",
        isOver && "bg-accent-soft",
        className,
      )}
    >
      {children}
    </div>
  );
}

function Tray({
  title,
  tasks,
  empty,
  fields,
  onToggle,
  onOpen,
}: {
  title: string;
  tasks: Task[];
  empty: string;
  fields: Fields;
  onToggle: (t: Task) => void;
  onOpen: (t: Task) => void;
}) {
  return (
    <section className="flex min-h-[92px] flex-col rounded-lg border border-line bg-surface">
      <header className="flex items-baseline gap-2 border-b border-line-soft px-3 py-2">
        <h2 className="font-mono text-2xs tracking-[0.14em] text-ink-3 uppercase">{title}</h2>
        <span className="font-mono text-2xs tabular-nums text-ink-4">{tasks.length}</span>
      </header>
      <Drop id={`tray-${title}`} date="" className="flex flex-row flex-wrap content-start gap-1.5 rounded-b-lg">
        {tasks.length === 0 ? (
          <p className="p-1 font-mono text-xs text-ink-4">{empty}</p>
        ) : (
          tasks.map((t) => (
            <div key={t.id} className="max-w-[300px] min-w-[200px] flex-1">
              <Card task={t} fields={fields} onToggle={() => onToggle(t)} onOpen={() => onOpen(t)} />
            </div>
          ))
        )}
      </Drop>
    </section>
  );
}

export function Week({
  data,
  fields,
  onSchedule,
  onToggle,
  onOpen,
  onNavigate,
}: {
  data: WeekResponse;
  fields: Fields;
  onSchedule: (id: number, date: string) => void;
  onToggle: (t: Task) => void;
  onOpen: (t: Task) => void;
  onNavigate: (start: string) => void;
}) {
  const [dragging, setDragging] = useState<Task | null>(null);
  const sensors = useSensors(useSensor(PointerSensor, { activationConstraint: { distance: 4 } }));

  const all = [...data.days.flatMap((d) => d.tasks), ...data.overdue, ...data.unscheduled];

  function onDragStart(e: DragStartEvent) {
    setDragging(all.find((t) => t.id === e.active.id) ?? null);
  }

  function onDragEnd(e: DragEndEvent) {
    setDragging(null);
    if (!e.over) return;
    const date = (e.over.data.current as { date?: string } | undefined)?.date ?? "";
    onSchedule(Number(e.active.id), date);
  }

  return (
    <DndContext sensors={sensors} onDragStart={onDragStart} onDragEnd={onDragEnd}>
      <div className="flex flex-wrap items-center gap-3 pb-3">
        <div className="flex items-center gap-1">
          {[
            { label: "←", to: data.prev, title: "Previous week" },
            { label: "this week", to: "", title: "This week" },
            { label: "→", to: data.next, title: "Next week" },
          ].map((b) => (
            <button
              key={b.label}
              title={b.title}
              onClick={() => onNavigate(b.to)}
              className="rounded-md border border-line px-2.5 py-1 font-mono text-xs text-ink-3 hover:border-ink-4 hover:text-ink"
            >
              {b.label}
            </button>
          ))}
        </div>
        <p className="font-mono text-base text-ink">{data.range}</p>
        <p className="ml-auto font-mono text-xs text-ink-4">drag onto a day to schedule</p>
      </div>

      <div className="grid grid-cols-2 gap-2 md:grid-cols-4 xl:grid-cols-7">
        {data.days.map((d) => (
          <section
            key={d.date}
            className={clsx(
              "flex min-h-[190px] flex-col rounded-lg border bg-surface",
              d.isToday ? "border-accent/55" : "border-line",
              d.isPast && "bg-sunk",
            )}
          >
            <header
              className={clsx(
                "flex items-baseline justify-between gap-1 border-b border-line-soft px-2.5 py-2",
                d.isToday && "bg-accent-soft",
              )}
            >
              <span
                className={clsx(
                  "font-mono text-2xs tracking-[0.12em] uppercase",
                  d.isToday ? "text-accent" : "text-ink-3",
                )}
              >
                {d.weekday}
              </span>
              <span className="font-mono text-2xs tabular-nums text-ink-4">{d.label}</span>
            </header>
            <Drop id={d.date} date={d.date} className="rounded-b-lg">
              {d.tasks.map((t) => (
                <Card
                  key={t.id}
                  task={t}
                  fields={fields}
                  onToggle={() => onToggle(t)}
                  onOpen={() => onOpen(t)}
                />
              ))}
            </Drop>
          </section>
        ))}
      </div>

      <div className="mt-3 grid gap-2 lg:grid-cols-2">
        <Tray
          title="overdue"
          tasks={data.overdue}
          empty="nothing late."
          fields={fields}
          onToggle={onToggle}
          onOpen={onOpen}
        />
        <Tray
          title="unscheduled"
          tasks={data.unscheduled}
          empty="everything has a day."
          fields={fields}
          onToggle={onToggle}
          onOpen={onOpen}
        />
      </div>

      <DragOverlay dropAnimation={null}>
        {dragging && (
          <div className="w-[240px]">
            <Card task={dragging} fields={fields} overlay />
          </div>
        )}
      </DragOverlay>
    </DndContext>
  );
}
