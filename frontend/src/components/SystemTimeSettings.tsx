import { useCallback, useEffect, useRef, useState } from "react";
import type { TFunction } from "i18next";
import { CalendarClock, ChevronDown, Clock3, Globe2, RefreshCw, Save, Search } from "lucide-react";

import { BannerAlert } from "../components/BannerAlert";
import { InfoTile } from "../components/InfoTile";
import { Panel, PanelBody } from "../components/Panel";
import { SectionHeader } from "../components/SectionHeader";
import type { Banner } from "../app/types";
import {
  getSystemTime,
  getSystemTimezones,
  updateSystemManualTime,
  updateSystemNTP,
  updateSystemTimezone,
} from "../api";
import type { SystemTimeInfo } from "../types";
import { extractErrorMessage } from "../utils/session";

type Action = "ntp" | "timezone" | "manual" | "";

function toDateTimeLocal(value: string | undefined) {
  const normalized = value?.trim().replace(" ", "T") || "";
  return /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}$/.test(normalized) ? normalized : "";
}

function toRemoteDateTime(value: string) {
  let normalized = value.trim().replace("T", " ");
  if (/^\d{4}-\d{2}-\d{2} \d{2}:\d{2}$/.test(normalized)) {
    normalized += ":00";
  }
  return /^\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}$/.test(normalized) ? normalized : "";
}

function uniqueTimezones(timezones: string[], current: string | undefined) {
  const values = [...timezones, current || ""].map((item) => item.trim()).filter(Boolean);
  return Array.from(new Set(values)).sort((left, right) => left.localeCompare(right));
}

