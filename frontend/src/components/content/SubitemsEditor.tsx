/**
 * SubitemsEditor.tsx — generic editor for a collection's child (one-to-many)
 * relation, e.g. a page's `pages_blocks`. Schema-driven and collection-agnostic:
 * it reads the child collection's fields and renders a sortable list of rows,
 * each edited with the same FieldGroup used for top-level items.
 *
 * State model (uncontrolled after mount): seeded once from `initialRows`, it owns
 * the rows + their drag keys and reports the plain row array via `onChange`. Mount
 * it only when `initialRows` is ready (the edit panel gates on the deep fetch).
 * The FK column is set by the backend and hidden here; if the child has a `sort`
 * field it is managed automatically from row order.
 */
import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { motion, AnimatePresence } from "framer-motion";
import {
  DndContext,
  closestCenter,
  PointerSensor,
  KeyboardSensor,
  useSensor,
  useSensors,
  type DragEndEvent,
} from "@dnd-kit/core";
import {
  SortableContext,
  sortableKeyboardCoordinates,
  verticalListSortingStrategy,
  useSortable,
  arrayMove,
} from "@dnd-kit/sortable";
import { CSS } from "@dnd-kit/utilities";
import { GripVertical, Plus, Trash2, ChevronDown } from "lucide-react";
import { get } from "@/lib/api";
import { FieldGroup } from "@/components/content/fields";
import { allFields } from "@/lib/collection";
import type { ChildRelation } from "@/lib/relational";
import type { Collection, Field } from "@/types";

const SORT_FIELD = "sort";
const genKey = () =>
  (globalThis.crypto?.randomUUID?.() ?? `k_${Math.random().toString(36).slice(2)}`);

type Row = { key: string; data: Record<string, unknown> };

export function SubitemsEditor({
  child,
  initialRows,
  onChange,
}: {
  child: ChildRelation;
  initialRows: Record<string, unknown>[];
  onChange: (rows: Record<string, unknown>[]) => void;
}) {
  const { data: schema, isLoading } = useQuery({
    queryKey: ["collection", child.childCollection],
    queryFn: () => get<Collection>(`/collections/${child.childCollection}`),
  });

  const fields = allFields(schema);
  const hasSort = fields.some((f) => f.name === SORT_FIELD);
  // Editable fields: skip system columns, the FK (set by the server) and the
  // managed sort column.
  const editableFields: Field[] = fields.filter(
    (f) => !f.is_system && f.name !== child.fkColumn && f.name !== SORT_FIELD,
  );

  const [rows, setRows] = useState<Row[]>(() =>
    initialRows.map((data) => ({ key: String(data.id ?? genKey()), data })),
  );
  const [openKey, setOpenKey] = useState<string | null>(null);

  const sensors = useSensors(
    useSensor(PointerSensor, { activationConstraint: { distance: 4 } }),
    useSensor(KeyboardSensor, { coordinateGetter: sortableKeyboardCoordinates }),
  );

  // Commit new state: persist locally and emit the plain rows, stamping `sort`
  // from order when the child supports it.
  function commit(next: Row[]) {
    setRows(next);
    onChange(
      next.map((r, i) => (hasSort ? { ...r.data, [SORT_FIELD]: i } : r.data)),
    );
  }

  function addRow() {
    const row: Row = { key: genKey(), data: {} };
    commit([...rows, row]);
    setOpenKey(row.key);
  }

  function removeRow(key: string) {
    commit(rows.filter((r) => r.key !== key));
  }

  function editField(key: string, name: string, value: unknown) {
    commit(
      rows.map((r) =>
        r.key === key ? { ...r, data: { ...r.data, [name]: value } } : r,
      ),
    );
  }

  function onDragEnd(e: DragEndEvent) {
    const { active, over } = e;
    if (!over || active.id === over.id) return;
    const from = rows.findIndex((r) => r.key === active.id);
    const to = rows.findIndex((r) => r.key === over.id);
    if (from < 0 || to < 0) return;
    commit(arrayMove(rows, from, to));
  }

  function rowSummary(data: Record<string, unknown>, index: number): string {
    for (const f of editableFields) {
      const v = data[f.name];
      if (typeof v === "string" && v.trim()) return v;
      if (typeof v === "number") return String(v);
    }
    return `${child.childCollection} #${index + 1}`;
  }

  return (
    <div className="rounded-xl border border-slate-200 dark:border-white/10 bg-slate-50/50 dark:bg-white/[0.02]">
      <div className="flex items-center justify-between px-4 py-3">
        <div>
          <p className="text-xs font-semibold uppercase tracking-wide text-slate-500 dark:text-slate-400">
            {schema?.display_name ?? child.childCollection}
          </p>
          <p className="text-[11px] text-slate-400">
            {rows.length} item{rows.length !== 1 ? "s" : ""}
            {child.owned ? "" : " · linked"}
          </p>
        </div>
        <button
          type="button"
          onClick={addRow}
          disabled={isLoading}
          className="flex items-center gap-1 rounded-lg border border-slate-200 dark:border-white/10 bg-white dark:bg-white/5 px-2.5 py-1.5 text-xs font-medium text-slate-600 dark:text-slate-300 transition hover:bg-brand-50 dark:hover:bg-brand-500/15 hover:text-brand-700 dark:hover:text-brand-300 disabled:opacity-50"
        >
          <Plus size={13} />
          Add
        </button>
      </div>

      {isLoading ? (
        <div className="px-4 pb-4">
          <div className="h-9 animate-pulse rounded-lg bg-slate-100 dark:bg-white/5" />
        </div>
      ) : rows.length === 0 ? (
        <p className="px-4 pb-4 text-xs text-slate-400">No items yet.</p>
      ) : (
        <div className="space-y-2 px-3 pb-3">
          <DndContext sensors={sensors} collisionDetection={closestCenter} onDragEnd={onDragEnd}>
            <SortableContext items={rows.map((r) => r.key)} strategy={verticalListSortingStrategy}>
              {rows.map((row, i) => (
                <SortableRow
                  key={row.key}
                  id={row.key}
                  title={rowSummary(row.data, i)}
                  open={openKey === row.key}
                  onToggle={() => setOpenKey(openKey === row.key ? null : row.key)}
                  onRemove={() => removeRow(row.key)}
                >
                  <FieldGroup
                    fields={editableFields}
                    values={row.data}
                    fieldErrors={{}}
                    onChange={(name, v) => editField(row.key, name, v)}
                  />
                </SortableRow>
              ))}
            </SortableContext>
          </DndContext>
        </div>
      )}
    </div>
  );
}

