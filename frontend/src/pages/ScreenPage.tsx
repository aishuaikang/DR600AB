import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import type { TFunction } from "i18next";
import type L from "leaflet";
import { ChevronDown, ChevronLeft, ChevronRight, ChevronUp, Globe2, Settings2 } from "lucide-react";

import { cx } from "../utils/classnames";
import footerBg from "../assets/images/screen/footerBg.svg?raw";
import headerBg from "../assets/images/screen/headerBg.svg?raw";
import mini2Image from "../assets/images/uav/mini2.png";
import i18n from "../i18n";
import { compactLocaleName } from "../utils/locales";
import { ScreenMap } from "./ScreenMap";
import { screenAlerts, type ScreenAlert, type ScreenAlertKind } from "./screenData";

function formatBearing(value: number) {
  return `${Math.round(value)}°`;
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

function AlertCard({
  alert,
  selected,
  t,
  onSelect,
}: {
  alert: ScreenAlert;
  selected: boolean;
  t: TFunction;
  onSelect: (alert: ScreenAlert) => void;
}) {
  const [collapsed, setCollapsed] = useState(false);

  return (
    <article
      className={cx("screen-alert-card", selected && "screen-alert-card--selected", alert.kind === "fpv" && "screen-alert-card--fpv")}
      onClick={() => onSelect(alert)}
    >
      <div className="screen-alert-card__main">
        <span className="screen-alert-card__icon">
          <img src={mini2Image} alt="" />
          <span className="screen-alert-card__glow" />
        </span>

        <span className="screen-alert-card__body">
          <strong>{t(alert.nameKey, { ns: "screen" })}</strong>
          <span className="screen-alert-card__meta">
            {alert.frequencyMHz}MHz / {formatBearing(alert.orientation)}
          </span>
          <span className="screen-alert-card__sn">{alert.sn}</span>
        </span>

        <button
          className="screen-alert-card__collapse"
          type="button"
          aria-label={t(collapsed ? "expandAlert" : "collapseAlert", { ns: "screen" })}
          onClick={(event) => {
            event.stopPropagation();
            setCollapsed((value) => !value);
          }}
        >
          {collapsed ? <ChevronDown size={16} /> : <ChevronUp size={16} />}
        </button>
      </div>

      {!collapsed ? (
        <div className="screen-alert-card__details">
          <span>
            <em>{t("deviceSn", { ns: "screen" })}: </em>
            {alert.sn}
          </span>
          <span>
            <em>{t("signalStrength", { ns: "screen" })}: </em>
            {alert.rssi}dBm
          </span>
          <span>
            <em>{t("time", { ns: "screen" })}: </em>
            {alert.time}
          </span>
        </div>
      ) : null}
    </article>
  );
}

function RightList({
  t,
  selectedId,
  collapsed,
  onSelectAlert,
  onToggleCollapsed,
}: {
  t: TFunction;
  selectedId: string;
  collapsed: boolean;
  onSelectAlert: (alert: ScreenAlert) => void;
  onToggleCollapsed: () => void;
}) {
  const [tab, setTab] = useState<ScreenAlertKind>("detection");
  const [hovered, setHovered] = useState(false);
  const filtered = screenAlerts.filter((alert) => alert.kind === tab);

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
                <span>{screenAlerts.filter((alert) => alert.kind === item).length}</span>
              </strong>
            </button>
          ))}
        </div>

        <div className="screen-list">
          {filtered.length ? (
            filtered.map((alert) => (
              <AlertCard
                key={alert.id}
                alert={alert}
                selected={selectedId === alert.id}
                t={t}
                onSelect={onSelectAlert}
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
  const [rightCollapsed, setRightCollapsed] = useState(false);
  const now = useNow();

  const selectedAlert = useMemo(() => screenAlerts.find((alert) => alert.id === selectedId), [selectedId]);

  const handleMapReady = useCallback((map: L.Map | null) => {
    mapRef.current = map;
  }, []);

  const handleSelectAlert = useCallback((alert: ScreenAlert) => {
    setSelectedId((current) => (current === alert.id ? "" : alert.id));
  }, []);

  const enterAdmin = useCallback(() => {
    window.location.hash = "#/settings";
  }, []);

  useEffect(() => {
    window.setTimeout(() => mapRef.current?.invalidateSize(), 350);
  }, [rightCollapsed]);

  useEffect(() => {
    if (!selectedAlert || !mapRef.current) {
      return;
    }

    mapRef.current.setView([selectedAlert.lat, selectedAlert.lng], Math.max(mapRef.current.getZoom(), 14), { animate: false });
  }, [selectedAlert]);

  return (
    <section className="screen-shell">
      <ScreenMap t={t} selectedId={selectedId} onSelectAlert={handleSelectAlert} onMapReady={handleMapReady} />

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
        collapsed={rightCollapsed}
        onSelectAlert={handleSelectAlert}
        onToggleCollapsed={() => setRightCollapsed((value) => !value)}
      />

      <ScreenFooter />
    </section>
  );
}
