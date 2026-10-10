/**
 * Tenants — super-admin screen to list and manage tenants.
 *
 * Creating a tenant here hits POST /api/tenants, which now fully provisions the
 * tenant (schema + base tables + seed admin) synchronously — no manual DB inserts
 * and no server restart required.
 */
import { useState } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { motion, AnimatePresence } from "framer-motion";
import { Plus, X, Pencil, Trash2, Building2, Globe, AlertTriangle } from "lucide-react";
import { get, post, patch, del } from "@/lib/api";
import { Button } from "@/components/ui/Button";
import { useAuthStore } from "@/store/auth.store";
import type { Tenant, PaginatedResponse } from "@/types";

function slugify(s: string) {
  return s
    .toLowerCase()
    .normalize("NFD")
    .replace(/[̀-ͯ]/g, "")
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-+|-+$/g, "");
}

export default function TenantsPage() {
  const user = useAuthStore((s) => s.user);
  const isSuper = !!user?.is_super_admin || !!user?.roles?.includes("super_admin");
  const qc = useQueryClient();

  const [panelOpen, setPanelOpen] = useState(false);
  const [editing, setEditing] = useState<Tenant | null>(null);
  const [confirm, setConfirm] = useState<Tenant | null>(null);

  const { data: tenants = [], isLoading } = useQuery({
    queryKey: ["tenants"],
    queryFn: () => get<PaginatedResponse<Tenant>>("/tenants").then((r) => r.data ?? []),
  });

  const deleteMut = useMutation({
    mutationFn: (id: string) => del(`/tenants/${id}`),
    onSuccess: () => {
      setConfirm(null);
      qc.invalidateQueries({ queryKey: ["tenants"] });
    },
  });

  if (!isSuper) {
    return (
      <div className="mx-auto max-w-2xl px-6 py-16 text-center">
        <AlertTriangle className="mx-auto mb-3 text-amber-400" size={28} />
        <h1 className="text-lg font-semibold text-slate-800 dark:text-slate-100">Super-admin only</h1>
        <p className="mt-1 text-sm text-slate-500 dark:text-slate-400">
          Managing tenants requires a super-admin account. To change your own tenant's branding
          or plugins, use Settings.
        </p>
      </div>
    );
  }

  return (
    <div className="mx-auto max-w-5xl px-4 py-6 sm:px-6">
      <div className="mb-6 flex items-center justify-between">
        <div>
          <h1 className="text-xl font-bold text-slate-900 dark:text-slate-100">Tenants</h1>
          <p className="text-sm text-slate-400">
            {tenants.length} tenant{tenants.length === 1 ? "" : "s"}
          </p>
        </div>
        <Button onClick={() => { setEditing(null); setPanelOpen(true); }}>
          <Plus size={15} className="mr-1.5" />
          New tenant
        </Button>
      </div>

      <div className="overflow-hidden rounded-xl border border-slate-200 bg-white dark:border-white/10 dark:bg-white/5">
        <table className="w-full text-sm">
          <thead>
            <tr className="border-b border-slate-100 text-left text-xs uppercase tracking-wide text-slate-400 dark:border-white/10">
              <th className="px-4 py-3 font-semibold">Name</th>
              <th className="px-4 py-3 font-semibold">Slug</th>
              <th className="px-4 py-3 font-semibold">Domain</th>
              <th className="px-4 py-3 font-semibold">Status</th>
              <th className="px-4 py-3" />
            </tr>
          </thead>
          <tbody>
            {isLoading && (
              <tr><td colSpan={5} className="px-4 py-8 text-center text-slate-400">Loading…</td></tr>
            )}
            {!isLoading && tenants.length === 0 && (
              <tr><td colSpan={5} className="px-4 py-8 text-center text-slate-400">No tenants yet.</td></tr>
            )}
            {tenants.map((t) => (
              <tr key={t.id} className="border-b border-slate-50 last:border-0 dark:border-white/5">
                <td className="px-4 py-3">
                  <div className="flex items-center gap-2.5">
                    <span
                      className="flex h-8 w-8 items-center justify-center rounded-lg text-[11px] font-bold text-white"
                      style={{ backgroundColor: t.primary_color ?? "#6366f1" }}
                    >
                      {t.name?.[0]?.toUpperCase() ?? "T"}
                    </span>
                    <span className="font-medium text-slate-800 dark:text-slate-100">{t.name}</span>
                  </div>
                </td>
                <td className="px-4 py-3 font-mono text-xs text-slate-500 dark:text-slate-400">{t.slug}</td>
                <td className="px-4 py-3 text-slate-500 dark:text-slate-400">
                  {t.domain ? (
                    <span className="inline-flex items-center gap-1.5"><Globe size={12} />{t.domain}</span>
                  ) : (
                    <span className="text-slate-300 dark:text-slate-600">—</span>
                  )}
                </td>
                <td className="px-4 py-3">
                  <span className={`inline-flex rounded-full px-2 py-0.5 text-xs font-medium ${
                    t.active
                      ? "bg-emerald-50 text-emerald-600 dark:bg-emerald-500/15 dark:text-emerald-300"
                      : "bg-slate-100 text-slate-500 dark:bg-white/10 dark:text-slate-400"
                  }`}>
                    {t.active ? "Active" : "Disabled"}
                  </span>
                </td>
                <td className="px-4 py-3">
                  <div className="flex items-center justify-end gap-1">
                    <button
                      type="button"
                      onClick={() => { setEditing(t); setPanelOpen(true); }}
                      className="flex h-8 w-8 items-center justify-center rounded-lg text-slate-400 transition hover:bg-slate-100 hover:text-brand-600 dark:hover:bg-white/10"
                      title="Edit"
                    >
                      <Pencil size={14} />
                    </button>
                    <button
                      type="button"
                      onClick={() => setConfirm(t)}
                      className="flex h-8 w-8 items-center justify-center rounded-lg text-slate-400 transition hover:bg-red-50 hover:text-red-500 dark:hover:bg-red-500/10"
                      title="Delete"
                    >
                      <Trash2 size={14} />
                    </button>
                  </div>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>

      <TenantPanel
        open={panelOpen}
        tenant={editing}
        onClose={() => setPanelOpen(false)}
        onSaved={() => { setPanelOpen(false); qc.invalidateQueries({ queryKey: ["tenants"] }); }}
      />

      {/* Delete confirm */}
      <AnimatePresence>
        {confirm && (
          <>
            <motion.div
              initial={{ opacity: 0 }} animate={{ opacity: 1 }} exit={{ opacity: 0 }}
              className="fixed inset-0 z-40 bg-black/20 backdrop-blur-[2px]"
              onClick={() => setConfirm(null)}
            />
            <motion.div
              initial={{ opacity: 0, scale: 0.96 }} animate={{ opacity: 1, scale: 1 }} exit={{ opacity: 0, scale: 0.96 }}
              className="fixed left-1/2 top-1/2 z-50 w-full max-w-sm -translate-x-1/2 -translate-y-1/2 rounded-2xl bg-white p-6 shadow-2xl dark:bg-slate-900"
            >
              <h2 className="text-base font-semibold text-slate-900 dark:text-slate-100">Delete tenant?</h2>
              <p className="mt-2 text-sm text-slate-500 dark:text-slate-400">
                This removes <span className="font-medium">{confirm.name}</span> from the tenant list.
                Its data schema is left intact on the database and must be dropped separately.
              </p>
              <div className="mt-5 flex justify-end gap-2">
                <Button variant="ghost" onClick={() => setConfirm(null)}>Cancel</Button>
                <Button variant="danger" onClick={() => deleteMut.mutate(confirm.id)} loading={deleteMut.isPending}>
                  Delete
                </Button>
              </div>
            </motion.div>
          </>
        )}
      </AnimatePresence>
    </div>
  );
}

function TenantPanel({
  open,
  tenant,
  onClose,
  onSaved,
}: {
  open: boolean;
  tenant: Tenant | null;
  onClose: () => void;
  onSaved: () => void;
}) {
  const isEdit = !!tenant;
  const [name, setName] = useState("");
  const [slug, setSlug] = useState("");
  const [slugTouched, setSlugTouched] = useState(false);
  const [domain, setDomain] = useState("");
  const [lang, setLang] = useState("en");
  const [active, setActive] = useState(true);
  const [error, setError] = useState<string | null>(null);

  // Seed the form whenever the panel opens for a given tenant (or for create).
  const [seededFor, setSeededFor] = useState<string | null>(null);
  const key = tenant?.id ?? "new";
  if (open && seededFor !== key) {
    setName(tenant?.name ?? "");
    setSlug(tenant?.slug ?? "");
    setSlugTouched(false);
    setDomain(tenant?.domain ?? "");
    setLang(tenant?.default_language ?? "en");
    setActive(tenant?.active ?? true);
    setError(null);
    setSeededFor(key);
  }
  if (!open && seededFor !== null) setSeededFor(null);

  const mutation = useMutation({
    mutationFn: () => {
      if (isEdit && tenant) {
        return patch(`/tenants/${tenant.id}`, {
          name,
          domain: domain || null,
          default_language: lang,
          active,
        });
      }
      return post(`/tenants`, {
        name,
        slug: slug || slugify(name),
        domain: domain || "",
        default_language: lang,
      });
    },
    onSuccess: onSaved,
    onError: (e: Error) => setError(e.message),
  });

  return (
    <AnimatePresence>
      {open && (
        <>
          <motion.div
            initial={{ opacity: 0 }} animate={{ opacity: 1 }} exit={{ opacity: 0 }}
            transition={{ duration: 0.18 }}
            className="fixed inset-0 z-40 bg-black/20 backdrop-blur-[2px]"
            onClick={onClose}
          />
          <motion.aside
            initial={{ x: "100%" }} animate={{ x: 0 }} exit={{ x: "100%" }}
            transition={{ type: "spring", damping: 28, stiffness: 280 }}
            className="fixed right-0 top-0 z-50 flex h-full w-full max-w-sm flex-col bg-white shadow-2xl dark:bg-slate-900"
          >
            <div className="flex shrink-0 items-center justify-between border-b border-slate-100 px-6 py-5 dark:border-white/10">
              <h2 className="flex items-center gap-2 text-base font-semibold text-slate-900 dark:text-slate-100">
                <Building2 size={17} className="text-slate-400" />
                {isEdit ? "Edit tenant" : "New tenant"}
              </h2>
              <button type="button" onClick={onClose} className="rounded-lg p-1.5 text-slate-400 transition hover:bg-slate-100 dark:hover:bg-white/5">
                <X size={18} />
              </button>
            </div>

            <div className="flex-1 space-y-5 overflow-y-auto px-6 py-6">
              <div>
                <label className="mb-1.5 block text-xs font-medium text-slate-700 dark:text-slate-200">
                  Name <span className="text-red-400">*</span>
                </label>
                <input
                  className="input"
                  value={name}
                  onChange={(e) => setName(e.target.value)}
                  placeholder="Acme Inc."
                />
              </div>

              <div>
                <label className="mb-1.5 block text-xs font-medium text-slate-700 dark:text-slate-200">
                  Slug <span className="text-red-400">*</span>
                </label>
                <input
                  className="input font-mono text-sm disabled:opacity-60"
                  value={slugTouched || isEdit ? slug : slugify(name)}
                  disabled={isEdit}
                  onChange={(e) => { setSlugTouched(true); setSlug(slugify(e.target.value)); }}
                  placeholder="acme"
                />
                <p className="mt-1 text-xs text-slate-400">
                  {isEdit ? "The slug can't be changed after creation." : "Used for login and identification. Auto-generated from the name."}
                </p>
              </div>

              <div>
                <label className="mb-1.5 block text-xs font-medium text-slate-700 dark:text-slate-200">Domain</label>
                <input
                  className="input"
                  value={domain}
                  onChange={(e) => setDomain(e.target.value.trim())}
                  placeholder="acme.example.com"
                />
                <p className="mt-1 text-xs text-slate-400">
                  The host that resolves to this tenant. Use <span className="font-mono">localhost</span> for the local dev tenant.
                </p>
              </div>

              <div>
                <label className="mb-1.5 block text-xs font-medium text-slate-700 dark:text-slate-200">Default language</label>
                <input
                  className="input"
                  value={lang}
                  onChange={(e) => setLang(e.target.value.trim() || "en")}
                  placeholder="en"
                />
              </div>

              {isEdit && (
                <label className="flex cursor-pointer items-start gap-3 rounded-xl border border-slate-100 bg-slate-50 px-4 py-3 dark:border-white/10 dark:bg-white/5">
                  <input type="checkbox" checked={active} onChange={(e) => setActive(e.target.checked)} className="mt-0.5 h-4 w-4 rounded border-slate-300 text-brand-600 focus:ring-brand-400 dark:border-white/10" />
                  <div>
                    <p className="text-sm font-medium text-slate-800 dark:text-slate-100">Active</p>
                    <p className="text-xs text-slate-400">Disabled tenants return 403 on every request.</p>
                  </div>
                </label>
              )}

              {!isEdit && (
                <p className="rounded-xl bg-brand-50 px-3 py-2.5 text-xs text-brand-700 dark:bg-brand-500/10 dark:text-brand-300">
                  Creating a tenant provisions its schema, base tables and a seed admin
                  (<span className="font-mono">admin@{domain || slug || slugify(name) || "slug"}</span>, password <span className="font-mono">admin</span>) automatically.
                </p>
              )}

              {error && (
                <div className="flex items-start gap-2 rounded-xl bg-red-50 px-3 py-2.5 text-sm text-red-600 dark:bg-red-500/10 dark:text-red-400">
                  <AlertTriangle size={14} className="mt-0.5 shrink-0" />
                  {error}
                </div>
              )}
            </div>

            <div className="flex shrink-0 items-center justify-end gap-2 border-t border-slate-100 px-6 py-4 dark:border-white/10">
              <Button variant="ghost" onClick={onClose}>Cancel</Button>
              <Button
                onClick={() => { setError(null); mutation.mutate(); }}
                loading={mutation.isPending}
                disabled={!name.trim() || (!isEdit && !(slug || slugify(name)))}
              >
                {isEdit ? "Save changes" : "Create tenant"}
              </Button>
            </div>
          </motion.aside>
        </>
      )}
    </AnimatePresence>
  );
}
