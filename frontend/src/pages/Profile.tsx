/**
 * Self profile — GET/PATCH /users/me, POST /users/me/password.
 * No cms_users permission required. Password change requires confirmation.
 */
import { useEffect, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { CheckCircle2, KeyRound, User } from "lucide-react";
import { get, patch, post } from "@/lib/api";
import { useAuthStore } from "@/store/auth.store";
import { Card, CardHeader, CardTitle } from "@/components/ui/Card";
import { Button } from "@/components/ui/Button";
import {
  LanguageSelector,
  useAvailableLanguages,
} from "@/components/content/fields";
import { initials } from "@/lib/utils";
import type { User as CmsUser } from "@/types";

export default function ProfilePage() {
  const queryClient = useQueryClient();
  const updateUser = useAuthStore((s) => s.updateUser);
  const languages = useAvailableLanguages();

  const { data: me, isLoading } = useQuery({
    queryKey: ["users", "me"],
    queryFn: () => get<CmsUser>("/users/me"),
  });

  const [firstName, setFirstName] = useState("");
  const [lastName, setLastName] = useState("");
  const [language, setLanguage] = useState("en");
  const [profileSaved, setProfileSaved] = useState(false);

  const [password, setPassword] = useState("");
  const [confirm, setConfirm] = useState("");
  const [pwdError, setPwdError] = useState<string | null>(null);
  const [pwdSaved, setPwdSaved] = useState(false);

  useEffect(() => {
    if (!me) return;
    setFirstName(me.first_name);
    setLastName(me.last_name);
    setLanguage(me.language || languages[0] || "en");
  }, [me, languages]);

  const profileMut = useMutation({
    mutationFn: (body: {
      first_name: string;
      last_name: string;
      language: string;
    }) => patch<CmsUser>("/users/me", body),
    onSuccess: (updated) => {
      queryClient.setQueryData(["users", "me"], updated);
      updateUser({
        first_name: updated.first_name,
        last_name: updated.last_name,
        language: updated.language,
        avatar: updated.avatar,
      });
      setProfileSaved(true);
      window.setTimeout(() => setProfileSaved(false), 2000);
    },
  });

  const passwordMut = useMutation({
    mutationFn: (pwd: string) => post("/users/me/password", { password: pwd }),
    onSuccess: () => {
      setPassword("");
      setConfirm("");
      setPwdError(null);
      setPwdSaved(true);
      window.setTimeout(() => setPwdSaved(false), 2000);
    },
  });

  function handleProfileSubmit(e: React.FormEvent) {
    e.preventDefault();
    profileMut.mutate({
      first_name: firstName.trim(),
      last_name: lastName.trim(),
      language,
    });
  }

  function handlePasswordSubmit(e: React.FormEvent) {
    e.preventDefault();
    setPwdError(null);
    if (password.length < 8) {
      setPwdError("Password must be at least 8 characters.");
      return;
    }
    if (password !== confirm) {
      setPwdError("Passwords do not match.");
      return;
    }
    passwordMut.mutate(password);
  }

  if (isLoading || !me) {
    return (
      <div className="space-y-5">
        <div>
          <h1 className="text-2xl font-bold text-slate-900 dark:text-slate-100">
            My Profile
          </h1>
          <p className="mt-1 text-sm text-slate-500 dark:text-slate-400">
            Loading…
          </p>
        </div>
      </div>
    );
  }

  const showPassword = me.provider === "local";

  return (
    <div className="mx-auto space-y-5">
      <div>
        <h1 className="text-2xl font-bold text-slate-900 dark:text-slate-100">
          My Profile
        </h1>
        <p className="mt-1 text-sm text-slate-500 dark:text-slate-400">
          Update your name, language, and password. No admin permission required.
        </p>
      </div>

      <Card>
        <div className="mb-5 flex items-center gap-3">
          <div className="flex h-12 w-12 items-center justify-center rounded-full bg-brand-600 text-sm font-bold text-white">
            {initials(`${me.first_name} ${me.last_name}`)}
          </div>
          <div className="min-w-0">
            <p className="truncate font-semibold text-slate-900 dark:text-slate-100">
              {me.first_name} {me.last_name}
            </p>
            <p className="truncate text-sm text-slate-400">{me.email}</p>
          </div>
        </div>

        <CardHeader>
          <CardTitle className="flex items-center gap-2 text-base">
            <User size={16} className="text-slate-400" />
            Profile
          </CardTitle>
        </CardHeader>

        <form onSubmit={handleProfileSubmit} className="space-y-4">
          <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
            <div>
              <label className="mb-1.5 block text-xs font-medium text-slate-700 dark:text-slate-200">
                First name
              </label>
              <input
                className="input"
                value={firstName}
                onChange={(e) => setFirstName(e.target.value)}
                required
                autoComplete="given-name"
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
                autoComplete="family-name"
              />
            </div>
          </div>

          <div>
            <label className="mb-1.5 block text-xs font-medium text-slate-700 dark:text-slate-200">
              Email
            </label>
            <input
              className="input opacity-70"
              value={me.email}
              disabled
              readOnly
            />
            <p className="mt-1 text-[11px] text-slate-400">
              Email cannot be changed here.
            </p>
          </div>

          <div>
            <label className="mb-1.5 block text-xs font-medium text-slate-700 dark:text-slate-200">
              Language
            </label>
            <LanguageSelector
              value={language}
              onChange={setLanguage}
              languages={
                languages.includes(language)
                  ? languages
                  : [language, ...languages]
              }
            />
          </div>

          {profileMut.isError && (
            <p className="rounded-xl bg-red-50 px-3 py-2 text-sm text-red-600 dark:bg-red-500/10 dark:text-red-400">
              {(profileMut.error as Error).message}
            </p>
          )}

          <div className="flex items-center justify-end gap-3 pt-1">
            {profileSaved && (
              <span className="flex items-center gap-1.5 text-xs font-medium text-emerald-600 dark:text-emerald-400">
                <CheckCircle2 size={14} />
                Saved
              </span>
            )}
            <Button type="submit" disabled={profileMut.isPending}>
              {profileMut.isPending ? "Saving…" : "Save profile"}
            </Button>
          </div>
        </form>
      </Card>

      {showPassword && (
        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2 text-base">
              <KeyRound size={16} className="text-slate-400" />
              Password
            </CardTitle>
          </CardHeader>

          <form onSubmit={handlePasswordSubmit} className="space-y-4">
            <div>
              <label className="mb-1.5 block text-xs font-medium text-slate-700 dark:text-slate-200">
                New password
              </label>
              <input
                type="password"
                className="input"
                value={password}
                onChange={(e) => {
                  setPassword(e.target.value);
                  setPwdError(null);
                }}
                autoComplete="new-password"
                minLength={8}
                required
              />
              <p className="mt-1 text-[11px] text-slate-400">
                Minimum 8 characters.
              </p>
            </div>
            <div>
              <label className="mb-1.5 block text-xs font-medium text-slate-700 dark:text-slate-200">
                Confirm new password
              </label>
              <input
                type="password"
                className="input"
                value={confirm}
                onChange={(e) => {
                  setConfirm(e.target.value);
                  setPwdError(null);
                }}
                autoComplete="new-password"
                minLength={8}
                required
              />
            </div>

            {(pwdError || passwordMut.isError) && (
              <p className="rounded-xl bg-red-50 px-3 py-2 text-sm text-red-600 dark:bg-red-500/10 dark:text-red-400">
                {pwdError ?? (passwordMut.error as Error).message}
              </p>
            )}

            <div className="flex items-center justify-end gap-3 pt-1">
              {pwdSaved && (
                <span className="flex items-center gap-1.5 text-xs font-medium text-emerald-600 dark:text-emerald-400">
                  <CheckCircle2 size={14} />
                  Password updated
                </span>
              )}
              <Button type="submit" disabled={passwordMut.isPending}>
                {passwordMut.isPending ? "Updating…" : "Update password"}
              </Button>
            </div>
          </form>
        </Card>
      )}

      {!showPassword && (
        <Card>
          <p className="text-sm text-slate-500 dark:text-slate-400">
            Your account uses SSO. Password is managed by your identity provider.
          </p>
        </Card>
      )}
    </div>
  );
}