function SortableRow({
  id,
  title,
  open,
  onToggle,
  onRemove,
  children,
}: {
  id: string;
  title: string;
  open: boolean;
  onToggle: () => void;
  onRemove: () => void;
  children: React.ReactNode;
}) {
  const { attributes, listeners, setNodeRef, transform, transition, isDragging } =
    useSortable({ id });

  const style = {
    transform: CSS.Transform.toString(transform),
    transition,
    zIndex: isDragging ? 10 : undefined,
  };

  return (
    <div
      ref={setNodeRef}
      style={style}
      className={`rounded-lg border border-slate-200 dark:border-white/10 bg-white dark:bg-slate-900/60 ${
        isDragging ? "shadow-lg" : "shadow-card"
      }`}
    >
      <div className="flex items-center gap-2 px-2 py-2">
        <button
          type="button"
          className="flex h-7 w-7 shrink-0 cursor-grab items-center justify-center rounded-md text-slate-400 hover:bg-slate-100 dark:hover:bg-white/5 active:cursor-grabbing"
          aria-label="Drag to reorder"
          {...attributes}
          {...listeners}
        >
          <GripVertical size={14} />
        </button>
        <button
          type="button"
          onClick={onToggle}
          className="flex min-w-0 flex-1 items-center gap-2 text-left"
        >
          <ChevronDown
            size={14}
            className={`shrink-0 text-slate-400 transition-transform ${open ? "" : "-rotate-90"}`}
          />
          <span className="truncate text-sm font-medium text-slate-700 dark:text-slate-200">
            {title}
          </span>
        </button>
        <button
          type="button"
          onClick={onRemove}
          className="flex h-7 w-7 shrink-0 items-center justify-center rounded-md text-slate-400 transition hover:bg-red-50 dark:hover:bg-red-500/10 hover:text-red-600 dark:hover:text-red-400"
          aria-label="Remove"
        >
          <Trash2 size={13} />
        </button>
      </div>

      <AnimatePresence initial={false}>
        {open && (
          <motion.div
            initial={{ opacity: 0, height: 0 }}
            animate={{ opacity: 1, height: "auto" }}
            exit={{ opacity: 0, height: 0 }}
            transition={{ duration: 0.18 }}
            className="overflow-hidden"
          >
            <div className="border-t border-slate-100 dark:border-white/10 px-3 py-3">
              {children}
            </div>
          </motion.div>
        )}
      </AnimatePresence>
    </div>
  );
}
