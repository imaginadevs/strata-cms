import { useState } from "react";
import { createPortal } from "react-dom";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { useNavigate, useParams } from "@tanstack/react-router";
import { useForm, Controller } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import * as Dialog from "@radix-ui/react-dialog";
import * as Switch from "@radix-ui/react-switch";
import {
  DndContext,
  closestCenter,
  PointerSensor,
  useSensor,
  useSensors,
  type DragEndEvent,
} from "@dnd-kit/core";
import {
  SortableContext,
  useSortable,
  verticalListSortingStrategy,
  arrayMove,
} from "@dnd-kit/sortable";
import { CSS } from "@dnd-kit/utilities";
import {
  Plus, Database, Lock, ChevronLeft, ChevronRight, Trash2, X, Pencil,
  Type, Hash, ToggleLeft, Calendar, FileText,
  List, Binary, Clock, Globe, Code, Upload, GripVertical,
  ArrowLeftRight, AlertTriangle, Eye, EyeOff, ArrowUpDown,
} from "lucide-react";
import { motion, AnimatePresence } from "framer-motion";

import { get, post, del, patch } from "@/lib/api";
import { Button } from "@/components/ui/Button";
import { allFields, mergeFieldsIntoSections, sortedSections } from "@/lib/collection";
import { EditSectionPanel } from "@/components/collections/EditSectionPanel";
import { AddSectionPanel } from "@/components/collections/AddSectionPanel";
import { SectionReorderPanel } from "@/components/collections/SectionReorderPanel";
import type { Collection, Field, FieldType, PaginatedResponse, Relation, RelationType, CollectionIndex, IndexType, AutoGenFunction, AutoGenTrigger, Section } from "@/types";

// ── API helpers ───────────────────────────────────────────────────────────────

const fetchCollections = () =>
  get<PaginatedResponse<Collection>>("/collections").then((r) => r.data ?? []);
const fetchCollection = (name: string) => get<Collection>(`/collections/${name}`);
const createCollection = (data: CreateCollectionInput) =>
  post<Collection>("/collections", data);
const deleteCollection = (name: string) => del(`/collections/${name}`);
const updateCollection = (name: string, data: Partial<Pick<Collection, "is_hidden">>) =>
  patch<Collection>(`/collections/${name}`, data);
const createField = (coll: string, data: CreateFieldInput) =>
  post<Field>(`/collections/${coll}/fields`, data);
const deleteField = (coll: string, name: string) =>
  del(`/collections/${coll}/fields/${name}`);
const reorderFields = (coll: string, order: string[]) =>
  patch<Field[]>(`/collections/${coll}/fields/reorder`, { order });
const updateField = (coll: string, name: string, data: Partial<UpdateFieldInput>) =>
  patch<Field>(`/collections/${coll}/fields/${name}`, data);

// ── Zod schemas ───────────────────────────────────────────────────────────────

const SNAKE_RE = /^[a-z][a-z0-9_]*$/;

const createCollectionSchema = z.object({
  name: z
    .string()
    .min(1, "Name is required")
    .regex(SNAKE_RE, "Use lowercase letters, numbers and underscores. Must start with a letter."),
  display_name: z.string().min(1, "Display name is required"),
  type: z.enum(["base", "auth"] as const),
  color: z.string().optional(),
  note: z.string().optional(),
});

const createFieldSchema = z.object({
  name: z
    .string()
    .min(1, "Name is required")
    .regex(SNAKE_RE, "Use lowercase letters, numbers and underscores."),
  display_name: z.string().min(1, "Display name is required"),
  type: z.string().min(1, "Type is required") as z.ZodType<FieldType>,
  required:     z.boolean(),
  unique:       z.boolean(),
  hidden:       z.boolean(),
  quick_edit:   z.boolean(),
  show_in_table: z.boolean(),
  readonly:     z.boolean(),
  translatable: z.boolean(),
  section: z.string().optional(),
  options: z.record(z.unknown()).optional(),
});

type CreateCollectionInput = z.infer<typeof createCollectionSchema>;
type CreateFieldInput = z.infer<typeof createFieldSchema>;

interface UpdateFieldInput {
  display_name: string;
  required: boolean;
  unique: boolean;
  hidden: boolean;
  quick_edit: boolean;
  show_in_table: boolean;
  readonly: boolean;
  translatable: boolean;
  options?: Record<string, unknown>;
}

// ── Field type config ─────────────────────────────────────────────────────────

const FIELD_TYPES: { type: FieldType; label: string; Icon: React.ElementType; color: string }[] = [
  { type: "string",    label: "Text",      Icon: Type,       color: "text-blue-500 dark:text-blue-300 bg-blue-50 dark:bg-blue-500/10" },
  { type: "text",      label: "Long Text", Icon: FileText,   color: "text-blue-600 dark:text-blue-300 bg-blue-50 dark:bg-blue-500/10" },
  { type: "integer",   label: "Integer",   Icon: Hash,       color: "text-violet-500 dark:text-violet-300 bg-violet-50 dark:bg-violet-500/10" },
  { type: "float",     label: "Float",     Icon: Hash,       color: "text-violet-600 dark:text-violet-300 bg-violet-50 dark:bg-violet-500/10" },
  { type: "boolean",   label: "Boolean",   Icon: ToggleLeft, color: "text-green-500 dark:text-emerald-300 bg-green-50 dark:bg-emerald-500/10" },
  { type: "date",      label: "Date",      Icon: Calendar,   color: "text-orange-500 dark:text-orange-300 bg-orange-50 dark:bg-orange-500/10" },
  { type: "datetime",  label: "DateTime",  Icon: Clock,      color: "text-orange-600 dark:text-orange-300 bg-orange-50 dark:bg-orange-500/10" },
  { type: "json",      label: "JSON",      Icon: Code,       color: "text-slate-500 dark:text-slate-400 bg-slate-50 dark:bg-white/5" },
  { type: "uuid",      label: "UUID",      Icon: Binary,     color: "text-slate-600 dark:text-slate-300 bg-slate-50 dark:bg-white/5" },
  { type: "select",    label: "Select",    Icon: List,       color: "text-amber-500 dark:text-amber-300 bg-amber-50 dark:bg-amber-500/10" },
  { type: "file",      label: "File",      Icon: Upload,     color: "text-teal-500 dark:text-teal-300 bg-teal-50 dark:bg-teal-500/10" },
  { type: "ip",        label: "IP",        Icon: Globe,      color: "text-slate-500 dark:text-slate-400 bg-slate-50 dark:bg-white/5" },
  { type: "rich_text", label: "Rich Text", Icon: FileText,   color: "text-blue-700 dark:text-blue-300 bg-blue-50 dark:bg-blue-500/10" },
];

const PRESET_COLORS = [
  "#6366f1", "#8b5cf6", "#ec4899", "#f43f5e",
  "#f97316", "#eab308", "#22c55e", "#14b8a6",
  "#3b82f6", "#0ea5e9", "#64748b", "#1e293b",
];

// ── Main page (list) ──────────────────────────────────────────────────────────

export default function CollectionsPage() {
  return <CollectionList />;
}

// ── Detail page (routed at /collections/:name) ────────────────────────────────

export function CollectionDetailPage() {
  const navigate = useNavigate();
  const { name } = useParams({ strict: false }) as { name: string };
  return (
    <CollectionDetail
      name={name}
      onBack={() => navigate({ to: "/collections" })}
    />
  );
}

// ── Collection list ───────────────────────────────────────────────────────────

