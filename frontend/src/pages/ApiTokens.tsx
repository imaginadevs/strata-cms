/**
 * ApiTokens — manage policy-scoped programmatic access tokens.
 *
 * The plaintext secret is shown exactly once (after create or rotate) in a reveal
 * modal; afterwards only the prefix + metadata are visible. Policies reuse the RBAC
 * (collection, action) model and stay editable. Tokens rotate with a 24h grace and
 * can be forever or monthly (admins emailed 24h before an auto-rotation).
 */
import { useState } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { motion, AnimatePresence } from "framer-motion";
import {
  KeyRound, Plus, X, Copy, Check, RefreshCw, Trash2, Ban, AlertTriangle, ShieldCheck,
} from "lucide-react";
import { get, post, patch, del } from "@/lib/api";
import { Button } from "@/components/ui/Button";
import type { ApiToken, ApiTokenSecret, ApiTokenPolicy, Collection, PaginatedResponse } from "@/types";

const ACTIONS = ["read", "create", "update", "delete", "all"] as const;
const VIRTUAL_RESOURCES = [
  { value: "cms_users", label: "Users" },
  { value: "cms_roles", label: "Roles" },
  { value: "cms_schema", label: "Collection config" },
  { value: "cms_settings", label: "Settings" },
  { value: "cms_audit", label: "Audit logs" },
  { value: "cms_workflows", label: "Workflows" },
];

function fmtDate(s?: string | null) {
  if (!s) return "—";
  return new Date(s).toLocaleString();
}

