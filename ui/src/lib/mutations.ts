import { useCallback } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { api, type Filters, type ListResponse, type Task } from "../api";

/**
 * Everything that changes a task, in one place.
 *
 * Completing is optimistic and rolls back on failure; the rest report what went
 * wrong rather than quietly not happening. Which queries to invalidate is
 * decided here too: an edit refreshes the board you are looking at, while a
 * capture or an undo can change any of them.
 */
export function useTaskMutations({
  filters,
  isWeek,
  linger,
  onFailed,
}: {
  filters: Filters;
  isWeek: boolean;
  /** How long a completed task stays struck through before the list catches up. */
  linger: number;
  onFailed: (e: unknown) => void;
}) {
  const qc = useQueryClient();

  const invalidateAll = useCallback(() => {
    void qc.invalidateQueries({ queryKey: ["list"] });
    void qc.invalidateQueries({ queryKey: ["week"] });
    void qc.invalidateQueries({ queryKey: ["sessions"] });
  }, [qc]);

  const refresh = useCallback(() => {
    void qc.invalidateQueries({ queryKey: [isWeek ? "week" : "list"] });
  }, [qc, isWeek]);

  const toggle = useMutation({
    mutationFn: (t: Task) => api.toggle(t.id),
    // Tick immediately; the list catches up behind it.
    onMutate: async (t: Task) => {
      await qc.cancelQueries({ queryKey: ["list", filters] });
      const previous = qc.getQueryData<ListResponse>(["list", filters]);
      qc.setQueryData<ListResponse>(["list", filters], (old) =>
        old
          ? {
              ...old,
              sections: old.sections.map((s) => ({
                ...s,
                tasks: s.tasks.map((x) => (x.id === t.id ? { ...x, done: !x.done } : x)),
              })),
            }
          : old,
      );
      return { previous };
    },
    onError: (e, _t, ctx) => {
      if (ctx?.previous) qc.setQueryData(["list", filters], ctx.previous);
      onFailed(e);
    },
    // Held back on purpose: refetching at once would whip the struck-through
    // row off the page before that registered as anything.
    onSettled: () => setTimeout(refresh, linger),
  });

  const schedule = useMutation({
    mutationFn: ({ id, date }: { id: number; date: string }) => api.schedule(id, date),
    onSuccess: refresh,
    onError: onFailed,
  });

  const move = useMutation({
    mutationFn: ({ id, above, below }: { id: number; above: number; below: number }) =>
      api.move(id, above, below),
    onSuccess: refresh,
    onError: onFailed,
  });

  const remove = useCallback(
    (id: number) => api.remove(id).then(invalidateAll).catch(onFailed),
    [invalidateAll, onFailed],
  );

  const undoBatch = useCallback(
    (batchId: number) => api.undoBatch(batchId).then(invalidateAll).catch(onFailed),
    [invalidateAll, onFailed],
  );

  return { toggle, schedule, move, remove, undoBatch, invalidateAll, refresh };
}
