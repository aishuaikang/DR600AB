import { Monitor, Network, Satellite, Settings2, Zap } from "lucide-react";

import { MESSAGE_PAGE_CONFIG } from "./message-pages";
import type { NavItem, Page } from "./types";

export const debugPageItems: NavItem[] = [
  { id: "detection-records", icon: MESSAGE_PAGE_CONFIG["detection-records"].icon, labelKey: MESSAGE_PAGE_CONFIG["detection-records"].navLabelKey },
  { id: "parsed-records", icon: MESSAGE_PAGE_CONFIG["parsed-records"].icon, labelKey: MESSAGE_PAGE_CONFIG["parsed-records"].navLabelKey },
  { id: "gps-records", icon: Satellite, labelKey: "gpsRecords" },
  { id: "interference", icon: Zap, labelKey: "interference" },
  { id: "network-settings", icon: Network, labelKey: "networkSettings" },
  { id: "developer-settings", icon: Settings2, labelKey: "developerSettings" },
];

export const pageItems: NavItem[] = [
  { id: "screen", icon: Monitor, labelKey: "screen" },
  { id: "settings", icon: Settings2, labelKey: "settings" },
  ...debugPageItems,
];

export function isDebugPage(page: Page) {
  return debugPageItems.some((item) => item.id === page);
}

export function normalizePage(hash: string): Page {
  const next = hash.replace(/^#\/?/, "");
  return pageItems.some((item) => item.id === next) ? (next as Page) : "screen";
}
