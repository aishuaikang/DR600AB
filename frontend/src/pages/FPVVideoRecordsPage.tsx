import { useCallback, useEffect, useMemo, useState } from "react";
import type { TFunction } from "i18next";
import { ChevronDown, ChevronLeft, ChevronRight, Eye, Pause, Play, RefreshCw, Trash2, X } from "lucide-react";

import { deleteFPVVideoRecords, getFPVVideoRecord, getFPVVideoRecords } from "../api";
import type { Banner, Tone } from "../app/types";
import { Badge } from "../components/Badge";
import { Panel, PanelBody } from "../components/Panel";
import { SectionHeader } from "../components/SectionHeader";
import type { FPVVideoRecord, FPVVideoRecordFrame, FPVVideoRecordStatus } from "../types";
import { cx } from "../utils/classnames";
import { formatNumber, formatTime } from "../utils/format";

type RecordFilter = "all" | FPVVideoRecordStatus;

const recordPageSize = 50;
const recordFilters: RecordFilter[] = ["all", "completed", "failed"];
const framePlaybackIntervalMs = 160;

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

function displayFrameSize(frame?: FPVVideoRecordFrame) {
  if (!frame?.rows || !frame.cols) {
    return "-";
  }
  return `${frame.rows} x ${frame.cols}`;
}

