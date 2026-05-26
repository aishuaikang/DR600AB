import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import type { TFunction } from "i18next";
import { ChevronDown, MapPinned, RefreshCw, ShieldMinus, ShieldPlus, Trash2, X } from "lucide-react";

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
  UserSettings,
} from "../types";
import { cx } from "../utils/classnames";
import { formatNumber, formatTime } from "../utils/format";
import { resolveDisplayModel } from "../utils/models";
import { extractErrorMessage } from "../utils/session";
import { isSerialWhitelisted, normalizeWhitelistSerial, removeWhitelistSerial, upsertWhitelistItem } from "../utils/whitelist";
import { PositionMap } from "./ScreenMap";
import { referenceMapLayers } from "./screenData";

type IntrusionFilter = "all" | IntrusionTargetType;
type CellDetail = {
  title: string;
  content: string;
  mono?: boolean;
};

const intrusionPageSize = 50;
const intrusionFilters: IntrusionFilter[] = ["all", "detection", "position"];

function appendIntrusionRecords(
  current: IntrusionRecord[],
  incoming: IntrusionRecord[],
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

function formatIntrusionDateKey(value: string) {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return "";
  }
  const year = String(date.getFullYear());
  const month = String(date.getMonth() + 1).padStart(2, "0");
  const day = String(date.getDate()).padStart(2, "0");
  return `${year}-${month}-${day}`;
}

function normalizeIntrusionQuery(value: string | undefined) {
  return (value || "").trim().toLowerCase();
}

function intrusionDateRangeMatches(record: IntrusionRecord, from: string, to: string) {
  if (!from && !to) {
    return true;
  }
  const start = formatIntrusionDateKey(record.firstSeen);
  const end = formatIntrusionDateKey(record.lastSeen) || start;
  if (!start || !end) {
    return false;
  }
  if (from && end < from) {
    return false;
  }
  if (to && start > to) {
    return false;
  }
  return true;
}

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

function formatDistanceMetric(locale: string, value: number | undefined) {
  if (typeof value !== "number" || !Number.isFinite(value)) {
    return "-";
  }
  if (Math.abs(value) >= 1000) {
    return `${formatNumber(locale, value / 1000, value >= 100_000 ? 0 : 1)} km`;
  }
  return `${formatNumber(locale, value, 0)} m`;
}

function hasDisplayableIntrusionPoint(point?: ScreenPositionPoint | null): point is ScreenPositionPoint {
  if (!point) {
    return false;
  }
  return typeof point.latitude === "number" && typeof point.longitude === "number";
}

function formatCoordinateNumber(value: number) {
  return Number.isFinite(value) ? value.toFixed(6) : String(value);
}

