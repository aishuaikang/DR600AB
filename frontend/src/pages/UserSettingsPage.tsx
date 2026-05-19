import { useEffect, useMemo, useState } from "react";
import type { TFunction } from "i18next";

import { Panel, PanelBody } from "../components/Panel";
import { SectionHeader } from "../components/SectionHeader";
import type { UserSettings } from "../types";
import { extractErrorMessage } from "../utils/session";

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
  const normalizedDraft = titleDraft.trim();
  const changed = normalizedDraft !== appTitle;
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

  useEffect(() => {
    setTitleDraft(appTitle);
  }, [appTitle]);

  useEffect(() => {
    setLatitudeDraft(coordinateDraft(userSettings.manualDeviceLocation?.latitude));
    setLongitudeDraft(coordinateDraft(userSettings.manualDeviceLocation?.longitude));
  }, [userSettings.manualDeviceLocation?.latitude, userSettings.manualDeviceLocation?.longitude]);

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

  return (
    <section className="grid gap-3">
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
    </section>
  );
}
