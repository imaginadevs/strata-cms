/**
 * Users list + edit aside.
 * Profile: PATCH /users/{id} (first_name, last_name, status, language, avatar).
 * Roles: POST/DELETE /users/{id}/roles/{roleId} (not nested in PATCH).
 * Password: POST /users/{id}/password.
 */
import { useState } from "react";
import { createPortal } from "react-dom";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { AnimatePresence, motion } from "framer-motion";
import * as Select from "@radix-ui/react-select";
import { ChevronDown, Plus, Shield, Trash2, X, Copy, Check, Mail, AlertTriangle } from "lucide-react";
import { del, get, patch, post } from "@/lib/api";
import { Card } from "@/components/ui/Card";
import { Button } from "@/components/ui/Button";
import { LanguageSelector, useAvailableLanguages } from "@/components/content/fields";
import { initials, formatDate } from "@/lib/utils";
import type { PaginatedResponse, Role, User, UserStatus } from "@/types";

const STATUSES: { value: UserStatus; label: string }[] = [
  { value: "active", label: "Active" },
  { value: "inactive", label: "Inactive" },
  { value: "suspended", label: "Suspended" },
];

type UpdateUserBody = {
  first_name: string;
  last_name: string;
  status: UserStatus;
  language: string;
};

export default function UsersPage() {
  const [editing, setEditing] = useState<User | null>(null);
  const [inviting, setInviting] = useState(false);
  const { data, isLoading } = useQuery({
    queryKey: ["users"],
    queryFn: () => get<PaginatedResponse<User>>("/users"),
  });

  return (
    <div className="space-y-5">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold text-slate-900 dark:text-slate-100">Users</h1>
          <p className="mt-1 text-sm text-slate-500 dark:text-slate-400">
            {data?.total ?? 0} total users
          </p>
        </div>
        <Button type="button" onClick={() => setInviting(true)}>
          <Plus size={16} />
          Invite User
        </Button>
      </div>

      <Card className="overflow-hidden p-0">
        <table className="w-full text-sm">
          <thead>
            <tr className="border-b border-slate-100 dark:border-white/10 text-left text-xs font-medium text-slate-500 dark:text-slate-400">
              <th className="px-5 py-3">User</th>
              <th className="px-5 py-3">Status</th>
              <th className="px-5 py-3">Roles</th>
              <th className="px-5 py-3">Provider</th>
              <th className="px-5 py-3">Last Login</th>
              <th className="px-5 py-3" />
            </tr>
          </thead>
          <tbody>
            {isLoading && (
              <tr>
                <td colSpan={6} className="py-12 text-center text-slate-400">
                  Loading…
                </td>
              </tr>
            )}
            {data?.data?.map((user) => (
              <tr
                key={user.id}
                className="border-b border-slate-100 dark:border-white/10 last:border-0 hover:bg-slate-50/60 dark:hover:bg-white/5"
              >
                <td className="px-5 py-3">
                  <div className="flex items-center gap-3">
                    <div className="flex h-8 w-8 shrink-0 items-center justify-center rounded-full bg-brand-600 text-xs font-bold text-white">
                      {initials(`${user.first_name} ${user.last_name}`)}
                    </div>
                    <div>
                      <p className="font-medium text-slate-900 dark:text-slate-100">
                        {user.first_name} {user.last_name}
                      </p>
                      <p className="text-xs text-slate-400">{user.email}</p>
                    </div>
                  </div>
                </td>
                <td className="px-5 py-3">
                  <span
                    className={`badge ${
                      user.status === "active"
                        ? "badge-green"
                        : user.status === "suspended"
                          ? "badge-red"
                          : "badge-gray"
                    }`}
                  >
                    {user.status}
                  </span>
                </td>
                <td className="px-5 py-3">
                  <div className="flex flex-wrap gap-1">
                    {(user.roles ?? []).length === 0 ? (
                      <span className="text-xs text-slate-300">—</span>
                    ) : (
                      user.roles!.map((r) => (
                        <span key={r.id} className="badge badge-blue">
                          {r.name}
                        </span>
                      ))
                    )}
                  </div>
                </td>
                <td className="px-5 py-3">
                  <span
                    className={`badge ${user.provider === "sso" ? "badge-blue" : "badge-gray"}`}
                  >
                    {user.provider}
                  </span>
                </td>
                <td className="px-5 py-3 text-slate-500 dark:text-slate-400">
                  {user.last_login ? formatDate(user.last_login) : "—"}
                </td>
                <td className="px-5 py-3 text-right">
                  <Button
                    type="button"
                    variant="ghost"
                    size="sm"
                    onClick={() => setEditing(user)}
                  >
                    Edit
                  </Button>
                </td>
              </tr>
            ))}
            {!isLoading && !data?.data?.length && (
              <tr>
                <td colSpan={6} className="py-12 text-center text-slate-400">
                  No users found
                </td>
              </tr>
            )}
          </tbody>
        </table>
      </Card>

      <AnimatePresence>
        {editing && (
          <EditUserPanel user={editing} onClose={() => setEditing(null)} />
        )}
      </AnimatePresence>

      <InviteUserPanel open={inviting} onClose={() => setInviting(false)} />
    </div>
  );
}