function CollectionList() {
  const navigate = useNavigate();
  const [filter, setFilter] = useState<"all" | "base" | "auth">("all");
  const [showCreate, setShowCreate] = useState(false);

  const { data: rawCollections, isLoading } = useQuery({
    queryKey: ["collections"],
    queryFn: fetchCollections,
  });
  const collections: Collection[] = Array.isArray(rawCollections) ? rawCollections : [];
  const visible = collections.filter((c) => filter === "all" || c.type === filter);

  return (
    <div className="space-y-5">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold text-slate-900 dark:text-slate-100">Collections</h1>
          <p className="mt-1 text-sm text-slate-500 dark:text-slate-400">
            {collections.length} data model{collections.length !== 1 ? "s" : ""}
          </p>
        </div>
        <Button onClick={() => setShowCreate(true)}>
          <Plus size={16} />
          New Collection
        </Button>
      </div>

      <div className="flex gap-2">
        {(["all", "base", "auth"] as const).map((t) => (
          <button
            key={t}
            onClick={() => setFilter(t)}
            className={`rounded-xl px-3 py-1.5 text-xs font-medium capitalize transition ${
              filter === t
                ? "bg-brand-600 text-white"
                : "bg-white dark:bg-white/5 text-slate-600 dark:text-slate-300 hover:bg-brand-50 dark:hover:bg-brand-500/15 hover:text-brand-700 dark:hover:text-brand-300 border border-slate-200 dark:border-white/10"
            }`}
          >
            {t}
          </button>
        ))}
      </div>

      {isLoading ? (
        <div className="rounded-2xl border border-slate-100 dark:border-white/10 bg-white dark:bg-slate-900 shadow-card overflow-hidden">
          <div className="divide-y divide-slate-50 dark:divide-white/10">
            {Array.from({ length: 5 }).map((_, i) => (
              <div key={i} className="flex items-center gap-4 px-5 py-4">
                <div className="h-8 w-8 animate-pulse rounded-lg bg-slate-100 dark:bg-white/10" />
                <div className="flex-1 space-y-1.5">
                  <div className="h-3.5 w-32 animate-pulse rounded bg-slate-100 dark:bg-white/10" />
                  <div className="h-3 w-20 animate-pulse rounded bg-slate-50 dark:bg-white/5" />
                </div>
              </div>
            ))}
          </div>
        </div>
      ) : visible.length === 0 ? (
        <div className="rounded-2xl border border-slate-100 dark:border-white/10 bg-white dark:bg-slate-900 py-16 text-center shadow-card">
          <div className="mx-auto mb-4 flex h-14 w-14 items-center justify-center rounded-2xl bg-brand-50 dark:bg-brand-500/15">
            <Database size={24} className="text-brand-400" />
          </div>
          <p className="font-medium text-slate-900 dark:text-slate-100">No collections yet</p>
          <p className="mt-1 text-sm text-slate-500 dark:text-slate-400">
            Create your first collection to start building your data model.
          </p>
        </div>
      ) : (
        <div className="rounded-2xl border border-slate-100 dark:border-white/10 bg-white dark:bg-slate-900 shadow-card overflow-hidden">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-slate-100 dark:border-white/10 text-left text-xs font-medium text-slate-400">
                <th className="px-5 py-3">Collection</th>
                <th className="px-5 py-3">Type</th>
                <th className="px-5 py-3">Fields</th>
                <th className="px-5 py-3 hidden md:table-cell">Note</th>
                <th className="w-8 px-5 py-3" />
              </tr>
            </thead>
            <tbody className="divide-y divide-slate-50 dark:divide-white/10">
              {visible.map((c) => {
                const iconColor = c.color ?? "#6366f1";
                const fieldCount = allFields(c).length;
                return (
                  <tr
                    key={c.id}
                    onClick={() => navigate({ to: "/collections/$name", params: { name: c.name } })}
                    className="group cursor-pointer transition hover:bg-slate-50/60 dark:hover:bg-white/5"
                  >
                    <td className="px-5 py-3.5">
                      <div className="flex items-center gap-3">
                        <div
                          className="flex h-8 w-8 shrink-0 items-center justify-center rounded-lg"
                          style={{ backgroundColor: `${iconColor}18` }}
                        >
                          {c.is_system
                            ? <Lock size={13} style={{ color: iconColor }} />
                            : <Database size={13} style={{ color: iconColor }} />}
                        </div>
                        <div>
                          <p className="font-medium text-slate-900 dark:text-slate-100 group-hover:text-brand-700 dark:group-hover:text-brand-300">
                            {c.display_name}
                          </p>
                          <p className="font-mono text-[11px] text-slate-400">{c.name}</p>
                        </div>
                      </div>
                    </td>
                    <td className="px-5 py-3.5">
                      <div className="flex flex-wrap gap-1">
                        <span className={`badge ${c.type === "auth" ? "badge-amber" : "badge-blue"}`}>
                          {c.type}
                        </span>
                        {c.is_system && <span className="badge badge-gray">system</span>}
                      </div>
                    </td>
                    <td className="px-5 py-3.5 text-slate-500 dark:text-slate-400">
                      {fieldCount > 0 ? fieldCount : <span className="text-slate-300">—</span>}
                    </td>
                    <td className="hidden px-5 py-3.5 text-slate-400 md:table-cell">
                      <span className="line-clamp-1 max-w-[280px]">{c.note || "—"}</span>
                    </td>
                    <td className="px-5 py-3.5 text-slate-300 group-hover:text-brand-400">
                      <ChevronRight size={15} />
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      )}

      <CreateCollectionDialog open={showCreate} onClose={() => setShowCreate(false)} />
    </div>
  );
}

// ── Collection detail (field editor) ─────────────────────────────────────────

function CollectionDetail({ name, onBack }: { name: string; onBack: () => void }) {
  const [addFieldSection, setAddFieldSection] = useState<string | null>(null);
  const [editingSection, setEditingSection] = useState<Section | null>(null);
  const [showCreateSection, setShowCreateSection] = useState(false);
  const [sortSectionsOpen, setSortSectionsOpen] = useState(false);
  const [confirmDelete, setConfirmDelete] = useState(false);
  const queryClient = useQueryClient();

  // All collections for the relation target picker (cached — no extra request)
  const { data: rawAllCollections = [] } = useQuery({
    queryKey: ["collections"],
    queryFn: fetchCollections,
  });
  const allCollections: Collection[] = Array.isArray(rawAllCollections) ? rawAllCollections : [];

  const { data: collection, isLoading } = useQuery({
    queryKey: ["collection", name],
    queryFn: () => fetchCollection(name),
  });

  const deleteMutation = useMutation({
    mutationFn: () => deleteCollection(name),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["collections"] });
      onBack();
    },
  });

  // Collection-level settings (e.g. is_hidden) — optimistic PATCH.
  const settingsMutation = useMutation({
    mutationFn: (data: Partial<Pick<Collection, "is_hidden">>) =>
      updateCollection(name, data),
    onMutate: async (data) => {
      await queryClient.cancelQueries({ queryKey: ["collection", name] });
      const prev = queryClient.getQueryData<Collection>(["collection", name]);
      queryClient.setQueryData<Collection>(["collection", name], (old) =>
        old ? { ...old, ...data } : old
      );
      return { prev };
    },
    onError: (_e, _data, ctx) => {
      if (ctx?.prev) queryClient.setQueryData(["collection", name], ctx.prev);
    },
    onSettled: () => {
      queryClient.invalidateQueries({ queryKey: ["collection", name] });
      queryClient.invalidateQueries({ queryKey: ["collections"] });
    },
  });

  // Optimistic per-section field order while a drag is pending
  const [localBySection, setLocalBySection] = useState<Record<string, Field[] | null>>({});

  const reorderMutation = useMutation({
    mutationFn: (order: string[]) => reorderFields(name, order),
    onSuccess: (updatedFields) => {
      queryClient.setQueryData<Collection>(["collection", name], (old) =>
        old ? mergeFieldsIntoSections(old, updatedFields) : old
      );
      setLocalBySection({});
    },
    onError: () => setLocalBySection({}),
  });

  const sensors = useSensors(
    useSensor(PointerSensor, { activationConstraint: { distance: 6 } })
  );

  if (isLoading || !collection) {
    return (
      <div className="flex items-center justify-center py-20">
        <div className="h-8 w-8 animate-spin rounded-full border-2 border-brand-200 border-t-brand-600" />
      </div>
    );
  }

  const iconColor = collection.color ?? "#6366f1";
  const sections = sortedSections(collection);
  const flatFields = allFields(collection);
  const userFields = flatFields.filter((f) => !f.is_system);
  const systemFields = flatFields.filter((f) => f.is_system);

  function fieldsForSection(section: Section): Field[] {
    const local = localBySection[section.name];
    if (local) return local;
    return [...(section.fields ?? [])].sort((a, b) => a.sort - b.sort);
  }

  function handleSectionDragEnd(section: Section, event: DragEndEvent) {
    if (reorderMutation.isPending) return;
    const { active, over } = event;
    if (!over || active.id === over.id) return;

    const current = fieldsForSection(section);
    const oldIndex = current.findIndex((f) => f.id === active.id);
    const newIndex = current.findIndex((f) => f.id === over.id);
    if (oldIndex === -1 || newIndex === -1) return;

    const reordered = arrayMove(current, oldIndex, newIndex);
    setLocalBySection((prev) => ({ ...prev, [section.name]: reordered }));

    const order: string[] = [];
    for (const s of sections) {
      const fields = s.name === section.name ? reordered : fieldsForSection(s);
      for (const f of fields) order.push(f.name);
    }
    reorderMutation.mutate(order);
  }

  return (
    <div className="space-y-5">
      {/* Header */}
      <div className="flex items-center gap-4">
        <button
          onClick={onBack}
          className="flex h-9 w-9 items-center justify-center rounded-xl border border-slate-200 dark:border-white/10 bg-white dark:bg-slate-900 text-slate-500 dark:text-slate-400 shadow-card transition hover:bg-brand-50 dark:hover:bg-brand-500/15 hover:text-brand-700 dark:hover:text-brand-300"
        >
          <ChevronLeft size={18} />
        </button>
        <div
          className="flex h-10 w-10 shrink-0 items-center justify-center rounded-xl"
          style={{ backgroundColor: `${iconColor}18` }}
        >
          {collection.is_system ? (
            <Lock size={16} style={{ color: iconColor }} />
          ) : (
            <Database size={16} style={{ color: iconColor }} />
          )}
        </div>
        <div className="flex-1">
          <div className="flex items-center gap-2">
            <h1 className="text-xl font-bold text-slate-900 dark:text-slate-100">{collection.display_name}</h1>
            <span className={`badge ${collection.type === "auth" ? "badge-amber" : "badge-blue"}`}>
              {collection.type}
            </span>
            {collection.is_system && <span className="badge badge-gray">system</span>}
          </div>
          <p className="mt-0.5 text-xs text-slate-400">{collection.name}</p>
        </div>
        {/* Delete only for non-system collections */}
        {!collection.is_system && (
          <Button
            variant="ghost"
            className="gap-1.5 text-red-500 dark:text-red-400 hover:bg-red-50 dark:hover:bg-red-500/10 hover:text-red-600 dark:hover:text-red-400"
            onClick={() => setConfirmDelete(true)}
          >
            <Trash2 size={15} />
            Delete
          </Button>
        )}
      </div>

      {collection.note && (
        <p className="rounded-xl border border-slate-100 dark:border-white/10 bg-white dark:bg-slate-900 px-4 py-3 text-sm text-slate-500 dark:text-slate-400 shadow-card">
          {collection.note}
        </p>
      )}

      {/* Settings section */}
      <div className="rounded-2xl border border-slate-100 dark:border-white/10 bg-white dark:bg-slate-900 shadow-card">
        <div className="border-b border-slate-100 dark:border-white/10 px-5 py-4">
          <p className="font-semibold text-slate-900 dark:text-slate-100">Settings</p>
          <p className="text-xs text-slate-400">Collection-level options</p>
        </div>
        <div className="flex items-center justify-between gap-4 px-5 py-4">
          <div className="flex items-start gap-3">
            <div className="mt-0.5 flex h-8 w-8 shrink-0 items-center justify-center rounded-lg bg-slate-100 text-slate-500 dark:bg-white/10 dark:text-slate-400">
              {collection.is_hidden ? <EyeOff size={15} /> : <Eye size={15} />}
            </div>
            <div>
              <label
                htmlFor="collection-hidden"
                className="text-sm font-medium text-slate-800 dark:text-slate-100"
              >
                Hidden
              </label>
              <p className="text-xs text-slate-400">
                Hide this collection from navigation and content listings.
              </p>
            </div>
          </div>
          <Switch.Root
            id="collection-hidden"
            checked={collection.is_hidden}
            onCheckedChange={(v) => settingsMutation.mutate({ is_hidden: v })}
            disabled={settingsMutation.isPending}
            className={`relative h-5 w-9 shrink-0 rounded-full transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500 focus-visible:ring-offset-1 disabled:opacity-50 ${
              collection.is_hidden ? "bg-brand-600" : "bg-slate-200 dark:bg-white/15"
            }`}
          >
            <Switch.Thumb
              className={`block h-3.5 w-3.5 rounded-full bg-white shadow-sm transition-transform ${
                collection.is_hidden ? "translate-x-[18px]" : "translate-x-[3px]"
              }`}
            />
          </Switch.Root>
        </div>
      </div>

      {/* Sections + fields */}
      <div className="flex items-center justify-between gap-3">
        <div>
          <p className="font-semibold text-slate-900 dark:text-slate-100">Sections</p>
          <p className="text-xs text-slate-400">
            {sections.length} section{sections.length !== 1 ? "s" : ""} · {userFields.length} custom
            field{userFields.length !== 1 ? "s" : ""}
            {systemFields.length > 0 ? ` · ${systemFields.length} system` : ""}
          </p>
        </div>
        <div className="flex items-center gap-2">
          {sections.length > 1 && (
            <Button size="sm" variant="outline" onClick={() => setSortSectionsOpen(true)}>
              <ArrowUpDown size={14} />
              Sort
            </Button>
          )}
          <Button size="sm" variant="outline" onClick={() => setShowCreateSection(true)}>
            <Plus size={14} />
            Add Section
          </Button>
        </div>
      </div>

      {sections.length === 0 ? (
        <div className="rounded-2xl border border-slate-100 dark:border-white/10 bg-white dark:bg-slate-900 py-12 text-center text-sm text-slate-400 shadow-card">
          No sections yet
        </div>
      ) : (
        <div className="space-y-4">
          {sections.map((section) => {
            const sectionFields = fieldsForSection(section);
            const hasVisibility = section.visibility != null;
            return (
              <div
                key={section.id}
                className="overflow-hidden rounded-2xl border border-slate-100 dark:border-white/10 bg-white dark:bg-slate-900 shadow-card"
              >
                <div className="flex items-start justify-between gap-3 border-b border-slate-100 dark:border-white/10 px-5 py-4">
                  <div className="min-w-0">
                    <div className="flex flex-wrap items-center gap-2">
                      <p className="font-semibold text-slate-900 dark:text-slate-100">
                        {section.display_name}
                      </p>
                      <span className="font-mono text-[11px] text-slate-400">{section.name}</span>
                      {hasVisibility && (
                        <span className="badge badge-blue">conditional</span>
                      )}
                    </div>
                    {section.note ? (
                      <p className="mt-0.5 text-xs text-slate-400">{section.note}</p>
                    ) : (
                      <p className="mt-0.5 text-xs text-slate-400">
                        {sectionFields.length} field{sectionFields.length !== 1 ? "s" : ""}
                      </p>
                    )}
                  </div>
                  <div className="flex shrink-0 items-center gap-2">
                    <Button
                      size="sm"
                      variant="ghost"
                      onClick={() => setEditingSection(section)}
                    >
                      <Pencil size={13} />
                      Edit
                    </Button>
                    <Button size="sm" onClick={() => setAddFieldSection(section.name)}>
                      <Plus size={14} />
                      Add Field
                    </Button>
                  </div>
                </div>

                {sectionFields.length === 0 ? (
                  <div className="py-10 text-center text-sm text-slate-400">
                    No fields in this section yet
                  </div>
                ) : (
                  <DndContext
                    sensors={sensors}
                    collisionDetection={closestCenter}
                    onDragEnd={(e) => handleSectionDragEnd(section, e)}
                  >
                    <SortableContext
                      items={sectionFields.map((f) => f.id)}
                      strategy={verticalListSortingStrategy}
                    >
                      <table className="w-full text-sm">
                        <thead>
                          <tr className="text-left text-xs font-medium text-slate-400">
                            <th className="w-8 py-3 pl-3" />
                            <th className="px-4 py-3">Name</th>
                            <th className="px-4 py-3">Display Name</th>
                            <th className="px-4 py-3">Type</th>
                            <th className="px-4 py-3">Required</th>
                            <th className="px-4 py-3">Unique</th>
                            <th className="w-10 px-4 py-3" />
                          </tr>
                        </thead>
                        <tbody className="divide-y divide-slate-50 dark:divide-white/10">
                          {sectionFields.map((f) => (
                            <SortableFieldRow
                              key={f.id}
                              field={f}
                              collectionName={name}
                              isReordering={reorderMutation.isPending}
                            />
                          ))}
                        </tbody>
                      </table>
                    </SortableContext>
                  </DndContext>
                )}
              </div>
            );
          })}
        </div>
      )}

      {/* Relations section */}
      <RelationsSection collectionName={name} allCollections={allCollections} />

      {/* Indexes section */}
      <IndexesSection collectionName={name} fields={userFields} />

      <AddFieldPanel
        open={addFieldSection != null}
        collectionName={name}
        section={addFieldSection ?? "general"}
        existingFields={flatFields}
        onClose={() => setAddFieldSection(null)}
      />

      <EditSectionPanel
        open={editingSection != null}
        collectionName={name}
        section={editingSection}
        sections={sections}
        collectionFields={userFields}
        onClose={() => setEditingSection(null)}
      />

      <AddSectionPanel
        open={showCreateSection}
        collectionName={name}
        nextSort={sections.length}
        onClose={() => setShowCreateSection(false)}
      />

      <SectionReorderPanel
        open={sortSectionsOpen}
        collectionName={name}
        sections={sections}
        onClose={() => setSortSectionsOpen(false)}
      />

      {/* Delete collection confirm — only non-system */}
      <ConfirmDialog
        open={confirmDelete}
        title={`Delete "${collection.display_name}"?`}
        description="This will permanently drop the database table and all its data. This action cannot be undone."
        confirmLabel="Delete collection"
        onConfirm={() => deleteMutation.mutate()}
        onCancel={() => setConfirmDelete(false)}
        loading={deleteMutation.isPending}
        danger
      />
    </div>
  );
}

// ── Sortable field row ────────────────────────────────────────────────────────

function SortableFieldRow({
  field: f,
  collectionName,
  isReordering,
}: {
  field: Field;
  collectionName: string;
  isReordering?: boolean;
}) {
  const {
    attributes,
    listeners,
    setNodeRef,
    transform,
    transition,
    isDragging,
  } = useSortable({ id: f.id });

  const style = {
    transform: CSS.Transform.toString(transform),
    transition,
    zIndex: isDragging ? 10 : undefined,
    position: isDragging ? ("relative" as const) : undefined,
  };

  const [confirm, setConfirm] = useState(false);
  const [showEdit, setShowEdit] = useState(false);
  const queryClient = useQueryClient();

  const deleteMutation = useMutation({
    mutationFn: () => deleteField(collectionName, f.name),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["collection", collectionName] });
    },
  });

  const typeConfig = FIELD_TYPES.find((t) => t.type === f.type);

  return (
    <tr
      ref={setNodeRef}
      style={style}
      className={`group transition ${
        isDragging ? "bg-brand-50/60 dark:bg-brand-500/15 shadow-sm opacity-80" : "hover:bg-slate-50/50 dark:hover:bg-white/5"
      }`}
    >
      {/* Drag handle — shows a spinner while a reorder is in flight */}
      <td className="pl-3 py-3 w-8">
        {isReordering ? (
          <span
            className="flex h-6 w-6 items-center justify-center"
            title="Saving order…"
          >
            <span className="h-3 w-3 animate-spin rounded-full border border-slate-200 dark:border-white/10 border-t-slate-400" />
          </span>
        ) : (
          <button
            type="button"
            className="flex h-6 w-6 cursor-grab items-center justify-center rounded text-slate-300 hover:text-slate-500 dark:hover:text-slate-300 active:cursor-grabbing"
            {...attributes}
            {...listeners}
            title="Drag to reorder"
          >
            <GripVertical size={14} />
          </button>
        )}
      </td>

      <td className="px-4 py-3">
        <div className="flex items-center gap-2">
          {f.is_system && <Lock size={11} className="text-slate-300" />}
          <span className="font-mono text-xs text-slate-700 dark:text-slate-200">{f.name}</span>
        </div>
      </td>
      <td className="px-4 py-3 text-slate-600 dark:text-slate-300">{f.display_name}</td>
      <td className="px-4 py-3">
        <span
          className={`inline-flex items-center gap-1.5 rounded-lg px-2 py-1 text-xs font-medium ${
            typeConfig?.color ?? "bg-slate-100 dark:bg-white/10 text-slate-600 dark:text-slate-300"
          }`}
        >
          {typeConfig && <typeConfig.Icon size={11} />}
          {f.type}
        </span>
      </td>
      <td className="px-4 py-3">
        {f.required ? (
          <span className="badge badge-red">yes</span>
        ) : (
          <span className="text-xs text-slate-300">—</span>
        )}
      </td>
      <td className="px-4 py-3">
        {f.unique ? (
          <span className="badge badge-blue">yes</span>
        ) : (
          <span className="text-xs text-slate-300">—</span>
        )}
      </td>

      {/* Edit / Delete — only for non-system fields */}
      <td className="px-4 py-3">
        {!f.is_system ? (
          <>
            <div className="flex items-center justify-end gap-1 opacity-0 transition group-hover:opacity-100">
              <button
                onClick={() => setShowEdit(true)}
                className="rounded-lg p-1.5 text-slate-300 transition hover:bg-brand-50 dark:hover:bg-brand-500/15 hover:text-brand-600 dark:hover:text-brand-300"
                title="Edit field"
              >
                <Pencil size={13} />
              </button>
              <button
                onClick={() => setConfirm(true)}
                className="rounded-lg p-1.5 text-slate-300 transition hover:bg-red-50 dark:hover:bg-red-500/10 hover:text-red-500 dark:hover:text-red-400"
                title="Delete field"
              >
                <Trash2 size={14} />
              </button>
            </div>
            <EditFieldPanel
              open={showEdit}
              field={f}
              collectionName={collectionName}
              onClose={() => setShowEdit(false)}
            />
            <ConfirmDialog
              open={confirm}
              title={`Delete field "${f.name}"?`}
              description="This will drop the column from the database. Any stored data in this column will be lost."
              confirmLabel="Delete field"
              onConfirm={() => deleteMutation.mutate()}
              onCancel={() => setConfirm(false)}
              loading={deleteMutation.isPending}
              danger
            />
          </>
        ) : (
          /* System field — lock icon instead, always visible */
          <span className="flex justify-center text-slate-200" title="System field — cannot be deleted">
            <Lock size={13} />
          </span>
        )}
      </td>
    </tr>
  );
}