function formatPoint(point?: ScreenPositionPoint | null) {
  if (!hasDisplayableIntrusionPoint(point)) {
    return "-";
  }
  return `${formatCoordinateNumber(point.latitude)}, ${formatCoordinateNumber(point.longitude)}`;
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
  if (hasDisplayableIntrusionPoint(record.deviceLocation?.point)) {
    parts.push(`${t("intrusionDeviceLocation", { ns: "settings" })}: ${formatPoint(record.deviceLocation.point)}`);
  }
  if (record.targetType !== "position") {
    return parts.length > 0 ? parts.join(" / ") : "-";
  }
  if (hasDisplayableIntrusionPoint(record.drone)) {
    parts.push(`${t("intrusionDrone", { ns: "settings" })}: ${formatPoint(record.drone)}`);
  }
  if (hasDisplayableIntrusionPoint(record.pilot)) {
    parts.push(`${t("intrusionPilot", { ns: "settings" })}: ${formatPoint(record.pilot)}`);
  }
  if (hasDisplayableIntrusionPoint(record.home)) {
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

function hasIntrusionMapData(record: IntrusionRecord) {
  return (
    Boolean(record.deviceLocation?.valid && validIntrusionPoint(record.deviceLocation.point)) ||
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
    drone: validIntrusionPoint(record.drone) ? record.drone : undefined,
    pilot: validIntrusionPoint(record.pilot) ? record.pilot : undefined,
    droneTrajectory: (record.droneTrajectory ?? []).filter(validIntrusionTrackPoint),
    pilotTrajectory: (record.pilotTrajectory ?? []).filter(validIntrusionTrackPoint),
    height: record.height,
    altitude: record.altitude,
    speed: record.speed,
    pilotDistanceM: record.pilotDistanceM,
    droneDistanceM: record.droneDistanceM,
    droneDirectionDeg: record.droneDirectionDeg,
    deviceDirectionDeg: record.deviceDirectionDeg,
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
  mapLabel,
  value,
  record,
  onOpenMap,
}: {
  label: string;
  mapLabel: string;
  value: string;
  record: IntrusionRecord;
  onOpenMap: (record: IntrusionRecord) => void;
}) {
  const displayValue = value || "-";
  const lines = displayValue === "-" ? [displayValue] : displayValue.split(" / ");
  const hasMapData = hasIntrusionMapData(record);

  return (
    <div className="intrusion-coordinate-cell font-mono text-xs tabular-nums text-base-content/80">
      <div className="intrusion-coordinate-cell__values" aria-label={label}>
        {lines.map((line, index) => (
          <span key={`${index}-${line}`}>{line}</span>
        ))}
      </div>
      {hasMapData ? (
        <button
          className="intrusion-coordinate-cell__map-button"
          type="button"
          title={mapLabel}
          aria-label={mapLabel}
          onClick={() => onOpenMap(record)}
        >
          <MapPinned size={14} aria-hidden="true" />
        </button>
      ) : null}
    </div>
  );
}

function WhitelistActionButton({
  record,
  whitelisted,
  busy,
  disabled,
  t,
  onToggle,
}: {
  record: IntrusionRecord;
  whitelisted: boolean;
  busy: boolean;
  disabled: boolean;
  t: TFunction;
  onToggle: (record: IntrusionRecord) => void;
}) {
  if (record.targetType !== "position") {
    return null;
  }

  const label = whitelisted
    ? t("removeFromWhitelist", { ns: "screen" })
    : t("addToWhitelist", { ns: "screen" });

  return (
    <button
      className={cx("intrusion-whitelist-button", whitelisted && "intrusion-whitelist-button--active")}
      type="button"
      disabled={disabled || busy}
      title={label}
      aria-label={label}
      onClick={() => onToggle(record)}
    >
      {whitelisted ? <ShieldMinus size={13} aria-hidden="true" /> : <ShieldPlus size={13} aria-hidden="true" />}
      <span>{busy ? t("loading", { ns: "common" }) : label}</span>
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
  const hasTargetMapData =
    validIntrusionPoint(record.drone) ||
    validIntrusionPoint(record.pilot) ||
    Boolean(record.droneTrajectory?.some(validIntrusionTrackPoint)) ||
    Boolean(record.pilotTrajectory?.some(validIntrusionTrackPoint));
  const identity = record.serial || record.device || record.targetId || record.id;
  const title = resolveDisplayModel(record) || t("intrusionMapTitle", { ns: "settings" });

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
          deviceLocation={hasTargetMapData ? null : record.deviceLocation ?? null}
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
  userSettings,
  t,
  onUserSettingsChange,
}: {
  locale: string;
  userSettings: UserSettings;
  t: TFunction;
  onUserSettingsChange: (settings: UserSettings) => Promise<UserSettings>;
}) {
  const [records, setRecords] = useState<IntrusionRecord[]>([]);
  const [filter, setFilter] = useState<IntrusionFilter>("all");
  const [modelQuery, setModelQuery] = useState("");
  const [serialQuery, setSerialQuery] = useState("");
  const [intrusionDateFrom, setIntrusionDateFrom] = useState("");
  const [intrusionDateTo, setIntrusionDateTo] = useState("");
  const [banner, setBanner] = useState<Banner>({ kind: "idle", message: "" });
  const [loading, setLoading] = useState(false);
  const [loadingMore, setLoadingMore] = useState(false);
  const [hasMore, setHasMore] = useState(false);
  const [nextOffset, setNextOffset] = useState(0);
  const [cellDetail, setCellDetail] = useState<CellDetail | null>(null);
  const [mapRecord, setMapRecord] = useState<IntrusionRecord | null>(null);
  const [selectedIds, setSelectedIds] = useState<string[]>([]);
  const [deleteConfirmOpen, setDeleteConfirmOpen] = useState(false);
  const [deleteBusy, setDeleteBusy] = useState(false);
  const [whitelistBusySerial, setWhitelistBusySerial] = useState("");

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
      const response = await getIntrusions(locale, intrusionPageSize, filter, offset);
      const items = response.items ?? [];
      setRecords((current) => (append ? appendIntrusionRecords(current, items) : items));
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
    void loadRecords();
  }, [loadRecords]);

  const handleIntrusionDateFromChange = useCallback((value: string) => {
    setIntrusionDateFrom(value);
    setIntrusionDateTo((currentTo) => (value && currentTo && currentTo < value ? value : currentTo));
  }, []);

  const handleIntrusionDateToChange = useCallback((value: string) => {
    setIntrusionDateTo(value && intrusionDateFrom && value < intrusionDateFrom ? intrusionDateFrom : value);
  }, [intrusionDateFrom]);

  const clearIntrusionFilters = useCallback(() => {
    setModelQuery("");
    setSerialQuery("");
    setIntrusionDateFrom("");
    setIntrusionDateTo("");
  }, []);

  const visibleRecords = useMemo(() => {
    const modelNeedle = normalizeIntrusionQuery(modelQuery);
    const serialNeedle = normalizeIntrusionQuery(serialQuery);
    return records.filter((record) => {
      const modelText = normalizeIntrusionQuery(`${resolveDisplayModel(record)} ${record.model || ""}`);
      const serialText = normalizeIntrusionQuery(record.serial);
      if (modelNeedle && !modelText.includes(modelNeedle)) {
        return false;
      }
      if (serialNeedle && !serialText.includes(serialNeedle)) {
        return false;
      }
      return intrusionDateRangeMatches(record, intrusionDateFrom, intrusionDateTo);
    });
  }, [intrusionDateFrom, intrusionDateTo, modelQuery, records, serialQuery]);

  useEffect(() => {
    const availableIds = new Set(visibleRecords.map((record) => record.id));
    setSelectedIds((items) => {
      const next = items.filter((id) => availableIds.has(id));
      return next.length === items.length ? items : next;
    });
  }, [visibleRecords]);

  const totalTrajectoryCount = useMemo(
    () => visibleRecords.reduce((sum, record) => sum + (record.droneTrajectory?.length ?? 0) + (record.pilotTrajectory?.length ?? 0), 0),
    [visibleRecords],
  );
  const selectedIdSet = useMemo(() => new Set(selectedIds), [selectedIds]);
  const selectedCount = selectedIds.length;
  const allCurrentSelected = visibleRecords.length > 0 && visibleRecords.every((record) => selectedIdSet.has(record.id));
  const someCurrentSelected = visibleRecords.some((record) => selectedIdSet.has(record.id));
  const hasIntrusionFilters = Boolean(modelQuery.trim() || serialQuery.trim() || intrusionDateFrom || intrusionDateTo);

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

  const toggleRecordWhitelist = useCallback(async (record: IntrusionRecord) => {
    if (record.targetType !== "position") {
      return;
    }
    const serial = record.serial?.trim() ?? "";
    if (!serial) {
      setBanner({ kind: "error", message: t("whitelistSerialRequired", { ns: "settings" }) });
      return;
    }
    const busySerial = normalizeWhitelistSerial(serial);
    const whitelisted = isSerialWhitelisted(serial, userSettings.whitelist);
    setWhitelistBusySerial(busySerial);
    try {
      await onUserSettingsChange({
        ...userSettings,
        whitelist: whitelisted
          ? removeWhitelistSerial(userSettings.whitelist, serial)
          : upsertWhitelistItem(userSettings.whitelist, {
            serial,
            model: resolveDisplayModel(record) || record.model,
            source: record.source || "intrusion",
          }),
      });
      setBanner({ kind: "success", message: t(whitelisted ? "whitelistDeleted" : "whitelistSaved", { ns: "settings" }) });
    } catch (error) {
      setBanner({ kind: "error", message: extractErrorMessage(error, t("unexpectedError", { ns: "common" })) });
    } finally {
      setWhitelistBusySerial("");
    }
  }, [onUserSettingsChange, t, userSettings]);

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
                  disabled={selectedCount === 0 || loading || loadingMore || deleteBusy}
                  onClick={() => setDeleteConfirmOpen(true)}
                >
                  <Trash2 size={16} />
                  <span>{t("intrusionDeleteSelected", { ns: "settings", count: selectedCount })}</span>
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
              {t("intrusionCount", { ns: "settings", value: visibleRecords.length })} · {t("intrusionSelectedCount", { ns: "settings", value: selectedCount })} · {t("intrusionTrajectoryCount", { ns: "settings", value: totalTrajectoryCount })}
            </span>
          </div>

          <div className="flex flex-wrap items-end gap-2">
            <label className="flex min-w-0 flex-col gap-1 text-xs text-base-content/60">
              <span>{t("intrusionModel", { ns: "settings" })}</span>
              <input
                className="input input-bordered input-sm w-44 bg-base-100"
                type="search"
                value={modelQuery}
                placeholder={t("intrusionModel", { ns: "settings" })}
                onChange={(event) => setModelQuery(event.target.value)}
              />
            </label>
            <label className="flex min-w-0 flex-col gap-1 text-xs text-base-content/60">
              <span>{t("intrusionIdentity", { ns: "settings" })}</span>
              <input
                className="input input-bordered input-sm w-44 bg-base-100 font-mono"
                type="search"
                value={serialQuery}
                placeholder={t("intrusionIdentity", { ns: "settings" })}
                onChange={(event) => setSerialQuery(event.target.value)}
              />
            </label>
            <div className="flex min-w-0 flex-col gap-1 text-xs text-base-content/60">
              <span>{t("intrusionDateRange", { ns: "settings" })}</span>
              <div className="flex flex-wrap items-end gap-2">
                <label className="flex min-w-0 flex-col gap-1">
                  <span>{t("intrusionDateFrom", { ns: "settings" })}</span>
                  <input
                    className="input input-bordered input-sm w-44 bg-base-100"
                    type="date"
                    value={intrusionDateFrom}
                    onChange={(event) => handleIntrusionDateFromChange(event.target.value)}
                  />
                </label>
                <label className="flex min-w-0 flex-col gap-1">
                  <span>{t("intrusionDateTo", { ns: "settings" })}</span>
                  <input
                    className="input input-bordered input-sm w-44 bg-base-100"
                    type="date"
                    min={intrusionDateFrom || undefined}
                    value={intrusionDateTo}
                    onChange={(event) => handleIntrusionDateToChange(event.target.value)}
                  />
                </label>
              </div>
            </div>
            <button className="btn btn-sm btn-ghost" type="button" disabled={!hasIntrusionFilters} onClick={clearIntrusionFilters}>
              {t("clear", { ns: "common" })}
            </button>
          </div>

          {(banner.kind === "error" || banner.kind === "success") && banner.message ? (
            <div className={cx("alert alert-soft py-3 text-sm", banner.kind === "error" ? "alert-error" : "alert-success")} role="alert">
              <span className="min-w-0 [overflow-wrap:anywhere]">{banner.message}</span>
            </div>
          ) : null}

          <div className="min-h-0 min-w-0 flex-1 overflow-auto rounded-2xl border border-base-300 bg-base-100/70">
            <table className="table table-zebra table-sm w-full min-w-[115rem] table-fixed whitespace-nowrap">
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
                      disabled={visibleRecords.length === 0 || loading || loadingMore || deleteBusy}
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
                  <th className="w-[22rem]">{t("intrusionCoordinates", { ns: "settings" })}</th>
                  <th className="w-[10rem]">{t("intrusionPilotDistance", { ns: "settings" })}</th>
                  <th className="w-[10rem]">{t("intrusionDroneDistance", { ns: "settings" })}</th>
                  <th className="w-[9rem]">{t("intrusionSpeed", { ns: "settings" })}</th>
                  <th className="w-[9rem]">{t("intrusionHeight", { ns: "settings" })}</th>
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
                            ? t("intrusionNoMatch", { ns: "settings" })
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
                          value={resolveDisplayModel(record) || "-"}
                          onOpen={setCellDetail}
                        />
                      </td>
                      <td>
                        <div className="intrusion-identity-cell">
                          <OverflowCell
                            label={t("intrusionIdentity", { ns: "settings" })}
                            value={record.serial || "-"}
                            mono
                            className="rounded-xl bg-base-200/80 px-2 py-1 text-xs"
                            onOpen={setCellDetail}
                          />
                          <WhitelistActionButton
                            record={record}
                            whitelisted={isSerialWhitelisted(record.serial, userSettings.whitelist)}
                            busy={Boolean(record.serial && whitelistBusySerial === normalizeWhitelistSerial(record.serial))}
                            disabled={deleteBusy || Boolean(whitelistBusySerial) || !record.serial?.trim()}
                            t={t}
                            onToggle={(target) => void toggleRecordWhitelist(target)}
                          />
                        </div>
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
                      <td>
                        <CoordinateMapCell
                          label={t("intrusionCoordinates", { ns: "settings" })}
                          mapLabel={t("intrusionMapTitle", { ns: "settings" })}
                          value={coordinateSummary(record, t)}
                          record={record}
                          onOpenMap={setMapRecord}
                        />
                      </td>
                      <td className="tabular-nums">{formatDistanceMetric(locale, record.pilotDistanceM)}</td>
                      <td className="tabular-nums">{formatDistanceMetric(locale, record.droneDistanceM)}</td>
                      <td className="tabular-nums">{formatOptionalMetric(locale, record.speed, "m/s", 1)}</td>
                      <td className="tabular-nums">{formatOptionalMetric(locale, record.height, "m", 0)}</td>
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