function EditUserPanel({ user, onClose }: { user: User; onClose: () => void }) {
  const queryClient = useQueryClient();
  const languages = useAvailableLanguages();
  const [firstName, setFirstName] = useState(user.first_name);
  const [lastName, setLastName] = useState(user.last_name);
  const [status, setStatus] = useState<UserStatus>(user.status);
  const [language, setLanguage] = useState(user.language || languages[0] || "en");
  const [password, setPassword] = useState("");
  const [passwordMsg, setPasswordMsg] = useState<string | null>(null);

  // Fresh user with roles (list payload may omit them).
  const { data: detail, isLoading: detailLoading } = useQuery({
    queryKey: ["user", user.id],
    queryFn: () => get<User>(`/users/${user.id}`),
  });
  const assigned = detail?.roles ?? user.roles ?? [];

  const { data: rolesPage } = useQuery({
    queryKey: ["roles"],
    queryFn: () => get<PaginatedResponse<Role>>("/roles"),
  });
  const allRoles = rolesPage?.data ?? [];
  const availableRoles = allRoles.filter(
    (r) => !assigned.some((a) => a.id === r.id),
  );
  const [roleToAdd, setRoleToAdd] = useState("");

  const profileMut = useMutation({
    mutationFn: (body: UpdateUserBody) => patch<User>(`/users/${user.id}`, body),
    onSuccess: (updated) => {
      queryClient.setQueryData<PaginatedResponse<User>>(["users"], (old) => {
        if (!old) return old;
        return {
          ...old,
          data: old.data.map((u) =>
            u.id === updated.id ? { ...u, ...updated, roles: assigned } : u,
          ),
        };
      });
      queryClient.invalidateQueries({ queryKey: ["users"] });
      queryClient.invalidateQueries({ queryKey: ["user", user.id] });
      onClose();
    },
  });

  const assignMut = useMutation({
    mutationFn: (roleId: string) => post(`/users/${user.id}/roles/${roleId}`),
    onSuccess: () => {
      setRoleToAdd("");
      queryClient.invalidateQueries({ queryKey: ["user", user.id] });
      queryClient.invalidateQueries({ queryKey: ["users"] });
    },
  });

  const revokeMut = useMutation({
    mutationFn: (roleId: string) => del(`/users/${user.id}/roles/${roleId}`),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["user", user.id] });
      queryClient.invalidateQueries({ queryKey: ["users"] });
    },
  });

  const passwordMut = useMutation({
    mutationFn: (pwd: string) => post(`/users/${user.id}/password`, { password: pwd }),
    onSuccess: () => {
      setPassword("");
      setPasswordMsg("Password updated");
    },
    onError: (e: Error) => setPasswordMsg(e.message),
  });

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    profileMut.mutate({
      first_name: firstName.trim(),
      last_name: lastName.trim(),
      status,
      language,
    });
  }

  const busy =
    profileMut.isPending ||
    assignMut.isPending ||
    revokeMut.isPending ||
    passwordMut.isPending;

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
        className="fixed right-0 top-0 z-50 flex h-full w-full max-w-md flex-col bg-white dark:bg-slate-900 shadow-2xl"
      >
        <form onSubmit={handleSubmit} className="flex h-full flex-col">
          <div className="flex shrink-0 items-center justify-between border-b border-slate-100 dark:border-white/10 px-6 py-5">
            <div>
              <h2 className="text-base font-semibold text-slate-900 dark:text-slate-100">
                Edit User
              </h2>
              <p className="mt-0.5 text-xs text-slate-400">{user.email}</p>
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
            <div className="grid grid-cols-2 gap-3">
              <div>
                <label className="mb-1.5 block text-xs font-medium text-slate-700 dark:text-slate-200">
                  First name
                </label>
                <input
                  className="input"
                  value={firstName}
                  onChange={(e) => setFirstName(e.target.value)}
                  required
                />
              </div>
              <div>
                <label className="mb-1.5 block text-xs font-medium text-slate-700 dark:text-slate-200">
                  Last name
                </label>
                <input
                  className="input"
                  value={lastName}
                  onChange={(e) => setLastName(e.target.value)}
                  required
                />
              </div>
            </div>

            <div>
              <label className="mb-1.5 block text-xs font-medium text-slate-700 dark:text-slate-200">
                Status
              </label>
              <Select.Root value={status} onValueChange={(v) => setStatus(v as UserStatus)}>
                <Select.Trigger className="flex h-10 w-full items-center justify-between rounded-lg border border-slate-200 dark:border-white/10 bg-white dark:bg-white/5 px-3 text-sm text-slate-800 dark:text-slate-100 focus:outline-none focus:ring-2 focus:ring-brand-100 dark:focus:ring-brand-500/20">
                  <Select.Value />
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
                      {STATUSES.map((s) => (
                        <Select.Item
                          key={s.value}
                          value={s.value}
                          className="cursor-pointer rounded-lg px-3 py-2 text-sm text-slate-700 dark:text-slate-200 outline-none data-[highlighted]:bg-brand-50 dark:data-[highlighted]:bg-brand-500/15 data-[highlighted]:text-brand-700 dark:data-[highlighted]:text-brand-300"
                        >
                          <Select.ItemText>{s.label}</Select.ItemText>
                        </Select.Item>
                      ))}
                    </Select.Viewport>
                  </Select.Content>
                </Select.Portal>
              </Select.Root>
            </div>

            <div>
              <label className="mb-1.5 block text-xs font-medium text-slate-700 dark:text-slate-200">
                Language
              </label>
              <LanguageSelector
                value={language}
                onChange={setLanguage}
                languages={languages}
              />
            </div>

            {/* Roles */}
            <div className="space-y-3">
              <div className="flex items-center gap-2">
                <Shield size={14} className="text-slate-400" />
                <p className="text-xs font-semibold uppercase tracking-wide text-slate-500 dark:text-slate-400">
                  Roles
                </p>
              </div>
              <p className="text-[11px] text-slate-400">
                Collection permissions come from roles. Assign roles here; edit
                permissions on the Roles page.
              </p>

              {detailLoading ? (
                <p className="text-xs text-slate-400">Loading roles…</p>
              ) : assigned.length === 0 ? (
                <p className="rounded-xl border border-dashed border-slate-200 dark:border-white/10 px-3 py-4 text-center text-xs text-slate-400">
                  No roles assigned
                </p>
              ) : (
                <ul className="space-y-1.5">
                  {assigned.map((r) => (
                    <li
                      key={r.id}
                      className="flex items-center justify-between gap-2 rounded-xl border border-slate-100 dark:border-white/10 bg-slate-50/80 dark:bg-white/[0.03] px-3 py-2"
                    >
                      <div className="min-w-0">
                        <p className="truncate text-sm font-medium text-slate-800 dark:text-slate-100">
                          {r.name}
                          {r.is_system && (
                            <span className="ml-1.5 badge badge-gray">system</span>
                          )}
                        </p>
                        {r.description && (
                          <p className="truncate text-[11px] text-slate-400">{r.description}</p>
                        )}
                      </div>
                      <button
                        type="button"
                        disabled={busy}
                        onClick={() => revokeMut.mutate(r.id)}
                        className="rounded-lg p-1.5 text-slate-300 transition hover:bg-red-50 hover:text-red-500 dark:hover:bg-red-500/10"
                        title="Revoke role"
                      >
                        <Trash2 size={13} />
                      </button>
                    </li>
                  ))}
                </ul>
              )}

              {availableRoles.length > 0 && (
                <div className="flex gap-2">
                  <Select.Root value={roleToAdd || undefined} onValueChange={setRoleToAdd}>
                    <Select.Trigger className="flex h-9 min-w-0 flex-1 items-center justify-between rounded-lg border border-slate-200 dark:border-white/10 bg-white dark:bg-white/5 px-2.5 text-xs text-slate-800 dark:text-slate-100 focus:outline-none focus:ring-2 focus:ring-brand-100">
                      <Select.Value placeholder="Add a role…" />
                      <Select.Icon>
                        <ChevronDown size={12} className="text-slate-400" />
                      </Select.Icon>
                    </Select.Trigger>
                    <Select.Portal>
                      <Select.Content
                        position="popper"
                        sideOffset={4}
                        className="z-[60] overflow-hidden rounded-xl border border-slate-100 dark:border-white/10 bg-white dark:bg-slate-800 p-1 shadow-lg"
                      >
                        <Select.Viewport>
                          {availableRoles.map((r) => (
                            <Select.Item
                              key={r.id}
                              value={r.id}
                              className="cursor-pointer rounded-lg px-2.5 py-1.5 text-xs text-slate-700 dark:text-slate-200 outline-none data-[highlighted]:bg-brand-50 dark:data-[highlighted]:bg-brand-500/15"
                            >
                              <Select.ItemText>{r.name}</Select.ItemText>
                            </Select.Item>
                          ))}
                        </Select.Viewport>
                      </Select.Content>
                    </Select.Portal>
                  </Select.Root>
                  <Button
                    type="button"
                    size="sm"
                    disabled={!roleToAdd || busy}
                    onClick={() => roleToAdd && assignMut.mutate(roleToAdd)}
                  >
                    Add
                  </Button>
                </div>
              )}
              {(assignMut.isError || revokeMut.isError) && (
                <p className="text-xs text-red-500">
                  {((assignMut.error ?? revokeMut.error) as Error).message}
                </p>
              )}
            </div>

            {/* Password */}
            {user.provider === "local" && (
              <div className="space-y-3 rounded-xl border border-slate-100 dark:border-white/10 px-4 py-4">
                <p className="text-xs font-semibold uppercase tracking-wide text-slate-500 dark:text-slate-400">
                  Password
                </p>
                <input
                  type="password"
                  className="input"
                  value={password}
                  onChange={(e) => {
                    setPassword(e.target.value);
                    setPasswordMsg(null);
                  }}
                  placeholder="New password (min 8)"
                  autoComplete="new-password"
                />
                <Button
                  type="button"
                  size="sm"
                  variant="outline"
                  disabled={busy || password.length < 8}
                  onClick={() => passwordMut.mutate(password)}
                >
                  {passwordMut.isPending ? "Updating…" : "Update password"}
                </Button>
                {passwordMsg && (
                  <p
                    className={`text-xs ${
                      passwordMut.isError ? "text-red-500" : "text-emerald-600 dark:text-emerald-400"
                    }`}
                  >
                    {passwordMsg}
                  </p>
                )}
              </div>
            )}

            <div className="rounded-xl border border-slate-100 dark:border-white/10 bg-slate-50/80 dark:bg-white/[0.03] px-4 py-3 text-xs text-slate-400">
              <p>
                Provider:{" "}
                <span className="font-medium text-slate-600 dark:text-slate-300">
                  {user.provider}
                </span>
              </p>
              <p className="mt-1">Email cannot be changed here.</p>
            </div>

            {profileMut.isError && (
              <p className="rounded-xl bg-red-50 dark:bg-red-500/10 px-3 py-2 text-sm text-red-600 dark:text-red-400">
                {(profileMut.error as Error).message}
              </p>
            )}
          </div>

          <div className="flex shrink-0 items-center justify-between border-t border-slate-100 dark:border-white/10 px-6 py-4">
            <Button type="button" variant="ghost" onClick={onClose}>
              Cancel
            </Button>
            <Button
              type="submit"
              disabled={busy || !firstName.trim() || !lastName.trim()}
            >
              {profileMut.isPending ? "Saving…" : "Save Changes"}
            </Button>
          </div>
        </form>
      </motion.aside>
    </>,
    document.body,
  );
}

