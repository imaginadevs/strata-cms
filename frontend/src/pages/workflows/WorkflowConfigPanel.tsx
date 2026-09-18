/**
 * Right-hand config panel for the selected canvas node.
 */
import { useMemo } from "react";
import { useQuery } from "@tanstack/react-query";
import * as Switch from "@radix-ui/react-switch";
import { Plus, Trash2, X } from "lucide-react";
import { get } from "@/lib/api";
import { allFields } from "@/lib/collection";
import { cn } from "@/lib/utils";
import { Button } from "@/components/ui/Button";
import type {
  Collection,
  Field,
  PaginatedResponse,
  WorkflowCatalog,
  WorkflowTriggerType,
} from "@/types";
import {
  basicToCron,
  cronToBasic,
  DEFAULT_BASIC,
  describeBasic,
  listTimezones,
  WEEKDAYS,
  type BasicSchedule,
  type FilterRule,
  type ScheduleFrequency,
} from "./schedule";
import type {
  ActionDraft,
  ScheduleMode,
  TriggerDraft,
  WorkflowDraft,
} from "./model";
import { TRIGGER_NODE_ID } from "./model";

function FilterBuilder({
  fields,
  rules,
  onChange,
  disabled,
}: {
  fields: Field[];
  rules: FilterRule[];
  onChange: (rules: FilterRule[]) => void;
  disabled?: boolean;
}) {
  function update(id: string, patch: Partial<FilterRule>) {
    onChange(rules.map((r) => (r.id === id ? { ...r, ...patch } : r)));
  }

  return (
    <div className="space-y-2">
      <div className="flex items-center justify-between gap-2">
        <div>
          <p className="text-xs font-medium text-slate-700 dark:text-slate-200">
            Only include items where
          </p>
          <p className="text-[11px] text-slate-400">
            Equality match · one run per item each tick
          </p>
        </div>
        <Button
          type="button"
          size="sm"
          variant="secondary"
          disabled={disabled}
          onClick={() =>
            onChange([
              ...rules,
              {
                id: crypto.randomUUID(),
                field: fields[0]?.name ?? "",
                value: "",
              },
            ])
          }
        >
          <Plus size={14} />
          Condition
        </Button>
      </div>
      {rules.length === 0 ? (
        <p className="rounded-xl border border-dashed border-slate-200 px-3 py-3 text-center text-xs text-slate-400 dark:border-white/10">
          No filter — all items (up to max) are processed.
        </p>
      ) : (
        <ul className="space-y-2">
          {rules.map((rule) => (
            <li
              key={rule.id}
              className="flex flex-wrap items-center gap-2 rounded-xl border border-slate-100 bg-slate-50/60 p-2 dark:border-white/10 dark:bg-white/[0.03]"
            >
              <select
                className="input h-9 min-w-[7rem] flex-1 text-xs"
                value={rule.field}
                disabled={disabled}
                onChange={(e) => update(rule.id, { field: e.target.value })}
              >
                <option value="">Field…</option>
                {fields.map((f) => (
                  <option key={f.id} value={f.name}>
                    {f.display_name || f.name}
                  </option>
                ))}
              </select>
              <span className="text-xs text-slate-400">equals</span>
              <input
                className="input h-9 min-w-[7rem] flex-1 text-xs"
                value={rule.value}
                disabled={disabled}
                onChange={(e) => update(rule.id, { value: e.target.value })}
                placeholder="value"
              />
              <button
                type="button"
                className="rounded-lg p-2 text-slate-400 hover:bg-red-50 hover:text-red-500 dark:hover:bg-red-500/10"
                onClick={() => onChange(rules.filter((r) => r.id !== rule.id))}
                aria-label="Remove"
              >
                <Trash2 size={14} />
              </button>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}

function ScheduleBasicBuilder({
  value,
  onChange,
}: {
  value: BasicSchedule;
  onChange: (next: BasicSchedule) => void;
}) {
  const freqs: { id: ScheduleFrequency; label: string }[] = [
    { id: "hourly", label: "Hourly" },
    { id: "daily", label: "Daily" },
    { id: "weekly", label: "Weekly" },
    { id: "monthly", label: "Monthly" },
  ];

  function toggleDay(day: number) {
    const has = value.weekdays.includes(day);
    const weekdays = has
      ? value.weekdays.filter((d) => d !== day)
      : [...value.weekdays, day].sort((a, b) => a - b);
    onChange({ ...value, weekdays: weekdays.length ? weekdays : [day] });
  }

  return (
    <div className="space-y-3">
      <div className="flex flex-wrap gap-1.5">
        {freqs.map((f) => (
          <button
            key={f.id}
            type="button"
            onClick={() => onChange({ ...value, frequency: f.id })}
            className={cn(
              "rounded-lg px-2.5 py-1.5 text-xs font-medium transition",
              value.frequency === f.id
                ? "bg-brand-50 text-brand-800 dark:bg-brand-500/15 dark:text-brand-200"
                : "bg-slate-100 text-slate-600 dark:bg-white/5 dark:text-slate-300",
            )}
          >
            {f.label}
          </button>
        ))}
      </div>

      {value.frequency === "hourly" ? (
        <div>
          <label className="mb-1.5 block text-xs font-medium text-slate-700 dark:text-slate-200">
            At minute
          </label>
          <input
            type="number"
            min={0}
            max={59}
            className="input w-24"
            value={value.minute}
            onChange={(e) =>
              onChange({ ...value, minute: Number(e.target.value) })
            }
          />
        </div>
      ) : (
        <div className="grid grid-cols-2 gap-2">
          <div>
            <label className="mb-1.5 block text-xs font-medium text-slate-700 dark:text-slate-200">
              Hour
            </label>
            <input
              type="number"
              min={0}
              max={23}
              className="input"
              value={value.hour}
              onChange={(e) =>
                onChange({ ...value, hour: Number(e.target.value) })
              }
            />
          </div>
          <div>
            <label className="mb-1.5 block text-xs font-medium text-slate-700 dark:text-slate-200">
              Minute
            </label>
            <input
              type="number"
              min={0}
              max={59}
              className="input"
              value={value.minute}
              onChange={(e) =>
                onChange({ ...value, minute: Number(e.target.value) })
              }
            />
          </div>
        </div>
      )}

      {value.frequency === "weekly" && (
        <div className="flex flex-wrap gap-1.5">
          {WEEKDAYS.map((d) => {
            const on = value.weekdays.includes(d.value);
            return (
              <button
                key={d.value}
                type="button"
                title={d.label}
                onClick={() => toggleDay(d.value)}
                className={cn(
                  "h-8 min-w-8 rounded-lg px-2 text-xs font-semibold",
                  on
                    ? "bg-brand-600 text-white"
                    : "bg-slate-100 text-slate-500 dark:bg-white/5 dark:text-slate-400",
                )}
              >
                {d.short}
              </button>
            );
          })}
        </div>
      )}

      {value.frequency === "monthly" && (
        <div>
          <label className="mb-1.5 block text-xs font-medium text-slate-700 dark:text-slate-200">
            Day of month
          </label>
          <input
            type="number"
            min={1}
            max={31}
            className="input w-24"
            value={value.monthDay}
            onChange={(e) =>
              onChange({ ...value, monthDay: Number(e.target.value) })
            }
          />
        </div>
      )}

      <p className="rounded-lg bg-slate-50 px-2.5 py-2 text-[11px] text-slate-600 dark:bg-white/5 dark:text-slate-300">
        {describeBasic(value)}{" "}
        <span className="font-mono text-slate-400">({basicToCron(value)})</span>
      </p>
    </div>
  );
}

function CollectionSelect({
  value,
  onChange,
}: {
  value: string;
  onChange: (name: string) => void;
}) {
  const { data } = useQuery({
    queryKey: ["collections", { hidden: true }],
    queryFn: () =>
      get<PaginatedResponse<Collection>>("/collections?hidden=true"),
  });
  const collections = [...(data?.data ?? [])].sort((a, b) =>
    a.display_name.localeCompare(b.display_name),
  );

  return (
    <select
      className="input"
      value={value}
      onChange={(e) => onChange(e.target.value)}
    >
      <option value="">Select collection…</option>
      {collections.map((c) => (
        <option key={c.id} value={c.name}>
          {c.display_name} ({c.name})
        </option>
      ))}
    </select>
  );
}

function useCollectionFields(collection: string) {
  const { data } = useQuery({
    queryKey: ["collection", collection],
    queryFn: () => get<Collection>(`/collections/${collection}`),
    enabled: !!collection,
  });
  return allFields(data);
}

function TriggerConfig({
  trigger,
  catalog,
  onChange,
}: {
  trigger: TriggerDraft;
  catalog?: WorkflowCatalog;
  onChange: (next: TriggerDraft) => void;
}) {
  const fields = useCollectionFields(trigger.collection);
  const triggers = catalog?.triggers ?? [];
  const timezones = useMemo(() => {
    const zones = listTimezones();
    const current = trigger.timezone || "UTC";
    return zones.includes(current) ? zones : [current, ...zones];
  }, [trigger.timezone]);

  function patch(p: Partial<TriggerDraft>) {
    onChange({ ...trigger, ...p });
  }

  return (
    <div className="space-y-4">
      <div>
        <label className="mb-1.5 block text-xs font-medium text-slate-700 dark:text-slate-200">
          Trigger type
        </label>
        <div className="grid gap-1.5">
          {(triggers.length
            ? triggers
            : [
                { type: "field_changed", name: "Field changed", description: "" },
                { type: "schedule", name: "Schedule", description: "" },
              ]
          ).map((t) => (
            <button
              key={t.type}
              type="button"
              onClick={() => patch({ type: t.type as WorkflowTriggerType })}
              className={cn(
                "rounded-xl border px-3 py-2 text-left text-xs transition",
                trigger.type === t.type
                  ? "border-brand-300 bg-brand-50 dark:border-brand-500/40 dark:bg-brand-500/10"
                  : "border-slate-100 dark:border-white/10",
              )}
            >
              <span className="font-semibold text-slate-800 dark:text-slate-100">
                {t.name}
              </span>
            </button>
          ))}
        </div>
      </div>

      <div>
        <label className="mb-1.5 block text-xs font-medium text-slate-700 dark:text-slate-200">
          Collection
        </label>
        <CollectionSelect
          value={trigger.collection}
          onChange={(collection) =>
            patch({ collection, field: "", filterRules: [] })
          }
        />
      </div>

      {trigger.type === "field_changed" ? (
        <>
          <div>
            <label className="mb-1.5 block text-xs font-medium text-slate-700 dark:text-slate-200">
              Field
            </label>
            <select
              className="input"
              value={trigger.field}
              disabled={!trigger.collection}
              onChange={(e) => patch({ field: e.target.value })}
            >
              <option value="">Select field…</option>
              {fields.map((f) => (
                <option key={f.id} value={f.name}>
                  {f.display_name || f.name}
                </option>
              ))}
            </select>
          </div>
          <div>
            <label className="mb-1.5 block text-xs font-medium text-slate-700 dark:text-slate-200">
              Changes to
            </label>
            <input
              className="input"
              value={trigger.to}
              onChange={(e) => patch({ to: e.target.value })}
              placeholder="approved"
            />
          </div>
          <div>
            <label className="mb-1.5 block text-xs font-medium text-slate-700 dark:text-slate-200">
              From (optional)
            </label>
            <input
              className="input"
              value={trigger.from}
              onChange={(e) => patch({ from: e.target.value })}
              placeholder="draft"
            />
          </div>
        </>
      ) : (
        <>
          <div className="flex items-center justify-between">
            <p className="text-xs font-medium text-slate-700 dark:text-slate-200">
              Schedule
            </p>
            <div className="flex rounded-lg border border-slate-200 p-0.5 dark:border-white/10">
              {(["basic", "advanced"] as ScheduleMode[]).map((mode) => (
                <button
                  key={mode}
                  type="button"
                  onClick={() => {
                    if (mode === "basic") {
                      patch({
                        scheduleMode: mode,
                        basic: cronToBasic(trigger.cron) ?? {
                          ...DEFAULT_BASIC,
                        },
                      });
                    } else {
                      patch({
                        scheduleMode: mode,
                        cron: basicToCron(trigger.basic),
                      });
                    }
                  }}
                  className={cn(
                    "rounded-md px-2 py-1 text-[11px] font-semibold capitalize",
                    trigger.scheduleMode === mode
                      ? "bg-slate-900 text-white dark:bg-white dark:text-slate-900"
                      : "text-slate-500",
                  )}
                >
                  {mode}
                </button>
              ))}
            </div>
          </div>

          {trigger.scheduleMode === "basic" ? (
            <ScheduleBasicBuilder
              value={trigger.basic}
              onChange={(basic) =>
                patch({ basic, cron: basicToCron(basic) })
              }
            />
          ) : (
            <div>
              <label className="mb-1.5 block text-xs font-medium text-slate-700 dark:text-slate-200">
                Cron
              </label>
              <input
                className="input font-mono text-xs"
                value={trigger.cron}
                onChange={(e) => patch({ cron: e.target.value })}
              />
            </div>
          )}

          <div>
            <label className="mb-1.5 block text-xs font-medium text-slate-700 dark:text-slate-200">
              Timezone
            </label>
            <select
              className="input"
              value={trigger.timezone || "UTC"}
              onChange={(e) => patch({ timezone: e.target.value })}
            >
              {timezones.map((tz) => (
                <option key={tz} value={tz}>
                  {tz.replace(/_/g, " ")}
                </option>
              ))}
            </select>
          </div>

          <FilterBuilder
            fields={fields}
            rules={trigger.filterRules}
            disabled={!trigger.collection}
            onChange={(filterRules) => patch({ filterRules })}
          />

          <div>
            <label className="mb-1.5 block text-xs font-medium text-slate-700 dark:text-slate-200">
              Max items per run
            </label>
            <input
              type="number"
              min={1}
              max={200}
              className="input w-28"
              value={trigger.limit}
              onChange={(e) => patch({ limit: e.target.value })}
            />
            <p className="mt-1 text-[11px] text-slate-400">
              Cap matching items each tick (default 50, max 200).
            </p>
          </div>
        </>
      )}
    </div>
  );
}

function ActionConfig({
  action,
  onChange,
  onDelete,
  canDelete,
}: {
  action: ActionDraft;
  onChange: (next: ActionDraft) => void;
  onDelete: () => void;
  canDelete: boolean;
}) {
  function patch(p: Partial<ActionDraft>) {
    onChange({ ...action, ...p });
  }

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <p className="text-xs font-semibold uppercase tracking-wide text-slate-400">
          Email with PDF
        </p>
        {canDelete && (
          <Button type="button" size="sm" variant="ghost" onClick={onDelete}>
            <Trash2 size={14} />
            Remove
          </Button>
        )}
      </div>
      <div>
        <label className="mb-1.5 block text-xs font-medium text-slate-700 dark:text-slate-200">
          To
        </label>
        <input
          className="input font-mono text-xs"
          value={action.toEmail}
          onChange={(e) => patch({ toEmail: e.target.value })}
          placeholder="{{item.customer_email}}"
        />
      </div>
      <div>
        <label className="mb-1.5 block text-xs font-medium text-slate-700 dark:text-slate-200">
          Cc
        </label>
        <input
          className="input font-mono text-xs"
          value={action.cc}
          onChange={(e) => patch({ cc: e.target.value })}
        />
      </div>
      <div>
        <label className="mb-1.5 block text-xs font-medium text-slate-700 dark:text-slate-200">
          Subject
        </label>
        <input
          className="input"
          value={action.subject}
          onChange={(e) => patch({ subject: e.target.value })}
        />
      </div>
      <div>
        <label className="mb-1.5 block text-xs font-medium text-slate-700 dark:text-slate-200">
          Body
        </label>
        <textarea
          className="input min-h-[88px]"
          value={action.body}
          onChange={(e) => patch({ body: e.target.value })}
        />
      </div>
      <div className="grid grid-cols-1 gap-3">
        <div>
          <label className="mb-1.5 block text-xs font-medium text-slate-700 dark:text-slate-200">
            PDF template
          </label>
          <input
            className="input font-mono text-xs"
            value={action.pdfTemplate}
            onChange={(e) => patch({ pdfTemplate: e.target.value })}
            placeholder="order_summary"
          />
        </div>
        <div>
          <label className="mb-1.5 block text-xs font-medium text-slate-700 dark:text-slate-200">
            PDF filename
          </label>
          <input
            className="input font-mono text-xs"
            value={action.pdfFilename}
            onChange={(e) => patch({ pdfFilename: e.target.value })}
            placeholder="order-{{item.id}}.pdf"
          />
        </div>
      </div>
    </div>
  );
}

export function WorkflowConfigPanel({
  draft,
  selectedId,
  catalog,
  onClose,
  onChangeDraft,
}: {
  draft: WorkflowDraft;
  selectedId: string | null;
  catalog?: WorkflowCatalog;
  onClose: () => void;
  onChangeDraft: (next: WorkflowDraft) => void;
}) {
  if (!selectedId) return null;

  const isTrigger = selectedId === TRIGGER_NODE_ID;
  const action = draft.actions.find((a) => a.nodeId === selectedId);

  return (
    <aside className="flex h-full w-full max-w-md flex-col border-l border-slate-200 bg-white dark:border-white/10 dark:bg-slate-900">
      <div className="flex items-center justify-between border-b border-slate-100 px-4 py-3 dark:border-white/10">
        <div>
          <p className="text-sm font-semibold text-slate-900 dark:text-slate-100">
            {isTrigger ? "Trigger" : "Action"}
          </p>
          <p className="text-[11px] text-slate-400">
            Configure the selected node
          </p>
        </div>
        <button
          type="button"
          onClick={onClose}
          className="rounded-lg p-1.5 text-slate-400 hover:bg-slate-100 dark:hover:bg-white/5"
        >
          <X size={16} />
        </button>
      </div>
      <div className="flex-1 overflow-y-auto px-4 py-4">
        {isTrigger ? (
          <TriggerConfig
            trigger={draft.trigger}
            catalog={catalog}
            onChange={(trigger) => onChangeDraft({ ...draft, trigger })}
          />
        ) : action ? (
          <ActionConfig
            action={action}
            canDelete={draft.actions.length > 1}
            onChange={(next) =>
              onChangeDraft({
                ...draft,
                actions: draft.actions.map((a) =>
                  a.nodeId === next.nodeId ? next : a,
                ),
              })
            }
            onDelete={() =>
              onChangeDraft({
                ...draft,
                actions: draft.actions.filter((a) => a.nodeId !== action.nodeId),
              })
            }
          />
        ) : (
          <p className="text-sm text-slate-400">Node not found</p>
        )}
      </div>
    </aside>
  );
}

export function WorkflowMetaBar({
  draft,
  onChange,
}: {
  draft: WorkflowDraft;
  onChange: (next: WorkflowDraft) => void;
}) {
  return (
    <div className="flex flex-wrap items-center gap-3">
      <input
        className="input h-9 max-w-xs flex-1 text-sm font-medium"
        value={draft.name}
        onChange={(e) => onChange({ ...draft, name: e.target.value })}
        placeholder="Workflow name"
      />
      <div className="flex items-center gap-2 rounded-xl border border-slate-200 px-3 py-1.5 dark:border-white/10">
        <span className="text-xs text-slate-500">Active</span>
        <Switch.Root
          checked={draft.active}
          onCheckedChange={(active) => onChange({ ...draft, active })}
          className={cn(
            "relative h-5 w-9 rounded-full transition-colors",
            draft.active ? "bg-brand-600" : "bg-slate-200 dark:bg-white/15",
          )}
        >
          <Switch.Thumb
            className={cn(
              "block h-3.5 w-3.5 rounded-full bg-white shadow-sm transition-transform",
              draft.active ? "translate-x-[18px]" : "translate-x-[3px]",
            )}
          />
        </Switch.Root>
      </div>
    </div>
  );
}
