import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import type { TFunction } from "i18next";
import { MapPinned, RefreshCw, Trash2, X } from "lucide-react";

import { deleteIntrusions, getIntrusions } from "../api";
import { Badge } from "../components/Badge";
import { Panel, PanelBody } from "../components/Panel";
import { SectionHeader } from "../components/SectionHeader";
import type { Banner } from "../app/types";
import type { Tone } from "../app/types";
import type {
  IntrusionRecord,
  IntrusionTargetType,
  ScreenPositionPoint,
  ScreenPositionTarget,
  ScreenPositionTrackPoint,
} from "../types";
import { cx } from "../utils/classnames";
import { formatNumber, formatTime } from "../utils/format";
import { extractErrorMessage } from "../utils/session";
import { PositionMap } from "./ScreenMap";
import { referenceMapLayers } from "./screenData";

type IntrusionFilter = "all" | IntrusionTargetType;
type CellDetail = {
  title: string;
  content: string;
  mono?: boolean;
};

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

function validIntrusionPoint(point?: ScreenPositionPoint | null): point is ScreenPositionPoint {
  return Boolean(
    point &&
      Number.isFinite(point.latitude) &&
      Number.isFinite(point.longitude) &&
      point.latitude >= -90 &&
      point.latitude <= 90 &&
      point.longitude >= -180 &&
      point.longitude <= 180 &&
      !(point.latitude === 0 && point.longitude === 0),
  );
}

function validIntrusionTrackPoint(point?: ScreenPositionTrackPoint | null): point is ScreenPositionTrackPoint {
  return Boolean(point && validIntrusionPoint(point));
}

