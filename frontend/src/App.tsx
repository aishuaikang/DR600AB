import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import type { ReactNode } from "react";
import { useTranslation } from "react-i18next";
import type { TFunction } from "i18next";
import {
  Activity,
  CircleAlert,
  CircleCheck,
  FileText,
  Fingerprint,
  Languages,
  Play,
  Radio,
  RefreshCw,
  Settings2,
  Shield,
  Square,
  Unplug,
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

type MessageColumn = {
  labelKey: string;
  render: (record: ParsedMessage, locale: string) => ReactNode;
};

type MessagePageConfig = {
  icon: LucideIcon;
  navLabelKey: string;
  titleKey: string;
  descriptionKey: string;
  tone: Tone;
  tableWidth: string;
  subject: (record: ParsedMessage) => string;
  columns: MessageColumn[];
};

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
    navLabelKey: "nav.didEncrypted",
    titleKey: "didEncrypted.title",
    descriptionKey: "didEncrypted.description",
    tone: "info",
    tableWidth: "min-w-[1100px]",
    subject: (record) => {
      const data = getRecordData(record);
      return joinParts([getTextValue(data.device), getTextValue(data.encrypted_id)]);
    },
    columns: [
      {
        labelKey: "didEncrypted.device",
        render: (record) => getTextValue(getRecordData(record).device),
      },
      {
        labelKey: "didEncrypted.encryptedId",
        render: (record) => getTextValue(getRecordData(record).encrypted_id),
      },
      {
        labelKey: "didEncrypted.frequency",
        render: (record, locale) => formatNumber(locale, getNumberValue(getRecordData(record).freq)),
      },
      {
        labelKey: "didEncrypted.rssi",
        render: (record, locale) => formatNumber(locale, getNumberValue(getRecordData(record).rssi)),
      },
      {
        labelKey: "didEncrypted.bytes",
        render: (record) => getTextValue(getRecordData(record).bytes),
      },
    ],
  },
  rid: {
    icon: Fingerprint,
    navLabelKey: "nav.rid",
    titleKey: "rid.title",
    descriptionKey: "rid.description",
    tone: "success",
    tableWidth: "min-w-[1320px]",
    subject: (record) => {
      const data = getRecordData(record);
      return joinParts([getTextValue(data.ssid), getTextValue(data.serial)]);
    },
    columns: [
      {
        labelKey: "rid.ssid",
        render: (record) => getTextValue(getRecordData(record).ssid),
      },
      {
        labelKey: "rid.serial",
        render: (record) => getTextValue(getRecordData(record).serial),
      },
      {
        labelKey: "rid.model",
        render: (record) => getTextValue(getRecordData(record).model),
      },
      {
        labelKey: "rid.uaType",
        render: (record) => getTextValue(getRecordData(record).UA_type),
      },
      {
        labelKey: "rid.droneGps",
        render: (record) => formatGps(getRecordData(record).drone_GPS),
      },
      {
        labelKey: "rid.pilotGps",
        render: (record) => formatGps(getRecordData(record).pilot_GPS),
      },
      {
        labelKey: "rid.frequency",
        render: (record, locale) => formatNumber(locale, getNumberValue(getRecordData(record).freq)),
      },
      {
        labelKey: "rid.rssi",
        render: (record, locale) => formatNumber(locale, getNumberValue(getRecordData(record).rssi)),
      },
    ],
  },
  did_plain: {
    icon: FileText,
    navLabelKey: "nav.didPlain",
    titleKey: "didPlain.title",
    descriptionKey: "didPlain.description",
    tone: "warning",
    tableWidth: "min-w-[1180px]",
    subject: (record) => {
      const data = getRecordData(record);
      return joinParts([getTextValue(data.device), getTextValue(data.uuid)]);
    },
    columns: [
      {
        labelKey: "didPlain.device",
        render: (record) => getTextValue(getRecordData(record).device),
      },
      {
        labelKey: "didPlain.serial",
        render: (record) => getTextValue(getRecordData(record).serial),
      },
      {
        labelKey: "didPlain.model",
        render: (record) => getTextValue(getRecordData(record).model),
      },
      {
        labelKey: "didPlain.uuid",
        render: (record) => getTextValue(getRecordData(record).uuid),
      },
      {
        labelKey: "didPlain.distance",
        render: (record) => getTextValue(getRecordData(record).distance),
      },
      {
        labelKey: "didPlain.frequency",
        render: (record, locale) => formatNumber(locale, getNumberValue(getRecordData(record).freq)),
      },
      {
        labelKey: "didPlain.rssi",
        render: (record, locale) => formatNumber(locale, getNumberValue(getRecordData(record).rssi)),
      },
    ],
  },
  detect: {
    icon: Radio,
    navLabelKey: "nav.detect",
    titleKey: "detect.title",
    descriptionKey: "detect.description",
    tone: "info",
    tableWidth: "min-w-[920px]",
    subject: (record) => {
      const data = getRecordData(record);
      return joinParts([getTextValue(data.device), getTextValue(data.model)]);
    },
    columns: [
      {
        labelKey: "detect.device",
        render: (record) => getTextValue(getRecordData(record).device),
      },
      {
        labelKey: "detect.model",
        render: (record) => getTextValue(getRecordData(record).model),
      },
      {
        labelKey: "detect.frequency",
        render: (record, locale) => formatNumber(locale, getNumberValue(getRecordData(record).freq)),
      },
      {
        labelKey: "detect.rssi",
        render: (record, locale) => formatNumber(locale, getNumberValue(getRecordData(record).rssi)),
      },
    ],
  },
  heartbeat: {
    icon: Activity,
    navLabelKey: "nav.heartbeat",
    titleKey: "heartbeat.title",
    descriptionKey: "heartbeat.description",
    tone: "error",
    tableWidth: "min-w-[840px]",
    subject: (record) => {
      const data = getRecordData(record);
      return joinParts([getTextValue(data.device), getTextValue(data.seq)]);
    },
    columns: [
      {
        labelKey: "heartbeat.device",
        render: (record) => getTextValue(getRecordData(record).device),
      },
      {
        labelKey: "heartbeat.seq",
        render: (record) => getTextValue(getRecordData(record).seq),
      },
    ],
  },
};

