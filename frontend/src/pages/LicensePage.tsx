import { useEffect, useMemo, useState } from "react";
import type { TFunction } from "i18next";
import { CheckCircle2, Copy, RefreshCw, ShieldAlert, ShieldCheck } from "lucide-react";

import type { Banner } from "../app/types";
import { BannerAlert } from "../components/BannerAlert";
import { InfoTile } from "../components/InfoTile";
import { LoadingSpinner } from "../components/LoadingState";
import { Panel, PanelBody } from "../components/Panel";
import { PortSelect } from "../components/PortSelect";
import { SectionHeader } from "../components/SectionHeader";
import {
  SERIAL_BAUD_RATE_LIMITS,
  normalizeSerialBaudRate,
} from "../serial-profile";
import type { DetectionSessionResponse, LicenseInfo, PortInfo } from "../types";
import { cx } from "../utils/classnames";

function formatDateTime(value: string | undefined, locale: string) {
  if (!value) {
    return "-";
  }
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return value;
  }
  return date.toLocaleString(locale, { hour12: false });
}

function formatRemainingDays(license: LicenseInfo, t: TFunction) {
  if (license.isPermanent) {
    return t("license.permanent", { ns: "common" });
  }
  if (typeof license.remainingDays !== "number") {
    return "-";
  }
  return t("license.remainingDays", { ns: "common", count: license.remainingDays });
}

function formatBaudRate(value: number) {
  return String(normalizeSerialBaudRate(value));
}

function copyTextFallback(text: string) {
  const textarea = document.createElement("textarea");
  textarea.value = text;
  textarea.setAttribute("readonly", "true");
  textarea.style.position = "fixed";
  textarea.style.left = "-9999px";
  textarea.style.top = "0";
  textarea.style.opacity = "0";

  document.body.appendChild(textarea);
  textarea.focus();
  textarea.select();
  textarea.setSelectionRange(0, text.length);

  try {
    return document.execCommand("copy");
  } finally {
    document.body.removeChild(textarea);
  }
}

async function copyText(text: string) {
  if (!text) {
    return false;
  }

  if (copyTextFallback(text)) {
    return true;
  }

  if (window.isSecureContext && navigator.clipboard?.writeText) {
    try {
      await navigator.clipboard.writeText(text);
      return true;
    } catch {
      return false;
    }
  }

  return false;
}

