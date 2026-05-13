import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import type { ReactNode, UIEvent } from "react";
import { useTranslation } from "react-i18next";
import type { TFunction } from "i18next";
import {
  Activity,
  ChevronDown,
  CircleAlert,
  FileText,
  Fingerprint,
  Languages,
  Play,
  Radio,
  RefreshCw,
  Settings2,
  Shield,
  Square,
  Wrench,
  Zap,
} from "lucide-react";
import type { LucideIcon } from "lucide-react";

import {
  getChannels,
  getDetectionSettings,
  getLocales,
  getParsed,
  getPorts,
  getSession,
  openDetectionStream,
  setChannelState,
  updateDetectionSettings,
} from "./api";
import { getStoredLocale, persistLocale, supportedLocales } from "./i18n";
import { FIXED_SERIAL_PROFILE } from "./serial-profile";
import type {
  DetectionSessionResponse,
  DetectionSettings,
  GpioChannel,
  LocaleMeta,
  ParsedMessage,
  ParsedMessageType,
  PortInfo,
} from "./types";

type Page = ParsedMessageType | "interference" | "settings";
type Tone = "neutral" | "success" | "warning" | "error" | "info";
type NavItem = { id: Page; icon: LucideIcon; labelKey: string };

type MessageColumn = {
  labelKey: string;
  width: string;
  render: (record: ParsedMessage, locale: string) => ReactNode;
};

type DetailContent = {
  title: string;
  value: string;
};

type MessagePageConfig = {
  icon: LucideIcon;
  navLabelKey: string;
  titleKey: string;
  tone: Tone;
  tableWidth: string;
  columns: MessageColumn[];
};

const VIRTUAL_TABLE_ROW_HEIGHT = 48;
const VIRTUAL_TABLE_OVERSCAN = 8;
const TIME_COLUMN_WIDTH = "w-[13rem]";
const DETAIL_COLUMN_WIDTH = "w-[24rem]";

const MESSAGE_PAGE_ORDER: ParsedMessageType[] = [
  "did_encrypted",
  "rid",
  "did_plain",
  "detect",
  "heartbeat",
];

const MESSAGE_PAGE_CONFIG: Record<ParsedMessageType, MessagePageConfig> = {
  did_encrypted: {
    icon: Shield,
    navLabelKey: "didEncrypted",
    titleKey: "didEncrypted.title",
    tone: "info",
    tableWidth: "min-w-[118rem]",
    columns: [
      {
        labelKey: "didEncrypted.device",
        width: "w-[18rem]",
        render: (record) => getTextValue(getRecordData(record).device),
      },
      {
        labelKey: "didEncrypted.encryptedId",
        width: "w-[38rem]",
        render: (record) => getTextValue(getRecordData(record).encrypted_id),
      },
      {
        labelKey: "didEncrypted.frequency",
        width: "w-[10rem]",
        render: (record, locale) => formatNumber(locale, getNumberValue(getRecordData(record).freq)),
      },
      {
        labelKey: "didEncrypted.rssi",
        width: "w-[9rem]",
        render: (record, locale) => formatNumber(locale, getNumberValue(getRecordData(record).rssi)),
      },
      {
        labelKey: "didEncrypted.bytes",
        width: "w-[24rem]",
        render: (record) => getTextValue(getRecordData(record).bytes),
      },
    ],
  },
  rid: {
    icon: Fingerprint,
    navLabelKey: "rid",
    titleKey: "rid.title",
    tone: "success",
    tableWidth: "min-w-[142rem]",
    columns: [
      {
        labelKey: "rid.ssid",
        width: "w-[16rem]",
        render: (record) => getTextValue(getRecordData(record).ssid),
      },
      {
        labelKey: "rid.serial",
        width: "w-[18rem]",
        render: (record) => getTextValue(getRecordData(record).serial),
      },
      {
        labelKey: "rid.model",
        width: "w-[16rem]",
        render: (record) => getTextValue(getRecordData(record).model),
      },
      {
        labelKey: "rid.uaType",
        width: "w-[10rem]",
        render: (record) => getTextValue(getRecordField(record, "ua_type", "UA_type")),
      },
      {
        labelKey: "rid.droneGps",
        width: "w-[18rem]",
        render: (record) => formatGps(getRecordField(record, "drone_gps", "drone_GPS")),
      },
      {
        labelKey: "rid.pilotGps",
        width: "w-[18rem]",
        render: (record) => formatGps(getRecordField(record, "pilot_gps", "pilot_GPS")),
      },
      {
        labelKey: "rid.frequency",
        width: "w-[10rem]",
        render: (record, locale) => formatNumber(locale, getNumberValue(getRecordData(record).freq)),
      },
      {
        labelKey: "rid.rssi",
        width: "w-[9rem]",
        render: (record, locale) => formatNumber(locale, getNumberValue(getRecordData(record).rssi)),
      },
    ],
  },
  did_plain: {
    icon: FileText,
    navLabelKey: "didPlain",
    titleKey: "didPlain.title",
    tone: "warning",
    tableWidth: "min-w-[136rem]",
    columns: [
      {
        labelKey: "didPlain.device",
        width: "w-[16rem]",
        render: (record) => getTextValue(getRecordData(record).device),
      },
      {
        labelKey: "didPlain.serial",
        width: "w-[18rem]",
        render: (record) => getTextValue(getRecordData(record).serial),
      },
      {
        labelKey: "didPlain.model",
        width: "w-[16rem]",
        render: (record) => getTextValue(getRecordData(record).model),
      },
      {
        labelKey: "didPlain.uuid",
        width: "w-[30rem]",
        render: (record) => getTextValue(getRecordData(record).uuid),
      },
      {
        labelKey: "didPlain.distance",
        width: "w-[10rem]",
        render: (record) => getTextValue(getRecordData(record).distance),
      },
      {
        labelKey: "didPlain.frequency",
        width: "w-[10rem]",
        render: (record, locale) => formatNumber(locale, getNumberValue(getRecordData(record).freq)),
      },
      {
        labelKey: "didPlain.rssi",
        width: "w-[9rem]",
        render: (record, locale) => formatNumber(locale, getNumberValue(getRecordData(record).rssi)),
      },
    ],
  },
  detect: {
    icon: Radio,
    navLabelKey: "detect",
    titleKey: "detect.title",
    tone: "info",
    tableWidth: "min-w-[90rem]",
    columns: [
      {
        labelKey: "detect.device",
        width: "w-[20rem]",
        render: (record) => getTextValue(getRecordData(record).device),
      },
      {
        labelKey: "detect.model",
        width: "w-[18rem]",
        render: (record) => getTextValue(getRecordData(record).model),
      },
      {
        labelKey: "detect.frequency",
        width: "w-[10rem]",
        render: (record, locale) => formatNumber(locale, getNumberValue(getRecordData(record).freq)),
      },
      {
        labelKey: "detect.rssi",
        width: "w-[9rem]",
        render: (record, locale) => formatNumber(locale, getNumberValue(getRecordData(record).rssi)),
      },
    ],
  },
  heartbeat: {
    icon: Activity,
    navLabelKey: "heartbeat",
    titleKey: "heartbeat.title",
    tone: "error",
    tableWidth: "min-w-[72rem]",
    columns: [
      {
        labelKey: "heartbeat.device",
        width: "w-[18rem]",
        render: (record) => getTextValue(getRecordData(record).device),
      },
      {
        labelKey: "heartbeat.seq",
        width: "w-[12rem]",
        render: (record) => getTextValue(getRecordData(record).seq),
      },
    ],
  },
};

