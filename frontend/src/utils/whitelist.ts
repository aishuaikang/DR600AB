import type {
  GeoPoint,
  ScreenAlarmSettings,
  ScreenDeviceLocationResponse,
  ScreenPositionTarget,
  UserSettings,
  WarningZone,
  WhitelistItem,
} from "../types";

export const defaultScreenAlarmSettings: ScreenAlarmSettings = {
  detection: true,
  position: true,
  fpv: true,
  sound: true,
};

export const defaultWarningZoneRadiusMeters = 500;
export const minWarningZoneRadiusMeters = 10;
export const maxWarningZoneRadiusMeters = 50000;

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

export function validWarningZoneRadius(value: number) {
  return Number.isFinite(value) &&
    value >= minWarningZoneRadiusMeters &&
    value <= maxWarningZoneRadiusMeters;
}

export function resolveWarningZoneEnabled(settings: UserSettings) {
  return settings.warningZoneEnabled === true;
}

export function resolveWarningZoneRadiusMeters(settings: UserSettings) {
  const radius = settings.warningZoneRadiusMeters;
  if (typeof radius === "number" && validWarningZoneRadius(radius)) {
    return radius;
  }
  return defaultWarningZoneRadiusMeters;
}

function validGeoPoint(point?: GeoPoint | null): point is GeoPoint {
  return Boolean(
    point &&
      Number.isFinite(point.latitude) &&
      Number.isFinite(point.longitude) &&
      point.latitude >= -90 &&
      point.latitude <= 90 &&
      point.longitude >= -180 &&
      point.longitude <= 180 &&
      !(point.latitude === 0 && point.longitude === 0),
  );
}

function degreesToRadians(value: number) {
  return value * Math.PI / 180;
}

export function distanceMeters(from: GeoPoint, to: GeoPoint) {
  const earthRadiusM = 6_371_000;
  const lat1 = degreesToRadians(from.latitude);
  const lat2 = degreesToRadians(to.latitude);
  const deltaLat = degreesToRadians(to.latitude - from.latitude);
  const deltaLon = degreesToRadians(to.longitude - from.longitude);
  const a = Math.sin(deltaLat / 2) ** 2 +
    Math.cos(lat1) * Math.cos(lat2) * Math.sin(deltaLon / 2) ** 2;
  return earthRadiusM * 2 * Math.atan2(Math.sqrt(a), Math.sqrt(1 - a));
}

export function resolveActiveWarningZone(
  settings: UserSettings,
  deviceLocation: ScreenDeviceLocationResponse | null,
): WarningZone | null {
  if (!resolveWarningZoneEnabled(settings) || !deviceLocation?.valid || !validGeoPoint(deviceLocation.point)) {
    return null;
  }
  return {
    id: "device-warning-zone",
    center: deviceLocation.point,
    radiusMeters: resolveWarningZoneRadiusMeters(settings),
  };
}

export function targetInsideWarningZone(target: ScreenPositionTarget, warningZone: WarningZone) {
  const points = [target.drone, target.pilot].filter(validGeoPoint);
  return points.some((point) => distanceMeters(warningZone.center, point) <= warningZone.radiusMeters);
}

export function targetTriggersAlarm(
  target: ScreenPositionTarget,
  whitelist: WhitelistItem[] | undefined,
  warningZone: WarningZone | null,
  warningZoneEnabled: boolean,
) {
  if (isSerialWhitelisted(target.serial, whitelist)) {
    return false;
  }
  if (!warningZoneEnabled) {
    return true;
  }
  return Boolean(warningZone && targetInsideWarningZone(target, warningZone));
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

export function updateWhitelistItem(
  whitelist: WhitelistItem[] | undefined,
  currentSerial: string | undefined | null,
  item: Pick<WhitelistItem, "serial"> & Partial<WhitelistItem>,
) {
  const currentKey = normalizeWhitelistSerial(currentSerial);
  const serial = item.serial.trim();
  const nextKey = normalizeWhitelistSerial(serial);
  if (!currentKey || !nextKey) {
    return whitelist ?? [];
  }

  const items = whitelist ?? [];
  const currentIndex = items.findIndex((current) => normalizeWhitelistSerial(current.serial) === currentKey);
  if (currentIndex < 0) {
    return items;
  }

  const updatedItem: WhitelistItem = {
    ...items[currentIndex],
    serial,
    model: item.model?.trim() || undefined,
    source: item.source?.trim() || items[currentIndex].source,
    createdAt: items[currentIndex].createdAt || item.createdAt || new Date().toISOString(),
  };
  const duplicateIndex = items.findIndex((current, index) => (
    index !== currentIndex &&
    normalizeWhitelistSerial(current.serial) === nextKey
  ));
  if (duplicateIndex < 0) {
    const next = [...items];
    next[currentIndex] = updatedItem;
    return next;
  }

  const next = items.filter((_, index) => index !== currentIndex);
  const adjustedDuplicateIndex = duplicateIndex > currentIndex ? duplicateIndex - 1 : duplicateIndex;
  next[adjustedDuplicateIndex] = {
    ...next[adjustedDuplicateIndex],
    ...updatedItem,
    createdAt: next[adjustedDuplicateIndex].createdAt || updatedItem.createdAt,
  };
  return next;
}
