import { useState, useEffect } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { useTranslation, Trans } from "react-i18next";
import {
  BookOpen,
  CheckCircle2,
  Globe,
  Package,
  Puzzle,
  Search,
} from "lucide-react";
import { Card, CardHeader, CardTitle } from "@/components/ui/Card";
import { Button } from "@/components/ui/Button";
import { get, patch, post } from "@/lib/api";
import { useTenantStore } from "@/store/tenant.store";
import type {
  Plugin,
  PluginInstallResponse,
  SettingsResponse,
  Tenant,
  TenantSettings,
  TenantStorageSettings,
} from "@/types";

// ── Language catalogue ────────────────────────────────────────────────────────
const ALL_LANGUAGES = [
  { code: "en",    name: "English" },
  { code: "es",    name: "Spanish" },
  { code: "fr",    name: "French" },
  { code: "de",    name: "German" },
  { code: "pt",    name: "Portuguese" },
  { code: "it",    name: "Italian" },
  { code: "nl",    name: "Dutch" },
  { code: "pl",    name: "Polish" },
  { code: "ru",    name: "Russian" },
  { code: "uk",    name: "Ukrainian" },
  { code: "cs",    name: "Czech" },
  { code: "ro",    name: "Romanian" },
  { code: "hu",    name: "Hungarian" },
  { code: "sv",    name: "Swedish" },
  { code: "da",    name: "Danish" },
  { code: "fi",    name: "Finnish" },
  { code: "nb",    name: "Norwegian" },
  { code: "el",    name: "Greek" },
  { code: "tr",    name: "Turkish" },
  { code: "ar",    name: "Arabic" },
  { code: "he",    name: "Hebrew" },
  { code: "fa",    name: "Persian" },
  { code: "hi",    name: "Hindi" },
  { code: "bn",    name: "Bengali" },
  { code: "zh",    name: "Chinese (Simplified)" },
  { code: "zh-TW", name: "Chinese (Traditional)" },
  { code: "ja",    name: "Japanese" },
  { code: "ko",    name: "Korean" },
  { code: "th",    name: "Thai" },
  { code: "vi",    name: "Vietnamese" },
  { code: "id",    name: "Indonesian" },
  { code: "ms",    name: "Malay" },
] as const;

export type LanguageCode = typeof ALL_LANGUAGES[number]["code"];

/** Localized display name for a language code (falls back to the English name). */
function languageLabel(code: string, fallback: string, locale: string): string {
  try {
    const label = new Intl.DisplayNames([locale], { type: "language" }).of(code);
    if (label && label !== code) return label.charAt(0).toLocaleUpperCase(locale) + label.slice(1);
  } catch {
    /* unsupported locale / code — use the fallback */
  }
  return fallback;
}

