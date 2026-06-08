import { useCallback, useEffect, useMemo, useState } from "react";
import type { TFunction } from "i18next";
import { ChevronDown, RefreshCw, Trash2 } from "lucide-react";

import { deleteFPVVideoRecords, getFPVVideoRecords } from "../api";
import type { Banner, Tone } from "../app/types";
import { Badge } from "../components/Badge";
import { Panel, PanelBody } from "../components/Panel";
import { SectionHeader } from "../components/SectionHeader";
import type { FPVVideoRecord, FPVVideoRecordStatus } from "../types";
import { cx } from "../utils/classnames";
import { formatNumber, formatTime } from "../utils/format";

type RecordFilter = "all" | FPVVideoRecordStatus;

const recordPageSize = 50;
const recordFilters: RecordFilter[] = ["all", "completed", "failed"];

function appendRecords(current: FPVVideoRecord[], incoming: FPVVideoRecord[]) {
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

function formatDateKey(value: string) {
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
    return t("fpvRecordDurationSeconds", { ns: "settings", value: rest });
  }
  return t("fpvRecordDurationMinutes", { ns: "settings", minutes, seconds: rest });
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

function statusLabel(status: FPVVideoRecordStatus, t: TFunction) {
  return t(`fpvRecordStatus.${status}`, { ns: "settings" });
}

function statusTone(status: FPVVideoRecordStatus): Tone {
  switch (status) {
    case "completed":
      return "success";
    case "failed":
      return "error";
    default:
      return "neutral";
  }
}

function frameSize(record: FPVVideoRecord) {
  if (!record.lastFrameRows || !record.lastFrameCols) {
    return "-";
  }
  return `${record.lastFrameRows} x ${record.lastFrameCols}`;
}

export function FPVVideoRecordsPage({
  locale,
  t,
}: {
  locale: string;
  t: TFunction;
}) {
  const [records, setRecords] = useState<FPVVideoRecord[]>([]);
  const [filter, setFilter] = useState<RecordFilter>("all");
  const [dateFrom, setDateFrom] = useState("");
  const [dateTo, setDateTo] = useState("");
  const [banner, setBanner] = useState<Banner>({ kind: "idle", message: "" });
  const [loading, setLoading] = useState(false);
  const [loadingMore, setLoadingMore] = useState(false);
  const [hasMore, setHasMore] = useState(false);
  const [nextOffset, setNextOffset] = useState(0);
  const [selectedIds, setSelectedIds] = useState<string[]>([]);
  const [deleteBusy, setDeleteBusy] = useState(false);

  const loadRecords = useCallback(async (options?: { append?: boolean; offset?: number; preserveBanner?: boolean }) => {
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
      const response = await getFPVVideoRecords(locale, recordPageSize, filter, offset);
      const items = response.items ?? [];
      setRecords((current) => (append ? appendRecords(current, items) : items));
      if (!append) {
        const availableIds = new Set(items.map((item) => item.id));
        setSelectedIds((selected) => selected.filter((id) => availableIds.has(id)));
      }
      setHasMore(Boolean(response.hasMore));
      setNextOffset(response.hasMore ? response.nextOffset ?? offset + items.length : 0);
      if (!append && !options?.preserveBanner) {
        setBanner({ kind: "idle", message: "" });
      }
    } catch (error) {
      const message = error instanceof Error ? error.message : t("unexpectedError", { ns: "common" });
      setBanner({ kind: "error", message });
    } finally {
      if (append) {
        setLoadingMore(false);
      } else {
        setLoading(false);
      }
    }
  }, [filter, locale, t]);

  useEffect(() => {
    void loadRecords();
  }, [loadRecords]);

  const handleDateFromChange = useCallback((value: string) => {
    setDateFrom(value);
    setDateTo((currentTo) => (value && currentTo && currentTo < value ? value : currentTo));
  }, []);

  const handleDateToChange = useCallback((value: string) => {
    setDateTo(value && dateFrom && value < dateFrom ? dateFrom : value);
  }, [dateFrom]);

  const visibleRecords = useMemo(() => {
    return records.filter((record) => {
      const startDate = formatDateKey(record.startedAt);
      const endDate = formatDateKey(record.endedAt) || startDate;
      if (!startDate) {
        return false;
      }
      if (dateFrom && endDate < dateFrom) {
        return false;
      }
      if (dateTo && startDate > dateTo) {
        return false;
      }
      return true;
    });
  }, [dateFrom, dateTo, records]);

  useEffect(() => {
    const visibleIds = new Set(visibleRecords.map((record) => record.id));
    setSelectedIds((items) => {
      const next = items.filter((id) => visibleIds.has(id));
      return next.length === items.length ? items : next;
    });
  }, [visibleRecords]);

  const selectedIdSet = useMemo(() => new Set(selectedIds), [selectedIds]);
  const allCurrentSelected = visibleRecords.length > 0 && visibleRecords.every((record) => selectedIdSet.has(record.id));
  const someCurrentSelected = visibleRecords.some((record) => selectedIdSet.has(record.id));
  const hasFilters = Boolean(dateFrom || dateTo);
  const selectedCount = selectedIds.length;

  const clearFilters = useCallback(() => {
    setDateFrom("");
    setDateTo("");
  }, []);

  const toggleRecordSelection = (id: string, checked: boolean) => {
    setSelectedIds((items) => {
      if (checked) {
        return items.includes(id) ? items : [...items, id];
      }
      return items.filter((item) => item !== id);
    });
  };

  const toggleCurrentPageSelection = (checked: boolean) => {
    if (!checked) {
      const currentIds = new Set(visibleRecords.map((record) => record.id));
      setSelectedIds((items) => items.filter((id) => !currentIds.has(id)));
      return;
    }
    setSelectedIds((items) => {
      const next = [...items];
      for (const record of visibleRecords) {
        if (!next.includes(record.id)) {
          next.push(record.id);
        }
      }
      return next;
    });
  };

  const deleteSelectedRecords = async () => {
    if (selectedIds.length === 0) {
      return;
    }
    const confirmed = window.confirm(t("fpvRecordDeleteConfirmDescription", { ns: "settings", count: selectedIds.length }));
    if (!confirmed) {
      return;
    }
    setDeleteBusy(true);
    setBanner({ kind: "loading", message: t("loading", { ns: "common" }) });
    try {
      const response = await deleteFPVVideoRecords({ ids: selectedIds }, locale);
      setSelectedIds([]);
      await loadRecords({ preserveBanner: true });
      setBanner({ kind: "success", message: t("fpvRecordDeleteSuccess", { ns: "settings", count: response.deleted }) });
    } catch (error) {
      const message = error instanceof Error ? error.message : t("unexpectedError", { ns: "common" });
      setBanner({ kind: "error", message });
    } finally {
      setDeleteBusy(false);
    }
  };

  return (
    <section className="flex min-h-0 min-w-0 flex-1">
      <Panel className="flex min-h-0 min-w-0 flex-1 flex-col">
        <PanelBody className="min-h-0 min-w-0 flex-1">
          <SectionHeader
            title={t("fpvRecordsTitle", { ns: "settings" })}
            description={t("fpvRecordsDescription", { ns: "settings" })}
            action={
              <div className="flex flex-wrap justify-end gap-2">
                <button
                  className="btn btn-sm btn-outline btn-error"
                  type="button"
                  disabled={selectedCount === 0 || loading || loadingMore || deleteBusy}
                  onClick={() => void deleteSelectedRecords()}
                >
                  <Trash2 size={16} />
                  <span>{t("fpvRecordDeleteSelected", { ns: "settings", count: selectedCount })}</span>
                </button>
                <button className="btn btn-sm btn-outline btn-info" type="button" onClick={() => void loadRecords()} disabled={loading || loadingMore || deleteBusy}>
                  <RefreshCw size={16} className={loading ? "animate-spin" : undefined} />
                  <span>{t("refresh", { ns: "common" })}</span>
                </button>
              </div>
            }
          />

          <div className="flex flex-wrap items-center gap-2">
            <div className="join">
              {recordFilters.map((item) => (
                <button
                  key={item}
                  className={cx("btn btn-sm join-item", filter === item ? "btn-primary" : "btn-outline")}
                  type="button"
                  onClick={() => setFilter(item)}
                >
                  {item === "all" ? t("fpvRecordFilter.all", { ns: "settings" }) : statusLabel(item, t)}
                </button>
              ))}
            </div>
            <span className="text-xs text-base-content/60">
              {t("fpvRecordCount", { ns: "settings", value: visibleRecords.length })} · {t("fpvRecordSelectedCount", { ns: "settings", value: selectedCount })}
            </span>
          </div>

          <div className="flex flex-wrap items-end gap-2">
            <div className="flex min-w-0 flex-col gap-1 text-xs text-base-content/60">
              <span>{t("fpvRecordDateRange", { ns: "settings" })}</span>
              <div className="flex flex-wrap items-end gap-2">
                <label className="flex min-w-0 flex-col gap-1">
                  <span>{t("fpvRecordDateFrom", { ns: "settings" })}</span>
                  <input
                    className="input input-bordered input-sm w-44 bg-base-100"
                    type="date"
                    value={dateFrom}
                    onChange={(event) => handleDateFromChange(event.target.value)}
                  />
                </label>
                <label className="flex min-w-0 flex-col gap-1">
                  <span>{t("fpvRecordDateTo", { ns: "settings" })}</span>
                  <input
                    className="input input-bordered input-sm w-44 bg-base-100"
                    type="date"
                    min={dateFrom || undefined}
                    value={dateTo}
                    onChange={(event) => handleDateToChange(event.target.value)}
                  />
                </label>
              </div>
            </div>
            <button className="btn btn-sm btn-ghost" type="button" disabled={!hasFilters} onClick={clearFilters}>
              {t("clear", { ns: "common" })}
            </button>
          </div>

          {(banner.kind === "error" || banner.kind === "success") && banner.message ? (
            <div className={cx("alert alert-soft py-3 text-sm", banner.kind === "error" ? "alert-error" : "alert-success")} role="alert">
              <span className="min-w-0 [overflow-wrap:anywhere]">{banner.message}</span>
            </div>
          ) : null}

          <div className="min-h-0 min-w-0 flex-1 overflow-auto rounded-2xl border border-base-300 bg-base-100/70">
            <table className="table table-zebra table-sm w-full min-w-[86rem] table-fixed">
              <thead className="sticky top-0 z-10 bg-base-200">
                <tr>
                  <th className="w-[4rem]">
                    <input
                      className="checkbox checkbox-sm"
                      type="checkbox"
                      checked={allCurrentSelected}
                      ref={(element) => {
                        if (element) {
                          element.indeterminate = !allCurrentSelected && someCurrentSelected;
                        }
                      }}
                      aria-label={t("fpvRecordSelectCurrentPage", { ns: "settings" })}
                      disabled={visibleRecords.length === 0 || loading || loadingMore || deleteBusy}
                      onChange={(event) => toggleCurrentPageSelection(event.currentTarget.checked)}
                    />
                  </th>
                  <th className="w-[8rem]">{t("fpvRecordStatus", { ns: "settings" })}</th>
                  <th className="w-[16rem]">{t("fpvRecordModel", { ns: "settings" })}</th>
                  <th className="w-[16rem]">{t("fpvRecordIdentity", { ns: "settings" })}</th>
                  <th className="w-[9rem]">{t("fpvRecordFrequency", { ns: "settings" })}</th>
                  <th className="w-[8rem]">{t("fpvRecordRssi", { ns: "settings" })}</th>
                  <th className="w-[13rem]">{t("fpvRecordStartedAt", { ns: "settings" })}</th>
                  <th className="w-[13rem]">{t("fpvRecordEndedAt", { ns: "settings" })}</th>
                  <th className="w-[8rem]">{t("fpvRecordDuration", { ns: "settings" })}</th>
                  <th className="w-[7rem]">{t("fpvRecordFrameCount", { ns: "settings" })}</th>
                  <th className="w-[9rem]">{t("fpvRecordLastFrameSize", { ns: "settings" })}</th>
                  <th className="w-[13rem]">{t("fpvRecordLastFrameAt", { ns: "settings" })}</th>
                  <th className="w-[14rem]">{t("fpvRecordError", { ns: "settings" })}</th>
                </tr>
              </thead>
              <tbody>
                {visibleRecords.length === 0 ? (
                  <tr>
                    <td colSpan={13} className="p-3">
                      <div className="admin-empty-state admin-empty-state--table">
                        {loading
                          ? t("loading", { ns: "common" })
                          : records.length > 0
                            ? t("fpvRecordNoMatch", { ns: "settings" })
                            : t("empty", { ns: "common" })}
                      </div>
                    </td>
                  </tr>
                ) : (
                  visibleRecords.map((record) => (
                    <tr key={record.id} className="row-hover">
                      <td>
                        <input
                          className="checkbox checkbox-sm"
                          type="checkbox"
                          checked={selectedIdSet.has(record.id)}
                          aria-label={t("fpvRecordSelectRecord", { ns: "settings" })}
                          disabled={deleteBusy}
                          onChange={(event) => toggleRecordSelection(record.id, event.currentTarget.checked)}
                        />
                      </td>
                      <td><Badge tone={statusTone(record.status)}>{statusLabel(record.status, t)}</Badge></td>
                      <td className="whitespace-normal break-words">{record.displayModel || record.model || "-"}</td>
                      <td className="whitespace-normal break-words font-mono text-xs">{record.serial || record.targetId || "-"}</td>
                      <td className="tabular-nums whitespace-normal break-words">{formatFrequency(locale, record.frequency)}</td>
                      <td className="tabular-nums whitespace-normal break-words">{formatRSSI(locale, record.rssi)}</td>
                      <td className="tabular-nums whitespace-normal break-words">{formatTime(locale, record.startedAt)}</td>
                      <td className="tabular-nums whitespace-normal break-words">{formatTime(locale, record.endedAt)}</td>
                      <td className="tabular-nums whitespace-normal break-words">{formatDuration(record.durationSeconds, t)}</td>
                      <td className="tabular-nums whitespace-normal break-words">{record.frameCount}</td>
                      <td className="tabular-nums whitespace-normal break-words">{frameSize(record)}</td>
                      <td className="tabular-nums whitespace-normal break-words">{formatTime(locale, record.lastFrameAt)}</td>
                      <td className={cx(record.error && "text-error", "whitespace-normal break-words")}>{record.error || "-"}</td>
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
                disabled={loading || loadingMore || deleteBusy}
                onClick={() => void loadRecords({ append: true, offset: nextOffset, preserveBanner: true })}
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
