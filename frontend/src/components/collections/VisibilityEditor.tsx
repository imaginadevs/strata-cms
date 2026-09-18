/**
 * Recursive editor for section visibility Condition trees.
 * Persist the JSON as-is — root is and/or group; children are nested groups or leaves.
 */
import * as Select from "@radix-ui/react-select";
import { ChevronDown, Plus, Trash2 } from "lucide-react";
import { Button } from "@/components/ui/Button";
import type { CondOp, Condition, Field } from "@/types";

const OPS: { value: CondOp; label: string }[] = [
  { value: "eq", label: "equals" },
  { value: "neq", label: "not equals" },
  { value: "gt", label: "greater than" },
  { value: "gte", label: "≥" },
  { value: "lt", label: "less than" },
  { value: "lte", label: "≤" },
  { value: "in", label: "in list" },
  { value: "nin", label: "not in list" },
  { value: "empty", label: "is empty" },
  { value: "nempty", label: "is not empty" },
  { value: "contains", label: "contains" },
];

const NO_VALUE_OPS = new Set<CondOp>(["empty", "nempty"]);

function isGroup(c: Condition): c is { and: Condition[] } | { or: Condition[] } {
  return "and" in c || "or" in c;
}

function defaultLeaf(fields: Field[]): Condition {
  return {
    field: fields[0]?.name ?? "",
    op: "eq",
    value: "",
  };
}

const selectTrigger =
  "flex h-9 w-full items-center justify-between rounded-lg border border-slate-200 dark:border-white/10 bg-white dark:bg-white/5 px-2.5 text-xs text-slate-800 dark:text-slate-100 focus:outline-none focus:ring-2 focus:ring-brand-100 dark:focus:ring-brand-500/20 focus:border-brand-400 [&[data-placeholder]]:text-slate-400";
const selectItem =
  "cursor-pointer rounded-lg px-2.5 py-1.5 text-xs text-slate-700 dark:text-slate-200 outline-none data-[highlighted]:bg-brand-50 dark:data-[highlighted]:bg-brand-500/15 data-[highlighted]:text-brand-700 dark:data-[highlighted]:text-brand-300";

function MiniSelect({
  value,
  onChange,
  options,
  placeholder,
  className,
}: {
  value: string;
  onChange: (v: string) => void;
  options: { value: string; label: string }[];
  placeholder?: string;
  className?: string;
}) {
  return (
    <Select.Root value={value || undefined} onValueChange={onChange}>
      <Select.Trigger className={`${selectTrigger} ${className ?? ""}`}>
        <Select.Value placeholder={placeholder ?? "—"} />
        <Select.Icon>
          <ChevronDown size={12} className="text-slate-400" />
        </Select.Icon>
      </Select.Trigger>
      <Select.Portal>
        <Select.Content
          position="popper"
          sideOffset={4}
          className="z-[60] max-h-56 overflow-hidden rounded-xl border border-slate-100 dark:border-white/10 bg-white dark:bg-slate-800 p-1 shadow-lg"
        >
          <Select.Viewport>
            {options.map((o) => (
              <Select.Item key={o.value} value={o.value} className={selectItem}>
                <Select.ItemText>{o.label}</Select.ItemText>
              </Select.Item>
            ))}
          </Select.Viewport>
        </Select.Content>
      </Select.Portal>
    </Select.Root>
  );
}

function LeafEditor({
  condition,
  fields,
  onChange,
  onRemove,
}: {
  condition: Extract<Condition, { field: string }>;
  fields: Field[];
  onChange: (c: Condition) => void;
  onRemove?: () => void;
}) {
  const needsValue = !NO_VALUE_OPS.has(condition.op);
  const isList = condition.op === "in" || condition.op === "nin";
  const valueStr = isList
    ? (Array.isArray(condition.value) ? condition.value : []).map(String).join(", ")
    : condition.value == null
      ? ""
      : String(condition.value);

  return (
    <div className="flex flex-wrap items-start gap-2 rounded-xl border border-slate-100 dark:border-white/10 bg-white dark:bg-white/[0.03] p-2.5">
      <div className="min-w-[7.5rem] flex-1">
        <MiniSelect
          value={condition.field}
          onChange={(field) => onChange({ ...condition, field })}
          options={fields.map((f) => ({ value: f.name, label: f.display_name || f.name }))}
          placeholder="Field"
        />
      </div>
      <div className="w-[8.5rem]">
        <MiniSelect
          value={condition.op}
          onChange={(op) => {
            const next = op as CondOp;
            if (NO_VALUE_OPS.has(next)) {
              onChange({ field: condition.field, op: next });
            } else if (next === "in" || next === "nin") {
              onChange({
                field: condition.field,
                op: next,
                value: Array.isArray(condition.value) ? condition.value : [],
              });
            } else {
              onChange({
                field: condition.field,
                op: next,
                value: Array.isArray(condition.value) ? "" : (condition.value ?? ""),
              });
            }
          }}
          options={OPS}
        />
      </div>
      {needsValue && (
        <input
          className="input h-9 min-w-[8rem] flex-1 text-xs"
          placeholder={isList ? "a, b, c" : "Value"}
          value={valueStr}
          onChange={(e) => {
            const raw = e.target.value;
            if (isList) {
              const parts = raw
                .split(",")
                .map((s) => s.trim())
                .filter(Boolean);
              onChange({ ...condition, value: parts });
            } else {
              onChange({ ...condition, value: raw });
            }
          }}
        />
      )}
      {onRemove && (
        <button
          type="button"
          onClick={onRemove}
          className="mt-1 rounded-lg p-1.5 text-slate-300 transition hover:bg-red-50 dark:hover:bg-red-500/10 hover:text-red-500"
          title="Remove"
        >
          <Trash2 size={13} />
        </button>
      )}
    </div>
  );
}