// ── Language selector card ────────────────────────────────────────────────────
function LanguagesCard({
  currentLanguages,
  defaultLanguage,
  onSave,
}: {
  currentLanguages: string[];
  defaultLanguage: string;
  onSave: (langs: string[]) => Promise<unknown>;
}) {
  const { t, i18n } = useTranslation();
  const [selected, setSelected] = useState<Set<string>>(new Set(currentLanguages));
  const [search, setSearch] = useState("");
  const [saving, setSaving] = useState(false);
  const [saved, setSaved] = useState(false);

  useEffect(() => {
    setSelected(new Set(currentLanguages));
  }, [currentLanguages.join(",")]);

  const visible = ALL_LANGUAGES.filter(
    (l) =>
      l.name.toLowerCase().includes(search.toLowerCase()) ||
      languageLabel(l.code, l.name, i18n.language).toLowerCase().includes(search.toLowerCase()) ||
      l.code.toLowerCase().includes(search.toLowerCase())
  );

  function toggle(code: string) {
    if (code === defaultLanguage) return; // default language cannot be removed
    setSelected((prev) => {
      const next = new Set(prev);
      if (next.has(code)) {
        if (next.size === 1) return prev; // always keep at least one
        next.delete(code);
      } else {
        next.add(code);
      }
      return next;
    });
  }

  async function handleSave() {
    setSaving(true);
    try {
      await onSave([...selected]);
      setSaved(true);
      setTimeout(() => setSaved(false), 2500);
    } finally {
      setSaving(false);
    }
  }

  const hasChanges =
    selected.size !== currentLanguages.length ||
    [...selected].some((c) => !currentLanguages.includes(c));

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t("settings.languagesTitle")}</CardTitle>
      </CardHeader>
      <div className="space-y-4">
        <p className="text-xs text-slate-500 dark:text-slate-400">
          {t("settings.languagesDesc")}
        </p>

        {/* Search */}
        <div className="relative">
          <Search size={13} className="absolute left-3 top-1/2 -translate-y-1/2 text-slate-400" />
          <input
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder={t("settings.searchLanguages")}
            className="input pl-8 text-xs"
          />
        </div>

        {/* Grid */}
        <div className="grid grid-cols-2 gap-1.5 sm:grid-cols-3">
          {visible.map((lang) => {
            const active = selected.has(lang.code);
            const isDefault = lang.code === defaultLanguage;
            return (
              <label
                key={lang.code}
                title={isDefault ? t("settings.defaultLangTitle") : undefined}
                className={`flex items-center gap-2.5 rounded-lg border px-3 py-2 transition ${
                  isDefault ? "cursor-default" : "cursor-pointer"
                } ${
                  active
                    ? "border-brand-300 bg-brand-50 dark:bg-brand-500/15"
                    : "border-slate-100 dark:border-white/10 hover:border-slate-200 dark:hover:border-white/10 hover:bg-slate-50 dark:hover:bg-white/5"
                }`}
              >
                <input
                  type="checkbox"
                  checked={active}
                  onChange={() => toggle(lang.code)}
                  disabled={isDefault}
                  className="h-3.5 w-3.5 rounded border-slate-300 text-brand-600 focus:ring-brand-400 disabled:cursor-not-allowed"
                />
                <span className={`font-mono text-[11px] font-semibold ${active ? "text-brand-700 dark:text-brand-300" : "text-slate-400"}`}>
                  {lang.code}
                </span>
                <span className={`truncate text-xs ${active ? "text-brand-800 dark:text-brand-200" : "text-slate-600 dark:text-slate-300"}`}>
                  {languageLabel(lang.code, lang.name, i18n.language)}
                </span>
              </label>
            );
          })}
        </div>

        <div className="flex items-center gap-3 pt-1">
          <Button onClick={handleSave} disabled={!hasChanges || saving}>
            {saving ? t("common.saving") : t("settings.saveLanguages")}
          </Button>
          {saved && (
            <span className="flex items-center gap-1.5 text-sm text-emerald-600 dark:text-emerald-300">
              <CheckCircle2 size={14} /> {t("common.saved")}
            </span>
          )}
          <span className="text-xs text-slate-400">
            {t("settings.languagesSelected", { count: selected.size })}
          </span>
        </div>
      </div>
    </Card>
  );
}

// ── Plugins ───────────────────────────────────────────────────────────────────
const PLUGIN_ICONS: Record<string, React.ElementType> = {
  website: Globe,
  blog: BookOpen,
};

