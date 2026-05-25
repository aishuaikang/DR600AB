import { useEffect, useMemo, useState } from "react";
import type { TFunction } from "i18next";
import { BellRing, Plus, ShieldCheck, Trash2, Volume2 } from "lucide-react";

import { Panel, PanelBody } from "../components/Panel";
import { SectionHeader } from "../components/SectionHeader";
import i18n from "../i18n";
import { cx } from "../utils/classnames";
import type { UserSettings } from "../types";
import { extractErrorMessage } from "../utils/session";
import {
  removeWhitelistSerial,
  resolveScreenAlarmSettings,
  upsertWhitelistItem,
} from "../utils/whitelist";

const defaultIntrusionRetentionDays = 90;
const intrusionRetentionOptions = [30, 90, 180, 0];

function coordinateDraft(value: number | undefined) {
  return typeof value === "number" && Number.isFinite(value) ? String(value) : "";
}

function normalizeCoordinateInput(value: string) {
  return value.replace(/[^\d.,-]/g, "").replace(",", ".");
}

function parseCoordinate(value: string) {
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

function formatSettingsDate(value: string | undefined, locale: string) {
  if (!value) {
    return "-";
  }
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return "-";
  }
  return date.toLocaleString(locale.startsWith("zh") ? "zh-CN" : "en-US", { hour12: false });
}

function formatWhitelistSource(value: string | undefined, t: TFunction) {
  const source = value?.trim();
  if (!source) {
    return "-";
  }
  if (source === "manual") {
    return t("whitelistSourceManual", { ns: "settings" });
  }
  if (source === "screen_position") {
    return t("whitelistSourceScreenPosition", { ns: "settings" });
  }
  return source;
}

