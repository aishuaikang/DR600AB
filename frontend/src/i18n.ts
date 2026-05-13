import i18n from "i18next";
import { initReactI18next } from "react-i18next";

type NamespaceResources = Record<string, Record<string, string>>;
type ResourceMap = Record<string, NamespaceResources>;

const localeModules = import.meta.glob("./locales/*/*.json", {
  eager: true,
  import: "default",
}) as Record<string, Record<string, string>>;

function buildResources(): ResourceMap {
  const resources: ResourceMap = {};

  for (const [path, mod] of Object.entries(localeModules)) {
    const match = path.match(/\.\/locales\/([^/]+)\/([^/]+)\.json$/);
    if (!match) {
      continue;
    }
    const [, locale, namespace] = match;
    if (!resources[locale]) {
      resources[locale] = {};
    }
    resources[locale][namespace] = mod;
  }

  return resources;
}

export const resources = buildResources();
export const supportedLocales = Object.keys(resources).sort();
export const namespaces = Array.from(
  new Set(Object.values(resources).flatMap((localeNamespaces) => Object.keys(localeNamespaces))),
).sort();

const STORAGE_KEY = "dr600ab.locale";

function pickInitialLocale() {
  if (typeof window === "undefined") {
    return "zh-CN";
  }

  try {
    const stored = window.localStorage.getItem(STORAGE_KEY);
    if (stored && supportedLocales.includes(stored)) {
      return stored;
    }
  } catch {
    // ignore storage errors
  }

  const browserLocale = window.navigator.language.replace("_", "-");
  if (supportedLocales.includes(browserLocale)) {
    return browserLocale;
  }
  if (browserLocale.startsWith("en") && supportedLocales.includes("en-US")) {
    return "en-US";
  }
  if (browserLocale.startsWith("zh") && supportedLocales.includes("zh-CN")) {
    return "zh-CN";
  }
  return supportedLocales[0] ?? "zh-CN";
}

i18n.use(initReactI18next).init({
  resources,
  lng: pickInitialLocale(),
  fallbackLng: "zh-CN",
  ns: namespaces,
  defaultNS: "common",
  nsSeparator: false,
  keySeparator: false,
  interpolation: {
    escapeValue: false,
  },
  react: {
    useSuspense: false,
  },
});

export function getStoredLocale() {
  return pickInitialLocale();
}

export function persistLocale(locale: string) {
  if (typeof window === "undefined") {
    return;
  }
  try {
    window.localStorage.setItem(STORAGE_KEY, locale);
  } catch {
    // ignore storage errors
  }
}

export default i18n;