function PluginsCard() {
  const { t } = useTranslation();
  const queryClient = useQueryClient();
  const { tenant, setTenant } = useTenantStore();
  const [lastResult, setLastResult] = useState<PluginInstallResponse["result"] | null>(
    null,
  );

  const { data, isLoading } = useQuery({
    queryKey: ["plugins"],
    queryFn: () => get<{ data: Plugin[] }>("/plugins"),
  });
  const plugins = data?.data ?? [];

  const installMut = useMutation({
    mutationFn: (id: string) =>
      post<PluginInstallResponse>(`/plugins/${id}/install`),
    onSuccess: (res) => {
      setLastResult(res.result);
      queryClient.setQueryData(["plugins"], { data: res.plugins });
      queryClient.invalidateQueries({ queryKey: ["plugins"] });
      queryClient.invalidateQueries({ queryKey: ["settings"] });
      queryClient.invalidateQueries({ queryKey: ["collections"] });
      if (tenant && res.settings) {
        setTenant({ ...tenant, settings: res.settings });
      }
    },
  });

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t("settings.pluginsTitle")}</CardTitle>
      </CardHeader>
      <div className="space-y-3">
        <p className="text-xs text-slate-500 dark:text-slate-400">
          <Trans
            i18nKey="settings.pluginsDesc"
            components={{ mono: <span className="font-mono" /> }}
          />
        </p>

        {isLoading && (
          <p className="py-6 text-center text-sm text-slate-400">{t("settings.loadingPlugins")}</p>
        )}

        {!isLoading && plugins.length === 0 && (
          <p className="rounded-xl border border-dashed border-slate-200 px-4 py-8 text-center text-sm text-slate-400 dark:border-white/10">
            {t("settings.noPlugins")}
          </p>
        )}

        {plugins.map((plugin) => {
          const Icon = PLUGIN_ICONS[plugin.id] ?? Puzzle;
          const installing =
            installMut.isPending && installMut.variables === plugin.id;
          return (
            <div
              key={plugin.id}
              className="flex items-start gap-4 rounded-xl border border-slate-100 p-4 transition hover:border-slate-200 dark:border-white/10 dark:hover:border-white/15"
            >
              <div
                className={`mt-0.5 flex h-9 w-9 shrink-0 items-center justify-center rounded-lg transition ${
                  plugin.installed
                    ? "bg-brand-50 text-brand-600 dark:bg-brand-500/15 dark:text-brand-300"
                    : "bg-slate-100 text-slate-400 dark:bg-white/10"
                }`}
              >
                <Icon size={18} />
              </div>

              <div className="min-w-0 flex-1">
                <div className="flex flex-wrap items-center gap-2">
                  <p className="text-sm font-semibold text-slate-800 dark:text-slate-100">
                    {plugin.name}
                  </p>
                  {plugin.installed ? (
                    <span className="badge badge-blue">{t("settings.installed")}</span>
                  ) : (
                    <span className="badge badge-gray">{t("settings.notInstalled")}</span>
                  )}
                </div>
                <p className="mt-0.5 text-xs text-slate-400">{plugin.description}</p>
                {plugin.collections.length > 0 && (
                  <div className="mt-2 flex flex-wrap gap-1.5">
                    {plugin.collections.map((name) => (
                      <span
                        key={name}
                        className="inline-flex items-center gap-1 rounded-md bg-slate-50 px-1.5 py-0.5 font-mono text-[10px] text-slate-500 dark:bg-white/5 dark:text-slate-400"
                      >
                        <Package size={10} />
                        {name}
                      </span>
                    ))}
                  </div>
                )}
              </div>

              <Button
                type="button"
                size="sm"
                variant={plugin.installed ? "secondary" : "primary"}
                disabled={installMut.isPending}
                onClick={() => installMut.mutate(plugin.id)}
                className="shrink-0"
              >
                {installing
                  ? t("settings.installing")
                  : plugin.installed
                    ? t("settings.reinstall")
                    : t("settings.install")}
              </Button>
            </div>
          );
        })}

        {installMut.isError && (
          <p className="rounded-xl bg-red-50 px-3 py-2 text-sm text-red-600 dark:bg-red-500/10 dark:text-red-400">
            {(installMut.error as Error).message}
          </p>
        )}

        {lastResult && (
          <div className="rounded-xl border border-emerald-200/80 bg-emerald-50/80 px-3 py-2.5 text-xs text-emerald-900 dark:border-emerald-500/25 dark:bg-emerald-500/10 dark:text-emerald-100">
            <p className="flex items-center gap-1.5 font-semibold">
              <CheckCircle2 size={14} />
              {t("settings.installedResult", { plugin: lastResult.plugin })}
            </p>
            <ul className="mt-1.5 space-y-0.5 text-emerald-800/90 dark:text-emerald-100/80">
              <li>
                {t("settings.created")}{" "}
                {lastResult.collections_created.length
                  ? lastResult.collections_created.join(", ")
                  : t("common.none").toLowerCase()}
              </li>
              {lastResult.collections_omitted.length > 0 && (
                <li>
                  {t("settings.skipped")}{" "}
                  {lastResult.collections_omitted
                    .map((o) => `${o.name} (${o.reason})`)
                    .join("; ")}
                </li>
              )}
              {lastResult.relations_created.length > 0 && (
                <li>
                  {t("settings.relationsCreated")} {lastResult.relations_created.join(", ")}
                </li>
              )}
              {lastResult.relations_omitted.length > 0 && (
                <li>
                  {t("settings.relationsSkipped")}{" "}
                  {lastResult.relations_omitted
                    .map((o) => `${o.name} (${o.reason})`)
                    .join("; ")}
                </li>
              )}
            </ul>
          </div>
        )}
      </div>
    </Card>
  );
}

