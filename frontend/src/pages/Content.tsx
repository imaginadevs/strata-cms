/**
 * Content.tsx — Items browser + CRUD for a specific collection.
 * Routes:
 *   /content/:collection          — list
 *   /content/:collection/create   — full-page create form
 *
 * Supports nested child relations (item + subitems): when a collection is the
 * parent of one or more one-to-many relations (e.g. pages → pages_blocks), the
 * edit/create forms render a SubitemsEditor per relation and save the whole graph
 * in one request via the backend's nested-write API.
 */
import { useState } from "react";
import { createPortal } from "react-dom";
import { useTranslation } from "react-i18next";
import { useParams, useNavigate } from "@tanstack/react-router";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { motion, AnimatePresence } from "framer-motion";
import {
  Database, Plus, X, Trash2, Pencil, AlertTriangle, Save, ChevronLeft, Maximize2, ArrowUpDown,
} from "lucide-react";
import { get, post, patch, del } from "@/lib/api";
import { Card } from "@/components/ui/Card";
import { Button } from "@/components/ui/Button";
import { formatDate, itemId as toItemId } from "@/lib/utils";
import {
  FieldGroup,
  LanguageSelector,
  useAvailableLanguages,
} from "@/components/content/fields";
import { SubitemsEditor } from "@/components/content/SubitemsEditor";
import { SectionReorderPanel } from "@/components/collections/SectionReorderPanel";
import {
  childRelationsOf,
  getItemDeep,
  listRelations,
  withRelationOptions,
  type ChildRelation,
} from "@/lib/relational";
import { allFields, isSectionVisible, sortedSections, visibleSectionFields } from "@/lib/collection";
import type { Collection, Field, PaginatedResponse } from "@/types";

// ── API helpers ───────────────────────────────────────────────────────────────

const createItem = (coll: string, data: Record<string, unknown>) =>
  post<Record<string, unknown>>(`/items/${coll}`, data);

const updateItem = (coll: string, id: string, data: Record<string, unknown>) =>
  patch<Record<string, unknown>>(`/items/${coll}/${id}`, data);

const deleteItem = (coll: string, id: string) =>
  del(`/items/${coll}/${id}`);

// buildPayload assembles the flat field values plus any nested child arrays, with
// the parent language stamped onto every child row.
function buildPayload(
  fields: Field[],
  values: Record<string, unknown>,
  childRels: ChildRelation[],
  subitems: Record<string, Record<string, unknown>[]>,
  language: string,
): Record<string, unknown> {
  const payload: Record<string, unknown> = {};
  for (const f of fields) {
    const v = values[f.name];
    payload[f.name] = v === "" ? null : v;
  }
  for (const c of childRels) {
    payload[c.childCollection] = (subitems[c.childCollection] ?? []).map((row) => ({
      ...row,
      language,
    }));
  }
  return payload;
}

// ── Item panel (edit slide-in) ────────────────────────────────────────────────

type PanelMode = { kind: "create" } | { kind: "edit"; item: Record<string, unknown> };

