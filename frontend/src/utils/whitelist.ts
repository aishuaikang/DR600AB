import type { ScreenAlarmSettings, WhitelistItem } from "../types";

export const defaultScreenAlarmSettings: ScreenAlarmSettings = {
  detection: true,
  position: true,
  fpv: true,
  sound: true,
};

export function normalizeWhitelistSerial(serial: string | undefined | null) {
  return (serial ?? "").trim().toLowerCase();
}

export function resolveScreenAlarmSettings(settings?: Partial<ScreenAlarmSettings>): ScreenAlarmSettings {
  return {
    ...defaultScreenAlarmSettings,
    ...(settings ?? {}),
  };
}

export function isSerialWhitelisted(serial: string | undefined | null, whitelist: WhitelistItem[] | undefined) {
  const normalized = normalizeWhitelistSerial(serial);
  if (!normalized) {
    return false;
  }
  return Boolean(whitelist?.some((item) => normalizeWhitelistSerial(item.serial) === normalized));
}

export function upsertWhitelistItem(
  whitelist: WhitelistItem[] | undefined,
  item: Pick<WhitelistItem, "serial"> & Partial<WhitelistItem>,
) {
  const serial = item.serial.trim();
  const key = normalizeWhitelistSerial(serial);
  if (!key) {
    return whitelist ?? [];
  }

  const nextItem: WhitelistItem = {
    serial,
    model: item.model?.trim() || undefined,
    source: item.source?.trim() || undefined,
    createdAt: item.createdAt || new Date().toISOString(),
  };
  const items = whitelist ?? [];
  const index = items.findIndex((current) => normalizeWhitelistSerial(current.serial) === key);
  if (index < 0) {
    return [...items, nextItem];
  }

  const next = [...items];
  next[index] = {
    ...next[index],
    ...nextItem,
    createdAt: next[index].createdAt || nextItem.createdAt,
  };
  return next;
}

export function removeWhitelistSerial(whitelist: WhitelistItem[] | undefined, serial: string | undefined | null) {
  const key = normalizeWhitelistSerial(serial);
  if (!key) {
    return whitelist ?? [];
  }
  return (whitelist ?? []).filter((item) => normalizeWhitelistSerial(item.serial) !== key);
}
