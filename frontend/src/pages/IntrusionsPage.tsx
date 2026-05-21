import { useCallback, useEffect, useMemo, useState } from "react";
import type { TFunction } from "i18next";
import { RefreshCw } from "lucide-react";

import { getIntrusions } from "../api";
import { Badge } from "../components/Badge";
import { Panel, PanelBody } from "../components/Panel";
import { SectionHeader } from "../components/SectionHeader";
import type { Banner } from "../app/types";
import type { Tone } from "../app/types";
import type { IntrusionRecord, IntrusionTargetType, ScreenPositionPoint } from "../types";
import { cx } from "../utils/classnames";
import { formatNumber, formatTime } from "../utils/format";
import { extractErrorMessage } from "../utils/session";

type IntrusionFilter = "all" | IntrusionTargetType;

const intrusionLimit = 200;
const intrusionFilters: IntrusionFilter[] = ["all", "detection", "position"];

function formatDuration(seconds: number, t: TFunction) {
  if (!Number.isFinite(seconds) || seconds <= 0) {
    return "-";
  }
  const total = Math.round(seconds);
  const minutes = Math.floor(total / 60);
  const rest = total % 60;
  if (minutes <= 0) {
    return t("intrusionDurationSeconds", { ns: "settings", value: rest });
  }
  return t("intrusionDurationMinutes", { ns: "settings", minutes, seconds: rest });
}

function formatFrequency(locale: string, value?: number) {
  if (typeof value !== "number" || !Number.isFinite(value) || value === 0) {
    return "-";
  }
  return `${formatNumber(locale, value, 1)} MHz`;
}

function formatRSSI(locale: string, value?: number) {
  if (typeof value !== "number" || !Number.isFinite(value) || value === 0) {
    return "-";
  }
  return `${formatNumber(locale, value, 0)} dBm`;
}

function formatOptionalMetric(locale: string, value: number | undefined, unit: string, digits = 1) {
  if (typeof value !== "number" || !Number.isFinite(value)) {
    return "-";
  }
  return `${formatNumber(locale, value, digits)} ${unit}`;
}

function formatPoint(point?: ScreenPositionPoint) {
  if (!point) {
    return "-";
  }
  return `${point.latitude.toFixed(6)}, ${point.longitude.toFixed(6)}`;
}

function coordinateSummary(record: IntrusionRecord, t: TFunction) {
  if (record.targetType !== "position") {
    return "-";
  }
  const parts: string[] = [];
  if (record.drone) {
    parts.push(`${t("intrusionDrone", { ns: "settings" })}: ${formatPoint(record.drone)}`);
  }
  if (record.pilot) {
    parts.push(`${t("intrusionPilot", { ns: "settings" })}: ${formatPoint(record.pilot)}`);
  }
  if (record.home) {
    parts.push(`${t("intrusionHome", { ns: "settings" })}: ${formatPoint(record.home)}`);
  }
  return parts.length > 0 ? parts.join(" / ") : "-";
}

function targetTypeLabel(type: IntrusionTargetType, t: TFunction) {
  return t(type === "position" ? "intrusionTypePosition" : "intrusionTypeDetection", { ns: "settings" });
}

function targetTypeTone(type: IntrusionTargetType): Tone {
  return type === "position" ? "success" : "info";
}

