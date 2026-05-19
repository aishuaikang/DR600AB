import { useCallback, useEffect, useRef, useState } from "react";
import type { TFunction } from "i18next";
import type L from "leaflet";
import { ChevronDown, ChevronLeft, ChevronRight, Globe2, Settings2 } from "lucide-react";

import { getScreenDetections, getScreenDeviceLocation, getScreenPositions, openScreenStream } from "../api";
import type {
  ScreenDetectionTarget,
  ScreenDeviceLocationResponse,
  ScreenPositionPoint,
  ScreenPositionTarget,
} from "../types";
import { cx } from "../utils/classnames";
import footerBg from "../assets/images/screen/footerBg.svg?raw";
import headerBg from "../assets/images/screen/headerBg.svg?raw";
import mini2Image from "../assets/images/uav/mini2.png";
import i18n from "../i18n";
import { compactLocaleName } from "../utils/locales";
import { ScreenMap } from "./ScreenMap";
import type { ScreenAlertKind } from "./screenData";

const screenDetectionLimit = 100;
const screenPositionLimit = 100;
const screenDetectionFreshMs = 15_000;
const screenDetectionStaleMs = 40_000;
const droneImageModules = import.meta.glob("../assets/images/drone/*.png", {
  eager: true,
  query: "?url",
  import: "default",
}) as Record<string, string>;

function getDroneImageUrl(model: string) {
  if (!model) {
    return mini2Image;
  }
  return droneImageModules[`../assets/images/drone/${model}.png`] ?? mini2Image;
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

function formatOptionalNumber(value: number | undefined, unit: string, digits = 0) {
  if (typeof value !== "number" || !Number.isFinite(value)) {
    return "-";
  }
  return `${value.toFixed(digits)}${unit}`;
}

function formatPositionPoint(point?: ScreenPositionPoint) {
  if (!point || !Number.isFinite(point.latitude) || !Number.isFinite(point.longitude)) {
    return "-";
  }
  return `${point.latitude.toFixed(6)}, ${point.longitude.toFixed(6)}`;
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

function positionSourceLabel(source: string, t: TFunction) {
  return t(`positionSource.${source || "unknown"}`, { ns: "screen", defaultValue: source || "-" });
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

        <span className="screen-detection-card__primary">
          <strong>{formatFrequency(target.frequency)}</strong>
          <em>{formatRSSI(target.rssi)}</em>
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

function PositionTargetCard({
  target,
  selected,
  t,
  now,
  onSelect,
}: {
  target: ScreenPositionTarget;
  selected: boolean;
  t: TFunction;
  now: Date;
  onSelect: (target: ScreenPositionTarget) => void;
}) {
  const timeTone = getTargetTimeTone(target.lastSeen, now);
  const timeToneTitle = getTargetTimeToneTitle(timeTone, t);

  return (
    <article
      className={cx("screen-position-card", selected && "screen-position-card--selected")}
      onClick={() => onSelect(target)}
    >
      <div className="screen-position-card__head">
        <span>
          <strong>{target.model || t("unknownTarget", { ns: "screen" })}</strong>
          <em>{target.serial || "-"}</em>
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

      <div className="screen-position-card__grid">
        <span>
          <em>{t("positionDrone", { ns: "screen" })}</em>
          <strong>{formatPositionPoint(target.drone)}</strong>
        </span>
        <span>
          <em>{target.pilot ? t("positionPilot", { ns: "screen" }) : t("positionHome", { ns: "screen" })}</em>
          <strong>{formatPositionPoint(target.pilot ?? target.home)}</strong>
        </span>
      </div>

      <div className="screen-position-card__meta">
        <span>{positionSourceLabel(target.source, t)}</span>
        <span>{formatOptionalNumber(target.frequency, "MHz", 1)}</span>
        <span>{formatOptionalNumber(target.rssi, "dBm", 0)}</span>
        <span>{formatOptionalNumber(target.height, "m", 0)}</span>
      </div>
    </article>
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
        <div className="screen-tabs" role="tablist">
          {(["detection", "position", "fpv"] as ScreenAlertKind[]).map((item) => (
            <button
              key={item}
              className={cx("screen-tab", tab === item && "screen-tab--active")}
              type="button"
              role="tab"
              aria-selected={tab === item}
              onClick={() => setTab(item)}
            >
              <span>{t(`tabs.${item}`, { ns: "screen" })}</span>
              <strong>
                <span>{getTabCount(item)}</span>
              </strong>
            </button>
          ))}
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
          ) : (
            <p className="screen-empty">{t("noData", { ns: "screen" })}</p>
          )}
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
  onLocaleChange,
}: {
  appTitle: string;
  t: TFunction;
  locale: string;
  localeOptions: string[];
  onLocaleChange: (locale: string) => void;
}) {
  const mapRef = useRef<L.Map | null>(null);
  const [selectedId, setSelectedId] = useState("");
  const [targets, setTargets] = useState<ScreenDetectionTarget[]>([]);
  const [positions, setPositions] = useState<ScreenPositionTarget[]>([]);
  const [deviceLocation, setDeviceLocation] = useState<ScreenDeviceLocationResponse | null>(null);
  const [rightCollapsed, setRightCollapsed] = useState(false);
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
    window.setTimeout(() => mapRef.current?.invalidateSize(), 350);
  }, [rightCollapsed]);

  return (
    <section className="screen-shell">
      <ScreenMap
        t={t}
        selectedId={selectedId}
        positions={positions}
        deviceLocation={deviceLocation}
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

      <RightList
        t={t}
        selectedId={selectedId}
        targets={targets}
        positions={positions}
        now={now}
        collapsed={rightCollapsed}
        onSelectTarget={handleSelectTarget}
        onSelectPosition={handleSelectPosition}
        onToggleCollapsed={() => setRightCollapsed((value) => !value)}
      />

      <ScreenFooter />
    </section>
  );
}
