import type { TFunction } from "i18next";
import { useTranslation } from "react-i18next";
import { cn } from "@/lib/utils";
import type {
  WorkflowRunStatus,
  WorkflowStepStatus,
  WorkflowTriggerType,
} from "@/types";

export function triggerTypeLabel(
  t: TFunction,
  type: WorkflowTriggerType | string,
  fallback?: string,
) {
  switch (type) {
    case "field_changed":
    case "schedule":
      return t(`workflows.triggerTypes.${type}`);
    default:
      return fallback ?? type;
  }
}

export function actionTypeLabel(t: TFunction, type: string) {
  return type === "email_with_pdf" ? t("workflows.actionTypes.email_with_pdf") : type;
}

export function runStatusLabel(
  t: TFunction,
  status: WorkflowRunStatus | WorkflowStepStatus,
) {
  return t(`workflows.runStatus.${status}`, { defaultValue: status });
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
  const { t } = useTranslation();
  return (
    <span
      className={cn(
        "inline-block h-2 w-2 rounded-full",
        active ? "bg-emerald-500" : "bg-slate-300 dark:bg-slate-600",
      )}
      title={active ? t("common.active") : t("workflows.inactive")}
    />
  );
}