export function IntrusionsPage({
  locale,
  t,
}: {
  locale: string;
  t: TFunction;
}) {
  const [records, setRecords] = useState<IntrusionRecord[]>([]);
  const [filter, setFilter] = useState<IntrusionFilter>("all");
  const [banner, setBanner] = useState<Banner>({ kind: "idle", message: "" });
  const [loading, setLoading] = useState(false);

  const loadRecords = useCallback(async () => {
    setLoading(true);
    setBanner({ kind: "loading", message: t("loading", { ns: "common" }) });
    try {
      const response = await getIntrusions(locale, intrusionLimit, filter);
      setRecords(response.items);
      setBanner({ kind: "idle", message: "" });
    } catch (error) {
      setBanner({ kind: "error", message: extractErrorMessage(error, t("unexpectedError", { ns: "common" })) });
    } finally {
      setLoading(false);
    }
  }, [filter, locale, t]);

  useEffect(() => {
    void loadRecords();
  }, [loadRecords]);

  const totalTrajectoryCount = useMemo(
    () => records.reduce((sum, record) => sum + (record.droneTrajectory?.length ?? 0) + (record.pilotTrajectory?.length ?? 0), 0),
    [records],
  );

  return (
    <section className="flex min-h-0 min-w-0 flex-1">
      <Panel className="flex min-h-0 min-w-0 flex-1 flex-col">
        <PanelBody className="min-h-0 min-w-0 flex-1">
          <SectionHeader
            title={t("intrusionsTitle", { ns: "settings" })}
            description={t("intrusionsDescription", { ns: "settings" })}
            action={
              <button className="btn btn-sm btn-outline btn-info" type="button" onClick={() => void loadRecords()} disabled={loading}>
                <RefreshCw size={16} className={loading ? "animate-spin" : undefined} />
                <span>{t("refresh", { ns: "common" })}</span>
              </button>
            }
          />

          <div className="flex flex-wrap items-center gap-2">
            <div className="join">
              {intrusionFilters.map((item) => (
                <button
                  key={item}
                  className={cx("btn btn-sm join-item", filter === item ? "btn-primary" : "btn-outline")}
                  type="button"
                  onClick={() => setFilter(item)}
                >
                  {t(`intrusionFilter.${item}`, { ns: "settings" })}
                </button>
              ))}
            </div>
            <span className="text-xs text-base-content/60">
              {t("intrusionCount", { ns: "settings", value: records.length })} · {t("intrusionTrajectoryCount", { ns: "settings", value: totalTrajectoryCount })}
            </span>
          </div>

          {banner.kind === "error" && banner.message ? (
            <div className="alert alert-soft alert-error py-3 text-sm" role="alert">
              <span className="min-w-0 [overflow-wrap:anywhere]">{banner.message}</span>
            </div>
          ) : null}

          <div className="min-h-0 min-w-0 flex-1 overflow-auto rounded-2xl border border-base-300 bg-base-100/70">
            <table className="table table-zebra table-sm w-full min-w-[112rem] table-fixed whitespace-nowrap">
              <thead className="sticky top-0 z-10 bg-base-200">
                <tr>
                  <th className="w-[8rem]">{t("intrusionType", { ns: "settings" })}</th>
                  <th className="w-[16rem]">{t("intrusionModel", { ns: "settings" })}</th>
                  <th className="w-[18rem]">{t("intrusionIdentity", { ns: "settings" })}</th>
                  <th className="w-[9rem]">{t("intrusionFrequency", { ns: "settings" })}</th>
                  <th className="w-[8rem]">{t("intrusionRssi", { ns: "settings" })}</th>
                  <th className="w-[13rem]">{t("intrusionFirstSeen", { ns: "settings" })}</th>
                  <th className="w-[13rem]">{t("intrusionLastSeen", { ns: "settings" })}</th>
                  <th className="w-[8rem]">{t("intrusionDuration", { ns: "settings" })}</th>
                  <th className="w-[7rem]">{t("intrusionHitCount", { ns: "settings" })}</th>
                  <th className="w-[22rem]">{t("intrusionCoordinates", { ns: "settings" })}</th>
                  <th className="w-[9rem]">{t("intrusionSpeed", { ns: "settings" })}</th>
                  <th className="w-[9rem]">{t("intrusionHeight", { ns: "settings" })}</th>
                  <th className="w-[10rem]">{t("intrusionSource", { ns: "settings" })}</th>
                </tr>
              </thead>
              <tbody>
                {records.length === 0 ? (
                  <tr>
                    <td colSpan={13} className="p-3">
                      <div className="admin-empty-state admin-empty-state--table">
                        {loading ? t("loading", { ns: "common" }) : t("empty", { ns: "common" })}
                      </div>
                    </td>
                  </tr>
                ) : (
                  records.map((record) => (
                    <tr key={record.id} className="row-hover">
                      <td>
                        <Badge tone={targetTypeTone(record.targetType)}>{targetTypeLabel(record.targetType, t)}</Badge>
                      </td>
                      <td>
                        <span className="block truncate" title={record.model || "-"}>
                          {record.model || "-"}
                        </span>
                      </td>
                      <td>
                        <code className="block truncate rounded-xl bg-base-200/80 px-2 py-1 text-xs" title={record.serial || record.device || record.targetId}>
                          {record.serial || record.device || record.targetId}
                        </code>
                      </td>
                      <td className="tabular-nums">{formatFrequency(locale, record.frequency)}</td>
                      <td className="tabular-nums">{formatRSSI(locale, record.rssi)}</td>
                      <td className="tabular-nums">
                        <span className="block truncate" title={formatTime(locale, record.firstSeen)}>
                          {formatTime(locale, record.firstSeen)}
                        </span>
                      </td>
                      <td className="tabular-nums">
                        <span className="block truncate" title={formatTime(locale, record.lastSeen)}>
                          {formatTime(locale, record.lastSeen)}
                        </span>
                      </td>
                      <td className="tabular-nums">{formatDuration(record.durationSeconds, t)}</td>
                      <td className="tabular-nums">{formatNumber(locale, record.hitCount, 0)}</td>
                      <td>
                        <span className="block truncate font-mono text-xs tabular-nums text-base-content/80" title={coordinateSummary(record, t)}>
                          {coordinateSummary(record, t)}
                        </span>
                      </td>
                      <td className="tabular-nums">{formatOptionalMetric(locale, record.speed, "m/s", 1)}</td>
                      <td className="tabular-nums">{formatOptionalMetric(locale, record.height, "m", 0)}</td>
                      <td>
                        <span className="block truncate" title={record.source || "-"}>
                          {record.source || "-"}
                        </span>
                      </td>
                    </tr>
                  ))
                )}
              </tbody>
            </table>
          </div>
        </PanelBody>
      </Panel>
    </section>
  );
}