// ItemPanel is a thin gate: it discovers the collection's child relations and, for
// edit mode, loads the item with those relations expanded (?deep=) before mounting
// the form. Gating on load lets the form seed its state synchronously from final
// data (no effect juggling).
function ItemPanel({
  collectionName,
  fields,
  mode,
  onClose,
  onSaved,
}: {
  collectionName: string;
  fields: Field[];
  mode: PanelMode;
  onClose: () => void;
  onSaved: () => void;
}) {
  const { data: relations, isLoading: relLoading } = useQuery({
    queryKey: ["relations", collectionName],
    queryFn: () => listRelations(collectionName),
  });
  const childRels = childRelationsOf(relations ?? [], collectionName);
  const childNames = childRels.map((c) => c.childCollection);

  const resolvedId = mode.kind === "edit" ? toItemId(mode.item.id) : "";
  const lang = mode.kind === "edit" ? String(mode.item.language ?? "") : "";
  const needDeep = mode.kind === "edit" && childRels.length > 0;

  const { data: deepItem, isLoading: deepLoading } = useQuery({
    queryKey: ["item-deep", collectionName, resolvedId, childNames.join(",")],
    queryFn: () => getItemDeep(collectionName, resolvedId, childNames, lang),
    enabled: needDeep && !!resolvedId,
  });

  const ready = !relLoading && (!needDeep || !deepLoading);
  const initialItem: Record<string, unknown> =
    mode.kind === "edit" ? deepItem ?? mode.item : {};

  return createPortal(
    <>
      {/* Backdrop */}
      <motion.div
        key="backdrop"
        className="fixed inset-0 z-40 bg-slate-900/20 backdrop-blur-[2px]"
        initial={{ opacity: 0 }}
        animate={{ opacity: 1 }}
        exit={{ opacity: 0 }}
        transition={{ duration: 0.18 }}
        onClick={onClose}
      />

      {/* Panel */}
      <motion.aside
        key="panel"
        className="fixed right-0 top-0 z-50 flex h-full w-full max-w-md flex-col bg-white dark:bg-slate-900 shadow-2xl"
        initial={{ x: "100%" }}
        animate={{ x: 0 }}
        exit={{ x: "100%" }}
        transition={{ type: "spring", damping: 28, stiffness: 280 }}
      >
        {!ready ? (
          <div className="flex h-full w-full items-center justify-center">
            <div className="h-8 w-8 animate-spin rounded-full border-2 border-brand-200 border-t-brand-600" />
          </div>
        ) : (
          <ItemPanelForm
            collectionName={collectionName}
            fields={withRelationOptions(fields, relations ?? [], collectionName)}
            mode={mode}
            childRels={childRels}
            initialItem={initialItem}
            onClose={onClose}
            onSaved={onSaved}
          />
        )}
      </motion.aside>
    </>,
    document.body
  );
}