function formatFrameBytes(locale: string, value?: number) {
  if (typeof value !== "number" || !Number.isFinite(value) || value <= 0) {
    return "-";
  }
  if (value >= 1024 * 1024) {
    return `${formatNumber(locale, value / 1024 / 1024, 1)} MB`;
  }
  if (value >= 1024) {
    return `${formatNumber(locale, value / 1024, 1)} KB`;
  }
  return `${formatNumber(locale, value, 0)} B`;
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
  const [detailRecord, setDetailRecord] = useState<FPVVideoRecord | null>(null);
  const [detailLoadingId, setDetailLoadingId] = useState("");
  const [detailFrameIndex, setDetailFrameIndex] = useState(0);
  const [detailPlaying, setDetailPlaying] = useState(false);

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

  const openRecordDetail = async (record: FPVVideoRecord) => {
    setDetailLoadingId(record.id);
    setBanner({ kind: "loading", message: t("loading", { ns: "common" }) });
    try {
      const detail = await getFPVVideoRecord(record.id, locale);
      setDetailRecord(detail);
      setDetailFrameIndex(0);
      setDetailPlaying((detail.frames?.length ?? 0) > 1);
      setBanner({ kind: "idle", message: "" });
    } catch (error) {
      const message = error instanceof Error ? error.message : t("unexpectedError", { ns: "common" });
      setBanner({ kind: "error", message });
    } finally {
      setDetailLoadingId("");
    }
  };

  const closeRecordDetail = () => {
    setDetailRecord(null);
    setDetailFrameIndex(0);
    setDetailPlaying(false);
  };

  useEffect(() => {
    if (!detailPlaying || !detailRecord?.frames || detailRecord.frames.length <= 1) {
      return undefined;
    }
    const timer = window.setInterval(() => {
      setDetailFrameIndex((current) => {
        const frameCount = detailRecord.frames?.length ?? 0;
        return frameCount > 0 ? (current + 1) % frameCount : 0;
      });
    }, framePlaybackIntervalMs);
    return () => window.clearInterval(timer);
  }, [detailPlaying, detailRecord]);

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
            <table className="table table-zebra table-sm w-full min-w-[94rem] table-fixed">
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
                  <th className="w-[8rem]">{t("fpvRecordActions", { ns: "settings" })}</th>
                </tr>
              </thead>
              <tbody>
                {visibleRecords.length === 0 ? (
                  <tr>
                    <td colSpan={14} className="p-3">
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
                      <td>
                        <button
                          className="btn btn-xs btn-outline btn-info"
                          type="button"
                          disabled={deleteBusy || detailLoadingId === record.id}
                          onClick={() => void openRecordDetail(record)}
                          title={t("fpvRecordView", { ns: "settings" })}
                        >
                          <Eye size={13} aria-hidden="true" />
                          <span>{detailLoadingId === record.id ? t("loading", { ns: "common" }) : t("fpvRecordView", { ns: "settings" })}</span>
                        </button>
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
      {detailRecord ? (
        <FPVVideoRecordDetailModal
          locale={locale}
          record={detailRecord}
          frameIndex={detailFrameIndex}
          playing={detailPlaying}
          t={t}
          onClose={closeRecordDetail}
          onFrameIndexChange={(index) => {
            setDetailFrameIndex(index);
            setDetailPlaying(false);
          }}
          onPlayingChange={setDetailPlaying}
        />
      ) : null}
    </section>
  );
}

function FPVVideoRecordDetailModal({
  locale,
  record,
  frameIndex,
  playing,
  t,
  onClose,
  onFrameIndexChange,
  onPlayingChange,
}: {
  locale: string;
  record: FPVVideoRecord;
  frameIndex: number;
  playing: boolean;
  t: TFunction;
  onClose: () => void;
  onFrameIndexChange: (index: number) => void;
  onPlayingChange: (playing: boolean) => void;
}) {
  const frames = record.frames ?? [];
  const frameCount = frames.length;
  const boundedFrameIndex = Math.min(Math.max(frameIndex, 0), Math.max(0, frameCount - 1));
  const currentFrame = frameCount > 0 ? frames[boundedFrameIndex] : undefined;
  const title = record.displayModel || record.model || t("fpvRecordUnknownTarget", { ns: "settings" });
  const canSeekFrames = frameCount > 1;
  const framePosition = frameCount > 0 ? `${boundedFrameIndex + 1} / ${frameCount}` : "0 / 0";
  const goToPreviousFrame = () => {
    if (!canSeekFrames) {
      return;
    }
    onFrameIndexChange((boundedFrameIndex - 1 + frameCount) % frameCount);
  };
  const goToNextFrame = () => {
    if (!canSeekFrames) {
      return;
    }
    onFrameIndexChange((boundedFrameIndex + 1) % frameCount);
  };

  return (
    <div className="app-modal-backdrop fixed inset-0 z-50 grid place-items-center bg-black/60 p-4" role="presentation" onClick={onClose}>
      <section
        className="app-modal-card flex max-h-[94vh] w-full max-w-6xl flex-col overflow-hidden rounded-xl border border-base-300 bg-base-100 shadow-2xl shadow-black/45"
        role="dialog"
        aria-modal="true"
        aria-labelledby="fpv-record-detail-title"
        onClick={(event) => event.stopPropagation()}
      >
        <div className="flex min-w-0 items-start justify-between gap-3 border-b border-base-300 px-4 py-3">
          <div className="min-w-0">
            <span className="text-xs font-semibold uppercase text-base-content/50">{t("fpvRecordPreview", { ns: "settings" })}</span>
            <h2 id="fpv-record-detail-title" className="mt-0.5 truncate text-lg font-semibold text-base-content">
              {title}
            </h2>
            <div className="mt-2 flex flex-wrap items-center gap-2 text-xs text-base-content/60">
              <Badge tone={statusTone(record.status)}>{statusLabel(record.status, t)}</Badge>
              <span className="rounded-lg border border-base-300 bg-base-200/60 px-2 py-1 tabular-nums">{formatFrequency(locale, record.frequency)}</span>
              <span className="rounded-lg border border-base-300 bg-base-200/60 px-2 py-1 tabular-nums">{formatRSSI(locale, record.rssi)}</span>
              <span className="rounded-lg border border-base-300 bg-base-200/60 px-2 py-1 tabular-nums">{formatDuration(record.durationSeconds, t)}</span>
              <span className="rounded-lg border border-base-300 bg-base-200/60 px-2 py-1 tabular-nums">{t("fpvRecordFrameCount", { ns: "settings" })}: {record.frameCount}</span>
            </div>
          </div>
          <button className="btn btn-ghost btn-sm h-8 min-h-8 w-8 shrink-0 rounded-lg px-0" type="button" aria-label={t("close", { ns: "common" })} onClick={onClose}>
            <X size={16} aria-hidden="true" />
          </button>
        </div>

        <div className="grid min-h-0 gap-4 overflow-auto p-4 lg:grid-cols-[minmax(0,1fr)_20rem] lg:items-start">
          <div className="grid min-w-0 gap-3 lg:self-start">
            <div className="grid aspect-video max-h-[62vh] place-items-center overflow-hidden rounded-lg border border-base-300 bg-black">
              {currentFrame?.image ? (
                <img className="h-full w-full object-contain [image-rendering:auto]" src={currentFrame.image} alt={t("fpvRecordFrameAlt", { ns: "settings" })} />
              ) : (
                <div className="grid place-items-center gap-2 px-4 text-center text-sm text-neutral-content/70">
                  <Eye size={30} aria-hidden="true" />
                  <span>{t("fpvRecordNoFrames", { ns: "settings" })}</span>
                </div>
              )}
            </div>

            <div className="rounded-lg border border-base-300 bg-base-200/50 p-3">
              <div className="grid grid-cols-[auto_auto_auto_minmax(4rem,1fr)] items-center gap-2 sm:grid-cols-[auto_auto_auto_minmax(0,1fr)_auto]">
                <button
                  className="btn btn-sm btn-circle btn-ghost"
                  type="button"
                  disabled={!canSeekFrames}
                  aria-label={t("fpvRecordPreviousFrame", { ns: "settings" })}
                  title={t("fpvRecordPreviousFrame", { ns: "settings" })}
                  onClick={goToPreviousFrame}
                >
                  <ChevronLeft size={17} aria-hidden="true" />
                </button>
                <button
                  className={cx("btn btn-sm min-w-24", playing ? "btn-outline btn-info" : "btn-info")}
                  type="button"
                  disabled={!canSeekFrames}
                  aria-label={playing ? t("fpvRecordPause", { ns: "settings" }) : t("fpvRecordPlay", { ns: "settings" })}
                  onClick={() => onPlayingChange(!playing)}
                >
                  {playing ? <Pause size={15} aria-hidden="true" /> : <Play size={15} aria-hidden="true" />}
                  <span>{playing ? t("fpvRecordPause", { ns: "settings" }) : t("fpvRecordPlay", { ns: "settings" })}</span>
                </button>
                <button
                  className="btn btn-sm btn-circle btn-ghost"
                  type="button"
                  disabled={!canSeekFrames}
                  aria-label={t("fpvRecordNextFrame", { ns: "settings" })}
                  title={t("fpvRecordNextFrame", { ns: "settings" })}
                  onClick={goToNextFrame}
                >
                  <ChevronRight size={17} aria-hidden="true" />
                </button>
                <input
                  className="range range-info range-sm col-span-full min-w-0 sm:col-span-1 sm:col-start-auto sm:row-start-auto"
                  type="range"
                  min={0}
                  max={Math.max(0, frameCount - 1)}
                  step={1}
                  value={boundedFrameIndex}
                  disabled={!canSeekFrames}
                  onChange={(event) => onFrameIndexChange(Number(event.currentTarget.value))}
                />
                <span className="col-start-4 row-start-1 w-full text-right text-xs tabular-nums text-base-content/70 sm:col-start-auto sm:row-start-auto sm:w-20">{framePosition}</span>
              </div>
              <div className="mt-2 flex flex-wrap items-center justify-between gap-x-4 gap-y-1 text-xs text-base-content/55">
                <span className="tabular-nums">{t("fpvRecordCurrentFrame", { ns: "settings" })}: {currentFrame ? `#${currentFrame.num}` : "-"}</span>
                <span className="tabular-nums">{displayFrameSize(currentFrame)}</span>
                <span className="tabular-nums">{formatFrameBytes(locale, currentFrame?.frameBytes)}</span>
                <span className="tabular-nums">{formatTime(locale, currentFrame?.receivedAt || record.lastFrameAt)}</span>
              </div>
            </div>
          </div>

          <div className="grid content-start gap-3 rounded-lg border border-base-300 bg-base-200/35 p-3 text-sm lg:self-start">
            <div className="grid grid-cols-2 gap-2">
              <RecordDetailItem label={t("fpvRecordStartedAt", { ns: "settings" })} value={formatTime(locale, record.startedAt)} />
              <RecordDetailItem label={t("fpvRecordEndedAt", { ns: "settings" })} value={formatTime(locale, record.endedAt)} />
              <RecordDetailItem label={t("fpvRecordIdentity", { ns: "settings" })} value={record.serial || record.targetId || "-"} mono wide />
              <RecordDetailItem label={t("fpvRecordModel", { ns: "settings" })} value={title} wide />
              <RecordDetailItem label={t("fpvRecordFrequency", { ns: "settings" })} value={formatFrequency(locale, record.frequency)} />
              <RecordDetailItem label={t("fpvRecordRssi", { ns: "settings" })} value={formatRSSI(locale, record.rssi)} />
              <RecordDetailItem label={t("fpvRecordDuration", { ns: "settings" })} value={formatDuration(record.durationSeconds, t)} />
              <RecordDetailItem label={t("fpvRecordFrameCount", { ns: "settings" })} value={String(record.frameCount)} />
              <RecordDetailItem label={t("fpvRecordLastFrameSize", { ns: "settings" })} value={frameSize(record)} />
              <RecordDetailItem label={t("fpvRecordLastFrameAt", { ns: "settings" })} value={formatTime(locale, record.lastFrameAt)} />
              {record.error ? <RecordDetailItem label={t("fpvRecordError", { ns: "settings" })} value={record.error} tone="error" wide /> : null}
            </div>
          </div>
        </div>
      </section>
    </div>
  );
}

function RecordDetailItem({
  label,
  value,
  mono,
  tone,
  wide,
}: {
  label: string;
  value: string;
  mono?: boolean;
  tone?: "error";
  wide?: boolean;
}) {
  return (
    <div className={cx("grid min-w-0 gap-1 rounded-lg border border-base-300 bg-base-100/70 px-3 py-2", wide && "col-span-2")}>
      <span className="truncate text-xs text-base-content/50">{label}</span>
      <strong className={cx("min-w-0 break-words text-sm font-semibold leading-snug", mono && "font-mono text-xs", tone === "error" && "text-error")}>{value}</strong>
    </div>
  );
}
