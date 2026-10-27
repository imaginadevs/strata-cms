/**
 * Aside panel to edit a section's display name, note, and visibility rules,
 * or delete the section (reassigning fields when needed).
 */
import { useState } from "react";
import { useTranslation } from "react-i18next";
import { createPortal } from "react-dom";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { AnimatePresence, motion } from "framer-motion";
import * as Select from "@radix-ui/react-select";
import { ChevronDown, Trash2, X } from "lucide-react";
import { del, patch } from "@/lib/api";
import { Button } from "@/components/ui/Button";
import { VisibilityEditor } from "@/components/collections/VisibilityEditor";
import type { Collection, Condition, Field, Section } from "@/types";

type UpdateBody = {
  display_name: string;
  note: string | null;
  visibility: Condition | null;
};

export function EditSectionPanel({
  open,
  collectionName,
  section,
  sections,
  collectionFields,
  onClose,
}: {
  open: boolean;
  collectionName: string;
  section: Section | null;
  /** All sections on the collection (for delete / reassign). */
  sections: Section[];
  /** All collection fields (for visibility field picker). */
  collectionFields: Field[];
  onClose: () => void;
}) {
  if (!open || !section) return null;
  return createPortal(
    <EditSectionPanelInner
      collectionName={collectionName}
      section={section}
      sections={sections}
      collectionFields={collectionFields}
      onClose={onClose}
    />,
    document.body,
  );
}