function ItemPanelForm({
  collectionName,
  fields,
  mode,
  childRels,
  initialItem,
  onClose,
  onSaved,
}: {
  collectionName: string;
  fields: Field[];
  mode: PanelMode;
  childRels: ChildRelation[];
  initialItem: Record<string, unknown>;
  onClose: () => void;
  onSaved: () => void;
}) {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const availableLanguages = useAvailableLanguages();
  const [language, setLanguage] = useState(
    mode.kind === "edit"
      ? String(initialItem.language ?? availableLanguages[0])
      : availableLanguages[0],
  );

  const [values, setValues] = useState<Record<string, unknown>>(() => {
    const initial: Record<string, unknown> = {};
    for (const f of fields) {
      initial[f.name] = mode.kind === "edit" ? initialItem[f.name] ?? null : null;
    }
    return initial;
  });
  const [subitems, setSubitems] = useState<Record<string, Record<string, unknown>[]>>(() => {
    const seed: Record<string, Record<string, unknown>[]> = {};
    for (const c of childRels) {
      const arr = initialItem[c.childCollection];
      seed[c.childCollection] = Array.isArray(arr)
        ? (arr as Record<string, unknown>[])
        : [];
    }
    return seed;
  });
  const [fieldErrors, setFieldErrors] = useState<Record<string, string>>({});
  const [serverError, setServerError] = useState<string | null>(null);

  const saveMut = useMutation({
    mutationFn: (data: Record<string, unknown>) =>
      mode.kind === "create"
        ? createItem(collectionName, { ...data, language })
        : updateItem(collectionName, toItemId(mode.item.id), { ...data, language }),
    onSuccess: () => {
      onSaved();
      onClose();
    },
    onError: (e: Error) => setServerError(e.message),
  });

  function validate(): boolean {
    const errors: Record<string, string> = {};
    for (const f of fields) {
      if (f.required && (values[f.name] == null || values[f.name] === "")) {
        errors[f.name] = t("content.fieldRequired", { field: f.display_name });
      }
    }
    setFieldErrors(errors);
    return Object.keys(errors).length === 0;
  }

  function handleChange(name: string, v: unknown) {
    setValues((prev) => ({ ...prev, [name]: v }));
    setFieldErrors((prev) => ({ ...prev, [name]: "" }));
  }

  function onSubmit(e: React.FormEvent) {
    e.preventDefault();
    if (!validate()) return;
    setServerError(null);
    saveMut.mutate(buildPayload(fields, values, childRels, subitems, language));
  }

  return (
    <>
      {/* Header */}
      <div className="flex shrink-0 items-center justify-between border-b border-slate-100 dark:border-white/10 px-5 py-4">
        <div>
          <p className="text-xs font-semibold uppercase tracking-widest text-slate-400">
            {collectionName}
          </p>
          <h2 className="text-base font-bold text-slate-900 dark:text-slate-100">
            {mode.kind === "create" ? t("content.newItem") : t("content.editItem")}
          </h2>
        </div>
        <div className="flex items-center gap-2">
          {mode.kind === "edit" && (
            <button
              type="button"
              onClick={() => {
                onClose();
                navigate({
                  to: "/content/$collection/$id/edit",
                  params: { collection: collectionName, id: toItemId(mode.item.id) },
                });
              }}
              title={t("content.openFullEditor")}
              className="flex h-8 items-center gap-1.5 rounded-lg px-2.5 text-xs font-medium text-slate-500 dark:text-slate-400 transition hover:bg-slate-100 dark:hover:bg-white/5 hover:text-slate-700 dark:hover:text-slate-100"
            >
              <Maximize2 size={13} />
              {t("content.fullEditor")}
            </button>
          )}
          <LanguageSelector
            value={language}
            onChange={setLanguage}
            languages={availableLanguages}
          />
          <button
            type="button"
            onClick={onClose}
            className="flex h-8 w-8 items-center justify-center rounded-lg text-slate-400 transition hover:bg-slate-100 dark:hover:bg-white/5 hover:text-slate-700 dark:hover:text-slate-100"
          >
            <X size={16} />
          </button>
        </div>
      </div>

      {/* Fields */}
      <form
        id="item-form"
        onSubmit={onSubmit}
        className="flex-1 space-y-5 overflow-y-auto px-5 py-4"
      >
        {fields.length === 0 ? (
          <p className="text-sm text-slate-400">
            {t("content.noFields")}
          </p>
        ) : (
          <FieldGroup
            fields={fields}
            values={values}
            fieldErrors={fieldErrors}
            onChange={handleChange}
          />
        )}

        {childRels.map((c) => (
          <SubitemsEditor
            key={c.childCollection}
            child={c}
            initialRows={subitems[c.childCollection] ?? []}
            onChange={(rows) =>
              setSubitems((prev) => ({ ...prev, [c.childCollection]: rows }))
            }
          />
        ))}

        {serverError && (
          <div className="flex items-start gap-2 rounded-lg bg-red-50 dark:bg-red-500/10 p-3 text-sm text-red-700 dark:text-red-400">
            <AlertTriangle size={14} className="mt-0.5 shrink-0" />
            {serverError}
          </div>
        )}
      </form>

      {/* Footer */}
      <div className="flex shrink-0 items-center justify-between border-t border-slate-100 dark:border-white/10 px-5 py-4">
        <Button variant="ghost" type="button" onClick={onClose}>
          {t("common.cancel")}
        </Button>
        <Button type="submit" form="item-form" disabled={saveMut.isPending}>
          <Save size={14} />
          {saveMut.isPending
            ? t("common.saving")
            : mode.kind === "create"
            ? t("content.createItem")
            : t("content.saveChanges")}
        </Button>
      </div>
    </>
  );
}

// ── Delete confirmation dialog ────────────────────────────────────────────────

