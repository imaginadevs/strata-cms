/**
 * Roles list + permission editor.
 * Matrix rows = virtual cms_* targets (GET /permissions/catalog) + content
 * collections (GET /collections). Actions include "all".
 * Saved via PUT /roles/{id}/permissions.
 */
import { useEffect, useMemo, useState } from "react";
import { createPortal } from "react-dom";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { AnimatePresence, motion } from "framer-motion";
import { Lock, Shield, X } from "lucide-react";
import { get, put } from "@/lib/api";
import { Card } from "@/components/ui/Card";
import { Button } from "@/components/ui/Button";
import type {
  Collection,
  PaginatedResponse,
  Permission,
  PermissionAction,
  PermissionCatalog,
  Role,
} from "@/types";

const DEFAULT_ACTIONS: PermissionAction[] = [
  "create",
  "read",
  "update",
  "delete",
  "all",
];

type PermRow = {
  name: string;
  display_name: string;
  description?: string;
  virtual?: boolean;
};

function permKey(collection: string, action: PermissionAction) {
  return `${collection}:${action}`;
}

export default function RolesPage() {
  const [editing, setEditing] = useState<Role | null>(null);
  const { data, isLoading } = useQuery({
    queryKey: ["roles"],
    queryFn: () => get<PaginatedResponse<Role>>("/roles"),
  });
  const roles = data?.data ?? [];

  return (
    <div className="space-y-5">
      <div>
        <h1 className="text-2xl font-bold text-slate-900 dark:text-slate-100">Roles</h1>
        <p className="mt-1 text-sm text-slate-500 dark:text-slate-400">
          Manage roles and collection permissions. Assign roles to users from the Users page.
        </p>
      </div>

      <Card className="overflow-hidden p-0">
        <table className="w-full text-sm">
          <thead>
            <tr className="border-b border-slate-100 dark:border-white/10 text-left text-xs font-medium text-slate-500 dark:text-slate-400">
              <th className="px-5 py-3">Role</th>
              <th className="px-5 py-3">Description</th>
              <th className="px-5 py-3">Type</th>
              <th className="w-28 px-5 py-3" />
            </tr>
          </thead>
          <tbody>
            {isLoading && (
              <tr>
                <td colSpan={4} className="py-12 text-center text-slate-400">
                  Loading…
                </td>
              </tr>
            )}
            {roles.map((role) => (
              <tr
                key={role.id}
                className="border-b border-slate-100 dark:border-white/10 last:border-0 hover:bg-slate-50/60 dark:hover:bg-white/5"
              >
                <td className="px-5 py-3">
                  <div className="flex items-center gap-2.5">
                    <div className="flex h-8 w-8 items-center justify-center rounded-lg bg-brand-50 text-brand-600 dark:bg-brand-500/15 dark:text-brand-300">
                      {role.is_system ? <Lock size={14} /> : <Shield size={14} />}
                    </div>
                    <p className="font-medium text-slate-900 dark:text-slate-100">{role.name}</p>
                  </div>
                </td>
                <td className="px-5 py-3 text-slate-500 dark:text-slate-400">
                  {role.description || "—"}
                </td>
                <td className="px-5 py-3">
                  {role.is_system ? (
                    <span className="badge badge-gray">system</span>
                  ) : (
                    <span className="badge badge-blue">custom</span>
                  )}
                </td>
                <td className="px-5 py-3 text-right">
                  <Button
                    type="button"
                    variant="ghost"
                    size="sm"
                    onClick={() => setEditing(role)}
                  >
                    Permissions
                  </Button>
                </td>
              </tr>
            ))}
            {!isLoading && roles.length === 0 && (
              <tr>
                <td colSpan={4} className="py-12 text-center text-slate-400">
                  No roles found
                </td>
              </tr>
            )}
          </tbody>
        </table>
      </Card>

      <AnimatePresence>
        {editing && (
          <RolePermissionsPanel role={editing} onClose={() => setEditing(null)} />
        )}
      </AnimatePresence>
    </div>
  );
}