const pageItems: Array<{ id: Page; icon: LucideIcon; labelKey: string }> = [
  { id: "did_encrypted", icon: MESSAGE_PAGE_CONFIG.did_encrypted.icon, labelKey: MESSAGE_PAGE_CONFIG.did_encrypted.navLabelKey },
  { id: "rid", icon: MESSAGE_PAGE_CONFIG.rid.icon, labelKey: MESSAGE_PAGE_CONFIG.rid.navLabelKey },
  { id: "did_plain", icon: MESSAGE_PAGE_CONFIG.did_plain.icon, labelKey: MESSAGE_PAGE_CONFIG.did_plain.navLabelKey },
  { id: "detect", icon: MESSAGE_PAGE_CONFIG.detect.icon, labelKey: MESSAGE_PAGE_CONFIG.detect.navLabelKey },
  { id: "heartbeat", icon: MESSAGE_PAGE_CONFIG.heartbeat.icon, labelKey: MESSAGE_PAGE_CONFIG.heartbeat.navLabelKey },
  { id: "interference", icon: Zap, labelKey: "nav.interference" },
  { id: "settings", icon: Settings2, labelKey: "nav.settings" },
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

function countByType(items: ParsedMessage[]) {
  const counts = new Map<ParsedMessageType, number>();
  for (const type of MESSAGE_PAGE_ORDER) {
    counts.set(type, 0);
  }
  for (const item of items) {
    if (MESSAGE_PAGE_ORDER.includes(item.type as ParsedMessageType)) {
      const key = item.type as ParsedMessageType;
      counts.set(key, (counts.get(key) ?? 0) + 1);
    }
  }
  return MESSAGE_PAGE_ORDER.map((type) => [type, counts.get(type) ?? 0] as const);
}

function formatPortPair(receivePort?: string, sendPort?: string) {
  const rx = receivePort?.trim() ?? "";
  const tx = sendPort?.trim() ?? "";

  if (!rx && !tx) {
    return "";
  }
  if (!tx || tx === rx) {
    return rx;
  }
  if (!rx) {
    return tx;
  }
  return `${rx} / ${tx}`;
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

function joinParts(parts: string[]): string {
  return parts.filter((part) => part && part !== "-").join(" / ") || "-";
}

function buildSearchText(record: ParsedMessage): string {
  return `${record.type} ${record.raw} ${JSON.stringify(record.data ?? {})}`.toLowerCase();
}

function normalizeSummary(value: string, maxLength = 64): string {
  if (value.length <= maxLength) {
    return value;
  }
  return `${value.slice(0, maxLength - 1)}…`;
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
    <section className={cx("rounded-lg border border-base-300 bg-base-200/80 shadow-sm shadow-black/20", className)}>
      {children}
    </section>
  );
}

function PanelBody({ children, className }: { children: ReactNode; className?: string }) {
  return <div className={cx("flex flex-col gap-5 p-5", className)}>{children}</div>;
}

function MetricCard({
  icon,
  label,
  value,
  hint,
  tone = "info",
}: {
  icon: ReactNode;
  label: string;
  value: string;
  hint: string;
  tone?: Tone;
}) {
  const toneClass: Record<Tone, string> = {
    neutral: "text-base-content bg-base-300/70",
    success: "text-success bg-success/10",
    warning: "text-warning bg-warning/10",
    error: "text-error bg-error/10",
    info: "text-info bg-info/10",
  };

  return (
    <article className="rounded-lg border border-base-300 bg-base-200/75 p-4 shadow-sm shadow-black/20">
      <div className="flex min-w-0 items-start gap-3">
        <div className={cx("grid h-10 w-10 shrink-0 place-items-center rounded-md", toneClass[tone])}>{icon}</div>
        <div className="min-w-0">
          <p className="truncate text-xs font-medium text-base-content/60">{label}</p>
          <strong className="mt-1 block break-words text-xl font-semibold leading-tight text-base-content">{value}</strong>
          <span className="mt-1 block truncate text-xs text-base-content/50">{hint}</span>
        </div>
      </div>
    </article>
  );
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
    <div className="min-w-0 rounded-md border border-base-300 bg-base-100/70 px-4 py-3">
      <span className="block text-xs font-medium text-base-content/55">{label}</span>
      <div className="mt-2 min-w-0 break-words text-sm font-semibold text-base-content">{children ?? value}</div>
    </div>
  );
}