// ── Relations section ─────────────────────────────────────────────────────────

const RELATION_TYPE_LABELS: Record<RelationType, string> = {
  many_to_one:  "Many → One",
  one_to_many:  "One → Many",
  many_to_many: "Many ↔ Many",
  one_to_one:   "One → One",
};

function RelationsSection({
  collectionName,
  allCollections,
}: {
  collectionName: string;
  allCollections: Collection[];
}) {
  const [showAdd, setShowAdd] = useState(false);
  const [confirmDeleteId, setConfirmDeleteId] = useState<string | null>(null);
  const queryClient = useQueryClient();

  const { data: relations = [] } = useQuery({
    queryKey: ["relations", collectionName],
    queryFn: () =>
      get<Relation[]>(`/collections/${collectionName}/relations`).then(
        (r) => (Array.isArray(r) ? r : [])
      ),
  });

  const deleteMut = useMutation({
    mutationFn: (id: string) =>
      del(`/collections/${collectionName}/relations/${id}`),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["relations", collectionName] });
      queryClient.invalidateQueries({ queryKey: ["collection", collectionName] });
    },
  });

  return (
    <div className="rounded-2xl border border-slate-100 dark:border-white/10 bg-white dark:bg-slate-900 shadow-card">
      <div className="flex items-center justify-between border-b border-slate-100 dark:border-white/10 px-5 py-4">
        <div>
          <p className="font-semibold text-slate-900 dark:text-slate-100">Relations</p>
          <p className="text-xs text-slate-400">
            {relations.length} relation{relations.length !== 1 ? "s" : ""}
          </p>
        </div>
        <Button size="sm" onClick={() => setShowAdd(true)}>
          <Plus size={14} />
          Add Relation
        </Button>
      </div>

      {relations.length === 0 ? (
        <div className="py-10 text-center text-sm text-slate-400">
          No relations yet — link this collection to another
        </div>
      ) : (
        <table className="w-full text-sm">
          <thead>
            <tr className="text-left text-xs font-medium text-slate-400">
              <th className="px-5 py-3">Type</th>
              <th className="px-5 py-3">Related Collection</th>
              <th className="px-5 py-3">Field / Junction</th>
              <th className="px-5 py-3">On Delete</th>
              <th className="w-10 px-5 py-3" />
            </tr>
          </thead>
          <tbody className="divide-y divide-slate-50 dark:divide-white/10">
            {relations.map((rel) => {
              const relatedColl =
                rel.collection_many === collectionName
                  ? rel.collection_one
                  : rel.collection_many;
              const fieldOrJunction =
                rel.type === "many_to_many"
                  ? rel.junction_table ?? "—"
                  : rel.field_many;

              return (
                <tr key={rel.id} className="group transition hover:bg-slate-50/50 dark:hover:bg-white/5">
                  <td className="px-5 py-3">
                    <span className="inline-flex items-center gap-1.5 rounded-lg bg-violet-50 dark:bg-violet-500/10 px-2 py-1 text-xs font-medium text-violet-700 dark:text-violet-300">
                      <ArrowLeftRight size={11} />
                      {RELATION_TYPE_LABELS[rel.type]}
                    </span>
                  </td>
                  <td className="px-5 py-3 font-medium text-slate-700 dark:text-slate-200">{relatedColl}</td>
                  <td className="px-5 py-3 font-mono text-xs text-slate-500 dark:text-slate-400">{fieldOrJunction}</td>
                  <td className="px-5 py-3 text-xs capitalize text-slate-400">
                    {rel.on_delete.replace("_", " ")}
                  </td>
                  <td className="px-5 py-3 text-right">
                    <button
                      onClick={() => setConfirmDeleteId(rel.id)}
                      className="rounded-lg p-1.5 text-slate-300 opacity-0 transition hover:bg-red-50 dark:hover:bg-red-500/10 hover:text-red-500 dark:hover:text-red-400 group-hover:opacity-100"
                      title="Remove relation"
                    >
                      <Trash2 size={14} />
                    </button>
                  </td>
                </tr>
              );
            })}
          </tbody>
        </table>
      )}

      <AddRelationPanel
        open={showAdd}
        collectionName={collectionName}
        allCollections={allCollections}
        onClose={() => setShowAdd(false)}
        onCreated={() => {
          queryClient.invalidateQueries({ queryKey: ["relations", collectionName] });
          queryClient.invalidateQueries({ queryKey: ["collection", collectionName] });
          setShowAdd(false);
        }}
      />

      <ConfirmDialog
        open={confirmDeleteId !== null}
        title="Remove relation?"
        description="This will drop the foreign key column (or junction table) from the database. Any data in that column will be permanently lost."
        confirmLabel="Remove relation"
        onConfirm={() => {
          if (confirmDeleteId) deleteMut.mutate(confirmDeleteId);
          setConfirmDeleteId(null);
        }}
        onCancel={() => setConfirmDeleteId(null)}
        loading={deleteMut.isPending}
        danger
      />
    </div>
  );
}

