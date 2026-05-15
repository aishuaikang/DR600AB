import type { TFunction } from "i18next";
import { Check, Globe2, RefreshCw, Satellite } from "lucide-react";

import { BannerAlert } from "../components/BannerAlert";
import { InfoTile } from "../components/InfoTile";
import { Panel, PanelBody } from "../components/Panel";
import { PortSelect } from "../components/PortSelect";
import { SectionHeader } from "../components/SectionHeader";
import type { Banner } from "../app/types";
import type { GPSSessionResponse, PortInfo } from "../types";
import { cx } from "../utils/classnames";
import { fullLocaleName } from "../utils/locales";

export function SettingsPage({
  banner,
  ports,
  selectedReceivePort,
  selectedSendPort,
  selectedGPSDataPort,
  selectedGPSControlPort,
  sessionStateLabel,
  currentReceivePort,
  currentSendPort,
  gpsBanner,
  gpsSession,
  gpsSessionStateLabel,
  currentGPSDataPort,
  currentGPSControlPort,
  allLocaleOptions,
  visibleLocales,
  currentLocale,
  t,
  onRefresh,
  onReceivePortChange,
  onSendPortChange,
  onGPSDataPortChange,
  onGPSControlPortChange,
  onVisibleLocalesChange,
}: {
  banner: Banner;
  ports: PortInfo[];
  selectedReceivePort: string;
  selectedSendPort: string;
  selectedGPSDataPort: string;
  selectedGPSControlPort: string;
  sessionStateLabel: string;
  currentReceivePort: string;
  currentSendPort: string;
  gpsBanner: Banner;
  gpsSession: GPSSessionResponse | null;
  gpsSessionStateLabel: string;
  currentGPSDataPort: string;
  currentGPSControlPort: string;
  allLocaleOptions: string[];
  visibleLocales: string[];
  currentLocale: string;
  t: TFunction;
  onRefresh: () => void;
  onReceivePortChange: (value: string) => void;
  onSendPortChange: (value: string) => void;
  onGPSDataPortChange: (value: string) => void;
  onGPSControlPortChange: (value: string) => void;
  onVisibleLocalesChange: (locales: string[]) => void;
}) {
  const visibleLocaleSet = new Set(visibleLocales);

  const handleToggleLocale = (locale: string) => {
    if (locale === currentLocale) {
      return;
    }
    const next = visibleLocaleSet.has(locale)
      ? visibleLocales.filter((item) => item !== locale)
      : [...visibleLocales, locale];
    onVisibleLocalesChange(next);
  };

  return (
    <section className="grid gap-3">
      <Panel>
        <PanelBody>
          <SectionHeader
            title={t("detectionSerialRuntimeTitle", { ns: "settings" })}
            description={t("sessionHint", { ns: "detection" })}
          />

          <div className="grid gap-3 xl:grid-cols-[minmax(0,1fr)_auto]">
            <div className="grid gap-3 md:grid-cols-3">
              <InfoTile label={t("sessionTitle", { ns: "detection" })}>
                {sessionStateLabel}
              </InfoTile>
              <InfoTile label={t("receivePort", { ns: "detection" })} value={currentReceivePort || t("unknown", { ns: "common" })} />
              <InfoTile label={t("sendPort", { ns: "detection" })} value={currentSendPort || t("unknown", { ns: "common" })} />
            </div>
          </div>
          {banner.kind === "error" ? <BannerAlert banner={banner} /> : null}
        </PanelBody>
      </Panel>

      <Panel>
        <PanelBody>
          <SectionHeader
            title={t("detectionSerialTitle", { ns: "settings" })}
            description={t("detectionSerialDescription", { ns: "settings" })}
            action={
              <button className="btn btn-sm btn-outline btn-info" type="button" onClick={onRefresh}>
                <RefreshCw size={16} />
                <span>{t("refresh", { ns: "common" })}</span>
              </button>
            }
          />

          <div className="grid gap-3 md:grid-cols-2">
            <PortSelect
              label={t("detectionReceivePort", { ns: "settings" })}
              placeholder={t("selectDetectionReceivePort", { ns: "settings" })}
              value={selectedReceivePort}
              ports={ports}
              activeText={t("active", { ns: "common" })}
              onChange={onReceivePortChange}
            />
            <PortSelect
              label={t("detectionSendPort", { ns: "settings" })}
              placeholder={t("selectDetectionSendPort", { ns: "settings" })}
              value={selectedSendPort}
              ports={ports}
              activeText={t("active", { ns: "common" })}
              onChange={onSendPortChange}
            />
          </div>

          {ports.length === 0 ? <span className="text-sm text-base-content/55">{t("noPorts", { ns: "settings" })}</span> : null}
        </PanelBody>
      </Panel>

      <Panel>
        <PanelBody>
          <SectionHeader
            title={t("gpsSerialTitle", { ns: "settings" })}
            description={t("gpsSerialDescription", { ns: "settings" })}
            action={
              <span className="inline-flex h-8 items-center gap-2 rounded-xl border border-info/25 bg-info/10 px-3 text-xs font-semibold text-info">
                <Satellite size={15} />
                NMEA 0183
              </span>
            }
          />

          <div className="grid gap-3 md:grid-cols-3">
            <InfoTile label={t("gpsSessionTitle", { ns: "settings" })}>
              {gpsSessionStateLabel}
            </InfoTile>
            <InfoTile label={t("gpsDataPort", { ns: "settings" })} value={currentGPSDataPort || t("unknown", { ns: "common" })} />
            <InfoTile label={t("gpsControlPort", { ns: "settings" })} value={currentGPSControlPort || t("unknown", { ns: "common" })} />
          </div>

          <div className="grid gap-3 md:grid-cols-2">
            <PortSelect
              label={t("gpsDataPort", { ns: "settings" })}
              placeholder={t("selectGpsDataPort", { ns: "settings" })}
              value={selectedGPSDataPort}
              ports={ports}
              activeText={t("active", { ns: "common" })}
              onChange={onGPSDataPortChange}
            />
            <PortSelect
              label={t("gpsControlPort", { ns: "settings" })}
              placeholder={t("selectGpsControlPort", { ns: "settings" })}
              value={selectedGPSControlPort}
              ports={ports}
              activeText={t("active", { ns: "common" })}
              onChange={onGPSControlPortChange}
            />
          </div>

          <div className="grid gap-2 lg:grid-cols-[minmax(0,1fr)_minmax(0,1.4fr)]">
            <InfoTile label={t("gpsLastFix", { ns: "settings" })}>
              {gpsSession?.lastFix
                ? `${gpsSession.lastFix.latitude.toFixed(6)}, ${gpsSession.lastFix.longitude.toFixed(6)}`
                : t("unknown", { ns: "common" })}
            </InfoTile>
            <InfoTile label={t("gpsLastNmea", { ns: "settings" })}>
              <span className="line-clamp-2 font-mono text-[11px] leading-4">{gpsSession?.lastNmea || "-"}</span>
            </InfoTile>
          </div>

          {gpsBanner.kind === "error" ? <BannerAlert banner={gpsBanner} /> : null}
        </PanelBody>
      </Panel>

      <Panel>
        <PanelBody>
          <SectionHeader
            title={t("languageTitle", { ns: "settings" })}
            description={t("languageDescription", { ns: "settings" })}
          />

          <div className="grid gap-2 sm:grid-cols-2 lg:grid-cols-3">
            {allLocaleOptions.map((option) => {
              const active = visibleLocaleSet.has(option);
              const locked = option === currentLocale;
              return (
                <button
                  key={option}
                  className={cx(
                    "flex h-10 items-center justify-between gap-3 rounded-2xl border px-3 text-left text-sm font-semibold",
                    active
                      ? "border-primary/35 bg-primary/10 text-primary"
                      : "border-base-300 bg-base-100/55 text-base-content/65 hover:bg-base-300/65",
                    locked && "cursor-default",
                  )}
                  type="button"
                  aria-pressed={active}
                  onClick={() => handleToggleLocale(option)}
                >
                  <span className="flex min-w-0 items-center gap-2">
                    <Globe2 size={16} className="shrink-0" />
                    <span className="truncate">{fullLocaleName(option)}</span>
                  </span>
                  {active ? <Check size={16} className="shrink-0" /> : null}
                </button>
              );
            })}
          </div>

          <p className="text-xs leading-5 text-base-content/55">
            {t("currentLanguageRequired", { ns: "settings" })}
          </p>
        </PanelBody>
      </Panel>
    </section>
  );
}
