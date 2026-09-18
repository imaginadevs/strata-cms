import { memo } from "react";
import {
  Handle,
  Position,
  type Node,
  type NodeProps,
} from "@xyflow/react";
import { CalendarClock, Mail, Zap } from "lucide-react";
import { cn } from "@/lib/utils";

export type WfNodeData = {
  kind: "trigger" | "action";
  title: string;
  subtitle: string;
  selected?: boolean;
};

export type WfNode = Node<WfNodeData, "wf">;

function WfNodeView({ data, selected }: NodeProps<WfNode>) {
  const isTrigger = data.kind === "trigger";
  const Icon = isTrigger
    ? data.title.toLowerCase().includes("schedule")
      ? CalendarClock
      : Zap
    : Mail;

  return (
    <div
      className={cn(
        "w-[220px] rounded-2xl border bg-white shadow-md transition dark:bg-slate-900",
        selected
          ? "border-brand-400 ring-2 ring-brand-400/30 dark:border-brand-400"
          : "border-slate-200 dark:border-white/10",
      )}
    >
      {/* Actions accept incoming links; triggers are start-only (no entrance). */}
      {!isTrigger && (
        <Handle
          type="target"
          position={Position.Left}
          className="!h-2.5 !w-2.5 !border-2 !border-white !bg-slate-400 dark:!border-slate-900"
        />
      )}
      <div
        className={cn(
          "flex items-center gap-2 rounded-t-2xl px-3 py-2 text-white",
          isTrigger ? "bg-amber-500" : "bg-brand-600",
          data.title.toLowerCase().includes("schedule") && isTrigger && "bg-violet-600",
        )}
      >
        <Icon size={14} />
        <span className="truncate text-xs font-semibold uppercase tracking-wide">
          {isTrigger ? "Trigger" : "Action"}
        </span>
      </div>
      <div className="px-3 py-2.5">
        <p className="truncate text-sm font-semibold text-slate-900 dark:text-slate-100">
          {data.title}
        </p>
        <p className="mt-0.5 line-clamp-2 text-[11px] leading-snug text-slate-500 dark:text-slate-400">
          {data.subtitle}
        </p>
      </div>
      <Handle
        type="source"
        position={Position.Right}
        className="!h-2.5 !w-2.5 !border-2 !border-white !bg-brand-500 dark:!border-slate-900"
      />
    </div>
  );
}

export const nodeTypes = {
  wf: memo(WfNodeView),
};