// ── Add Relation panel ────────────────────────────────────────────────────────

function AddRelationPanel({
  open,
  collectionName,
  allCollections,
  onClose,
  onCreated,
}: {
  open: boolean;
  collectionName: string;
  allCollections: Collection[];
  onClose: () => void;
  onCreated: () => void;
}) {
  const [relType, setRelType] = useState<RelationType>("many_to_one");
  const [relatedCollection, setRelatedCollection] = useState("");
  const [fieldName, setFieldName] = useState("");
  const [displayName, setDisplayName] = useState("");
  const [required, setRequired] = useState(false);
  const [onDelete, setOnDelete] = useState<"set_null" | "cascade" | "restrict">("set_null");
  const [junctionTable, setJunctionTable] = useState("");
  const [serverError, setServerError] = useState<string | null>(null);

  const needsField = relType !== "many_to_many";
  const needsJunction = relType === "many_to_many";
  const otherCollections = allCollections.filter((c) => c.name !== collectionName);

  // Global system tables that live outside the tenant schema but can be referenced.
  // These are always shown regardless of what the collections API returns.
  const GLOBAL_TARGETS = [
    { name: "users", label: "Users (system)" },
  ];

  // Auto-suggest names when inputs change
  function applySuggestions(type: RelationType, related: string) {
    if (type === "many_to_one" || type === "one_to_one") {
      setFieldName(related ? related + "_id" : "");
      setDisplayName(related ? related.replace(/_/g, " ") : "");
    } else if (type === "one_to_many") {
      setFieldName(related ? collectionName + "_id" : "");
      setDisplayName(related ? collectionName.replace(/_/g, " ") : "");
    } else {
      setJunctionTable(related ? collectionName + "_" + related : "");
      setFieldName("");
      setDisplayName("");
    }
  }

  function handleTypeChange(t: RelationType) {
    setRelType(t);
    applySuggestions(t, relatedCollection);
  }

  function handleRelatedChange(val: string) {
    setRelatedCollection(val);
    applySuggestions(relType, val);
  }

  function handleRequiredChange(val: boolean) {
    setRequired(val);
    // set_null is incompatible with NOT NULL columns — switch to restrict
    if (val && onDelete === "set_null") setOnDelete("restrict");
  }

  const mutation = useMutation({
    mutationFn: () =>
      post<Relation>(`/collections/${collectionName}/relations`, {
        type: relType,
        related_collection: relatedCollection,
        field_name: fieldName,
        display_name: displayName,
        required,
        on_delete: onDelete,
        junction_table: junctionTable,
      }),
    onSuccess: onCreated,
    onError: (e: Error) => setServerError(e.message),
  });

  const RELATION_TYPES: { type: RelationType; label: string; desc: string }[] = [
    { type: "many_to_one",  label: "Many → One",  desc: `Many ${collectionName} items reference one in the related collection` },
    { type: "one_to_many",  label: "One → Many",  desc: `One ${collectionName} item is referenced by many in the related collection` },
    { type: "many_to_many", label: "Many ↔ Many", desc: "A junction table links items from both collections" },
    { type: "one_to_one",   label: "One → One",   desc: `Each ${collectionName} item maps to exactly one in the related collection` },
  ];

  const panel = (
    <AnimatePresence>
      {open && (
        <>
          <motion.div
            key="rel-backdrop"
            initial={{ opacity: 0 }} animate={{ opacity: 1 }} exit={{ opacity: 0 }}
            transition={{ duration: 0.18 }}
            className="fixed inset-0 z-40 bg-black/20 backdrop-blur-[2px]"
            onClick={onClose}
          />
          <motion.aside
            key="rel-panel"
            initial={{ x: "100%" }} animate={{ x: 0 }} exit={{ x: "100%" }}
            transition={{ type: "spring", damping: 28, stiffness: 280 }}
            className="fixed right-0 top-0 z-50 flex h-full w-full max-w-sm flex-col bg-white dark:bg-slate-900 shadow-2xl"
          >
            {/* Header */}
            <div className="flex shrink-0 items-center justify-between border-b border-slate-100 dark:border-white/10 px-6 py-5">
              <div>
                <h2 className="text-base font-semibold text-slate-900 dark:text-slate-100">Add Relation</h2>
                <p className="mt-0.5 text-xs text-slate-400">{collectionName}</p>
              </div>
              <button type="button" onClick={onClose}
                className="rounded-lg p-1.5 text-slate-400 transition hover:bg-slate-100 dark:hover:bg-white/5 hover:text-slate-600 dark:hover:text-slate-200">
                <X size={18} />
              </button>
            </div>

            {/* Body */}
            <div className="flex-1 space-y-5 overflow-y-auto px-6 py-6">

              {/* Relation type */}
              <div>
                <label className="mb-2 block text-xs font-medium text-slate-700 dark:text-slate-200">
                  Relation Type <span className="text-red-400">*</span>
                </label>
                <div className="space-y-2">
                  {RELATION_TYPES.map(({ type, label, desc }) => (
                    <button
                      key={type}
                      type="button"
                      onClick={() => handleTypeChange(type)}
                      className={`w-full rounded-xl border p-3 text-left transition ${
                        relType === type
                          ? "border-brand-400 bg-brand-50 dark:bg-brand-500/15"
                          : "border-slate-100 dark:border-white/10 hover:border-slate-200 dark:hover:border-white/10 hover:bg-slate-50 dark:hover:bg-white/5"
                      }`}
                    >
                      <p className={`text-sm font-semibold ${relType === type ? "text-brand-700 dark:text-brand-300" : "text-slate-700 dark:text-slate-200"}`}>
                        {label}
                      </p>
                      <p className="mt-0.5 text-xs text-slate-400">{desc}</p>
                    </button>
                  ))}
                </div>
              </div>

              {/* Related collection */}
              <div>
                <label className="mb-1.5 block text-xs font-medium text-slate-700 dark:text-slate-200">
                  Related Collection <span className="text-red-400">*</span>
                </label>
                <select
                  className="w-full rounded-lg border border-slate-200 dark:border-white/10 bg-white dark:bg-white/5 px-3 py-2 text-sm text-slate-800 dark:text-slate-100 focus:border-brand-400 dark:focus:border-brand-500/60 focus:outline-none focus:ring-2 focus:ring-brand-100 dark:focus:ring-brand-500/20"
                  value={relatedCollection}
                  onChange={(e) => handleRelatedChange(e.target.value)}
                >
                  <option value="">— choose a collection —</option>
                  <optgroup label="System">
                    {GLOBAL_TARGETS.map((t) => (
                      <option key={t.name} value={t.name}>{t.label}</option>
                    ))}
                  </optgroup>
                  {otherCollections.length > 0 && (
                    <optgroup label="Collections">
                      {otherCollections.map((c) => (
                        <option key={c.id} value={c.name}>
                          {c.display_name} ({c.name})
                        </option>
                      ))}
                    </optgroup>
                  )}
                </select>
              </div>

              {/* FK field name */}
              {needsField && (
                <>
                  <div>
                    <label className="mb-1.5 block text-xs font-medium text-slate-700 dark:text-slate-200">
                      Foreign Key Field <span className="text-red-400">*</span>
                    </label>
                    <input
                      className="input font-mono text-sm"
                      value={fieldName}
                      onChange={(e) => setFieldName(e.target.value)}
                      placeholder="category_id"
                    />
                    <p className="mt-1 text-xs text-slate-400">
                      {relType === "one_to_many"
                        ? `UUID column added to the "${relatedCollection || "related"}" table`
                        : `UUID column added to the "${collectionName}" table`}
                    </p>
                  </div>
                  <div>
                    <label className="mb-1.5 block text-xs font-medium text-slate-700 dark:text-slate-200">
                      Display Name
                    </label>
                    <input
                      className="input"
                      value={displayName}
                      onChange={(e) => setDisplayName(e.target.value)}
                      placeholder="Category"
                    />
                  </div>
                </>
              )}

              {/* Junction table */}
              {needsJunction && (
                <div>
                  <label className="mb-1.5 block text-xs font-medium text-slate-700 dark:text-slate-200">
                    Junction Table
                  </label>
                  <input
                    className="input font-mono text-sm"
                    value={junctionTable}
                    onChange={(e) => setJunctionTable(e.target.value)}
                    placeholder={`${collectionName}_${relatedCollection || "other"}`}
                  />
                  <p className="mt-1 text-xs text-slate-400">
                    Auto-generated as {collectionName}_{relatedCollection || "related"} if left blank
                  </p>
                </div>
              )}

              {/* Required */}
              {needsField && (
                <label className="flex cursor-pointer items-start gap-3 rounded-xl border border-slate-100 dark:border-white/10 bg-slate-50 dark:bg-white/5 px-4 py-3">
                  <button
                    type="button"
                    role="switch"
                    aria-checked={required}
                    onClick={() => handleRequiredChange(!required)}
                    className={`relative mt-0.5 inline-flex h-5 w-9 shrink-0 items-center rounded-full transition-colors focus:outline-none focus:ring-2 focus:ring-brand-500 focus:ring-offset-1 ${
                      required ? "bg-brand-600" : "bg-slate-200 dark:bg-white/15"
                    }`}
                  >
                    <span
                      className={`inline-block h-3.5 w-3.5 rounded-full bg-white shadow-sm transition-transform ${
                        required ? "translate-x-[18px]" : "translate-x-[3px]"
                      }`}
                    />
                  </button>
                  <div>
                    <p className="text-sm font-medium text-slate-800 dark:text-slate-100">Required</p>
                    <p className="text-xs text-slate-400">
                      Column will be <span className="font-mono">NOT NULL</span> — every item must reference a related record
                    </p>
                  </div>
                </label>
              )}

              {/* On Delete */}
              {needsField && (
                <div>
                  <label className="mb-1.5 block text-xs font-medium text-slate-700 dark:text-slate-200">
                    On Delete
                  </label>
                  <div className="flex gap-2">
                    {(["set_null", "cascade", "restrict"] as const).map((v) => {
                      const disabled = v === "set_null" && required;
                      return (
                        <button
                          key={v}
                          type="button"
                          disabled={disabled}
                          onClick={() => !disabled && setOnDelete(v)}
                          title={disabled ? "Cannot set null on a required (NOT NULL) column" : undefined}
                          className={`flex-1 rounded-xl border py-2 text-xs font-medium transition ${
                            onDelete === v
                              ? "border-brand-400 bg-brand-50 dark:bg-brand-500/15 text-brand-700 dark:text-brand-300"
                              : disabled
                              ? "cursor-not-allowed border-slate-100 dark:border-white/10 text-slate-300"
                              : "border-slate-200 dark:border-white/10 text-slate-500 dark:text-slate-400 hover:bg-slate-50 dark:hover:bg-white/5"
                          }`}
                        >
                          {v.replace("_", " ")}
                        </button>
                      );
                    })}
                  </div>
                  <p className="mt-1.5 text-xs text-slate-400">
                    {onDelete === "set_null" && "Set the FK to NULL when the referenced row is deleted"}
                    {onDelete === "cascade" && "Delete this row when the referenced row is deleted"}
                    {onDelete === "restrict" && "Prevent deleting the referenced row while references exist"}
                  </p>
                </div>
              )}

              {serverError && (
                <div className="flex items-start gap-2 rounded-xl bg-red-50 dark:bg-red-500/10 px-3 py-2.5 text-sm text-red-600 dark:text-red-400">
                  <AlertTriangle size={14} className="mt-0.5 shrink-0" />
                  {serverError}
                </div>
              )}
            </div>

            {/* Footer */}
            <div className="flex shrink-0 items-center justify-end gap-2 border-t border-slate-100 dark:border-white/10 px-6 py-4">
              <Button type="button" variant="ghost" onClick={onClose}>Cancel</Button>
              <Button
                onClick={() => { setServerError(null); mutation.mutate(); }}
                disabled={
                  !relatedCollection ||
                  mutation.isPending ||
                  (needsField && !fieldName)
                }
              >
                {mutation.isPending ? "Creating…" : "Create Relation"}
              </Button>
            </div>
          </motion.aside>
        </>
      )}
    </AnimatePresence>
  );

  return createPortal(panel, document.body);
}

