import { useEffect, useRef, useState } from "react";
import clsx from "clsx";
import { AnimatePresence, motion } from "motion/react";

/**
 * A dropdown that belongs to this app rather than to the operating system.
 * The native control cannot be styled past its trigger, and its popup was the
 * loudest thing on the page.
 */
export function Select<T extends string>({
  value,
  options,
  onChange,
  label,
}: {
  value: T;
  options: { value: T; label: string }[];
  onChange: (v: T) => void;
  label: string;
}) {
  const [open, setOpen] = useState(false);
  const ref = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!open) return;
    const onDown = (e: MouseEvent) => {
      if (!ref.current?.contains(e.target as Node)) setOpen(false);
    };
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") {
        e.stopPropagation();
        setOpen(false);
      }
    };
    addEventListener("mousedown", onDown);
    addEventListener("keydown", onKey, true);
    return () => {
      removeEventListener("mousedown", onDown);
      removeEventListener("keydown", onKey, true);
    };
  }, [open]);

  const current = options.find((o) => o.value === value);

  return (
    <div ref={ref} className="relative">
      <button
        type="button"
        aria-haspopup="listbox"
        aria-expanded={open}
        aria-label={label}
        onClick={() => setOpen((o) => !o)}
        className={clsx(
          "flex items-center gap-2 rounded-md border px-2.5 py-1.5 font-mono text-base transition-colors",
          open ? "border-ink-4 text-ink" : "border-line text-ink-2 hover:border-ink-4",
        )}
      >
        <span>{current?.label ?? value}</span>
        <svg
          viewBox="0 0 12 12"
          className={clsx("h-2.5 w-2.5 transition-transform", open && "rotate-180")}
          fill="none"
          stroke="currentColor"
          strokeWidth="1.6"
          strokeLinecap="round"
          strokeLinejoin="round"
        >
          <path d="M2.5 4.5L6 8l3.5-3.5" />
        </svg>
      </button>

      <AnimatePresence>
        {open && (
          <motion.ul
            role="listbox"
            initial={{ opacity: 0, y: -4 }}
            animate={{ opacity: 1, y: 0 }}
            exit={{ opacity: 0, y: -4 }}
            transition={{ duration: 0.12 }}
            className="absolute right-0 z-30 mt-1 min-w-full overflow-hidden rounded-md border border-line bg-raised p-1 shadow-[var(--shadow-panel)]"
          >
            {options.map((o) => (
              <li key={o.value}>
                <button
                  role="option"
                  aria-selected={o.value === value}
                  onClick={() => {
                    onChange(o.value);
                    setOpen(false);
                  }}
                  className={clsx(
                    "flex w-full items-center gap-2 rounded-sm px-2 py-1 text-left font-mono text-base whitespace-nowrap",
                    o.value === value ? "bg-sunk text-ink" : "text-ink-2 hover:bg-sunk/70",
                  )}
                >
                  <span className={clsx("w-2.5", o.value === value ? "text-accent" : "opacity-0")}>
                    ✓
                  </span>
                  {o.label}
                </button>
              </li>
            ))}
          </motion.ul>
        )}
      </AnimatePresence>
    </div>
  );
}
