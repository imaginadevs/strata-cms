/**
 * Aside panel to create a new section on a collection.
 */
import { useState } from "react";
import { createPortal } from "react-dom";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { AnimatePresence, motion } from "framer-motion";
import { X } from "lucide-react";
import { post } from "@/lib/api";
import { Button } from "@/components/ui/Button";
import type { Collection, Section } from "@/types";

const SNAKE_RE = /^[a-z][a-z0-9_]*$/;

export function AddSectionPanel({
  open,
  collectionName,
  nextSort,
  onClose,
}: {
  open: boolean;
  collectionName: string;
  nextSort: number;
  onClose: () => void;
}) {
  if (!open) return null;
  return createPortal(
    <AddSectionPanelInner
      collectionName={collectionName}
      nextSort={nextSort}
      onClose={onClose}
    />,
    document.body,
  );
}

function AddSectionPanelInner({
  collectionName,
  nextSort,
  onClose,
}: {
  collectionName: string;
  nextSort: number;
  onClose: () => void;
}) {
  const queryClient = useQueryClient();
  const [name, setName] = useState("");
  const [displayName, setDisplayName] = useState("");
  const [nameError, setNameError] = useState<string | null>(null);

  const mutation = useMutation({
    mutationFn: () =>
      post<Section>(`/collections/${collectionName}/sections`, {
        name: name.trim(),
        display_name: displayName.trim() || name.trim(),
        sort: nextSort,
      }),
    onSuccess: (created) => {
      queryClient.setQueryData<Collection>(["collection", collectionName], (old) => {
        if (!old) return old;
        return {
          ...old,
          sections: [...(old.sections ?? []), { ...created, fields: [] }],
        };
      });
      queryClient.invalidateQueries({ queryKey: ["collection", collectionName] });
      onClose();
    },
  });

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    const slug = name.trim();
    if (!SNAKE_RE.test(slug)) {
      setNameError("Use lowercase letters, numbers and underscores. Must start with a letter.");
      return;
    }
    setNameError(null);
    mutation.mutate();
  }

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
        className="fixed right-0 top-0 z-50 flex h-full w-full max-w-sm flex-col bg-white dark:bg-slate-900 shadow-2xl"
      >
        <form onSubmit={handleSubmit} className="flex h-full flex-col">
          <div className="flex shrink-0 items-center justify-between border-b border-slate-100 dark:border-white/10 px-6 py-5">
            <div>
              <h2 className="text-base font-semibold text-slate-900 dark:text-slate-100">
                Add Section
              </h2>
              <p className="mt-0.5 text-xs text-slate-400">{collectionName}</p>
            </div>
            <button
              type="button"
              onClick={onClose}
              className="rounded-lg p-1.5 text-slate-400 transition hover:bg-slate-100 dark:hover:bg-white/5 hover:text-slate-600 dark:hover:text-slate-200"
            >
              <X size={18} />
            </button>
          </div>

          <div className="flex-1 space-y-5 overflow-y-auto px-6 py-6">
            <div>
              <label className="mb-1.5 block text-xs font-medium text-slate-700 dark:text-slate-200">
                Key <span className="text-red-400">*</span>
              </label>
              <input
                className="input font-mono text-sm"
                value={name}
                onChange={(e) => setName(e.target.value)}
                placeholder="pricing"
              />
              <p className="mt-1 text-xs text-slate-400">Slug — snake_case, unique in this collection</p>
              {nameError && <p className="mt-1 text-xs text-red-500">{nameError}</p>}
            </div>
            <div>
              <label className="mb-1.5 block text-xs font-medium text-slate-700 dark:text-slate-200">
                Display name
              </label>
              <input
                className="input"
                value={displayName}
                onChange={(e) => setDisplayName(e.target.value)}
                placeholder="Pricing"
              />
            </div>
            {mutation.isError && (
              <p className="rounded-xl bg-red-50 dark:bg-red-500/10 px-3 py-2 text-sm text-red-600 dark:text-red-400">
                {(mutation.error as Error).message}
              </p>
            )}
          </div>

          <div className="flex shrink-0 items-center justify-between border-t border-slate-100 dark:border-white/10 px-6 py-4">
            <Button type="button" variant="ghost" onClick={onClose}>
              Cancel
            </Button>
            <Button type="submit" disabled={mutation.isPending || !name.trim()}>
              {mutation.isPending ? "Creating…" : "Create Section"}
            </Button>
          </div>
        </form>
      </motion.aside>
    </AnimatePresence>
  );
}