// ── Indexes section ───────────────────────────────────────────────────────────

const INDEX_TYPE_LABELS: Record<IndexType, string> = {
  btree:    "B-tree",
  fulltext: "Full-text",
};

const INDEX_TYPE_COLORS: Record<IndexType, string> = {
  btree:    "bg-sky-50 dark:bg-sky-500/10 text-sky-700 dark:text-sky-300",
  fulltext: "bg-emerald-50 dark:bg-emerald-500/10 text-emerald-700 dark:text-emerald-300",
};

function IndexesSection({
  collectionName,
  fields,
}: {
  collectionName: string;
  fields: Field[];
}) {
  const [showAdd, setShowAdd] = useState(false);
  const [confirmDeleteId, setConfirmDeleteId] = useState<string | null>(null);
  const queryClient = useQueryClient();

  const { data: indexes = [] } = useQuery({
    queryKey: ["indexes", collectionName],
    queryFn: () =>
      get<CollectionIndex[]>(`/collections/${collectionName}/indexes`).then(
        (r) => (Array.isArray(r) ? r : [])
      ),
  });

  const deleteMut = useMutation({
    mutationFn: (id: string) =>
      del(`/collections/${collectionName}/indexes/${id}`),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["indexes", collectionName] });
    },
  });

  return (
    <div className="rounded-2xl border border-slate-100 dark:border-white/10 bg-white dark:bg-slate-900 shadow-card">
      <div className="flex items-center justify-between border-b border-slate-100 dark:border-white/10 px-5 py-4">
        <div>
          <p className="font-semibold text-slate-900 dark:text-slate-100">Indexes</p>
          <p className="text-xs text-slate-400">
            {indexes.length} index{indexes.length !== 1 ? "es" : ""}
          </p>
        </div>
        <Button size="sm" onClick={() => setShowAdd(true)}>
          <Plus size={14} />
          Add Index
        </Button>
      </div>

      {indexes.length === 0 ? (
        <div className="py-10 text-center text-sm text-slate-400">
          No indexes yet — add one to speed up queries on this collection
        </div>
      ) : (
        <table className="w-full text-sm">
          <thead>
            <tr className="text-left text-xs font-medium text-slate-400">
              <th className="px-5 py-3">Name</th>
              <th className="px-5 py-3">Type</th>
              <th className="px-5 py-3">Fields</th>
              <th className="w-10 px-5 py-3" />
            </tr>
          </thead>
          <tbody className="divide-y divide-slate-50 dark:divide-white/10">
            {indexes.map((idx) => (
              <tr key={idx.id} className="group transition hover:bg-slate-50/50 dark:hover:bg-white/5">
                <td className="px-5 py-3 font-mono text-xs text-slate-600 dark:text-slate-300">{idx.name}</td>
                <td className="px-5 py-3">
                  <span className={`inline-flex items-center rounded-lg px-2 py-1 text-xs font-medium ${INDEX_TYPE_COLORS[idx.type]}`}>
                    {INDEX_TYPE_LABELS[idx.type]}
                  </span>
                </td>
                <td className="px-5 py-3">
                  <div className="flex flex-wrap gap-1">
                    {idx.fields.map((f) => (
                      <span key={f} className="rounded bg-slate-100 dark:bg-white/10 px-1.5 py-0.5 font-mono text-xs text-slate-600 dark:text-slate-300">
                        {f}
                      </span>
                    ))}
                  </div>
                </td>
                <td className="px-5 py-3 text-right">
                  <button
                    onClick={() => setConfirmDeleteId(idx.id)}
                    className="rounded-lg p-1.5 text-slate-300 opacity-0 transition hover:bg-red-50 dark:hover:bg-red-500/10 hover:text-red-500 dark:hover:text-red-400 group-hover:opacity-100"
                    title="Drop index"
                  >
                    <Trash2 size={14} />
                  </button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}

      <AddIndexPanel
        open={showAdd}
        collectionName={collectionName}
        fields={fields}
        onClose={() => setShowAdd(false)}
        onCreated={() => {
          queryClient.invalidateQueries({ queryKey: ["indexes", collectionName] });
          setShowAdd(false);
        }}
      />

      <ConfirmDialog
        open={confirmDeleteId !== null}
        title="Drop index?"
        description="This removes the index from the database. The data itself is not affected, but queries relying on this index may become slower."
        confirmLabel="Drop index"
        onConfirm={() => {
          if (confirmDeleteId) deleteMut.mutate(confirmDeleteId);
          setConfirmDeleteId(null);
        }}
        onCancel={() => setConfirmDeleteId(null)}
        loading={deleteMut.isPending}
        danger
      />
    </div>
  );
}

// ── Add Index panel ───────────────────────────────────────────────────────────

function AddIndexPanel({
  open,
  collectionName,
  fields,
  onClose,
  onCreated,
}: {
  open: boolean;
  collectionName: string;
  fields: Field[];
  onClose: () => void;
  onCreated: () => void;
}) {
  const [indexType, setIndexType] = useState<IndexType>("btree");
  const [selectedFields, setSelectedFields] = useState<string[]>([]);
  const [indexName, setIndexName] = useState("");
  const [serverError, setServerError] = useState<string | null>(null);

  function toggleField(fieldName: string) {
    setSelectedFields((prev) =>
      prev.includes(fieldName)
        ? prev.filter((f) => f !== fieldName)
        : [...prev, fieldName]
    );
  }

  const mutation = useMutation({
    mutationFn: () =>
      post<CollectionIndex>(`/collections/${collectionName}/indexes`, {
        type: indexType,
        fields: selectedFields,
        name: indexName || undefined,
      }),
    onSuccess: onCreated,
    onError: (e: Error) => setServerError(e.message),
  });

  const INDEX_TYPE_OPTIONS: { type: IndexType; label: string; desc: string }[] = [
    {
      type: "btree",
      label: "B-tree",
      desc: "Standard index for filtering, sorting, and range queries. Works on any field type. Use multiple fields to create a composite index.",
    },
    {
      type: "fulltext",
      label: "Full-text",
      desc: "GIN index using to_tsvector. Enables fast keyword search across one or more text fields.",
    },
  ];

  const panel = (
    <AnimatePresence>
      {open && (
        <>
          <motion.div
            key="idx-backdrop"
            initial={{ opacity: 0 }} animate={{ opacity: 1 }} exit={{ opacity: 0 }}
            transition={{ duration: 0.18 }}
            className="fixed inset-0 z-40 bg-black/20 backdrop-blur-[2px]"
            onClick={onClose}
          />
          <motion.aside
            key="idx-panel"
            initial={{ x: "100%" }} animate={{ x: 0 }} exit={{ x: "100%" }}
            transition={{ type: "spring", damping: 28, stiffness: 280 }}
            className="fixed right-0 top-0 z-50 flex h-full w-full max-w-sm flex-col bg-white dark:bg-slate-900 shadow-2xl"
          >
            <div className="flex shrink-0 items-center justify-between border-b border-slate-100 dark:border-white/10 px-6 py-5">
              <div>
                <h2 className="text-base font-semibold text-slate-900 dark:text-slate-100">Add Index</h2>
                <p className="mt-0.5 text-xs text-slate-400">{collectionName}</p>
              </div>
              <button type="button" onClick={onClose}
                className="rounded-lg p-1.5 text-slate-400 transition hover:bg-slate-100 dark:hover:bg-white/5 hover:text-slate-600 dark:hover:text-slate-200">
                <X size={18} />
              </button>
            </div>

            <div className="flex-1 space-y-5 overflow-y-auto px-6 py-6">
              <div>
                <label className="mb-2 block text-xs font-medium text-slate-700 dark:text-slate-200">
                  Index Type <span className="text-red-400">*</span>
                </label>
                <div className="space-y-2">
                  {INDEX_TYPE_OPTIONS.map(({ type, label, desc }) => (
                    <button
                      key={type}
                      type="button"
                      onClick={() => setIndexType(type)}
                      className={`w-full rounded-xl border p-3 text-left transition ${
                        indexType === type
                          ? "border-brand-400 bg-brand-50 dark:bg-brand-500/15"
                          : "border-slate-100 dark:border-white/10 hover:border-slate-200 dark:hover:border-white/10 hover:bg-slate-50 dark:hover:bg-white/5"
                      }`}
                    >
                      <p className={`text-sm font-semibold ${indexType === type ? "text-brand-700 dark:text-brand-300" : "text-slate-700 dark:text-slate-200"}`}>
                        {label}
                      </p>
                      <p className="mt-0.5 text-xs text-slate-400">{desc}</p>
                    </button>
                  ))}
                </div>
              </div>

              <div>
                <label className="mb-2 block text-xs font-medium text-slate-700 dark:text-slate-200">
                  Fields <span className="text-red-400">*</span>
                  {indexType === "btree" && selectedFields.length > 1 && (
                    <span className="ml-1.5 font-normal text-slate-400">(order matters for composite)</span>
                  )}
                </label>
                {fields.length === 0 ? (
                  <p className="text-xs text-slate-400">No fields available</p>
                ) : (
                  <div className="space-y-1.5 rounded-xl border border-slate-100 dark:border-white/10 bg-slate-50 dark:bg-white/5 px-3 py-3">
                    {fields.map((f) => {
                      const checked = selectedFields.includes(f.name);
                      const order = selectedFields.indexOf(f.name);
                      return (
                        <label
                          key={f.id}
                          className="flex cursor-pointer items-center gap-3 rounded-lg px-2 py-1.5 transition hover:bg-white dark:hover:bg-white/10"
                        >
                          <input
                            type="checkbox"
                            checked={checked}
                            onChange={() => toggleField(f.name)}
                            className="h-4 w-4 rounded border-slate-300 dark:border-white/10 text-brand-600 focus:ring-brand-400"
                          />
                          <span className="flex-1 text-sm text-slate-700 dark:text-slate-200">
                            {f.display_name}
                            <span className="ml-1.5 font-mono text-xs text-slate-400">{f.name}</span>
                          </span>
                          {checked && indexType === "btree" && selectedFields.length > 1 && (
                            <span className="flex h-5 w-5 items-center justify-center rounded-full bg-brand-100 dark:bg-brand-500/20 text-[10px] font-bold text-brand-700 dark:text-brand-300">
                              {order + 1}
                            </span>
                          )}
                        </label>
                      );
                    })}
                  </div>
                )}
                {indexType === "fulltext" && (
                  <p className="mt-1.5 text-xs text-slate-400">
                    Tip: pick <span className="font-mono">text</span> or <span className="font-mono">rich_text</span> fields for best results.
                  </p>
                )}
              </div>

              <div>
                <label className="mb-1.5 block text-xs font-medium text-slate-700 dark:text-slate-200">
                  Name <span className="font-normal text-slate-400">(auto-generated if blank)</span>
                </label>
                <input
                  className="input font-mono text-sm"
                  value={indexName}
                  onChange={(e) => setIndexName(e.target.value)}
                  placeholder={
                    selectedFields.length > 0
                      ? `${collectionName}_${selectedFields.join("_")}_${indexType === "fulltext" ? "ft_" : ""}idx`
                      : `${collectionName}_…_idx`
                  }
                />
              </div>

              {serverError && (
                <div className="flex items-start gap-2 rounded-xl bg-red-50 dark:bg-red-500/10 px-3 py-2.5 text-sm text-red-600 dark:text-red-400">
                  <AlertTriangle size={14} className="mt-0.5 shrink-0" />
                  {serverError}
                </div>
              )}
            </div>

            <div className="flex shrink-0 items-center justify-end gap-2 border-t border-slate-100 dark:border-white/10 px-6 py-4">
              <Button type="button" variant="ghost" onClick={onClose}>Cancel</Button>
              <Button
                onClick={() => { setServerError(null); mutation.mutate(); }}
                disabled={selectedFields.length === 0 || mutation.isPending}
              >
                {mutation.isPending ? "Creating…" : "Create Index"}
              </Button>
            </div>
          </motion.aside>
        </>
      )}
    </AnimatePresence>
  );

  return createPortal(panel, document.body);
}

// ── Create Collection dialog ──────────────────────────────────────────────────

function CreateCollectionDialog({ open, onClose }: { open: boolean; onClose: () => void }) {
  const queryClient = useQueryClient();
  const {
    register,
    handleSubmit,
    control,
    reset,
    watch,
    setValue,
    formState: { errors },
  } = useForm<CreateCollectionInput>({
    resolver: zodResolver(createCollectionSchema),
    defaultValues: { type: "base" },
  });

  const selectedColor = watch("color");

  const mutation = useMutation({
    mutationFn: createCollection,
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["collections"] });
      reset();
      onClose();
    },
  });

  return (
    <Dialog.Root open={open} onOpenChange={(v) => !v && onClose()}>
      <Dialog.Portal>
        <Dialog.Overlay className="fixed inset-0 z-40 bg-black/20 backdrop-blur-[2px] animate-fade-in" />
        <Dialog.Content className="fixed left-1/2 top-1/2 z-50 w-full max-w-md -translate-x-1/2 -translate-y-1/2 rounded-2xl bg-white dark:bg-slate-900 p-6 shadow-glass-lg animate-slide-up">
          <div className="flex items-center justify-between">
            <Dialog.Title className="text-lg font-semibold text-slate-900 dark:text-slate-100">
              New Collection
            </Dialog.Title>
            <button
              onClick={onClose}
              className="rounded-lg p-1.5 text-slate-400 hover:bg-slate-100 dark:hover:bg-white/5 hover:text-slate-600 dark:hover:text-slate-200"
            >
              <X size={18} />
            </button>
          </div>

          <form onSubmit={handleSubmit((data) => mutation.mutate(data))} className="mt-5 space-y-4">
            <div className="grid grid-cols-2 gap-4">
              <div>
                <label className="mb-1.5 block text-xs font-medium text-slate-700 dark:text-slate-200">
                  Name <span className="text-red-400">*</span>
                </label>
                <input {...register("name")} placeholder="blog_posts" className="input font-mono text-sm" />
                {errors.name && <p className="mt-1 text-xs text-red-500 dark:text-red-400">{errors.name.message}</p>}
              </div>
              <div>
                <label className="mb-1.5 block text-xs font-medium text-slate-700 dark:text-slate-200">
                  Display Name <span className="text-red-400">*</span>
                </label>
                <input {...register("display_name")} placeholder="Blog Posts" className="input" />
                {errors.display_name && (
                  <p className="mt-1 text-xs text-red-500 dark:text-red-400">{errors.display_name.message}</p>
                )}
              </div>
            </div>

            <div>
              <label className="mb-1.5 block text-xs font-medium text-slate-700 dark:text-slate-200">Type</label>
              <Controller
                control={control}
                name="type"
                render={({ field }) => (
                  <div className="flex gap-2">
                    {(["base", "auth"] as const).map((t) => (
                      <button
                        key={t}
                        type="button"
                        onClick={() => field.onChange(t)}
                        className={`flex-1 rounded-xl border py-2.5 text-sm font-medium capitalize transition ${
                          field.value === t
                            ? "border-brand-400 bg-brand-50 dark:bg-brand-500/15 text-brand-700 dark:text-brand-300"
                            : "border-slate-200 dark:border-white/10 text-slate-500 dark:text-slate-400 hover:bg-slate-50 dark:hover:bg-white/5"
                        }`}
                      >
                        {t}
                      </button>
                    ))}
                  </div>
                )}
              />
            </div>

            <div>
              <label className="mb-1.5 block text-xs font-medium text-slate-700 dark:text-slate-200">Color</label>
              <div className="flex flex-wrap gap-2">
                {PRESET_COLORS.map((c) => (
                  <button
                    key={c}
                    type="button"
                    onClick={() => setValue("color", c)}
                    className="h-7 w-7 rounded-lg border-2 transition"
                    style={{
                      backgroundColor: c,
                      borderColor: selectedColor === c ? c : "transparent",
                      outline: selectedColor === c ? `2px solid ${c}44` : undefined,
                    }}
                  />
                ))}
              </div>
            </div>

            <div>
              <label className="mb-1.5 block text-xs font-medium text-slate-700 dark:text-slate-200">Note</label>
              <textarea
                {...register("note")}
                rows={2}
                placeholder="Short description…"
                className="input resize-none"
              />
            </div>

            {mutation.isError && (
              <p className="rounded-xl bg-red-50 dark:bg-red-500/10 px-3 py-2 text-sm text-red-600 dark:text-red-400">
                {(mutation.error as Error).message}
              </p>
            )}

            <div className="flex justify-end gap-2 pt-1">
              <Button type="button" variant="ghost" onClick={onClose}>Cancel</Button>
              <Button type="submit" disabled={mutation.isPending}>
                {mutation.isPending ? "Creating…" : "Create Collection"}
              </Button>
            </div>
          </form>
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  );
}

