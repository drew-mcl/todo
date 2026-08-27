import { useQuery } from "@tanstack/react-query";
import { Dialog } from "./Dialog";
import { api } from "../api";

/**
 * The keys the capture box answers to.
 *
 * Fetched rather than written down here: the list lives in internal/vim and the
 * desktop window shows the same one, so a sheet cannot promise a key that only
 * one of them implements.
 */
export function VimSheet({ open, onClose }: { open: boolean; onClose: () => void }) {
  const keys = useQuery({ queryKey: ["keys"], queryFn: api.keys, enabled: open });

  return (
    <Dialog open={open} onClose={onClose} label="Keys in the capture box" width={620}>
      <div className="grid gap-x-10 gap-y-5 px-6 pt-1 pb-6 sm:grid-cols-2">
        {(keys.data ?? []).map((group) => (
          <section key={group.name}>
            <h3 className="mb-2 font-mono text-2xs tracking-wide text-ink-4 uppercase">
              {group.name}
            </h3>
            <dl className="space-y-1.5">
              {group.keys.map((k) => (
                <div key={k.press + k.does} className="flex items-baseline gap-3">
                  <dt className="flex shrink-0 gap-1">
                    {k.press.split(/\s+/).map((one) => (
                      <kbd
                        key={one}
                        className="rounded-xs border border-line bg-sunk px-1.5 py-0.5 font-mono text-2xs text-ink-3"
                      >
                        {one}
                      </kbd>
                    ))}
                  </dt>
                  <dd className="text-sm text-ink-3">{k.does}</dd>
                </div>
              ))}
            </dl>
          </section>
        ))}
      </div>
      <p className="border-t border-line-soft px-6 py-3 font-mono text-xs text-ink-4">
        esc stops typing · esc again files what is there · ⌘/ brings this back
      </p>
    </Dialog>
  );
}
