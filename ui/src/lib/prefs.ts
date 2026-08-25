// Per-browser preferences. Every access is guarded: a private window can throw
// on read as well as write.

import { useCallback, useEffect, useState } from "react";

export type Theme = "system" | "light" | "dark";
export type Density = "comfortable" | "compact";

/** Which fields a task row shows. The list is yours to quieten. */
export type Fields = {
  topic: boolean;
  due: boolean;
  assignee: boolean;
  tags: boolean;
  priority: boolean;
  note: boolean;
  captured: boolean;
};

export const DEFAULT_FIELDS: Fields = {
  topic: true,
  due: true,
  assignee: true,
  tags: true,
  priority: true,
  note: true,
  captured: false,
};

export type Prefs = {
  theme: Theme;
  density: Density;
  fields: Fields;
  sidebar: boolean;
};

const DEFAULTS: Prefs = {
  theme: "system",
  density: "comfortable",
  fields: DEFAULT_FIELDS,
  sidebar: true,
};

const KEY = "todo.prefs";

function read(): Prefs {
  try {
    const raw = localStorage.getItem(KEY);
    if (!raw) return DEFAULTS;
    const parsed = JSON.parse(raw);
    return {
      ...DEFAULTS,
      ...parsed,
      fields: { ...DEFAULT_FIELDS, ...(parsed.fields ?? {}) },
    };
  } catch {
    return DEFAULTS;
  }
}

function write(p: Prefs) {
  try {
    localStorage.setItem(KEY, JSON.stringify(p));
    // Kept in step with the pre-paint script in index.html.
    localStorage.setItem("todo.theme", p.theme);
  } catch {
    // The choice simply will not survive a reload.
  }
}

export function applyTheme(theme: Theme) {
  const resolved =
    theme === "system"
      ? matchMedia("(prefers-color-scheme: dark)").matches
        ? "dark"
        : "light"
      : theme;
  document.documentElement.dataset.theme = resolved;
}

export function usePrefs() {
  const [prefs, setPrefs] = useState<Prefs>(read);

  useEffect(() => {
    applyTheme(prefs.theme);
    write(prefs);
  }, [prefs]);

  // Following the system means following it as it changes.
  useEffect(() => {
    if (prefs.theme !== "system") return;
    const mq = matchMedia("(prefers-color-scheme: dark)");
    const onChange = () => applyTheme("system");
    mq.addEventListener("change", onChange);
    return () => mq.removeEventListener("change", onChange);
  }, [prefs.theme]);

  const update = useCallback(
    (patch: Partial<Prefs>) => setPrefs((p) => ({ ...p, ...patch })),
    [],
  );
  const setField = useCallback(
    (key: keyof Fields, on: boolean) =>
      setPrefs((p) => ({ ...p, fields: { ...p.fields, [key]: on } })),
    [],
  );

  return { prefs, update, setField };
}

/** A draft is kept as you type, so closing capture mid-call costs nothing. */
export const draftStore = {
  read(): string {
    try {
      return localStorage.getItem("todo.draft") ?? "";
    } catch {
      return "";
    }
  },
  write(v: string) {
    try {
      if (v.trim()) localStorage.setItem("todo.draft", v);
      else localStorage.removeItem("todo.draft");
    } catch {
      // Nothing to do; the draft just will not survive a reload.
    }
  },
  clear() {
    try {
      localStorage.removeItem("todo.draft");
    } catch {
      // Already gone, or storage is unavailable.
    }
  },
};