// ── Settings page ─────────────────────────────────────────────────────────────
export default function SettingsPage() {
  const { t, i18n } = useTranslation();
  const { tenant, setTenant } = useTenantStore();
  const queryClient = useQueryClient();

  const { data: settingsData } = useQuery({
    queryKey: ["settings"],
    queryFn: () =>
      get<{ tenant: Tenant; settings: TenantSettings }>("/settings"),
  });

  const serverLanguages: string[] = settingsData?.settings.languages ?? ["en"];
  const serverTenant = settingsData?.tenant;

  // General section state
  const [generalName, setGeneralName] = useState(tenant?.name ?? "");
  const [generalLang, setGeneralLang] = useState(tenant?.default_language ?? "en");
  const [generalSaved, setGeneralSaved] = useState(false);

  // Sync local state once the query resolves
  useEffect(() => {
    if (settingsData) {
      setGeneralName(settingsData.tenant.name);
      setGeneralLang(settingsData.tenant.default_language);
    }
  }, [settingsData]);

  async function saveLanguages(langs: string[]) {
    // Always keep the default language enabled
    const defaultLang = serverTenant?.default_language ?? generalLang;
    const finalLangs = langs.includes(defaultLang) ? langs : [...langs, defaultLang];
    const updated = await patch<Tenant>("/settings", {
      settings: { ...settingsData?.settings, languages: finalLangs },
    });
    setTenant(updated);
    queryClient.invalidateQueries({ queryKey: ["settings"] });
  }

  const saveGeneralMutation = useMutation({
    mutationFn: () =>
      patch<Tenant>("/settings", {
        name: generalName,
        default_language: generalLang,
      }),
    onSuccess: async (updatedTenant) => {
      setTenant(updatedTenant);
      // If the new default language is not yet in the languages list, add it
      const currentLangs = settingsData?.settings.languages ?? ["en"];
      if (!currentLangs.includes(updatedTenant.default_language)) {
        await saveLanguages([...currentLangs, updatedTenant.default_language]);
      } else {
        queryClient.invalidateQueries({ queryKey: ["settings"] });
      }
      setGeneralSaved(true);
      setTimeout(() => setGeneralSaved(false), 2500);
    },
  });

  const generalChanged =
    generalName !== (serverTenant?.name ?? tenant?.name ?? "") ||
    generalLang !== (serverTenant?.default_language ?? tenant?.default_language ?? "en");

  return (
    <div className="space-y-5 max-w-2xl">
      <div>
        <h1 className="text-2xl font-bold text-slate-900 dark:text-slate-100">{t("settings.title")}</h1>
        <p className="mt-1 text-sm text-slate-500 dark:text-slate-400">{t("settings.subtitle")}</p>
      </div>

      {/* General */}
      <Card>
        <CardHeader>
          <CardTitle>{t("settings.general")}</CardTitle>
        </CardHeader>
        <div className="space-y-4">
          <div>
            <label className="mb-1.5 block text-sm font-medium text-slate-700 dark:text-slate-200">
              {t("settings.tenantName")}
            </label>
            <input
              value={generalName}
              onChange={(e) => setGeneralName(e.target.value)}
              className="input"
            />
          </div>
          <div>
            <label className="mb-1.5 block text-sm font-medium text-slate-700 dark:text-slate-200">
              {t("settings.defaultLanguage")}
            </label>
            <select
              value={generalLang}
              onChange={(e) => setGeneralLang(e.target.value)}
              className="input"
            >
              {ALL_LANGUAGES.map((l) => (
                <option key={l.code} value={l.code}>
                  {l.code.toUpperCase()} — {languageLabel(l.code, l.name, i18n.language)}
                </option>
              ))}
            </select>
            <p className="mt-1 text-xs text-slate-400">
              {t("settings.defaultLanguageHint")}
            </p>
          </div>
          <div className="flex items-center gap-3">
            <Button
              onClick={() => saveGeneralMutation.mutate()}
              disabled={!generalChanged || saveGeneralMutation.isPending}
            >
              {saveGeneralMutation.isPending ? t("common.saving") : t("settings.saveChanges")}
            </Button>
            {generalSaved && (
              <span className="flex items-center gap-1.5 text-sm text-emerald-600 dark:text-emerald-300">
                <CheckCircle2 size={14} /> {t("common.saved")}
              </span>
            )}
            {saveGeneralMutation.isError && (
              <span className="text-sm text-red-500 dark:text-red-400">{t("settings.saveFailed")}</span>
            )}
          </div>
        </div>
      </Card>

      <PluginsCard />

      {/* Languages */}
      <LanguagesCard
        currentLanguages={serverLanguages}
        defaultLanguage={serverTenant?.default_language ?? generalLang}
        onSave={saveLanguages}
      />

      {/* S3 storage */}
      <StorageSettingsCard />

      {/* SSO */}
      <Card>
        <CardHeader>
          <CardTitle>{t("settings.ssoTitle")}</CardTitle>
        </CardHeader>
        <div className="space-y-4">
          <div>
            <label className="mb-1.5 block text-xs font-medium text-slate-600 dark:text-slate-300">{t("settings.logtoEndpoint")}</label>
            <input
              placeholder="https://your-tenant.logto.app"
              className="input text-xs"
            />
          </div>
          <div className="grid grid-cols-2 gap-3">
            <div>
              <label className="mb-1.5 block text-xs font-medium text-slate-600 dark:text-slate-300">{t("settings.appId")}</label>
              <input className="input text-xs" />
            </div>
            <div>
              <label className="mb-1.5 block text-xs font-medium text-slate-600 dark:text-slate-300">{t("settings.appSecret")}</label>
              <input type="password" className="input text-xs" />
            </div>
          </div>
          <Button>{t("settings.saveSso")}</Button>
        </div>
      </Card>
    </div>
  );
}

