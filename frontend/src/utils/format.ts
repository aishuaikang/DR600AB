export function formatTime(locale: string, value?: string) {
  if (!value) {
    return "-";
  }
  return new Intl.DateTimeFormat(locale, {
    dateStyle: "medium",
    timeStyle: "medium",
  }).format(new Date(value));
}

export function formatNumber(locale: string, value?: number, digits = 1) {
  if (typeof value !== "number" || Number.isNaN(value)) {
    return "-";
  }
  return new Intl.NumberFormat(locale, {
    maximumFractionDigits: digits,
  }).format(value);
}

export function detectLocaleName(locale: string) {
  const labels: Record<string, string> = {
    "zh-CN": "中文 / zh-CN",
    "en-US": "English / en-US",
  };
  return labels[locale] ?? locale;
}
