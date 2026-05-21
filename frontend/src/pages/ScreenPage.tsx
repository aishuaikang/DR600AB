import { useCallback, useEffect, useRef, useState } from "react";
import type { ReactNode } from "react";
import type { TFunction } from "i18next";
import type L from "leaflet";
import { Activity, ChevronDown, ChevronLeft, ChevronRight, Cpu, Globe2, Inbox, Loader2, MapPin, Orbit, QrCode, Radar, Radio, RadioTower, RefreshCw, Route, SatelliteDish, ScanSearch, Settings2, Shield, Square, Thermometer, TimerReset, X, Zap } from "lucide-react";
import * as QRCode from "qrcode";

import {
  getScreenDetections,
  getScreenDeception,
  getScreenDeceptionStatus,
  getScreenDeviceLocation,
  getScreenPositions,
  getScreenStatus,
  getScreenStrike,
  openScreenStream,
  updateScreenDeception,
  updateScreenStrike,
} from "../api";
import type {
  ScreenDetectionTarget,
  ScreenDeceptionDeviceSignalStatus,
  ScreenDeceptionDeviceStatus,
  ScreenDeceptionMode,
  ScreenDeceptionSignalWorkStatus,
  ScreenDeceptionState,
  ScreenDeviceLocationResponse,
  ScreenPositionPoint,
  ScreenPositionTarget,
  ScreenRuntimeStatus,
  ScreenSerialCapabilityStatus,
  ScreenStrikeChannel,
  ScreenStrikeState,
  UserSettings,
} from "../types";
import { cx } from "../utils/classnames";
import footerBg from "../assets/images/screen/footerBg.svg?raw";
import headerBg from "../assets/images/screen/headerBg.svg?raw";
import mini2Image from "../assets/images/uav/mini2.png";
import i18n from "../i18n";
import { noFlyZonePresets } from "../data/noFlyZones";
import type { NoFlyZonePreset } from "../data/noFlyZones";
import { gps84ToGcj02 } from "../utils/leafletCoordConverter";
import { compactLocaleName } from "../utils/locales";
import { readDeveloperSession } from "../utils/developer";
import { ScreenMap } from "./ScreenMap";
import type { ReferenceMapLayer, ScreenAlertKind } from "./screenData";

const screenDetectionLimit = 100;
const screenPositionLimit = 100;
const screenDetectionFreshMs = 15_000;
const screenDetectionStaleMs = 40_000;
const screenStrikeDefaultDurationSeconds = 60;
const screenStrikeMinDurationSeconds = 10;
const screenStrikeMaxDurationSeconds = 60;
const screenStrikeDurationPresets = [10, 15, 20, 30, 45, 60];
const screenDeceptionDefaultMode: ScreenDeceptionMode = "fixed_point";
const screenDeceptionMinAltitudeM = -500;
const screenDeceptionMaxAltitudeM = 10000;
const manualNoFlyZonePresetId = "__manual__";
const screenDeceptionModeOptions: Array<{
  id: ScreenDeceptionMode;
  labelKey: string;
  descriptionKey: string;
  Icon: typeof SatelliteDish;
}> = [
  { id: "fixed_point", labelKey: "deceptionModes.fixedPoint", descriptionKey: "deceptionModeDescriptions.fixedPoint", Icon: MapPin },
  { id: "circle", labelKey: "deceptionModes.circle", descriptionKey: "deceptionModeDescriptions.circle", Icon: Orbit },
  { id: "linear", labelKey: "deceptionModes.linear", descriptionKey: "deceptionModeDescriptions.linear", Icon: Route },
];

function isScreenDeceptionMode(value: string | undefined): value is ScreenDeceptionMode {
  return Boolean(value && screenDeceptionModeOptions.some((option) => option.id === value));
}

type ScreenOperationTab = "interference" | "deception";
type NavigationMapProvider = "amap" | "google";
type NavigationCoordinateSystem = "WGS84" | "GCJ-02";
type NavigationQRCodeItem = {
  provider: NavigationMapProvider;
  labelKey: "leaflet.map.gaodeMap" | "leaflet.map.googleMap";
  url: string;
  dataUrl: string;
  coordinate: ScreenPositionPoint;
  coordinateSystem: NavigationCoordinateSystem;
  coordinateLabelKey: "navigationCoordinateOriginal" | "navigationCoordinateConverted";
};
type NavigationQRCodeState = {
  label: string;
  point: ScreenPositionPoint;
  convertedPoint: ScreenPositionPoint;
  items: NavigationQRCodeItem[];
};
type NoFlyZonePresetWithDistance = NoFlyZonePreset & {
  distanceM?: number;
};
const droneImageModules = import.meta.glob("../assets/images/drone/*.png", {
  eager: true,
  query: "?url",
  import: "default",
}) as Record<string, string>;
const uavImageModules = import.meta.glob("../assets/images/uav/*.png", {
  eager: true,
  query: "?url",
  import: "default",
}) as Record<string, string>;
const positionModelImageNames: Record<string, string> = {
  "air 3": "dji_air3",
  "air 2s": "mavic3_mavicair2s",
  "dji air 3": "dji_air3",
  "dji air3": "dji_air3",
  "dji air 2s": "mavic3_mavicair2s",
  "dji air2s": "mavic3_mavicair2s",
  "mavic 3": "mavic3",
  "mavic 3 pro": "mavic_3_pro",
  "mavic air 2": "mavic_air2",
  "mavic air 2s": "mavic3_mavicair2s",
  "mini 4 pro": "mini4_pro",
};

function getDroneImageUrl(model: string) {
  if (!model) {
    return mini2Image;
  }
  return droneImageModules[`../assets/images/drone/${model}.png`] ?? mini2Image;
}

function getPositionDroneImageUrl(model: string) {
  const name = positionModelImageNames[model.trim().toLowerCase()];
  if (name) {
    return uavImageModules[`../assets/images/uav/${name}.png`] ?? mini2Image;
  }
  return getDroneImageUrl(model);
}

function isFpvTarget(target: ScreenDetectionTarget) {
  return target.model.trim() === "PAL Analog";
}

function formatFrequency(value: number) {
  if (!Number.isFinite(value)) {
    return "-";
  }
  return `${Math.round(value)}MHz`;
}

function formatRSSI(value: number) {
  if (!Number.isFinite(value)) {
    return "-";
  }
  return `${Math.round(value)}dBm`;
}

function formatStrikeBand(value: string) {
  const band = value.trim();
  if (!band) {
    return "";
  }
  const numeric = Number.parseFloat(band);
  if (Number.isFinite(numeric)) {
    return numeric >= 100 ? `${band}M` : `${band}G`;
  }
  return band;
}

function formatStrikeChannelLabel(channel: ScreenStrikeChannel, index: number, customLabels: string[] = []) {
  const customLabel = customLabels[index]?.trim();
  if (customLabel) {
    return customLabel;
  }
  const bands = Array.isArray(channel.bands)
    ? channel.bands.map(formatStrikeBand).filter(Boolean)
    : [];
  if (bands.length > 0) {
    return bands.join("/");
  }
  return channel.label || channel.id || `IO${index + 1}`;
}

function clampStrikeDuration(value: number) {
  if (!Number.isFinite(value)) {
    return screenStrikeDefaultDurationSeconds;
  }
  return Math.max(screenStrikeMinDurationSeconds, Math.min(screenStrikeMaxDurationSeconds, Math.round(value)));
}

function normalizeCoordinateInput(value: string) {
  return value.replace(/[^\d.,-]/g, "").replace(",", ".");
}

function parseCoordinateInput(value: string) {
  if (value.trim() === "") {
    return Number.NaN;
  }
  return Number(value.replace(",", "."));
}

function validLatitude(value: number) {
  return Number.isFinite(value) && value >= -90 && value <= 90;
}

function validLongitude(value: number) {
  return Number.isFinite(value) && value >= -180 && value <= 180;
}

function validDeceptionCoordinate(latitude: number, longitude: number) {
  return validLatitude(latitude) &&
    validLongitude(longitude) &&
    !(latitude === 0 && longitude === 0);
}

function validDeceptionAltitude(value: number) {
  return Number.isFinite(value) &&
    value >= screenDeceptionMinAltitudeM &&
    value <= screenDeceptionMaxAltitudeM;
}

function parsePanelNumber(value: string, fallback: number) {
  const parsed = Number(value);
  return Number.isFinite(parsed) ? parsed : fallback;
}

function bearingFromTo(from: ScreenPositionPoint, to: ScreenPositionPoint) {
  const lat1 = degreesToRadians(from.latitude);
  const lat2 = degreesToRadians(to.latitude);
  const deltaLon = degreesToRadians(to.longitude - from.longitude);
  const y = Math.sin(deltaLon) * Math.cos(lat2);
  const x = Math.cos(lat1) * Math.sin(lat2) - Math.sin(lat1) * Math.cos(lat2) * Math.cos(deltaLon);
  return normalizeDegrees(radiansToDegrees(Math.atan2(y, x)));
}

function defaultLinearDirection(deviceLocation: ScreenDeviceLocationResponse | null, target: ScreenPositionPoint | null) {
  if (!deviceLocation?.valid || !deviceLocation.point || !target) {
    return 0;
  }
  return normalizeDegrees(bearingFromTo(deviceLocation.point, target) + 180);
}

function distanceMeters(from: ScreenPositionPoint, to: ScreenPositionPoint) {
  const earthRadiusM = 6_371_000;
  const lat1 = degreesToRadians(from.latitude);
  const lat2 = degreesToRadians(to.latitude);
  const deltaLat = degreesToRadians(to.latitude - from.latitude);
  const deltaLon = degreesToRadians(to.longitude - from.longitude);
  const a = Math.sin(deltaLat / 2) ** 2 +
    Math.cos(lat1) * Math.cos(lat2) * Math.sin(deltaLon / 2) ** 2;
  return earthRadiusM * 2 * Math.atan2(Math.sqrt(a), Math.sqrt(1 - a));
}

function formatPresetDistance(distanceM?: number) {
  if (typeof distanceM !== "number" || !Number.isFinite(distanceM)) {
    return "-";
  }
  if (distanceM >= 1000) {
    return `${(distanceM / 1000).toFixed(distanceM >= 100_000 ? 0 : 1)}km`;
  }
  return `${Math.round(distanceM)}m`;
}

function getNoFlyZoneDistanceOrigin(
  deviceLocation: ScreenDeviceLocationResponse | null,
  deceptionDeviceStatus: ScreenDeceptionDeviceStatus | null,
): ScreenPositionPoint | null {
  if (deviceLocation?.valid && deviceLocation.point) {
    return deviceLocation.point;
  }
  const point = deceptionDeviceStatus?.currentPosition;
  if (point && validDeceptionCoordinate(point.latitude, point.longitude)) {
    return { latitude: point.latitude, longitude: point.longitude };
  }
  const queriedPoint = deceptionDeviceStatus?.queriedDevicePosition;
  if (queriedPoint && validDeceptionCoordinate(queriedPoint.latitude, queriedPoint.longitude)) {
    return { latitude: queriedPoint.latitude, longitude: queriedPoint.longitude };
  }
  return null;
}

function getSortedNoFlyZonePresets(origin: ScreenPositionPoint | null): NoFlyZonePresetWithDistance[] {
  return noFlyZonePresets
    .map((preset) => ({
      ...preset,
      distanceM: origin
        ? distanceMeters(origin, { latitude: preset.latitude, longitude: preset.longitude })
        : undefined,
    }))
    .sort((left, right) => {
      if (typeof left.distanceM === "number" && typeof right.distanceM === "number") {
        return left.distanceM - right.distanceM;
      }
      if (typeof left.distanceM === "number") {
        return -1;
      }
      if (typeof right.distanceM === "number") {
        return 1;
      }
      return left.name.localeCompare(right.name, "zh-Hans-CN");
    });
}

function normalizeDegrees(value: number) {
  if (!Number.isFinite(value)) {
    return 0;
  }
  const normalized = value % 360;
  return normalized < 0 ? normalized + 360 : normalized;
}

function degreesToRadians(value: number) {
  return value * Math.PI / 180;
}

function radiansToDegrees(value: number) {
  return value * 180 / Math.PI;
}

function EmptyState({
  t,
  compact = false,
  className = "",
}: {
  t: TFunction;
  compact?: boolean;
  className?: string;
}) {
  return (
    <div className={cx("screen-empty", compact && "screen-empty--compact", className)}>
      <Inbox className="screen-empty__icon" size={compact ? 16 : 20} aria-hidden="true" />
      <span>{t("noData", { ns: "screen" })}</span>
    </div>
  );
}

function ScreenOfflineState({
  title,
  message,
  detail,
  compact = false,
}: {
  title: string;
  message: string;
  detail?: string;
  compact?: boolean;
}) {
  return (
    <div className={cx("screen-offline-state", compact && "screen-offline-state--compact")}>
      <RadioTower className="screen-offline-state__icon" size={compact ? 16 : 20} aria-hidden="true" />
      <strong>{title}</strong>
      <span>{message}</span>
      {detail ? <em>{detail}</em> : null}
    </div>
  );
}