export function LicensePage({
  appTitle,
  license,
  loading,
  locale,
  ports,
  banner,
  session,
  selectedReceivePort,
  selectedSendPort,
  selectedDetectionRxBaudRate,
  selectedDetectionTxBaudRate,
  t,
  onRefreshLicense,
  onRefreshPorts,
  onReceivePortChange,
  onSendPortChange,
  onDetectionRxBaudRateChange,
  onDetectionTxBaudRateChange,
}: {
  appTitle: string;
  license: LicenseInfo | null;
  loading: boolean;
  locale: string;
  ports: PortInfo[];
  banner: Banner;
  session: DetectionSessionResponse | null;
  selectedReceivePort: string;
  selectedSendPort: string;
  selectedDetectionRxBaudRate: number;
  selectedDetectionTxBaudRate: number;
  t: TFunction;
  onRefreshLicense: () => Promise<void>;
  onRefreshPorts: () => Promise<void>;
  onReceivePortChange: (value: string) => void;
  onSendPortChange: (value: string) => void;
  onDetectionRxBaudRateChange: (value: number) => void;
  onDetectionTxBaudRateChange: (value: number) => void;
}) {
  const [copyDone, setCopyDone] = useState(false);
  const [detectionRxBaudRateDraft, setDetectionRxBaudRateDraft] = useState(() => formatBaudRate(selectedDetectionRxBaudRate));
  const [detectionTxBaudRateDraft, setDetectionTxBaudRateDraft] = useState(() => formatBaudRate(selectedDetectionTxBaudRate));
  const deviceSN = license?.deviceSn || "";

  useEffect(() => {
    setDetectionRxBaudRateDraft(formatBaudRate(selectedDetectionRxBaudRate));
  }, [selectedDetectionRxBaudRate]);

  useEffect(() => {
    setDetectionTxBaudRateDraft(formatBaudRate(selectedDetectionTxBaudRate));
  }, [selectedDetectionTxBaudRate]);

  const statusTone = useMemo(() => {
    if (license?.valid) {
      return "text-success";
    }
    if (license?.code === "device_sn_missing") {
      return "text-warning";
    }
    return "text-error";
  }, [license]);

  const commitDetectionRxBaudRate = () => {
    const nextBaudRate = normalizeSerialBaudRate(Number(detectionRxBaudRateDraft), selectedDetectionRxBaudRate);
    setDetectionRxBaudRateDraft(formatBaudRate(nextBaudRate));
    if (nextBaudRate !== selectedDetectionRxBaudRate) {
      onDetectionRxBaudRateChange(nextBaudRate);
    }
  };

  const commitDetectionTxBaudRate = () => {
    const nextBaudRate = normalizeSerialBaudRate(Number(detectionTxBaudRateDraft), selectedDetectionTxBaudRate);
    setDetectionTxBaudRateDraft(formatBaudRate(nextBaudRate));
    if (nextBaudRate !== selectedDetectionTxBaudRate) {
      onDetectionTxBaudRateChange(nextBaudRate);
    }
  };

  const copyDeviceSN = async () => {
    if (!deviceSN) {
      return;
    }
    if (await copyText(deviceSN)) {
      setCopyDone(true);
      window.setTimeout(() => setCopyDone(false), 1500);
    } else {
      setCopyDone(false);
    }
  };

  return (
    <div className="min-h-dvh bg-base-100 text-base-content">
      <main className="mx-auto flex min-h-dvh w-full max-w-5xl flex-col gap-3 px-3 py-4 sm:px-5 lg:py-6">
        <Panel>
          <PanelBody className="gap-4">
            <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
              <div className="min-w-0">
                <p className="text-xs font-semibold text-primary">{appTitle}</p>
                <h1 className="mt-1 text-xl font-semibold leading-8 text-base-content">
                  {t("license.title", { ns: "common" })}
                </h1>
                <p className="mt-1 max-w-3xl text-sm leading-6 text-base-content/65">
                  {t("license.description", { ns: "common" })}
                </p>
              </div>
              <button
                className="btn btn-sm btn-outline shrink-0"
                type="button"
                disabled={loading}
                onClick={() => void onRefreshLicense()}
              >
                {loading ? <LoadingSpinner size={16} /> : <RefreshCw size={16} />}
                <span>{t("refresh", { ns: "common" })}</span>
              </button>
            </div>

            <div className="grid gap-3 md:grid-cols-3">
              <InfoTile label={t("license.currentStatus", { ns: "common" })}>
                <span className={cx("inline-flex items-center gap-1.5", statusTone)}>
                  {license?.valid ? <ShieldCheck size={16} /> : <ShieldAlert size={16} />}
                  {license?.valid ? t("license.valid", { ns: "common" }) : t("license.invalid", { ns: "common" })}
                </span>
              </InfoTile>
              <InfoTile label={t("license.deviceSn", { ns: "common" })}>
                <span className="flex min-w-0 items-center gap-2">
                  <span className="min-w-0 break-all font-mono text-xs">{deviceSN || t("license.noDeviceSn", { ns: "common" })}</span>
                  {deviceSN ? (
                    <button
                      className="btn btn-square btn-ghost btn-xs shrink-0"
                      type="button"
                      title={t("license.copyDeviceSn", { ns: "common" })}
                      aria-label={t("license.copyDeviceSn", { ns: "common" })}
                      onClick={() => void copyDeviceSN()}
                    >
                      {copyDone ? <CheckCircle2 size={15} /> : <Copy size={15} />}
                    </button>
                  ) : null}
                </span>
              </InfoTile>
              <InfoTile label={t("license.remaining", { ns: "common" })} value={license ? formatRemainingDays(license, t) : "-"} />
            </div>

            <div className="grid gap-3 md:grid-cols-2">
              <InfoTile label={t("license.issuedAt", { ns: "common" })} value={formatDateTime(license?.issuedAt, locale)} />
              <InfoTile label={t("license.expiresAt", { ns: "common" })} value={formatDateTime(license?.expiresAt, locale)} />
            </div>

            {license?.message && !license.valid ? (
              <div className="alert alert-warning alert-soft py-3 text-sm">
                <ShieldAlert size={16} />
                <span className="min-w-0 [overflow-wrap:anywhere]">{license.message}</span>
              </div>
            ) : null}

            <BannerAlert banner={banner} />
          </PanelBody>
        </Panel>

        {!deviceSN ? (
          <Panel>
            <PanelBody>
              <SectionHeader
                title={t("license.recoveryTitle", { ns: "common" })}
                description={t("license.recoveryDescription", { ns: "common" })}
                action={
                  <button className="btn btn-sm btn-outline btn-info" type="button" onClick={() => void onRefreshPorts()}>
                    <RefreshCw size={16} />
                    <span>{t("refresh", { ns: "common" })}</span>
                  </button>
                }
              />

              <div className="grid gap-3 md:grid-cols-3">
                <InfoTile label={t("license.detectionState", { ns: "common" })}>
                  {session?.message || (session?.active ? t("active", { ns: "common" }) : t("idle", { ns: "common" }))}
                </InfoTile>
                <InfoTile label={t("license.currentReceivePort", { ns: "common" })} value={session?.rxPortName || session?.portName || selectedReceivePort || "-"} />
                <InfoTile label={t("license.currentSendPort", { ns: "common" })} value={session?.txPortName || selectedSendPort || "-"} />
              </div>

              <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-4">
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
                  <span className="text-xs font-medium text-base-content/60">{t("detectionRxBaudRate", { ns: "settings" })}</span>
                  <input
                    className="input input-bordered input-sm w-full bg-base-100"
                    type="number"
                    inputMode="numeric"
                    min={SERIAL_BAUD_RATE_LIMITS.min}
                    max={SERIAL_BAUD_RATE_LIMITS.max}
                    step={100}
                    value={detectionRxBaudRateDraft}
                    onChange={(event) => setDetectionRxBaudRateDraft(event.target.value)}
                    onBlur={commitDetectionRxBaudRate}
                    onKeyDown={(event) => {
                      if (event.key === "Enter") {
                        event.currentTarget.blur();
                      }
                    }}
                  />
                  <span className="text-xs leading-5 text-base-content/50">
                    {t("detectionRxBaudRateHint", { ns: "settings" })}
                  </span>
                </label>
                <label className="grid gap-1.5">
                  <span className="text-xs font-medium text-base-content/60">{t("detectionTxBaudRate", { ns: "settings" })}</span>
                  <input
                    className="input input-bordered input-sm w-full bg-base-100"
                    type="number"
                    inputMode="numeric"
                    min={SERIAL_BAUD_RATE_LIMITS.min}
                    max={SERIAL_BAUD_RATE_LIMITS.max}
                    step={100}
                    value={detectionTxBaudRateDraft}
                    onChange={(event) => setDetectionTxBaudRateDraft(event.target.value)}
                    onBlur={commitDetectionTxBaudRate}
                    onKeyDown={(event) => {
                      if (event.key === "Enter") {
                        event.currentTarget.blur();
                      }
                    }}
                  />
                  <span className="text-xs leading-5 text-base-content/50">
                    {t("detectionTxBaudRateHint", { ns: "settings" })}
                  </span>
                </label>
              </div>

              {ports.length === 0 ? <span className="text-sm text-base-content/55">{t("noPorts", { ns: "settings" })}</span> : null}
            </PanelBody>
          </Panel>
        ) : null}
      </main>
    </div>
  );
}
