/**
 * Workflow run history — poll list, open run detail drawer with steps.
 */
import { useEffect, useState } from "react";
import { createPortal } from "react-dom";
import { Link, useParams } from "@tanstack/react-router";
import { useQuery } from "@tanstack/react-query";
import { AnimatePresence, motion } from "framer-motion";
import { ArrowLeft, RefreshCw, X } from "lucide-react";
import { get } from "@/lib/api";
import { formatDate } from "@/lib/utils";
import { Card } from "@/components/ui/Card";
import { Button } from "@/components/ui/Button";
import type {
  PaginatedResponse,
  Workflow,
  WorkflowRun,
  WorkflowRunStatus,
} from "@/types";
import { runStatusBadge, triggerTypeLabel } from "./helpers";

const STATUS_FILTERS: Array<{ value: "" | WorkflowRunStatus; label: string }> =
  [
    { value: "", label: "All" },
    { value: "pending", label: "Pending" },
    { value: "running", label: "Running" },
    { value: "success", label: "Success" },
    { value: "failed", label: "Failed" },
  ];

export default function WorkflowRunsPage() {
  const { id } = useParams({ strict: false }) as { id: string };
  const [status, setStatus] = useState<"" | WorkflowRunStatus>("");
  const [selectedRunId, setSelectedRunId] = useState<string | null>(null);

  const { data: workflow } = useQuery({
    queryKey: ["workflows", id],
    queryFn: () => get<Workflow>(`/workflows/${id}`),
    enabled: !!id,
  });

  const { data, isLoading, isFetching, refetch } = useQuery({
    queryKey: ["workflows", id, "runs", { status }],
    queryFn: () =>
      get<PaginatedResponse<WorkflowRun>>(`/workflows/${id}/runs`, {
        page: 1,
        limit: 50,
        ...(status ? { status } : {}),
      }),
    enabled: !!id,
    refetchInterval: (q) => {
      const rows = q.state.data?.data ?? [];
      const busy = rows.some(
        (r) => r.status === "pending" || r.status === "running",
      );
      return busy ? 3_000 : 15_000;
    },
  });
  const runs = data?.data ?? [];

  return (
    <div className="space-y-5">
      <div className="flex items-start justify-between gap-3">
        <div className="flex items-start gap-3">
          <Link
            to="/workflows"
            className="mt-1 rounded-lg p-1.5 text-slate-400 transition hover:bg-slate-100 hover:text-slate-700 dark:hover:bg-white/5 dark:hover:text-slate-200"
          >
            <ArrowLeft size={18} />
          </Link>
          <div>
            <h1 className="text-2xl font-bold text-slate-900 dark:text-slate-100">
              Run history
            </h1>
            <p className="mt-1 text-sm text-slate-500 dark:text-slate-400">
              {workflow?.name ?? "…"}
              {workflow ? (
                <>
                  {" · "}
                  <span className="badge badge-gray">
                    {triggerTypeLabel(workflow.trigger.type)}
                  </span>
                </>
              ) : null}
            </p>
          </div>
        </div>
        <div className="flex items-center gap-2">
          <Button
            type="button"
            variant="secondary"
            size="sm"
            onClick={() => refetch()}
            disabled={isFetching}
          >
            <RefreshCw size={14} className={isFetching ? "animate-spin" : ""} />
            Refresh
          </Button>
          <Link
            to="/workflows/$id"
            params={{ id }}
            className="inline-flex h-8 items-center rounded-xl px-3 text-xs font-medium text-slate-600 transition hover:bg-slate-100 hover:text-slate-900 dark:text-slate-300 dark:hover:bg-white/10 dark:hover:text-slate-100"
          >
            Edit workflow
          </Link>
        </div>
      </div>

      <div className="flex flex-wrap gap-1.5">
        {STATUS_FILTERS.map((f) => (
          <button
            key={f.label}
            type="button"
            onClick={() => setStatus(f.value)}
            className={`rounded-lg px-2.5 py-1 text-xs font-medium transition ${
              status === f.value
                ? "bg-brand-50 text-brand-800 dark:bg-brand-500/15 dark:text-brand-200"
                : "text-slate-500 hover:bg-slate-100 dark:text-slate-400 dark:hover:bg-white/5"
            }`}
          >
            {f.label}
          </button>
        ))}
      </div>

      <Card className="overflow-hidden p-0">
        <table className="w-full text-sm">
          <thead>
            <tr className="border-b border-slate-100 text-left text-xs font-medium text-slate-500 dark:border-white/10 dark:text-slate-400">
              <th className="px-5 py-3">Status</th>
              <th className="px-5 py-3">Collection / item</th>
              <th className="px-5 py-3">Started</th>
              <th className="px-5 py-3">Finished</th>
              <th className="px-5 py-3">Error</th>
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
            {!isLoading && runs.length === 0 && (
              <tr>
                <td colSpan={5} className="py-12 text-center text-slate-400">
                  No runs yet
                </td>
              </tr>
            )}
            {runs.map((run) => (
              <tr
                key={run.id}
                className="cursor-pointer border-b border-slate-100 last:border-0 hover:bg-slate-50/60 dark:border-white/10 dark:hover:bg-white/5"
                onClick={() => setSelectedRunId(run.id)}
              >
                <td className="px-5 py-3">
                  <span className={runStatusBadge(run.status)}>{run.status}</span>
                </td>
                <td className="px-5 py-3">
                  <p className="font-mono text-xs text-slate-700 dark:text-slate-200">
                    {run.collection}
                  </p>
                  <p className="truncate font-mono text-[10px] text-slate-400">
                    {run.item_id || "—"}
                  </p>
                </td>
                <td className="px-5 py-3 text-slate-500 dark:text-slate-400">
                  {formatDate(run.started_at)}
                </td>
                <td className="px-5 py-3 text-slate-500 dark:text-slate-400">
                  {run.finished_at ? formatDate(run.finished_at) : "—"}
                </td>
                <td className="max-w-[14rem] px-5 py-3">
                  {run.error ? (
                    <p className="truncate text-xs text-red-600 dark:text-red-400">
                      {run.error}
                    </p>
                  ) : (
                    <span className="text-slate-300 dark:text-slate-600">—</span>
                  )}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </Card>

      <AnimatePresence>
        {selectedRunId && (
          <RunDetailPanel
            runId={selectedRunId}
            onClose={() => setSelectedRunId(null)}
          />
        )}
      </AnimatePresence>
    </div>
  );
}

function RunDetailPanel({
  runId,
  onClose,
}: {
  runId: string;
  onClose: () => void;
}) {
  const { data: run, isLoading, isFetching } = useQuery({
    queryKey: ["workflows", "runs", runId],
    queryFn: () => get<WorkflowRun>(`/workflows/runs/${runId}`),
    refetchInterval: (q) => {
      const s = q.state.data?.status;
      return s === "pending" || s === "running" ? 2_000 : false;
    },
  });

  // Escape to close
  useEffect(() => {
    function onKey(e: KeyboardEvent) {
      if (e.key === "Escape") onClose();
    }
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [onClose]);

  const steps = [...(run?.steps ?? [])].sort((a, b) => a.sort - b.sort);

  return createPortal(
    <>
      <motion.div
        key="backdrop"
        initial={{ opacity: 0 }}
        animate={{ opacity: 1 }}
        exit={{ opacity: 0 }}
        transition={{ duration: 0.2 }}
        className="fixed inset-0 z-40 bg-black/20 backdrop-blur-[2px]"
        onClick={onClose}
      />
      <motion.aside
        key="panel"
        initial={{ x: "100%" }}
        animate={{ x: 0 }}
        exit={{ x: "100%" }}
        transition={{ type: "spring", damping: 28, stiffness: 280 }}
        className="fixed right-0 top-0 z-50 flex h-full w-full max-w-md flex-col bg-white shadow-2xl dark:bg-slate-900"
      >
        <div className="flex shrink-0 items-center justify-between border-b border-slate-100 px-6 py-5 dark:border-white/10">
          <div>
            <h2 className="text-base font-semibold text-slate-900 dark:text-slate-100">
              Run detail
            </h2>
            <p className="mt-0.5 font-mono text-[10px] text-slate-400">{runId}</p>
          </div>
          <button
            type="button"
            onClick={onClose}
            className="rounded-lg p-1.5 text-slate-400 transition hover:bg-slate-100 hover:text-slate-600 dark:hover:bg-white/5 dark:hover:text-slate-200"
          >
            <X size={18} />
          </button>
        </div>

        <div className="flex-1 space-y-4 overflow-y-auto px-6 py-5">
          {isLoading || !run ? (
            <p className="py-10 text-center text-sm text-slate-400">Loading…</p>
          ) : (
            <>
              <div className="flex flex-wrap items-center gap-2">
                <span className={runStatusBadge(run.status)}>{run.status}</span>
                {isFetching && (
                  <span className="text-[10px] text-slate-400">Updating…</span>
                )}
              </div>

              <dl className="grid grid-cols-2 gap-3 text-xs">
                <div>
                  <dt className="text-slate-400">Collection</dt>
                  <dd className="mt-0.5 font-mono text-slate-800 dark:text-slate-100">
                    {run.collection}
                  </dd>
                </div>
                <div>
                  <dt className="text-slate-400">Item</dt>
                  <dd className="mt-0.5 truncate font-mono text-slate-800 dark:text-slate-100">
                    {run.item_id || "—"}
                  </dd>
                </div>
                <div>
                  <dt className="text-slate-400">Started</dt>
                  <dd className="mt-0.5 text-slate-800 dark:text-slate-100">
                    {formatDate(run.started_at)}
                  </dd>
                </div>
                <div>
                  <dt className="text-slate-400">Finished</dt>
                  <dd className="mt-0.5 text-slate-800 dark:text-slate-100">
                    {run.finished_at ? formatDate(run.finished_at) : "—"}
                  </dd>
                </div>
              </dl>

              {run.error && (
                <p className="rounded-xl bg-red-50 px-3 py-2 text-xs text-red-600 dark:bg-red-500/10 dark:text-red-400">
                  {run.error}
                </p>
              )}

              <div>
                <p className="mb-2 text-xs font-semibold uppercase tracking-wide text-slate-500 dark:text-slate-400">
                  Steps
                </p>
                {steps.length === 0 ? (
                  <p className="text-xs text-slate-400">No steps</p>
                ) : (
                  <ul className="space-y-2">
                    {steps.map((step) => (
                      <li
                        key={step.id}
                        className="rounded-xl border border-slate-100 px-3 py-2.5 dark:border-white/10"
                      >
                        <div className="flex items-center justify-between gap-2">
                          <p className="text-sm font-medium text-slate-800 dark:text-slate-100">
                            {step.action_type}
                          </p>
                          <span className={runStatusBadge(step.status)}>
                            {step.status}
                          </span>
                        </div>
                        {step.error && (
                          <p className="mt-1 text-xs text-red-600 dark:text-red-400">
                            {step.error}
                          </p>
                        )}
                        {step.output && Object.keys(step.output).length > 0 && (
                          <pre className="mt-2 overflow-x-auto rounded-lg bg-slate-50 p-2 font-mono text-[10px] text-slate-600 dark:bg-white/5 dark:text-slate-300">
                            {JSON.stringify(step.output, null, 2)}
                          </pre>
                        )}
                      </li>
                    ))}
                  </ul>
                )}
              </div>
            </>
          )}
        </div>
      </motion.aside>
    </>,
    document.body,
  );
}
