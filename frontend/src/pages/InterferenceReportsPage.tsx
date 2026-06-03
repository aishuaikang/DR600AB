import { useCallback, useEffect, useMemo, useState } from "react";
import type { TFunction } from "i18next";
import { ChevronDown, RefreshCw, Trash2 } from "lucide-react";

import { deleteFailedInterferenceReport, getInterferenceReports } from "../api";
import { Badge } from "../components/Badge";
import { Panel, PanelBody } from "../components/Panel";
import { SectionHeader } from "../components/SectionHeader";
import type { Banner, Tone } from "../app/types";
import type {
  InterferenceReportStatus,
  InterferenceReportSummary,
  UserSettings,
} from "../types";
import { cx } from "../utils/classnames";
import { formatTime } from "../utils/format";
import { extractErrorMessage } from "../utils/session";

type ReportFilter = "all" | InterferenceReportStatus;

const reportPageSize = 50;
const reportFilters: ReportFilter[] = ["all", "running", "completed", "failed", "abnormal"];
const defaultInterferenceBandsById: Record<string, string> = {
  io1: "433M/800M/900M/1.4G",
  io2: "1.2G/1.5G",
  io3: "2.4G/5.2G/5.8G",
};
const defaultInterferenceBandsByGPIO: Record<string, string> = {
  IO2: defaultInterferenceBandsById.io1,
  IO3: defaultInterferenceBandsById.io2,
  IO1: defaultInterferenceBandsById.io3,
  IOC4: defaultInterferenceBandsById.io1,
  IOC2: defaultInterferenceBandsById.io2,
  IOC3: defaultInterferenceBandsById.io3,
  GPIO20: defaultInterferenceBandsById.io1,
  GPIO18: defaultInterferenceBandsById.io2,
  GPIO19: defaultInterferenceBandsById.io3,
};
const strikeChannelIdOrder = ["io1", "io2", "io3"];

function appendReports(
  current: InterferenceReportSummary[],
  incoming: InterferenceReportSummary[],
) {
  const existingIds = new Set(current.map((item) => item.id));
  const next = [...current];
  for (const item of incoming) {
    if (existingIds.has(item.id)) {
      continue;
    }
    existingIds.add(item.id);
    next.push(item);
  }
  return next;
}

function formatReportDateKey(value: string) {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return "";
  }
  const year = String(date.getFullYear());
  const month = String(date.getMonth() + 1).padStart(2, "0");
  const day = String(date.getDate()).padStart(2, "0");
  return `${year}-${month}-${day}`;
}

function formatDuration(seconds: number, t: TFunction) {
  if (!Number.isFinite(seconds) || seconds <= 0) {
    return "-";
  }
  const total = Math.round(seconds);
  const minutes = Math.floor(total / 60);
  const rest = total % 60;
  if (minutes <= 0) {
    return t("interferenceReportDurationSeconds", { ns: "settings", value: rest });
  }
  return t("interferenceReportDurationMinutes", { ns: "settings", minutes, seconds: rest });
}

function statusLabel(status: InterferenceReportStatus, t: TFunction) {
  return t(`interferenceReportStatus.${status}`, { ns: "settings" });
}

function statusTone(status: InterferenceReportStatus): Tone {
  switch (status) {
    case "running":
      return "info";
    case "completed":
      return "success";
    case "failed":
      return "error";
    case "abnormal":
      return "warning";
    default:
      return "neutral";
  }
}

function configuredBandById(id: string, customLabels: string[]) {
  const index = strikeChannelIdOrder.indexOf(id);
  if (index < 0) {
    return "";
  }
  return customLabels[index]?.trim() ?? "";
}

function displayBandLabel(label: string, id: string, customLabels: string[]) {
  const value = label.trim();
  if (value && !defaultInterferenceBandsByGPIO[value]) {
    return value;
  }
  return (
    configuredBandById(id, customLabels) ||
    defaultInterferenceBandsByGPIO[value] ||
    defaultInterferenceBandsById[id] ||
    value ||
    id
  );
}