function DeleteConfirm({
  collectionName,
  itemId,
  onClose,
  onDeleted,
}: {
  collectionName: string;
  itemId: string;
  onClose: () => void;
  onDeleted: () => void;
}) {
  const { t } = useTranslation();
  const deleteMut = useMutation({
    mutationFn: () => deleteItem(collectionName, itemId),
    onSuccess: () => {
      onDeleted();
      onClose();
    },
  });

  return createPortal(
    <>
      <motion.div
        className="fixed inset-0 z-40 bg-slate-900/30 backdrop-blur-[2px]"
        initial={{ opacity: 0 }}
        animate={{ opacity: 1 }}
        exit={{ opacity: 0 }}
        onClick={onClose}
      />
      <motion.div
        className="fixed left-1/2 top-1/2 z-50 w-full max-w-sm -translate-x-1/2 -translate-y-1/2 rounded-2xl bg-white dark:bg-slate-900 p-6 shadow-2xl"
        initial={{ opacity: 0, scale: 0.95 }}
        animate={{ opacity: 1, scale: 1 }}
        exit={{ opacity: 0, scale: 0.95 }}
        transition={{ duration: 0.15 }}
      >
        <div className="mb-4 flex h-10 w-10 items-center justify-center rounded-full bg-red-50 dark:bg-red-500/10">
          <Trash2 size={18} className="text-red-500 dark:text-red-400" />
        </div>
        <h3 className="mb-1 text-base font-bold text-slate-900 dark:text-slate-100">{t("content.deleteItemTitle")}</h3>
        <p className="mb-5 text-sm text-slate-500 dark:text-slate-400">
          {t("content.deletePermanent")}
        </p>
        <div className="flex items-center justify-end gap-2">
          <Button variant="ghost" onClick={onClose}>
            {t("common.cancel")}
          </Button>
          <Button
            variant="danger"
            onClick={() => deleteMut.mutate()}
            disabled={deleteMut.isPending}
          >
            {deleteMut.isPending ? t("common.deleting") : t("common.delete")}
          </Button>
        </div>
      </motion.div>
    </>,
    document.body
  );
}

// ── Content page (list) ───────────────────────────────────────────────────────

type PanelState = { open: false } | { open: true; mode: PanelMode };
type DeleteState = { open: false } | { open: true; itemId: string };