function RolePermissionsPanel({
  role,
  onClose,
}: {
  role: Role;
  onClose: () => void;
}) {
  const queryClient = useQueryClient();

  const { data: catalog, isLoading: catalogLoading } = useQuery({
    queryKey: ["permissions-catalog"],
    queryFn: () => get<PermissionCatalog>("/permissions/catalog"),
  });

  const { data: collectionsPage, isLoading: collLoading } = useQuery({
    queryKey: ["collections", { hidden: true }],
    queryFn: () => get<PaginatedResponse<Collection>>("/collections?hidden=true"),
  });

  const actions = catalog?.actions?.length ? catalog.actions : DEFAULT_ACTIONS;

  const virtualRows: PermRow[] = useMemo(
    () =>
      (catalog?.virtual ?? []).map((v) => ({
        name: v.name,
        display_name: v.display_name,
        description: v.description,
        virtual: true,
      })),
    [catalog?.virtual],
  );

  const contentRows: PermRow[] = useMemo(() => {
    const virtualNames = new Set(virtualRows.map((r) => r.name));
    return [...(collectionsPage?.data ?? [])]
      .filter((c) => !virtualNames.has(c.name) && !c.name.startsWith("cms_"))
      .sort((a, b) => a.name.localeCompare(b.name))
      .map((c) => ({
        name: c.name,
        display_name: c.display_name || c.name,
      }));
  }, [collectionsPage?.data, virtualRows]);

  const { data: permsPage, isLoading: permsLoading } = useQuery({
    queryKey: ["role-permissions", role.id],
    queryFn: () => get<PaginatedResponse<Permission>>(`/roles/${role.id}/permissions`),
  });

  const [granted, setGranted] = useState<Set<string>>(new Set());

  useEffect(() => {
    const next = new Set<string>();
    for (const p of permsPage?.data ?? []) {
      next.add(permKey(p.collection, p.action));
    }
    setGranted(next);
  }, [permsPage]);

  function toggle(collection: string, action: PermissionAction) {
    const key = permKey(collection, action);
    setGranted((prev) => {
      const next = new Set(prev);
      if (next.has(key)) next.delete(key);
      else next.add(key);
      return next;
    });
  }

  const saveMut = useMutation({
    mutationFn: () => {
      const body = [...granted].map((key) => {
        const idx = key.lastIndexOf(":");
        const collection = key.slice(0, idx);
        const action = key.slice(idx + 1) as PermissionAction;
        return { collection, action, fields: [] as string[], filter: null };
      });
      return put(`/roles/${role.id}/permissions`, body);
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["role-permissions", role.id] });
      onClose();
    },
  });

  const loading = catalogLoading || collLoading || permsLoading;
  const hasRows = virtualRows.length > 0 || contentRows.length > 0;

  return createPortal(
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
        className="fixed right-0 top-0 z-50 flex h-full w-full max-w-xl flex-col bg-white dark:bg-slate-900 shadow-2xl"
      >
        <div className="flex shrink-0 items-center justify-between border-b border-slate-100 dark:border-white/10 px-6 py-5">
          <div>
            <h2 className="text-base font-semibold text-slate-900 dark:text-slate-100">
              Permissions
            </h2>
            <p className="mt-0.5 text-xs text-slate-400">{role.name}</p>
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
          {loading ? (
            <p className="py-10 text-center text-sm text-slate-400">Loading…</p>
          ) : !hasRows ? (
            <p className="py-10 text-center text-sm text-slate-400">No permission targets</p>
          ) : (
            <div className="space-y-5">
              {virtualRows.length > 0 && (
                <PermissionTable
                  title="System"
                  hint="Admin surfaces (not content collections)"
                  rows={virtualRows}
                  actions={actions}
                  granted={granted}
                  onToggle={toggle}
                />
              )}
              {contentRows.length > 0 && (
                <PermissionTable
                  title="Collections"
                  hint="Content collections from your schema"
                  rows={contentRows}
                  actions={actions}
                  granted={granted}
                  onToggle={toggle}
                />
              )}
            </div>
          )}

          <p className="mt-3 px-1 text-[11px] text-slate-400">
            Action <span className="font-mono">all</span> grants every action on
            that target. Super-admins and the Administrator role bypass checks.
          </p>

          {saveMut.isError && (
            <p className="mt-3 rounded-xl bg-red-50 dark:bg-red-500/10 px-3 py-2 text-sm text-red-600 dark:text-red-400">
              {(saveMut.error as Error).message}
            </p>
          )}
        </div>

        <div className="flex shrink-0 items-center justify-between border-t border-slate-100 dark:border-white/10 px-6 py-4">
          <Button type="button" variant="ghost" onClick={onClose}>
            Cancel
          </Button>
          <Button
            type="button"
            disabled={saveMut.isPending || loading}
            onClick={() => saveMut.mutate()}
          >
            {saveMut.isPending ? "Saving…" : "Save permissions"}
          </Button>
        </div>
      </motion.aside>
    </>,
    document.body,
  );
}

function PermissionTable({
  title,
  hint,
  rows,
  actions,
  granted,
  onToggle,
}: {
  title: string;
  hint: string;
  rows: PermRow[];
  actions: PermissionAction[];
  granted: Set<string>;
  onToggle: (collection: string, action: PermissionAction) => void;
}) {
  return (
    <div>
      <div className="mb-2 px-1">
        <p className="text-xs font-semibold uppercase tracking-wide text-slate-500 dark:text-slate-400">
          {title}
        </p>
        <p className="text-[11px] text-slate-400">{hint}</p>
      </div>
      <div className="overflow-x-auto rounded-xl border border-slate-100 dark:border-white/10">
        <table className="w-full text-xs">
          <thead>
            <tr className="border-b border-slate-100 dark:border-white/10 bg-slate-50/80 dark:bg-white/[0.03] text-left text-[10px] font-semibold uppercase tracking-wide text-slate-400">
              <th className="px-3 py-2.5">Target</th>
              {actions.map((a) => (
                <th key={a} className="px-2 py-2.5 text-center capitalize">
                  {a}
                </th>
              ))}
            </tr>
          </thead>
          <tbody className="divide-y divide-slate-50 dark:divide-white/5">
            {rows.map((row) => (
              <tr key={row.name} className="hover:bg-slate-50/50 dark:hover:bg-white/[0.02]">
                <td className="px-3 py-2">
                  <p className="font-medium text-slate-800 dark:text-slate-100">
                    {row.display_name}
                  </p>
                  <p className="font-mono text-[10px] text-slate-400">{row.name}</p>
                  {row.description && (
                    <p className="mt-0.5 text-[10px] text-slate-400">{row.description}</p>
                  )}
                </td>
                {actions.map((a) => {
                  const on = granted.has(permKey(row.name, a));
                  return (
                    <td key={a} className="px-2 py-2 text-center">
                      <input
                        type="checkbox"
                        checked={on}
                        onChange={() => onToggle(row.name, a)}
                        className="h-3.5 w-3.5 rounded border-slate-300 text-brand-600 focus:ring-brand-400 dark:border-white/20"
                        aria-label={`${row.name} ${a}`}
                      />
                    </td>
                  );
                })}
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}