function coordinateSummary(record: IntrusionRecord, t: TFunction) {
  const parts: string[] = [];
  if (record.deviceLocation?.valid && record.deviceLocation.point) {
    parts.push(`${t("intrusionDeviceLocation", { ns: "settings" })}: ${formatPoint(record.deviceLocation.point)}`);
  }
  if (record.targetType !== "position") {
    return parts.length > 0 ? parts.join(" / ") : "-";
  }
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

function sourceSummary(source?: string, sources?: string[]) {
  const values = [...(sources ?? []), source ?? ""]
    .map((item) => item.trim())
    .filter(Boolean);
  const unique = Array.from(new Set(values));
  return unique.length > 0 ? unique.join(" / ") : "-";
}

function hasIntrusionMapData(record: IntrusionRecord) {
  if (record.targetType !== "position") {
    return false;
  }
  return (
    validIntrusionPoint(record.drone) ||
    validIntrusionPoint(record.pilot) ||
    Boolean(record.droneTrajectory?.some(validIntrusionTrackPoint)) ||
    Boolean(record.pilotTrajectory?.some(validIntrusionTrackPoint))
  );
}

function intrusionToPositionTarget(record: IntrusionRecord): ScreenPositionTarget {
  const targetId = record.targetId || record.id;
  const serial = record.serial || record.device || targetId;
  const source = record.source || record.targetType;
  return {
    id: targetId,
    serial,
    model: record.model || "",
    source,
    sources: record.sources,
    frequency: record.frequency,
    rssi: record.rssi,
    device: record.device,
    drone: record.drone,
    pilot: record.pilot,
    home: record.home,
    droneTrajectory: record.droneTrajectory,
    pilotTrajectory: record.pilotTrajectory,
    height: record.height,
    altitude: record.altitude,
    speed: record.speed,
    cracked: record.cracked,
    firstSeen: record.firstSeen,
    lastSeen: record.lastSeen,
    hitCount: record.hitCount,
    lastRecord: {
      type: source,
      receivedAt: record.lastSeen,
      device: record.device,
      serial,
      model: record.model,
      frequency: record.frequency,
      rssi: record.rssi,
      cracked: record.cracked,
    },
  };
}

function OverflowCell({
  label,
  value,
  mono = false,
  className = "",
  onOpen,
}: {
  label: string;
  value: string;
  mono?: boolean;
  className?: string;
  onOpen: (detail: CellDetail) => void;
}) {
  const contentRef = useRef<HTMLSpanElement>(null);
  const [overflowing, setOverflowing] = useState(false);
  const displayValue = value || "-";

  useEffect(() => {
    const element = contentRef.current;
    if (!element) {
      return;
    }

    const measure = () => {
      setOverflowing(element.scrollWidth > element.clientWidth + 1);
    };
    measure();

    if (typeof ResizeObserver !== "undefined") {
      const observer = new ResizeObserver(measure);
      observer.observe(element);
      return () => observer.disconnect();
    }
    window.addEventListener("resize", measure);
    return () => window.removeEventListener("resize", measure);
  }, [displayValue]);

  return (
    <button
      className={cx("intrusion-overflow-cell", overflowing && "intrusion-overflow-cell--clickable", mono && "font-mono", className)}
      type="button"
      disabled={!overflowing}
      title={overflowing ? displayValue : undefined}
      onClick={() => onOpen({ title: label, content: displayValue, mono })}
    >
      <span ref={contentRef} className="block truncate">
        {displayValue}
      </span>
    </button>
  );
}

function CoordinateMapCell({
  label,
  value,
  record,
  onOpenText,
  onOpenMap,
}: {
  label: string;
  value: string;
  record: IntrusionRecord;
  onOpenText: (detail: CellDetail) => void;
  onOpenMap: (record: IntrusionRecord) => void;
}) {
  const displayValue = value || "-";
  if (!hasIntrusionMapData(record)) {
    return (
      <OverflowCell
        label={label}
        value={displayValue}
        mono
        className="text-xs tabular-nums text-base-content/80"
        onOpen={onOpenText}
      />
    );
  }

  return (
    <button
      className="intrusion-overflow-cell intrusion-overflow-cell--clickable intrusion-coordinate-cell font-mono text-xs tabular-nums text-base-content/80"
      type="button"
      title={displayValue}
      aria-label={label}
      onClick={() => onOpenMap(record)}
    >
      <span className="block truncate">{displayValue}</span>
      <MapPinned size={14} aria-hidden="true" />
    </button>
  );
}

function CellDetailModal({
  detail,
  t,
  onClose,
}: {
  detail: CellDetail | null;
  t: TFunction;
  onClose: () => void;
}) {
  if (!detail) {
    return null;
  }

  return (
    <div className="app-modal-backdrop fixed inset-0 z-50 grid place-items-center bg-black/55 p-4" role="presentation" onClick={onClose}>
      <section
        className="app-modal-card intrusion-cell-modal grid w-full max-w-2xl gap-3 rounded-2xl border border-base-300 bg-base-100 p-4 shadow-2xl shadow-black/40"
        role="dialog"
        aria-modal="true"
        aria-labelledby="intrusion-cell-modal-title"
        onClick={(event) => event.stopPropagation()}
      >
        <div className="flex min-w-0 items-center justify-between gap-3">
          <h2 id="intrusion-cell-modal-title" className="truncate text-sm font-semibold text-base-content">
            {detail.title}
          </h2>
          <button className="btn btn-ghost btn-sm h-8 min-h-8 w-8 shrink-0 rounded-xl px-0" type="button" aria-label={t("close", { ns: "common" })} onClick={onClose}>
            <X size={16} />
          </button>
        </div>
        <pre className={cx("intrusion-cell-modal__content", detail.mono && "font-mono")}>{detail.content}</pre>
      </section>
    </div>
  );
}

function IntrusionMapModal({
  record,
  t,
  onClose,
}: {
  record: IntrusionRecord | null;
  t: TFunction;
  onClose: () => void;
}) {
  useEffect(() => {
    if (!record) {
      return;
    }
    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape") {
        onClose();
      }
    };
    window.addEventListener("keydown", handleKeyDown);
    return () => window.removeEventListener("keydown", handleKeyDown);
  }, [onClose, record]);

  if (!record) {
    return null;
  }

  const target = intrusionToPositionTarget(record);
  const identity = record.serial || record.device || record.targetId || record.id;
  const title = record.model || t("intrusionMapTitle", { ns: "settings" });

  return (
    <div className="app-modal-backdrop fixed inset-0 z-50 grid place-items-center bg-black/60 p-4" role="presentation" onClick={onClose}>
      <section
        className="app-modal-card intrusion-map-modal grid w-full gap-3 rounded-2xl border border-base-300 bg-base-100 p-4 shadow-2xl shadow-black/45"
        role="dialog"
        aria-modal="true"
        aria-labelledby="intrusion-map-modal-title"
        onClick={(event) => event.stopPropagation()}
      >
        <div className="flex min-w-0 items-start justify-between gap-3">
          <div className="min-w-0">
            <h2 id="intrusion-map-modal-title" className="truncate text-sm font-semibold text-base-content">
              {title}
            </h2>
            <p className="mt-1 truncate font-mono text-xs text-base-content/60">{identity}</p>
          </div>
          <button className="btn btn-ghost btn-sm h-8 min-h-8 w-8 shrink-0 rounded-xl px-0" type="button" aria-label={t("close", { ns: "common" })} onClick={onClose}>
            <X size={16} />
          </button>
        </div>
        <PositionMap
          selectedId={target.id}
          positions={[target]}
          deviceLocation={null}
          visibleMapLayers={referenceMapLayers}
          onSelectPosition={() => undefined}
          className="intrusion-map-modal__map"
        />
      </section>
    </div>
  );
}