export default function ContentPage() {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const { collection: collectionName } = useParams({ strict: false }) as {
    collection: string;
  };
  const queryClient = useQueryClient();

  const { data: schema, isLoading: schemaLoading } = useQuery({
    queryKey: ["collection", collectionName],
    queryFn: () => get<Collection>(`/collections/${collectionName}`),
    enabled: !!collectionName,
  });

  const { data: items, isLoading: itemsLoading } = useQuery({
    queryKey: ["items", collectionName],
    queryFn: () =>
      get<PaginatedResponse<Record<string, unknown>>>(`/items/${collectionName}`),
    enabled: !!collectionName,
  });

  const [panel, setPanel] = useState<PanelState>({ open: false });
  const [delState, setDelState] = useState<DeleteState>({ open: false });

  const isLoading = schemaLoading || itemsLoading;

  // All non-system fields (for the panel) — flattened from sections
  const allUserFields = allFields(schema).filter((f) => !f.is_system);
  // Table columns: only fields explicitly marked show_in_table
  const userFields = allUserFields.filter((f) => f.show_in_table === true);
  // Quick-edit fields drive the aside panel. If a collection has none flagged,
  // the edit action skips the aside and opens the full editor instead.
  const quickEditFields = allUserFields.filter((f) => f.quick_edit);

  function handleEditRow(row: Record<string, unknown>) {
    if (quickEditFields.length > 0) {
      setPanel({ open: true, mode: { kind: "edit", item: row } });
    } else {
      navigate({
        to: "/content/$collection/$id/edit",
        params: { collection: collectionName, id: toItemId(row.id) },
      });
    }
  }

  const rows = items?.data ?? [];
  const total = items?.total ?? 0;
  const iconColor = schema?.color ?? "#6366f1";

  function invalidate() {
    queryClient.invalidateQueries({ queryKey: ["items", collectionName] });
  }

  return (
    <div className="space-y-5">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-3">
          <div
            className="flex h-9 w-9 items-center justify-center rounded-xl"
            style={{ backgroundColor: `${iconColor}18` }}
          >
            <Database size={16} style={{ color: iconColor }} />
          </div>
          <div>
            <h1 className="text-xl font-bold text-slate-900 dark:text-slate-100">
              {schema?.display_name ?? collectionName}
            </h1>
            <p className="text-xs text-slate-400">
              {t("content.itemCount", { count: total })}
            </p>
          </div>
        </div>

        <Button
          onClick={() =>
            navigate({
              to: "/content/$collection/create",
              params: { collection: collectionName },
            })
          }
        >
          <Plus size={15} />
          {t("content.addItem")}
        </Button>
      </div>

      {/* Table */}
      <Card className="overflow-hidden p-0">
        {isLoading ? (
          <div className="animate-pulse space-y-3 p-5">
            {Array.from({ length: 5 }).map((_, i) => (
              <div key={i} className="h-10 rounded-lg bg-slate-100 dark:bg-white/5" />
            ))}
          </div>
        ) : userFields.length === 0 ? (
          <div className="py-16 text-center">
            <p className="font-medium text-slate-900 dark:text-slate-100">{t("content.noTableColumns")}</p>
            <p className="mt-1 text-sm text-slate-400">
              {t("content.noTableColumnsHint")}
            </p>
          </div>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b border-slate-100 dark:border-white/10 text-left text-xs font-medium text-slate-400">
                  {userFields.map((f) => (
                    <th key={f.id} className="whitespace-nowrap px-5 py-3">
                      {f.display_name}
                    </th>
                  ))}
                  <th className="whitespace-nowrap px-5 py-3">{t("common.created")}</th>
                  <th className="w-16 px-5 py-3" />
                </tr>
              </thead>
              <tbody className="divide-y divide-slate-50 dark:divide-white/10">
                {rows.length === 0 ? (
                  <tr>
                    <td
                      colSpan={userFields.length + 2}
                      className="py-14 text-center text-slate-400"
                    >
                      {t("content.noItems")} —{" "}
                      <button
                        className="text-brand-600 dark:text-brand-300 hover:underline"
                        onClick={() =>
                          navigate({
                            to: "/content/$collection/create",
                            params: { collection: collectionName },
                          })
                        }
                      >
                        {t("content.createFirstOne")}
                      </button>
                    </td>
                  </tr>
                ) : (
                  rows.map((row, i) => (
                    <tr
                      key={toItemId(row.id) || String(i)}
                      className="group transition hover:bg-slate-50/60 dark:hover:bg-white/5"
                    >
                      {userFields.map((f) => (
                        <td
                          key={f.id}
                          className="max-w-[200px] truncate px-5 py-3 text-slate-700 dark:text-slate-200"
                        >
                          {row[f.name] == null ? (
                            <span className="text-slate-300">—</span>
                          ) : f.type === "boolean" ? (
                            <span
                              className={`inline-block rounded px-1.5 py-0.5 text-[10px] font-semibold ${
                                row[f.name]
                                  ? "bg-green-50 dark:bg-emerald-500/10 text-green-700 dark:text-emerald-300"
                                  : "bg-slate-100 dark:bg-white/5 text-slate-500 dark:text-slate-400"
                              }`}
                            >
                              {row[f.name] ? t("common.yes") : t("common.no")}
                            </span>
                          ) : (
                            String(row[f.name])
                          )}
                        </td>
                      ))}
                      <td className="whitespace-nowrap px-5 py-3 text-slate-400">
                        {row.created_at ? formatDate(String(row.created_at)) : "—"}
                      </td>
                      <td className="px-5 py-3 text-right">
                        <div className="flex items-center justify-end gap-1 opacity-0 transition group-hover:opacity-100">
                          <button
                            onClick={() => handleEditRow(row)}
                            className="flex h-7 w-7 items-center justify-center rounded-lg text-slate-400 transition hover:bg-slate-100 dark:hover:bg-white/5 hover:text-slate-700 dark:hover:text-slate-100"
                            title={t("common.edit")}
                          >
                            <Pencil size={13} />
                          </button>
                          <button
                            onClick={() =>
                              setDelState({ open: true, itemId: toItemId(row.id) })
                            }
                            className="flex h-7 w-7 items-center justify-center rounded-lg text-slate-400 transition hover:bg-red-50 dark:hover:bg-red-500/10 hover:text-red-600 dark:hover:text-red-400"
                            title={t("common.delete")}
                          >
                            <Trash2 size={13} />
                          </button>
                        </div>
                      </td>
                    </tr>
                  ))
                )}
              </tbody>
            </table>
          </div>
        )}
      </Card>

      {/* Item edit panel */}
      <AnimatePresence>
        {panel.open && (
          <ItemPanel
            collectionName={collectionName}
            fields={quickEditFields}
            mode={panel.mode}
            onClose={() => setPanel({ open: false })}
            onSaved={invalidate}
          />
        )}
      </AnimatePresence>

      {/* Delete confirmation */}
      <AnimatePresence>
        {delState.open && (
          <DeleteConfirm
            collectionName={collectionName}
            itemId={delState.itemId}
            onClose={() => setDelState({ open: false })}
            onDeleted={invalidate}
          />
        )}
      </AnimatePresence>
    </div>
  );
}