function screenCapabilityOfflineMessage(status: ScreenSerialCapabilityStatus | undefined, t: TFunction) {
  if (!status?.configured) {
    return "";
  }
  if (status.lastError) {
    return status.lastError;
  }
  if (status.state === "connecting" || status.state === "reconnecting") {
    return t("serialConnecting", { ns: "screen" });
  }
  return t("serialOffline", { ns: "screen" });
}

function getStrikeRemainingSeconds(state: ScreenStrikeState | null, now: Date) {
  if (!state?.active) {
    return 0;
  }
  if (state.endsAt) {
    const endsAt = new Date(state.endsAt).getTime();
    if (!Number.isNaN(endsAt)) {
      return Math.max(0, Math.ceil((endsAt - now.getTime()) / 1000));
    }
  }
  return Math.max(0, state.remainingSeconds || 0);
}

function formatCountdown(seconds: number) {
  const safeSeconds = Math.max(0, Math.floor(seconds));
  const minutes = Math.floor(safeSeconds / 60);
  const rest = safeSeconds % 60;
  return `${String(minutes).padStart(2, "0")}:${String(rest).padStart(2, "0")}`;
}

function formatOptionalNumber(value: number | undefined, unit: string, digits = 0) {
  if (typeof value !== "number" || !Number.isFinite(value)) {
    return "-";
  }
  return `${value.toFixed(digits)}${unit}`;
}

function formatCoordinateValue(value: number | undefined) {
  if (typeof value !== "number" || !Number.isFinite(value)) {
    return "-";
  }
  return value.toFixed(6);
}

function formatNavigationCoordinates(point: ScreenPositionPoint) {
  return `${formatCoordinateValue(point.latitude)}, ${formatCoordinateValue(point.longitude)}`;
}

function getNavigationCoordinates(point: ScreenPositionPoint) {
  const gcj02 = gps84ToGcj02(point.longitude, point.latitude);
  return {
    original: point,
    converted: {
      latitude: gcj02.lat,
      longitude: gcj02.lng,
    } satisfies ScreenPositionPoint,
  };
}