function DeleteConfirmModal({
  count,
  busy,
  t,
  onCancel,
  onConfirm,
}: {
  count: number;
  busy: boolean;
  t: TFunction;
  onCancel: () => void;
  onConfirm: () => void;
}) {
  if (count <= 0) {
    return null;
  }

  return (
    <div className="app-modal-backdrop fixed inset-0 z-50 grid place-items-center bg-black/55 p-4" role="presentation" onClick={busy ? undefined : onCancel}>
      <section
        className="app-modal-card grid w-full max-w-md gap-4 rounded-2xl border border-base-300 bg-base-100 p-4 shadow-2xl shadow-black/40"
        role="dialog"
        aria-modal="true"
        aria-labelledby="intrusion-delete-title"
        onClick={(event) => event.stopPropagation()}
      >
        <div className="flex min-w-0 items-start justify-between gap-3">
          <div className="min-w-0">
            <h2 id="intrusion-delete-title" className="text-sm font-semibold text-base-content">
              {t("intrusionDeleteConfirmTitle", { ns: "settings" })}
            </h2>
            <p className="mt-2 text-sm leading-6 text-base-content/70">
              {t("intrusionDeleteConfirmDescription", { ns: "settings", count })}
            </p>
          </div>
          <button className="btn btn-ghost btn-sm h-8 min-h-8 w-8 shrink-0 rounded-xl px-0" type="button" aria-label={t("close", { ns: "common" })} disabled={busy} onClick={onCancel}>
            <X size={16} />
          </button>
        </div>
        <div className="flex justify-end gap-2">
          <button className="btn btn-sm btn-outline" type="button" disabled={busy} onClick={onCancel}>
            {t("cancel", { ns: "common" })}
          </button>
          <button className="btn btn-sm btn-error" type="button" disabled={busy} onClick={onConfirm}>
            {busy ? t("loading", { ns: "common" }) : t("delete", { ns: "common" })}
          </button>
        </div>
      </section>
    </div>
  );
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
  const [cellDetail, setCellDetail] = useState<CellDetail | null>(null);
  const [mapRecord, setMapRecord] = useState<IntrusionRecord | null>(null);
  const [selectedIds, setSelectedIds] = useState<string[]>([]);
  const [deleteConfirmOpen, setDeleteConfirmOpen] = useState(false);
  const [deleteBusy, setDeleteBusy] = useState(false);

  const loadRecords = useCallback(async (options?: { preserveBanner?: boolean }) => {
    setLoading(true);
    if (!options?.preserveBanner) {
      setBanner({ kind: "loading", message: t("loading", { ns: "common" }) });
    }
    try {
      const response = await getIntrusions(locale, intrusionLimit, filter);
      setRecords(response.items);
      const availableIds = new Set(response.items.map((item) => item.id));
      setSelectedIds((items) => items.filter((id) => availableIds.has(id)));
      if (!options?.preserveBanner) {
        setBanner({ kind: "idle", message: "" });
      }
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
  const selectedIdSet = useMemo(() => new Set(selectedIds), [selectedIds]);
  const selectedCount = selectedIds.length;
  const allCurrentSelected = records.length > 0 && records.every((record) => selectedIdSet.has(record.id));
  const someCurrentSelected = records.some((record) => selectedIdSet.has(record.id));

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
      const currentIds = new Set(records.map((record) => record.id));
      setSelectedIds((items) => items.filter((id) => !currentIds.has(id)));
      return;
    }
    setSelectedIds((items) => {
      const next = [...items];
      for (const record of records) {
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
    setDeleteBusy(true);
    setBanner({ kind: "loading", message: t("loading", { ns: "common" }) });
    try {
      const response = await deleteIntrusions({ ids: selectedIds }, locale);
      setSelectedIds([]);
      setDeleteConfirmOpen(false);
      await loadRecords({ preserveBanner: true });
      setBanner({ kind: "success", message: t("intrusionDeleteSuccess", { ns: "settings", count: response.deleted }) });
    } catch (error) {
      setBanner({ kind: "error", message: extractErrorMessage(error, t("unexpectedError", { ns: "common" })) });
    } finally {
      setDeleteBusy(false);
    }
  };

  return (
    <section className="flex min-h-0 min-w-0 flex-1">
      <Panel className="flex min-h-0 min-w-0 flex-1 flex-col">
        <PanelBody className="min-h-0 min-w-0 flex-1">
          <SectionHeader
            title={t("intrusionsTitle", { ns: "settings" })}
            description={t("intrusionsDescription", { ns: "settings" })}
            action={
              <div className="flex flex-wrap justify-end gap-2">
                <button
                  className="btn btn-sm btn-outline btn-error"
                  type="button"
                  disabled={selectedCount === 0 || loading || deleteBusy}
                  onClick={() => setDeleteConfirmOpen(true)}
                >
                  <Trash2 size={16} />
                  <span>{t("intrusionDeleteSelected", { ns: "settings", count: selectedCount })}</span>
                </button>
                <button className="btn btn-sm btn-outline btn-info" type="button" onClick={() => void loadRecords()} disabled={loading || deleteBusy}>
                  <RefreshCw size={16} className={loading ? "animate-spin" : undefined} />
                  <span>{t("refresh", { ns: "common" })}</span>
                </button>
              </div>
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
              {t("intrusionCount", { ns: "settings", value: records.length })} · {t("intrusionSelectedCount", { ns: "settings", value: selectedCount })} · {t("intrusionTrajectoryCount", { ns: "settings", value: totalTrajectoryCount })}
            </span>
          </div>

          {(banner.kind === "error" || banner.kind === "success") && banner.message ? (
            <div className={cx("alert alert-soft py-3 text-sm", banner.kind === "error" ? "alert-error" : "alert-success")} role="alert">
              <span className="min-w-0 [overflow-wrap:anywhere]">{banner.message}</span>
            </div>
          ) : null}

          <div className="min-h-0 min-w-0 flex-1 overflow-auto rounded-2xl border border-base-300 bg-base-100/70">
            <table className="table table-zebra table-sm w-full min-w-[116rem] table-fixed whitespace-nowrap">
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
                      aria-label={t("intrusionSelectCurrentPage", { ns: "settings" })}
                      disabled={records.length === 0 || loading || deleteBusy}
                      onChange={(event) => toggleCurrentPageSelection(event.currentTarget.checked)}
                    />
                  </th>
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
                    <td colSpan={14} className="p-3">
                      <div className="admin-empty-state admin-empty-state--table">
                        {loading ? t("loading", { ns: "common" }) : t("empty", { ns: "common" })}
                      </div>
                    </td>
                  </tr>
                ) : (
                  records.map((record) => (
                    <tr key={record.id} className="row-hover">
                      <td>
                        <input
                          className="checkbox checkbox-sm"
                          type="checkbox"
                          checked={selectedIdSet.has(record.id)}
                          aria-label={t("intrusionSelectRecord", { ns: "settings" })}
                          disabled={deleteBusy}
                          onChange={(event) => toggleRecordSelection(record.id, event.currentTarget.checked)}
                        />
                      </td>
                      <td>
                        <Badge tone={targetTypeTone(record.targetType)}>{targetTypeLabel(record.targetType, t)}</Badge>
                      </td>
                      <td>
                        <OverflowCell
                          label={t("intrusionModel", { ns: "settings" })}
                          value={record.model || "-"}
                          onOpen={setCellDetail}
                        />
                      </td>
                      <td>
                        <OverflowCell
                          label={t("intrusionIdentity", { ns: "settings" })}
                          value={record.serial || record.device || record.targetId}
                          mono
                          className="rounded-xl bg-base-200/80 px-2 py-1 text-xs"
                          onOpen={setCellDetail}
                        />
                      </td>
                      <td className="tabular-nums">{formatFrequency(locale, record.frequency)}</td>
                      <td className="tabular-nums">{formatRSSI(locale, record.rssi)}</td>
                      <td className="tabular-nums">
                        <OverflowCell
                          label={t("intrusionFirstSeen", { ns: "settings" })}
                          value={formatTime(locale, record.firstSeen)}
                          className="tabular-nums"
                          onOpen={setCellDetail}
                        />
                      </td>
                      <td className="tabular-nums">
                        <OverflowCell
                          label={t("intrusionLastSeen", { ns: "settings" })}
                          value={formatTime(locale, record.lastSeen)}
                          className="tabular-nums"
                          onOpen={setCellDetail}
                        />
                      </td>
                      <td className="tabular-nums">{formatDuration(record.durationSeconds, t)}</td>
                      <td className="tabular-nums">{formatNumber(locale, record.hitCount, 0)}</td>
                      <td>
                        <CoordinateMapCell
                          label={t("intrusionCoordinates", { ns: "settings" })}
                          value={coordinateSummary(record, t)}
                          record={record}
                          onOpenText={setCellDetail}
                          onOpenMap={setMapRecord}
                        />
                      </td>
                      <td className="tabular-nums">{formatOptionalMetric(locale, record.speed, "m/s", 1)}</td>
                      <td className="tabular-nums">{formatOptionalMetric(locale, record.height, "m", 0)}</td>
                      <td>
                        <OverflowCell
                          label={t("intrusionSource", { ns: "settings" })}
                          value={sourceSummary(record.source, record.sources)}
                          onOpen={setCellDetail}
                        />
                      </td>
                    </tr>
                  ))
                )}
              </tbody>
            </table>
          </div>
        </PanelBody>
      </Panel>
      <CellDetailModal
        detail={cellDetail}
        t={t}
        onClose={() => setCellDetail(null)}
      />
      <IntrusionMapModal
        record={mapRecord}
        t={t}
        onClose={() => setMapRecord(null)}
      />
      <DeleteConfirmModal
        count={deleteConfirmOpen ? selectedCount : 0}
        busy={deleteBusy}
        t={t}
        onCancel={() => setDeleteConfirmOpen(false)}
        onConfirm={() => void deleteSelectedRecords()}
      />
    </section>
  );
}