function DataTable({
  minWidth,
  children,
}: {
  minWidth: string;
  children: ReactNode;
}) {
  return (
    <div className="overflow-x-auto rounded-lg border border-base-300 bg-base-100/70">
      <table className={cx("table table-zebra table-sm", minWidth)}>{children}</table>
    </div>
  );
}

function EmptyRow({ colSpan, message }: { colSpan: number; message: string }) {
  return (
    <tr>
      <td colSpan={colSpan} className="py-8 text-center text-sm text-base-content/55">
        {message}
      </td>
    </tr>
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
  if (!banner.message) {
    return null;
  }

  const classes: Record<Banner["kind"], string> = {
    idle: "alert-info",
    loading: "alert-info",
    success: "alert-success",
    error: "alert-error",
  };

  const Icon = banner.kind === "error" ? CircleAlert : CircleCheck;

  return (
    <div
      className={cx("alert alert-soft py-3 text-sm", classes[banner.kind])}
      role={banner.kind === "error" ? "alert" : "status"}
      aria-live={banner.kind === "error" ? "assertive" : "polite"}
    >
      <Icon size={16} />
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
    <article className="flex min-w-0 flex-col gap-4 rounded-lg border border-base-300 bg-base-100/70 p-4">
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

      {channel.lastError ? <p className="rounded-md bg-error/10 px-3 py-2 text-sm text-error">{channel.lastError}</p> : null}

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

  const latest = filteredRecords[0];
  const latestSubject = latest ? normalizeSummary(config.subject(latest)) : "-";
  const latestTime = latest ? formatTime(locale, latest.time) : "-";

  return (
    <section className="grid gap-5">
      <Panel>
        <PanelBody>
          <SectionHeader
            title={t(config.titleKey, { ns: "messages" })}
            description={t(config.descriptionKey, { ns: "messages" })}
            action={
              <label className="grid min-w-[240px] gap-2">
                <span className="text-xs font-medium text-base-content/60">{t("search", { ns: "common" })}</span>
                <input
                  className="input input-sm input-bordered w-full bg-base-100"
                  value={query}
                  onChange={(event) => onQueryChange(event.target.value)}
                  placeholder={t("search", { ns: "common" })}
                />
              </label>
            }
          />

          <div className="grid gap-3 sm:grid-cols-3">
            <MetricCard
              icon={<config.icon size={18} />}
              label={t("records", { ns: "common" })}
              value={String(filteredRecords.length)}
              hint={t(config.titleKey, { ns: "messages" })}
              tone={config.tone}
            />
            <MetricCard
              icon={<RefreshCw size={18} />}
              label={t("latest", { ns: "common" })}
              value={latestTime}
              hint={latestSubject}
              tone="neutral"
            />
            <MetricCard
              icon={<Activity size={18} />}
              label={t("summary", { ns: "common" })}
              value={latestSubject}
              hint={latest ? formatTime(locale, latest.time) : t("empty", { ns: "common" })}
              tone="info"
            />
          </div>

          <DataTable minWidth={config.tableWidth}>
            <thead>
              <tr>
                <th>{t("time", { ns: "common" })}</th>
                {config.columns.map((column) => (
                  <th key={column.labelKey}>{t(column.labelKey, { ns: "messages" })}</th>
                ))}
                <th>{t("details", { ns: "common" })}</th>
              </tr>
            </thead>
            <tbody>
              {filteredRecords.map((record) => (
                <tr key={`${record.type}-${record.time}-${record.raw}`} className="row-hover">
                  <td>{formatTime(locale, record.time)}</td>
                  {config.columns.map((column) => (
                    <td key={column.labelKey} className={cx("align-top", column.labelKey.includes("frequency") || column.labelKey.includes("rssi") ? "tabular-nums" : "")}>
                      {column.render(record, locale)}
                    </td>
                  ))}
                  <td className="align-top">
                    <details className="max-w-[420px]">
                      <summary className="cursor-pointer [overflow-wrap:anywhere]">{normalizeSummary(record.raw, 140)}</summary>
                      <pre className="mt-3 overflow-auto rounded-md border border-base-300 bg-base-200/80 p-3 text-xs leading-5 text-base-content/80">
                        {JSON.stringify(record.data, null, 2)}
                      </pre>
                    </details>
                  </td>
                </tr>
              ))}
              {filteredRecords.length === 0 ? <EmptyRow colSpan={config.columns.length + 2} message={t("empty", { ns: "common" })} /> : null}
            </tbody>
          </DataTable>
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

  const messageCounts = useMemo(() => countByType(messages), [messages]);
  const sessionActive = Boolean(session?.active);
  const sessionStateLabel = session
    ? sessionBannerText(session, sessionActive ? t("active", { ns: "common" }) : t("idle", { ns: "common" }))
    : t("idle", { ns: "common" });
  const sessionStateTone: Tone = session?.state === "connected"
    ? "success"
    : session?.state === "connecting" || session?.state === "reconnecting"
      ? "warning"
      : sessionActive
        ? "success"
        : "neutral";
  const currentReceivePort = session?.rxPortName || session?.portName || selectedReceivePort;
  const currentSendPort = session?.txPortName || selectedSendPort;
  const currentPortPair = formatPortPair(currentReceivePort, currentSendPort);
  const appTitle = t("app.title", { ns: "common" });
  const appSubtitle = t("app.subtitle", { ns: "common" });

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
    <div className="min-h-screen bg-base-100 text-base-content">
      <div className="grid min-h-screen grid-cols-1 xl:grid-cols-[280px_minmax(0,1fr)]">
        <aside className="border-b border-base-300 bg-base-200/95 xl:border-b-0 xl:border-r">
          <div className="flex h-full flex-col gap-5 p-4 sm:p-5">
            <div className="flex min-w-0 items-center gap-3">
              <div className="grid h-11 w-11 shrink-0 place-items-center rounded-lg border border-primary/30 bg-primary/10 text-primary">
                <Shield size={20} />
              </div>
              <div className="min-w-0">
                <strong className="block truncate text-sm font-semibold">{appTitle}</strong>
                <span className="mt-1 block truncate text-xs text-base-content/55">{appSubtitle}</span>
              </div>
            </div>

            <nav className="flex gap-2 overflow-x-auto pb-1 xl:flex-col xl:overflow-visible" aria-label={appTitle}>
              {pageItems.map((item) => {
                const Icon = item.icon;
                const active = page === item.id;
                return (
                  <a
                    key={item.id}
                    href={`#/${item.id}`}
                    aria-current={active ? "page" : undefined}
                    className={cx(
                      "btn btn-sm min-w-max justify-start border-base-300 xl:w-full",
                      active ? "btn-primary" : "btn-ghost text-base-content/70 hover:bg-base-300/70",
                    )}
                    onClick={() => navigate(item.id)}
                  >
                    <Icon size={17} />
                    <span>{t(item.labelKey)}</span>
                  </a>
                );
              })}
            </nav>

            <div className="mt-auto grid gap-3">
              <div className="rounded-lg border border-base-300 bg-base-100/65 p-3">
                <div className="flex flex-wrap items-center gap-2">
                  <Badge tone={sessionStateTone}>
                    {sessionStateLabel}
                  </Badge>
                  <span className="min-w-0 text-xs text-base-content/60 [overflow-wrap:anywhere]">
                    {currentPortPair || t("unknown", { ns: "common" })}
                  </span>
                </div>
              </div>
              <BannerAlert banner={banner} />
            </div>
          </div>
        </aside>

        <div className="flex min-w-0 flex-col">
          <header className="sticky top-0 z-20 border-b border-base-300 bg-base-100/90 backdrop-blur">
            <div className="flex flex-col gap-4 px-4 py-4 sm:px-6 lg:flex-row lg:items-center lg:justify-between">
              <div className="min-w-0">
                <h1 className="truncate text-xl font-semibold leading-7">{appTitle}</h1>
                <p className="mt-1 truncate text-sm text-base-content/60">{appSubtitle}</p>
              </div>
              <div className="flex flex-wrap items-center gap-2">
                <div className="flex items-center gap-2 rounded-lg border border-base-300 bg-base-200/80 px-3 py-2 text-sm">
                  <span
                    className={cx(
                      "status",
                      sessionStateTone === "success"
                        ? "status-success"
                        : sessionStateTone === "warning"
                          ? "status-warning"
                          : "status-neutral",
                    )}
                  />
                  <span className="text-base-content/70">{sessionStateLabel}</span>
                </div>
                <button className="btn btn-sm btn-outline btn-info" type="button" onClick={() => void bootstrap()}>
                  <RefreshCw size={16} />
                  <span>{t("refresh", { ns: "common" })}</span>
                </button>
              </div>
            </div>
          </header>

          <main className="flex min-w-0 flex-1 flex-col gap-5 px-4 py-5 sm:px-6">
            <Panel>
              <PanelBody>
                <SectionHeader
                  title={t("serialTitle", { ns: "detection" })}
                  description={t("sessionHint", { ns: "detection" })}
                  action={
                    <button className="btn btn-sm btn-outline btn-info" type="button" onClick={() => navigate("settings")}>
                      <Settings2 size={16} />
                      <span>{t("title", { ns: "settings" })}</span>
                    </button>
                  }
                />

                <div className="grid gap-3 xl:grid-cols-[minmax(0,1fr)_auto]">
                  <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-4">
                    <InfoTile label={t("sessionTitle", { ns: "detection" })}>
                      {sessionStateLabel}
                    </InfoTile>
                    <InfoTile label={t("receivePort", { ns: "detection" })} value={currentReceivePort || t("unknown", { ns: "common" })} />
                    <InfoTile label={t("sendPort", { ns: "detection" })} value={currentSendPort || t("unknown", { ns: "common" })} />
                    <InfoTile label={t("advanced", { ns: "detection" })} value={t("fixedSerialHint", { ns: "detection" })} />
                  </div>
                </div>
              </PanelBody>
            </Panel>

            <section className="grid gap-3 sm:grid-cols-2 2xl:grid-cols-5">
              {messageCounts.map(([type, count]) => {
                const config = MESSAGE_PAGE_CONFIG[type];
                return (
                  <MetricCard
                    key={type}
                    icon={<config.icon size={18} />}
                    label={t(config.navLabelKey)}
                    value={String(count)}
                    hint={t(config.titleKey, { ns: "messages" })}
                    tone={config.tone}
                  />
                );
              })}
            </section>

            {MESSAGE_PAGE_ORDER.includes(page as ParsedMessageType) ? (
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
              <section className="grid gap-5">
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
              <section className="grid gap-5 lg:grid-cols-2">
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

                    <div className="alert alert-soft alert-info py-3 text-sm">
                      <CircleCheck size={16} />
                      <span>{t("serialHint", { ns: "settings" })}</span>
                    </div>

                    <div className="flex flex-wrap gap-2">
                      {ports.length > 0 ? (
                        ports.map((port) => (
                          <Badge key={port.name} tone={port.active ? "success" : "neutral"} outline={!port.active}>
                            {port.name}
                          </Badge>
                        ))
                      ) : (
                        <span className="text-sm text-base-content/55">{t("noPorts", { ns: "settings" })}</span>
                      )}
                    </div>
                  </PanelBody>
                </Panel>

                <Panel>
                  <PanelBody>
                    <SectionHeader
                      title={t("languageTitle", { ns: "settings" })}
                      description={t("languageDescription", { ns: "settings" })}
                    />
                    <SelectField label={t("language", { ns: "settings" })} value={locale} onChange={setLocale}>
                      {(meta?.supportedLocales.length ? meta.supportedLocales : supportedLocales).map((option) => (
                        <option key={option} value={option}>
                          {detectLocaleName(option)}
                        </option>
                      ))}
                    </SelectField>

                    <div className="grid gap-3">
                      <InfoTile label={t("currentLocale", { ns: "settings" })} value={locale} />
                      <InfoTile label={t("defaultLocale", { ns: "settings" })} value={meta?.defaultLocale || "zh-CN"} />
                      <InfoTile label={t("apiHint", { ns: "settings" })} value="/api/v1" />
                    </div>
                  </PanelBody>
                </Panel>

                <Panel className="lg:col-span-2">
                  <PanelBody>
                    <SectionHeader
                      title={t("backendLocales", { ns: "settings" })}
                      description={t("namespaces", { ns: "settings" })}
                    />
                    <div className="grid gap-4 md:grid-cols-2">
                      <div className="grid gap-2">
                        <div className="flex items-center gap-2 text-sm font-semibold">
                          <Languages size={16} />
                          <span>{t("supportedLocales", { ns: "settings" })}</span>
                        </div>
                        <div className="flex flex-wrap gap-2">
                          {(meta?.supportedLocales.length ? meta.supportedLocales : supportedLocales).map((item) => (
                            <Badge key={item} tone={item === locale ? "success" : "neutral"} outline={item !== locale}>
                              {detectLocaleName(item)}
                            </Badge>
                          ))}
                        </div>
                      </div>
                      <div className="grid gap-2">
                        <div className="flex items-center gap-2 text-sm font-semibold">
                          <Unplug size={16} />
                          <span>{t("namespaces", { ns: "settings" })}</span>
                        </div>
                        <div className="flex flex-wrap gap-2">
                          {(meta?.namespaces.length ? meta.namespaces : ["common", "nav", "messages", "interference", "settings"]).map((item) => (
                            <Badge key={item} tone="info" outline>
                              {item}
                            </Badge>
                          ))}
                        </div>
                      </div>
                    </div>
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
