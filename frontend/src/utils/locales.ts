const VISIBLE_LOCALES_KEY = "dr600ab.visible-locales";

export function compactLocaleName(locale: string) {
  const labels: Record<string, string> = {
    "zh-CN": "中文",
    "en-US": "English",
  };
  return labels[locale] ?? locale;
}

export function fullLocaleName(locale: string) {
  const labels: Record<string, string> = {
    "zh-CN": "中文",
    "en-US": "English",
  };
  return labels[locale] ?? locale;
}

export function normalizeVisibleLocales(options: string[], visible: string[], current: string) {
  const optionSet = new Set(options);
  const next = visible.filter((item) => optionSet.has(item));

  if (optionSet.has(current) && !next.includes(current)) {
    next.unshift(current);
  }

  if (next.length > 0) {
    return Array.from(new Set(next));
  }

  return options;
}

export function getStoredVisibleLocales() {
  if (typeof window === "undefined") {
    return [];
  }

  try {
    const raw = window.localStorage.getItem(VISIBLE_LOCALES_KEY);
    if (!raw) {
      return [];
    }
    const parsed = JSON.parse(raw) as unknown;
    return Array.isArray(parsed) ? parsed.filter((item): item is string => typeof item === "string") : [];
  } catch {
    return [];
  }
}

export function persistVisibleLocales(locales: string[]) {
  if (typeof window === "undefined") {
    return;
  }

  try {
    window.localStorage.setItem(VISIBLE_LOCALES_KEY, JSON.stringify(Array.from(new Set(locales))));
  } catch {
    // 忽略本地存储错误。
  }
}