// ── Add Field aside panel ─────────────────────────────────────────────────────

function AddFieldPanel({
  open,
  collectionName,
  section,
  existingFields,
  onClose,
}: {
  open: boolean;
  collectionName: string;
  section: string;
  existingFields: Field[];
  onClose: () => void;
}) {
  const queryClient = useQueryClient();
  const {
    register,
    handleSubmit,
    control,
    reset,
    watch,
    setValue,
    formState: { errors },
  } = useForm<CreateFieldInput>({
    resolver: zodResolver(createFieldSchema),
    defaultValues: {
      required: false,
      unique: false,
      hidden: false,
      quick_edit: false,
      show_in_table: false,
      readonly: false,
      translatable: false,
      section,
    },
  });

  const selectedType = watch("type");

  // Auto-gen state (managed outside RHF since it's nested options)
  const [autoGenFn, setAutoGenFn] = useState<string>("");
  const [autoGenSource, setAutoGenSource] = useState<string>("");
  const [autoGenTrigger, setAutoGenTrigger] = useState<AutoGenTrigger>("create");

  const { data: autoGenFunctions = [] } = useQuery<AutoGenFunction[]>({
    queryKey: ["autogen-functions"],
    queryFn: () => get<AutoGenFunction[]>("/autogen/functions"),
    staleTime: Infinity,
  });

  const selectedFnDef = autoGenFunctions.find((f) => f.id === autoGenFn);

  // When a function is selected, suggest hidden = true
  function handleFnSelect(fnId: string) {
    setAutoGenFn(fnId);
    if (fnId) setValue("hidden", true);
  }

  const mutation = useMutation({
    mutationFn: (data: CreateFieldInput) => {
      const options: Record<string, unknown> = { ...(data.options ?? {}) };
      if (autoGenFn) {
        options.auto_generate = {
          fn: autoGenFn,
          source_field: autoGenSource,
          trigger: autoGenTrigger,
        };
      }
      return createField(collectionName, { ...data, section, options });
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["collection", collectionName] });
      reset({
        required: false,
        unique: false,
        hidden: false,
        quick_edit: false,
        show_in_table: false,
        readonly: false,
        translatable: false,
        section,
      });
      setAutoGenFn("");
      setAutoGenSource("");
      setAutoGenTrigger("create");
      onClose();
    },
  });

  const panel = (
    <AnimatePresence>
      {open && (
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
            className="fixed right-0 top-0 z-50 flex h-full w-full max-w-sm flex-col bg-white dark:bg-slate-900 shadow-2xl"
          >
            <form
              onSubmit={handleSubmit((data) => mutation.mutate(data))}
              className="flex h-full flex-col"
            >
              <div className="flex shrink-0 items-center justify-between border-b border-slate-100 dark:border-white/10 px-6 py-5">
                <div>
                  <h2 className="text-base font-semibold text-slate-900 dark:text-slate-100">Add Field</h2>
                  <p className="mt-0.5 text-xs text-slate-400">
                    {collectionName}
                    <span className="mx-1 text-slate-300">·</span>
                    <span className="font-mono">{section}</span>
                  </p>
                </div>
                <button
                  type="button"
                  onClick={onClose}
                  className="rounded-lg p-1.5 text-slate-400 transition hover:bg-slate-100 dark:hover:bg-white/5 hover:text-slate-600 dark:hover:text-slate-200"
                >
                  <X size={18} />
                </button>
              </div>

              <div className="flex-1 space-y-6 overflow-y-auto px-6 py-6">
                {/* Type grid */}
                <div>
                  <label className="mb-3 block text-xs font-medium text-slate-700 dark:text-slate-200">
                    Type <span className="text-red-400">*</span>
                  </label>
                  <Controller
                    control={control}
                    name="type"
                    render={({ field }) => (
                      <div className="grid grid-cols-4 gap-2">
                        {FIELD_TYPES.map(({ type, label, Icon, color }) => {
                          const active = field.value === type;
                          return (
                            <button
                              key={type}
                              type="button"
                              onClick={() => field.onChange(type)}
                              className={`flex flex-col items-center gap-1.5 rounded-xl border p-3 text-center text-xs font-medium transition ${
                                active
                                  ? "border-brand-400 bg-brand-50 dark:bg-brand-500/15 text-brand-700 dark:text-brand-300"
                                  : "border-slate-100 dark:border-white/10 text-slate-500 dark:text-slate-400 hover:border-slate-200 dark:hover:border-white/10 hover:bg-slate-50 dark:hover:bg-white/5"
                              }`}
                            >
                              <span className={`flex h-8 w-8 items-center justify-center rounded-lg ${active ? "bg-brand-100 dark:bg-brand-500/20" : color}`}>
                                <Icon size={14} />
                              </span>
                              <span className="leading-tight">{label}</span>
                            </button>
                          );
                        })}
                      </div>
                    )}
                  />
                  {errors.type && <p className="mt-1.5 text-xs text-red-500 dark:text-red-400">{errors.type.message}</p>}
                </div>

                {/* Key */}
                <div>
                  <label className="mb-1.5 block text-xs font-medium text-slate-700 dark:text-slate-200">
                    Key <span className="text-red-400">*</span>
                  </label>
                  <input {...register("name")} placeholder="published_at" className="input font-mono text-sm" />
                  <p className="mt-1 text-xs text-slate-400">Stored column name — snake_case, no spaces</p>
                  {errors.name && <p className="mt-1 text-xs text-red-500 dark:text-red-400">{errors.name.message}</p>}
                </div>

                {/* Label */}
                <div>
                  <label className="mb-1.5 block text-xs font-medium text-slate-700 dark:text-slate-200">
                    Label <span className="text-red-400">*</span>
                  </label>
                  <input {...register("display_name")} placeholder="Published At" className="input" />
                  {errors.display_name && <p className="mt-1 text-xs text-red-500 dark:text-red-400">{errors.display_name.message}</p>}
                </div>

                {/* Flags */}
                <div className="space-y-3 rounded-xl border border-slate-100 dark:border-white/10 bg-slate-50 dark:bg-white/5 px-4 py-4">
                  {(
                    [
                      { key: "required",      label: "Required",      desc: "Field must have a value when creating items" },
                      { key: "unique",        label: "Unique",        desc: "No two items can share the same value" },
                      { key: "hidden",        label: "Hidden",        desc: "Hidden from the content editor; set via API or auto-generate" },
                      { key: "show_in_table", label: "Show in table", desc: "Show as a column on the content list table" },
                      { key: "quick_edit",    label: "Quick edit",    desc: "Show in the quick-edit aside panel on the content list" },
                      { key: "readonly",      label: "Read-only",     desc: "Value is set on create but cannot be updated afterwards" },
                      { key: "translatable",  label: "Translatable",  desc: "Marks this field for per-language content (metadata only — i18n storage TBD)" },
                    ] as const
                  ).map(({ key, label, desc }) => (
                    <label key={key} className="flex cursor-pointer items-start gap-3">
                      <input
                        type="checkbox"
                        {...register(key)}
                        className="mt-0.5 h-4 w-4 rounded border-slate-300 dark:border-white/10 text-brand-600 focus:ring-brand-400"
                      />
                      <div>
                        <p className="text-sm font-medium text-slate-800 dark:text-slate-100">{label}</p>
                        <p className="text-xs text-slate-400">{desc}</p>
                      </div>
                    </label>
                  ))}
                </div>

                {/* Auto-generate */}
                <div className="rounded-xl border border-slate-100 dark:border-white/10 bg-slate-50 dark:bg-white/5 px-4 py-4 space-y-4">
                  <div>
                    <p className="text-xs font-medium text-slate-700 dark:text-slate-200 mb-0.5">Auto-generate</p>
                    <p className="text-xs text-slate-400">
                      The server will compute this field's value automatically on save.
                    </p>
                  </div>

                  {/* Function selector */}
                  <div>
                    <label className="mb-1.5 block text-xs font-medium text-slate-600 dark:text-slate-300">Function</label>
                    <select
                      value={autoGenFn}
                      onChange={(e) => handleFnSelect(e.target.value)}
                      className="w-full rounded-lg border border-slate-200 dark:border-white/10 bg-white dark:bg-white/5 px-3 py-2 text-sm text-slate-800 dark:text-slate-100 focus:border-brand-400 dark:focus:border-brand-500/60 focus:outline-none focus:ring-2 focus:ring-brand-100 dark:focus:ring-brand-500/20"
                    >
                      <option value="">— none —</option>
                      {autoGenFunctions.map((f) => (
                        <option key={f.id} value={f.id}>{f.label}</option>
                      ))}
                    </select>
                    {selectedFnDef && (
                      <p className="mt-1 text-xs text-slate-400">{selectedFnDef.description}</p>
                    )}
                  </div>

                  {/* Source field — shown when the function needs one */}
                  {autoGenFn && selectedFnDef?.needs_source_field && (
                    <div>
                      <label className="mb-1.5 block text-xs font-medium text-slate-600 dark:text-slate-300">
                        Source field <span className="text-red-400">*</span>
                      </label>
                      <select
                        value={autoGenSource}
                        onChange={(e) => setAutoGenSource(e.target.value)}
                        className="w-full rounded-lg border border-slate-200 dark:border-white/10 bg-white dark:bg-white/5 px-3 py-2 text-sm text-slate-800 dark:text-slate-100 focus:border-brand-400 dark:focus:border-brand-500/60 focus:outline-none focus:ring-2 focus:ring-brand-100 dark:focus:ring-brand-500/20"
                      >
                        <option value="">— choose a field —</option>
                        {existingFields
                          .filter((f) => !f.is_system)
                          .map((f) => (
                            <option key={f.id} value={f.name}>
                              {f.display_name} ({f.name})
                            </option>
                          ))}
                      </select>
                      <p className="mt-1 text-xs text-slate-400">
                        The function reads this field's value to compute the result.
                      </p>
                    </div>
                  )}

                  {/* Trigger */}
                  {autoGenFn && (
                    <div>
                      <label className="mb-1.5 block text-xs font-medium text-slate-600 dark:text-slate-300">Trigger</label>
                      <div className="flex gap-2">
                        {(["create", "always"] as const).map((t) => (
                          <button
                            key={t}
                            type="button"
                            onClick={() => setAutoGenTrigger(t)}
                            className={`flex-1 rounded-lg border py-2 text-xs font-medium transition ${
                              autoGenTrigger === t
                                ? "border-brand-400 bg-brand-50 dark:bg-brand-500/15 text-brand-700 dark:text-brand-300"
                                : "border-slate-200 dark:border-white/10 text-slate-500 dark:text-slate-400 hover:bg-white dark:hover:bg-white/10"
                            }`}
                          >
                            {t === "create" ? "On create only" : "On create & update"}
                          </button>
                        ))}
                      </div>
                    </div>
                  )}
                </div>

                {mutation.isError && (
                  <p className="rounded-xl bg-red-50 dark:bg-red-500/10 px-3 py-2.5 text-sm text-red-600 dark:text-red-400">
                    {(mutation.error as Error).message}
                  </p>
                )}
              </div>

              <div className="flex shrink-0 items-center justify-end gap-2 border-t border-slate-100 dark:border-white/10 px-6 py-4">
                <Button type="button" variant="ghost" onClick={onClose}>Cancel</Button>
                <Button
                  type="submit"
                  disabled={
                    mutation.isPending ||
                    !selectedType ||
                    (!!autoGenFn && !!selectedFnDef?.needs_source_field && !autoGenSource)
                  }
                >
                  {mutation.isPending ? "Adding…" : "Add Field"}
                </Button>
              </div>
            </form>
          </motion.aside>
        </>
      )}
    </AnimatePresence>
  );

  return createPortal(panel, document.body);
}

