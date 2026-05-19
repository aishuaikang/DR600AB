import { useCallback, useEffect, useRef, useState } from "react";
import type { TFunction } from "i18next";
import type L from "leaflet";
import { ChevronDown, ChevronLeft, ChevronRight, Globe2, Inbox, Loader2, MapPin, QrCode, Radar, Radio, RadioTower, SatelliteDish, Settings2, Square, X, Zap } from "lucide-react";
import * as QRCode from "qrcode";

import {
  getScreenDetections,
  getScreenDeviceLocation,
  getScreenPositions,
  getScreenStrike,
  openScreenStream,
  updateScreenStrike,
} from "../api";
import type {
  ScreenDetectionTarget,
  ScreenDeviceLocationResponse,
  ScreenPositionPoint,
  ScreenPositionTarget,
  ScreenStrikeChannel,
  ScreenStrikeState,
  UserSettings,
} from "../types";
import { cx } from "../utils/classnames";
import footerBg from "../assets/images/screen/footerBg.svg?raw";
import headerBg from "../assets/images/screen/headerBg.svg?raw";
import mini2Image from "../assets/images/uav/mini2.png";
import i18n from "../i18n";
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

function formatDevice(device?: string) {
  const value = device?.trim();
  if (!value) {
    return "-";
  }
  return value;
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

        <span className="screen-detection-card__device">
          <em>{t("device", { ns: "screen" })}</em>
          <strong>{formatDevice(target.device)}</strong>
        </span>

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

function ScreenStrikePanel({
  state,
  now,
  locale,
  userSettings,
  collapsed,
  t,
  onStateChange,
  onToggleCollapsed,
}: {
  state: ScreenStrikeState | null;
  now: Date;
  locale: string;
  userSettings: UserSettings;
  collapsed: boolean;
  t: TFunction;
  onStateChange: (state: ScreenStrikeState) => void;
  onToggleCollapsed: () => void;
}) {
  const [hovered, setHovered] = useState(false);
  const [operationTab, setOperationTab] = useState<ScreenOperationTab>("interference");
  const [selectedChannelIds, setSelectedChannelIds] = useState<string[]>([]);
  const [durationInput, setDurationInput] = useState(String(screenStrikeDefaultDurationSeconds));
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const channels = state?.channels ?? [];
  const strikeChannelLabels = userSettings.screenStrikeChannelLabels ?? [];
  const activeChannelIdsKey = state?.active ? state.channelIds.join("|") : "";
  const remainingSeconds = getStrikeRemainingSeconds(state, now);
  const active = Boolean(state?.active);
  const selectedCount = active ? state?.channelIds.length ?? 0 : selectedChannelIds.length;
  const operationTitle = t(operationTab === "interference" ? "strike" : "deception", { ns: "screen" });
  const statusValue = operationTab === "interference"
    ? active ? formatCountdown(remainingSeconds) : selectedCount
    : 0;
  const statusActive = operationTab === "interference" && active;
  const durationNumber = Number(durationInput);
  const durationValid = Number.isFinite(durationNumber) &&
    durationNumber >= screenStrikeMinDurationSeconds &&
    durationNumber <= screenStrikeMaxDurationSeconds;
  const startDisabled = busy || active || selectedChannelIds.length === 0 || !durationValid;
  const stopDisabled = busy || !active;

  useEffect(() => {
    if (state?.active) {
      setSelectedChannelIds(state.channelIds);
    }
  }, [activeChannelIdsKey, state?.active]);

  const toggleChannel = (id: string) => {
    setSelectedChannelIds((items) => (
      items.includes(id) ? items.filter((item) => item !== id) : [...items, id]
    ));
  };

  const submit = async () => {
    setError("");
    setBusy(true);
    try {
      const developerToken = readDeveloperSession()?.token ?? "";
      if (active) {
        const response = await updateScreenStrike({ enabled: false, channelIds: [], durationSeconds: 0 }, locale, developerToken);
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
      }, locale, developerToken);
      onStateChange(response.state);
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
          <span>
            <em>{t("operationPanel", { ns: "screen" })}</em>
            <strong>{operationTitle}</strong>
          </span>
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
          ) : <EmptyState t={t} />}
        </div>

        <div className="screen-strike-panel__tabs" role="tablist">
          {([
            { id: "interference", label: "strike", Icon: RadioTower },
            { id: "deception", label: "deception", Icon: SatelliteDish },
          ] as const).map(({ id, label, Icon }) => (
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
  now: Date;
  collapsed: boolean;
  onSelectTarget: (target: ScreenDetectionTarget) => void;
  onSelectPosition: (target: ScreenPositionTarget) => void;
  onOpenNavigationQRCode: (label: string, point: ScreenPositionPoint) => void;
  onToggleCollapsed: () => void;
}) {
  const [tab, setTab] = useState<ScreenAlertKind>("detection");
  const [hovered, setHovered] = useState(false);
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
  const activeLabel = t(`tabs.${tab}`, { ns: "screen" });

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
          <span>
            <em>{t("targetList", { ns: "screen" })}</em>
            <strong>{activeLabel}</strong>
          </span>
          <strong className="screen-info-list__count">{activeCount}</strong>
        </div>

        <div className="screen-list">
          {tab === "fpv" && visibleTargets.length ? (
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
          {(["detection", "position", "fpv"] as ScreenAlertKind[]).map((item) => {
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
  const [strikeState, setStrikeState] = useState<ScreenStrikeState | null>(null);
  const [rightCollapsed, setRightCollapsed] = useState(false);
  const [strikeCollapsed, setStrikeCollapsed] = useState(false);
  const [navigationQRCode, setNavigationQRCode] = useState<NavigationQRCodeState | null>(null);
  const [navigationQRCodeLoading, setNavigationQRCodeLoading] = useState(false);
  const [navigationQRCodeError, setNavigationQRCodeError] = useState("");
  const navigationQRCodeRequestRef = useRef(0);
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
    });
  }, []);

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
    window.setTimeout(() => mapRef.current?.invalidateSize(), 350);
  }, [rightCollapsed]);

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

  return (
    <section className="screen-shell">
      <ScreenMap
        t={t}
        selectedId={selectedId}
        positions={positions}
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
        now={now}
        locale={locale}
        userSettings={userSettings}
        collapsed={strikeCollapsed}
        t={t}
        onStateChange={setStrikeState}
        onToggleCollapsed={() => setStrikeCollapsed((value) => !value)}
      />

      <RightList
        t={t}
        selectedId={selectedId}
        targets={targets}
        positions={positions}
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
    </section>
  );
}
