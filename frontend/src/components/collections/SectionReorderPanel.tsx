/**
 * Compact aside to reorder collection sections by title only (no field lists).
 * Persists via PATCH /collections/{collection}/sections/reorder.
 * Used from content create/edit and the collection editor.
 */
import { useEffect, useState } from "react";
import { createPortal } from "react-dom";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { AnimatePresence, motion } from "framer-motion";
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
import { GripVertical, X } from "lucide-react";
import { patch } from "@/lib/api";
import { Button } from "@/components/ui/Button";
import type { Collection, Section } from "@/types";

export function SectionReorderPanel({
  open,
  collectionName,
  sections,
  onClose,
}: {
  open: boolean;
  collectionName: string;
  sections: Section[];
  onClose: () => void;
}) {
  return createPortal(
    <AnimatePresence>
      {open && (
        <SectionReorderPanelInner
          key={`${collectionName}:${sections.map((s) => s.id).join(",")}`}
          collectionName={collectionName}
          sections={sections}
          onClose={onClose}
        />
      )}
    </AnimatePresence>,
    document.body,
  );
}

function SectionReorderPanelInner({
  collectionName,
  sections,
  onClose,
}: {
  collectionName: string;
  sections: Section[];
  onClose: () => void;
}) {
  const queryClient = useQueryClient();
  const [order, setOrder] = useState(() =>
    [...sections].sort((a, b) => a.sort - b.sort),
  );

  const sectionSig = sections.map((s) => `${s.id}:${s.sort}`).join("|");
  // Re-seed only when server order / membership changes — not on every parent render.
  useEffect(() => {
    setOrder([...sections].sort((a, b) => a.sort - b.sort));
    // eslint-disable-next-line react-hooks/exhaustive-deps -- intentional: seed from sectionSig
  }, [sectionSig]);

  const sensors = useSensors(
    useSensor(PointerSensor, { activationConstraint: { distance: 4 } }),
  );

  const mutation = useMutation({
    mutationFn: (names: string[]) =>
      patch<Section[]>(`/collections/${collectionName}/sections/reorder`, {
        order: names,
      }),
    onSuccess: (updated) => {
      queryClient.setQueryData<Collection>(["collection", collectionName], (old) => {
        if (!old) return old;
        const byName = new Map((old.sections ?? []).map((s) => [s.name, s]));
        const merged = updated.map((s, i) => {
          const prev = byName.get(s.name);
          return {
            ...(prev ?? s),
            ...s,
            sort: i,
            // Reorder API returns sections without nested fields — keep them.
            fields: prev?.fields ?? s.fields ?? [],
          };
        });
        return { ...old, sections: merged };
      });
      // Soft refresh so create + edit forms pick up the new order without a hard remount.
      void queryClient.invalidateQueries({
        queryKey: ["collection", collectionName],
        refetchType: "active",
      });
      onClose();
    },
  });

  function handleDragEnd(event: DragEndEvent) {
    const { active, over } = event;
    if (!over || active.id === over.id) return;
    const oldIndex = order.findIndex((s) => s.id === active.id);
    const newIndex = order.findIndex((s) => s.id === over.id);
    if (oldIndex === -1 || newIndex === -1) return;
    setOrder(arrayMove(order, oldIndex, newIndex));
  }

  function handleSave() {
    mutation.mutate(order.map((s) => s.name));
  }

  return (
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
        <div className="flex shrink-0 items-center justify-between border-b border-slate-100 dark:border-white/10 px-6 py-5">
          <div>
            <h2 className="text-base font-semibold text-slate-900 dark:text-slate-100">
              Sort sections
            </h2>
            <p className="mt-0.5 text-xs text-slate-400">
              Drag to reorder — fields stay inside each section
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

        <div className="flex-1 overflow-y-auto px-4 py-4">
          {order.length === 0 ? (
            <p className="py-8 text-center text-sm text-slate-400">No sections to sort</p>
          ) : (
            <DndContext
              sensors={sensors}
              collisionDetection={closestCenter}
              onDragEnd={handleDragEnd}
            >
              <SortableContext
                items={order.map((s) => s.id)}
                strategy={verticalListSortingStrategy}
              >
                <ul className="space-y-2">
                  {order.map((section, index) => (
                    <SortableSectionRow
                      key={section.id}
                      section={section}
                      index={index}
                    />
                  ))}
                </ul>
              </SortableContext>
            </DndContext>
          )}
        </div>

        {mutation.isError && (
          <p className="mx-6 mb-2 rounded-xl bg-red-50 dark:bg-red-500/10 px-3 py-2 text-sm text-red-600 dark:text-red-400">
            {(mutation.error as Error).message}
          </p>
        )}

        <div className="flex shrink-0 items-center justify-between border-t border-slate-100 dark:border-white/10 px-6 py-4">
          <Button type="button" variant="ghost" onClick={onClose}>
            Cancel
          </Button>
          <Button
            type="button"
            onClick={handleSave}
            disabled={mutation.isPending || order.length < 2}
          >
            {mutation.isPending ? "Saving…" : "Save order"}
          </Button>
        </div>
      </motion.aside>
    </>
  );
}

function SortableSectionRow({
  section,
  index,
}: {
  section: Section;
  index: number;
}) {
  const {
    attributes,
    listeners,
    setNodeRef,
    transform,
    transition,
    isDragging,
  } = useSortable({ id: section.id });

  const style = {
    transform: CSS.Transform.toString(transform),
    transition,
  };

  const fieldCount = section.fields?.length ?? 0;

  return (
    <li
      ref={setNodeRef}
      style={style}
      className={`flex items-center gap-3 rounded-xl border px-3 py-3 ${
        isDragging
          ? "z-10 border-brand-300 bg-brand-50/80 shadow-md dark:border-brand-500/40 dark:bg-brand-500/15"
          : "border-slate-100 bg-slate-50/80 dark:border-white/10 dark:bg-white/[0.04]"
      }`}
    >
      <button
        type="button"
        className="flex h-8 w-8 shrink-0 cursor-grab items-center justify-center rounded-lg text-slate-300 hover:bg-white hover:text-slate-500 active:cursor-grabbing dark:hover:bg-white/10"
        {...attributes}
        {...listeners}
        title="Drag to reorder"
      >
        <GripVertical size={16} />
      </button>
      <span className="flex h-6 w-6 shrink-0 items-center justify-center rounded-md bg-white text-[11px] font-semibold text-slate-400 dark:bg-white/10">
        {index + 1}
      </span>
      <div className="min-w-0 flex-1">
        <p className="truncate text-sm font-medium text-slate-900 dark:text-slate-100">
          {section.display_name}
        </p>
        <p className="truncate font-mono text-[11px] text-slate-400">
          {section.name}
          {fieldCount > 0 ? ` · ${fieldCount} field${fieldCount !== 1 ? "s" : ""}` : ""}
        </p>
      </div>
    </li>
  );
}
