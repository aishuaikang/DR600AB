import { Settings2, Zap } from "lucide-react";

import { MESSAGE_PAGE_CONFIG } from "./message-pages";
import type { NavItem, Page } from "./types";

export const debugPageItems: NavItem[] = [
  { id: "heartbeat", icon: MESSAGE_PAGE_CONFIG.heartbeat.icon, labelKey: MESSAGE_PAGE_CONFIG.heartbeat.navLabelKey },
  { id: "detect", icon: MESSAGE_PAGE_CONFIG.detect.icon, labelKey: MESSAGE_PAGE_CONFIG.detect.navLabelKey },
  { id: "did_encrypted", icon: MESSAGE_PAGE_CONFIG.did_encrypted.icon, labelKey: MESSAGE_PAGE_CONFIG.did_encrypted.navLabelKey },
  { id: "did_plain", icon: MESSAGE_PAGE_CONFIG.did_plain.icon, labelKey: MESSAGE_PAGE_CONFIG.did_plain.navLabelKey },
  { id: "rid", icon: MESSAGE_PAGE_CONFIG.rid.icon, labelKey: MESSAGE_PAGE_CONFIG.rid.navLabelKey },
  { id: "interference", icon: Zap, labelKey: "interference" },
];

export const pageItems: NavItem[] = [
  ...debugPageItems,
  { id: "settings", icon: Settings2, labelKey: "settings" },
];

export function normalizePage(hash: string): Page {
  const next = hash.replace(/^#\/?/, "");
  return pageItems.some((item) => item.id === next) ? (next as Page) : "did_encrypted";
}