export function UserSettingsPage({
  appTitle,
  defaultAppTitle,
  userSettings,
  t,
  onAppTitleChange,
  onUserSettingsChange,
}: {
  appTitle: string;
  defaultAppTitle: string;
  userSettings: UserSettings;
  t: TFunction;
  onAppTitleChange: (value: string) => void;
  onUserSettingsChange: (settings: UserSettings) => Promise<UserSettings>;
}) {
  const [titleDraft, setTitleDraft] = useState(appTitle);
  const [latitudeDraft, setLatitudeDraft] = useState(() => coordinateDraft(userSettings.manualDeviceLocation?.latitude));
  const [longitudeDraft, setLongitudeDraft] = useState(() => coordinateDraft(userSettings.manualDeviceLocation?.longitude));
  const [locationSaving, setLocationSaving] = useState(false);
  const [locationMessage, setLocationMessage] = useState<{ kind: "idle" | "success" | "error"; text: string }>({
    kind: "idle",
    text: "",
  });
  const [retentionSaving, setRetentionSaving] = useState(false);
  const [retentionMessage, setRetentionMessage] = useState<{ kind: "idle" | "success" | "error"; text: string }>({
    kind: "idle",
    text: "",
  });
  const [retentionDraft, setRetentionDraft] = useState(() => String(userSettings.intrusionRetentionDays ?? defaultIntrusionRetentionDays));
  const [whitelistSerialDraft, setWhitelistSerialDraft] = useState("");
  const [whitelistModelDraft, setWhitelistModelDraft] = useState("");
  const [whitelistSaving, setWhitelistSaving] = useState(false);
  const [whitelistMessage, setWhitelistMessage] = useState<{ kind: "idle" | "success" | "error"; text: string }>({
    kind: "idle",
    text: "",
  });
  const [alarmSaving, setAlarmSaving] = useState(false);
  const [alarmMessage, setAlarmMessage] = useState<{ kind: "idle" | "success" | "error"; text: string }>({
    kind: "idle",
    text: "",
  });
  const normalizedDraft = titleDraft.trim();
  const changed = normalizedDraft !== appTitle;
  const savedRetentionDays = userSettings.intrusionRetentionDays ?? defaultIntrusionRetentionDays;
  const retentionDays = Number(retentionDraft);
  const retentionChanged = Number.isFinite(retentionDays) && retentionDays >= 0 && retentionDays !== savedRetentionDays;
  const savedLatitude = userSettings.manualDeviceLocation?.latitude;
  const savedLongitude = userSettings.manualDeviceLocation?.longitude;
  const latitude = parseCoordinate(latitudeDraft);
  const longitude = parseCoordinate(longitudeDraft);
  const locationComplete = latitudeDraft.trim() !== "" && longitudeDraft.trim() !== "";
  const locationValid = locationComplete && validLatitude(latitude) && validLongitude(longitude);
  const locationChanged = useMemo(() => {
    if (!locationComplete) {
      return Boolean(userSettings.manualDeviceLocation);
    }
    if (!locationValid) {
      return false;
    }
    return savedLatitude !== latitude || savedLongitude !== longitude;
  }, [latitude, locationComplete, locationValid, longitude, savedLatitude, savedLongitude, userSettings.manualDeviceLocation]);
  const whitelist = userSettings.whitelist ?? [];
  const alarmSettings = resolveScreenAlarmSettings(userSettings.screenAlarmSettings);
  const whitelistSerial = whitelistSerialDraft.trim();

  useEffect(() => {
    setTitleDraft(appTitle);
  }, [appTitle]);

  useEffect(() => {
    setLatitudeDraft(coordinateDraft(userSettings.manualDeviceLocation?.latitude));
    setLongitudeDraft(coordinateDraft(userSettings.manualDeviceLocation?.longitude));
  }, [userSettings.manualDeviceLocation?.latitude, userSettings.manualDeviceLocation?.longitude]);

  useEffect(() => {
    setRetentionDraft(String(userSettings.intrusionRetentionDays ?? defaultIntrusionRetentionDays));
  }, [userSettings.intrusionRetentionDays]);

  const saveManualLocation = async () => {
    if (!locationValid) {
      setLocationMessage({ kind: "error", text: t("manualDeviceLocationInvalid", { ns: "settings" }) });
      return;
    }
    setLocationSaving(true);
    setLocationMessage({ kind: "idle", text: "" });
    try {
      await onUserSettingsChange({
        ...userSettings,
        manualDeviceLocation: {
          latitude,
          longitude,
        },
      });
      setLocationMessage({ kind: "success", text: t("manualDeviceLocationSaved", { ns: "settings" }) });
    } catch (error) {
      setLocationMessage({ kind: "error", text: extractErrorMessage(error, t("unexpectedError", { ns: "common" })) });
    } finally {
      setLocationSaving(false);
    }
  };

  const clearManualLocation = async () => {
    setLocationSaving(true);
    setLocationMessage({ kind: "idle", text: "" });
    try {
      await onUserSettingsChange({
        ...userSettings,
        manualDeviceLocation: undefined,
      });
      setLocationMessage({ kind: "success", text: t("manualDeviceLocationCleared", { ns: "settings" }) });
    } catch (error) {
      setLocationMessage({ kind: "error", text: extractErrorMessage(error, t("unexpectedError", { ns: "common" })) });
    } finally {
      setLocationSaving(false);
    }
  };

  const saveIntrusionRetention = async () => {
    if (!Number.isFinite(retentionDays) || retentionDays < 0) {
      setRetentionMessage({ kind: "error", text: t("intrusionRetentionInvalid", { ns: "settings" }) });
      return;
    }
    setRetentionSaving(true);
    setRetentionMessage({ kind: "idle", text: "" });
    try {
      await onUserSettingsChange({
        ...userSettings,
        intrusionRetentionDays: retentionDays,
      });
      setRetentionMessage({ kind: "success", text: t("intrusionRetentionSaved", { ns: "settings" }) });
    } catch (error) {
      setRetentionMessage({ kind: "error", text: extractErrorMessage(error, t("unexpectedError", { ns: "common" })) });
    } finally {
      setRetentionSaving(false);
    }
  };

  const addWhitelistItem = async () => {
    if (!whitelistSerial) {
      setWhitelistMessage({ kind: "error", text: t("whitelistSerialRequired", { ns: "settings" }) });
      return;
    }
    setWhitelistSaving(true);
    setWhitelistMessage({ kind: "idle", text: "" });
    try {
      await onUserSettingsChange({
        ...userSettings,
        whitelist: upsertWhitelistItem(whitelist, {
          serial: whitelistSerial,
          model: whitelistModelDraft,
          source: "manual",
        }),
      });
      setWhitelistSerialDraft("");
      setWhitelistModelDraft("");
      setWhitelistMessage({ kind: "success", text: t("whitelistSaved", { ns: "settings" }) });
    } catch (error) {
      setWhitelistMessage({ kind: "error", text: extractErrorMessage(error, t("unexpectedError", { ns: "common" })) });
    } finally {
      setWhitelistSaving(false);
    }
  };

  const deleteWhitelistItem = async (serial: string) => {
    setWhitelistSaving(true);
    setWhitelistMessage({ kind: "idle", text: "" });
    try {
      await onUserSettingsChange({
        ...userSettings,
        whitelist: removeWhitelistSerial(whitelist, serial),
      });
      setWhitelistMessage({ kind: "success", text: t("whitelistDeleted", { ns: "settings" }) });
    } catch (error) {
      setWhitelistMessage({ kind: "error", text: extractErrorMessage(error, t("unexpectedError", { ns: "common" })) });
    } finally {
      setWhitelistSaving(false);
    }
  };

  const updateAlarmSetting = async (key: keyof typeof alarmSettings, value: boolean) => {
    setAlarmSaving(true);
    setAlarmMessage({ kind: "idle", text: "" });
    try {
      await onUserSettingsChange({
        ...userSettings,
        screenAlarmSettings: {
          ...alarmSettings,
          [key]: value,
        },
      });
      setAlarmMessage({ kind: "success", text: t("screenAlarmSaved", { ns: "settings" }) });
    } catch (error) {
      setAlarmMessage({ kind: "error", text: extractErrorMessage(error, t("unexpectedError", { ns: "common" })) });
    } finally {
      setAlarmSaving(false);
    }
  };

  const alarmOptions = [
    { key: "detection" as const, label: t("screenAlarmDetection", { ns: "settings" }), icon: <BellRing size={15} aria-hidden="true" /> },
    { key: "position" as const, label: t("screenAlarmPosition", { ns: "settings" }), icon: <ShieldCheck size={15} aria-hidden="true" /> },
    { key: "fpv" as const, label: t("screenAlarmFpv", { ns: "settings" }), icon: <BellRing size={15} aria-hidden="true" /> },
    { key: "sound" as const, label: t("screenAlarmSound", { ns: "settings" }), icon: <Volume2 size={15} aria-hidden="true" /> },
  ];

  return (
    <section className="grid gap-3">
      <Panel>
        <PanelBody>
          <SectionHeader
            title={t("deviceSnTitle", { ns: "settings" })}
            description={t("deviceSnDescription", { ns: "settings" })}
          />

          <div className="rounded-2xl border border-primary/20 bg-primary/10 px-4 py-3">
            <span className="text-[11px] font-semibold uppercase tracking-wide text-primary/70">
              {t("deviceSnField", { ns: "settings" })}
            </span>
            <strong className="mt-2 block min-w-0 break-words font-mono text-lg font-semibold text-base-content">
              {userSettings.deviceSn || t("unknown", { ns: "common" })}
            </strong>
          </div>
        </PanelBody>
      </Panel>

      <Panel>
        <PanelBody>
          <SectionHeader
            title={t("displayTitle", { ns: "settings" })}
            description={t("displayDescription", { ns: "settings" })}
          />

          <div className="grid gap-3 lg:grid-cols-[minmax(0,1fr)_18rem]">
            <label className="grid gap-1.5">
              <span className="text-xs font-medium text-base-content/60">{t("customTitle", { ns: "settings" })}</span>
              <input
                className="input input-bordered input-sm w-full bg-base-100"
                value={titleDraft}
                maxLength={32}
                placeholder={defaultAppTitle}
                onChange={(event) => setTitleDraft(event.target.value)}
              />
              <span className="text-xs leading-5 text-base-content/50">{t("customTitleHint", { ns: "settings" })}</span>
            </label>

            <div className="rounded-2xl border border-base-300 bg-base-100/45 p-3">
              <span className="text-[11px] font-semibold uppercase tracking-wide text-base-content/45">{t("preview", { ns: "settings" })}</span>
              <strong className="mt-2 block truncate text-sm font-semibold text-base-content">
                {normalizedDraft || defaultAppTitle}
              </strong>
            </div>
          </div>

          <div className="flex flex-wrap justify-end gap-2">
            <button
              className="btn btn-sm btn-outline"
              type="button"
              onClick={() => {
                setTitleDraft(defaultAppTitle);
                onAppTitleChange("");
              }}
            >
              {t("restoreDefault", { ns: "settings" })}
            </button>
            <button
              className="btn btn-sm btn-primary"
              type="button"
              disabled={!changed}
              onClick={() => onAppTitleChange(normalizedDraft)}
            >
              {t("save", { ns: "common" })}
            </button>
          </div>
        </PanelBody>
      </Panel>

      <Panel>
        <PanelBody>
          <SectionHeader
            title={t("manualDeviceLocationTitle", { ns: "settings" })}
            description={t("manualDeviceLocationDescription", { ns: "settings" })}
          />

          <div className="grid gap-3 lg:grid-cols-[minmax(0,1fr)_18rem]">
            <div className="grid gap-3 sm:grid-cols-2">
              <label className="grid gap-1.5">
                <span className="text-xs font-medium text-base-content/60">{t("latitude", { ns: "settings" })}</span>
                <input
                  className="input input-bordered input-sm w-full bg-base-100"
                  type="text"
                  inputMode="decimal"
                  data-keyboard="numeric"
                  pattern="-?[0-9]*[.,]?[0-9]*"
                  value={latitudeDraft}
                  placeholder="23.129110"
                  onChange={(event) => setLatitudeDraft(normalizeCoordinateInput(event.target.value))}
                />
              </label>
              <label className="grid gap-1.5">
                <span className="text-xs font-medium text-base-content/60">{t("longitude", { ns: "settings" })}</span>
                <input
                  className="input input-bordered input-sm w-full bg-base-100"
                  type="text"
                  inputMode="decimal"
                  data-keyboard="numeric"
                  pattern="-?[0-9]*[.,]?[0-9]*"
                  value={longitudeDraft}
                  placeholder="113.264385"
                  onChange={(event) => setLongitudeDraft(normalizeCoordinateInput(event.target.value))}
                />
              </label>
              <p className="text-xs leading-5 text-base-content/50 sm:col-span-2">
                {t("manualDeviceLocationHint", { ns: "settings" })}
              </p>
            </div>

            <div className="rounded-2xl border border-base-300 bg-base-100/45 p-3">
              <span className="text-[11px] font-semibold uppercase tracking-wide text-base-content/45">{t("savedValue", { ns: "settings" })}</span>
              <strong className="mt-2 block text-sm font-semibold text-base-content">
                {userSettings.manualDeviceLocation
                  ? `${userSettings.manualDeviceLocation.latitude.toFixed(6)}, ${userSettings.manualDeviceLocation.longitude.toFixed(6)}`
                  : t("notConfigured", { ns: "common" })}
              </strong>
            </div>
          </div>

          {locationMessage.text ? (
            <div className={`alert py-2 text-sm ${locationMessage.kind === "error" ? "alert-error" : "alert-success"}`}>
              {locationMessage.text}
            </div>
          ) : null}

          <div className="flex flex-wrap justify-end gap-2">
            <button
              className="btn btn-sm btn-outline"
              type="button"
              disabled={locationSaving || !userSettings.manualDeviceLocation}
              onClick={() => void clearManualLocation()}
            >
              {t("clear", { ns: "common" })}
            </button>
            <button
              className="btn btn-sm btn-primary"
              type="button"
              disabled={locationSaving || !locationChanged}
              onClick={() => void saveManualLocation()}
            >
              {locationSaving ? t("loading", { ns: "common" }) : t("save", { ns: "common" })}
            </button>
          </div>
        </PanelBody>
      </Panel>

      <Panel>
        <PanelBody>
          <SectionHeader
            title={t("intrusionRetentionTitle", { ns: "settings" })}
            description={t("intrusionRetentionDescription", { ns: "settings" })}
          />

          <div className="grid gap-3 lg:grid-cols-[minmax(0,1fr)_18rem]">
            <label className="grid gap-1.5">
              <span className="text-xs font-medium text-base-content/60">{t("intrusionRetentionField", { ns: "settings" })}</span>
              <select
                className="select select-bordered select-sm w-full bg-base-100"
                value={retentionDraft}
                onChange={(event) => setRetentionDraft(event.target.value)}
              >
                {intrusionRetentionOptions.map((days) => (
                  <option key={days} value={days}>
                    {days === 0
                      ? t("intrusionRetentionForever", { ns: "settings" })
                      : t("intrusionRetentionDays", { ns: "settings", value: days })}
                  </option>
                ))}
              </select>
              <span className="text-xs leading-5 text-base-content/50">{t("intrusionRetentionHint", { ns: "settings" })}</span>
            </label>

            <div className="rounded-2xl border border-base-300 bg-base-100/45 p-3">
              <span className="text-[11px] font-semibold uppercase tracking-wide text-base-content/45">{t("savedValue", { ns: "settings" })}</span>
              <strong className="mt-2 block text-sm font-semibold text-base-content">
                {savedRetentionDays === 0
                  ? t("intrusionRetentionForever", { ns: "settings" })
                  : t("intrusionRetentionDays", { ns: "settings", value: savedRetentionDays })}
              </strong>
            </div>
          </div>

          {retentionMessage.text ? (
            <div className={`alert py-2 text-sm ${retentionMessage.kind === "error" ? "alert-error" : "alert-success"}`}>
              {retentionMessage.text}
            </div>
          ) : null}

          <div className="flex flex-wrap justify-end gap-2">
            <button
              className="btn btn-sm btn-primary"
              type="button"
              disabled={retentionSaving || !retentionChanged}
              onClick={() => void saveIntrusionRetention()}
            >
              {retentionSaving ? t("loading", { ns: "common" }) : t("save", { ns: "common" })}
            </button>
          </div>
        </PanelBody>
      </Panel>

      <Panel>
        <PanelBody>
          <SectionHeader
            title={t("whitelistTitle", { ns: "settings" })}
            description={t("whitelistDescription", { ns: "settings" })}
            action={
              <span className="inline-flex h-8 items-center gap-2 rounded-xl border border-success/25 bg-success/10 px-3 text-xs font-semibold text-success">
                <ShieldCheck size={15} aria-hidden="true" />
                {t("whitelistCount", { ns: "settings", count: whitelist.length })}
              </span>
            }
          />

          <div className="grid gap-3 lg:grid-cols-[minmax(0,1fr)_12rem]">
            <div className="grid gap-3 sm:grid-cols-2">
              <label className="grid gap-1.5">
                <span className="text-xs font-medium text-base-content/60">{t("whitelistSerial", { ns: "settings" })}</span>
                <input
                  className="input input-bordered input-sm w-full bg-base-100"
                  value={whitelistSerialDraft}
                  maxLength={128}
                  placeholder="UAV-8F12"
                  onChange={(event) => setWhitelistSerialDraft(event.target.value)}
                />
              </label>
              <label className="grid gap-1.5">
                <span className="text-xs font-medium text-base-content/60">{t("whitelistModel", { ns: "settings" })}</span>
                <input
                  className="input input-bordered input-sm w-full bg-base-100"
                  value={whitelistModelDraft}
                  maxLength={64}
                  placeholder={t("whitelistModelPlaceholder", { ns: "settings" })}
                  onChange={(event) => setWhitelistModelDraft(event.target.value)}
                />
              </label>
            </div>

            <button
              className="btn btn-sm btn-primary self-end"
              type="button"
              disabled={whitelistSaving || !whitelistSerial}
              onClick={() => void addWhitelistItem()}
            >
              <Plus size={15} aria-hidden="true" />
              {t("whitelistAdd", { ns: "settings" })}
            </button>
          </div>

          {whitelistMessage.text ? (
            <div className={`alert py-2 text-sm ${whitelistMessage.kind === "error" ? "alert-error" : "alert-success"}`}>
              {whitelistMessage.text}
            </div>
          ) : null}

          <div className="overflow-x-auto rounded-2xl border border-base-300 bg-base-100/45">
            <table className="table table-sm">
              <thead>
                <tr>
                  <th>{t("whitelistSerial", { ns: "settings" })}</th>
                  <th>{t("whitelistModel", { ns: "settings" })}</th>
                  <th>{t("whitelistSource", { ns: "settings" })}</th>
                  <th>{t("whitelistCreatedAt", { ns: "settings" })}</th>
                  <th className="text-right">{t("whitelistActions", { ns: "settings" })}</th>
                </tr>
              </thead>
              <tbody>
                {whitelist.length > 0 ? whitelist.map((item) => (
                  <tr key={`${item.serial}-${item.createdAt ?? ""}`}>
                    <td className="font-mono font-semibold">{item.serial}</td>
                    <td>{item.model || "-"}</td>
                    <td>{formatWhitelistSource(item.source, t)}</td>
                    <td>{formatSettingsDate(item.createdAt, i18n.language)}</td>
                    <td className="text-right">
                      <button
                        className="btn btn-ghost btn-xs text-error"
                        type="button"
                        disabled={whitelistSaving}
                        aria-label={t("whitelistDelete", { ns: "settings", serial: item.serial })}
                        onClick={() => void deleteWhitelistItem(item.serial)}
                      >
                        <Trash2 size={14} aria-hidden="true" />
                      </button>
                    </td>
                  </tr>
                )) : (
                  <tr>
                    <td colSpan={5} className="py-6 text-center text-sm text-base-content/50">
                      {t("whitelistEmpty", { ns: "settings" })}
                    </td>
                  </tr>
                )}
              </tbody>
            </table>
          </div>
        </PanelBody>
      </Panel>

      <Panel>
        <PanelBody>
          <SectionHeader
            title={t("screenAlarmTitle", { ns: "settings" })}
            description={t("screenAlarmDescription", { ns: "settings" })}
          />

          <div className="grid gap-2 sm:grid-cols-2 lg:grid-cols-4">
            {alarmOptions.map((option) => {
              const active = alarmSettings[option.key];
              return (
                <button
                  key={option.key}
                  className={cx(
                    "flex h-11 items-center justify-between gap-3 rounded-2xl border px-3 text-left text-sm font-semibold",
                    active
                      ? "border-error/35 bg-error/10 text-error"
                      : "border-base-300 bg-base-100/55 text-base-content/55 hover:bg-base-300/65",
                  )}
                  type="button"
                  aria-pressed={active}
                  disabled={alarmSaving}
                  onClick={() => void updateAlarmSetting(option.key, !active)}
                >
                  <span className="inline-flex min-w-0 items-center gap-2">
                    {option.icon}
                    <span className="truncate">{option.label}</span>
                  </span>
                  <input
                    className="toggle toggle-error toggle-sm"
                    type="checkbox"
                    tabIndex={-1}
                    checked={active}
                    readOnly
                    aria-hidden="true"
                  />
                </button>
              );
            })}
          </div>

          {alarmMessage.text ? (
            <div className={`alert py-2 text-sm ${alarmMessage.kind === "error" ? "alert-error" : "alert-success"}`}>
              {alarmMessage.text}
            </div>
          ) : null}
        </PanelBody>
      </Panel>
    </section>
  );
}
