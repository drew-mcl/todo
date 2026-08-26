import clsx from "clsx";

/** Every input in the app is one of these. No native chrome anywhere. */
export function Field({
  label,
  children,
  className,
}: {
  label: string;
  children: React.ReactNode;
  className?: string;
}) {
  return (
    <label className={clsx("block", className)}>
      <span className="mb-1 block font-mono text-2xs tracking-[0.14em] text-ink-4 uppercase">
        {label}
      </span>
      {children}
    </label>
  );
}

export const inputClass =
  "w-full rounded-md border border-line bg-bg px-2.5 py-1.5 text-base text-ink outline-none transition-colors placeholder:text-ink-4 focus:border-accent";

export function Segmented<T extends string | number>({
  value,
  options,
  onChange,
  className,
}: {
  value: T;
  options: { value: T; label: string; tone?: "danger" }[];
  onChange: (v: T) => void;
  className?: string;
}) {
  return (
    <div className={clsx("inline-flex gap-0.5 rounded-md border border-line bg-sunk p-0.5", className)}>
      {options.map((o) => (
        <button
          key={String(o.value)}
          type="button"
          aria-pressed={o.value === value}
          onClick={() => onChange(o.value)}
          className={clsx(
            "rounded-sm px-2.5 py-1 font-mono text-xs transition-colors",
            o.value === value
              ? "bg-raised text-ink shadow-[0_1px_2px_rgb(0_0_0/.07)]"
              : "text-ink-3 hover:text-ink",
            o.tone === "danger" && o.value === value && "text-danger",
          )}
        >
          {o.label}
        </button>
      ))}
    </div>
  );
}

export function Toggle({
  checked,
  onChange,
  label,
}: {
  checked: boolean;
  onChange: (v: boolean) => void;
  label: string;
}) {
  return (
    <button
      type="button"
      role="switch"
      aria-checked={checked}
      onClick={() => onChange(!checked)}
      className="flex w-full items-center justify-between gap-3 rounded-md px-1 py-1.5 text-left hover:bg-sunk"
    >
      <span className="font-mono text-base text-ink-2">{label}</span>
      <span
        className={clsx(
          "relative h-[18px] w-8 shrink-0 rounded-full transition-colors",
          checked ? "bg-accent" : "bg-line",
        )}
      >
        <span
          className={clsx(
            "absolute top-[2px] h-[14px] w-[14px] rounded-full bg-surface transition-[left] duration-150 ease-out-quick",
            checked ? "left-[16px]" : "left-[2px]",
          )}
        />
      </span>
    </button>
  );
}
