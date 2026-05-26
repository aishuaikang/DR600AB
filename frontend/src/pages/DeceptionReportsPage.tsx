import { useCallback, useEffect, useMemo, useState } from "react";
import type { TFunction } from "i18next";
import { ChevronDown, RefreshCw, Trash2 } from "lucide-react";

import { deleteFailedDeceptionReport, getDeceptionReports } from "../api";
import { Badge } from "../components/Badge";
import { Panel, PanelBody } from "../components/Panel";
import { SectionHeader } from "../components/SectionHeader";
import type { Banner, Tone } from "../app/types";
import type { DeceptionReportStatus, DeceptionReportSummary } from "../types";
import { cx } from "../utils/classnames";
import { formatTime } from "../utils/format";
import { extractErrorMessage } from "../utils/session";

type ReportFilter = "all" | DeceptionReportStatus;
type ReportModeFilter = "all" | "fixed_point" | "circle" | "linear";

const reportPageSize = 50;
const reportFilters: ReportFilter[] = ["all", "running", "completed", "failed", "abnormal"];
const reportModeFilters: ReportModeFilter[] = ["all", "fixed_point", "circle", "linear"];

function appendReports(
  current: DeceptionReportSummary[],
  incoming: DeceptionReportSummary[],
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
    return t("deceptionReportDurationSeconds", { ns: "settings", value: rest });
  }
  return t("deceptionReportDurationMinutes", { ns: "settings", minutes, seconds: rest });
}

function statusLabel(status: DeceptionReportStatus, t: TFunction) {
  return t(`deceptionReportStatus.${status}`, { ns: "settings" });
}