const debugPageItems: NavItem[] = [
  { id: "heartbeat", icon: MESSAGE_PAGE_CONFIG.heartbeat.icon, labelKey: MESSAGE_PAGE_CONFIG.heartbeat.navLabelKey },
  { id: "detect", icon: MESSAGE_PAGE_CONFIG.detect.icon, labelKey: MESSAGE_PAGE_CONFIG.detect.navLabelKey },
  { id: "did_encrypted", icon: MESSAGE_PAGE_CONFIG.did_encrypted.icon, labelKey: MESSAGE_PAGE_CONFIG.did_encrypted.navLabelKey },
  { id: "did_plain", icon: MESSAGE_PAGE_CONFIG.did_plain.icon, labelKey: MESSAGE_PAGE_CONFIG.did_plain.navLabelKey },
  { id: "rid", icon: MESSAGE_PAGE_CONFIG.rid.icon, labelKey: MESSAGE_PAGE_CONFIG.rid.navLabelKey },
  { id: "interference", icon: Zap, labelKey: "interference" },
];

const pageItems: NavItem[] = [
  ...debugPageItems,
  { id: "settings", icon: Settings2, labelKey: "settings" },
];

type Banner = {
  kind: "idle" | "loading" | "success" | "error";
  message: string;
};

function cx(...classes: Array<string | false | null | undefined>) {
  return classes.filter(Boolean).join(" ");
}

