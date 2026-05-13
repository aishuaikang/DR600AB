import {
  Activity,
  FileText,
  Fingerprint,
  Radio,
  Shield,
} from "lucide-react";

import type { MessagePageConfig } from "./types";
import { formatNumber } from "../utils/format";
import {
  formatGps,
  getNumberValue,
  getRecordData,
  getRecordField,
  getTextValue,
} from "../utils/records";
import type { ParsedMessageType } from "../types";

export const MESSAGE_PAGE_ORDER: ParsedMessageType[] = [
  "did_encrypted",
  "rid",
  "did_plain",
  "detect",
  "heartbeat",
];

export const MESSAGE_PAGE_CONFIG: Record<ParsedMessageType, MessagePageConfig> = {
  did_encrypted: {
    icon: Shield,
    navLabelKey: "didEncrypted",
    titleKey: "didEncrypted.title",
    tone: "info",
    tableWidth: "min-w-[118rem]",
    columns: [
      {
        labelKey: "didEncrypted.device",
        width: "w-[18rem]",
        render: (record) => getTextValue(getRecordData(record).device),
      },
      {
        labelKey: "didEncrypted.encryptedId",
        width: "w-[38rem]",
        render: (record) => getTextValue(getRecordData(record).encrypted_id),
      },
      {
        labelKey: "didEncrypted.frequency",
        width: "w-[10rem]",
        render: (record, locale) => formatNumber(locale, getNumberValue(getRecordData(record).freq)),
      },
      {
        labelKey: "didEncrypted.rssi",
        width: "w-[9rem]",
        render: (record, locale) => formatNumber(locale, getNumberValue(getRecordData(record).rssi)),
      },
      {
        labelKey: "didEncrypted.bytes",
        width: "w-[24rem]",
        render: (record) => getTextValue(getRecordData(record).bytes),
      },
    ],
  },
  rid: {
    icon: Fingerprint,
    navLabelKey: "rid",
    titleKey: "rid.title",
    tone: "success",
    tableWidth: "min-w-[142rem]",
    columns: [
      {
        labelKey: "rid.ssid",
        width: "w-[16rem]",
        render: (record) => getTextValue(getRecordData(record).ssid),
      },
      {
        labelKey: "rid.serial",
        width: "w-[18rem]",
        render: (record) => getTextValue(getRecordData(record).serial),
      },
      {
        labelKey: "rid.model",
        width: "w-[16rem]",
        render: (record) => getTextValue(getRecordData(record).model),
      },
      {
        labelKey: "rid.uaType",
        width: "w-[10rem]",
        render: (record) => getTextValue(getRecordField(record, "ua_type", "UA_type")),
      },
      {
        labelKey: "rid.droneGps",
        width: "w-[18rem]",
        render: (record) => formatGps(getRecordField(record, "drone_gps", "drone_GPS")),
      },
      {
        labelKey: "rid.pilotGps",
        width: "w-[18rem]",
        render: (record) => formatGps(getRecordField(record, "pilot_gps", "pilot_GPS")),
      },
      {
        labelKey: "rid.frequency",
        width: "w-[10rem]",
        render: (record, locale) => formatNumber(locale, getNumberValue(getRecordData(record).freq)),
      },
      {
        labelKey: "rid.rssi",
        width: "w-[9rem]",
        render: (record, locale) => formatNumber(locale, getNumberValue(getRecordData(record).rssi)),
      },
    ],
  },
  did_plain: {
    icon: FileText,
    navLabelKey: "didPlain",
    titleKey: "didPlain.title",
    tone: "warning",
    tableWidth: "min-w-[136rem]",
    columns: [
      {
        labelKey: "didPlain.device",
        width: "w-[16rem]",
        render: (record) => getTextValue(getRecordData(record).device),
      },
      {
        labelKey: "didPlain.serial",
        width: "w-[18rem]",
        render: (record) => getTextValue(getRecordData(record).serial),
      },
      {
        labelKey: "didPlain.model",
        width: "w-[16rem]",
        render: (record) => getTextValue(getRecordData(record).model),
      },
      {
        labelKey: "didPlain.uuid",
        width: "w-[30rem]",
        render: (record) => getTextValue(getRecordData(record).uuid),
      },
      {
        labelKey: "didPlain.distance",
        width: "w-[10rem]",
        render: (record) => getTextValue(getRecordData(record).distance),
      },
      {
        labelKey: "didPlain.frequency",
        width: "w-[10rem]",
        render: (record, locale) => formatNumber(locale, getNumberValue(getRecordData(record).freq)),
      },
      {
        labelKey: "didPlain.rssi",
        width: "w-[9rem]",
        render: (record, locale) => formatNumber(locale, getNumberValue(getRecordData(record).rssi)),
      },
    ],
  },
  detect: {
    icon: Radio,
    navLabelKey: "detect",
    titleKey: "detect.title",
    tone: "info",
    tableWidth: "min-w-[90rem]",
    columns: [
      {
        labelKey: "detect.device",
        width: "w-[20rem]",
        render: (record) => getTextValue(getRecordData(record).device),
      },
      {
        labelKey: "detect.model",
        width: "w-[18rem]",
        render: (record) => getTextValue(getRecordData(record).model),
      },
      {
        labelKey: "detect.frequency",
        width: "w-[10rem]",
        render: (record, locale) => formatNumber(locale, getNumberValue(getRecordData(record).freq)),
      },
      {
        labelKey: "detect.rssi",
        width: "w-[9rem]",
        render: (record, locale) => formatNumber(locale, getNumberValue(getRecordData(record).rssi)),
      },
    ],
  },
  heartbeat: {
    icon: Activity,
    navLabelKey: "heartbeat",
    titleKey: "heartbeat.title",
    tone: "error",
    tableWidth: "min-w-[72rem]",
    columns: [
      {
        labelKey: "heartbeat.device",
        width: "w-[18rem]",
        render: (record) => getTextValue(getRecordData(record).device),
      },
      {
        labelKey: "heartbeat.seq",
        width: "w-[12rem]",
        render: (record) => getTextValue(getRecordData(record).seq),
      },
    ],
  },
};
