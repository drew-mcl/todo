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
  todayLabel: string;
  doneToday: number;
};

export type Session = {
  id: number;
  title: string;
  source: string;
  when: string;
  date: string;
  total: number;
  open: number;
  done: number;
  preview: string;
};

export type Role =
  | "ignore" | "title" | "topic" | "owner" | "due" | "note" | "priority" | "tags" | "status";

export type TableColumn = { index: number; header: string; role: Role };
export type TableRow = { n: number; cells: string[]; task?: PreviewTask; reason?: string };
export type TablePreview = {
  columns: TableColumn[];
  rows: TableRow[];
  format: string;
  preset: string;
  roles: Role[];
  tasks: number;
  skipped: number;
};

export type ListResponse = {
  view: string;
  sort: string;
  sections: Section[];
  total: number;
  shown: number;
  truncated: boolean;
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
  when?: string;
  batch?: string;
};

/** The named periods the "when" filter offers. */
export const WHEN = [
  { value: "", label: "any time" },
  { value: "today", label: "today" },
  { value: "yesterday", label: "yesterday" },
  { value: "week", label: "this week" },
  { value: "lastweek", label: "last week" },
  { value: "month", label: "this month" },
];

export class ApiError extends Error {
  status: number;
  /** The input the server refused, when it named one. */
  field?: string;
  constructor(status: number, message: string, field?: string) {
    super(message);
    this.status = status;
    this.field = field;
  }
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(path, {
    ...init,
    headers: init?.body ? { "Content-Type": "application/json" } : undefined,
  });
  if (!res.ok) {
    let message = `Request failed (${res.status})`;
    let field: string | undefined;
    try {
      const body = await res.json();
      if (body?.error) message = body.error;
      if (body?.field) field = body.field;
    } catch {
      // A non-JSON error body is not worth surfacing verbatim.
    }
    throw new ApiError(res.status, message, field);
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

/** One key in the capture box, and the group it is shown under. */
export type KeyGroup = { name: string; keys: { press: string; does: string }[] };

export const api = {
  keys: () => request<{ vim: KeyGroup[] }>("/api/keys").then((r) => r.vim),
  list: (f: Filters) => request<ListResponse>(`/api/list${toQuery(f)}`),
  week: (f: Filters) => request<WeekResponse>(`/api/week${toQuery(f)}`),
  preview: (draft: string) => post<PreviewResponse>("/api/preview", { draft }),
  capture: (draft: string, title?: string) =>
    post<{ batchId: number; added: number }>("/api/capture", { draft, title }),
  tablePreview: (input: string, mapping?: Role[], topic?: string) =>
    post<TablePreview>("/api/table/preview", { input, mapping, topic }),
  tableCapture: (input: string, mapping: Role[], topic: string, title: string, source: string) =>
    post<{ batchId: number; added: number }>("/api/table/capture", {
      input, mapping, topic, title, source,
    }),
  sessions: () => request<Session[]>("/api/sessions"),
  renameSession: (id: number, title: string) =>
    post<void>(`/api/sessions/${id}/rename`, { title }),
  exportSession: (id: number, group: boolean, notes: boolean) =>
    request<{ text: string }>(
      `/api/sessions/${id}/export?group=${group ? "owner" : "none"}&notes=${notes ? 1 : 0}`,
    ),
  undoBatch: (id: number) => post<{ removed: number }>(`/api/batches/${id}/undo`),
  toggle: (id: number) => post<Task>(`/api/tasks/${id}/toggle`),
  move: (id: number, above: number, below: number) =>
    post<void>(`/api/tasks/${id}/move`, { Above: above, Below: below }),
  schedule: (id: number, date: string) => post<Task>(`/api/tasks/${id}/schedule`, { date }),
  update: (id: number, patch: Record<string, unknown>) =>
    request<Task>(`/api/tasks/${id}`, { method: "PATCH", body: JSON.stringify(patch) }),
  remove: (id: number) => request<void>(`/api/tasks/${id}`, { method: "DELETE" }),
};