function normalizePage(hash: string): Page {
  const next = hash.replace(/^#\/?/, "");
  return pageItems.some((item) => item.id === next) ? (next as Page) : "did_encrypted";
}

function useHashPage(): [Page, (page: Page) => void] {
  const [page, setPage] = useState<Page>(() =>
    typeof window === "undefined" ? "did_encrypted" : normalizePage(window.location.hash),
  );

  useEffect(() => {
    const onHashChange = () => setPage(normalizePage(window.location.hash));
    window.addEventListener("hashchange", onHashChange);
    if (!window.location.hash) {
      window.location.hash = "#/did_encrypted";
    }
    return () => window.removeEventListener("hashchange", onHashChange);
  }, []);

  const navigate = useCallback((next: Page) => {
    window.location.hash = `#/${next}`;
  }, []);

  return [page, navigate];
}

function formatTime(locale: string, value?: string) {
  if (!value) {
    return "-";
  }
  return new Intl.DateTimeFormat(locale, {
    dateStyle: "medium",
    timeStyle: "medium",
  }).format(new Date(value));
}

function formatNumber(locale: string, value?: number, digits = 1) {
  if (typeof value !== "number" || Number.isNaN(value)) {
    return "-";
  }
  return new Intl.NumberFormat(locale, {
    maximumFractionDigits: digits,
  }).format(value);
}

function serialKey(receivePort: string, sendPort: string) {
  return `${receivePort.trim()}|${sendPort.trim()}`;
}

function resolveInitialPorts(
  session: DetectionSessionResponse | null,
  settings: DetectionSettings | null,
  ports: PortInfo[],
) {
  const activePorts = ports.filter((item) => item.active).map((item) => item.name);
  const sessionReceive = session?.rxPortName || session?.portName || "";
  const sessionSend = session?.txPortName || sessionReceive || "";
  const savedReceive = settings?.rxPortName || settings?.portName || "";
  const savedSend = settings?.txPortName || savedReceive || "";

  const receivePort = sessionReceive || savedReceive || activePorts[0] || ports[0]?.name || "";
  const sendPort =
    sessionSend
    || settings?.txPortName
    || activePorts.find((item) => item !== receivePort)
    || savedSend
    || receivePort;

  return { receivePort, sendPort };
}

function sessionBannerText(session: DetectionSessionResponse, fallback: string) {
  const message = session.message || fallback;
  if (session.lastError && session.state && session.state !== "connected" && session.state !== "inactive") {
    return `${message}：${session.lastError}`;
  }
  return message;
}

function sessionBannerKind(session: DetectionSessionResponse): Banner["kind"] {
  if (session.state === "connected" || session.active) {
    return "success";
  }
  if (session.state === "connecting" || session.state === "reconnecting") {
    return "loading";
  }
  return "idle";
}

function dedupeById<T extends { id: string }>(items: T[], item: T, limit: number) {
  return [item, ...items.filter((entry) => entry.id !== item.id)].slice(0, limit);
}

function dedupeParsed(items: ParsedMessage[], item: ParsedMessage, limit: number) {
  const key = `${item.type}|${item.time}|${item.raw}`;
  return [item, ...items.filter((entry) => `${entry.type}|${entry.time}|${entry.raw}` !== key)].slice(0, limit);
}

function extractErrorMessage(error: unknown) {
  if (error instanceof Error) {
    return error.message;
  }
  return "Unexpected error";
}

function toneForStatus(kind: string): Tone {
  switch (kind) {
    case "success":
    case "active":
    case "enabled":
      return "success";
    case "loading":
    case "idle":
      return "neutral";
    case "error":
    case "reserved":
      return "error";
    default:
      return "warning";
  }
}

function channelStatusLabel(channel: GpioChannel, t: TFunction) {
  if (channel.reserved) {
    return t("reserved", { ns: "common" });
  }
  if (channel.status === "active" || channel.status === "enabled") {
    return t("statusActive", { ns: "interference" });
  }
  if (channel.status === "idle" || channel.status === "disabled") {
    return t("statusIdle", { ns: "interference" });
  }
  if (channel.status === "ready") {
    return t("statusReady", { ns: "interference" });
  }
  if (channel.status === "error") {
    return t("statusError", { ns: "interference" });
  }
  return channel.status;
}

function getRecordData(record: ParsedMessage): Record<string, unknown> {
  const data = record.data;
  if (data && typeof data === "object" && !Array.isArray(data)) {
    return data as Record<string, unknown>;
  }
  return {};
}

function getRecordField(record: ParsedMessage, ...keys: string[]): unknown {
  const data = getRecordData(record);
  for (const key of keys) {
    if (data[key] !== undefined && data[key] !== null) {
      return data[key];
    }
  }
  return undefined;
}

function getTextValue(value: unknown): string {
  if (value === null || value === undefined) {
    return "-";
  }
  if (typeof value === "string") {
    return value.trim() || "-";
  }
  if (typeof value === "number") {
    return Number.isFinite(value) ? String(value) : "-";
  }
  if (typeof value === "boolean") {
    return value ? "true" : "false";
  }
  if (Array.isArray(value)) {
    return value.map((item) => getTextValue(item)).join(", ");
  }
  if (typeof value === "object") {
    return JSON.stringify(value);
  }
  return String(value);
}

function getNumberValue(value: unknown): number | undefined {
  if (typeof value === "number" && Number.isFinite(value)) {
    return value;
  }
  if (typeof value === "string" && value.trim() !== "") {
    const next = Number(value);
    if (Number.isFinite(next)) {
      return next;
    }
  }
  return undefined;
}

function formatGps(value: unknown): string {
  if (value && typeof value === "object" && !Array.isArray(value)) {
    const gps = value as { lat?: unknown; lng?: unknown };
    const lat = getNumberValue(gps.lat);
    const lng = getNumberValue(gps.lng);
    if (typeof lat === "number" && typeof lng === "number") {
      return `${lat.toFixed(6)}, ${lng.toFixed(6)}`;
    }
  }
  return getTextValue(value);
}

function buildSearchText(record: ParsedMessage): string {
  return `${record.type} ${record.raw} ${JSON.stringify(record.data ?? {})}`.toLowerCase();
}

function Badge({
  children,
  tone = "neutral",
  outline = false,
}: {
  children: ReactNode;
  tone?: Tone;
  outline?: boolean;
}) {
  const toneClass: Record<Tone, string> = {
    neutral: "badge-ghost",
    success: "badge-success",
    warning: "badge-warning",
    error: "badge-error",
    info: "badge-info",
  };
  const variantClass = outline ? "badge-outline" : tone === "neutral" ? "badge-ghost" : "badge-soft";

  return <span className={cx("badge badge-sm max-w-full whitespace-nowrap", toneClass[tone], variantClass)}>{children}</span>;
}

function SectionHeader({
  title,
  description,
  action,
}: {
  title: string;
  description?: string;
  action?: ReactNode;
}) {
  return (
    <div className="flex flex-col gap-3 border-b border-base-300/80 pb-4 sm:flex-row sm:items-start sm:justify-between">
      <div className="min-w-0">
        <h2 className="text-base font-semibold leading-6 text-base-content">{title}</h2>
        {description ? <p className="mt-1 max-w-3xl text-sm leading-6 text-base-content/65">{description}</p> : null}
      </div>
      {action ? <div className="flex shrink-0 items-center gap-2">{action}</div> : null}
    </div>
  );
}

function Panel({ children, className }: { children: ReactNode; className?: string }) {
  return (
    <section className={cx("rounded-3xl border border-base-300 bg-base-200/80 shadow-sm shadow-black/20", className)}>
      {children}
    </section>
  );
}

function PanelBody({ children, className }: { children: ReactNode; className?: string }) {
  return <div className={cx("flex flex-col gap-4 p-4 sm:p-5", className)}>{children}</div>;
}

function InfoTile({
  label,
  value,
  children,
}: {
  label: string;
  value?: ReactNode;
  children?: ReactNode;
}) {
  return (
    <div className="min-w-0 rounded-3xl border border-base-300 bg-base-100/70 px-4 py-3">
      <span className="block text-xs font-medium text-base-content/55">{label}</span>
      <div className="mt-2 min-w-0 break-words text-sm font-semibold text-base-content">{children ?? value}</div>
    </div>
  );
}

function CellValue({
  children,
  detail,
  onOpenDetail,
}: {
  children: ReactNode;
  detail?: DetailContent;
  onOpenDetail?: (detail: DetailContent) => void;
}) {
  if (typeof children !== "string") {
    return <div className="max-w-full truncate whitespace-nowrap">{children}</div>;
  }

  return <LongTextCell value={children} detail={detail} onOpenDetail={onOpenDetail} />;
}

function LongTextCell({
  value,
  detail,
  onOpenDetail,
}: {
  value: string;
  detail?: DetailContent;
  onOpenDetail?: (detail: DetailContent) => void;
}) {
  const canOpen = Boolean(detail && value !== "-");
  const content = (
    <code
      className={cx(
        "block max-w-full truncate whitespace-nowrap rounded-xl bg-base-200/80 px-2 py-1 text-xs leading-5 text-base-content/75",
        canOpen ? "cursor-pointer hover:bg-base-300/80 hover:text-base-content" : "",
      )}
      title={value === "-" ? undefined : value}
    >
      {value}
    </code>
  );

  if (!canOpen || !detail || !onOpenDetail) {
    return content;
  }

  return (
    <button
      className="block max-w-full text-left"
      type="button"
      onClick={() => onOpenDetail(detail)}
      aria-label={detail.title}
    >
      {content}
    </button>
  );
}

function VirtualMessageTable({
  config,
  records,
  locale,
  resetKey,
  t,
}: {
  config: MessagePageConfig;
  records: ParsedMessage[];
  locale: string;
  resetKey: string;
  t: TFunction;
}) {
  const [scrollTop, setScrollTop] = useState(0);
  const [viewportHeight, setViewportHeight] = useState(420);
  const [detail, setDetail] = useState<DetailContent | null>(null);
  const containerRef = useRef<HTMLDivElement | null>(null);
  const totalHeight = records.length * VIRTUAL_TABLE_ROW_HEIGHT;
  const visibleCount = Math.max(1, Math.ceil(viewportHeight / VIRTUAL_TABLE_ROW_HEIGHT));
  const startIndex = Math.max(0, Math.floor(scrollTop / VIRTUAL_TABLE_ROW_HEIGHT) - VIRTUAL_TABLE_OVERSCAN);
  const endIndex = Math.min(records.length, startIndex + visibleCount + VIRTUAL_TABLE_OVERSCAN * 2);
  const visibleRecords = records.slice(startIndex, endIndex);
  const topPadding = startIndex * VIRTUAL_TABLE_ROW_HEIGHT;
  const bottomPadding = Math.max(0, totalHeight - topPadding - visibleRecords.length * VIRTUAL_TABLE_ROW_HEIGHT);
  const colSpan = config.columns.length + 2;

  const measureViewport = useCallback(() => {
    const nextHeight = containerRef.current?.clientHeight;
    if (nextHeight) {
      setViewportHeight(nextHeight);
    }
  }, []);

  useEffect(() => {
    measureViewport();
    window.addEventListener("resize", measureViewport);
    return () => window.removeEventListener("resize", measureViewport);
  }, [measureViewport]);

  useEffect(() => {
    setScrollTop(0);
    if (containerRef.current) {
      containerRef.current.scrollTop = 0;
    }
  }, [resetKey]);

  const handleScroll = useCallback((event: UIEvent<HTMLDivElement>) => {
    setScrollTop(event.currentTarget.scrollTop);
  }, []);

  return (
    <>
      <div
        ref={containerRef}
        className="min-h-0 min-w-0 flex-1 overflow-auto rounded-3xl border border-base-300 bg-base-100/70"
        onScroll={handleScroll}
      >
        <table className={cx("table table-zebra table-sm w-full table-fixed whitespace-nowrap", config.tableWidth)}>
          <thead className="sticky top-0 z-10 bg-base-200">
            <tr>
              <th className={TIME_COLUMN_WIDTH}>{t("time", { ns: "common" })}</th>
              {config.columns.map((column) => (
                <th key={column.labelKey} className={column.width}>
                  {t(column.labelKey, { ns: "messages" })}
                </th>
              ))}
              <th className={DETAIL_COLUMN_WIDTH}>{t("details", { ns: "common" })}</th>
            </tr>
          </thead>
          <tbody>
            {records.length === 0 ? (
              <tr>
                <td colSpan={colSpan} className="py-8 text-center text-sm text-base-content/55">
                  {t("empty", { ns: "common" })}
                </td>
              </tr>
            ) : (
              <>
                {topPadding > 0 ? (
                  <tr aria-hidden="true">
                    <td colSpan={colSpan} style={{ height: topPadding, padding: 0 }} />
                  </tr>
                ) : null}
                {visibleRecords.map((record) => (
                  <tr
                    key={`${record.type}-${record.time}-${record.raw}`}
                    className="row-hover"
                    style={{ height: VIRTUAL_TABLE_ROW_HEIGHT }}
                  >
                    <td className={cx("align-middle tabular-nums whitespace-nowrap", TIME_COLUMN_WIDTH)}>
                      <LongTextCell value={formatTime(locale, record.time)} />
                    </td>
                    {config.columns.map((column) => {
                      const rendered = column.render(record, locale);
                      const label = t(column.labelKey, { ns: "messages" });
                      const detailValue = typeof rendered === "string" ? rendered : undefined;

                      return (
                        <td
                          key={column.labelKey}
                          className={cx(
                            "align-middle overflow-hidden whitespace-nowrap",
                            column.width,
                            column.labelKey.includes("frequency") || column.labelKey.includes("rssi") ? "tabular-nums" : "",
                          )}
                        >
                          <CellValue
                            detail={detailValue ? { title: label, value: detailValue } : undefined}
                            onOpenDetail={setDetail}
                          >
                            {rendered}
                          </CellValue>
                        </td>
                      );
                    })}
                    <td className={cx("align-middle overflow-hidden whitespace-nowrap", DETAIL_COLUMN_WIDTH)}>
                      <LongTextCell
                        value={record.raw}
                        detail={{ title: t("details", { ns: "common" }), value: JSON.stringify(record.data, null, 2) }}
                        onOpenDetail={setDetail}
                      />
                    </td>
                  </tr>
                ))}
                {bottomPadding > 0 ? (
                  <tr aria-hidden="true">
                    <td colSpan={colSpan} style={{ height: bottomPadding, padding: 0 }} />
                  </tr>
                ) : null}
              </>
            )}
          </tbody>
        </table>
      </div>

      {detail ? <DetailDialog detail={detail} onClose={() => setDetail(null)} /> : null}
    </>
  );
}

function DetailDialog({ detail, onClose }: { detail: DetailContent; onClose: () => void }) {
  return (
    <div className="fixed inset-0 z-50 grid place-items-center bg-black/55 p-4" role="dialog" aria-modal="true">
      <div className="flex max-h-[80dvh] w-full max-w-3xl flex-col overflow-hidden rounded-[28px] border border-base-300 bg-base-100 shadow-2xl shadow-black/40">
        <div className="flex shrink-0 items-center justify-between gap-3 border-b border-base-300 px-5 py-4">
          <h2 className="min-w-0 truncate text-base font-semibold text-base-content">{detail.title}</h2>
          <button className="btn btn-sm btn-outline" type="button" onClick={onClose}>
            ×
          </button>
        </div>
        <pre className="min-h-0 flex-1 overflow-auto whitespace-pre-wrap break-words p-5 text-sm leading-6 text-base-content/80">
          {detail.value}
        </pre>
      </div>
    </div>
  );
}

function SelectField({
  label,
  value,
  disabled,
  children,
  onChange,
}: {
  label: string;
  value: string;
  disabled?: boolean;
  children: ReactNode;
  onChange: (value: string) => void;
}) {
  return (
    <label className="grid min-w-0 gap-2">
      <span className="text-xs font-medium text-base-content/60">{label}</span>
      <select
        className="select select-sm select-primary w-full bg-base-100"
        value={value}
        onChange={(event) => onChange(event.target.value)}
        disabled={disabled}
      >
        {children}
      </select>
    </label>
  );
}

function PortSelect({
  label,
  placeholder,
  value,
  ports,
  activeText,
  onChange,
}: {
  label: string;
  placeholder: string;
  value: string;
  ports: PortInfo[];
  activeText: string;
  onChange: (value: string) => void;
}) {
  const hasCurrent = Boolean(value) && !ports.some((port) => port.name === value);

  return (
    <SelectField label={label} value={value} disabled={ports.length === 0 && !hasCurrent} onChange={onChange}>
      <option value="">{placeholder}</option>
      {hasCurrent ? <option value={value}>{value}</option> : null}
      {ports.map((port) => (
        <option key={port.name} value={port.name}>
          {port.active ? `${port.name} (${activeText})` : port.name}
        </option>
      ))}
    </SelectField>
  );
}

function BannerAlert({ banner }: { banner: Banner }) {
  if (!banner.message || banner.kind !== "error") {
    return null;
  }

  return (
    <div
      className="alert alert-soft alert-error py-3 text-sm"
      role="alert"
      aria-live="assertive"
    >
      <CircleAlert size={16} />
      <span className="min-w-0 [overflow-wrap:anywhere]">{banner.message}</span>
    </div>
  );
}

function ChannelCard({
  channel,
  t,
  onToggle,
}: {
  channel: GpioChannel;
  t: TFunction;
  onToggle: (channel: GpioChannel) => void;
}) {
  const tone = channel.reserved ? "warning" : toneForStatus(channel.status);
  const toggleLabel = channel.enabled ? t("disable", { ns: "interference" }) : t("enable", { ns: "interference" });
  const bands = Array.isArray(channel.bands) ? channel.bands : [];

  return (
    <article className="flex min-w-0 flex-col gap-4 rounded-3xl border border-base-300 bg-base-100/70 p-4">
      <div className="flex min-w-0 items-start justify-between gap-3">
        <div className="min-w-0">
          <div className="flex flex-wrap items-center gap-2">
            <h3 className="text-base font-semibold leading-6 text-base-content">{channel.label}</h3>
            {channel.reserved ? <Badge tone="warning">{t("reserved", { ns: "common" })}</Badge> : null}
          </div>
          <p className="mt-1 text-xs text-base-content/55">
            {t("pin", { ns: "interference" })} GPIO{channel.pin}
          </p>
        </div>
        <Badge tone={tone}>{channelStatusLabel(channel, t)}</Badge>
      </div>

      <div className="flex flex-wrap gap-2">
        {bands.length > 0 ? (
          bands.map((band) => (
            <Badge key={band} tone="info" outline>
              {band}
            </Badge>
          ))
        ) : (
          <span className="text-sm text-base-content/55">{t("reservedChannel", { ns: "interference" })}</span>
        )}
      </div>

      <div className="grid gap-2 sm:grid-cols-2">
        <InfoTile label={t("desired", { ns: "interference" })} value={channel.desiredLevel} />
        <InfoTile label={t("actual", { ns: "interference" })} value={channel.actualLevel} />
      </div>

      {channel.lastError ? <p className="rounded-3xl bg-error/10 px-3 py-2 text-sm text-error">{channel.lastError}</p> : null}

      <div className="mt-auto grid gap-2">
        <button
          className={cx("btn btn-sm btn-block", channel.enabled ? "btn-outline btn-error" : "btn-primary")}
          type="button"
          disabled={channel.reserved}
          onClick={() => onToggle(channel)}
        >
          {channel.enabled ? <Square size={16} /> : <Play size={16} />}
          <span>{toggleLabel}</span>
        </button>
        <span className="text-xs text-base-content/50">
          {channel.reserved ? t("reservedChannel", { ns: "interference" }) : t("toggle", { ns: "interference" })}
        </span>
      </div>
    </article>
  );
}

function MessagePagePanel({
  page,
  records,
  locale,
  query,
  onQueryChange,
  t,
}: {
  page: ParsedMessageType;
  records: ParsedMessage[];
  locale: string;
  query: string;
  onQueryChange: (value: string) => void;
  t: TFunction;
}) {
  const config = MESSAGE_PAGE_CONFIG[page];
  const filteredRecords = useMemo(() => {
    const needle = query.trim().toLowerCase();
    if (!needle) {
      return records;
    }
    return records.filter((record) => buildSearchText(record).includes(needle));
  }, [query, records]);

  return (
    <section className="flex min-h-0 min-w-0 flex-1">
      <Panel className="flex min-h-0 min-w-0 flex-1 flex-col">
        <PanelBody className="min-h-0 min-w-0 flex-1">
          <label className="grid max-w-md gap-2">
            <span className="text-xs font-medium text-base-content/60">{t("search", { ns: "common" })}</span>
            <input
              className="input input-sm input-bordered w-full bg-base-100"
              value={query}
              onChange={(event) => onQueryChange(event.target.value)}
              placeholder={t("search", { ns: "common" })}
            />
          </label>

          <VirtualMessageTable config={config} records={filteredRecords} locale={locale} resetKey={`${page}:${query}`} t={t} />
        </PanelBody>
      </Panel>
    </section>
  );
}

function App() {
  const { t, i18n } = useTranslation();
  const [page, navigate] = useHashPage();
  const [locale, setLocale] = useState(() => getStoredLocale());
  const [meta, setMeta] = useState<LocaleMeta | null>(null);
  const [ports, setPorts] = useState<PortInfo[]>([]);
  const [session, setSession] = useState<DetectionSessionResponse | null>(null);
  const [messages, setMessages] = useState<ParsedMessage[]>([]);
  const [channels, setChannels] = useState<GpioChannel[]>([]);
  const [selectedReceivePort, setSelectedReceivePort] = useState("");
  const [selectedSendPort, setSelectedSendPort] = useState("");
  const [messageSearch, setMessageSearch] = useState("");
  const [banner, setBanner] = useState<Banner>({ kind: "idle", message: "" });
  const lastAppliedSerialRef = useRef("");

  const syncSerialSelection = useCallback((receivePort: string, sendPort: string) => {
    const nextReceivePort = receivePort.trim();
    const nextSendPort = sendPort.trim();
    lastAppliedSerialRef.current = serialKey(nextReceivePort, nextSendPort);
    setSelectedReceivePort(nextReceivePort);
    setSelectedSendPort(nextSendPort);
  }, []);

  const bootstrap = useCallback(async () => {
    setBanner({ kind: "loading", message: t("loading", { ns: "common" }) });
    try {
      const [metaRes, portsRes, sessionRes, settingsRes, parsedRes, channelsRes] = await Promise.all([
        getLocales(),
        getPorts(locale),
        getSession(locale),
        getDetectionSettings(locale),
        getParsed(locale, 400),
        getChannels(locale),
      ]);

      setMeta(metaRes);
      setPorts(portsRes.ports);
      setSession(sessionRes);
      setMessages(parsedRes.items);
      setChannels(channelsRes.channels);

      const { receivePort, sendPort } = resolveInitialPorts(sessionRes, settingsRes, portsRes.ports);
      syncSerialSelection(receivePort, sendPort);
      setBanner({
        kind: sessionBannerKind(sessionRes),
        message: sessionBannerText(sessionRes, sessionRes.active ? t("active", { ns: "common" }) : t("idle", { ns: "common" })),
      });
    } catch (error) {
      setBanner({ kind: "error", message: extractErrorMessage(error) });
    }
  }, [locale, syncSerialSelection, t]);

  useEffect(() => {
    void i18n.changeLanguage(locale);
    persistLocale(locale);
  }, [i18n, locale]);

  useEffect(() => {
    let cancelled = false;
    const load = async () => {
      if (cancelled) {
        return;
      }
      await bootstrap();
    };
    void load();
    return () => {
      cancelled = true;
    };
  }, [bootstrap]);

  useEffect(() => {
    const receivePort = selectedReceivePort.trim();
    const sendPort = selectedSendPort.trim();
    if (!receivePort || !sendPort) {
      return;
    }

    const currentKey = serialKey(receivePort, sendPort);
    if (currentKey === lastAppliedSerialRef.current) {
      return;
    }

    const timer = window.setTimeout(() => {
      void (async () => {
        try {
          setBanner({ kind: "loading", message: t("loading", { ns: "common" }) });
          const response = await updateDetectionSettings(
            {
              portName: receivePort,
              rxPortName: receivePort,
              txPortName: sendPort,
              baudRate: FIXED_SERIAL_PROFILE.baudRate,
              dataBits: FIXED_SERIAL_PROFILE.dataBits,
              stopBits: FIXED_SERIAL_PROFILE.stopBits,
              parity: FIXED_SERIAL_PROFILE.parity,
              readTimeoutMs: FIXED_SERIAL_PROFILE.readTimeoutMs,
              autoConnect: true,
            },
            locale,
          );
          lastAppliedSerialRef.current = currentKey;
          setSession(response);
          setBanner({
            kind: sessionBannerKind(response),
            message: sessionBannerText(response, response.message || t("active", { ns: "common" })),
          });
          await bootstrap();
        } catch (error) {
          setBanner({ kind: "error", message: extractErrorMessage(error) });
        }
      })();
    }, 350);

    return () => window.clearTimeout(timer);
  }, [bootstrap, locale, selectedReceivePort, selectedSendPort, t]);

  useEffect(() => {
    const close = openDetectionStream(locale, {
      onSessionStarted: (event) => {
        if (event.payload) {
          setSession(event.payload);
          const nextReceivePort = event.payload.rxPortName || event.payload.portName || "";
          const nextSendPort = event.payload.txPortName || nextReceivePort || "";
          if (nextReceivePort || nextSendPort) {
            syncSerialSelection(nextReceivePort, nextSendPort || nextReceivePort);
          }
          setBanner({
            kind: sessionBannerKind(event.payload),
            message: sessionBannerText(event.payload, t("active", { ns: "common" })),
          });
        }
      },
      onSessionStopped: (event) => {
        if (event.payload) {
          setSession(event.payload);
          const nextReceivePort = event.payload.rxPortName || event.payload.portName || "";
          const nextSendPort = event.payload.txPortName || nextReceivePort || "";
          if (nextReceivePort || nextSendPort) {
            syncSerialSelection(nextReceivePort, nextSendPort || nextReceivePort);
          }
          setBanner({
            kind: sessionBannerKind(event.payload),
            message: sessionBannerText(event.payload, t("idle", { ns: "common" })),
          });
        }
      },
      onSessionState: (event) => {
        if (event.payload) {
          setSession(event.payload);
          const nextReceivePort = event.payload.rxPortName || event.payload.portName || "";
          const nextSendPort = event.payload.txPortName || nextReceivePort || "";
          if (nextReceivePort || nextSendPort) {
            syncSerialSelection(nextReceivePort, nextSendPort || nextReceivePort);
          }
          setBanner({
            kind: sessionBannerKind(event.payload),
            message: sessionBannerText(event.payload, t("loading", { ns: "common" })),
          });
        }
      },
      onParsed: (event) => {
        if (event.payload) {
          setMessages((items) => dedupeParsed(items, event.payload!, 400));
        }
      },
      onChannelUpdated: (event) => {
        if (event.payload) {
          setChannels((items) => dedupeById(items, event.payload!, 16));
        }
      },
      onError: (error) => {
        setBanner({ kind: "error", message: error.message });
      },
    });

    return close;
  }, [locale, syncSerialSelection, t]);

  const sessionActive = Boolean(session?.active);
  const sessionStateLabel = session
    ? sessionBannerText(session, sessionActive ? t("active", { ns: "common" }) : t("idle", { ns: "common" }))
    : t("idle", { ns: "common" });
  const currentReceivePort = session?.rxPortName || session?.portName || selectedReceivePort;
  const currentSendPort = session?.txPortName || selectedSendPort;
  const appTitle = t("app.title", { ns: "common" });
  const debugNavActive = debugPageItems.some((item) => item.id === page);
  const isMessagePage = MESSAGE_PAGE_ORDER.includes(page as ParsedMessageType);
  const showOverviewPanels = page === "settings";
  const localeOptions = meta?.supportedLocales.length ? meta.supportedLocales : supportedLocales;

  useEffect(() => {
    document.title = appTitle;
  }, [appTitle]);

  const handleToggleChannel = async (channel: GpioChannel) => {
    try {
      const response = await setChannelState(channel.id, { enabled: !channel.enabled }, locale);
      setChannels((items) => dedupeById(items, response.channel, 16));
      setBanner({ kind: "success", message: response.message });
    } catch (error) {
      setBanner({ kind: "error", message: extractErrorMessage(error) });
    }
  };

  return (
    <div className="h-dvh overflow-hidden bg-base-100 text-base-content">
      <div className="grid h-full min-h-0 grid-cols-1 gap-0 overflow-hidden p-0 xl:grid-cols-[292px_minmax(0,1fr)] xl:gap-4 xl:p-4">
        <aside className="min-h-0 overflow-hidden border-b border-base-300 bg-base-200/95 xl:rounded-[28px] xl:border xl:border-base-300/80 xl:bg-base-200/85 xl:shadow-2xl xl:shadow-black/20">
          <div className="flex h-full min-h-0 flex-col gap-4 p-4">
            <div className="flex min-w-0 items-center gap-3">
              <div className="grid h-11 w-11 shrink-0 place-items-center rounded-3xl border border-primary/30 bg-primary/10 text-primary">
                <Shield size={20} />
              </div>
              <div className="min-w-0 self-center">
                <strong className="block truncate text-sm font-semibold">{appTitle}</strong>
              </div>
            </div>

            <nav className="flex min-h-0 gap-2 overflow-x-auto pb-1 xl:flex-col xl:overflow-y-auto xl:overflow-x-hidden" aria-label={appTitle}>
              <details
                className="group min-w-max rounded-3xl border border-base-300/80 bg-base-100/40 p-1 xl:min-w-0"
                open={debugNavActive}
              >
                <summary
                  className={cx(
                    "flex h-10 cursor-pointer list-none items-center gap-2 rounded-3xl px-3 text-sm font-medium transition-colors",
                    debugNavActive
                      ? "bg-primary/10 text-primary shadow-[inset_0_0_0_1px_color-mix(in_oklab,var(--color-primary)_34%,transparent)]"
                      : "text-base-content/72 hover:bg-base-300/70 hover:text-base-content",
                  )}
                >
                  <Wrench size={17} />
                  <span className="min-w-0 flex-1 truncate">{t("debugGroup", { ns: "nav" })}</span>
                  <ChevronDown size={15} className="shrink-0 transition-transform group-open:rotate-180" />
                </summary>

                <div className="mt-1 flex gap-2 xl:flex-col xl:gap-1">
                  {debugPageItems.map((item) => {
                    const Icon = item.icon;
                    const active = page === item.id;
                    return (
                      <a
                        key={item.id}
                        href={`#/${item.id}`}
                        aria-current={active ? "page" : undefined}
                        className={cx(
                          "flex h-10 min-w-max items-center gap-2 rounded-3xl px-3 text-sm transition-colors xl:min-w-0",
                          active
                            ? "bg-primary/10 text-primary shadow-[inset_0_0_0_1px_color-mix(in_oklab,var(--color-primary)_34%,transparent)]"
                            : "text-base-content/64 hover:bg-base-300/70 hover:text-base-content",
                        )}
                        onClick={() => navigate(item.id)}
                      >
                        <Icon size={16} />
                        <span className="truncate">{t(item.labelKey, { ns: "nav" })}</span>
                      </a>
                    );
                  })}
                </div>
              </details>

              <a
                href="#/settings"
                aria-current={page === "settings" ? "page" : undefined}
                className={cx(
                  "flex h-11 min-w-max items-center gap-2 rounded-3xl border border-base-300/80 px-3 text-sm font-medium transition-colors xl:w-full",
                  page === "settings"
                    ? "bg-primary text-primary-content"
                    : "bg-base-100/35 text-base-content/72 hover:bg-base-300/70 hover:text-base-content",
                )}
                onClick={() => navigate("settings")}
              >
                <Settings2 size={17} />
                <span>{t("settings", { ns: "nav" })}</span>
              </a>
            </nav>

            <div className="mt-auto grid gap-3">
              <div className="rounded-3xl border border-base-300 bg-base-100/65 p-3">
                <label className="grid gap-2">
                  <span className="flex items-center gap-2 text-xs font-medium text-base-content/60">
                    <Languages size={15} />
                    <span>{t("language", { ns: "settings" })}</span>
                  </span>
                  <select
                    className="select select-sm select-primary w-full bg-base-100"
                    value={locale}
                    onChange={(event) => setLocale(event.target.value)}
                  >
                    {localeOptions.map((option) => (
                      <option key={option} value={option}>
                        {detectLocaleName(option)}
                      </option>
                    ))}
                  </select>
                </label>
              </div>
            </div>
          </div>
        </aside>

        <div className="flex min-h-0 min-w-0 flex-col overflow-hidden">
          <main
            className={cx(
              "flex min-h-0 min-w-0 flex-1 flex-col gap-4 overflow-x-hidden",
              isMessagePage ? "overflow-hidden" : "overflow-y-auto",
            )}
          >
            {showOverviewPanels ? (
              <>
                <Panel>
                  <PanelBody>
                    <SectionHeader
                      title={t("serialTitle", { ns: "detection" })}
                      description={t("sessionHint", { ns: "detection" })}
                    />

                    <div className="grid gap-4 xl:grid-cols-[minmax(0,1fr)_auto]">
                      <div className="grid gap-4 md:grid-cols-3">
                        <InfoTile label={t("sessionTitle", { ns: "detection" })}>
                          {sessionStateLabel}
                        </InfoTile>
                        <InfoTile label={t("receivePort", { ns: "detection" })} value={currentReceivePort || t("unknown", { ns: "common" })} />
                        <InfoTile label={t("sendPort", { ns: "detection" })} value={currentSendPort || t("unknown", { ns: "common" })} />
                      </div>
                    </div>
                    {banner.kind === "error" ? <BannerAlert banner={banner} /> : null}
                  </PanelBody>
                </Panel>
              </>
            ) : null}

            {isMessagePage ? (
              <MessagePagePanel
                page={page as ParsedMessageType}
                records={messages.filter((item) => item.type === page)}
                locale={locale}
                query={messageSearch}
                onQueryChange={setMessageSearch}
                t={t}
              />
            ) : null}

            {page === "interference" ? (
              <section className="grid gap-4">
                <Panel>
                  <PanelBody>
                    <SectionHeader
                      title={t("title", { ns: "interference" })}
                      description={t("description", { ns: "interference" })}
                    />
                    <div className="grid gap-4 lg:grid-cols-2 2xl:grid-cols-4">
                      {channels.map((channel) => (
                        <ChannelCard key={channel.id} channel={channel} t={t} onToggle={(item) => void handleToggleChannel(item)} />
                      ))}
                    </div>
                  </PanelBody>
                </Panel>
              </section>
            ) : null}

            {page === "settings" ? (
              <section className="grid gap-4">
                <Panel>
                  <PanelBody>
                    <SectionHeader
                      title={t("serialTitle", { ns: "settings" })}
                      description={t("serialDescription", { ns: "settings" })}
                      action={
                        <button className="btn btn-sm btn-outline btn-info" type="button" onClick={() => void bootstrap()}>
                          <RefreshCw size={16} />
                          <span>{t("refresh", { ns: "common" })}</span>
                        </button>
                      }
                    />

                    <div className="grid gap-4 md:grid-cols-2">
                      <PortSelect
                        label={t("receivePort", { ns: "settings" })}
                        placeholder={t("selectReceivePort", { ns: "settings" })}
                        value={selectedReceivePort}
                        ports={ports}
                        activeText={t("active", { ns: "common" })}
                        onChange={setSelectedReceivePort}
                      />
                      <PortSelect
                        label={t("sendPort", { ns: "settings" })}
                        placeholder={t("selectSendPort", { ns: "settings" })}
                        value={selectedSendPort}
                        ports={ports}
                        activeText={t("active", { ns: "common" })}
                        onChange={setSelectedSendPort}
                      />
                    </div>

                    {ports.length === 0 ? <span className="text-sm text-base-content/55">{t("noPorts", { ns: "settings" })}</span> : null}
                  </PanelBody>
                </Panel>
              </section>
            ) : null}
          </main>
        </div>
      </div>
    </div>
  );
}

function detectLocaleName(locale: string) {
  const labels: Record<string, string> = {
    "zh-CN": "中文 / zh-CN",
    "en-US": "English / en-US",
  };
  return labels[locale] ?? locale;
}

export default App;