export function SystemTimeSettings({ locale, t }: { locale: string; t: TFunction }) {
  const [info, setInfo] = useState<SystemTimeInfo | null>(null);
  const [timezones, setTimezones] = useState<string[]>([]);
  const [timezone, setTimezone] = useState("");
  const [timezoneQuery, setTimezoneQuery] = useState("");
  const [timezoneMenuOpen, setTimezoneMenuOpen] = useState(false);
  const timezonePickerRef = useRef<HTMLDivElement>(null);
  const [manualTime, setManualTime] = useState("");
  const [loading, setLoading] = useState(false);
  const [action, setAction] = useState<Action>("");
  const [banner, setBanner] = useState<Banner>({ kind: "idle", message: "" });

  const loadTimeInfo = useCallback(async (silent = false) => {
    if (!silent) {
      setLoading(true);
      setBanner({ kind: "loading", message: t("systemTimeLoading", { ns: "settings" }) });
    }
    try {
      const current = await getSystemTime(locale);
      setInfo(current);
      setTimezone(current.timezone || "");
      setManualTime((value) => value || toDateTimeLocal(current.current_time));
      if (!current.time_management_supported) {
        setTimezones([]);
        if (!silent) {
          setBanner({ kind: "idle", message: "" });
        }
        return current;
      }
      const zones = await getSystemTimezones(locale);
      setTimezones(uniqueTimezones(zones, current.timezone));
      if (!silent) {
        setBanner({ kind: "success", message: t("systemTimeRefreshed", { ns: "settings" }) });
      }
      return current;
    } catch (error) {
      if (!silent) {
        setBanner({ kind: "error", message: extractErrorMessage(error, t("unexpectedError", { ns: "common" })) });
      }
      return null;
    } finally {
      if (!silent) {
        setLoading(false);
      }
    }
  }, [locale, t]);

  useEffect(() => {
    void loadTimeInfo();
    const timer = window.setInterval(() => {
      void loadTimeInfo(true);
    }, 30000);
    return () => window.clearInterval(timer);
  }, [loadTimeInfo]);

  useEffect(() => {
    if (!timezoneMenuOpen) {
      return;
    }
    const closeOnOutsideClick = (event: MouseEvent) => {
      if (!timezonePickerRef.current?.contains(event.target as Node)) {
        setTimezoneMenuOpen(false);
        setTimezoneQuery("");
      }
    };
    const closeOnEscape = (event: KeyboardEvent) => {
      if (event.key === "Escape") {
        setTimezoneMenuOpen(false);
        setTimezoneQuery("");
      }
    };
    document.addEventListener("mousedown", closeOnOutsideClick);
    document.addEventListener("keydown", closeOnEscape);
    return () => {
      document.removeEventListener("mousedown", closeOnOutsideClick);
      document.removeEventListener("keydown", closeOnEscape);
    };
  }, [timezoneMenuOpen]);

  const toggleNTP = async (enabled: boolean) => {
    setAction("ntp");
    setBanner({ kind: "loading", message: t("systemTimeNtpUpdating", { ns: "settings" }) });
    try {
      await updateSystemNTP(enabled, locale);
      const refreshed = await loadTimeInfo(true);
      if (!refreshed || refreshed.ntp_enabled !== enabled) {
        throw new Error(t("systemTimeStateMismatch", { ns: "settings" }));
      }
      setBanner({
        kind: "success",
        message: t(enabled ? "systemTimeNtpEnabled" : "systemTimeNtpDisabled", { ns: "settings" }),
      });
    } catch (error) {
      setBanner({ kind: "error", message: extractErrorMessage(error, t("unexpectedError", { ns: "common" })) });
    } finally {
      setAction("");
    }
  };

  const saveTimezone = async () => {
    if (!timezone) {
      setBanner({ kind: "error", message: t("systemTimeTimezoneRequired", { ns: "settings" }) });
      return;
    }
    setAction("timezone");
    setBanner({ kind: "loading", message: t("systemTimeTimezoneUpdating", { ns: "settings" }) });
    try {
      await updateSystemTimezone(timezone, locale);
      const refreshed = await loadTimeInfo(true);
      if (!refreshed || refreshed.timezone !== timezone) {
        throw new Error(t("systemTimeStateMismatch", { ns: "settings" }));
      }
      setBanner({ kind: "success", message: t("systemTimeTimezoneUpdated", { ns: "settings" }) });
    } catch (error) {
      setBanner({ kind: "error", message: extractErrorMessage(error, t("unexpectedError", { ns: "common" })) });
    } finally {
      setAction("");
    }
  };

  const saveManualTime = async () => {
    const value = toRemoteDateTime(manualTime);
    if (!value) {
      setBanner({ kind: "error", message: t("systemTimeManualTimeInvalid", { ns: "settings" }) });
      return;
    }
    setAction("manual");
    setBanner({ kind: "loading", message: t("systemTimeManualUpdating", { ns: "settings" }) });
    try {
      await updateSystemManualTime(value, locale);
      const refreshed = await loadTimeInfo(true);
      if (!refreshed || refreshed.ntp_enabled) {
        throw new Error(t("systemTimeStateMismatch", { ns: "settings" }));
      }
      setManualTime("");
      setBanner({ kind: "success", message: t("systemTimeManualUpdated", { ns: "settings" }) });
    } catch (error) {
      setBanner({ kind: "error", message: extractErrorMessage(error, t("unexpectedError", { ns: "common" })) });
    } finally {
      setAction("");
    }
  };

  const busy = action !== "";
  const supported = info?.time_management_supported === true;
  const normalizedTimezoneQuery = timezoneQuery.trim().toLowerCase();
  const filteredTimezones = timezones.filter((item) => item.toLowerCase().includes(normalizedTimezoneQuery));
  const visibleTimezones = timezone && !filteredTimezones.includes(timezone)
    ? [timezone, ...filteredTimezones]
    : filteredTimezones;
  const timezonePickerDisabled = busy || timezones.length === 0;

  return (
    <Panel>
      <PanelBody>
        <SectionHeader
          title={t("systemTimeTitle", { ns: "settings" })}
          description={t("systemTimeDescription", { ns: "settings" })}
          action={
            <button className="btn btn-sm btn-outline btn-info" type="button" disabled={loading || busy} onClick={() => void loadTimeInfo()}>
              <RefreshCw size={16} className={loading ? "app-spinner" : undefined} />
              <span>{t("refresh", { ns: "common" })}</span>
            </button>
          }
        />

        {banner.message ? <BannerAlert banner={banner} /> : null}
        {!info ? <p className="text-sm text-base-content/55">{t("systemTimeNoInfo", { ns: "settings" })}</p> : null}
        {info && !supported ? (
          <div className="alert alert-warning text-sm">
            <Clock3 size={16} />
            <span>{t("systemTimeUnsupported", { ns: "settings" })}</span>
          </div>
        ) : null}
        {info && supported ? (
          <>
            <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
              <InfoTile label={t("systemTimeCurrent", { ns: "settings" })} value={info.current_time || "-"} />
              <InfoTile label={t("systemTimeTimezone", { ns: "settings" })} value={info.timezone || "-"} />
              <InfoTile label={t("systemTimeUtcOffset", { ns: "settings" })} value={info.utc_offset || "-"} />
              <InfoTile
                label={t("systemTimeNtpSyncStatus", { ns: "settings" })}
                value={info.ntp_synced ? t("systemTimeNtpSynced", { ns: "settings" }) : t("systemTimeNtpNotSynced", { ns: "settings" })}
              />
            </div>

            <div className="grid gap-3 border-t border-base-300/80 pt-3">
              <div className="grid gap-3 rounded-2xl border border-base-300 bg-base-100/55 p-3 sm:grid-cols-[minmax(0,1fr)_auto] sm:items-center">
                <div className="flex min-w-0 items-start gap-2">
                  <RefreshCw size={17} className="mt-0.5 shrink-0 text-info" />
                  <div className="min-w-0">
                    <strong className="block text-sm">{t("systemTimeNtp", { ns: "settings" })}</strong>
                    <span className="mt-1 block text-xs leading-5 text-base-content/55">{t("systemTimeNtpDescription", { ns: "settings" })}</span>
                  </div>
                </div>
                <label className="flex items-center gap-2 justify-self-start sm:justify-self-end">
                  <span className="text-xs font-semibold text-base-content/65">
                    {info.ntp_enabled ? t("systemTimeNtpEnabledShort", { ns: "settings" }) : t("systemTimeNtpDisabledShort", { ns: "settings" })}
                  </span>
                  <input
                    className="toggle toggle-success"
                    type="checkbox"
                    aria-label={t("systemTimeNtp", { ns: "settings" })}
                    checked={info.ntp_enabled}
                    disabled={busy}
                    onChange={(event) => void toggleNTP(event.target.checked)}
                  />
                </label>
              </div>

              <div className="grid gap-3 rounded-2xl border border-base-300 bg-base-100/55 p-3 lg:grid-cols-[minmax(0,1fr)_minmax(0,1.2fr)_auto] lg:items-end">
                <label className="grid gap-1.5">
                  <span className="flex items-center gap-2 text-sm font-semibold">
                    <Globe2 size={16} className="text-info" />
                    {t("systemTimeTimezoneSelect", { ns: "settings" })}
                  </span>
                  <span className="text-xs leading-5 text-base-content/55">{t("systemTimeTimezoneHint", { ns: "settings" })}</span>
                </label>
                <div className="relative grid min-w-0 gap-2" ref={timezonePickerRef}>
                  <div className="input input-bordered input-sm flex w-full items-center gap-2 bg-base-100">
                    <Search size={15} className="shrink-0 text-base-content/45" aria-hidden="true" />
                    <input
                      className="min-w-0 grow bg-transparent outline-none"
                      type="search"
                      inputMode="text"
                      data-keyboard="ascii"
                      role="combobox"
                      aria-label={t("systemTimeTimezoneSearch", { ns: "settings" })}
                      aria-expanded={timezoneMenuOpen}
                      aria-controls="system-timezone-options"
                      aria-autocomplete="list"
                      autoComplete="off"
                      placeholder={timezone || t("systemTimeTimezonePlaceholder", { ns: "settings" })}
                      value={timezoneMenuOpen ? timezoneQuery : timezone}
                      readOnly={!timezoneMenuOpen}
                      disabled={timezonePickerDisabled}
                      onFocus={() => setTimezoneMenuOpen(true)}
                      onChange={(event) => {
                        setTimezoneQuery(event.target.value);
                        setTimezoneMenuOpen(true);
                      }}
                    />
                    <button
                      className="btn btn-ghost btn-xs shrink-0 px-1"
                      type="button"
                      aria-label={t("systemTimeTimezoneSelect", { ns: "settings" })}
                      aria-expanded={timezoneMenuOpen}
                      disabled={timezonePickerDisabled}
                      onClick={() => {
                        setTimezoneMenuOpen((open) => !open);
                        setTimezoneQuery("");
                      }}
                    >
                      <ChevronDown size={15} aria-hidden="true" />
                    </button>
                  </div>
                  {timezoneMenuOpen ? (
                    <div
                      id="system-timezone-options"
                      className="absolute inset-x-0 top-full z-30 max-h-56 overflow-y-auto rounded-box border border-base-300 bg-base-100 p-1 shadow-xl"
                      role="listbox"
                      aria-label={t("systemTimeTimezoneSelect", { ns: "settings" })}
                    >
                      {visibleTimezones.length > 0 ? visibleTimezones.map((item) => (
                        <button
                          className={`btn btn-ghost btn-sm h-auto min-h-0 w-full justify-start px-3 py-2 text-left font-normal ${item === timezone ? "bg-primary/10 text-primary" : ""}`}
                          key={item}
                          type="button"
                          role="option"
                          aria-selected={item === timezone}
                          onClick={() => {
                            setTimezone(item);
                            setTimezoneQuery("");
                            setTimezoneMenuOpen(false);
                          }}
                        >
                          {item}
                        </button>
                      )) : (
                        <span className="block px-3 py-2 text-sm text-base-content/55">
                          {t("systemTimeTimezoneNoMatch", { ns: "settings" })}
                        </span>
                      )}
                    </div>
                  ) : null}
                </div>
                <button className="btn btn-sm btn-primary" type="button" disabled={busy || !timezone} onClick={() => void saveTimezone()}>
                  <Save size={15} />
                  <span>{action === "timezone" ? t("systemTimeUpdating", { ns: "settings" }) : t("systemTimeApplyTimezone", { ns: "settings" })}</span>
                </button>
              </div>

              <div className="grid gap-3 rounded-2xl border border-base-300 bg-base-100/55 p-3 lg:grid-cols-[minmax(0,1fr)_minmax(0,1.2fr)_auto] lg:items-end">
                <label className="grid gap-1.5">
                  <span className="flex items-center gap-2 text-sm font-semibold">
                    <CalendarClock size={16} className="text-info" />
                    {t("systemTimeManual", { ns: "settings" })}
                  </span>
                  <span className="text-xs leading-5 text-base-content/55">{t("systemTimeManualHint", { ns: "settings" })}</span>
                </label>
                <input
                  className="input input-bordered input-sm w-full bg-base-100"
                  type="datetime-local"
                  step="1"
                  aria-label={t("systemTimeManual", { ns: "settings" })}
                  value={manualTime}
                  disabled={busy || info.ntp_enabled}
                  onChange={(event) => setManualTime(event.target.value)}
                />
                <button className="btn btn-sm btn-primary" type="button" disabled={busy || info.ntp_enabled || !manualTime} onClick={() => void saveManualTime()}>
                  <Clock3 size={15} />
                  <span>{action === "manual" ? t("systemTimeUpdating", { ns: "settings" }) : t("systemTimeSetManual", { ns: "settings" })}</span>
                </button>
              </div>
            </div>
          </>
        ) : null}
      </PanelBody>
    </Panel>
  );
}