// ── Edit Field panel ──────────────────────────────────────────────────────────

function EditFieldPanel({
  open,
  field,
  collectionName,
  onClose,
}: {
  open: boolean;
  field: Field;
  collectionName: string;
  onClose: () => void;
}) {
  const queryClient = useQueryClient();
  const fieldKey = field.id;

  // Scalar flags — seeded from the field prop
  const [displayName, setDisplayName] = useState(field.display_name);
  const [required, setRequired]       = useState(field.required);
  const [unique, setUnique]           = useState(field.unique);
  const [hidden, setHidden]           = useState(field.hidden);
  const [quickEdit, setQuickEdit]     = useState(field.quick_edit);
  const [showInTable, setShowInTable] = useState(field.show_in_table ?? false);
  const [readonly, setReadonly]       = useState(field.readonly);
  const [translatable, setTranslatable] = useState(field.translatable);

  // Select/multiselect choices
  type Choice = { value: string; label: string };
  const [choices, setChoices] = useState<Choice[]>(
    (field.options?.choices as Choice[] | undefined) ?? []
  );
  const [newValue, setNewValue] = useState("");
  const [newLabel, setNewLabel] = useState("");

  // Auto-generate — read existing config from field.options
  const existingAg = field.options?.auto_generate as
    | { fn?: string; source_field?: string; trigger?: string }
    | undefined;
  const [autoGenFn, setAutoGenFn]           = useState(existingAg?.fn ?? "");
  const [autoGenSource, setAutoGenSource]   = useState(existingAg?.source_field ?? "");
  const [autoGenTrigger, setAutoGenTrigger] = useState<AutoGenTrigger>(
    (existingAg?.trigger as AutoGenTrigger) ?? "create"
  );

  const { data: autoGenFunctions = [] } = useQuery<AutoGenFunction[]>({
    queryKey: ["autogen-functions"],
    queryFn: () => get<AutoGenFunction[]>("/autogen/functions"),
    staleTime: Infinity,
  });

  const selectedFnDef = autoGenFunctions.find((f) => f.id === autoGenFn);

  // Fetch existing fields for source-field picker (cache hit)
  const { data: rawColl } = useQuery({
    queryKey: ["collection", collectionName],
    queryFn: () => get<Collection>(`/collections/${collectionName}`),
    staleTime: 60_000,
  });
  const existingFields = allFields(rawColl).filter(
    (f) => !f.is_system && f.name !== field.name
  );

  const mutation = useMutation({
    mutationFn: () => {
      // Build options: keep all existing keys, merge auto_generate
      const updatedOptions: Record<string, unknown> = {};
      if (field.type === "select" || field.type === "multiselect") {
        updatedOptions.choices = choices;
      }
      if (autoGenFn) {
        updatedOptions.auto_generate = {
          fn: autoGenFn,
          source_field: autoGenSource,
          trigger: autoGenTrigger,
        };
      } else {
        // Explicitly pass null to clear any existing auto_generate
        updatedOptions.auto_generate = null;
      }
      return updateField(collectionName, field.name, {
        display_name: displayName,
        required,
        unique,
        hidden,
        quick_edit: quickEdit,
        show_in_table: showInTable,
        readonly,
        translatable,
        options: updatedOptions,
      });
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["collection", collectionName] });
      onClose();
    },
  });

  const isSelect = field.type === "select" || field.type === "multiselect";
  const typeConfig = FIELD_TYPES.find((t) => t.type === field.type);

  const panel = (
    <AnimatePresence>
      {open && (
        <>
          <motion.div
            key={`edit-backdrop-${fieldKey}`}
            initial={{ opacity: 0 }} animate={{ opacity: 1 }} exit={{ opacity: 0 }}
            transition={{ duration: 0.18 }}
            className="fixed inset-0 z-40 bg-black/20 backdrop-blur-[2px]"
            onClick={onClose}
          />
          <motion.aside
            key={`edit-panel-${fieldKey}`}
            initial={{ x: "100%" }} animate={{ x: 0 }} exit={{ x: "100%" }}
            transition={{ type: "spring", damping: 28, stiffness: 280 }}
            className="fixed right-0 top-0 z-50 flex h-full w-full max-w-sm flex-col bg-white dark:bg-slate-900 shadow-2xl"
          >
            {/* Header */}
            <div className="flex shrink-0 items-center justify-between border-b border-slate-100 dark:border-white/10 px-6 py-5">
              <div>
                <h2 className="text-base font-semibold text-slate-900 dark:text-slate-100">Edit Field</h2>
                <div className="mt-1 flex items-center gap-2">
                  <span className="font-mono text-xs text-slate-400">{field.name}</span>
                  {typeConfig && (
                    <span className={`inline-flex items-center gap-1 rounded px-1.5 py-0.5 text-[10px] font-medium ${typeConfig.color}`}>
                      <typeConfig.Icon size={9} />
                      {field.type}
                    </span>
                  )}
                </div>
              </div>
              <button type="button" onClick={onClose}
                className="rounded-lg p-1.5 text-slate-400 transition hover:bg-slate-100 dark:hover:bg-white/5 hover:text-slate-600 dark:hover:text-slate-200">
                <X size={18} />
              </button>
            </div>

            {/* Body */}
            <div className="flex-1 space-y-5 overflow-y-auto px-6 py-6">

              {/* Label */}
              <div>
                <label className="mb-1.5 block text-xs font-medium text-slate-700 dark:text-slate-200">
                  Label <span className="text-red-400">*</span>
                </label>
                <input
                  className="input"
                  value={displayName}
                  onChange={(e) => setDisplayName(e.target.value)}
                  placeholder="Display Name"
                />
              </div>

              {/* Flags — aligned with create panel */}
              <div className="space-y-3 rounded-xl border border-slate-100 dark:border-white/10 bg-slate-50 dark:bg-white/5 px-4 py-4">
                {(
                  [
                    { key: "required",      label: "Required",      desc: "Field must have a value when creating items",                              value: required,     set: setRequired },
                    { key: "unique",        label: "Unique",        desc: "No two items can share the same value (applies DDL constraint)",           value: unique,       set: setUnique },
                    { key: "hidden",        label: "Hidden",        desc: "Hidden from the content editor; set via API or auto-generate",             value: hidden,       set: setHidden },
                    { key: "show_in_table", label: "Show in table", desc: "Show as a column on the content list table",                               value: showInTable,  set: setShowInTable },
                    { key: "quick_edit",    label: "Quick edit",    desc: "Show in the quick-edit aside panel on the content list",                   value: quickEdit,    set: setQuickEdit },
                    { key: "readonly",      label: "Read-only",     desc: "Value is set on create but cannot be updated afterwards",                  value: readonly,     set: setReadonly },
                    { key: "translatable",  label: "Translatable",  desc: "Marks this field for per-language content (metadata — i18n storage TBD)", value: translatable, set: setTranslatable },
                  ] as const
                ).map(({ key, label, desc, value, set }) => (
                  <label key={key} className="flex cursor-pointer items-start gap-3">
                    <input
                      type="checkbox"
                      checked={value}
                      onChange={(e) => set(e.target.checked)}
                      className="mt-0.5 h-4 w-4 rounded border-slate-300 dark:border-white/10 text-brand-600 focus:ring-brand-400"
                    />
                    <div>
                      <p className="text-sm font-medium text-slate-800 dark:text-slate-100">{label}</p>
                      <p className="text-xs text-slate-400">{desc}</p>
                    </div>
                  </label>
                ))}
              </div>

              {/* Choices — select / multiselect only */}
              {isSelect && (
                <div>
                  <p className="mb-2 text-xs font-medium text-slate-700 dark:text-slate-200">Choices</p>
                  <div className="space-y-1.5">
                    {choices.map((c, i) => (
                      <div key={i} className="flex items-center gap-2">
                        <input
                          className="input flex-1 font-mono text-xs"
                          value={c.value}
                          onChange={(e) => setChoices((prev) => prev.map((x, j) => j === i ? { ...x, value: e.target.value } : x))}
                          placeholder="value"
                        />
                        <input
                          className="input flex-1 text-xs"
                          value={c.label}
                          onChange={(e) => setChoices((prev) => prev.map((x, j) => j === i ? { ...x, label: e.target.value } : x))}
                          placeholder="Label"
                        />
                        <button
                          type="button"
                          onClick={() => setChoices((prev) => prev.filter((_, j) => j !== i))}
                          className="flex h-7 w-7 shrink-0 items-center justify-center rounded-lg text-slate-300 transition hover:bg-red-50 dark:hover:bg-red-500/10 hover:text-red-500 dark:hover:text-red-400"
                        >
                          <X size={13} />
                        </button>
                      </div>
                    ))}
                  </div>
                  <div className="mt-2 flex items-center gap-2">
                    <input className="input flex-1 font-mono text-xs" value={newValue}
                      onChange={(e) => setNewValue(e.target.value)} placeholder="value" />
                    <input className="input flex-1 text-xs" value={newLabel}
                      onChange={(e) => setNewLabel(e.target.value)} placeholder="Label"
                      onKeyDown={(e) => {
                        if (e.key === "Enter" && newValue && newLabel) {
                          e.preventDefault();
                          setChoices((p) => [...p, { value: newValue, label: newLabel }]);
                          setNewValue(""); setNewLabel("");
                        }
                      }}
                    />
                    <button type="button" disabled={!newValue || !newLabel}
                      onClick={() => { setChoices((p) => [...p, { value: newValue, label: newLabel }]); setNewValue(""); setNewLabel(""); }}
                      className="flex h-7 w-7 shrink-0 items-center justify-center rounded-lg border border-slate-200 dark:border-white/10 text-slate-500 dark:text-slate-400 transition hover:bg-brand-50 dark:hover:bg-brand-500/15 hover:text-brand-600 dark:hover:text-brand-300 disabled:opacity-40"
                    >
                      <Plus size={13} />
                    </button>
                  </div>
                  <p className="mt-1.5 text-xs text-slate-400">Press Enter or + to add a choice.</p>
                </div>
              )}

              {/* Auto-generate */}
              <div className="rounded-xl border border-slate-100 dark:border-white/10 bg-slate-50 dark:bg-white/5 px-4 py-4 space-y-4">
                <div>
                  <p className="text-xs font-medium text-slate-700 dark:text-slate-200 mb-0.5">Auto-generate</p>
                  <p className="text-xs text-slate-400">
                    Server computes this field automatically on save.
                  </p>
                </div>
                <div>
                  <label className="mb-1.5 block text-xs font-medium text-slate-600 dark:text-slate-300">Function</label>
                  <select
                    value={autoGenFn}
                    onChange={(e) => { setAutoGenFn(e.target.value); if (e.target.value) setHidden(true); }}
                    className="w-full rounded-lg border border-slate-200 dark:border-white/10 bg-white dark:bg-white/5 px-3 py-2 text-sm text-slate-800 dark:text-slate-100 focus:border-brand-400 dark:focus:border-brand-500/60 focus:outline-none focus:ring-2 focus:ring-brand-100 dark:focus:ring-brand-500/20"
                  >
                    <option value="">— none —</option>
                    {autoGenFunctions.map((f) => (
                      <option key={f.id} value={f.id}>{f.label}</option>
                    ))}
                  </select>
                  {selectedFnDef && (
                    <p className="mt-1 text-xs text-slate-400">{selectedFnDef.description}</p>
                  )}
                </div>
                {autoGenFn && selectedFnDef?.needs_source_field && (
                  <div>
                    <label className="mb-1.5 block text-xs font-medium text-slate-600 dark:text-slate-300">
                      Source field <span className="text-red-400">*</span>
                    </label>
                    <select
                      value={autoGenSource}
                      onChange={(e) => setAutoGenSource(e.target.value)}
                      className="w-full rounded-lg border border-slate-200 dark:border-white/10 bg-white dark:bg-white/5 px-3 py-2 text-sm text-slate-800 dark:text-slate-100 focus:border-brand-400 dark:focus:border-brand-500/60 focus:outline-none focus:ring-2 focus:ring-brand-100 dark:focus:ring-brand-500/20"
                    >
                      <option value="">— choose a field —</option>
                      {existingFields.map((f) => (
                        <option key={f.id} value={f.name}>{f.display_name} ({f.name})</option>
                      ))}
                    </select>
                  </div>
                )}
                {autoGenFn && (
                  <div>
                    <label className="mb-1.5 block text-xs font-medium text-slate-600 dark:text-slate-300">Trigger</label>
                    <div className="flex gap-2">
                      {(["create", "always"] as const).map((t) => (
                        <button key={t} type="button" onClick={() => setAutoGenTrigger(t)}
                          className={`flex-1 rounded-lg border py-2 text-xs font-medium transition ${
                            autoGenTrigger === t
                              ? "border-brand-400 bg-brand-50 dark:bg-brand-500/15 text-brand-700 dark:text-brand-300"
                              : "border-slate-200 dark:border-white/10 text-slate-500 dark:text-slate-400 hover:bg-white dark:hover:bg-white/10"
                          }`}
                        >
                          {t === "create" ? "On create only" : "On create & update"}
                        </button>
                      ))}
                    </div>
                  </div>
                )}
              </div>

              {mutation.isError && (
                <div className="flex items-start gap-2 rounded-xl bg-red-50 dark:bg-red-500/10 px-3 py-2.5 text-sm text-red-600 dark:text-red-400">
                  <AlertTriangle size={14} className="mt-0.5 shrink-0" />
                  {(mutation.error as Error).message}
                </div>
              )}
            </div>

            {/* Footer */}
            <div className="flex shrink-0 items-center justify-end gap-2 border-t border-slate-100 dark:border-white/10 px-6 py-4">
              <Button type="button" variant="ghost" onClick={onClose}>Cancel</Button>
              <Button
                onClick={() => mutation.mutate()}
                disabled={
                  mutation.isPending ||
                  !displayName.trim() ||
                  (!!autoGenFn && !!selectedFnDef?.needs_source_field && !autoGenSource)
                }
              >
                {mutation.isPending ? "Saving…" : "Save Changes"}
              </Button>
            </div>
          </motion.aside>
        </>
      )}
    </AnimatePresence>
  );

  return createPortal(panel, document.body);
}

