import { useEffect, useRef } from "react";
import type { Filters, Task } from "../api";
import type { Route } from "./route";

/** g-chords reach the narrowing filters; the places you live have a plain key. */
const JUMPS: Record<string, string> = {
  o: "overdue",
  u: "upcoming",
  a: "anytime",
  d: "delegated",
};

/** The plain keys, and where each one goes. */
const VIEWS: Record<string, string> = {
  t: "today",
  a: "all",
  l: "logbook",
};

export type KeyActions = {
  go: (kind: Route["kind"], f: Filters) => void;
  filters: Filters;
  cursorTask?: Task;
  overlayOpen: boolean;
  detailOpen: boolean;
  searching: boolean;
  canUndo: boolean;
  openOverlay: (o: "capture" | "palette" | "settings" | "shortcuts") => void;
  closeOverlay: () => void;
  closeDetail: () => void;
  openDetail: (t: Task) => void;
  moveCursor: (delta: number) => void;
  toggle: (t: Task) => void;
  remove: (id: number) => void;
  undo: () => void;
  startSearch: () => void;
  stopSearch: () => void;
  toggleSidebar: () => void;
  toggleFilters: () => void;
};

function isTyping(el: EventTarget | null): boolean {
  const t = el as HTMLElement | null;
  return Boolean(
    t && (t.tagName === "INPUT" || t.tagName === "TEXTAREA" || t.isContentEditable),
  );
}

/**
 * The keyboard layer.
 *
 * Kept apart from the layout because it is the thing most likely to grow, and
 * because a component that both draws a list and interprets `dd` is two things.
 */
export function useKeyboard(a: KeyActions) {
  const chord = useRef<string | null>(null);
  const ref = useRef(a);
  ref.current = a;

  useEffect(() => {
    function onKey(e: KeyboardEvent) {
      const k = ref.current;

      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === "k") {
        e.preventDefault();
        if (k.overlayOpen) k.closeOverlay();
        else k.openOverlay("palette");
        return;
      }

      if (e.key === "Escape") {
        if (k.overlayOpen) return k.closeOverlay();
        if (k.detailOpen) return k.closeDetail();
        if (k.searching) return k.stopSearch();
        if (isTyping(e.target)) (e.target as HTMLElement).blur();
        return;
      }

      if (isTyping(e.target) || e.metaKey || e.ctrlKey || e.altKey || k.overlayOpen) return;

      if (chord.current === "g") {
        chord.current = null;
        const view = JUMPS[e.key];
        if (view) {
          e.preventDefault();
          k.go("list", { view });
        }
        return;
      }

      if (chord.current === "d") {
        chord.current = null;
        if (e.key === "d" && k.cursorTask) {
          e.preventDefault();
          k.remove(k.cursorTask.id);
        }
        return;
      }

      if (VIEWS[e.key]) {
        e.preventDefault();
        k.go("list", { view: VIEWS[e.key] });
        return;
      }

      switch (e.key) {
        case "g":
        case "d":
          chord.current = e.key;
          setTimeout(() => (chord.current = null), 800);
          break;
        case "c":
          e.preventDefault();
          k.go("calls", {});
          break;
        case "w":
          e.preventDefault();
          k.go("week", {});
          break;
        case "n":
          e.preventDefault();
          k.openOverlay("capture");
          break;
        case "?":
          e.preventDefault();
          k.openOverlay("shortcuts");
          break;
        case "b":
        case "[":
          e.preventDefault();
          k.toggleSidebar();
          break;
        case "f":
          e.preventDefault();
          k.toggleFilters();
          break;
        case "/":
          e.preventDefault();
          k.startSearch();
          break;
        case "j":
          e.preventDefault();
          k.moveCursor(1);
          break;
        case "k":
          e.preventDefault();
          k.moveCursor(-1);
          break;
        case "x":
          if (k.cursorTask) {
            e.preventDefault();
            k.toggle(k.cursorTask);
          }
          break;
        case "e":
          if (k.cursorTask) {
            e.preventDefault();
            k.openDetail(k.cursorTask);
          }
          break;
        case "u":
          if (k.canUndo) {
            e.preventDefault();
            k.undo();
          }
          break;
      }
    }

    addEventListener("keydown", onKey);
    return () => removeEventListener("keydown", onKey);
  }, []);
}