// ── Create / edit item — full page ────────────────────────────────────────────

type FormMode = { kind: "create" } | { kind: "edit"; itemId: string };

// Gate: loads schema, relations and (for edit) the item with its child relations
// expanded, before mounting the form so it can seed its state synchronously.
function ContentFormPage({ mode }: { mode: FormMode }) {
  const { collection: collectionName } = useParams({ strict: false }) as {
    collection: string;
  };

  const { data: schema, isLoading: schemaLoading } = useQuery({
    queryKey: ["collection", collectionName],
    queryFn: () => get<Collection>(`/collections/${collectionName}`),
    enabled: !!collectionName,
  });

  const { data: relations, isLoading: relLoading } = useQuery({
    queryKey: ["relations", collectionName],
    queryFn: () => listRelations(collectionName),
    enabled: !!collectionName,
  });
  const childRels = childRelationsOf(relations ?? [], collectionName);
  const childNames = childRels.map((c) => c.childCollection);

  const itemId = mode.kind === "edit" ? mode.itemId : "";
  const { data: deepItem, isLoading: itemLoading } = useQuery({
    queryKey: ["item-deep", collectionName, itemId, childNames.join(",")],
    queryFn: () => getItemDeep(collectionName, itemId, childNames),
    // Wait for relations so childNames is complete before the deep fetch.
    enabled: mode.kind === "edit" && !!collectionName && !relLoading,
  });

  const ready =
    !schemaLoading && !relLoading && (mode.kind === "create" || !itemLoading);

  if (!ready) {
    return (
      <div className="flex items-center justify-center py-20">
        <div className="h-8 w-8 animate-spin rounded-full border-2 border-brand-200 border-t-brand-600" />
      </div>
    );
  }

  return (
    <ContentFormBody
      key={mode.kind === "edit" ? `edit:${mode.itemId}` : "create"}
      collectionName={collectionName}
      schema={schema}
      childRels={childRels}
      mode={mode}
      initialItem={mode.kind === "edit" ? deepItem ?? {} : {}}
    />
  );
}