// ── Storage (S3) settings card ──────────────────────────────────────────────────

const emptyStorage: TenantStorageSettings = {
  inherit: true,
  endpoint: "",
  region: "us-east-1",
  bucket: "",
  access_key_id: "",
  secret_key: "",
  public_base_url: "",
  cdn_base_url: "",
  force_path_style: false,
};

function StorageSettingsCard() {
  const { t } = useTranslation();
  const queryClient = useQueryClient();
  const { data } = useQuery({
    queryKey: ["settings"],
    queryFn: () => get<SettingsResponse>("/settings"),
  });

  const [form, setForm] = useState<TenantStorageSettings>(emptyStorage);
  const [saved, setSaved] = useState(false);
  const [seeded, setSeeded] = useState(false);

  useEffect(() => {
    if (data && !seeded) {
      setForm({ ...emptyStorage, ...(data.settings.storage ?? {}), secret_key: "" });
      setSeeded(true);
    }
  }, [data, seeded]);

  const env = data?.storage_env;
  const secretSet = data?.storage_secret_set ?? false;
  const set = (patchForm: Partial<TenantStorageSettings>) => setForm((f) => ({ ...f, ...patchForm }));

  const saveMut = useMutation({
    mutationFn: () =>
      patch<Tenant>("/settings", {
        settings: { ...data?.settings, storage: form },
      }),
    onSuccess: () => {
      setSaved(true);
      setTimeout(() => setSaved(false), 2000);
      setSeeded(false);
      queryClient.invalidateQueries({ queryKey: ["settings"] });
    },
  });

  const inputCls = "input text-xs";
  const labelCls = "mb-1.5 block text-xs font-medium text-slate-600 dark:text-slate-300";

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t("settings.storageTitle")}</CardTitle>
      </CardHeader>
      <div className="space-y-4">
        {/* Inherit default (env) credentials */}
        <label
          className={`flex items-start gap-3 rounded-xl border px-4 py-3 ${
            env?.configured
              ? "cursor-pointer border-slate-100 bg-slate-50 dark:border-white/10 dark:bg-white/5"
              : "border-slate-100 bg-slate-50 opacity-60 dark:border-white/10 dark:bg-white/5"
          }`}
        >
          <input
            type="checkbox"
            checked={form.inherit}
            disabled={!env?.configured}
            onChange={(e) => set({ inherit: e.target.checked })}
            className="mt-0.5 h-4 w-4 rounded border-slate-300 text-brand-600 focus:ring-brand-400 dark:border-white/10"
          />
          <div className="min-w-0">
            <p className="text-sm font-medium text-slate-800 dark:text-slate-100">
              {t("settings.inheritTitle")}
            </p>
            {env?.configured ? (
              <p className="text-xs text-slate-400">
                <Trans
                  i18nKey="settings.inheritDesc"
                  values={{ bucket: env.bucket }}
                  components={{ mono: <span className="font-mono" /> }}
                />
              </p>
            ) : (
              <p className="text-xs text-slate-400">
                {t("settings.noDefaultS3")}
              </p>
            )}
          </div>
        </label>

        {/* Own credentials — hidden while inheriting */}
        {!form.inherit && (
          <div className="grid grid-cols-2 gap-3">
            <div>
              <label className={labelCls}>{t("settings.endpoint")}</label>
              <input className={inputCls} placeholder="https://s3.wasabisys.com"
                value={form.endpoint} onChange={(e) => set({ endpoint: e.target.value })} />
            </div>
            <div>
              <label className={labelCls}>{t("settings.region")}</label>
              <input className={inputCls} placeholder="us-east-1"
                value={form.region} onChange={(e) => set({ region: e.target.value })} />
            </div>
            <div>
              <label className={labelCls}>{t("settings.bucket")}</label>
              <input className={inputCls} placeholder="my-tenant-bucket"
                value={form.bucket} onChange={(e) => set({ bucket: e.target.value })} />
            </div>
            <div>
              <label className={labelCls}>{t("settings.accessKeyId")}</label>
              <input className={inputCls} placeholder="AKIA…"
                value={form.access_key_id} onChange={(e) => set({ access_key_id: e.target.value })} />
            </div>
            <div className="col-span-2">
              <label className={labelCls}>{t("settings.secretAccessKey")}</label>
              <input className={inputCls} type="password"
                placeholder={secretSet ? t("settings.secretUnchanged") : ""}
                value={form.secret_key ?? ""} onChange={(e) => set({ secret_key: e.target.value })} />
            </div>
            <div>
              <label className={labelCls}>{t("settings.publicBaseUrl")}</label>
              <input className={inputCls} placeholder="https://cdn.example.com"
                value={form.public_base_url} onChange={(e) => set({ public_base_url: e.target.value })} />
            </div>
            <div>
              <label className={labelCls}>{t("settings.cdnBaseUrl")}</label>
              <input className={inputCls} placeholder="https://images.example.com"
                value={form.cdn_base_url} onChange={(e) => set({ cdn_base_url: e.target.value })} />
            </div>
            <label className="col-span-2 flex items-center gap-2 text-xs text-slate-600 dark:text-slate-300">
              <input type="checkbox" checked={form.force_path_style}
                onChange={(e) => set({ force_path_style: e.target.checked })}
                className="h-4 w-4 rounded border-slate-300 text-brand-600 focus:ring-brand-400 dark:border-white/10" />
              {t("settings.forcePathStyle")}
            </label>
          </div>
        )}

        <div className="flex items-center gap-3">
          <Button onClick={() => saveMut.mutate()} loading={saveMut.isPending}>{t("settings.saveStorage")}</Button>
          {saved && <span className="text-xs text-emerald-600 dark:text-emerald-300">{t("common.saved")}</span>}
        </div>
      </div>
    </Card>
  );
}