function GroupEditor({
  condition,
  fields,
  onChange,
  onRemove,
  depth,
}: {
  condition: { and: Condition[] } | { or: Condition[] };
  fields: Field[];
  onChange: (c: Condition) => void;
  onRemove?: () => void;
  depth: number;
}) {
  const kind = "and" in condition ? ("and" as const) : ("or" as const);
  const children: Condition[] =
    "and" in condition ? condition.and : condition.or;

  function setChildren(next: Condition[]) {
    onChange(kind === "and" ? { and: next } : { or: next });
  }

  function updateChild(i: number, child: Condition) {
    setChildren(children.map((c, idx) => (idx === i ? child : c)));
  }

  function removeChild(i: number) {
    setChildren(children.filter((_, idx) => idx !== i));
  }

  return (
    <div
      className={`space-y-2 rounded-xl border p-3 ${
        depth === 0
          ? "border-slate-200 dark:border-white/10 bg-slate-50/80 dark:bg-white/[0.04]"
          : "border-dashed border-slate-200 dark:border-white/10 bg-white/60 dark:bg-white/[0.02]"
      }`}
    >
      <div className="flex flex-wrap items-center gap-2">
        <span className="text-[10px] font-semibold uppercase tracking-wide text-slate-400">
          Match
        </span>
        <div className="flex rounded-lg border border-slate-200 dark:border-white/10 p-0.5">
          {(["and", "or"] as const).map((k) => (
            <button
              key={k}
              type="button"
              onClick={() =>
                onChange(k === "and" ? { and: children } : { or: children })
              }
              className={`rounded-md px-2.5 py-1 text-[11px] font-semibold uppercase transition ${
                kind === k
                  ? "bg-brand-600 text-white"
                  : "text-slate-500 dark:text-slate-400 hover:bg-slate-100 dark:hover:bg-white/5"
              }`}
            >
              {k}
            </button>
          ))}
        </div>
        <div className="ml-auto flex items-center gap-1">
          <Button
            type="button"
            size="sm"
            variant="ghost"
            onClick={() => setChildren([...children, defaultLeaf(fields)])}
          >
            <Plus size={12} />
            Rule
          </Button>
          <Button
            type="button"
            size="sm"
            variant="ghost"
            onClick={() => setChildren([...children, { and: [defaultLeaf(fields)] }])}
          >
            <Plus size={12} />
            Group
          </Button>
          {onRemove && (
            <button
              type="button"
              onClick={onRemove}
              className="rounded-lg p-1.5 text-slate-300 transition hover:bg-red-50 dark:hover:bg-red-500/10 hover:text-red-500"
              title="Remove group"
            >
              <Trash2 size={13} />
            </button>
          )}
        </div>
      </div>

      {children.length === 0 ? (
        <p className="py-2 text-center text-xs text-slate-400">
          No rules — add a rule or nested group.
        </p>
      ) : (
        <div className="space-y-2">
          {children.map((child, i) => (
            <div key={i}>
              {i > 0 && (
                <p className="mb-1.5 pl-1 text-[10px] font-semibold uppercase tracking-wide text-slate-300 dark:text-slate-500">
                  {kind}
                </p>
              )}
              {isGroup(child) ? (
                <GroupEditor
                  condition={child}
                  fields={fields}
                  onChange={(c) => updateChild(i, c)}
                  onRemove={() => removeChild(i)}
                  depth={depth + 1}
                />
              ) : (
                <LeafEditor
                  condition={child}
                  fields={fields}
                  onChange={(c) => updateChild(i, c)}
                  onRemove={() => removeChild(i)}
                />
              )}
            </div>
          ))}
        </div>
      )}
    </div>
  );
}

export function VisibilityEditor({
  value,
  onChange,
  fields,
}: {
  value: Condition | null;
  onChange: (next: Condition | null) => void;
  fields: Field[];
}) {
  const enabled = value != null;

  return (
    <div className="space-y-3">
      <div className="flex items-start justify-between gap-3">
        <div>
          <p className="text-xs font-medium text-slate-700 dark:text-slate-200">Visibility</p>
          <p className="mt-0.5 text-[11px] text-slate-400">
            When off, the section is always shown. When on, show only if the rules match.
          </p>
        </div>
        <button
          type="button"
          role="switch"
          aria-checked={enabled}
          onClick={() =>
            onChange(
              enabled
                ? null
                : { and: [defaultLeaf(fields)] },
            )
          }
          className={`relative h-5 w-9 shrink-0 rounded-full transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500 ${
            enabled ? "bg-brand-600" : "bg-slate-200 dark:bg-white/15"
          }`}
        >
          <span
            className={`block h-3.5 w-3.5 rounded-full bg-white shadow-sm transition-transform ${
              enabled ? "translate-x-[18px]" : "translate-x-[3px]"
            }`}
          />
        </button>
      </div>

      {enabled && value && (
        fields.length === 0 ? (
          <p className="rounded-xl bg-amber-50 dark:bg-amber-500/10 px-3 py-2 text-xs text-amber-700 dark:text-amber-300">
            Add fields to this collection before configuring visibility rules.
          </p>
        ) : isGroup(value) ? (
          <GroupEditor
            condition={value}
            fields={fields}
            onChange={onChange}
            depth={0}
          />
        ) : (
          // Normalize a bare leaf into an and-group for editing.
          <GroupEditor
            condition={{ and: [value] }}
            fields={fields}
            onChange={onChange}
            depth={0}
          />
        )
      )}
    </div>
  );
}