function ContentFormBody({
  collectionName,
  schema: schemaProp,
  childRels,
  mode,
  initialItem,
}: {
  collectionName: string;
  schema: Collection | undefined;
  childRels: ChildRelation[];
  mode: FormMode;
  initialItem: Record<string, unknown>;
}) {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const queryClient = useQueryClient();

  // Subscribe to the same collection query used by create + edit so section
  // reorder (and other schema edits) update the form live without remounting.
  const { data: liveSchema } = useQuery({
    queryKey: ["collection", collectionName],
    queryFn: () => get<Collection>(`/collections/${collectionName}`),
    initialData: schemaProp,
    enabled: !!collectionName,
  });
  const schema = liveSchema ?? schemaProp;

  // Relations (cache-shared with the gate) so UUID FK fields render as pickers.
  const { data: relations } = useQuery({
    queryKey: ["relations", collectionName],
    queryFn: () => listRelations(collectionName),
    enabled: !!collectionName,
  });

  const sections = sortedSections(schema);
  const allUserFields = allFields(schema).filter((f) => !f.is_system);
  const iconColor = schema?.color ?? "#6366f1";
  const isEdit = mode.kind === "edit";

  const availableLanguages = useAvailableLanguages();
  const [language, setLanguage] = useState(
    isEdit
      ? String(initialItem.language ?? availableLanguages[0])
      : availableLanguages[0],
  );

  const [values, setValues] = useState<Record<string, unknown>>(() => {
    if (mode.kind === "create") return {};
    const init: Record<string, unknown> = {};
    for (const f of allUserFields) init[f.name] = initialItem[f.name] ?? null;
    return init;
  });
  const [subitems, setSubitems] = useState<Record<string, Record<string, unknown>[]>>(() => {
    const seed: Record<string, Record<string, unknown>[]> = {};
    for (const c of childRels) {
      const arr = initialItem[c.childCollection];
      seed[c.childCollection] = Array.isArray(arr) ? (arr as Record<string, unknown>[]) : [];
    }
    return seed;
  });
  const [fieldErrors, setFieldErrors] = useState<Record<string, string>>({});
  const [serverError, setServerError] = useState<string | null>(null);
  const [sortSectionsOpen, setSortSectionsOpen] = useState(false);

  // Fields in sections that currently pass visibility — used for validate + submit.
  const visibleUserFields = visibleSectionFields(sections, values);

  const saveMut = useMutation({
    mutationFn: (data: Record<string, unknown>) =>
      mode.kind === "create"
        ? createItem(collectionName, { ...data, language })
        : updateItem(collectionName, mode.itemId, { ...data, language }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["items", collectionName] });
      navigate({ to: "/content/$collection", params: { collection: collectionName } });
    },
    onError: (e: Error) => setServerError(e.message),
  });

  function validate(): boolean {
    const errors: Record<string, string> = {};
    for (const f of visibleUserFields) {
      if (f.required && (values[f.name] == null || values[f.name] === "")) {
        errors[f.name] = t("content.fieldRequired", { field: f.display_name });
      }
    }
    setFieldErrors(errors);
    return Object.keys(errors).length === 0;
  }

  function handleChange(name: string, v: unknown) {
    setValues((prev) => ({ ...prev, [name]: v }));
    setFieldErrors((prev) => ({ ...prev, [name]: "" }));
  }

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    if (!validate()) return;
    setServerError(null);
    saveMut.mutate(buildPayload(visibleUserFields, values, childRels, subitems, language));
  }

  function goBack() {
    navigate({ to: "/content/$collection", params: { collection: collectionName } });
  }

  return (
    <div className="space-y-5">
      {/* Header */}
      <div className="flex items-center gap-4">
        <button
          onClick={goBack}
          className="flex h-9 w-9 items-center justify-center rounded-xl border border-slate-200 dark:border-white/10 bg-white dark:bg-white/5 text-slate-500 dark:text-slate-400 shadow-card transition hover:bg-brand-50 dark:hover:bg-brand-500/15 hover:text-brand-700 dark:hover:text-brand-300"
        >
          <ChevronLeft size={18} />
        </button>
        <div
          className="flex h-10 w-10 shrink-0 items-center justify-center rounded-xl"
          style={{ backgroundColor: `${iconColor}18` }}
        >
          <Database size={16} style={{ color: iconColor }} />
        </div>
        <div className="flex-1">
          <h1 className="text-xl font-bold text-slate-900 dark:text-slate-100">
            {isEdit
              ? t("content.editEntity", { name: schema?.display_name ?? collectionName })
              : t("content.newEntity", { name: schema?.display_name ?? collectionName })}
          </h1>
          <p className="mt-0.5 text-xs text-slate-400">{collectionName}</p>
        </div>
        <div className="flex items-center gap-2">
          {sections.length > 1 && (
            <Button
              type="button"
              size="sm"
              variant="outline"
              onClick={() => setSortSectionsOpen(true)}
            >
              <ArrowUpDown size={14} />
              {t("content.sortSections")}
            </Button>
          )}
          <LanguageSelector
            value={language}
            onChange={setLanguage}
            languages={availableLanguages}
          />
        </div>
      </div>

      <SectionReorderPanel
        open={sortSectionsOpen}
        collectionName={collectionName}
        sections={sections}
        onClose={() => setSortSectionsOpen(false)}
      />

      {/* Form — one card per section */}
      <form onSubmit={handleSubmit} className="space-y-5">
          {allUserFields.length === 0 ? (
            <Card>
              <p className="text-sm text-slate-400">
                {t("content.noFieldsAddFrom")}
                <button
                  type="button"
                  onClick={() =>
                    navigate({
                      to: "/collections/$name",
                      params: { name: collectionName },
                    })
                  }
                  className="text-brand-600 dark:text-brand-300 hover:underline"
                >
                  {t("content.collectionEditor")}
                </button>
                .
              </p>
            </Card>
          ) : (
            sections.map((section) => {
              if (!isSectionVisible(section.visibility, values)) return null;
              const sectionFields = [...(section.fields ?? [])]
                .filter((f) => !f.is_system)
                .sort((a, b) => a.sort - b.sort);
              if (sectionFields.length === 0) return null;
              return (
                <Card key={section.id} className="p-0">
                  <div className="border-b border-slate-100 dark:border-white/10 px-6 py-4">
                    <h2 className="text-sm font-semibold text-slate-900 dark:text-slate-100">
                      {section.display_name}
                    </h2>
                    {section.note && (
                      <p className="mt-0.5 text-xs text-slate-400">{section.note}</p>
                    )}
                  </div>
                  <div className="grid grid-cols-1 gap-5 px-6 py-6 md:grid-cols-2">
                    <FieldGroup
                      fields={withRelationOptions(sectionFields, relations ?? [], collectionName)}
                      values={values}
                      fieldErrors={fieldErrors}
                      onChange={handleChange}
                      gridClass="contents"
                    />
                  </div>
                </Card>
              );
            })
          )}

          {childRels.length > 0 && (
            <div className="space-y-4">
              {childRels.map((c) => (
                <SubitemsEditor
                  key={c.childCollection}
                  child={c}
                  initialRows={subitems[c.childCollection] ?? []}
                  onChange={(rows) =>
                    setSubitems((prev) => ({ ...prev, [c.childCollection]: rows }))
                  }
                />
              ))}
            </div>
          )}

          {serverError && (
            <div className="flex items-start gap-2 rounded-xl bg-red-50 dark:bg-red-500/10 px-3 py-3 text-sm text-red-700 dark:text-red-400">
              <AlertTriangle size={14} className="mt-0.5 shrink-0" />
              {serverError}
            </div>
          )}

          <div className="flex items-center justify-between rounded-2xl border border-slate-100 dark:border-white/10 bg-white dark:bg-slate-900 px-6 py-4 shadow-card">
            <Button type="button" variant="ghost" onClick={goBack}>
              {t("common.cancel")}
            </Button>
            <Button
              type="submit"
              disabled={saveMut.isPending || allUserFields.length === 0}
            >
              <Save size={14} />
              {saveMut.isPending
                ? isEdit ? t("common.saving") : t("content.creating")
                : isEdit ? t("content.saveChanges") : t("content.createItem")}
            </Button>
          </div>
      </form>
    </div>
  );
}

export function ContentCreatePage() {
  return <ContentFormPage mode={{ kind: "create" }} />;
}

export function ContentEditPage() {
  const { id } = useParams({ strict: false }) as { id: string };
  return <ContentFormPage mode={{ kind: "edit", itemId: id }} />;
}