// ── Confirm dialog ────────────────────────────────────────────────────────────

function ConfirmDialog({
  open, title, description, confirmLabel,
  onConfirm, onCancel, loading, danger,
}: {
  open: boolean;
  title: string;
  description: string;
  confirmLabel: string;
  onConfirm: () => void;
  onCancel: () => void;
  loading?: boolean;
  danger?: boolean;
}) {
  return (
    <Dialog.Root open={open} onOpenChange={(v) => !v && onCancel()}>
      <Dialog.Portal>
        <Dialog.Overlay className="fixed inset-0 z-40 bg-black/20 backdrop-blur-[2px] animate-fade-in" />
        <Dialog.Content className="fixed left-1/2 top-1/2 z-50 w-full max-w-sm -translate-x-1/2 -translate-y-1/2 rounded-2xl bg-white dark:bg-slate-900 p-6 shadow-glass-lg animate-slide-up">
          <Dialog.Title className="text-base font-semibold text-slate-900 dark:text-slate-100">{title}</Dialog.Title>
          <p className="mt-2 text-sm text-slate-500 dark:text-slate-400">{description}</p>
          <div className="mt-5 flex justify-end gap-2">
            <Button variant="ghost" onClick={onCancel} disabled={loading}>Cancel</Button>
            <button
              onClick={onConfirm}
              disabled={loading}
              className={`inline-flex items-center gap-2 rounded-xl px-4 py-2.5 text-sm font-semibold text-white transition disabled:opacity-50 ${
                danger ? "bg-red-500 hover:bg-red-600" : "bg-brand-600 hover:bg-brand-700"
              }`}
            >
              {loading ? "Deleting…" : confirmLabel}
            </button>
          </div>
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  );
}
