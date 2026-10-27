/**
 * i18n engine for the admin panel.
 *
 * Catalogs live in `src/locales/<lng>/<area>.json`, each keyed by its area
 * (e.g. `{ "content": { ... } }`). They are auto-discovered with Vite's
 * `import.meta.glob`, so adding a new area file needs no change here. All areas
 * are merged into a single default namespace ("translation"), so components use
 * nested keys: `t("content.save")`, `t("common.cancel")`.
 *
 * The active language is the user's INTERFACE language (`ui_language`), distinct
 * from their content language. Resolution order: explicit set → persisted
 * ui_language from the auth session → localStorage → browser → English.
 */
import i18n from "i18next";
import { initReactI18next } from "react-i18next";

export const SUPPORTED_LANGUAGES = ["en", "es"] as const;
export type UILanguage = (typeof SUPPORTED_LANGUAGES)[number];

const UI_LANG_KEY = "cms-ui-lang";

const enModules = import.meta.glob("../locales/en/*.json", { eager: true });
const esModules = import.meta.glob("../locales/es/*.json", { eager: true });

function assemble(modules: Record<string, unknown>): Record<string, unknown> {
  const out: Record<string, unknown> = {};
  for (const mod of Object.values(modules)) {
    const data = (mod as { default?: unknown }).default ?? mod;
    Object.assign(out, data as Record<string, unknown>);
  }
  return out;
}

/** Narrow any string to a supported language, or undefined. */
export function normalizeLang(value: string | null | undefined): UILanguage | undefined {
  if (!value) return undefined;
  const base = value.slice(0, 2).toLowerCase();
  return (SUPPORTED_LANGUAGES as readonly string[]).includes(base)
    ? (base as UILanguage)
    : undefined;
}

/** Read a persisted interface-language preference (best-effort). */
function storedLang(): UILanguage | undefined {
  try {
    return normalizeLang(localStorage.getItem(UI_LANG_KEY));
  } catch {
    return undefined;
  }
}

/** Pull the ui_language off the persisted auth session, if any. */
function sessionLang(): UILanguage | undefined {
  try {
    const raw = localStorage.getItem("cms-auth");
    if (!raw) return undefined;
    const parsed = JSON.parse(raw);
    return normalizeLang(parsed?.state?.user?.ui_language);
  } catch {
    return undefined;
  }
}

function resolveInitialLang(): UILanguage {
  return (
    sessionLang() ??
    storedLang() ??
    normalizeLang(typeof navigator !== "undefined" ? navigator.language : undefined) ??
    "en"
  );
}

void i18n.use(initReactI18next).init({
  resources: {
    en: { translation: assemble(enModules) },
    es: { translation: assemble(esModules) },
  },
  lng: resolveInitialLang(),
  fallbackLng: "en",
  defaultNS: "translation",
  interpolation: { escapeValue: false },
  returnNull: false,
});

/** Change the active interface language and persist the choice locally. */
export function setUILanguage(lng: UILanguage) {
  void i18n.changeLanguage(lng);
  try {
    localStorage.setItem(UI_LANG_KEY, lng);
  } catch {
    /* ignore */
  }
}

/** Apply a language only if it differs (used to sync from the signed-in user). */
export function syncUILanguage(lng: string | null | undefined) {
  const norm = normalizeLang(lng);
  if (norm && norm !== i18n.language) setUILanguage(norm);
}

export default i18n;
