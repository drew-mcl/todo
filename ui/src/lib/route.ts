// A very small router. The app has two shapes -- a list and the planner -- so a
// routing library would be more machinery than the problem needs.

import { useCallback, useEffect, useState } from "react";
import type { Filters } from "../api";

export type Route = { kind: "list" | "week" | "calls"; filters: Filters };

function parse(): Route {
  const url = new URL(location.href);
  const q = Object.fromEntries(url.searchParams) as Filters;
  if (url.pathname.startsWith("/week")) return { kind: "week", filters: q };
  if (url.pathname.startsWith("/calls")) return { kind: "calls", filters: q };
  const m = url.pathname.match(/^\/v\/([a-z]+)/);
  return { kind: "list", filters: { ...q, view: m?.[1] ?? "today" } };
}

export function href(kind: Route["kind"], filters: Filters): string {
  const { view, ...rest } = filters;
  const p = new URLSearchParams();
  for (const [k, v] of Object.entries(rest)) if (v) p.set(k, v);
  const qs = p.toString();
  const base =
    kind === "week" ? "/week" : kind === "calls" ? "/calls" : `/v/${view ?? "today"}`;
  return qs ? `${base}?${qs}` : base;
}

export function useRoute() {
  const [route, setRoute] = useState<Route>(parse);

  useEffect(() => {
    const onPop = () => setRoute(parse());
    addEventListener("popstate", onPop);
    return () => removeEventListener("popstate", onPop);
  }, []);

  const go = useCallback((kind: Route["kind"], filters: Filters) => {
    const to = href(kind, filters);
    if (to !== location.pathname + location.search) history.pushState(null, "", to);
    setRoute(parse());
  }, []);

  return { route, go };
}
