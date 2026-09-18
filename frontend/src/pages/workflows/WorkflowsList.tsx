/**
 * Workflows list — name, active toggle, trigger type, last fired.
 */
import { useState } from "react";
import { Link, useNavigate } from "@tanstack/react-router";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import * as Switch from "@radix-ui/react-switch";
import { History, Pencil, Plus, Trash2, Workflow } from "lucide-react";
import { del, get, patch } from "@/lib/api";
import { formatDate } from "@/lib/utils";
import { Card } from "@/components/ui/Card";
import { Button } from "@/components/ui/Button";
import type { PaginatedResponse, Workflow as Wf } from "@/types";
import { ActiveDot, triggerTypeLabel } from "./helpers";

export default function WorkflowsPage() {
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const [deletingId, setDeletingId] = useState<string | null>(null);

  const { data, isLoading } = useQuery({
    queryKey: ["workflows"],
    queryFn: () => get<PaginatedResponse<Wf>>("/workflows"),
  });
  const workflows = data?.data ?? [];

  const toggleMut = useMutation({
    mutationFn: ({ id, active }: { id: string; active: boolean }) =>
      patch<Wf>(`/workflows/${id}`, { active }),
    onSuccess: (updated) => {
      queryClient.setQueryData<PaginatedResponse<Wf>>(["workflows"], (prev) => {
        if (!prev) return prev;
        return {
          ...prev,
          data: prev.data.map((w) => (w.id === updated.id ? updated : w)),
        };
      });
    },
  });

  const deleteMut = useMutation({
    mutationFn: (id: string) => del(`/workflows/${id}`),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["workflows"] });
      setDeletingId(null);
    },
  });

  return (
    <div className="space-y-5">
      <div className="flex items-center justify-between gap-3">
        <div>
          <h1 className="text-2xl font-bold text-slate-900 dark:text-slate-100">
            Workflows
          </h1>
          <p className="mt-1 text-sm text-slate-500 dark:text-slate-400">
            Automations that run when a field changes or on a schedule.
          </p>
        </div>
        <Button type="button" onClick={() => navigate({ to: "/workflows/new" })}>
          <Plus size={16} />
          New workflow
        </Button>
      </div>

      <Card className="overflow-hidden p-0">
        <table className="w-full text-sm">
          <thead>
            <tr className="border-b border-slate-100 text-left text-xs font-medium text-slate-500 dark:border-white/10 dark:text-slate-400">
              <th className="px-5 py-3">Name</th>
              <th className="px-5 py-3">Trigger</th>
              <th className="px-5 py-3">Active</th>
              <th className="px-5 py-3">Last fired</th>
              <th className="w-40 px-5 py-3" />
            </tr>
          </thead>
          <tbody>
            {isLoading && (
              <tr>
                <td colSpan={5} className="py-12 text-center text-slate-400">
                  Loading…
                </td>
              </tr>
            )}
            {!isLoading && workflows.length === 0 && (
              <tr>
                <td colSpan={5} className="px-5 py-14 text-center">
                  <div className="mx-auto flex max-w-sm flex-col items-center gap-2">
                    <div className="flex h-10 w-10 items-center justify-center rounded-xl bg-slate-100 text-slate-400 dark:bg-white/10">
                      <Workflow size={18} />
                    </div>
                    <p className="text-sm font-medium text-slate-700 dark:text-slate-200">
                      No workflows yet
                    </p>
                    <p className="text-xs text-slate-400">
                      Create one to automate your workflows.
                    </p>
                    <Button
                      type="button"
                      size="sm"
                      className="mt-2"
                      onClick={() => navigate({ to: "/workflows/new" })}
                    >
                      <Plus size={14} />
                      New workflow
                    </Button>
                  </div>
                </td>
              </tr>
            )}
            {workflows.map((wf) => (
              <tr
                key={wf.id}
                className="border-b border-slate-100 last:border-0 hover:bg-slate-50/60 dark:border-white/10 dark:hover:bg-white/5"
              >
                <td className="px-5 py-3">
                  <div className="flex items-center gap-2.5">
                    <ActiveDot active={wf.active} />
                    <div>
                      <Link
                        to="/workflows/$id"
                        params={{ id: wf.id }}
                        className="font-medium text-slate-900 hover:text-brand-700 dark:text-slate-100 dark:hover:text-brand-300"
                      >
                        {wf.name}
                      </Link>
                      <p className="font-mono text-[10px] text-slate-400">
                        {(wf.trigger.config.collection as string) || "—"}
                      </p>
                    </div>
                  </div>
                </td>
                <td className="px-5 py-3">
                  <span className="badge badge-gray">
                    {triggerTypeLabel(wf.trigger.type)}
                  </span>
                </td>
                <td className="px-5 py-3">
                  <Switch.Root
                    checked={wf.active}
                    disabled={
                      toggleMut.isPending && toggleMut.variables?.id === wf.id
                    }
                    onCheckedChange={(active) =>
                      toggleMut.mutate({ id: wf.id, active })
                    }
                    className={`relative h-5 w-9 shrink-0 rounded-full transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500 focus-visible:ring-offset-1 disabled:opacity-50 ${
                      wf.active
                        ? "bg-brand-600"
                        : "bg-slate-200 dark:bg-white/15"
                    }`}
                    aria-label={wf.active ? "Deactivate" : "Activate"}
                  >
                    <Switch.Thumb
                      className={`block h-3.5 w-3.5 rounded-full bg-white shadow-sm transition-transform ${
                        wf.active ? "translate-x-[18px]" : "translate-x-[3px]"
                      }`}
                    />
                  </Switch.Root>
                </td>
                <td className="px-5 py-3 text-slate-500 dark:text-slate-400">
                  {wf.last_fired_at ? formatDate(wf.last_fired_at) : "—"}
                </td>
                <td className="px-5 py-3">
                  <div className="flex items-center justify-end gap-1">
                    <Button
                      type="button"
                      variant="ghost"
                      size="sm"
                      onClick={() =>
                        navigate({
                          to: "/workflows/$id/runs",
                          params: { id: wf.id },
                        })
                      }
                      title="Run history"
                    >
                      <History size={14} />
                    </Button>
                    <Button
                      type="button"
                      variant="ghost"
                      size="sm"
                      onClick={() =>
                        navigate({
                          to: "/workflows/$id",
                          params: { id: wf.id },
                        })
                      }
                      title="Edit"
                    >
                      <Pencil size={14} />
                    </Button>
                    <Button
                      type="button"
                      variant="ghost"
                      size="sm"
                      className="text-red-500 hover:text-red-600"
                      disabled={deleteMut.isPending && deletingId === wf.id}
                      onClick={() => {
                        if (
                          !window.confirm(
                            `Delete workflow “${wf.name}”? This cannot be undone.`,
                          )
                        ) {
                          return;
                        }
                        setDeletingId(wf.id);
                        deleteMut.mutate(wf.id);
                      }}
                      title="Delete"
                    >
                      <Trash2 size={14} />
                    </Button>
                  </div>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </Card>

      {toggleMut.isError && (
        <p className="text-sm text-red-600 dark:text-red-400">
          {(toggleMut.error as Error).message}
        </p>
      )}
      {deleteMut.isError && (
        <p className="text-sm text-red-600 dark:text-red-400">
          {(deleteMut.error as Error).message}
        </p>
      )}
    </div>
  );
}
