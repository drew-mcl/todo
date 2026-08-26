import clsx from "clsx";

/**
 * The checkbox. Filled with ink rather than a colour: completion is not an
 * alarm, and the palette is saved for things that need attention.
 */
export function Check({
  done,
  onToggle,
  label,
  size = 15,
}: {
  done: boolean;
  onToggle: () => void;
  label: string;
  size?: number;
}) {
  return (
    <button
      type="button"
      role="checkbox"
      aria-checked={done}
      aria-label={label}
      onClick={(e) => {
        e.stopPropagation();
        onToggle();
      }}
      style={{ width: size, height: size }}
      className={clsx(
        "grid shrink-0 place-items-center rounded-sm border transition-colors duration-150",
        done
          ? "border-ink bg-ink"
          : "border-ink-4 hover:border-ink-2 active:scale-95",
      )}
    >
      <svg viewBox="0 0 16 16" className="h-[10px] w-[10px] overflow-visible">
        <path
          d="M3 8.4l3.2 3.1L13 4.6"
          fill="none"
          strokeWidth={2.4}
          strokeLinecap="round"
          strokeLinejoin="round"
          className={clsx(
            "transition-[stroke-dashoffset] duration-200 ease-out-quick",
            done ? "stroke-bg" : "stroke-ink-4 group-hover/row:stroke-ink-4",
          )}
          style={{ strokeDasharray: 16, strokeDashoffset: done ? 0 : 16 }}
        />
      </svg>
    </button>
  );
}