// ── Invite user panel ───────────────────────────────────────────────────────────

function InviteUserPanel({ open, onClose }: { open: boolean; onClose: () => void }) {
  const qc = useQueryClient();
  const [email, setEmail] = useState("");
  const [firstName, setFirstName] = useState("");
  const [lastName, setLastName] = useState("");
  const [roleIds, setRoleIds] = useState<string[]>([]);
  const [result, setResult] = useState<{ invite_link: string; email_sent: boolean } | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [copied, setCopied] = useState(false);

  const { data: rolesResp } = useQuery({
    queryKey: ["roles"],
    queryFn: () => get<PaginatedResponse<Role>>("/roles"),
    enabled: open,
  });
  const roles = rolesResp?.data ?? [];

  function reset() {
    setEmail(""); setFirstName(""); setLastName(""); setRoleIds([]);
    setResult(null); setError(null); setCopied(false);
  }
  function close() { reset(); onClose(); }

  const mutation = useMutation({
    mutationFn: () =>
      post<{ invite_link: string; email_sent: boolean }>("/users/invite", {
        email, first_name: firstName, last_name: lastName, role_ids: roleIds,
      }),
    onSuccess: (res) => {
      setResult({ invite_link: res.invite_link, email_sent: res.email_sent });
      qc.invalidateQueries({ queryKey: ["users"] });
    },
    onError: (e: Error) => setError(e.message),
  });

  const toggleRole = (id: string) =>
    setRoleIds((prev) => (prev.includes(id) ? prev.filter((r) => r !== id) : [...prev, id]));

  return createPortal(
    <AnimatePresence>
      {open && (
        <>
          <motion.div
            initial={{ opacity: 0 }} animate={{ opacity: 1 }} exit={{ opacity: 0 }}
            transition={{ duration: 0.18 }}
            className="fixed inset-0 z-40 bg-black/20 backdrop-blur-[2px]"
            onClick={close}
          />
          <motion.aside
            initial={{ x: "100%" }} animate={{ x: 0 }} exit={{ x: "100%" }}
            transition={{ type: "spring", damping: 28, stiffness: 280 }}
            className="fixed right-0 top-0 z-50 flex h-full w-full max-w-sm flex-col bg-white shadow-2xl dark:bg-slate-900"
          >
            <div className="flex shrink-0 items-center justify-between border-b border-slate-100 px-6 py-5 dark:border-white/10">
              <h2 className="flex items-center gap-2 text-base font-semibold text-slate-900 dark:text-slate-100">
                <Mail size={17} className="text-slate-400" />
                Invite user
              </h2>
              <button type="button" onClick={close} className="rounded-lg p-1.5 text-slate-400 transition hover:bg-slate-100 dark:hover:bg-white/5">
                <X size={18} />
              </button>
            </div>

            {result ? (
              <div className="flex-1 space-y-4 overflow-y-auto px-6 py-6">
                <div className="flex items-start gap-2 rounded-xl bg-emerald-50 px-3 py-2.5 text-sm text-emerald-700 dark:bg-emerald-500/10 dark:text-emerald-300">
                  <Check size={15} className="mt-0.5 shrink-0" />
                  {result.email_sent
                    ? `Invitation emailed to ${email}.`
                    : `User created. Email isn't configured — share this invite link manually.`}
                </div>
                <div>
                  <label className="mb-1.5 block text-xs font-medium text-slate-700 dark:text-slate-200">Invite link</label>
                  <div className="flex items-center gap-2 rounded-xl border border-slate-200 bg-slate-50 p-2.5 dark:border-white/10 dark:bg-white/5">
                    <code className="flex-1 break-all font-mono text-xs text-slate-700 dark:text-slate-200">{result.invite_link}</code>
                    <button type="button"
                      onClick={() => { navigator.clipboard?.writeText(result.invite_link); setCopied(true); setTimeout(() => setCopied(false), 1500); }}
                      className="flex h-8 w-8 shrink-0 items-center justify-center rounded-lg border border-slate-200 text-slate-500 transition hover:bg-white dark:border-white/10 dark:hover:bg-white/10">
                      {copied ? <Check size={14} className="text-emerald-500" /> : <Copy size={14} />}
                    </button>
                  </div>
                  <p className="mt-1 text-xs text-slate-400">Valid for 7 days. The recipient sets their password to activate the account.</p>
                </div>
                <div className="flex gap-2 pt-2">
                  <Button variant="ghost" onClick={reset}>Invite another</Button>
                  <Button onClick={close}>Done</Button>
                </div>
              </div>
            ) : (
              <>
                <div className="flex-1 space-y-5 overflow-y-auto px-6 py-6">
                  <div>
                    <label className="mb-1.5 block text-xs font-medium text-slate-700 dark:text-slate-200">Email <span className="text-red-400">*</span></label>
                    <input className="input" type="email" value={email} onChange={(e) => setEmail(e.target.value)} placeholder="person@company.com" />
                  </div>
                  <div className="grid grid-cols-2 gap-3">
                    <div>
                      <label className="mb-1.5 block text-xs font-medium text-slate-700 dark:text-slate-200">First name</label>
                      <input className="input" value={firstName} onChange={(e) => setFirstName(e.target.value)} placeholder="Jane" />
                    </div>
                    <div>
                      <label className="mb-1.5 block text-xs font-medium text-slate-700 dark:text-slate-200">Last name</label>
                      <input className="input" value={lastName} onChange={(e) => setLastName(e.target.value)} placeholder="Doe" />
                    </div>
                  </div>
                  <div>
                    <label className="mb-1.5 block text-xs font-medium text-slate-700 dark:text-slate-200">Roles</label>
                    <div className="space-y-1.5">
                      {roles.length === 0 && <p className="text-xs text-slate-400">No roles available.</p>}
                      {roles.map((role) => (
                        <label key={role.id} className="flex cursor-pointer items-center gap-2.5 rounded-lg border border-slate-100 px-3 py-2 dark:border-white/10">
                          <input type="checkbox" checked={roleIds.includes(role.id)} onChange={() => toggleRole(role.id)}
                            className="h-4 w-4 rounded border-slate-300 text-brand-600 focus:ring-brand-400 dark:border-white/10" />
                          <span className="text-sm text-slate-700 dark:text-slate-200">{role.name}</span>
                        </label>
                      ))}
                    </div>
                  </div>
                  {error && (
                    <div className="flex items-start gap-2 rounded-xl bg-red-50 px-3 py-2.5 text-sm text-red-600 dark:bg-red-500/10 dark:text-red-400">
                      <AlertTriangle size={14} className="mt-0.5 shrink-0" />{error}
                    </div>
                  )}
                </div>
                <div className="flex shrink-0 items-center justify-end gap-2 border-t border-slate-100 px-6 py-4 dark:border-white/10">
                  <Button variant="ghost" onClick={close}>Cancel</Button>
                  <Button onClick={() => { setError(null); mutation.mutate(); }} loading={mutation.isPending} disabled={!email.trim()}>
                    Send invite
                  </Button>
                </div>
              </>
            )}
          </motion.aside>
        </>
      )}
    </AnimatePresence>,
    document.body,
  );
}
