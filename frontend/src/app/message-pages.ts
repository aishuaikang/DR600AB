import {
  FileText,
  Radar,
} from "lucide-react";

import type { MessagePageConfig } from "./types";
import { formatNumber } from "../utils/format";
import {
  getRecordData,
  getRecordField,
  getTextValue,
  isDetectionRecord,
} from "../utils/records";
import type { DebugRecord, DebugRecordPage } from "../types";

export const MESSAGE_PAGE_ORDER: DebugRecordPage[] = [
  "detection-records",
  "parsed-records",
];

export const MESSAGE_PAGE_CONFIG: Record<DebugRecordPage, MessagePageConfig> = {
  "detection-records": {
    icon: Radar,
    navLabelKey: "detectionRecords",
    titleKey: "detectionRecords.title",
    tone: "info",
    tableWidth: "min-w-[104rem]",
    columns: [
      {
        labelKey: "detectionRecords.type",
        width: "w-[10rem]",
        render: (record) => getTextValue(isDetectionRecord(record) ? record.kind : record.type),
      },
      {
        labelKey: "detectionRecords.device",
        width: "w-[18rem]",
        render: (record) => getTextValue(isDetectionRecord(record) ? record.device : getRecordField(record, "device", "ssid")),
      },
      {
        labelKey: "detectionRecords.model",
        width: "w-[20rem]",
        render: (record) => getTextValue(isDetectionRecord(record) ? record.model : getRecordField(record, "model", "seq", "encrypted_id")),
      },
      {
        labelKey: "detectionRecords.frequency",
        width: "w-[10rem]",
        render: (record, locale) => formatNumber(locale, isDetectionRecord(record) ? record.frequency : undefined),
      },
      {
        labelKey: "detectionRecords.rssi",
        width: "w-[9rem]",
        render: (record, locale) => formatNumber(locale, isDetectionRecord(record) ? record.rssi : undefined),
      },
      {
        labelKey: "detectionRecords.summary",
        width: "w-[26rem]",
        render: (record) => getTextValue(isDetectionRecord(record) ? record.summary : record.raw),
      },
    ],
  },
  "parsed-records": {
    icon: FileText,
    navLabelKey: "parsedRecords",
    titleKey: "parsedRecords.title",
    tone: "neutral",
    tableWidth: "min-w-[92rem]",
    columns: [
      {
        labelKey: "parsedRecords.type",
        width: "w-[12rem]",
        render: (record) => getTextValue(isDetectionRecord(record) ? record.kind : record.type),
      },
      {
        labelKey: "parsedRecords.device",
        width: "w-[18rem]",
        render: (record) => getTextValue(getRecordField(record, "device", "ssid")),
      },
      {
        labelKey: "parsedRecords.model",
        width: "w-[18rem]",
        render: (record) => getTextValue(getRecordField(record, "model", "seq", "encrypted_id")),
      },
      {
        labelKey: "parsedRecords.frequency",
        width: "w-[10rem]",
        render: (record, locale) => formatNumber(locale, getRecordNumber(record, "freq")),
      },
      {
        labelKey: "parsedRecords.rssi",
        width: "w-[9rem]",
        render: (record, locale) => formatNumber(locale, getRecordNumber(record, "rssi")),
      },
    ],
  },
};

function getRecordNumber(record: DebugRecord, key: string) {
  const value = getRecordData(record)[key];
  if (typeof value === "number" && Number.isFinite(value)) {
    return value;
  }
  if (typeof value === "string" && value.trim()) {
    const next = Number(value);
    if (Number.isFinite(next)) {
      return next;
    }
  }
  return undefined;
}
