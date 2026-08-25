// Typed client for the Go API.
//
// Every derived value -- due labels, overdue-ness, section grouping, shorthand
// highlighting -- is computed on the server and sent down. Nothing here
// re-implements a rule that already exists in Go.

export type Task = {
  id: number;
  topic: string;
  title: string;
  note: string;
  due: string | null;
  dueLabel: string;
  overdue: boolean;
  assignee: string;
  priority: number;
  done: boolean;
  tags: string[];
  raw: string;
  batchId: number | null;
  capturedAt: string;
  capturedWhen: string;
  completedAt?: string;
};

export type Section = { label: string; tasks: Task[] };
export type Group = { name: string; count: number };

export type Meta = {
  counts: Record<string, number>;
  topics: Group[];
  people: Group[];
  tags: Group[];
  today: string;
};

export type ListResponse = {
  view: string;
  sort: string;
  sections: Section[];
  total: number;
  canDrag: boolean;
  meta: Meta;
};

export type Day = {
  date: string;
  weekday: string;
  label: string;
  isToday: boolean;
  isPast: boolean;
  tasks: Task[];
};

export type WeekResponse = {
  start: string;
  prev: string;
  next: string;
  range: string;
  days: Day[];
  overdue: Task[];
  unscheduled: Task[];
  meta: Meta;
};

export type TokenKind =
  | "text" | "pipe" | "topic" | "due" | "who" | "pri" | "tag" | "note";

export type Token = { kind: TokenKind; text: string };

export type PreviewTask = {
  topic: string;
  title: string;
  note: string;
  dueLabel: string;
  assignee: string;
  priority: number;
  tags: string[];
  warning: string;
};

export type PreviewLine = {
  n: number;
  raw: string;
  kind: "task" | "note" | "skipped";
  reason?: string;
  tokens?: Token[];
  task?: PreviewTask;
  ownerTitle?: string;
};

export type PreviewResponse = {
  lines: PreviewLine[];
  tasks: number;
  notes: number;
  skipped: number;
};

export type Filters = {
  view?: string;
  sort?: string;
  topic?: string;
  tag?: string;
  assignee?: string;
  q?: string;
  start?: string;
};

export class ApiError extends Error {
  status: number;
  constructor(status: number, message: string) {
    super(message);
    this.status = status;
  }
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(path, {
    ...init,
    headers: init?.body ? { "Content-Type": "application/json" } : undefined,
  });
  if (!res.ok) {
    let message = `Request failed (${res.status})`;
    try {
      const body = await res.json();
      if (body?.error) message = body.error;
    } catch {
      // A non-JSON error body is not worth surfacing verbatim.
    }
    throw new ApiError(res.status, message);
  }
  if (res.status === 204) return undefined as T;
  return res.json() as Promise<T>;
}

export function toQuery(f: Filters): string {
  const p = new URLSearchParams();
  for (const [k, v] of Object.entries(f)) if (v) p.set(k, v);
  const s = p.toString();
  return s ? `?${s}` : "";
}

const post = <T,>(path: string, body?: unknown) =>
  request<T>(path, { method: "POST", body: body ? JSON.stringify(body) : undefined });

export const api = {
  list: (f: Filters) => request<ListResponse>(`/api/list${toQuery(f)}`),
  week: (f: Filters) => request<WeekResponse>(`/api/week${toQuery(f)}`),
  preview: (draft: string) => post<PreviewResponse>("/api/preview", { draft }),
  capture: (draft: string) => post<{ batchId: number; added: number }>("/api/capture", { draft }),
  undoBatch: (id: number) => post<{ removed: number }>(`/api/batches/${id}/undo`),
  toggle: (id: number) => post<Task>(`/api/tasks/${id}/toggle`),
  move: (id: number, above: number, below: number) =>
    post<void>(`/api/tasks/${id}/move`, { Above: above, Below: below }),
  schedule: (id: number, date: string) => post<Task>(`/api/tasks/${id}/schedule`, { date }),
  update: (id: number, patch: Record<string, unknown>) =>
    request<Task>(`/api/tasks/${id}`, { method: "PATCH", body: JSON.stringify(patch) }),
  remove: (id: number) => request<void>(`/api/tasks/${id}`, { method: "DELETE" }),
};
