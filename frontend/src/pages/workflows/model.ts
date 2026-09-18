import type {
  EmailWithPdfActionConfig,
  FieldChangedTriggerConfig,
  ScheduleTriggerConfig,
  Workflow,
  WorkflowActionType,
  WorkflowTriggerType,
  WorkflowWriteBody,
} from "@/types";
import {
  basicToCron,
  cronToBasic,
  DEFAULT_BASIC,
  describeBasic,
  filterToRules,
  rulesToFilter,
  type BasicSchedule,
  type FilterRule,
} from "./schedule";

export type ScheduleMode = "basic" | "advanced";

export type TriggerDraft = {
  type: WorkflowTriggerType;
  collection: string;
  field: string;
  to: string;
  from: string;
  scheduleMode: ScheduleMode;
  basic: BasicSchedule;
  cron: string;
  timezone: string;
  filterRules: FilterRule[];
  limit: string;
};

export type ActionDraft = {
  /** Canvas / React Flow node id */
  nodeId: string;
  /** Server action id when editing an existing workflow */
  serverId?: string;
  type: WorkflowActionType;
  toEmail: string;
  subject: string;
  body: string;
  cc: string;
  pdfTemplate: string;
  pdfFilename: string;
};

export type WorkflowDraft = {
  name: string;
  active: boolean;
  trigger: TriggerDraft;
  actions: ActionDraft[];
};

export const TRIGGER_NODE_ID = "trigger";

export function newActionDraft(
  type: WorkflowActionType = "email_with_pdf",
): ActionDraft {
  return {
    nodeId: crypto.randomUUID(),
    type,
    toEmail: "",
    subject: "",
    body: "",
    cc: "",
    pdfTemplate: "",
    pdfFilename: "",
  };
}

export function emptyDraft(): WorkflowDraft {
  return {
    name: "",
    active: true,
    trigger: {
      type: "field_changed",
      collection: "",
      field: "",
      to: "",
      from: "",
      scheduleMode: "basic",
      basic: { ...DEFAULT_BASIC },
      cron: basicToCron(DEFAULT_BASIC),
      timezone: "UTC",
      filterRules: [],
      limit: "50",
    },
    actions: [newActionDraft()],
  };
}

export function workflowToDraft(wf: Workflow): WorkflowDraft {
  const cfg = wf.trigger.config ?? {};
  const cron = String(cfg.cron ?? basicToCron(DEFAULT_BASIC));
  const parsed = cronToBasic(cron);

  const actions =
    wf.actions?.length > 0
      ? [...wf.actions]
          .sort((a, b) => a.sort - b.sort)
          .map((a) => {
            const ac = (a.config ?? {}) as Partial<EmailWithPdfActionConfig> &
              Record<string, unknown>;
            const pdf =
              ac.pdf && typeof ac.pdf === "object"
                ? (ac.pdf as { template?: string; filename?: string })
                : {};
            return {
              nodeId: a.id || crypto.randomUUID(),
              serverId: a.id,
              type: a.type,
              toEmail: String(ac.to ?? ""),
              subject: String(ac.subject ?? ""),
              body: String(ac.body ?? ""),
              cc: String(ac.cc ?? ""),
              pdfTemplate: String(pdf.template ?? ""),
              pdfFilename: String(pdf.filename ?? ""),
            } satisfies ActionDraft;
          })
      : [newActionDraft()];

  return {
    name: wf.name,
    active: wf.active,
    trigger: {
      type: wf.trigger.type,
      collection: String(cfg.collection ?? ""),
      field: String(cfg.field ?? ""),
      to: cfg.to != null ? String(cfg.to) : "",
      from: cfg.from != null ? String(cfg.from) : "",
      scheduleMode: parsed ? "basic" : "advanced",
      basic: parsed ?? { ...DEFAULT_BASIC },
      cron,
      timezone: String(cfg.timezone ?? "UTC"),
      filterRules: filterToRules(cfg.filter),
      limit: cfg.limit != null ? String(cfg.limit) : "50",
    },
    actions,
  };
}

export function draftToBody(draft: WorkflowDraft): WorkflowWriteBody {
  const t = draft.trigger;
  let triggerConfig: Record<string, unknown>;

  if (t.type === "field_changed") {
    const cfg: FieldChangedTriggerConfig = {
      collection: t.collection.trim(),
      field: t.field.trim(),
      to: t.to,
      op: "eq",
    };
    if (t.from.trim()) cfg.from = t.from.trim();
    triggerConfig = cfg as unknown as Record<string, unknown>;
  } else {
    const cron =
      t.scheduleMode === "basic" ? basicToCron(t.basic) : t.cron.trim();
    const cfg: ScheduleTriggerConfig = {
      cron,
      collection: t.collection.trim(),
      timezone: t.timezone.trim() || "UTC",
    };
    const lim = Number(t.limit);
    if (Number.isFinite(lim) && lim > 0) cfg.limit = Math.min(200, lim);
    const filter = rulesToFilter(t.filterRules);
    if (filter) cfg.filter = filter;
    triggerConfig = cfg as unknown as Record<string, unknown>;
  }

  if (draft.actions.length === 0) {
    throw new Error("Add at least one action");
  }

  const actions = draft.actions.map((a) => {
    const emailCfg: EmailWithPdfActionConfig = {
      to: a.toEmail.trim(),
      subject: a.subject.trim(),
      pdf: { template: a.pdfTemplate.trim() },
    };
    if (a.body.trim()) emailCfg.body = a.body;
    if (a.cc.trim()) emailCfg.cc = a.cc.trim();
    if (a.pdfFilename.trim()) emailCfg.pdf.filename = a.pdfFilename.trim();

    const out: {
      id?: string;
      type: WorkflowActionType;
      config: Record<string, unknown>;
    } = {
      type: a.type,
      config: emailCfg as unknown as Record<string, unknown>,
    };
    if (a.serverId) out.id = a.serverId;
    return out;
  });

  return {
    name: draft.name.trim(),
    active: draft.active,
    trigger: { type: t.type, config: triggerConfig },
    actions,
  };
}

export function triggerSummary(t: TriggerDraft): string {
  if (!t.collection) return "Choose a collection";
  if (t.type === "field_changed") {
    if (!t.field) return `${t.collection} · pick a field`;
    return `${t.collection}.${t.field} → ${t.to || "…"}`;
  }
  if (t.scheduleMode === "basic") {
    return `${t.collection} · ${describeBasic(t.basic)}`;
  }
  return `${t.collection} · ${t.cron}`;
}

export function actionSummary(a: ActionDraft): string {
  if (a.type === "email_with_pdf") {
    return a.toEmail || a.subject || "Configure email + PDF";
  }
  return a.type;
}