function channelLabel(report: InterferenceReportSummary, customLabels: string[]) {
  const ids = report.channelIds?.filter(Boolean) ?? [];
  const labels = report.channelLabels?.filter(Boolean) ?? [];
  if (labels.length > 0) {
    return labels
      .map((label, index) => displayBandLabel(label, ids[index] ?? "", customLabels))
      .filter(Boolean)
      .join(", ");
  }
  return ids.length > 0 ? ids.map((id) => displayBandLabel("", id, customLabels)).join(", ") : "-";
}

export function InterferenceReportsPage({
  locale,
  userSettings,
  t,
}: {
  locale: string;
  userSettings: UserSettings;
  t: TFunction;
}) {
  const [reports, setReports] = useState<InterferenceReportSummary[]>([]);
  const [filter, setFilter] = useState<ReportFilter>("all");
  const [reportDateFrom, setReportDateFrom] = useState("");
  const [reportDateTo, setReportDateTo] = useState("");
  const [banner, setBanner] = useState<Banner>({ kind: "idle", message: "" });
  const [loading, setLoading] = useState(false);
  const [loadingMore, setLoadingMore] = useState(false);
  const [hasMore, setHasMore] = useState(false);
  const [nextOffset, setNextOffset] = useState(0);
  const [deletingId, setDeletingId] = useState("");
  const strikeChannelLabels = userSettings.screenStrikeChannelLabels ?? [];

  const loadReports = useCallback(async (options?: { append?: boolean; offset?: number; preserveBanner?: boolean }) => {
    const append = Boolean(options?.append);
    const offset = append ? options?.offset ?? 0 : 0;
    if (append) {
      setLoadingMore(true);
    } else {
      setLoading(true);
    }
    if (!append && !options?.preserveBanner) {
      setBanner({ kind: "loading", message: t("loading", { ns: "common" }) });
    }
    try {
      const response = await getInterferenceReports(locale, reportPageSize, filter, offset);
      const items = response.items ?? [];
      setReports((current) => (append ? appendReports(current, items) : items));
      setHasMore(Boolean(response.hasMore));
      setNextOffset(response.hasMore ? response.nextOffset ?? offset + items.length : 0);
      if (!append && !options?.preserveBanner) {
        setBanner({ kind: "idle", message: "" });
      }
    } catch (error) {
      setBanner({ kind: "error", message: extractErrorMessage(error, t("unexpectedError", { ns: "common" })) });
    } finally {
      if (append) {
        setLoadingMore(false);
      } else {
        setLoading(false);
      }
    }
  }, [filter, locale, t]);

  useEffect(() => {
    void loadReports();
  }, [loadReports]);

  const handleReportDateFromChange = useCallback((value: string) => {
    setReportDateFrom(value);
    setReportDateTo((currentTo) => (value && currentTo && currentTo < value ? value : currentTo));
  }, []);

  const handleReportDateToChange = useCallback((value: string) => {
    setReportDateTo(value && reportDateFrom && value < reportDateFrom ? reportDateFrom : value);
  }, [reportDateFrom]);

  const visibleReports = useMemo(() => {
    return reports.filter((report) => {
      const reportDate = formatReportDateKey(report.startedAt);
      if (reportDateFrom && reportDate < reportDateFrom) {
        return false;
      }
      if (reportDateTo && reportDate > reportDateTo) {
        return false;
      }
      return true;
    });
  }, [reportDateFrom, reportDateTo, reports]);

  const visibleCounters = useMemo(() => {
    const initial: Record<InterferenceReportStatus, number> = {
      running: 0,
      completed: 0,
      failed: 0,
      abnormal: 0,
    };
    for (const report of visibleReports) {
      initial[report.status] += 1;
    }
    return initial;
  }, [visibleReports]);

  const hasReportFilters = Boolean(reportDateFrom || reportDateTo);

  const clearReportFilters = useCallback(() => {
    setReportDateFrom("");
    setReportDateTo("");
  }, []);

  const deleteFailedReport = async (report: InterferenceReportSummary) => {
    if (report.status !== "failed" || deletingId) {
      return;
    }
    const confirmed = window.confirm(t("interferenceReportDeleteConfirmDescription", { ns: "settings" }));
    if (!confirmed) {
      return;
    }
    setDeletingId(report.id);
    setBanner({ kind: "idle", message: "" });
    try {
      const response = await deleteFailedInterferenceReport(report.id, locale);
      setReports((items) => items.filter((item) => item.id !== report.id));
      setBanner({ kind: "success", message: t("interferenceReportDeleteSuccess", { ns: "settings", count: response.deleted }) });
      void loadReports({ preserveBanner: true });
    } catch (error) {
      setBanner({ kind: "error", message: extractErrorMessage(error, t("unexpectedError", { ns: "common" })) });
    } finally {
      setDeletingId("");
    }
  };

  return (
    <section className="flex min-h-0 min-w-0 flex-1">
      <Panel className="flex min-h-0 min-w-0 flex-1 flex-col">
        <PanelBody className="min-h-0 min-w-0 flex-1">
          <SectionHeader
            title={t("interferenceReportsTitle", { ns: "settings" })}
            description={t("interferenceReportsDescription", { ns: "settings" })}
            action={
              <button className="btn btn-sm btn-outline btn-info" type="button" onClick={() => void loadReports()} disabled={loading || loadingMore || Boolean(deletingId)}>
                <RefreshCw size={16} className={loading ? "animate-spin" : undefined} />
                <span>{t("refresh", { ns: "common" })}</span>
              </button>
            }
          />

          <div className="flex flex-wrap items-center gap-2">
            <div className="join">
              {reportFilters.map((item) => (
                <button
                  key={item}
                  className={cx("btn btn-sm join-item", filter === item ? "btn-primary" : "btn-outline")}
                  type="button"
                  onClick={() => setFilter(item)}
                >
                  {item === "all" ? t("interferenceReportFilter.all", { ns: "settings" }) : statusLabel(item, t)}
                </button>
              ))}
            </div>
            <span className="text-xs text-base-content/60">
              {t("interferenceReportCount", { ns: "settings", value: visibleReports.length })} · {t("interferenceReportRunningCount", { ns: "settings", value: visibleCounters.running })}
            </span>
          </div>

          <div className="flex flex-wrap items-end gap-2">
            <div className="flex min-w-0 flex-col gap-1 text-xs text-base-content/60">
              <span>{t("interferenceReportDateRange", { ns: "settings" })}</span>
              <div className="flex flex-wrap items-end gap-2">
                <label className="flex min-w-0 flex-col gap-1">
                  <span>{t("interferenceReportDateFrom", { ns: "settings" })}</span>
                  <input
                    className="input input-bordered input-sm w-44 bg-base-100"
                    type="date"
                    value={reportDateFrom}
                    onChange={(event) => handleReportDateFromChange(event.target.value)}
                  />
                </label>
                <label className="flex min-w-0 flex-col gap-1">
                  <span>{t("interferenceReportDateTo", { ns: "settings" })}</span>
                  <input
                    className="input input-bordered input-sm w-44 bg-base-100"
                    type="date"
                    min={reportDateFrom || undefined}
                    value={reportDateTo}
                    onChange={(event) => handleReportDateToChange(event.target.value)}
                  />
                </label>
              </div>
            </div>
            <button className="btn btn-sm btn-ghost" type="button" onClick={clearReportFilters} disabled={!hasReportFilters}>
              {t("clear", { ns: "common" })}
            </button>
          </div>

          {(banner.kind === "error" || banner.kind === "success") && banner.message ? (
            <div className={cx("alert alert-soft py-3 text-sm", banner.kind === "error" ? "alert-error" : "alert-success")} role="alert">
              <span className="min-w-0 [overflow-wrap:anywhere]">{banner.message}</span>
            </div>
          ) : null}

          <div className="min-h-0 min-w-0 flex-1 overflow-auto rounded-2xl border border-base-300 bg-base-100/70">
            <table className="interference-reports-table table table-zebra table-sm w-full min-w-[56rem]">
              <thead className="sticky top-0 z-10 bg-base-200">
                <tr>
                  <th className="w-[8rem]">{t("interferenceReportStatus", { ns: "settings" })}</th>
                  <th className="w-[13rem]">{t("interferenceReportStartedAt", { ns: "settings" })}</th>
                  <th className="w-[13rem]">{t("interferenceReportEndedAt", { ns: "settings" })}</th>
                  <th className="w-[8rem]">{t("interferenceReportDuration", { ns: "settings" })}</th>
                  <th className="w-[16rem]">{t("interferenceReportChannels", { ns: "settings" })}</th>
                  <th className="w-[8rem]">{t("interferenceReportRequestedDuration", { ns: "settings" })}</th>
                  <th className="w-[16rem]">{t("interferenceReportError", { ns: "settings" })}</th>
                  <th className="w-[10rem]">{t("interferenceReportActions", { ns: "settings" })}</th>
                </tr>
              </thead>
              <tbody>
                {visibleReports.length === 0 ? (
                  <tr>
                    <td colSpan={8} className="p-3">
                      <div className="admin-empty-state admin-empty-state--table">
                        {loading
                          ? t("loading", { ns: "common" })
                          : hasReportFilters
                            ? t("interferenceReportNoMatch", { ns: "settings" })
                            : t("empty", { ns: "common" })}
                      </div>
                    </td>
                  </tr>
                ) : (
                  visibleReports.map((report) => (
                    <tr key={report.id} className="row-hover">
                      <td><Badge tone={statusTone(report.status)}>{statusLabel(report.status, t)}</Badge></td>
                      <td className="tabular-nums whitespace-normal break-words">{formatTime(locale, report.startedAt)}</td>
                      <td className="tabular-nums whitespace-normal break-words">{formatTime(locale, report.endedAt)}</td>
                      <td className="tabular-nums whitespace-normal break-words">{formatDuration(report.durationSeconds, t)}</td>
                      <td className="whitespace-normal break-words">{channelLabel(report, strikeChannelLabels)}</td>
                      <td className="tabular-nums whitespace-normal break-words">{formatDuration(report.requestedDurationSeconds ?? 0, t)}</td>
                      <td className={cx(report.lastError && "text-error", "whitespace-normal break-words")}>{report.lastError || report.abnormalReason || "-"}</td>
                      <td>
                        <div className="flex items-center gap-1">
                          {report.status === "failed" ? (
                            <button
                              className="btn btn-ghost btn-xs h-8 min-h-8 rounded-xl text-error"
                              type="button"
                              disabled={deletingId === report.id}
                              title={t("interferenceReportDeleteFailed", { ns: "settings" })}
                              onClick={() => void deleteFailedReport(report)}
                            >
                              <Trash2 size={14} />
                              <span>{deletingId === report.id ? t("loading", { ns: "common" }) : t("delete", { ns: "common", defaultValue: t("interferenceReportDeleteFailed", { ns: "settings" }) })}</span>
                            </button>
                          ) : (
                            <span className="text-base-content/45">-</span>
                          )}
                        </div>
                      </td>
                    </tr>
                  ))
                )}
              </tbody>
            </table>
          </div>
          {hasMore ? (
            <div className="flex justify-center">
              <button
                className="btn btn-sm btn-outline"
                type="button"
                disabled={loading || loadingMore || Boolean(deletingId)}
                onClick={() => void loadReports({ append: true, offset: nextOffset, preserveBanner: true })}
              >
                <ChevronDown size={15} aria-hidden="true" />
                <span>{loadingMore ? t("loading", { ns: "common" }) : t("loadMore", { ns: "common" })}</span>
              </button>
            </div>
          ) : null}
        </PanelBody>
      </Panel>
    </section>
  );
}