export default function ApiTokensPage() {
  const qc = useQueryClient();
  const [panelOpen, setPanelOpen] = useState(false);
  const [editing, setEditing] = useState<ApiToken | null>(null);
  const [reveal, setReveal] = useState<ApiTokenSecret | null>(null);
  const [rotating, setRotating] = useState<ApiToken | null>(null);
  const [confirmDelete, setConfirmDelete] = useState<ApiToken | null>(null);

  const { data: tokens = [], isLoading } = useQuery({
    queryKey: ["api-tokens"],
    queryFn: () => get<ApiToken[]>("/api-tokens"),
  });
  const { data: collections = [] } = useQuery({
    queryKey: ["collections"],
    queryFn: () => get<PaginatedResponse<Collection>>("/collections").then((r) => r.data ?? []),
  });

  const invalidate = () => qc.invalidateQueries({ queryKey: ["api-tokens"] });

  const rotateMut = useMutation({
    mutationFn: (v: { id: string; mode: "now" | "grace" }) =>
      post<ApiTokenSecret>(`/api-tokens/${v.id}/rotate`, { mode: v.mode }),
    onSuccess: (res) => { setRotating(null); setReveal(res); invalidate(); },
  });
  const revokeMut = useMutation({
    mutationFn: (v: { id: string; revoked: boolean }) => patch(`/api-tokens/${v.id}`, { revoked: v.revoked }),
    onSuccess: invalidate,
  });
  const deleteMut = useMutation({
    mutationFn: (id: string) => del(`/api-tokens/${id}`),
    onSuccess: () => { setConfirmDelete(null); invalidate(); },
  });

  return (
    <div className="mx-auto max-w-5xl px-4 py-6 sm:px-6">
      <div className="mb-6 flex items-center justify-between">
        <div>
          <h1 className="text-xl font-bold text-slate-900 dark:text-slate-100">API tokens</h1>
          <p className="text-sm text-slate-400">Policy-scoped tokens for programmatic access.</p>
        </div>
        <Button onClick={() => { setEditing(null); setPanelOpen(true); }}>
          <Plus size={15} className="mr-1.5" />
          New token
        </Button>
      </div>

      <div className="overflow-hidden rounded-xl border border-slate-200 bg-white dark:border-white/10 dark:bg-white/5">
        <table className="w-full text-sm">
          <thead>
            <tr className="border-b border-slate-100 text-left text-xs uppercase tracking-wide text-slate-400 dark:border-white/10">
              <th className="px-4 py-3 font-semibold">Name</th>
              <th className="px-4 py-3 font-semibold">Prefix</th>
              <th className="px-4 py-3 font-semibold">Policies</th>
              <th className="px-4 py-3 font-semibold">Lifetime</th>
              <th className="px-4 py-3 font-semibold">Status</th>
              <th className="px-4 py-3" />
            </tr>
          </thead>
          <tbody>
            {isLoading && <tr><td colSpan={6} className="px-4 py-8 text-center text-slate-400">Loading…</td></tr>}
            {!isLoading && tokens.length === 0 && (
              <tr><td colSpan={6} className="px-4 py-8 text-center text-slate-400">No tokens yet.</td></tr>
            )}
            {tokens.map((t) => (
              <tr key={t.id} className="border-b border-slate-50 last:border-0 dark:border-white/5">
                <td className="px-4 py-3">
                  <div className="flex items-center gap-2.5">
                    <span className="flex h-8 w-8 items-center justify-center rounded-lg bg-slate-100 text-slate-500 dark:bg-white/10 dark:text-slate-300">
                      <KeyRound size={15} />
                    </span>
                    <div>
                      <p className="font-medium text-slate-800 dark:text-slate-100">{t.name}</p>
                      <p className="text-xs text-slate-400">last used {fmtDate(t.last_used_at)}</p>
                    </div>
                  </div>
                </td>
                <td className="px-4 py-3 font-mono text-xs text-slate-500 dark:text-slate-400">cms_{t.secret_prefix}_…</td>
                <td className="px-4 py-3">
                  <div className="flex flex-wrap gap-1">
                    {t.policies.length === 0 && <span className="text-xs text-slate-300 dark:text-slate-600">none</span>}
                    {t.policies.slice(0, 3).map((p, i) => (
                      <span key={i} className="rounded bg-slate-100 px-1.5 py-0.5 font-mono text-[11px] text-slate-600 dark:bg-white/10 dark:text-slate-300">
                        {p.collection}:{p.action}
                      </span>
                    ))}
                    {t.policies.length > 3 && <span className="text-[11px] text-slate-400">+{t.policies.length - 3}</span>}
                  </div>
                </td>
                <td className="px-4 py-3 text-slate-500 dark:text-slate-400">{t.lifetime}</td>
                <td className="px-4 py-3">
                  <span className={`inline-flex rounded-full px-2 py-0.5 text-xs font-medium ${
                    t.revoked
                      ? "bg-red-50 text-red-600 dark:bg-red-500/15 dark:text-red-300"
                      : "bg-emerald-50 text-emerald-600 dark:bg-emerald-500/15 dark:text-emerald-300"
                  }`}>
                    {t.revoked ? "Revoked" : "Active"}
                  </span>
                </td>
                <td className="px-4 py-3">
                  <div className="flex items-center justify-end gap-1">
                    <button type="button" onClick={() => { setEditing(t); setPanelOpen(true); }} title="Edit policies"
                      className="flex h-8 w-8 items-center justify-center rounded-lg text-slate-400 transition hover:bg-slate-100 hover:text-brand-600 dark:hover:bg-white/10">
                      <ShieldCheck size={14} />
                    </button>
                    <button type="button" onClick={() => setRotating(t)} title="Rotate" disabled={t.revoked}
                      className="flex h-8 w-8 items-center justify-center rounded-lg text-slate-400 transition hover:bg-slate-100 hover:text-brand-600 disabled:opacity-30 dark:hover:bg-white/10">
                      <RefreshCw size={14} />
                    </button>
                    <button type="button" onClick={() => revokeMut.mutate({ id: t.id, revoked: !t.revoked })} title={t.revoked ? "Un-revoke" : "Revoke"}
                      className="flex h-8 w-8 items-center justify-center rounded-lg text-slate-400 transition hover:bg-amber-50 hover:text-amber-600 dark:hover:bg-amber-500/10">
                      <Ban size={14} />
                    </button>
                    <button type="button" onClick={() => setConfirmDelete(t)} title="Delete"
                      className="flex h-8 w-8 items-center justify-center rounded-lg text-slate-400 transition hover:bg-red-50 hover:text-red-500 dark:hover:bg-red-500/10">
                      <Trash2 size={14} />
                    </button>
                  </div>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>

      <TokenPanel
        open={panelOpen}
        token={editing}
        collections={collections}
        onClose={() => setPanelOpen(false)}
        onCreated={(res) => { setPanelOpen(false); setReveal(res); invalidate(); }}
        onSaved={() => { setPanelOpen(false); invalidate(); }}
      />

      <RevealModal data={reveal} onClose={() => setReveal(null)} />

      {/* Rotate chooser */}
      <AnimatePresence>
        {rotating && (
          <Backdrop onClose={() => setRotating(null)}>
            <h2 className="text-base font-semibold text-slate-900 dark:text-slate-100">Rotate “{rotating.name}”</h2>
            <p className="mt-2 text-sm text-slate-500 dark:text-slate-400">
              A new secret is generated and shown once. Choose how the current secret is retired.
            </p>
            <div className="mt-4 space-y-2">
              <button type="button" onClick={() => rotateMut.mutate({ id: rotating.id, mode: "grace" })}
                className="w-full rounded-xl border border-slate-200 p-3 text-left transition hover:border-brand-400 dark:border-white/10">
                <p className="text-sm font-medium text-slate-800 dark:text-slate-100">Grace — keep old valid 24h</p>
                <p className="text-xs text-slate-400">Recommended. Gives integrations a day to switch over.</p>
              </button>
              <button type="button" onClick={() => rotateMut.mutate({ id: rotating.id, mode: "now" })}
                className="w-full rounded-xl border border-slate-200 p-3 text-left transition hover:border-red-400 dark:border-white/10">
                <p className="text-sm font-medium text-slate-800 dark:text-slate-100">Now — revoke old immediately</p>
                <p className="text-xs text-slate-400">The current secret stops working at once.</p>
              </button>
            </div>
          </Backdrop>
        )}
      </AnimatePresence>

      {/* Delete confirm */}
      <AnimatePresence>
        {confirmDelete && (
          <Backdrop onClose={() => setConfirmDelete(null)}>
            <h2 className="text-base font-semibold text-slate-900 dark:text-slate-100">Delete token?</h2>
            <p className="mt-2 text-sm text-slate-500 dark:text-slate-400">
              <span className="font-medium">{confirmDelete.name}</span> will stop working immediately and can't be recovered.
            </p>
            <div className="mt-5 flex justify-end gap-2">
              <Button variant="ghost" onClick={() => setConfirmDelete(null)}>Cancel</Button>
              <Button variant="danger" loading={deleteMut.isPending} onClick={() => deleteMut.mutate(confirmDelete.id)}>Delete</Button>
            </div>
          </Backdrop>
        )}
      </AnimatePresence>
    </div>
  );
}

function Backdrop({ children, onClose }: { children: React.ReactNode; onClose: () => void }) {
  return (
    <>
      <motion.div initial={{ opacity: 0 }} animate={{ opacity: 1 }} exit={{ opacity: 0 }}
        className="fixed inset-0 z-40 bg-black/20 backdrop-blur-[2px]" onClick={onClose} />
      <motion.div initial={{ opacity: 0, scale: 0.96 }} animate={{ opacity: 1, scale: 1 }} exit={{ opacity: 0, scale: 0.96 }}
        className="fixed left-1/2 top-1/2 z-50 w-full max-w-md -translate-x-1/2 -translate-y-1/2 rounded-2xl bg-white p-6 shadow-2xl dark:bg-slate-900">
        {children}
      </motion.div>
    </>
  );
}

function RevealModal({ data, onClose }: { data: ApiTokenSecret | null; onClose: () => void }) {
  const [copied, setCopied] = useState(false);
  return (
    <AnimatePresence>
      {data && (
        <Backdrop onClose={onClose}>
          <div className="flex items-center gap-2">
            <ShieldCheck size={18} className="text-emerald-500" />
            <h2 className="text-base font-semibold text-slate-900 dark:text-slate-100">Copy your secret now</h2>
          </div>
          <p className="mt-2 text-sm text-slate-500 dark:text-slate-400">
            This is the only time “{data.token.name}”’s secret is shown. Store it securely — you can't retrieve it later.
          </p>
          <div className="mt-4 flex items-center gap-2 rounded-xl border border-slate-200 bg-slate-50 p-3 dark:border-white/10 dark:bg-white/5">
            <code className="flex-1 break-all font-mono text-xs text-slate-800 dark:text-slate-100">{data.secret}</code>
            <button type="button"
              onClick={() => { navigator.clipboard?.writeText(data.secret); setCopied(true); setTimeout(() => setCopied(false), 1500); }}
              className="flex h-8 w-8 shrink-0 items-center justify-center rounded-lg border border-slate-200 text-slate-500 transition hover:bg-white dark:border-white/10 dark:hover:bg-white/10">
              {copied ? <Check size={14} className="text-emerald-500" /> : <Copy size={14} />}
            </button>
          </div>
          <div className="mt-5 flex justify-end">
            <Button onClick={onClose}>Done</Button>
          </div>
        </Backdrop>
      )}
    </AnimatePresence>
  );
}

function TokenPanel({
  open, token, collections, onClose, onCreated, onSaved,
}: {
  open: boolean;
  token: ApiToken | null;
  collections: Collection[];
  onClose: () => void;
  onCreated: (res: ApiTokenSecret) => void;
  onSaved: () => void;
}) {
  const isEdit = !!token;
  const [name, setName] = useState("");
  const [lifetime, setLifetime] = useState<"forever" | "monthly">("forever");
  const [policies, setPolicies] = useState<ApiTokenPolicy[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [seededFor, setSeededFor] = useState<string | null>(null);

  const key = token?.id ?? "new";
  if (open && seededFor !== key) {
    setName(token?.name ?? "");
    setLifetime(token?.lifetime ?? "forever");
    setPolicies(token?.policies?.length ? token.policies.map((p) => ({ ...p })) : [{ collection: "*", action: "read" }]);
    setError(null);
    setSeededFor(key);
  }
  if (!open && seededFor !== null) setSeededFor(null);

  const mutation = useMutation({
    mutationFn: () => {
      const clean = policies.filter((p) => p.collection && p.action);
      if (isEdit && token) {
        return patch<ApiToken>(`/api-tokens/${token.id}`, { name, lifetime, policies: clean }).then(() => undefined);
      }
      return post<ApiTokenSecret>(`/api-tokens`, { name, lifetime, policies: clean });
    },
    onSuccess: (res) => {
      if (isEdit) onSaved();
      else onCreated(res as ApiTokenSecret);
    },
    onError: (e: Error) => setError(e.message),
  });

  const setRow = (i: number, patchRow: Partial<ApiTokenPolicy>) =>
    setPolicies((prev) => prev.map((p, j) => (j === i ? { ...p, ...patchRow } : p)));

  return (
    <AnimatePresence>
      {open && (
        <>
          <motion.div initial={{ opacity: 0 }} animate={{ opacity: 1 }} exit={{ opacity: 0 }}
            transition={{ duration: 0.18 }} className="fixed inset-0 z-40 bg-black/20 backdrop-blur-[2px]" onClick={onClose} />
          <motion.aside initial={{ x: "100%" }} animate={{ x: 0 }} exit={{ x: "100%" }}
            transition={{ type: "spring", damping: 28, stiffness: 280 }}
            className="fixed right-0 top-0 z-50 flex h-full w-full max-w-md flex-col bg-white shadow-2xl dark:bg-slate-900">
            <div className="flex shrink-0 items-center justify-between border-b border-slate-100 px-6 py-5 dark:border-white/10">
              <h2 className="flex items-center gap-2 text-base font-semibold text-slate-900 dark:text-slate-100">
                <KeyRound size={17} className="text-slate-400" />
                {isEdit ? "Edit token" : "New API token"}
              </h2>
              <button type="button" onClick={onClose} className="rounded-lg p-1.5 text-slate-400 transition hover:bg-slate-100 dark:hover:bg-white/5">
                <X size={18} />
              </button>
            </div>

            <div className="flex-1 space-y-5 overflow-y-auto px-6 py-6">
              <div>
                <label className="mb-1.5 block text-xs font-medium text-slate-700 dark:text-slate-200">Name <span className="text-red-400">*</span></label>
                <input className="input" value={name} onChange={(e) => setName(e.target.value)} placeholder="CI deploy bot" />
              </div>

              <div>
                <label className="mb-1.5 block text-xs font-medium text-slate-700 dark:text-slate-200">Lifetime</label>
                <div className="flex gap-2">
                  {(["forever", "monthly"] as const).map((lt) => (
                    <button key={lt} type="button" onClick={() => setLifetime(lt)}
                      className={`flex-1 rounded-xl border py-2 text-xs font-medium capitalize transition ${
                        lifetime === lt
                          ? "border-brand-400 bg-brand-50 text-brand-700 dark:bg-brand-500/15 dark:text-brand-300"
                          : "border-slate-200 text-slate-500 hover:bg-slate-50 dark:border-white/10 dark:text-slate-400 dark:hover:bg-white/5"
                      }`}>
                      {lt}
                    </button>
                  ))}
                </div>
                <p className="mt-1 text-xs text-slate-400">
                  {lifetime === "monthly"
                    ? "Auto-rotates every 30 days (24h grace). Admins are emailed 24h before — rotate manually then to capture the new secret."
                    : "Never auto-rotates. Rotate manually whenever you need to."}
                </p>
              </div>

              <div>
                <div className="mb-1.5 flex items-center justify-between">
                  <label className="text-xs font-medium text-slate-700 dark:text-slate-200">Policies</label>
                  <button type="button" onClick={() => setPolicies((p) => [...p, { collection: "*", action: "read" }])}
                    className="flex items-center gap-1 text-xs font-medium text-brand-600 hover:text-brand-700 dark:text-brand-300">
                    <Plus size={12} /> Add
                  </button>
                </div>
                <div className="space-y-2">
                  {policies.map((p, i) => (
                    <div key={i} className="flex items-center gap-2">
                      <select value={p.collection} onChange={(e) => setRow(i, { collection: e.target.value })}
                        className="min-w-0 flex-1 rounded-lg border border-slate-200 bg-white px-2 py-1.5 text-xs text-slate-700 focus:border-brand-400 focus:outline-none dark:border-white/10 dark:bg-white/5 dark:text-slate-200">
                        <option value="*">★ All resources (*)</option>
                        <optgroup label="System">
                          {VIRTUAL_RESOURCES.map((v) => <option key={v.value} value={v.value}>{v.label}</option>)}
                        </optgroup>
                        {collections.length > 0 && (
                          <optgroup label="Collections">
                            {collections.map((c) => <option key={c.id} value={c.name}>{c.display_name} ({c.name})</option>)}
                          </optgroup>
                        )}
                      </select>
                      <select value={p.action} onChange={(e) => setRow(i, { action: e.target.value })}
                        className="w-28 rounded-lg border border-slate-200 bg-white px-2 py-1.5 text-xs text-slate-700 focus:border-brand-400 focus:outline-none dark:border-white/10 dark:bg-white/5 dark:text-slate-200">
                        {ACTIONS.map((a) => <option key={a} value={a}>{a}</option>)}
                      </select>
                      <button type="button" onClick={() => setPolicies((prev) => prev.filter((_, j) => j !== i))}
                        className="flex h-7 w-7 shrink-0 items-center justify-center rounded-lg text-slate-300 transition hover:bg-red-50 hover:text-red-500 dark:hover:bg-red-500/10">
                        <X size={13} />
                      </button>
                    </div>
                  ))}
                  {policies.length === 0 && (
                    <p className="rounded-lg border border-dashed border-slate-200 px-3 py-3 text-center text-xs text-slate-400 dark:border-white/10">
                      No policies — this token can't access anything. Add at least one.
                    </p>
                  )}
                </div>
                <p className="mt-1.5 text-xs text-slate-400">
                  Each row grants an action on a resource. “All resources” (*) and action “all” are wildcards.
                </p>
              </div>

              {error && (
                <div className="flex items-start gap-2 rounded-xl bg-red-50 px-3 py-2.5 text-sm text-red-600 dark:bg-red-500/10 dark:text-red-400">
                  <AlertTriangle size={14} className="mt-0.5 shrink-0" />{error}
                </div>
              )}
            </div>

            <div className="flex shrink-0 items-center justify-end gap-2 border-t border-slate-100 px-6 py-4 dark:border-white/10">
              <Button variant="ghost" onClick={onClose}>Cancel</Button>
              <Button onClick={() => { setError(null); mutation.mutate(); }} loading={mutation.isPending} disabled={!name.trim()}>
                {isEdit ? "Save changes" : "Create token"}
              </Button>
            </div>
          </motion.aside>
        </>
      )}
    </AnimatePresence>
  );
}
