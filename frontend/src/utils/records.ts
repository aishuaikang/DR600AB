import type { DebugRecord, DetectionRecord, ParsedMessage } from "../types";

export function isDetectionRecord(record: DebugRecord): record is DetectionRecord {
  return "parsed" in record;
}

export function getRecordData(record: DebugRecord): Record<string, unknown> {
  const data = isDetectionRecord(record) ? record.parsed.data : record.data;
  if (data && typeof data === "object" && !Array.isArray(data)) {
    return data as Record<string, unknown>;
  }
  return {};
}

export function getRecordField(record: DebugRecord, ...keys: string[]): unknown {
  const data = getRecordData(record);
  for (const key of keys) {
    if (data[key] !== undefined && data[key] !== null) {
      return data[key];
    }
  }
  return undefined;
}

export function getTextValue(value: unknown): string {
  if (value === null || value === undefined) {
    return "-";
  }
  if (typeof value === "string") {
    return value.trim() || "-";
  }
  if (typeof value === "number") {
    return Number.isFinite(value) ? String(value) : "-";
  }
  if (typeof value === "boolean") {
    return value ? "true" : "false";
  }
  if (Array.isArray(value)) {
    return value.map((item) => getTextValue(item)).join(", ");
  }
  if (typeof value === "object") {
    return JSON.stringify(value);
  }
  return String(value);
}

export function getNumberValue(value: unknown): number | undefined {
  if (typeof value === "number" && Number.isFinite(value)) {
    return value;
  }
  if (typeof value === "string" && value.trim() !== "") {
    const next = Number(value);
    if (Number.isFinite(next)) {
      return next;
    }
  }
  return undefined;
}

export function formatGps(value: unknown): string {
  if (value && typeof value === "object" && !Array.isArray(value)) {
    const gps = value as { lat?: unknown; lng?: unknown };
    const lat = getNumberValue(gps.lat);
    const lng = getNumberValue(gps.lng);
    if (typeof lat === "number" && typeof lng === "number") {
      return `${lat.toFixed(6)}, ${lng.toFixed(6)}`;
    }
  }
  return getTextValue(value);
}

export function buildSearchText(record: DebugRecord): string {
  if (isDetectionRecord(record)) {
    return `${record.kind} ${record.displayModel ?? ""} ${record.summary} ${record.parsed.raw} ${JSON.stringify(record.parsed.data ?? {})}`.toLowerCase();
  }
  return `${record.type} ${record.raw} ${JSON.stringify(record.data ?? {})}`.toLowerCase();
}