function EditSectionPanelInner({
  collectionName,
  section,
  sections,
  collectionFields,
  onClose,
}: {
  collectionName: string;
  section: Section;
  sections: Section[];
  collectionFields: Field[];
  onClose: () => void;
}) {
  const { t } = useTranslation();
  const queryClient = useQueryClient();
  const [displayName, setDisplayName] = useState(section.display_name);
  const [note, setNote] = useState(section.note ?? "");
  const [visibility, setVisibility] = useState<Condition | null>(
    section.visibility ?? null,
  );
  const [confirmDelete, setConfirmDelete] = useState(false);
  const otherSections = sections.filter((s) => s.name !== section.name);
  const fieldCount = section.fields?.length ?? 0;
  const canDelete = otherSections.length > 0;
  const [reassignTo, setReassignTo] = useState(otherSections[0]?.name ?? "");

  const mutation = useMutation({
    mutationFn: (body: UpdateBody) =>
      patch<Section>(`/collections/${collectionName}/sections/${section.name}`, body),
    onSuccess: (updated) => {
      queryClient.setQueryData<Collection>(["collection", collectionName], (old) => {
        if (!old) return old;
        return {
          ...old,
          sections: (old.sections ?? []).map((s) =>
            s.name === section.name
              ? {
                  ...s,
                  display_name: updated.display_name,
                  note: updated.note,
                  visibility: updated.visibility ?? null,
                }
              : s,
          ),
        };
      });
      queryClient.invalidateQueries({ queryKey: ["collection", collectionName] });
      onClose();
    },
  });

  const deleteMutation = useMutation({
    mutationFn: () => {
      const qs =
        fieldCount > 0 && reassignTo
          ? `?reassign_to=${encodeURIComponent(reassignTo)}`
          : "";
      return del(`/collections/${collectionName}/sections/${section.name}${qs}`);
    },
    onSuccess: () => {
      queryClient.setQueryData<Collection>(["collection", collectionName], (old) => {
        if (!old) return old;
        const remaining = (old.sections ?? []).filter((s) => s.name !== section.name);
        if (fieldCount > 0 && reassignTo) {
          const moved = section.fields ?? [];
          return {
            ...old,
            sections: remaining.map((s) =>
              s.name === reassignTo
                ? {
                    ...s,
                    fields: [
                      ...(s.fields ?? []),
                      ...moved.map((f) => ({ ...f, section: reassignTo })),
                    ],
                  }
                : s,
            ),
          };
        }
        return { ...old, sections: remaining };
      });
      queryClient.invalidateQueries({ queryKey: ["collection", collectionName] });
      onClose();
    },
  });

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    mutation.mutate({
      display_name: displayName.trim() || section.name,
      note: note.trim() || null,
      visibility,
    });
  }

  const busy = mutation.isPending || deleteMutation.isPending;
  const error = mutation.error ?? deleteMutation.error;

  return (
    <AnimatePresence>
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
        className="fixed right-0 top-0 z-50 flex h-full w-full max-w-md flex-col bg-white dark:bg-slate-900 shadow-2xl"
      >
        <form onSubmit={handleSubmit} className="flex h-full flex-col">
          <div className="flex shrink-0 items-center justify-between border-b border-slate-100 dark:border-white/10 px-6 py-5">
            <div>
              <h2 className="text-base font-semibold text-slate-900 dark:text-slate-100">
                {t("collections.editSection")}
              </h2>
              <p className="mt-0.5 font-mono text-xs text-slate-400">{section.name}</p>
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
            {!confirmDelete ? (
              <>
                <div>
                  <label className="mb-1.5 block text-xs font-medium text-slate-700 dark:text-slate-200">
                    {t("collections.displayName")}
                  </label>
                  <input
                    className="input"
                    value={displayName}
                    onChange={(e) => setDisplayName(e.target.value)}
                    placeholder={t("collections.placeholderGeneral")}
                  />
                </div>

                <div>
                  <label className="mb-1.5 block text-xs font-medium text-slate-700 dark:text-slate-200">
                    {t("collections.note")}
                  </label>
                  <textarea
                    className="input resize-none"
                    rows={2}
                    value={note}
                    onChange={(e) => setNote(e.target.value)}
                    placeholder={t("collections.noteHelperPlaceholder")}
                  />
                </div>

                <VisibilityEditor
                  value={visibility}
                  onChange={setVisibility}
                  fields={collectionFields}
                />
              </>
            ) : (
              <div className="space-y-4">
                <div className="rounded-xl border border-red-100 dark:border-red-500/20 bg-red-50/80 dark:bg-red-500/10 px-4 py-3">
                  <p className="text-sm font-medium text-red-700 dark:text-red-300">
                    {t("collections.deleteSectionTitle", { name: section.display_name })}
                  </p>
                  <p className="mt-1 text-xs text-red-600/80 dark:text-red-400/80">
                    {fieldCount > 0
                      ? t("collections.deleteSectionWithFields", { count: fieldCount })
                      : t("collections.deleteSectionEmpty")}
                  </p>
                </div>

                {fieldCount > 0 && (
                  <div>
                    <label className="mb-1.5 block text-xs font-medium text-slate-700 dark:text-slate-200">
                      {t("collections.moveFieldsTo")}
                    </label>
                    <Select.Root value={reassignTo} onValueChange={setReassignTo}>
                      <Select.Trigger className="flex h-10 w-full items-center justify-between rounded-lg border border-slate-200 dark:border-white/10 bg-white dark:bg-white/5 px-3 text-sm text-slate-800 dark:text-slate-100 focus:outline-none focus:ring-2 focus:ring-brand-100 dark:focus:ring-brand-500/20">
                        <Select.Value placeholder={t("collections.selectSection")} />
                        <Select.Icon>
                          <ChevronDown size={14} className="text-slate-400" />
                        </Select.Icon>
                      </Select.Trigger>
                      <Select.Portal>
                        <Select.Content
                          position="popper"
                          sideOffset={4}
                          className="z-[60] overflow-hidden rounded-xl border border-slate-100 dark:border-white/10 bg-white dark:bg-slate-800 p-1 shadow-lg"
                        >
                          <Select.Viewport>
                            {otherSections.map((s) => (
                              <Select.Item
                                key={s.id}
                                value={s.name}
                                className="cursor-pointer rounded-lg px-3 py-2 text-sm text-slate-700 dark:text-slate-200 outline-none data-[highlighted]:bg-brand-50 dark:data-[highlighted]:bg-brand-500/15 data-[highlighted]:text-brand-700 dark:data-[highlighted]:text-brand-300"
                              >
                                <Select.ItemText>{s.display_name}</Select.ItemText>
                              </Select.Item>
                            ))}
                          </Select.Viewport>
                        </Select.Content>
                      </Select.Portal>
                    </Select.Root>
                  </div>
                )}
              </div>
            )}

            {error && (
              <p className="rounded-xl bg-red-50 dark:bg-red-500/10 px-3 py-2 text-sm text-red-600 dark:text-red-400">
                {(error as Error).message}
              </p>
            )}
          </div>

          <div className="flex shrink-0 items-center justify-between gap-2 border-t border-slate-100 dark:border-white/10 px-6 py-4">
            {!confirmDelete ? (
              <>
                <Button
                  type="button"
                  variant="ghost"
                  disabled={!canDelete || busy}
                  onClick={() => setConfirmDelete(true)}
                  title={
                    canDelete
                      ? t("collections.deleteSection")
                      : t("collections.minOneSection")
                  }
                  className={
                    canDelete
                      ? "text-red-600 hover:bg-red-50 hover:text-red-700 dark:text-red-400 dark:hover:bg-red-500/10"
                      : undefined
                  }
                >
                  <Trash2 size={14} />
                  {t("common.delete")}
                </Button>
                <div className="flex items-center gap-2">
                  <Button type="button" variant="ghost" onClick={onClose}>
                    {t("common.cancel")}
                  </Button>
                  <Button type="submit" disabled={busy || !displayName.trim()}>
                    {mutation.isPending ? t("common.saving") : t("collections.saveSection")}
                  </Button>
                </div>
              </>
            ) : (
              <>
                <Button
                  type="button"
                  variant="ghost"
                  disabled={busy}
                  onClick={() => setConfirmDelete(false)}
                >
                  {t("common.back")}
                </Button>
                <Button
                  type="button"
                  variant="danger"
                  disabled={busy || (fieldCount > 0 && !reassignTo)}
                  onClick={() => deleteMutation.mutate()}
                >
                  <Trash2 size={14} />
                  {deleteMutation.isPending ? t("common.deleting") : t("collections.deleteSection")}
                </Button>
              </>
            )}
          </div>
        </form>
      </motion.aside>
    </AnimatePresence>
  );
}