function statusTone(status: DeceptionReportStatus): Tone {
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

function reportModeLabel(mode: string | undefined, t: TFunction) {
  switch (mode) {
    case "fixed_point":
      return t("deceptionModes.fixedPoint", { ns: "screen" });
    case "circle":
      return t("deceptionModes.circle", { ns: "screen" });
    case "linear":
      return t("deceptionModes.linear", { ns: "screen" });
    default:
      return mode || "-";
  }
}

export function DeceptionReportsPage({
  locale,
  t,
}: {
  locale: string;
  t: TFunction;
}) {
  const [reports, setReports] = useState<DeceptionReportSummary[]>([]);
  const [filter, setFilter] = useState<ReportFilter>("all");
  const [reportDateFrom, setReportDateFrom] = useState("");
  const [reportDateTo, setReportDateTo] = useState("");
  const [reportMode, setReportMode] = useState<ReportModeFilter>("all");
  const [banner, setBanner] = useState<Banner>({ kind: "idle", message: "" });
  const [loading, setLoading] = useState(false);
  const [loadingMore, setLoadingMore] = useState(false);
  const [hasMore, setHasMore] = useState(false);
  const [nextOffset, setNextOffset] = useState(0);
  const [deletingId, setDeletingId] = useState("");

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
      const response = await getDeceptionReports(locale, reportPageSize, filter, offset);
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
      if (reportMode !== "all" && report.mode !== reportMode) {
        return false;
      }
      return true;
    });
  }, [reportDateFrom, reportDateTo, reportMode, reports]);

  const visibleCounters = useMemo(() => {
    const initial: Record<DeceptionReportStatus, number> = {
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

  const hasReportFilters = Boolean(reportDateFrom || reportDateTo || reportMode !== "all");

  const clearReportFilters = useCallback(() => {
    setReportDateFrom("");
    setReportDateTo("");
    setReportMode("all");
  }, []);

  const deleteFailedReport = async (report: DeceptionReportSummary) => {
    if (report.status !== "failed" || deletingId) {
      return;
    }
    const confirmed = window.confirm(t("deceptionReportDeleteConfirmDescription", { ns: "settings" }));
    if (!confirmed) {
      return;
    }
    setDeletingId(report.id);
    setBanner({ kind: "idle", message: "" });
    try {
      const response = await deleteFailedDeceptionReport(report.id, locale);
      setReports((items) => items.filter((item) => item.id !== report.id));
      setBanner({ kind: "success", message: t("deceptionReportDeleteSuccess", { ns: "settings", count: response.deleted }) });
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
            title={t("deceptionReportsTitle", { ns: "settings" })}
            description={t("deceptionReportsDescription", { ns: "settings" })}
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
                  {item === "all" ? t("deceptionReportFilter.all", { ns: "settings" }) : statusLabel(item, t)}
                </button>
              ))}
            </div>
            <span className="text-xs text-base-content/60">
              {t("deceptionReportCount", { ns: "settings", value: visibleReports.length })} · {t("deceptionReportRunningCount", { ns: "settings", value: visibleCounters.running })}
            </span>
          </div>

          <div className="flex flex-wrap items-end gap-2">
            <div className="flex min-w-0 flex-col gap-1 text-xs text-base-content/60">
              <span>{t("deceptionReportDateRange", { ns: "settings" })}</span>
              <div className="flex flex-wrap items-end gap-2">
                <label className="flex min-w-0 flex-col gap-1">
                  <span>{t("deceptionReportDateFrom", { ns: "settings" })}</span>
                  <input
                    className="input input-bordered input-sm w-44 bg-base-100"
                    type="date"
                    value={reportDateFrom}
                    onChange={(event) => handleReportDateFromChange(event.target.value)}
                  />
                </label>
                <label className="flex min-w-0 flex-col gap-1">
                  <span>{t("deceptionReportDateTo", { ns: "settings" })}</span>
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
            <label className="flex min-w-0 flex-col gap-1 text-xs text-base-content/60">
              <span>{t("deceptionReportMode", { ns: "settings" })}</span>
              <select
                className="select select-bordered select-sm w-56 bg-base-100"
                value={reportMode}
                onChange={(event) => setReportMode(event.target.value as ReportModeFilter)}
              >
                {reportModeFilters.map((mode) => (
                  <option key={mode} value={mode}>
                    {mode === "all" ? t("deceptionReportFilter.all", { ns: "settings" }) : reportModeLabel(mode, t)}
                  </option>
                ))}
              </select>
            </label>
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
            <table className="deception-reports-table table table-zebra table-sm w-full min-w-[50rem]">
              <thead className="sticky top-0 z-10 bg-base-200">
                <tr>
                  <th className="w-[8rem]">{t("deceptionReportStatus", { ns: "settings" })}</th>
                  <th className="w-[13rem]">{t("deceptionReportStartedAt", { ns: "settings" })}</th>
                  <th className="w-[13rem]">{t("deceptionReportEndedAt", { ns: "settings" })}</th>
                  <th className="w-[8rem]">{t("deceptionReportDuration", { ns: "settings" })}</th>
                  <th className="w-[12rem]">{t("deceptionReportMode", { ns: "settings" })}</th>
                  <th className="w-[16rem]">{t("deceptionReportError", { ns: "settings" })}</th>
                  <th className="w-[10rem]">{t("deceptionReportActions", { ns: "settings" })}</th>
                </tr>
              </thead>
              <tbody>
                {visibleReports.length === 0 ? (
                  <tr>
                    <td colSpan={7} className="p-3">
                      <div className="admin-empty-state admin-empty-state--table">
                        {loading
                          ? t("loading", { ns: "common" })
                          : hasReportFilters
                            ? t("deceptionReportNoMatch", { ns: "settings" })
                            : t("empty", { ns: "common" })}
                      </div>
                    </td>
                  </tr>
                ) : (
                  visibleReports.map((report) => {
                    return (
                      <tr key={report.id} className="row-hover">
                        <td><Badge tone={statusTone(report.status)}>{statusLabel(report.status, t)}</Badge></td>
                        <td className="tabular-nums whitespace-normal break-words">{formatTime(locale, report.startedAt)}</td>
                        <td className="tabular-nums whitespace-normal break-words">{formatTime(locale, report.endedAt)}</td>
                        <td className="tabular-nums whitespace-normal break-words">{formatDuration(report.durationSeconds, t)}</td>
                        <td className="whitespace-normal break-words">{reportModeLabel(report.mode, t)}</td>
                        <td className={cx(report.lastError && "text-error", "whitespace-normal break-words")}>{report.lastError || report.abnormalReason || "-"}</td>
                        <td>
                          <div className="flex items-center gap-1">
                            {report.status === "failed" ? (
                              <button
                                className="btn btn-ghost btn-xs h-8 min-h-8 rounded-xl text-error"
                                type="button"
                                disabled={deletingId === report.id}
                                title={t("deceptionReportDeleteFailed", { ns: "settings" })}
                                onClick={() => void deleteFailedReport(report)}
                              >
                                <Trash2 size={14} />
                                <span>{deletingId === report.id ? t("loading", { ns: "common" }) : t("delete", { ns: "common", defaultValue: t("deceptionReportDeleteFailed", { ns: "settings" }) })}</span>
                              </button>
                            ) : (
                              <span className="text-base-content/45">-</span>
                            )}
                          </div>
                        </td>
                      </tr>
                    );
                  })
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