function validPositionMapPoint(point?: ScreenPositionPoint | null): point is ScreenPositionPoint {
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

function firstPositionMapPoint(target: ScreenPositionTarget) {
  if (validPositionMapPoint(target.drone)) {
    return target.drone;
  }
  if (validPositionMapPoint(target.pilot)) {
    return target.pilot;
  }
  return null;
}

const navigationMapProviders: Array<{ id: NavigationMapProvider; label: "leaflet.map.gaodeMap" | "leaflet.map.googleMap" }> = [
  { id: "google", label: "leaflet.map.googleMap" },
  { id: "amap", label: "leaflet.map.gaodeMap" },
];

function buildNavigationUrl(coordinates: ReturnType<typeof getNavigationCoordinates>, provider: NavigationMapProvider) {
  if (provider === "google") {
    const latitude = coordinates.original.latitude.toFixed(6);
    const longitude = coordinates.original.longitude.toFixed(6);
    return `https://www.google.com/maps?q=${latitude},${longitude}`;
  }
  return `https://m.amap.com/share/index/lnglat=${coordinates.converted.longitude.toFixed(6)},${coordinates.converted.latitude.toFixed(6)}&src=mypage&callnative=1&innersrc=uriapi`;
}

async function createNavigationQRCode(
  point: ScreenPositionPoint,
  provider: (typeof navigationMapProviders)[number],
) {
  const coordinates = getNavigationCoordinates(point);
  const url = buildNavigationUrl(coordinates, provider.id);
  const coordinate = provider.id === "google" ? coordinates.original : coordinates.converted;
  const coordinateSystem: NavigationCoordinateSystem = provider.id === "google" ? "WGS84" : "GCJ-02";
  const coordinateLabelKey = provider.id === "google" ? "navigationCoordinateOriginal" : "navigationCoordinateConverted";
  const dataUrl = await QRCode.toDataURL(url, {
    errorCorrectionLevel: "M",
    margin: 1,
    width: 320,
    color: {
      dark: "#06131f",
      light: "#ffffff",
    },
  });
  return {
    provider: provider.id,
    labelKey: provider.label,
    url,
    dataUrl,
    coordinate,
    coordinateSystem,
    coordinateLabelKey,
  } satisfies NavigationQRCodeItem;
}

async function createNavigationQRCodes(label: string, point: ScreenPositionPoint) {
  const coordinates = getNavigationCoordinates(point);
  const results = await Promise.allSettled(
    navigationMapProviders.map((provider) => createNavigationQRCode(point, provider)),
  );
  const items = results.map((result, index) => {
    const provider = navigationMapProviders[index];
    if (result.status === "fulfilled") {
      return result.value;
    }
    const coordinate = provider.id === "google" ? coordinates.original : coordinates.converted;
    const coordinateSystem: NavigationCoordinateSystem = provider.id === "google" ? "WGS84" : "GCJ-02";
    const coordinateLabelKey = provider.id === "google" ? "navigationCoordinateOriginal" : "navigationCoordinateConverted";
    return {
      provider: provider.id,
      labelKey: provider.label,
      url: buildNavigationUrl(coordinates, provider.id),
      dataUrl: "",
      coordinate,
      coordinateSystem,
      coordinateLabelKey,
    } satisfies NavigationQRCodeItem;
  });
  return {
    label,
    point: coordinates.original,
    convertedPoint: coordinates.converted,
    items,
  } satisfies NavigationQRCodeState;
}

function getRSSIPercent(value: number) {
  if (!Number.isFinite(value)) {
    return 0;
  }
  return Math.max(0, Math.min(100, Math.round(((value + 100) / 65) * 100)));
}

function formatTargetTime(value: string) {
	const date = new Date(value);
	if (Number.isNaN(date.getTime())) {
		return "-";
	}
	return date.toLocaleTimeString(getScreenLocale(), { hour12: false });
}

function formatStatusTime(value?: string) {
	if (!value) {
		return "-";
	}
	const date = new Date(value);
	if (Number.isNaN(date.getTime())) {
		return "-";
	}
	return date.toLocaleString(getScreenLocale(), {
		month: "2-digit",
		day: "2-digit",
		hour: "2-digit",
		minute: "2-digit",
		second: "2-digit",
		hour12: false,
	});
}

function formatStatusPoint(point?: ScreenPositionPoint & { altitudeM?: number }) {
	if (!point) {
		return "-";
	}
	const altitude = typeof point.altitudeM === "number" && Number.isFinite(point.altitudeM)
		? ` / ${point.altitudeM.toFixed(1)}m`
		: "";
	return `${formatCoordinateValue(point.latitude)}, ${formatCoordinateValue(point.longitude)}${altitude}`;
}

function formatBooleanStatus(value: boolean | undefined, t: TFunction) {
	if (typeof value !== "boolean") {
		return "-";
	}
	return t(value ? "statusNormal" : "statusAbnormal", { ns: "screen" });
}

function formatOnOff(value: boolean | undefined, t: TFunction) {
	if (typeof value !== "boolean") {
		return "-";
	}
	return t(value ? "statusOn" : "statusOff", { ns: "screen" });
}

function formatDurationSeconds(value: number | undefined) {
	if (typeof value !== "number" || !Number.isFinite(value)) {
		return "-";
	}
	const seconds = Math.max(0, Math.round(value));
	const hours = Math.floor(seconds / 3600);
	const minutes = Math.floor((seconds % 3600) / 60);
	const rest = seconds % 60;
	if (hours > 0) {
		return `${hours}h ${minutes}m ${rest}s`;
	}
	if (minutes > 0) {
		return `${minutes}m ${rest}s`;
	}
	return `${rest}s`;
}

function formatSignalList(signals?: string[]) {
	return signals && signals.length > 0 ? signals.join(" / ") : "-";
}

function getDeceptionDeviceStatusTone(
	status: ScreenDeceptionDeviceStatus | null,
	loading: boolean,
): "loading" | "offline" | "error" | "normal" {
	if (loading) {
		return "loading";
	}
	if (!status?.serialActive) {
		return "offline";
	}
	if (status.lastError) {
		return "error";
	}
	return "normal";
}

function getTargetTimeTone(lastSeen: string, now: Date) {
  const lastSeenAt = new Date(lastSeen).getTime();
  if (Number.isNaN(lastSeenAt)) {
    return "unknown";
  }
  const ageMs = Math.max(0, now.getTime() - lastSeenAt);
  if (ageMs <= screenDetectionFreshMs) {
    return "fresh";
  }
  if (ageMs <= screenDetectionStaleMs) {
    return "stale";
  }
  return "old";
}

function getTargetTimeToneTitle(tone: string, t: TFunction) {
  return t(`targetFreshness.${tone}`, { ns: "screen" });
}

function screenTargetSortValue(target: ScreenDetectionTarget) {
  const value = new Date(target.firstSeen).getTime();
  return Number.isNaN(value) ? 0 : value;
}

function compareScreenTargets(a: ScreenDetectionTarget, b: ScreenDetectionTarget) {
  const timeDiff = screenTargetSortValue(b) - screenTargetSortValue(a);
  if (timeDiff !== 0) {
    return timeDiff;
  }
  return b.id.localeCompare(a.id);
}

function sortScreenTargets(items: ScreenDetectionTarget[]) {
  return [...items].sort(compareScreenTargets);
}

function mergeScreenTarget(
  items: ScreenDetectionTarget[],
  target: ScreenDetectionTarget,
  limit: number,
) {
  const index = items.findIndex((item) => item.id === target.id);
  if (index >= 0) {
    const next = [...items];
    next[index] = target;
    return next;
  }
  return sortScreenTargets([...items, target]).slice(0, limit);
}

function screenPositionSortValue(target: ScreenPositionTarget) {
  const value = new Date(target.firstSeen).getTime();
  return Number.isNaN(value) ? 0 : value;
}

function compareScreenPositions(a: ScreenPositionTarget, b: ScreenPositionTarget) {
  const timeDiff = screenPositionSortValue(b) - screenPositionSortValue(a);
  if (timeDiff !== 0) {
    return timeDiff;
  }
  return b.id.localeCompare(a.id);
}

function sortScreenPositions(items: ScreenPositionTarget[]) {
  return [...items].sort(compareScreenPositions);
}

function mergeScreenPosition(
  items: ScreenPositionTarget[],
  target: ScreenPositionTarget,
  limit: number,
) {
  const index = items.findIndex((item) => item.id === target.id);
  if (index >= 0) {
    const next = [...items];
    next[index] = target;
    return next;
  }
  return sortScreenPositions([...items, target]).slice(0, limit);
}

function getScreenLocale() {
  return i18n.language.startsWith("zh") ? "zh-CN" : "en-US";
}

function formatScreenDate(value: Date) {
  const year = value.getFullYear();
  const month = String(value.getMonth() + 1).padStart(2, "0");
  const day = String(value.getDate()).padStart(2, "0");
  return `${year}-${month}-${day}`;
}

function ScreenHeader({
  appTitle,
  t,
  now,
  locale,
  localeOptions,
  onLocaleChange,
  onEnterAdmin,
}: {
  appTitle: string;
  t: TFunction;
  now: Date;
  locale: string;
  localeOptions: string[];
  onLocaleChange: (locale: string) => void;
  onEnterAdmin: () => void;
}) {
  const [languageOpen, setLanguageOpen] = useState(false);
  const languageLabel = compactLocaleName(locale);

  return (
    <header className="screen-header">
      <span className="screen-header-bg" dangerouslySetInnerHTML={{ __html: headerBg }} />
      <div className="screen-header__left">
        <span className="screen-header__date">{formatScreenDate(now)}</span>
        <strong className="screen-header__time">{now.toLocaleTimeString(getScreenLocale(), { hour12: false })}</strong>
      </div>

      <div className="screen-header__title">
        <h1>{appTitle}</h1>
      </div>

      <div className="screen-header__right">
        <div
          className={cx("screen-language-switch", languageOpen && "screen-language-switch--open")}
          onBlur={(event) => {
            const nextFocus = event.relatedTarget;
            if (!(nextFocus instanceof Node) || !event.currentTarget.contains(nextFocus)) {
              setLanguageOpen(false);
            }
          }}
          onKeyDown={(event) => {
            if (event.key === "Escape") {
              setLanguageOpen(false);
            }
          }}
        >
          <button
            className="screen-language-switch__button"
            type="button"
            aria-label={t("language", { ns: "settings" })}
            aria-haspopup="listbox"
            aria-expanded={languageOpen}
            onClick={() => setLanguageOpen((value) => !value)}
          >
            <Globe2 size={15} />
            <span>{languageLabel}</span>
            <ChevronDown className="screen-language-switch__arrow" size={13} />
          </button>

          {languageOpen ? (
            <div className="screen-language-menu" role="listbox" aria-label={t("language", { ns: "settings" })}>
              {localeOptions.map((option) => (
                <button
                  key={option}
                  className={cx("screen-language-menu__item", option === locale && "screen-language-menu__item--active")}
                  type="button"
                  role="option"
                  aria-selected={option === locale}
                  onClick={() => {
                    onLocaleChange(option);
                    setLanguageOpen(false);
                  }}
                >
                  {compactLocaleName(option)}
                </button>
              ))}
            </div>
          ) : null}
        </div>
        <button className="screen-admin-link" type="button" onClick={onEnterAdmin} aria-label={t("enterAdmin", { ns: "screen" })}>
          <Settings2 size={18} />
        </button>
      </div>
    </header>
  );
}

function DetectionTargetCard({
  target,
  selected,
  t,
  now,
  onSelect,
}: {
  target: ScreenDetectionTarget;
  selected: boolean;
  t: TFunction;
  now: Date;
  onSelect: (target: ScreenDetectionTarget) => void;
}) {
  const title = target.model || t("unknownTarget", { ns: "screen" });
  const imageUrl = getDroneImageUrl(target.model);
  const timeTone = getTargetTimeTone(target.lastSeen, now);
  const timeToneTitle = getTargetTimeToneTitle(timeTone, t);
  const [freshnessOpen, setFreshnessOpen] = useState(false);

  return (
    <article
      className={cx(
        "screen-detection-card",
        selected && "screen-detection-card--selected",
        freshnessOpen && "screen-detection-card--freshness-open",
      )}
      onClick={() => onSelect(target)}
    >
      <span className="screen-detection-card__image">
        <img
          src={imageUrl}
          alt=""
          loading="lazy"
          decoding="async"
          onError={(event) => {
            event.currentTarget.src = mini2Image;
          }}
        />
        <span className="screen-detection-card__glow" />
      </span>

      <div className="screen-detection-card__content">
        <span className="screen-detection-card__title">
          <strong>{title}</strong>
          <button
            className={`screen-detection-card__time screen-detection-card__time--${timeTone}`}
            type="button"
            aria-expanded={freshnessOpen}
            aria-label={timeToneTitle}
            onClick={(event) => {
              event.stopPropagation();
              setFreshnessOpen((value) => !value);
            }}
          >
            {formatTargetTime(target.lastSeen)}
          </button>
        </span>

        <div className="screen-target-readouts screen-detection-card__readouts">
          <span className="screen-target-readout">
            <em>{t("frequency", { ns: "screen" })}</em>
            <strong>{formatFrequency(target.frequency)}</strong>
          </span>
          <span className="screen-target-readout">
            <em>{t("rssi", { ns: "screen" })}</em>
            <strong>{formatRSSI(target.rssi)}</strong>
          </span>
        </div>

        {freshnessOpen ? (
          <span className={`screen-detection-card__freshness screen-detection-card__freshness--${timeTone}`}>
            {timeToneTitle}
          </span>
        ) : null}
      </div>
    </article>
  );
}

function FpvTargetTable({
  targets,
  selectedId,
  t,
  onSelect,
}: {
  targets: ScreenDetectionTarget[];
  selectedId: string;
  t: TFunction;
  onSelect: (target: ScreenDetectionTarget) => void;
}) {
  return (
    <div className="screen-fpv-table">
      <div className="screen-fpv-table__head">
        <span>{t("signal", { ns: "screen" })}</span>
        <span>{t("frequency", { ns: "screen" })}</span>
        <span>{t("signalStrength", { ns: "screen" })}</span>
      </div>

      <div className="screen-fpv-table__body">
        {targets.map((target) => {
          const signalPercent = getRSSIPercent(target.rssi);

          return (
            <button
              key={target.id}
              className={cx("screen-fpv-row", selectedId === target.id && "screen-fpv-row--selected")}
              type="button"
              onClick={() => onSelect(target)}
            >
              <span className="screen-fpv-row__signal">
                <span>
                  <strong>{t("fpvSignalTransmission", { ns: "screen" })}</strong>
                  <em>{target.model || t("unknownTarget", { ns: "screen" })}</em>
                </span>
              </span>

              <span className="screen-fpv-row__value">{formatFrequency(target.frequency)}</span>

              <span className="screen-fpv-row__strength">
                <strong>{formatRSSI(target.rssi)}</strong>
                <span className="screen-fpv-row__meter" aria-hidden="true">
                  <span style={{ width: `${signalPercent}%` }} />
                </span>
              </span>
            </button>
          );
        })}
      </div>
    </div>
  );
}

function PositionPointRow({
  label,
  point,
  t,
  onOpenNavigationQRCode,
}: {
  label: string;
  point?: ScreenPositionPoint;
  t: TFunction;
  onOpenNavigationQRCode?: (label: string, point: ScreenPositionPoint) => void;
}) {
  if (validPositionMapPoint(point) && onOpenNavigationQRCode) {
    const coordinateText = formatNavigationCoordinates(point);

    return (
      <button
        className="screen-position-card__point screen-position-card__point--clickable"
        type="button"
        title={t("navigationQRCode", { ns: "screen" })}
        aria-label={`${label} ${coordinateText} ${t("navigationQRCode", { ns: "screen" })}`}
        onClick={(event) => {
          event.stopPropagation();
          onOpenNavigationQRCode(label, point);
        }}
      >
        <em>{label}</em>
        <strong>
          <small>{t("latitudeShort", { ns: "screen" })}</small>
          {formatCoordinateValue(point.latitude)}
        </strong>
        <strong>
          <small>{t("longitudeShort", { ns: "screen" })}</small>
          {formatCoordinateValue(point.longitude)}
        </strong>
        <span className="screen-position-card__point-action" aria-hidden="true">
          <QrCode size={11} />
        </span>
      </button>
    );
  }

  return (
    <span className="screen-position-card__point">
      <em>{label}</em>
      <strong>
        <small>{t("latitudeShort", { ns: "screen" })}</small>
        {formatCoordinateValue(point?.latitude)}
      </strong>
      <strong>
        <small>{t("longitudeShort", { ns: "screen" })}</small>
        {formatCoordinateValue(point?.longitude)}
      </strong>
    </span>
  );
}

function PositionTargetCard({
  target,
  selected,
  t,
  now,
  onSelect,
  onOpenNavigationQRCode,
}: {
  target: ScreenPositionTarget;
  selected: boolean;
  t: TFunction;
  now: Date;
  onSelect: (target: ScreenPositionTarget) => void;
  onOpenNavigationQRCode?: (label: string, point: ScreenPositionPoint) => void;
}) {
  const timeTone = getTargetTimeTone(target.lastSeen, now);
  const timeToneTitle = getTargetTimeToneTitle(timeTone, t);
  const imageUrl = getPositionDroneImageUrl(target.model);
  const pendingEncrypted = target.source === "did_encrypted" && target.model === "DJI-Drone" && !target.cracked;

  return (
    <article
      className={cx("screen-position-card", selected && "screen-position-card--selected")}
      onClick={() => onSelect(target)}
    >
      <div className="screen-position-card__head">
        <span className="screen-position-card__identity">
          <span className="screen-position-card__title-row">
            <strong>{target.model || t("unknownTarget", { ns: "screen" })}</strong>
            {pendingEncrypted ? (
              <span className="screen-position-card__parsing">
                <span aria-hidden="true" />
                {t("parsingTarget", { ns: "screen" })}
              </span>
            ) : null}
          </span>
          <em>{t("deviceSn", { ns: "screen" })}: {target.serial || "-"}</em>
        </span>
        <button
          className={`screen-detection-card__time screen-detection-card__time--${timeTone}`}
          type="button"
          title={timeToneTitle}
          aria-label={timeToneTitle}
          onClick={(event) => event.stopPropagation()}
        >
          {formatTargetTime(target.lastSeen)}
        </button>
      </div>

      {pendingEncrypted ? (
        <div className="screen-position-card__metrics screen-position-card__metrics--pending screen-target-readouts">
          <span className="screen-target-readout">
            <em>{t("frequency", { ns: "screen" })}</em>
            <strong>{formatOptionalNumber(target.frequency, "MHz", 1)}</strong>
          </span>
          <span className="screen-target-readout">
            <em>{t("rssi", { ns: "screen" })}</em>
            <strong>{formatOptionalNumber(target.rssi, "dBm", 0)}</strong>
          </span>
        </div>
      ) : (
        <>
          <div className="screen-position-card__location">
            <span className="screen-position-card__image">
              <img
                src={imageUrl}
                alt=""
                loading="lazy"
                decoding="async"
                onError={(event) => {
                  event.currentTarget.src = mini2Image;
                }}
              />
              <span className="screen-position-card__image-glow" />
            </span>

            <div className="screen-position-card__grid">
              <PositionPointRow
                label={t("positionDrone", { ns: "screen" })}
                point={target.drone}
                t={t}
                onOpenNavigationQRCode={onOpenNavigationQRCode}
              />
              <PositionPointRow
                label={t("positionPilot", { ns: "screen" })}
                point={target.pilot}
                t={t}
                onOpenNavigationQRCode={onOpenNavigationQRCode}
              />
              <PositionPointRow
                label={t("positionHome", { ns: "screen" })}
                point={target.home}
                t={t}
                onOpenNavigationQRCode={onOpenNavigationQRCode}
              />
            </div>
          </div>

          <div className="screen-position-card__metrics screen-target-readouts">
            <span className="screen-target-readout">
              <em>{t("frequency", { ns: "screen" })}</em>
              <strong>{formatOptionalNumber(target.frequency, "MHz", 1)}</strong>
            </span>
            <span className="screen-target-readout">
              <em>{t("rssi", { ns: "screen" })}</em>
              <strong>{formatOptionalNumber(target.rssi, "dBm", 0)}</strong>
            </span>
            <span className="screen-target-readout">
              <em>{t("height", { ns: "screen" })}</em>
              <strong>{formatOptionalNumber(target.height, "m", 0)}</strong>
            </span>
            <span className="screen-target-readout">
              <em>{t("altitude", { ns: "screen" })}</em>
              <strong>{formatOptionalNumber(target.altitude, "m", 0)}</strong>
            </span>
            <span className="screen-target-readout">
              <em>{t("speed", { ns: "screen" })}</em>
              <strong>{formatOptionalNumber(target.speed, "m/s", 1)}</strong>
            </span>
            <span className="screen-target-readout">
              <em>{t("firstSeen", { ns: "screen" })}</em>
              <strong>{formatTargetTime(target.firstSeen)}</strong>
            </span>
          </div>
        </>
      )}
    </article>
  );
}

function NavigationQRCodeModal({
	state,
	loading,
	error,
  t,
  onClose,
}: {
  state: NavigationQRCodeState | null;
  loading: boolean;
  error: string;
  t: TFunction;
  onClose: () => void;
}) {
  if (!state) {
    return null;
  }

  return (
    <div className="screen-navigation-modal app-modal-backdrop" role="presentation" onClick={onClose}>
      <section
        className="screen-navigation-modal__card app-modal-card"
        role="dialog"
        aria-modal="true"
        aria-labelledby="screen-navigation-modal-title"
        onClick={(event) => event.stopPropagation()}
      >
        <button
          className="screen-navigation-modal__close"
          type="button"
          aria-label={t("close", { ns: "common" })}
          onClick={onClose}
        >
          <X size={16} />
        </button>

        <div className="screen-navigation-modal__header">
          <span className="screen-navigation-modal__eyebrow">{t("navigationQRCode", { ns: "screen" })}</span>
          <h2 id="screen-navigation-modal-title">{state.label}</h2>
        </div>

        <div className="screen-navigation-modal__body">
          <div className="screen-navigation-modal__coordinate-grid">
            <div className="screen-navigation-modal__coordinate-item">
              <span>{t("navigationCoordinateOriginal", { ns: "screen" })}</span>
              <strong>{t("navigationCoordinateSystemWGS84", { ns: "screen" })}</strong>
              <code>{formatNavigationCoordinates(state.point)}</code>
            </div>
            <div className="screen-navigation-modal__coordinate-item">
              <span>{t("navigationCoordinateConverted", { ns: "screen" })}</span>
              <strong>{t("navigationCoordinateSystemGCJ02", { ns: "screen" })}</strong>
              <code>{formatNavigationCoordinates(state.convertedPoint)}</code>
            </div>
          </div>

          <div className="screen-navigation-modal__qr-grid" aria-busy={loading}>
            {navigationMapProviders.map((provider) => {
              const item = state.items.find((current) => current.provider === provider.id);

              return (
                <div key={provider.id} className="screen-navigation-modal__qr-item">
                  <strong>{t(provider.label, { ns: "screen" })}</strong>
                  {item && (
                    <span className="screen-navigation-modal__qr-coordinate">
                      {t(item.coordinateLabelKey, { ns: "screen" })} / {item.coordinateSystem}: {formatNavigationCoordinates(item.coordinate)}
                    </span>
                  )}
                  <div className="screen-navigation-modal__qr">
                    {loading ? (
                      <div className="screen-navigation-modal__loading">
                        <Loader2 className="app-spinner" size={22} aria-hidden="true" />
                        <span>{t("generatingQRCode", { ns: "screen" })}</span>
                      </div>
                    ) : item?.dataUrl ? (
                      <img src={item.dataUrl} alt={t(provider.label, { ns: "screen" })} loading="lazy" decoding="async" />
                    ) : (
                      <QrCode className="screen-navigation-modal__fallback-icon" size={46} aria-hidden="true" />
                    )}
                  </div>
                </div>
              );
            })}
          </div>

          <p className={cx("screen-navigation-modal__tip", error && "screen-navigation-modal__tip--error")}>
            {error || t("scanToNavigate", { ns: "screen" })}
          </p>
        </div>
      </section>
    </div>
	);
}

function DeceptionDeviceStatusModal({
	status,
	loading,
	error,
	t,
	onRefresh,
	onClose,
}: {
	status: ScreenDeceptionDeviceStatus | null;
	loading: boolean;
	error: string;
	t: TFunction;
	onRefresh: () => void;
	onClose: () => void;
}) {
	const rawEntries = Object.entries(status?.rawDescriptions ?? {});
	const queryErrorEntries = Object.entries(status?.queryErrors ?? {});
	const [rawDescriptionsOpen, setRawDescriptionsOpen] = useState(rawEntries.length > 0);
	const tone = getDeceptionDeviceStatusTone(status, loading);
	const transmitMask = typeof status?.transmitMask === "number"
		? `0x${status.transmitMask.toString(16).toUpperCase().padStart(4, "0")}`
		: "-";
	const deviceSignalMask = typeof status?.deviceSignal?.signalMask === "number"
		? `0x${status.deviceSignal.signalMask.toString(16).toUpperCase().padStart(4, "0")}`
		: "-";
	const deviceSignals = status?.deviceSignals?.length
		? status.deviceSignals
		: status?.deviceSignal
			? [status.deviceSignal]
			: [];

	useEffect(() => {
		if (rawEntries.length > 0) {
			setRawDescriptionsOpen(true);
		}
	}, [rawEntries.length]);

	return (
		<div className="screen-navigation-modal app-modal-backdrop" role="presentation" onClick={onClose}>
			<section
				className="screen-navigation-modal__card screen-device-status-modal app-modal-card"
				role="dialog"
				aria-modal="true"
				aria-labelledby="screen-device-status-title"
				onClick={(event) => event.stopPropagation()}
			>
				<button
					className="screen-navigation-modal__close"
					type="button"
					aria-label={t("close", { ns: "common" })}
					onClick={onClose}
				>
					<X size={15} aria-hidden="true" />
				</button>

				<div className="screen-navigation-modal__header screen-device-status-modal__header">
					<span className="screen-navigation-modal__eyebrow">{t("deception", { ns: "screen" })}</span>
					<h2 id="screen-device-status-title">{t("deceptionDeviceStatus", { ns: "screen" })}</h2>
					<p>{t("deceptionStatusUpdatedAt", { ns: "screen" })}: {formatStatusTime(status?.updatedAt)}</p>
					<button className="screen-device-status-modal__refresh" type="button" disabled={loading} onClick={onRefresh}>
						{loading ? <Loader2 className="app-spinner" size={13} aria-hidden="true" /> : <RefreshCw size={13} aria-hidden="true" />}
						<span>{t("refresh", { ns: "common" })}</span>
					</button>
				</div>

				<div className="screen-device-status-modal__summary">
					<StatusSummaryItem
						icon={<RadioTower size={15} />}
						label={t("deceptionStatusConnection", { ns: "screen" })}
						value={status?.serialActive ? t("online", { ns: "screen" }) : t("offline", { ns: "screen" })}
						tone={status?.serialActive ? "normal" : "offline"}
					/>
					<StatusSummaryItem
						icon={<SatelliteDish size={15} />}
						label={t("deceptionStatusTransmit", { ns: "screen" })}
						value={formatOnOff((status?.transmitMask ?? 0) > 0, t)}
						tone={(status?.transmitMask ?? 0) > 0 ? "active" : "offline"}
					/>
					<StatusSummaryItem
						icon={<ClockIcon />}
						label={t("deceptionStatusSync", { ns: "screen" })}
						value={formatBooleanStatus(status?.syncStatus?.timeSynced, t)}
						tone={status?.syncStatus?.timeSynced ? "normal" : "warning"}
					/>
					<StatusSummaryItem
						icon={<Cpu size={15} />}
						label={t("deceptionStatusOscillator", { ns: "screen" })}
						value={status?.oscillatorState ? t(`deceptionOscillator.${status.oscillatorState}`, { ns: "screen" }) : "-"}
						tone={status?.oscillatorState === "locked" || status?.oscillatorState === "hold" ? "normal" : "warning"}
					/>
					<StatusSummaryItem
						icon={<Thermometer size={15} />}
						label={t("deceptionStatusTemperature", { ns: "screen" })}
						value={formatOptionalNumber(status?.temperatureC, "°C", 1)}
						tone="neutral"
					/>
					<StatusSummaryItem
						icon={<ScanSearch size={15} />}
						label={t("deceptionStatusPseudoSignals", { ns: "screen" })}
						value={formatSignalList(status?.deviceSignal?.signalNames)}
						tone={status?.deviceSignal?.transmitSwitch ? "active" : "neutral"}
					/>
				</div>

				{error || status?.lastError ? (
					<p className="screen-device-status-modal__error">{error || status?.lastError}</p>
				) : null}

				<div className="screen-device-status-modal__sections">
					<StatusSection title={t("deceptionStatusOverview", { ns: "screen" })}>
						<StatusRow label={t("deceptionStatusConnection", { ns: "screen" })} value={status?.serialActive ? t("online", { ns: "screen" }) : t("offline", { ns: "screen" })} />
						<StatusRow label={t("deceptionStatusTransmit", { ns: "screen" })} value={`${formatOnOff((status?.transmitMask ?? 0) > 0, t)} / ${transmitMask}`} />
						<StatusRow label={t("deceptionStatusAmplifier", { ns: "screen" })} value={formatOnOff(status?.amplifierOn, t)} />
						<StatusRow label={t("deceptionStatusAutoTransmit", { ns: "screen" })} value={formatOnOff(status?.autoTransmit, t)} />
						<StatusRow label={t("deceptionStatusTimedSearch", { ns: "screen" })} value={formatOnOff(status?.timedSearch, t)} />
					</StatusSection>

					<StatusSection title={t("deceptionStatusPositioning", { ns: "screen" })}>
						<StatusRow label={t("deceptionStatusCurrentPosition", { ns: "screen" })} value={formatStatusPoint(status?.currentPosition)} />
						<StatusRow label={t("deceptionStatusSimulatedPosition", { ns: "screen" })} value={formatStatusPoint(status?.simulatedPosition)} />
						<StatusRow label={t("deceptionStatusQueriedDevicePosition", { ns: "screen" })} value={formatStatusPoint(status?.queriedDevicePosition)} />
						<StatusRow label={t("deceptionStatusQueriedSimulatedPosition", { ns: "screen" })} value={formatStatusPoint(status?.queriedSimulatedPosition)} />
						<StatusRow label={t("deceptionStatusTargetPosition", { ns: "screen" })} value={formatTargetPosition(status?.targetPosition)} />
						<StatusRow label={t("deceptionStatusReceiverWorking", { ns: "screen" })} value={formatBooleanStatus(status?.syncStatus?.receiverWorking, t)} />
						<StatusRow label={t("deceptionStatusReceiverPositioned", { ns: "screen" })} value={formatBooleanStatus(status?.syncStatus?.receiverPositioned, t)} />
						<StatusRow label={t("deceptionStatusAntenna", { ns: "screen" })} value={formatBooleanStatus(status?.syncStatus?.antennaOk, t)} />
						<StatusRow label={t("deceptionStatusTimeSynced", { ns: "screen" })} value={formatBooleanStatus(status?.syncStatus?.timeSynced, t)} />
					</StatusSection>

					<StatusSection title={t("deceptionStatusSignals", { ns: "screen" })}>
						<StatusRow label={t("deceptionStatusTransmitSignals", { ns: "screen" })} value={`${formatSignalList(status?.transmitSignals)} / ${transmitMask}`} />
						<StatusRow label={t("deceptionStatusSignalMask", { ns: "screen" })} value={deviceSignalMask} />
						<StatusRow label={t("deceptionStatusAttenuation", { ns: "screen" })} value={formatAttenuation(status?.attenuation)} />
						<StatusRow label={t("deceptionStatusDelay", { ns: "screen" })} value={formatDelay(status?.delayBySignalNs, status?.delayNS)} />
						<StatusRow label={t("deceptionStatusSuppression", { ns: "screen" })} value={formatSuppression(status?.suppression, t)} />
					</StatusSection>

					<StatusSection title={t("deceptionStatusPseudoSignals", { ns: "screen" })}>
						<StatusRow label={t("deceptionStatusPseudoSignals", { ns: "screen" })} value={formatSignalList(status?.deviceSignal?.signalNames)} />
						<StatusRow label={t("deceptionStatusPseudoTransmit", { ns: "screen" })} value={formatOnOff(status?.deviceSignal?.transmitSwitch, t)} />
						<StatusRow label={t("deceptionStatusSignalWork", { ns: "screen" })} value={formatSignalWorkStatus(status?.deviceSignal?.workStatus, t)} />
						<StatusRow label={t("deceptionStatusReceivedSatellites", { ns: "screen" })} value={formatSatelliteStatus(status?.deviceSignal?.receivedSatelliteCount, status?.deviceSignal?.receivedPrns)} />
						<StatusRow label={t("deceptionStatusReceivedCn0", { ns: "screen" })} value={formatNumberList(status?.deviceSignal?.receivedCn0)} />
						<StatusRow label={t("deceptionStatusTransmittedSatellites", { ns: "screen" })} value={formatSatelliteStatus(status?.deviceSignal?.transmittedCount, status?.deviceSignal?.transmittedPrns)} />
						<StatusRow label={t("deceptionStatusDeviceSignalDelay", { ns: "screen" })} value={formatOptionalNumber(status?.deviceSignal?.delayNs, "ns", 1)} />
						{deviceSignals.map((signal) => (
							<StatusRow
								key={`${signal.signalMask}-${signal.workStatus.raw}`}
								label={formatSignalList(signal.signalNames)}
								value={formatDeviceSignalDetail(signal, t)}
							/>
						))}
					</StatusSection>

					<StatusSection title={t("deceptionStatusMotion", { ns: "screen" })}>
						<StatusRow label={t("deceptionStatusMaxSpeed", { ns: "screen" })} value={formatOptionalNumber(status?.motion?.maxSpeedMps, "m/s", 1)} />
						<StatusRow label={t("deceptionStatusInitialSpeed", { ns: "screen" })} value={formatOptionalNumber(status?.motion?.initialSpeedMps, "m/s", 1)} />
						<StatusRow label={t("deceptionStatusInitialDirection", { ns: "screen" })} value={formatOptionalNumber(status?.motion?.initialDirectionDeg, "°", 0)} />
						<StatusRow label={t("deceptionStatusAcceleration", { ns: "screen" })} value={formatOptionalNumber(status?.motion?.accelerationMps2, "m/s²", 1)} />
						<StatusRow label={t("deceptionStatusAccelerationDirection", { ns: "screen" })} value={formatOptionalNumber(status?.motion?.accelerationDirectionDeg, "°", 0)} />
						<StatusRow label={t("deceptionStatusCircle", { ns: "screen" })} value={formatCircleMotion(status?.motion, t)} />
						<StatusRow label={t("deceptionStatusSpoofCircle", { ns: "screen" })} value={formatSpoofCircle(status?.spoofCircle, t)} />
						<StatusRow label={t("deceptionStatusRandomPosition", { ns: "screen" })} value={formatRandomPosition(status?.random, t)} />
					</StatusSection>

					<StatusSection title={t("deceptionStatusSystem", { ns: "screen" })}>
						<StatusRow label={t("deceptionStatusSystemTime", { ns: "screen" })} value={formatStatusTime(status?.systemTime)} />
						<StatusRow label={t("deceptionStatusReportedSystemTime", { ns: "screen" })} value={formatStatusTime(status?.reportedSystemTime)} />
						<StatusRow label={t("deceptionStatusVersion", { ns: "screen" })} value={formatVersionStatus(status?.version)} />
						<StatusRow label={t("deceptionStatusTimePrecision", { ns: "screen" })} value={formatOptionalNumber(status?.timePrecisionNs, "ns", 1)} />
						<StatusRow label={t("deceptionStatusUptime", { ns: "screen" })} value={formatDurationSeconds(status?.uptimeSeconds)} />
						<StatusRow label={t("deceptionStatusLeapSecond", { ns: "screen" })} value={formatBooleanStatus(status?.syncStatus?.leapSecondValid, t)} />
						<StatusRow label={t("deceptionStatusFirstTimeSynced", { ns: "screen" })} value={formatBooleanStatus(status?.firstTimeSynced, t)} />
					</StatusSection>
				</div>

				{queryErrorEntries.length > 0 ? (
					<details className="screen-device-status-modal__raw screen-device-status-modal__raw--errors">
						<summary>{t("deceptionStatusQueryErrors", { ns: "screen" })}</summary>
						{queryErrorEntries.map(([key, value]) => (
							<code key={key}>
								<strong>{t(`deceptionStatusRaw.${key}`, { ns: "screen", defaultValue: key })}</strong>
								<span>{value}</span>
							</code>
						))}
					</details>
				) : null}

					<details
						className="screen-device-status-modal__raw"
						open={rawDescriptionsOpen}
						onToggle={(event) => setRawDescriptionsOpen(event.currentTarget.open)}
					>
						<summary>
							<span>{t("deceptionStatusRawDescriptions", { ns: "screen" })}</span>
							<em>{rawEntries.length}</em>
						</summary>
						{rawEntries.length > 0 ? rawEntries.map(([key, value]) => (
							<code key={key}>
								<strong>{t(`deceptionStatusRaw.${key}`, { ns: "screen", defaultValue: key })}</strong>
								<pre>{value}</pre>
							</code>
						)) : <span>{t("noData", { ns: "screen" })}</span>}
					</details>

				<span className={cx("screen-device-status-modal__tone", `screen-device-status-modal__tone--${tone}`)} aria-hidden="true" />
			</section>
		</div>
	);
}

function ClockIcon() {
	return <TimerReset size={15} aria-hidden="true" />;
}

function StatusSummaryItem({
	icon,
	label,
	value,
	tone,
}: {
	icon: ReactNode;
	label: string;
	value: string;
	tone: "normal" | "active" | "warning" | "offline" | "neutral";
}) {
	return (
		<div className={cx("screen-device-status-summary", `screen-device-status-summary--${tone}`)}>
			<span className="screen-device-status-summary__icon" aria-hidden="true">{icon}</span>
			<span>{label}</span>
			<strong>{value}</strong>
		</div>
	);
}

function StatusSection({
	title,
	children,
}: {
	title: string;
	children: ReactNode;
}) {
	return (
		<section className="screen-device-status-section">
			<h3>{title}</h3>
			<div>{children}</div>
		</section>
	);
}

function StatusRow({ label, value }: { label: string; value: string }) {
	return (
		<span className="screen-device-status-row">
			<em>{label}</em>
			<strong>{value}</strong>
		</span>
	);
}

function formatAttenuation(value?: ScreenDeceptionDeviceStatus["attenuation"]) {
	if (!value) {
		return "-";
	}
	return `GPS ${value.gps}dB / BDS ${value.bds}dB / GLO ${value.glo}dB / GAL ${value.gal}dB`;
}

function formatDelay(value: ScreenDeceptionDeviceStatus["delayBySignalNs"], fallback?: number) {
	if (value) {
		const parts = [
			["GPS", value.gps],
			["BDS", value.bds],
			["GLO", value.glo],
			["GAL", value.gal],
		]
			.filter(([, item]) => typeof item === "number" && Number.isFinite(item as number))
			.map(([label, item]) => `${label} ${(item as number).toFixed(1)}ns`);
		if (parts.length > 0) {
			return parts.join(" / ");
		}
	}
	return formatOptionalNumber(fallback, "ns", 1);
}

function formatNumberList(values?: number[]) {
	if (!values || values.length === 0) {
		return "-";
	}
	return values.join(", ");
}

function formatSatelliteStatus(count?: number, prns?: number[]) {
	if (typeof count !== "number" || !Number.isFinite(count)) {
		return "-";
	}
	const list = formatNumberList(prns);
	return list === "-" ? String(count) : `${count} / PRN ${list}`;
}

function formatSignalWorkStatus(
	status: ScreenDeceptionSignalWorkStatus | undefined,
	t: TFunction,
) {
	if (!status) {
		return "-";
	}
	const items = [
		["deceptionSignalWork.clock", status.clockOk],
		["deceptionSignalWork.ephemeris", status.ephemerisValid],
		["deceptionSignalWork.rf", status.rfModuleOk],
		["deceptionSignalWork.transmit", status.signalTransmit],
		["deceptionSignalWork.channel", status.transmitChannel],
		["deceptionSignalWork.fpga", status.fpgaOk],
	] as const;
	const abnormal = items
		.filter(([, ok]) => !ok)
		.map(([key]) => t(key, { ns: "screen" }));
	if (abnormal.length === 0) {
		return t("statusNormal", { ns: "screen" });
	}
	return abnormal.join(" / ");
}

function formatDeviceSignalDetail(signal: ScreenDeceptionDeviceSignalStatus, t: TFunction) {
	const mask = `0x${signal.signalMask.toString(16).toUpperCase().padStart(4, "0")}`;
	return [
		`${t("deceptionStatusSignalMask", { ns: "screen" })} ${mask}`,
		`${t("deceptionStatusPseudoTransmit", { ns: "screen" })} ${formatOnOff(signal.transmitSwitch, t)}`,
		`${t("deceptionStatusSignalWork", { ns: "screen" })} ${formatSignalWorkStatus(signal.workStatus, t)}`,
		`${t("deceptionStatusDeviceSignalDelay", { ns: "screen" })} ${formatOptionalNumber(signal.delayNs, "ns", 1)}`,
		`${t("deceptionStatusAttenuation", { ns: "screen" })} ${signal.attenuationDb}dB`,
	].join(" / ");
}

function formatCircleMotion(motion: ScreenDeceptionDeviceStatus["motion"], t: TFunction) {
	if (!motion) {
		return "-";
	}
	const radius = formatOptionalNumber(motion.circleRadiusM, "m", 1);
	const period = formatOptionalNumber(motion.circlePeriodSeconds, "s", 1);
	const direction = motion.circleDirection
		? t(`deceptionDirections.${motion.circleDirection}`, { ns: "screen", defaultValue: motion.circleDirection })
		: "-";
	return `${radius} / ${period} / ${direction}`;
}

function formatTargetPosition(value?: ScreenDeceptionDeviceStatus["targetPosition"]) {
	if (!value) {
		return "-";
	}
	return `${value.distanceM}m / ${value.heightM}m / ${formatDegrees(value.directionDeg)} / ${formatDegrees(value.headingDeg)}`;
}

function formatSpoofCircle(value: ScreenDeceptionDeviceStatus["spoofCircle"], t: TFunction) {
	if (!value) {
		return "-";
	}
	const direction = value.direction
		? t(`deceptionDirections.${value.direction}`, { ns: "screen", defaultValue: value.direction })
		: "-";
	return `${value.distanceM}m / ${value.heightM}m / ${formatDegrees(value.directionDeg)} / ${formatDegrees(value.headingDeg)} / ${value.radiusM.toFixed(1)}m / ${value.periodSeconds.toFixed(1)}s / ${direction}`;
}

function formatRandomPosition(value: ScreenDeceptionDeviceStatus["random"], t: TFunction) {
	if (!value) {
		return "-";
	}
	return `${formatOnOff(value.enabled, t)} / ${value.radiusM}m / ${value.refreshSeconds}s`;
}

function formatSuppression(value: ScreenDeceptionDeviceStatus["suppression"], t: TFunction) {
	if (!value) {
		return "-";
	}
	const mask = `0x${(value.waveformMask >>> 0).toString(16).toUpperCase().padStart(8, "0")}`;
	return `${formatOnOff(value.transmitOn, t)} / ${mask}`;
}

function formatVersionStatus(value?: ScreenDeceptionDeviceStatus["version"]) {
	if (!value) {
		return "-";
	}
	return `SW ${value.software || "-"} / FPGA ${value.fpga || "-"} / PROTO ${value.protocol || "-"}`;
}

function formatDegrees(value: number | undefined) {
	if (typeof value !== "number" || !Number.isFinite(value)) {
		return "-";
	}
	return `${Math.round(value)}°`;
}

function ScreenStrikePanel({
	state,
	deceptionState,
	deceptionDeviceStatus,
	deceptionDeviceStatusLoading,
  screenStatus,
  deviceLocation,
  now,
  locale,
  userSettings,
  collapsed,
  t,
	onStateChange,
	onDeceptionStateChange,
	onOpenDeceptionStatus,
	onRefreshDeceptionStatus,
	onToggleCollapsed,
}: {
	state: ScreenStrikeState | null;
	deceptionState: ScreenDeceptionState | null;
	deceptionDeviceStatus: ScreenDeceptionDeviceStatus | null;
	deceptionDeviceStatusLoading: boolean;
  screenStatus: ScreenRuntimeStatus | null;
  deviceLocation: ScreenDeviceLocationResponse | null;
  now: Date;
  locale: string;
  userSettings: UserSettings;
  collapsed: boolean;
  t: TFunction;
	onStateChange: (state: ScreenStrikeState) => void;
	onDeceptionStateChange: (state: ScreenDeceptionState) => void;
	onOpenDeceptionStatus: () => void;
	onRefreshDeceptionStatus: () => void;
	onToggleCollapsed: () => void;
}) {
  const [hovered, setHovered] = useState(false);
  const reflectedActiveDeceptionRef = useRef(false);
  const [operationTab, setOperationTab] = useState<ScreenOperationTab>("interference");
  const [selectedChannelIds, setSelectedChannelIds] = useState<string[]>([]);
  const [durationInput, setDurationInput] = useState(String(screenStrikeDefaultDurationSeconds));
  const [deceptionLatitudeInput, setDeceptionLatitudeInput] = useState("");
  const [deceptionLongitudeInput, setDeceptionLongitudeInput] = useState("");
  const [deceptionAltitudeInput, setDeceptionAltitudeInput] = useState("0");
  const [selectedNoFlyZoneId, setSelectedNoFlyZoneId] = useState(manualNoFlyZonePresetId);
  const [deceptionMode, setDeceptionMode] = useState<ScreenDeceptionMode>(screenDeceptionDefaultMode);
  const [circleRadiusInput, setCircleRadiusInput] = useState("100");
  const [circlePeriodInput, setCirclePeriodInput] = useState("60");
  const [circleDirection, setCircleDirection] = useState<"cw" | "ccw">("cw");
  const [linearSpeedInput, setLinearSpeedInput] = useState("10");
  const [linearDirectionInput, setLinearDirectionInput] = useState("");
  const [linearMaxSpeedInput, setLinearMaxSpeedInput] = useState("10");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const channels = state?.channels ?? [];
  const deceptionConfigured = screenStatus?.deception.configured !== false;
  const operationTabs: Array<{ id: ScreenOperationTab; label: string; Icon: typeof RadioTower }> = [
    { id: "interference", label: "strike", Icon: RadioTower },
  ];
  if (deceptionConfigured) {
    operationTabs.push({ id: "deception", label: "deception", Icon: SatelliteDish });
  }
  const strikeChannelLabels = userSettings.screenStrikeChannelLabels ?? [];
  const activeChannelIdsKey = state?.active ? state.channelIds.join("|") : "";
  const remainingSeconds = getStrikeRemainingSeconds(state, now);
  const active = Boolean(state?.active);
  const deceptionActive = Boolean(deceptionState?.active);
  const deceptionDeviceStatusTone = getDeceptionDeviceStatusTone(deceptionDeviceStatus, deceptionDeviceStatusLoading);
  const deceptionLatitudeNumber = parseCoordinateInput(deceptionLatitudeInput);
  const deceptionLongitudeNumber = parseCoordinateInput(deceptionLongitudeInput);
  const deceptionAltitudeNumber = parseCoordinateInput(deceptionAltitudeInput);
  const hasDeceptionCoordinate = validDeceptionCoordinate(deceptionLatitudeNumber, deceptionLongitudeNumber);
  const deceptionAltitudeValid = validDeceptionAltitude(deceptionAltitudeNumber);
  const noFlyZoneDistanceOrigin = getNoFlyZoneDistanceOrigin(deviceLocation, deceptionDeviceStatus);
  const sortedNoFlyZonePresets = getSortedNoFlyZonePresets(noFlyZoneDistanceOrigin);
  const selectedCount = active ? state?.channelIds.length ?? 0 : selectedChannelIds.length;
  const operationTitle = t(operationTab === "interference" ? "strike" : "deception", { ns: "screen" });
  const deceptionOfflineMessage = screenCapabilityOfflineMessage(screenStatus?.deception, t);
  const statusValue = operationTab === "interference"
    ? active ? formatCountdown(remainingSeconds) : selectedCount
    : deceptionActive ? t("active", { ns: "common" }) : (deceptionState?.serialActive ? "OK" : t("offline", { ns: "screen" }));
  const statusActive = operationTab === "interference" ? active : deceptionActive;
  const durationNumber = Number(durationInput);
  const durationValid = Number.isFinite(durationNumber) &&
    durationNumber >= screenStrikeMinDurationSeconds &&
    durationNumber <= screenStrikeMaxDurationSeconds;
  const startDisabled = busy || active || selectedChannelIds.length === 0 || !durationValid;
  const stopDisabled = busy || !active;
  const deceptionNeedsCoordinate = deceptionMode === "fixed_point";
  const deceptionPoint = hasDeceptionCoordinate
    ? { latitude: deceptionLatitudeNumber, longitude: deceptionLongitudeNumber }
    : null;
  const autoDirection = defaultLinearDirection(deviceLocation, deceptionPoint);
  const manualDirection = parsePanelNumber(linearDirectionInput, autoDirection);
  const deceptionStartDisabled = busy ||
    deceptionActive ||
    (deceptionNeedsCoordinate && !hasDeceptionCoordinate) ||
    (deceptionNeedsCoordinate && !deceptionAltitudeValid) ||
    !deceptionState?.serialActive;
  const deceptionStopDisabled = busy || !deceptionActive;
  const deceptionBlockingReasons = [
    !deceptionState?.serialActive ? deceptionOfflineMessage || t("deceptionSerialInactive", { ns: "screen" }) : "",
    deceptionNeedsCoordinate && !hasDeceptionCoordinate ? t("deceptionCoordinateRequired", { ns: "screen" }) : "",
    deceptionNeedsCoordinate && !deceptionAltitudeValid ? t("deceptionAltitudeInvalid", { ns: "screen" }) : "",
  ].filter(Boolean);
  const deceptionDisabledReason = !deceptionActive && deceptionBlockingReasons.length > 0
    ? `${t("deceptionStartBlocked", { ns: "screen" })}: ${deceptionBlockingReasons.join(" / ")}`
    : "";
  const hasDeceptionPanelMessages = Boolean(
    deceptionDisabledReason ||
    deceptionState?.summary ||
    deceptionState?.unsupportedReason ||
    deceptionState?.lastError ||
    error,
  );

  useEffect(() => {
    if (operationTab === "deception" && !deceptionConfigured) {
      setOperationTab("interference");
    }
  }, [deceptionConfigured, operationTab]);

  useEffect(() => {
    if (state?.active) {
      setSelectedChannelIds(state.channelIds);
    }
  }, [activeChannelIdsKey, state?.active]);

  useEffect(() => {
    if (deceptionConfigured && deceptionState?.active) {
      if (!reflectedActiveDeceptionRef.current) {
        setOperationTab("deception");
        reflectedActiveDeceptionRef.current = true;
      }
      return;
    }
    reflectedActiveDeceptionRef.current = false;
  }, [deceptionConfigured, deceptionState?.active]);

  useEffect(() => {
    if (!deceptionState?.active) {
      return;
    }
    if (isScreenDeceptionMode(deceptionState.mode)) {
      setDeceptionMode(deceptionState.mode);
    }
    if (deceptionState.point) {
      setDeceptionLatitudeInput(formatCoordinateValue(deceptionState.point.latitude));
      setDeceptionLongitudeInput(formatCoordinateValue(deceptionState.point.longitude));
    }
    if (typeof deceptionState.altitudeM === "number" && Number.isFinite(deceptionState.altitudeM)) {
      setDeceptionAltitudeInput(String(Math.round(deceptionState.altitudeM)));
    }
    if (deceptionState.circle) {
      if (typeof deceptionState.circle.radiusM === "number" && Number.isFinite(deceptionState.circle.radiusM)) {
        setCircleRadiusInput(String(deceptionState.circle.radiusM));
      }
      if (typeof deceptionState.circle.periodSeconds === "number" && Number.isFinite(deceptionState.circle.periodSeconds)) {
        setCirclePeriodInput(String(deceptionState.circle.periodSeconds));
      }
      if (deceptionState.circle.direction === "cw" || deceptionState.circle.direction === "ccw") {
        setCircleDirection(deceptionState.circle.direction);
      }
    }
    if (deceptionState.linear) {
      if (typeof deceptionState.linear.speedMps === "number" && Number.isFinite(deceptionState.linear.speedMps)) {
        setLinearSpeedInput(String(deceptionState.linear.speedMps));
      }
      if (typeof deceptionState.linear.directionDeg === "number" && Number.isFinite(deceptionState.linear.directionDeg)) {
        setLinearDirectionInput(String(Math.round(deceptionState.linear.directionDeg)));
      }
      if (typeof deceptionState.linear.maxSpeedMps === "number" && Number.isFinite(deceptionState.linear.maxSpeedMps)) {
        setLinearMaxSpeedInput(String(deceptionState.linear.maxSpeedMps));
      }
    }
  }, [deceptionState]);

  useEffect(() => {
    if (linearDirectionInput.trim() === "" && deceptionPoint) {
      setLinearDirectionInput(String(Math.round(autoDirection)));
    }
  }, [autoDirection, deceptionPoint, linearDirectionInput]);

  const selectNoFlyZonePreset = (presetId: string) => {
    setSelectedNoFlyZoneId(presetId);
    if (presetId === manualNoFlyZonePresetId) {
      return;
    }
    const preset = noFlyZonePresets.find((item) => item.id === presetId);
    if (!preset) {
      return;
    }
    setDeceptionLatitudeInput(formatCoordinateValue(preset.latitude));
    setDeceptionLongitudeInput(formatCoordinateValue(preset.longitude));
  };

  const updateManualNoFlyZoneLatitude = (value: string) => {
    setSelectedNoFlyZoneId(manualNoFlyZonePresetId);
    setDeceptionLatitudeInput(normalizeCoordinateInput(value));
  };

  const updateManualNoFlyZoneLongitude = (value: string) => {
    setSelectedNoFlyZoneId(manualNoFlyZonePresetId);
    setDeceptionLongitudeInput(normalizeCoordinateInput(value));
  };

	const toggleChannel = (id: string) => {
    setSelectedChannelIds((items) => (
      items.includes(id) ? items.filter((item) => item !== id) : [...items, id]
    ));
  };

	const submit = async () => {
		setError("");
		setBusy(true);
		try {
			if (active) {
				const response = await updateScreenStrike({ enabled: false, channelIds: [], durationSeconds: 0 }, locale);
				onStateChange(response.state);
				return;
			}
      if (selectedChannelIds.length === 0) {
        setError(t("strikeSelectRequired", { ns: "screen" }));
        return;
      }
      const durationSeconds = clampStrikeDuration(durationNumber);
      setDurationInput(String(durationSeconds));
			const response = await updateScreenStrike({
				enabled: true,
				channelIds: selectedChannelIds,
				durationSeconds,
			}, locale);
			onStateChange(response.state);
		} catch (err) {
			setError(err instanceof Error ? err.message : t("unexpectedError", { ns: "common" }));
    } finally {
      setBusy(false);
    }
  };

	const submitDeception = async () => {
		setError("");
		setBusy(true);
		try {
			if (deceptionActive) {
				const response = await updateScreenDeception({ enabled: false }, locale);
				onDeceptionStateChange(response.state);
				onRefreshDeceptionStatus();
				return;
      }
      if (deceptionNeedsCoordinate && !hasDeceptionCoordinate) {
        setError(t("deceptionCoordinateRequired", { ns: "screen" }));
        return;
      }
      if (deceptionNeedsCoordinate && !deceptionAltitudeValid) {
        setError(t("deceptionAltitudeInvalid", { ns: "screen" }));
        return;
      }
      const altitude = deceptionNeedsCoordinate ? Math.round(deceptionAltitudeNumber) : undefined;
      if (deceptionNeedsCoordinate && altitude !== undefined) {
        setDeceptionLatitudeInput(formatCoordinateValue(deceptionLatitudeNumber));
        setDeceptionLongitudeInput(formatCoordinateValue(deceptionLongitudeNumber));
        setDeceptionAltitudeInput(String(altitude));
      }
      const response = await updateScreenDeception({
        enabled: true,
        mode: deceptionMode,
        longitude: deceptionNeedsCoordinate ? deceptionLongitudeNumber : undefined,
        latitude: deceptionNeedsCoordinate ? deceptionLatitudeNumber : undefined,
        altitudeM: altitude,
        circle: deceptionMode === "circle" ? {
          radiusM: Math.max(1, parsePanelNumber(circleRadiusInput, 100)),
          periodSeconds: Math.max(1, parsePanelNumber(circlePeriodInput, 60)),
          direction: circleDirection,
        } : undefined,
				linear: deceptionMode === "linear" ? {
					speedMps: Math.max(0, parsePanelNumber(linearSpeedInput, 10)),
					directionDeg: normalizeDegrees(manualDirection),
					maxSpeedMps: Math.max(1, parsePanelNumber(linearMaxSpeedInput, 10)),
				} : undefined,
			}, locale);
			onDeceptionStateChange(response.state);
			onRefreshDeceptionStatus();
		} catch (err) {
      setError(err instanceof Error ? err.message : t("unexpectedError", { ns: "common" }));
    } finally {
      setBusy(false);
    }
  };

  return (
    <aside
      className={cx(
        "screen-strike-panel",
        collapsed && "screen-strike-panel--collapsed",
        (collapsed || hovered) && "screen-strike-panel--show-toggle",
      )}
      onMouseEnter={() => setHovered(true)}
      onMouseLeave={() => setHovered(false)}
    >
      <button
        className="screen-side-toggle screen-side-toggle--left"
        type="button"
        aria-label={t(collapsed ? "expandStrikePanel" : "collapseStrikePanel", { ns: "screen" })}
        onClick={onToggleCollapsed}
      >
        {collapsed ? <ChevronRight size={18} /> : <ChevronLeft size={18} />}
        <span aria-hidden="true" />
      </button>

      <div className="screen-strike-panel__inner">
        <div className="screen-strike-panel__header">
          <div className="screen-panel-title">
            <span className="screen-panel-title__icon screen-panel-title__icon--strike">
              <Shield size={15} aria-hidden="true" />
            </span>
            <span className="screen-panel-title__text">
              <em>{t("operationPanel", { ns: "screen" })}</em>
              <strong>{operationTitle}</strong>
            </span>
          </div>
          <strong className={cx("screen-strike-panel__status", statusActive && "screen-strike-panel__status--active")}>
            {statusValue}
          </strong>
        </div>

        <div className="screen-strike-panel__body">
          {operationTab === "interference" ? (
            <>
              <div className="screen-strike-panel__channels" aria-label={t("strikeChannels", { ns: "screen" })}>
                {channels.length > 0 ? channels.map((channel, index) => (
                  <label key={channel.id} className={cx("screen-strike-channel", selectedChannelIds.includes(channel.id) && "screen-strike-channel--checked")}>
                    <input
                      type="checkbox"
                      checked={selectedChannelIds.includes(channel.id)}
                      disabled={active || busy}
                      onChange={() => toggleChannel(channel.id)}
                    />
                    <span aria-hidden="true" />
                    <strong>{formatStrikeChannelLabel(channel, index, strikeChannelLabels)}</strong>
                  </label>
                )) : <EmptyState t={t} compact />}
              </div>

              <div className="screen-strike-duration">
                <span>{t("strikeDuration", { ns: "screen" })}</span>
                <strong>
                  {durationInput}
                  <em>{t("seconds", { ns: "screen" })}</em>
                </strong>
              </div>

              <div className="screen-strike-duration-presets" role="radiogroup" aria-label={t("strikeDuration", { ns: "screen" })}>
                {screenStrikeDurationPresets.map((duration) => {
                  const selected = durationInput === String(duration);

                  return (
                    <button
                      key={duration}
                      className={cx("screen-strike-duration-preset", selected && "screen-strike-duration-preset--active")}
                      type="button"
                      role="radio"
                      aria-checked={selected}
                      aria-label={t("strikeDurationPreset", { ns: "screen", seconds: duration })}
                      disabled={active || busy}
                      onClick={() => setDurationInput(String(duration))}
                    >
                      <span>{duration}</span>
                      <em>{t("seconds", { ns: "screen" })}</em>
                    </button>
                  );
                })}
              </div>

              <div className="screen-strike-panel__footer">
                <button
                  className={cx("screen-strike-action", active && "screen-strike-action--stop")}
                  type="button"
                  disabled={active ? stopDisabled : startDisabled}
                  onClick={() => void submit()}
                >
                  {active ? <Square size={14} /> : <Zap size={15} />}
                  <span>{active ? t("stopStrike", { ns: "screen" }) : t("startStrike", { ns: "screen" })}</span>
                </button>
                <span className="screen-strike-panel__remaining">
                  {t("strikeRemaining", { ns: "screen" })}: <strong>{formatCountdown(remainingSeconds)}</strong>
                </span>
              </div>

              {error ? <p className="screen-strike-panel__error">{error}</p> : null}
            </>
          ) : (
            <>
              {hasDeceptionPanelMessages ? (
                <div className="screen-deception-messages" aria-live="polite">
                  {deceptionDisabledReason ? (
                    <p className="screen-strike-panel__hint">{deceptionDisabledReason}</p>
                  ) : null}
                  {deceptionState?.summary ? (
                    <p className="screen-strike-panel__hint">{deceptionState.summary}</p>
                  ) : null}
                  {deceptionState?.unsupportedReason ? (
                    <p className="screen-strike-panel__error">{deceptionState.unsupportedReason}</p>
                  ) : null}
                  {deceptionState?.lastError ? (
                    <p className="screen-strike-panel__error">{deceptionState.lastError}</p>
                  ) : null}
                  {error ? <p className="screen-strike-panel__error">{error}</p> : null}
                </div>
              ) : null}

              <div className="screen-deception-grid">
                {deceptionMode === "fixed_point" ? (
                  <>
                    <label className="screen-deception-field screen-deception-field--wide">
                      <span>{t("deceptionNoFlyZonePreset", { ns: "screen" })}</span>
                      <select
                        value={selectedNoFlyZoneId}
                        disabled={deceptionActive || busy}
                        onChange={(event) => selectNoFlyZonePreset(event.target.value)}
                      >
                        <option value={manualNoFlyZonePresetId}>{t("deceptionNoFlyZoneManual", { ns: "screen" })}</option>
                        {sortedNoFlyZonePresets.map((preset) => (
                          <option key={preset.id} value={preset.id}>
                            {preset.name}{preset.code ? ` ${preset.code}` : ""} · {formatPresetDistance(preset.distanceM)}
                          </option>
                        ))}
                      </select>
                    </label>
                    <label className="screen-deception-field">
                      <span>{t("latitudeShort", { ns: "screen" })}</span>
                      <input
                        type="text"
                        inputMode="decimal"
                        data-keyboard="numeric"
                        pattern="-?[0-9]*[.,]?[0-9]*"
                        value={deceptionLatitudeInput}
                        placeholder="23.129110"
                        disabled={deceptionActive || busy}
                        onChange={(event) => updateManualNoFlyZoneLatitude(event.target.value)}
                      />
                    </label>
                    <label className="screen-deception-field">
                      <span>{t("longitudeShort", { ns: "screen" })}</span>
                      <input
                        type="text"
                        inputMode="decimal"
                        data-keyboard="numeric"
                        pattern="-?[0-9]*[.,]?[0-9]*"
                        value={deceptionLongitudeInput}
                        placeholder="113.264385"
                        disabled={deceptionActive || busy}
                        onChange={(event) => updateManualNoFlyZoneLongitude(event.target.value)}
                      />
                    </label>
                    <label className="screen-deception-field">
                      <span>{t("deceptionAltitude", { ns: "screen" })}</span>
                      <input
                        type="number"
                        min={screenDeceptionMinAltitudeM}
                        max={screenDeceptionMaxAltitudeM}
                        step={1}
                        value={deceptionAltitudeInput}
                        disabled={deceptionActive || busy}
                        onChange={(event) => setDeceptionAltitudeInput(event.target.value)}
                      />
                    </label>
                  </>
                ) : null}
                {deceptionMode === "circle" ? (
                  <>
                    <label className="screen-deception-field">
                      <span>{t("deceptionCircleRadius", { ns: "screen" })}</span>
                      <input
                        type="number"
                        min={1}
                        step={1}
                        value={circleRadiusInput}
                        disabled={deceptionActive || busy}
                        onChange={(event) => setCircleRadiusInput(event.target.value)}
                      />
                    </label>
                    <label className="screen-deception-field">
                      <span>{t("deceptionCirclePeriod", { ns: "screen" })}</span>
                      <input
                        type="number"
                        min={1}
                        step={1}
                        value={circlePeriodInput}
                        disabled={deceptionActive || busy}
                        onChange={(event) => setCirclePeriodInput(event.target.value)}
                      />
                    </label>
                    <label className="screen-deception-field">
                      <span>{t("deceptionCircleDirection", { ns: "screen" })}</span>
                      <select
                        value={circleDirection}
                        disabled={deceptionActive || busy}
                        onChange={(event) => setCircleDirection(event.target.value as "cw" | "ccw")}
                      >
                        <option value="cw">{t("deceptionDirections.cw", { ns: "screen" })}</option>
                        <option value="ccw">{t("deceptionDirections.ccw", { ns: "screen" })}</option>
                      </select>
                    </label>
                  </>
                ) : null}
                {deceptionMode === "linear" ? (
                  <>
                    <label className="screen-deception-field">
                      <span>{t("deceptionLinearSpeed", { ns: "screen" })}</span>
                      <input
                        type="number"
                        min={0}
                        step={0.1}
                        value={linearSpeedInput}
                        disabled={deceptionActive || busy}
                        onChange={(event) => setLinearSpeedInput(event.target.value)}
                      />
                    </label>
                    <label className="screen-deception-field">
                      <span>{t("deceptionLinearDirection", { ns: "screen" })}</span>
                      <input
                        type="number"
                        min={0}
                        max={359}
                        step={1}
                        value={linearDirectionInput}
                        placeholder={String(Math.round(autoDirection))}
                        disabled={deceptionActive || busy}
                        onChange={(event) => setLinearDirectionInput(event.target.value)}
                      />
                    </label>
                    <label className="screen-deception-field">
                      <span>{t("deceptionLinearMaxSpeed", { ns: "screen" })}</span>
                      <input
                        type="number"
                        min={1}
                        step={0.1}
                        value={linearMaxSpeedInput}
                        disabled={deceptionActive || busy}
                        onChange={(event) => setLinearMaxSpeedInput(event.target.value)}
                      />
                    </label>
                  </>
                ) : null}
              </div>

              <div className="screen-deception-modes" role="radiogroup" aria-label={t("deceptionMode", { ns: "screen" })}>
                {screenDeceptionModeOptions.map(({ id, labelKey, descriptionKey, Icon }) => {
                  const selected = deceptionMode === id;

                  return (
                    <button
                      key={id}
                      className={cx(
                        "screen-deception-mode",
                        selected && "screen-deception-mode--active",
                      )}
                      type="button"
                      role="radio"
                      aria-checked={selected}
                      disabled={deceptionActive || busy}
                      title={t(descriptionKey, { ns: "screen" })}
                      onClick={() => {
                        setError("");
                        setDeceptionMode(id);
                      }}
                    >
                      <Icon size={13} aria-hidden="true" />
                      <span>{t(labelKey, { ns: "screen" })}</span>
                    </button>
                  );
                })}
              </div>

              <div className="screen-strike-panel__footer screen-strike-panel__footer--deception">
                <button
                  className={cx("screen-strike-action", deceptionActive && "screen-strike-action--stop")}
                  type="button"
                  disabled={deceptionActive ? deceptionStopDisabled : deceptionStartDisabled}
                  title={deceptionActive ? undefined : deceptionDisabledReason}
                  onClick={() => void submitDeception()}
                >
                  {deceptionActive ? <Square size={14} /> : <SatelliteDish size={15} />}
                  <span>{deceptionActive ? t("stopDeception", { ns: "screen" }) : t("startDeception", { ns: "screen" })}</span>
                </button>
                <button
                  className={cx("screen-deception-status-button", `screen-deception-status-button--${deceptionDeviceStatusTone}`)}
                  type="button"
                  onClick={onOpenDeceptionStatus}
                >
                  <span aria-hidden="true" />
                  <Activity size={13} aria-hidden="true" />
                  <strong>{t("deceptionDeviceStatus", { ns: "screen" })}</strong>
                  <em>
                    {deceptionDeviceStatusLoading
                      ? t("deceptionStatusRefreshing", { ns: "screen" })
                      : deceptionDeviceStatus?.serialActive
                        ? deceptionDeviceStatus.lastError
                          ? t("statusAbnormal", { ns: "screen" })
                          : t("statusNormal", { ns: "screen" })
                        : t("offline", { ns: "screen" })}
                  </em>
                </button>
              </div>
            </>
          )}
        </div>

        <div className="screen-strike-panel__tabs" role="tablist">
          {operationTabs.map(({ id, label, Icon }) => (
            <button
              key={id}
              className={cx("screen-strike-tab", operationTab === id && "screen-strike-tab--active")}
              type="button"
              role="tab"
              aria-selected={operationTab === id}
              onClick={() => {
                setError("");
                setOperationTab(id);
              }}
            >
              <Icon className="screen-strike-tab__icon" size={13} aria-hidden="true" />
              <span>{t(label, { ns: "screen" })}</span>
            </button>
          ))}
        </div>
      </div>
    </aside>
  );
}

function RightList({
  t,
  selectedId,
  targets,
  positions,
  screenStatus,
  now,
  collapsed,
  onSelectTarget,
  onSelectPosition,
  onOpenNavigationQRCode,
  onToggleCollapsed,
}: {
  t: TFunction;
  selectedId: string;
  targets: ScreenDetectionTarget[];
  positions: ScreenPositionTarget[];
  screenStatus: ScreenRuntimeStatus | null;
  now: Date;
  collapsed: boolean;
  onSelectTarget: (target: ScreenDetectionTarget) => void;
  onSelectPosition: (target: ScreenPositionTarget) => void;
  onOpenNavigationQRCode: (label: string, point: ScreenPositionPoint) => void;
  onToggleCollapsed: () => void;
}) {
  const [tab, setTab] = useState<ScreenAlertKind>("detection");
  const [hovered, setHovered] = useState(false);
  const detectionConfigured = screenStatus?.detection.configured !== false;
  const detectionActive = screenStatus ? screenStatus.detection.active : true;
  const detectionOfflineMessage = screenCapabilityOfflineMessage(screenStatus?.detection, t);
  const availableTabs: ScreenAlertKind[] = detectionConfigured ? ["detection", "position", "fpv"] : [];
  const fpvTargets = targets.filter(isFpvTarget);
  const visibleTargets = tab === "fpv" ? fpvTargets : tab === "detection" ? targets : [];

  const getTabCount = (item: ScreenAlertKind) => {
    if (item === "detection") {
      return targets.length;
    }
    if (item === "fpv") {
      return fpvTargets.length;
    }
    if (item === "position") {
      return positions.length;
    }
    return 0;
  };
  const activeCount = getTabCount(tab);
  const activeLabel = availableTabs.length > 0
    ? t(`tabs.${tab}`, { ns: "screen" })
    : t("targetList", { ns: "screen" });

  if (!detectionConfigured) {
    return null;
  }

  return (
    <aside
      className={cx("screen-right-panel", collapsed && "screen-right-panel--collapsed", (collapsed || hovered) && "screen-right-panel--show-toggle")}
      onMouseEnter={() => setHovered(true)}
      onMouseLeave={() => setHovered(false)}
    >
      <button
        className="screen-side-toggle screen-side-toggle--right"
        type="button"
        aria-label={t(collapsed ? "expandRightList" : "collapseRightList", { ns: "screen" })}
        onClick={onToggleCollapsed}
      >
        {collapsed ? <ChevronLeft size={18} /> : <ChevronRight size={18} />}
        <span aria-hidden="true" />
      </button>

      <div className="screen-info-list">
        <div className="screen-info-list__header">
          <div className="screen-panel-title">
            <span className="screen-panel-title__icon screen-panel-title__icon--target">
              <ScanSearch size={15} aria-hidden="true" />
            </span>
            <span className="screen-panel-title__text">
              <em>{t("targetList", { ns: "screen" })}</em>
              <strong>{activeLabel}</strong>
            </span>
          </div>
          <strong className="screen-info-list__count">{activeCount}</strong>
        </div>

        <div className="screen-list">
          {!detectionActive ? (
            <ScreenOfflineState
              title={t("detectionOfflineTitle", { ns: "screen" })}
              message={t("detectionOfflineMessage", { ns: "screen" })}
              detail={detectionOfflineMessage}
            />
          ) : tab === "fpv" && visibleTargets.length ? (
            <FpvTargetTable
              targets={visibleTargets}
              selectedId={selectedId}
              t={t}
              onSelect={onSelectTarget}
            />
          ) : tab === "position" && positions.length ? (
            positions.map((target) => (
              <PositionTargetCard
                key={target.id}
                target={target}
                selected={selectedId === target.id}
                t={t}
                now={now}
                onSelect={onSelectPosition}
                onOpenNavigationQRCode={onOpenNavigationQRCode}
              />
            ))
          ) : visibleTargets.length ? (
            visibleTargets.map((target) => (
              <DetectionTargetCard
                key={target.id}
                target={target}
                selected={selectedId === target.id}
                t={t}
                now={now}
                onSelect={onSelectTarget}
              />
            ))
          ) : <EmptyState t={t} />}
        </div>

        <div className="screen-tabs" role="tablist">
          {availableTabs.map((item) => {
            const TabIcon = item === "detection" ? Radar : item === "position" ? MapPin : Radio;
            return (
              <button
                key={item}
                className={cx("screen-tab", tab === item && "screen-tab--active")}
                type="button"
                role="tab"
                aria-selected={tab === item}
                onClick={() => setTab(item)}
              >
                <TabIcon className="screen-tab__icon" size={13} aria-hidden="true" />
                <span>{t(`tabs.${item}`, { ns: "screen" })}</span>
                <strong>
                  <span>{getTabCount(item)}</span>
                </strong>
              </button>
            );
          })}
        </div>
      </div>
    </aside>
  );
}

function ScreenFooter() {
  return (
    <footer className="screen-footer">
      <span className="screen-footer-bg" dangerouslySetInnerHTML={{ __html: footerBg }} />
    </footer>
  );
}

function useNow() {
  const [now, setNow] = useState(() => new Date());

  useEffect(() => {
    const timer = window.setInterval(() => setNow(new Date()), 1000);
    return () => window.clearInterval(timer);
  }, []);

  return now;
}

export function ScreenPage({
  appTitle,
  t,
  locale,
  localeOptions,
  visibleMapLayers,
  onLocaleChange,
  userSettings,
}: {
  appTitle: string;
  t: TFunction;
  locale: string;
  localeOptions: string[];
  visibleMapLayers: ReferenceMapLayer[];
  onLocaleChange: (locale: string) => void;
  userSettings: UserSettings;
}) {
  const mapRef = useRef<L.Map | null>(null);
  const [selectedId, setSelectedId] = useState("");
  const [targets, setTargets] = useState<ScreenDetectionTarget[]>([]);
  const [positions, setPositions] = useState<ScreenPositionTarget[]>([]);
  const [deviceLocation, setDeviceLocation] = useState<ScreenDeviceLocationResponse | null>(null);
  const [screenStatus, setScreenStatus] = useState<ScreenRuntimeStatus | null>(null);
  const [strikeState, setStrikeState] = useState<ScreenStrikeState | null>(null);
  const [deceptionState, setDeceptionState] = useState<ScreenDeceptionState | null>(null);
  const [rightCollapsed, setRightCollapsed] = useState(false);
  const [strikeCollapsed, setStrikeCollapsed] = useState(false);
  const [navigationQRCode, setNavigationQRCode] = useState<NavigationQRCodeState | null>(null);
  const [navigationQRCodeLoading, setNavigationQRCodeLoading] = useState(false);
  const [navigationQRCodeError, setNavigationQRCodeError] = useState("");
  const navigationQRCodeRequestRef = useRef(0);
  const [deceptionDeviceStatus, setDeceptionDeviceStatus] = useState<ScreenDeceptionDeviceStatus | null>(null);
  const [deceptionStatusOpen, setDeceptionStatusOpen] = useState(false);
  const [deceptionStatusLoading, setDeceptionStatusLoading] = useState(false);
  const [deceptionStatusError, setDeceptionStatusError] = useState("");
  const deceptionStatusSyncingRef = useRef(false);
  const now = useNow();

  const handleMapReady = useCallback((map: L.Map | null) => {
    mapRef.current = map;
  }, []);

  const handleSelectTarget = useCallback((target: ScreenDetectionTarget) => {
    setSelectedId((current) => (current === target.id ? "" : target.id));
  }, []);

  const handleSelectPosition = useCallback((target: ScreenPositionTarget) => {
    setSelectedId((current) => (current === target.id ? "" : target.id));
    const point = firstPositionMapPoint(target);
    if (point && mapRef.current) {
      mapRef.current.setView([point.latitude, point.longitude], Math.max(mapRef.current.getZoom(), 14), { animate: false });
    }
  }, []);

  const selectedPosition = positions.find((target) => target.id === selectedId) ?? null;

  const enterAdmin = useCallback(() => {
    window.location.hash = "#/settings";
  }, []);

  const updateNavigationQRCode = useCallback(async (
    label: string,
    point: ScreenPositionPoint,
  ) => {
    const coordinates = getNavigationCoordinates(point);
    const requestId = navigationQRCodeRequestRef.current + 1;
    navigationQRCodeRequestRef.current = requestId;
    const nextState = {
      label,
      point: coordinates.original,
      convertedPoint: coordinates.converted,
      items: navigationMapProviders.map((provider) => ({
        provider: provider.id,
        labelKey: provider.label,
        url: buildNavigationUrl(coordinates, provider.id),
        dataUrl: "",
        coordinate: provider.id === "google" ? coordinates.original : coordinates.converted,
        coordinateSystem: provider.id === "google" ? "WGS84" : "GCJ-02",
        coordinateLabelKey: provider.id === "google" ? "navigationCoordinateOriginal" : "navigationCoordinateConverted",
      })),
    } satisfies NavigationQRCodeState;

    setNavigationQRCode(nextState);
    setNavigationQRCodeLoading(true);
    setNavigationQRCodeError("");

    try {
      const nextQRCode = await createNavigationQRCodes(label, point);
      if (navigationQRCodeRequestRef.current !== requestId) {
        return;
      }
      setNavigationQRCode(nextQRCode);
    } catch (error) {
      if (navigationQRCodeRequestRef.current !== requestId) {
        return;
      }
      console.error(error);
      setNavigationQRCodeError(t("generateNavigationQRCodeFailed", { ns: "screen" }));
    } finally {
      if (navigationQRCodeRequestRef.current === requestId) {
        setNavigationQRCodeLoading(false);
      }
    }
  }, [t]);

  const handleOpenNavigationQRCode = useCallback((label: string, point: ScreenPositionPoint) => {
    void updateNavigationQRCode(label, point);
  }, [updateNavigationQRCode]);

  const handleCloseNavigationQRCode = useCallback(() => {
    navigationQRCodeRequestRef.current += 1;
    setNavigationQRCode(null);
    setNavigationQRCodeLoading(false);
    setNavigationQRCodeError("");
  }, []);

  const syncDeceptionDeviceStatus = useCallback(async () => {
    if (deceptionStatusSyncingRef.current) {
      return;
    }
    deceptionStatusSyncingRef.current = true;
    setDeceptionStatusLoading(true);
    try {
      const response = await getScreenDeceptionStatus(locale);
      setDeceptionDeviceStatus(response);
      setDeceptionStatusError("");
    } catch (err) {
      setDeceptionStatusError(err instanceof Error ? err.message : t("unexpectedError", { ns: "common" }));
    } finally {
      deceptionStatusSyncingRef.current = false;
      setDeceptionStatusLoading(false);
    }
  }, [locale, t]);

  const handleOpenDeceptionStatus = useCallback(() => {
    setDeceptionStatusOpen(true);
    void syncDeceptionDeviceStatus();
  }, [syncDeceptionDeviceStatus]);

  const handleCloseDeceptionStatus = useCallback(() => {
    setDeceptionStatusOpen(false);
  }, []);

  useEffect(() => {
    return openScreenStream({
      onDetectionUpdated: (event) => {
        if (!event.payload) {
          return;
        }
        const payload = event.payload;
        setTargets((items) => mergeScreenTarget(items, payload, screenDetectionLimit));
      },
      onPositionUpdated: (event) => {
        if (!event.payload) {
          return;
        }
        const payload = event.payload;
        setPositions((items) => mergeScreenPosition(items, payload, screenPositionLimit));
      },
      onStrikeUpdated: (event) => {
        if (!event.payload) {
          return;
        }
        setStrikeState(event.payload);
      },
      onDeceptionUpdated: (event) => {
        if (!event.payload) {
          return;
        }
        setDeceptionState(event.payload);
      },
    });
  }, []);

  useEffect(() => {
    let cancelled = false;
    let syncing = false;

    const syncScreenStatus = async () => {
      if (syncing) {
        return;
      }
      syncing = true;
      try {
        const response = await getScreenStatus(locale);
        if (!cancelled) {
          setScreenStatus(response);
        }
      } catch {
        // Keep the last visible runtime status during a transient polling failure.
      } finally {
        syncing = false;
      }
    };

    void syncScreenStatus();
    const timer = window.setInterval(() => {
      void syncScreenStatus();
    }, 3000);
    return () => {
      cancelled = true;
      window.clearInterval(timer);
    };
  }, [locale]);

  useEffect(() => {
    let cancelled = false;
    let syncing = false;

    const syncTargets = async () => {
      if (syncing) {
        return;
      }
      syncing = true;
      try {
        const response = await getScreenDetections(screenDetectionLimit);
        if (!cancelled) {
          setTargets(sortScreenTargets(response.items).slice(0, screenDetectionLimit));
        }
      } catch {
        // Keep the last visible detections during a transient polling failure.
      } finally {
        syncing = false;
      }
    };

    void syncTargets();
    const timer = window.setInterval(() => {
      void syncTargets();
    }, 5000);
    return () => {
      cancelled = true;
      window.clearInterval(timer);
    };
  }, []);

  useEffect(() => {
    let cancelled = false;
    let syncing = false;

    const syncPositions = async () => {
      if (syncing) {
        return;
      }
      syncing = true;
      try {
        const response = await getScreenPositions(screenPositionLimit);
        if (!cancelled) {
          setPositions(sortScreenPositions(response.items).slice(0, screenPositionLimit));
        }
      } catch {
        // Keep the last visible positions during a transient polling failure.
      } finally {
        syncing = false;
      }
    };

    void syncPositions();
    const timer = window.setInterval(() => {
      void syncPositions();
    }, 5000);
    return () => {
      cancelled = true;
      window.clearInterval(timer);
    };
  }, []);

  useEffect(() => {
    let cancelled = false;
    let syncing = false;

    const syncDeviceLocation = async () => {
      if (syncing) {
        return;
      }
      syncing = true;
      try {
        const response = await getScreenDeviceLocation();
        if (!cancelled) {
          setDeviceLocation(response);
        }
      } catch {
        // Keep the last visible device location during a transient polling failure.
      } finally {
        syncing = false;
      }
    };

    void syncDeviceLocation();
    const timer = window.setInterval(() => {
      void syncDeviceLocation();
    }, 5000);
    return () => {
      cancelled = true;
      window.clearInterval(timer);
    };
  }, []);

  useEffect(() => {
    let cancelled = false;
    let syncing = false;

    const syncStrike = async () => {
      if (syncing) {
        return;
      }
      syncing = true;
      try {
        const response = await getScreenStrike(locale);
        if (!cancelled) {
          setStrikeState(response);
        }
      } catch {
        // Keep the last visible strike state during a transient polling failure.
      } finally {
        syncing = false;
      }
    };

    void syncStrike();
    const timer = window.setInterval(() => {
      void syncStrike();
    }, 1000);
    return () => {
      cancelled = true;
      window.clearInterval(timer);
    };
  }, [locale]);

  useEffect(() => {
    let cancelled = false;
    let syncing = false;

    const syncDeception = async () => {
      if (syncing) {
        return;
      }
      syncing = true;
      try {
        const response = await getScreenDeception(locale);
        if (!cancelled) {
          setDeceptionState(response);
        }
      } catch {
        // Keep the last visible deception state during a transient polling failure.
      } finally {
        syncing = false;
      }
    };

    void syncDeception();
    const timer = window.setInterval(() => {
      void syncDeception();
    }, 1000);
    return () => {
      cancelled = true;
      window.clearInterval(timer);
    };
  }, [locale]);

  useEffect(() => {
    if (!deceptionStatusOpen) {
      return;
    }
    void syncDeceptionDeviceStatus();
    const timer = window.setInterval(() => {
      void syncDeceptionDeviceStatus();
    }, 2000);
    return () => window.clearInterval(timer);
  }, [deceptionStatusOpen, syncDeceptionDeviceStatus]);

  useEffect(() => {
    if (screenStatus?.detection.configured === false) {
      setTargets([]);
      setPositions([]);
      setSelectedId("");
    }
  }, [screenStatus?.detection.configured]);

  useEffect(() => {
    window.setTimeout(() => mapRef.current?.invalidateSize(), 350);
  }, [rightCollapsed, screenStatus?.detection.configured]);

  useEffect(() => {
    if (!navigationQRCode) {
      return;
    }
    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape") {
        handleCloseNavigationQRCode();
      }
    };
    window.addEventListener("keydown", handleKeyDown);
    return () => window.removeEventListener("keydown", handleKeyDown);
  }, [handleCloseNavigationQRCode, navigationQRCode]);

  useEffect(() => {
    if (!deceptionStatusOpen) {
      return;
    }
    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape") {
        handleCloseDeceptionStatus();
      }
    };
    window.addEventListener("keydown", handleKeyDown);
    return () => window.removeEventListener("keydown", handleKeyDown);
  }, [deceptionStatusOpen, handleCloseDeceptionStatus]);

  return (
    <section className="screen-shell">
      <ScreenMap
        t={t}
        selectedId={selectedId}
        positions={screenStatus?.detection.configured === false ? [] : positions}
        deviceLocation={deviceLocation}
        visibleMapLayers={visibleMapLayers}
        onSelectPosition={handleSelectPosition}
        onMapReady={handleMapReady}
      />

      <ScreenHeader
        appTitle={appTitle}
        t={t}
        now={now}
        locale={locale}
        localeOptions={localeOptions}
        onLocaleChange={onLocaleChange}
        onEnterAdmin={enterAdmin}
      />

      <ScreenStrikePanel
        state={strikeState}
        deceptionState={deceptionState}
        deceptionDeviceStatus={deceptionDeviceStatus}
        deceptionDeviceStatusLoading={deceptionStatusLoading}
        screenStatus={screenStatus}
        deviceLocation={deviceLocation}
        now={now}
        locale={locale}
        userSettings={userSettings}
        collapsed={strikeCollapsed}
        t={t}
        onStateChange={setStrikeState}
        onDeceptionStateChange={setDeceptionState}
        onOpenDeceptionStatus={handleOpenDeceptionStatus}
        onRefreshDeceptionStatus={syncDeceptionDeviceStatus}
        onToggleCollapsed={() => setStrikeCollapsed((value) => !value)}
      />

      <RightList
        t={t}
        selectedId={selectedId}
        targets={targets}
        positions={positions}
        screenStatus={screenStatus}
        now={now}
        collapsed={rightCollapsed}
        onSelectTarget={handleSelectTarget}
        onSelectPosition={handleSelectPosition}
        onOpenNavigationQRCode={handleOpenNavigationQRCode}
        onToggleCollapsed={() => setRightCollapsed((value) => !value)}
      />

      <ScreenFooter />

      <NavigationQRCodeModal
        state={navigationQRCode}
        loading={navigationQRCodeLoading}
        error={navigationQRCodeError}
        t={t}
        onClose={handleCloseNavigationQRCode}
      />
      {deceptionStatusOpen ? (
        <DeceptionDeviceStatusModal
          status={deceptionDeviceStatus}
          loading={deceptionStatusLoading}
          error={deceptionStatusError}
          t={t}
          onRefresh={() => void syncDeceptionDeviceStatus()}
          onClose={handleCloseDeceptionStatus}
        />
      ) : null}
    </section>
  );
}
