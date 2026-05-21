import { useEffect, useState } from "react";
import type { TFunction } from "i18next";
import { Check, Globe2, Map as MapIcon, RefreshCw, Satellite, SatelliteDish } from "lucide-react";

import { BannerAlert } from "../components/BannerAlert";
import { InfoTile } from "../components/InfoTile";
import { Panel, PanelBody } from "../components/Panel";
import { PortSelect } from "../components/PortSelect";
import { SectionHeader } from "../components/SectionHeader";
import type { Banner } from "../app/types";
import {
  DETECTION_DEFAULT_BAUD_RATE,
  SERIAL_BAUD_RATE_LIMITS,
  normalizeSerialBaudRate,
} from "../serial-profile";
import type { DeceptionSessionResponse, GPSSessionResponse, PortInfo, UserSettings } from "../types";
import { cx } from "../utils/classnames";
import { fullLocaleName } from "../utils/locales";
import { extractErrorMessage } from "../utils/session";
import type { ReferenceMapLayer } from "./screenData";

const screenStrikeChannelLabelCount = 3;

function normalizeStrikeLabel(value: string) {
  return value.trim().slice(0, 24);
}

function formatBaudRate(value: number) {
  return String(normalizeSerialBaudRate(value));
}

export function SettingsPage({
  banner,
  ports,
  selectedReceivePort,
  selectedSendPort,
  selectedDetectionBaudRate,
  selectedGPSDataPort,
  selectedGPSControlPort,
  selectedDeceptionPort,
  sessionStateLabel,
  currentReceivePort,
  currentSendPort,
  currentDetectionBaudRate,
  gpsBanner,
  gpsSession,
  gpsSessionStateLabel,
  currentGPSDataPort,
  currentGPSControlPort,
  deceptionSession,
  deceptionSessionStateLabel,
  currentDeceptionPort,
  allLocaleOptions,
  visibleLocales,
  currentLocale,
  allMapLayerOptions,
  visibleMapLayers,
  userSettings,
  t,
  onRefresh,
  onReceivePortChange,
  onSendPortChange,
  onDetectionBaudRateChange,
  onGPSDataPortChange,
  onGPSControlPortChange,
  onDeceptionPortChange,
  onUserSettingsChange,
  onVisibleLocalesChange,
  onVisibleMapLayersChange,
}: {
  banner: Banner;
  ports: PortInfo[];
  selectedReceivePort: string;
  selectedSendPort: string;
  selectedDetectionBaudRate: number;
  selectedGPSDataPort: string;
  selectedGPSControlPort: string;
  selectedDeceptionPort: string;
  sessionStateLabel: string;
  currentReceivePort: string;
  currentSendPort: string;
  currentDetectionBaudRate: number;
  gpsBanner: Banner;
  gpsSession: GPSSessionResponse | null;
  gpsSessionStateLabel: string;
  currentGPSDataPort: string;
  currentGPSControlPort: string;
  deceptionSession: DeceptionSessionResponse | null;
  deceptionSessionStateLabel: string;
  currentDeceptionPort: string;
  allLocaleOptions: string[];
  visibleLocales: string[];
  currentLocale: string;
  allMapLayerOptions: ReferenceMapLayer[];
  visibleMapLayers: ReferenceMapLayer[];
  userSettings: UserSettings;
  t: TFunction;
  onRefresh: () => void;
  onReceivePortChange: (value: string) => void;
  onSendPortChange: (value: string) => void;
  onDetectionBaudRateChange: (value: number) => void;
  onGPSDataPortChange: (value: string) => void;
  onGPSControlPortChange: (value: string) => void;
  onDeceptionPortChange: (value: string) => void;
  onUserSettingsChange: (settings: UserSettings) => Promise<UserSettings>;
  onVisibleLocalesChange: (locales: string[]) => void;
  onVisibleMapLayersChange: (layers: ReferenceMapLayer[]) => void;
}) {
  const visibleLocaleSet = new Set(visibleLocales);
  const visibleMapLayerSet = new Set(visibleMapLayers);
  const savedStrikeLabels = Array.from({ length: screenStrikeChannelLabelCount }, (_, index) =>
    userSettings.screenStrikeChannelLabels?.[index] ?? "",
  );
  const [strikeLabelDrafts, setStrikeLabelDrafts] = useState(savedStrikeLabels);
  const [strikeLabelSaving, setStrikeLabelSaving] = useState(false);
  const [strikeLabelMessage, setStrikeLabelMessage] = useState<{ kind: "idle" | "success" | "error"; text: string }>({
    kind: "idle",
    text: "",
  });
  const [detectionBaudRateDraft, setDetectionBaudRateDraft] = useState(() => formatBaudRate(selectedDetectionBaudRate));
  const normalizedStrikeLabels = strikeLabelDrafts.map(normalizeStrikeLabel);
  const strikeLabelsChanged = normalizedStrikeLabels.join("|") !== savedStrikeLabels.map(normalizeStrikeLabel).join("|");
  const detectionBaudRate = normalizeSerialBaudRate(selectedDetectionBaudRate);

  useEffect(() => {
    setStrikeLabelDrafts(savedStrikeLabels);
  }, [savedStrikeLabels.join("|")]);

  useEffect(() => {
    setDetectionBaudRateDraft(formatBaudRate(selectedDetectionBaudRate));
  }, [selectedDetectionBaudRate]);

  const handleToggleLocale = (locale: string) => {
    if (locale === currentLocale) {
      return;
    }
    const next = visibleLocaleSet.has(locale)
      ? visibleLocales.filter((item) => item !== locale)
      : [...visibleLocales, locale];
    onVisibleLocalesChange(next);
  };

  const handleToggleMapLayer = (layer: ReferenceMapLayer) => {
    const active = visibleMapLayerSet.has(layer);
    if (active && visibleMapLayers.length <= 1) {
      return;
    }
    const next = active
      ? visibleMapLayers.filter((item) => item !== layer)
      : [...visibleMapLayers, layer];
    onVisibleMapLayersChange(next);
  };

  const updateStrikeLabelDraft = (index: number, value: string) => {
    setStrikeLabelDrafts((items) => items.map((item, itemIndex) => (itemIndex === index ? value : item)));
    setStrikeLabelMessage({ kind: "idle", text: "" });
  };

  const saveStrikeLabels = async () => {
    setStrikeLabelSaving(true);
    setStrikeLabelMessage({ kind: "idle", text: "" });
    try {
      await onUserSettingsChange({
        ...userSettings,
        screenStrikeChannelLabels: normalizedStrikeLabels,
      });
      setStrikeLabelMessage({ kind: "success", text: t("interferenceBandLabelsSaved", { ns: "settings" }) });
    } catch (error) {
      setStrikeLabelMessage({ kind: "error", text: extractErrorMessage(error, t("unexpectedError", { ns: "common" })) });
    } finally {
      setStrikeLabelSaving(false);
    }
  };

  const clearStrikeLabels = async () => {
    const nextLabels = Array.from({ length: screenStrikeChannelLabelCount }, () => "");
    setStrikeLabelDrafts(nextLabels);
    setStrikeLabelSaving(true);
    setStrikeLabelMessage({ kind: "idle", text: "" });
    try {
      await onUserSettingsChange({
        ...userSettings,
        screenStrikeChannelLabels: [],
      });
      setStrikeLabelMessage({ kind: "success", text: t("interferenceBandLabelsCleared", { ns: "settings" }) });
    } catch (error) {
      setStrikeLabelMessage({ kind: "error", text: extractErrorMessage(error, t("unexpectedError", { ns: "common" })) });
    } finally {
      setStrikeLabelSaving(false);
    }
  };

  const commitDetectionBaudRate = () => {
    const nextBaudRate = normalizeSerialBaudRate(Number(detectionBaudRateDraft), detectionBaudRate);
    setDetectionBaudRateDraft(formatBaudRate(nextBaudRate));
    if (nextBaudRate !== selectedDetectionBaudRate) {
      onDetectionBaudRateChange(nextBaudRate);
    }
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
              <InfoTile label={t("detectionBaudRate", { ns: "settings" })} value={`${currentDetectionBaudRate || DETECTION_DEFAULT_BAUD_RATE} bps`} />
            </div>
          </div>
          {banner.kind === "error" ? <BannerAlert banner={banner} /> : null}
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
            title={t("detectionSerialTitle", { ns: "settings" })}
            description={t("detectionSerialDescription", { ns: "settings" })}
            action={
              <button className="btn btn-sm btn-outline btn-info" type="button" onClick={onRefresh}>
                <RefreshCw size={16} />
                <span>{t("refresh", { ns: "common" })}</span>
              </button>
            }
          />

          <div className="grid gap-3 md:grid-cols-3">
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
            <label className="grid gap-1.5">
              <span className="text-xs font-medium text-base-content/60">{t("detectionBaudRate", { ns: "settings" })}</span>
              <input
                className="input input-bordered input-sm w-full bg-base-100"
                type="number"
                inputMode="numeric"
                min={SERIAL_BAUD_RATE_LIMITS.min}
                max={SERIAL_BAUD_RATE_LIMITS.max}
                step={100}
                value={detectionBaudRateDraft}
                onChange={(event) => {
                  setDetectionBaudRateDraft(event.target.value);
                }}
                onBlur={commitDetectionBaudRate}
                onKeyDown={(event) => {
                  if (event.key === "Enter") {
                    event.currentTarget.blur();
                  }
                }}
              />
              <span className="text-xs leading-5 text-base-content/50">
                {t("detectionBaudRateHint", { ns: "settings" })}
              </span>
            </label>
          </div>

          {ports.length === 0 ? <span className="text-sm text-base-content/55">{t("noPorts", { ns: "settings" })}</span> : null}
        </PanelBody>
      </Panel>

      <Panel>
        <PanelBody>
          <SectionHeader
            title={t("deceptionSerialTitle", { ns: "settings" })}
            description={t("deceptionSerialDescription", { ns: "settings" })}
            action={
              <span className="inline-flex h-8 items-center gap-2 rounded-xl border border-info/25 bg-info/10 px-3 text-xs font-semibold text-info">
                <SatelliteDish size={15} />
                GNSS
              </span>
            }
          />

          <div className="grid gap-3 md:grid-cols-3">
            <InfoTile label={t("deceptionSessionTitle", { ns: "settings" })}>
              {deceptionSessionStateLabel}
            </InfoTile>
            <InfoTile label={t("deceptionPort", { ns: "settings" })} value={currentDeceptionPort || t("unknown", { ns: "common" })} />
            <InfoTile label={t("deceptionLastError", { ns: "settings" })} value={deceptionSession?.lastError || "-"} />
          </div>

          <PortSelect
            label={t("deceptionPort", { ns: "settings" })}
            placeholder={t("selectDeceptionPort", { ns: "settings" })}
            value={selectedDeceptionPort}
            ports={ports}
            activeText={t("active", { ns: "common" })}
            onChange={onDeceptionPortChange}
          />
        </PanelBody>
      </Panel>

      <Panel>
        <PanelBody>
          <SectionHeader
            title={t("interferenceBandLabelsTitle", { ns: "settings" })}
            description={t("interferenceBandLabelsDescription", { ns: "settings" })}
          />

          <div className="grid gap-3 lg:grid-cols-[minmax(0,1fr)_18rem]">
            <div className="grid gap-3 sm:grid-cols-3">
              {strikeLabelDrafts.map((value, index) => (
                <label key={index} className="grid gap-1.5">
                  <span className="text-xs font-medium text-base-content/60">
                    {t("interferenceBandLabel", { ns: "settings", index: index + 1 })}
                  </span>
                  <input
                    className="input input-bordered input-sm w-full bg-base-100"
                    value={value}
                    maxLength={24}
                    placeholder={t("interferenceBandLabelPlaceholder", { ns: "settings", index: index + 1 })}
                    onChange={(event) => updateStrikeLabelDraft(index, event.target.value)}
                  />
                </label>
              ))}
              <p className="text-xs leading-5 text-base-content/50 sm:col-span-3">
                {t("interferenceBandLabelsHint", { ns: "settings" })}
              </p>
            </div>

            <div className="rounded-2xl border border-base-300 bg-base-100/45 p-3">
              <span className="text-[11px] font-semibold uppercase tracking-wide text-base-content/45">{t("preview", { ns: "settings" })}</span>
              <div className="mt-2 flex flex-wrap gap-1.5">
                {normalizedStrikeLabels.map((label, index) => (
                  <span key={index} className="rounded-full border border-primary/25 bg-primary/10 px-2 py-1 text-xs font-semibold text-primary">
                    {label || t("interferenceBandLabelPlaceholder", { ns: "settings", index: index + 1 })}
                  </span>
                ))}
              </div>
            </div>
          </div>

          {strikeLabelMessage.text ? (
            <div className={`alert py-2 text-sm ${strikeLabelMessage.kind === "error" ? "alert-error" : "alert-success"}`}>
              {strikeLabelMessage.text}
            </div>
          ) : null}

          <div className="flex flex-wrap justify-end gap-2">
            <button
              className="btn btn-sm btn-outline"
              type="button"
              disabled={strikeLabelSaving || normalizedStrikeLabels.every((label) => !label)}
              onClick={() => void clearStrikeLabels()}
            >
              {t("restoreDefault", { ns: "settings" })}
            </button>
            <button
              className="btn btn-sm btn-primary"
              type="button"
              disabled={strikeLabelSaving || !strikeLabelsChanged}
              onClick={() => void saveStrikeLabels()}
            >
              {strikeLabelSaving ? t("loading", { ns: "common" }) : t("save", { ns: "common" })}
            </button>
          </div>
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

      <Panel>
        <PanelBody>
          <SectionHeader
            title={t("mapLayerTitle", { ns: "settings" })}
            description={t("mapLayerDescription", { ns: "settings" })}
          />

          <div className="grid gap-2 sm:grid-cols-2 lg:grid-cols-3">
            {allMapLayerOptions.map((option) => {
              const active = visibleMapLayerSet.has(option);
              const locked = active && visibleMapLayers.length <= 1;
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
                  onClick={() => handleToggleMapLayer(option)}
                >
                  <span className="flex min-w-0 items-center gap-2">
                    <MapIcon size={16} className="shrink-0" />
                    <span className="truncate">{t(option, { ns: "screen" })}</span>
                  </span>
                  {active ? <Check size={16} className="shrink-0" /> : null}
                </button>
              );
            })}
          </div>

          <p className="text-xs leading-5 text-base-content/55">
            {t("mapLayerAtLeastOne", { ns: "settings" })}
          </p>
        </PanelBody>
      </Panel>
    </section>
  );
}
