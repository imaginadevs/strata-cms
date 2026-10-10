/**
 * AcceptInvite — public page where an invited user sets their password.
 *
 * Reads ?token= from the URL, POSTs to /auth/accept-invite, then stores the
 * returned session and lands the user in the app. Tenant is resolved by domain,
 * so the link must be opened on the tenant's own host.
 */
import { useState } from "react";
import { useNavigate } from "@tanstack/react-router";
import { KeyRound, AlertTriangle } from "lucide-react";
import { post } from "@/lib/api";
import { Button } from "@/components/ui/Button";
import { useAuthStore } from "@/store/auth.store";
import type { TokenResponse } from "@/types";

export default function AcceptInvitePage() {
  const navigate = useNavigate();
  const setSession = useAuthStore((s) => s.setSession);

  const token = new URLSearchParams(window.location.search).get("token") ?? "";
  const [password, setPassword] = useState("");
  const [confirm, setConfirm] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    setError(null);
    if (password.length < 8) { setError("Password must be at least 8 characters."); return; }
    if (password !== confirm) { setError("Passwords don't match."); return; }
    setSubmitting(true);
    try {
      const res = await post<TokenResponse>("/auth/accept-invite", { token, password });
      setSession(res);
      navigate({ to: "/" });
    } catch (err) {
      setError(err instanceof Error ? err.message : "Could not accept the invitation.");
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <div className="flex min-h-screen items-center justify-center bg-app-bg px-4 dark:bg-slate-950">
      <div className="w-full max-w-sm rounded-2xl border border-slate-200 bg-white p-8 shadow-card dark:border-white/10 dark:bg-slate-900">
        <div className="mb-6 flex flex-col items-center text-center">
          <span className="mb-3 flex h-11 w-11 items-center justify-center rounded-xl bg-brand-600 text-white">
            <KeyRound size={20} />
          </span>
          <h1 className="text-lg font-bold text-slate-900 dark:text-slate-100">Accept your invitation</h1>
          <p className="mt-1 text-sm text-slate-500 dark:text-slate-400">Choose a password to activate your account.</p>
        </div>

        {!token ? (
          <div className="flex items-start gap-2 rounded-xl bg-red-50 px-3 py-2.5 text-sm text-red-600 dark:bg-red-500/10 dark:text-red-400">
            <AlertTriangle size={15} className="mt-0.5 shrink-0" />
            This invitation link is missing its token. Ask an admin to resend the invite.
          </div>
        ) : (
          <form onSubmit={submit} className="space-y-4">
            <div>
              <label className="mb-1.5 block text-xs font-medium text-slate-700 dark:text-slate-200">New password</label>
              <input className="input" type="password" value={password} onChange={(e) => setPassword(e.target.value)}
                placeholder="At least 8 characters" autoComplete="new-password" autoFocus />
            </div>
            <div>
              <label className="mb-1.5 block text-xs font-medium text-slate-700 dark:text-slate-200">Confirm password</label>
              <input className="input" type="password" value={confirm} onChange={(e) => setConfirm(e.target.value)}
                placeholder="Re-enter password" autoComplete="new-password" />
            </div>
            {error && (
              <div className="flex items-start gap-2 rounded-xl bg-red-50 px-3 py-2.5 text-sm text-red-600 dark:bg-red-500/10 dark:text-red-400">
                <AlertTriangle size={14} className="mt-0.5 shrink-0" />{error}
              </div>
            )}
            <Button type="submit" className="w-full" loading={submitting} disabled={!password || !confirm}>
              Activate account
            </Button>
          </form>
        )}
      </div>
    </div>
  );
}
