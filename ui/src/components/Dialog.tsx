import { useEffect, useRef } from "react";
import { AnimatePresence, motion } from "motion/react";

export function Dialog({
  open,
  onClose,
  label,
  width = 640,
  children,
}: {
  open: boolean;
  onClose: () => void;
  label: string;
  width?: number;
  children: React.ReactNode;
}) {
  const ref = useRef<HTMLDivElement>(null);

  // Keep Tab inside the dialog and hand focus back where it came from. Without
  // this, tabbing walks out into the list behind and stays there.
  useEffect(() => {
    if (!open) return;
    const returnTo = document.activeElement as HTMLElement | null;

    function onKey(e: KeyboardEvent) {
      if (e.key !== "Tab" || !ref.current) return;
      const focusable = ref.current.querySelectorAll<HTMLElement>(
        'a[href], button:not([disabled]), input, textarea, select, [tabindex]:not([tabindex="-1"])',
      );
      if (focusable.length === 0) return;
      const first = focusable[0];
      const last = focusable[focusable.length - 1];
      if (e.shiftKey && document.activeElement === first) {
        e.preventDefault();
        last.focus();
      } else if (!e.shiftKey && document.activeElement === last) {
        e.preventDefault();
        first.focus();
      }
    }

    document.addEventListener("keydown", onKey);
    return () => {
      document.removeEventListener("keydown", onKey);
      returnTo?.focus?.();
    };
  }, [open]);

  return (
    <AnimatePresence>
      {open && (
        <motion.div
          initial={{ opacity: 0 }}
          animate={{ opacity: 1 }}
          exit={{ opacity: 0 }}
          transition={{ duration: 0.12 }}
          className="fixed inset-0 z-50 grid place-items-start justify-items-center bg-ink/30 p-4 pt-[12vh] backdrop-blur-[2px]"
          onMouseDown={(e) => e.target === e.currentTarget && onClose()}
        >
          <motion.div
            initial={{ y: -8, scale: 0.99 }}
            animate={{ y: 0, scale: 1 }}
            exit={{ y: -8, scale: 0.99 }}
            transition={{ duration: 0.16, ease: [0.22, 1, 0.36, 1] }}
            ref={ref}
            role="dialog"
            aria-modal
            aria-label={label}
            style={{ width: `min(${width}px, 100%)` }}
            className="max-h-[76vh] overflow-hidden rounded-xl border border-line bg-surface shadow-[var(--shadow-pop)]"
          >
            {children}
          </motion.div>
        </motion.div>
      )}
    </AnimatePresence>
  );
}
