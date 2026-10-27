/**
 * Self profile — GET/PATCH /users/me, POST /users/me/password.
 * No cms_users permission required. Password change requires confirmation.
 */
import { useEffect, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { CheckCircle2, KeyRound, Languages, User } from "lucide-react";
import { get, patch, post } from "@/lib/api";
import { useAuthStore } from "@/store/auth.store";
import { setUILanguage, SUPPORTED_LANGUAGES, type UILanguage } from "@/lib/i18n";
import { Card, CardHeader, CardTitle } from "@/components/ui/Card";
import { Button } from "@/components/ui/Button";
import {
  LanguageSelector,
  useAvailableLanguages,
} from "@/components/content/fields";
import { initials } from "@/lib/utils";
import type { User as CmsUser } from "@/types";

const UI_LANGUAGE_LABELS: Record<UILanguage, string> = {
  en: "English",
  es: "Español",
};

export default function ProfilePage() {
  const { t } = useTranslation();
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
  const [uiLanguage, setUiLanguage] = useState<UILanguage>("en");
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
    const ui = (me.ui_language || "").slice(0, 2).toLowerCase();
    setUiLanguage((SUPPORTED_LANGUAGES as readonly string[]).includes(ui) ? (ui as UILanguage) : "en");
  }, [me, languages]);

  const profileMut = useMutation({
    mutationFn: (body: {
      first_name: string;
      last_name: string;
      language: string;
      ui_language: string;
    }) => patch<CmsUser>("/users/me", body),
    onSuccess: (updated) => {
      queryClient.setQueryData(["users", "me"], updated);
      updateUser({
        first_name: updated.first_name,
        last_name: updated.last_name,
        language: updated.language,
        ui_language: updated.ui_language,
        avatar: updated.avatar,
      });
      // Apply the new interface language immediately.
      if (updated.ui_language) setUILanguage(uiLanguage);
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
      ui_language: uiLanguage,
    });
  }

  function handlePasswordSubmit(e: React.FormEvent) {
    e.preventDefault();
    setPwdError(null);
    if (password.length < 8) {
      setPwdError(t("profile.passwordTooShort"));
      return;
    }
    if (password !== confirm) {
      setPwdError(t("profile.passwordMismatch"));
      return;
    }
    passwordMut.mutate(password);
  }

  if (isLoading || !me) {
    return (
      <div className="space-y-5">
        <div>
          <h1 className="text-2xl font-bold text-slate-900 dark:text-slate-100">
            {t("profile.title")}
          </h1>
          <p className="mt-1 text-sm text-slate-500 dark:text-slate-400">
            {t("common.loading")}
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
          {t("profile.title")}
        </h1>
        <p className="mt-1 text-sm text-slate-500 dark:text-slate-400">
          {t("profile.subtitle")}
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
            {t("profile.sectionProfile")}
          </CardTitle>
        </CardHeader>

        <form onSubmit={handleProfileSubmit} className="space-y-4">
          <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
            <div>
              <label className="mb-1.5 block text-xs font-medium text-slate-700 dark:text-slate-200">
                {t("profile.firstName")}
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
                {t("profile.lastName")}
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
              {t("common.email")}
            </label>
            <input
              className="input opacity-70"
              value={me.email}
              disabled
              readOnly
            />
            <p className="mt-1 text-[11px] text-slate-400">
              {t("profile.emailLocked")}
            </p>
          </div>

          <div>
            <label className="mb-1.5 block text-xs font-medium text-slate-700 dark:text-slate-200">
              {t("profile.contentLanguage")}
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
            <p className="mt-1 text-[11px] text-slate-400">
              {t("profile.contentLanguageHint")}
            </p>
          </div>

          <div>
            <label className="mb-1.5 block text-xs font-medium text-slate-700 dark:text-slate-200">
              {t("profile.interfaceLanguage")}
            </label>
            <div className="flex items-center gap-2 rounded-lg border border-slate-200 dark:border-white/10 bg-white dark:bg-white/5 px-3 py-1.5">
              <Languages size={13} className="shrink-0 text-slate-400" />
              <select
                value={uiLanguage}
                onChange={(e) => setUiLanguage(e.target.value as UILanguage)}
                className="appearance-none bg-transparent text-sm font-medium text-slate-700 dark:text-slate-200 focus:outline-none"
              >
                {SUPPORTED_LANGUAGES.map((lng) => (
                  <option key={lng} value={lng}>
                    {UI_LANGUAGE_LABELS[lng]}
                  </option>
                ))}
              </select>
            </div>
            <p className="mt-1 text-[11px] text-slate-400">
              {t("profile.interfaceLanguageHint")}
            </p>
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
                {t("common.saved")}
              </span>
            )}
            <Button type="submit" disabled={profileMut.isPending}>
              {profileMut.isPending ? t("common.saving") : t("profile.saveProfile")}
            </Button>
          </div>
        </form>
      </Card>

      {showPassword && (
        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2 text-base">
              <KeyRound size={16} className="text-slate-400" />
              {t("profile.sectionPassword")}
            </CardTitle>
          </CardHeader>

          <form onSubmit={handlePasswordSubmit} className="space-y-4">
            <div>
              <label className="mb-1.5 block text-xs font-medium text-slate-700 dark:text-slate-200">
                {t("profile.newPassword")}
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
                {t("profile.passwordHint")}
              </p>
            </div>
            <div>
              <label className="mb-1.5 block text-xs font-medium text-slate-700 dark:text-slate-200">
                {t("profile.confirmPassword")}
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
                  {t("profile.passwordUpdated")}
                </span>
              )}
              <Button type="submit" disabled={passwordMut.isPending}>
                {passwordMut.isPending ? t("common.updating") : t("profile.updatePassword")}
              </Button>
            </div>
          </form>
        </Card>
      )}

      {!showPassword && (
        <Card>
          <p className="text-sm text-slate-500 dark:text-slate-400">
            {t("profile.ssoNote")}
          </p>
        </Card>
      )}
    </div>
  );
}
