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
