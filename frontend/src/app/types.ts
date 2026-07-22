import type { ReactNode } from "react";
import type { LucideIcon } from "lucide-react";

import type { DebugRecord, DebugRecordPage } from "../types";

export type Page =
  | DebugRecordPage
  | "deception-reports"
  | "developer-settings"
  | "fpv-records"
  | "gps-records"
  | "cellular-status"
  | "intrusions"
  | "interference-reports"
  | "interference"
  | "network-settings"
  | "settings"
  | "whitelist"
  | "screen";

export type Tone = "neutral" | "success" | "warning" | "error" | "info";

export type NavItem = {
  id: Page;
  icon: LucideIcon;
  labelKey: string;
};

export type MessageColumn = {
  labelKey: string;
  width: string;
  render: (record: DebugRecord, locale: string) => ReactNode;
};

export type DetailContent = {
  title: string;
  value: string;
};

export type MessagePageConfig = {
  icon: LucideIcon;
  navLabelKey: string;
  titleKey: string;
  tone: Tone;
  tableWidth: string;
  columns: MessageColumn[];
};

export type Banner = {
  kind: "idle" | "loading" | "success" | "error";
  message: string;
};
