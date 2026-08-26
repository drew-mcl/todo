import clsx from "clsx";
import { useSortable } from "@dnd-kit/sortable";
import { CSS } from "@dnd-kit/utilities";
import type { Task } from "../api";
import type { Fields } from "../lib/prefs";
import { hueVar } from "../lib/topic";
import { useHue } from "../lib/hues";
import { Check } from "./Check";

const BANGS = ["", "!", "!!", "!!!"];

/** The dot that carries a topic's colour. */
export function TopicDot({ topic, className }: { topic: string; className?: string }) {
  const hue = useHue(topic);
  return (
    <span
      aria-hidden
      className={clsx("inline-block h-[6px] w-[6px] shrink-0 rounded-full", className)}
      style={{ background: hueVar(hue) }}
    />
  );
}

/** A middot separator that never strands itself at the end of a line. */
function Dot() {
  return <span className="text-ink-4 select-none">·</span>;
}

export function TaskMeta({
  task,
  fields,
  onFilter,
}: {
  task: Task;
  fields: Fields;
  onFilter?: (kind: "topic" | "assignee" | "tag", value: string) => void;
}) {
  const parts: React.ReactNode[] = [];

  if (fields.topic) {
    parts.push(
      <button
        key="topic"
        onClick={(e) => {
          e.stopPropagation();
          onFilter?.("topic", task.topic);
        }}
        className="inline-flex items-center gap-1.5 hover:text-ink"
      >
        <TopicDot topic={task.topic} />
        {task.topic}
      </button>,
    );
  }
  if (fields.due && task.dueLabel) {
    parts.push(
      <span key="due" className={clsx(task.overdue && "font-medium text-danger")}>
        {task.dueLabel.toLowerCase()}
      </span>,
    );
  }
  if (fields.assignee && task.assignee) {
    parts.push(
      <button
        key="who"
        onClick={(e) => {
          e.stopPropagation();
          onFilter?.("assignee", task.assignee);
        }}
        className="hover:text-ink"
      >
        {task.assignee}
      </button>,
    );
  }
  if (fields.tags) {
    for (const tag of task.tags) {
      parts.push(
        <button
          key={`tag-${tag}`}
          onClick={(e) => {
            e.stopPropagation();
            onFilter?.("tag", tag);
          }}
          className="hover:text-ink"
        >
          #{tag}
        </button>,
      );
    }
  }
  if (fields.captured && task.capturedWhen) {
    parts.push(
      <span key="captured" className="text-ink-4">
        captured {task.capturedWhen.toLowerCase()}
      </span>,
    );
  }

  if (parts.length === 0) return null;

  return (
    <div className="mt-0.5 flex flex-wrap items-center gap-x-1.5 font-mono text-xs text-ink-3">
      {parts.map((p, i) => (
        <span key={i} className="inline-flex items-center gap-1.5">
          {i > 0 && <Dot />}
          {p}
        </span>
      ))}
    </div>
  );
}

export function TaskRow({
  task,
  fields,
  compact,
  cursor,
  draggable,
  onToggle,
  onOpen,
  onFilter,
}: {
  task: Task;
  fields: Fields;
  compact: boolean;
  cursor: boolean;
  draggable: boolean;
  onToggle: () => void;
  onOpen: () => void;
  onFilter?: (kind: "topic" | "assignee" | "tag", value: string) => void;
}) {
  const { attributes, listeners, setNodeRef, transform, transition, isDragging } =
    useSortable({ id: task.id, disabled: !draggable });

  return (
    <li
      ref={setNodeRef}
      style={{ transform: CSS.Translate.toString(transform), transition }}
      data-task-id={task.id}
      className={clsx(
        "group/row relative flex items-start gap-2.5 border-b border-line-soft pr-2",
        compact ? "py-1.5" : "py-2.5",
        isDragging && "z-10 opacity-40",
        cursor && "bg-sunk",
      )}
    >
      {/* The keyboard cursor is a mark in the gutter, so it never competes with
          hover for the row's background. */}
      <span
        aria-hidden
        className={clsx(
          "absolute top-1.5 bottom-1.5 -left-3 w-[2px] rounded-full transition-opacity duration-150",
          cursor ? "bg-accent opacity-100" : "opacity-0",
        )}
      />

      <span
        {...attributes}
        {...listeners}
        aria-hidden
        className={clsx(
          "mt-[3px] h-4 w-3 shrink-0 rounded-xs bg-[radial-gradient(circle,var(--color-ink-4)_1px,transparent_1.2px)] bg-[length:4px_4px] opacity-0 transition-opacity",
          draggable && "cursor-grab group-hover/row:opacity-60 active:cursor-grabbing",
        )}
      />

      <span className="mt-[3px]">
        <Check done={task.done} onToggle={onToggle} label={`Complete ${task.title}`} />
      </span>

      <div className="min-w-0 flex-1">
        <button
          onClick={onOpen}
          className={clsx(
            "block w-full text-left text-md leading-snug",
            task.done ? "text-ink-4 line-through decoration-1" : "text-ink",
          )}
        >
          {task.title}
        </button>
        <TaskMeta task={task} fields={fields} onFilter={onFilter} />
        {fields.note && task.note && (
          <div className="mt-1 border-l border-line pl-2.5 text-sm text-ink-3">
            {task.note.split("\n").map((line, i) => (
              <p key={i}>{line}</p>
            ))}
          </div>
        )}
      </div>

      {fields.priority && task.priority > 0 && (
        <span
          title={["", "high", "urgent", "critical"][task.priority]}
          className="mt-[3px] shrink-0 font-mono text-sm leading-none font-semibold tracking-wide text-danger"
        >
          {BANGS[task.priority]}
        </span>
      )}
    </li>
  );
}
