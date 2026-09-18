import { cn } from "@/lib/utils";
import type {
  WorkflowRunStatus,
  WorkflowStepStatus,
  WorkflowTriggerType,
} from "@/types";

export function triggerTypeLabel(t: WorkflowTriggerType | string) {
  switch (t) {
    case "field_changed":
      return "Field changed";
    case "schedule":
      return "Schedule";
    default:
      return t;
  }
}

export function runStatusBadge(status: WorkflowRunStatus | WorkflowStepStatus) {
  switch (status) {
    case "success":
      return "badge badge-green";
    case "failed":
      return "badge badge-red";
    case "running":
      return "badge badge-blue";
    case "skipped":
      return "badge badge-gray";
    case "pending":
    default:
      return "badge badge-gray";
  }
}

export function ActiveDot({ active }: { active: boolean }) {
  return (
    <span
      className={cn(
        "inline-block h-2 w-2 rounded-full",
        active ? "bg-emerald-500" : "bg-slate-300 dark:bg-slate-600",
      )}
      title={active ? "Active" : "Inactive"}
    />
  );
}
