import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import type { TFunction } from "i18next";
import type { LucideIcon } from "lucide-react";
import {
  Activity,
  BadgeCheck,
  BadgeX,
  Cable,
  CircleAlert,
  Clock3,
  Cpu,
  Database,
  Gauge,
  Globe2,
  HardDrive,
  IdCard,
  MapPin,
  Power,
  Radio,
  RefreshCw,
  Router,
  Server,
  Signal,
  Smartphone,
  Wifi,
} from "lucide-react";

import { getNetworkInterfaces } from "../api";
import { Badge } from "../components/Badge";
import { BannerAlert } from "../components/BannerAlert";
import { LoadingSpinner } from "../components/LoadingState";
import { Panel, PanelBody } from "../components/Panel";
import { SectionHeader } from "../components/SectionHeader";
import type { Banner } from "../app/types";
import type { CellularModem, NetworkInterface } from "../types";
import { formatNumber, formatTime } from "../utils/format";
import { extractErrorMessage } from "../utils/session";

function isCellularInterface(item: NetworkInterface) {
  return item.kind === "cellular" || item.type.toLowerCase().includes("gsm") || item.type.toLowerCase().includes("wwan");
}

function valueOrDash(value?: string | number | null) {
  if (typeof value === "number") {
    return Number.isFinite(value) ? String(value) : "-";
  }
  const normalized = value?.trim();
  return !normalized || normalized === "--" ? "-" : normalized;
}

function signalTone(value: number | undefined) {
  if (typeof value !== "number") return "bg-base-300";
  if (value >= 70) return "bg-success";
  if (value >= 35) return "bg-warning";
  return "bg-error";
}

function signalTextTone(value: number | undefined) {
  if (typeof value !== "number") return "text-base-content/35";
  if (value >= 70) return "text-success";
  if (value >= 35) return "text-warning";
  return "text-error";
}

function signalWidth(value: number | undefined) {
  return typeof value === "number" && Number.isFinite(value) ? Math.min(100, Math.max(0, value)) : 0;
}

function formatTechnologies(value: string | undefined) {
  if (!value?.trim()) return "-";
  return value
    .split(/[,/]+/)
    .map((item) => item.trim())
    .filter(Boolean)
    .map((item) => item.replace(/\b(lte|5g|4g|umts|gsm|edge|hspa)\b/gi, (match) => match.toUpperCase()))
    .join(" / ");
}

function formatAddresses(item: NetworkInterface) {
  return item.ipv4.length ? item.ipv4.map((address) => `${address.address}/${address.prefix}`).join(", ") : "-";
}

function formatPorts(ports?: string[]) {
  return ports?.length ? ports.join(", ") : "-";
}

function stateLabel(value: string | undefined, t: TFunction) {
  const normalized = value?.trim().toLowerCase();
  const labels: Record<string, string> = {
    connected: "cellularStatusConnected",
    disconnected: "cellularStatusDisconnected",
    unavailable: "cellularStatusUnavailable",
    searching: "cellularStatusSearching",
    registered: "cellularStatusRegistered",
    home: "cellularStatusRegistered",
    roaming: "cellularStatusRoaming",
    denied: "cellularStatusDenied",
    attached: "cellularStatusAttached",
    detached: "cellularStatusDetached",
    enabled: "cellularStatusEnabled",
    disabled: "cellularStatusDisabled",
    failed: "cellularStatusUnavailable",
  };
  return normalized && labels[normalized]
    ? t(labels[normalized], { ns: "settings" })
    : value?.trim() || t("cellularStatusUnknown", { ns: "settings" });
}

function connectionLabel(item: NetworkInterface, modem: CellularModem | undefined, t: TFunction) {
  if (!modem?.simPath && modem) return t("cellularStatusSimMissing", { ns: "settings" });
  if (item.state === "connected" || modem?.state === "connected" || modem?.packetServiceState === "attached") {
    return t("cellularStatusConnected", { ns: "settings" });
  }
  if (modem?.failedReason) return t("cellularStatusUnavailable", { ns: "settings" });
  if (modem?.registrationState === "searching") return t("cellularStatusSearching", { ns: "settings" });
  return stateLabel(item.state || modem?.state, t);
}

function connectionTone(item: NetworkInterface, modem: CellularModem | undefined) {
  if (!modem?.simPath && modem) return "text-warning";
  if (item.state === "connected" || modem?.state === "connected" || modem?.packetServiceState === "attached") return "text-success";
  if (modem?.failedReason) return "text-error";
  return "text-warning";
}

function StatusMetric({ icon: Icon, label, value, tone }: { icon: LucideIcon; label: string; value: string; tone: string }) {
  return (
    <div className="group flex min-h-[6.25rem] items-center gap-3 rounded-2xl border border-base-300 bg-base-100/75 p-3 shadow-sm transition-colors hover:border-info/40">
      <div className={`grid size-11 shrink-0 place-items-center rounded-2xl bg-base-200 ${tone}`} title={label}>
        <Icon size={23} strokeWidth={1.8} aria-hidden="true" />
      </div>
      <div className="min-w-0">
        <div className="truncate text-xs text-base-content/55">{label}</div>
        <div className="mt-1 truncate text-lg font-semibold tabular-nums" title={value}>{value}</div>
      </div>
    </div>
  );
}

function DetailRow({ icon: Icon, label, value, tone = "text-info" }: { icon: LucideIcon; label: string; value: string; tone?: string }) {
  return (
    <div className="grid grid-cols-[1.5rem_minmax(0,1fr)_minmax(0,1.4fr)] items-center gap-2 rounded-xl px-2 py-1.5 hover:bg-base-200/60">
      <Icon size={16} className={tone} aria-hidden="true" />
      <dt className="truncate text-base-content/55">{label}</dt>
      <dd className="truncate text-right font-mono tabular-nums" title={value}>{value}</dd>
    </div>
  );
}

function SignalMeter({ value, locale, t }: { value: number | undefined; locale: string; t: TFunction }) {
  const label = typeof value === "number" ? `${formatNumber(locale, value, 0)}%` : "-";
  return (
    <div className="rounded-2xl border border-base-300 bg-base-100/70 p-3">
      <div className="flex items-center justify-between gap-2 text-sm">
        <span className="flex items-center gap-2 font-semibold"><Signal size={17} className={signalTextTone(value)} aria-hidden="true" />{t("cellularSignal", { ns: "settings" })}</span>
        <span className={`font-mono font-semibold tabular-nums ${signalTextTone(value)}`}>{label}</span>
      </div>
      <div className="mt-3 h-2 overflow-hidden rounded-full bg-base-300/80">
        <div className={`h-full rounded-full transition-[width] ${signalTone(value)}`} style={{ width: `${signalWidth(value)}%` }} />
      </div>
      <div className="mt-1 flex justify-between text-[10px] text-base-content/45"><span>0%</span><span>100%</span></div>
    </div>
  );
}

export function CellularStatusPage({ locale, developerToken, t }: { locale: string; developerToken: string; t: TFunction }) {
  const [interfaces, setInterfaces] = useState<NetworkInterface[]>([]);
  const [backendState, setBackendState] = useState({ available: true, readOnly: false, message: "" });
  const [banner, setBanner] = useState<Banner>({ kind: "idle", message: "" });
  const [loading, setLoading] = useState(false);
  const [lastUpdatedAt, setLastUpdatedAt] = useState("");
  const loadInFlightRef = useRef(false);

  const load = useCallback(async (silent = false) => {
    if (loadInFlightRef.current) return;
    loadInFlightRef.current = true;
    if (!silent) {
      setLoading(true);
      setBanner({ kind: "loading", message: t("cellularStatusLoading", { ns: "settings" }) });
    }
    try {
      const response = await getNetworkInterfaces(locale, developerToken);
      setInterfaces(response.interfaces);
      setBackendState({ available: response.available, readOnly: response.readOnly, message: response.message ?? "" });
      setLastUpdatedAt(new Date().toISOString());
      if (!silent) {
        setBanner({ kind: "idle", message: "" });
      } else {
        setBanner((current) => current.kind === "error" ? { kind: "idle", message: "" } : current);
      }
    } catch (error) {
      setBanner({ kind: "error", message: extractErrorMessage(error, t("unexpectedError", { ns: "common" })) });
    } finally {
      if (!silent) setLoading(false);
      loadInFlightRef.current = false;
    }
  }, [developerToken, locale, t]);

  useEffect(() => {
    void load();
    const timer = window.setInterval(() => void load(true), 5_000);
    return () => window.clearInterval(timer);
  }, [load]);

  const cellularInterfaces = useMemo(
    () => interfaces
      .filter(isCellularInterface)
      .sort((a, b) => {
        const rank = (item: NetworkInterface) => Number(item.defaultRoute) * 2 + Number(item.state === "connected");
        return rank(b) - rank(a);
      }),
    [interfaces],
  );
  const selected = cellularInterfaces[0];
  const modem = selected?.modem;
  const signalQuality = modem?.signalQuality;
  const operator = modem?.operatorName || modem?.operatorCode;
  const state = selected ? connectionLabel(selected, modem, t) : t("cellularStatusNotDetected", { ns: "settings" });
  const stateTone = selected ? connectionTone(selected, modem) : "text-base-content/35";
  const signalToneClass = signalTextTone(signalQuality);

  return (
    <section className="flex min-h-0 min-w-0 flex-1">
      <Panel className="flex min-h-0 min-w-0 flex-1 flex-col overflow-hidden">
        <PanelBody className="min-h-0 min-w-0 flex-1 overflow-auto">
          <SectionHeader
            title={t("cellularStatusTitle", { ns: "settings" })}
            description={t("cellularStatusDescription", { ns: "settings" })}
            action={
              <button className="btn btn-sm btn-outline btn-info" type="button" onClick={() => void load()} disabled={loading}>
                {loading ? <LoadingSpinner size={16} /> : <RefreshCw size={16} />}
                <span>{t("refresh", { ns: "common" })}</span>
              </button>
            }
          />

          <BannerAlert banner={banner} />
          {!backendState.available || backendState.readOnly ? (
            <div className="alert alert-soft alert-warning py-3 text-sm" role="status">
              <CircleAlert size={16} />
              <span className="min-w-0 [overflow-wrap:anywhere]">{backendState.message || t("networkUnsupportedPlatform", { ns: "settings" })}</span>
            </div>
          ) : null}

          <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-5">
            <StatusMetric icon={selected && stateTone === "text-success" ? BadgeCheck : selected ? CircleAlert : BadgeX} label={t("cellularState", { ns: "settings" })} value={state} tone={stateTone} />
            <StatusMetric icon={Signal} label={t("cellularSignal", { ns: "settings" })} value={typeof signalQuality === "number" ? `${formatNumber(locale, signalQuality, 0)}%` : "-"} tone={signalToneClass} />
            <StatusMetric icon={Globe2} label={t("cellularStatusOperator", { ns: "settings" })} value={valueOrDash(operator)} tone="text-info" />
            <StatusMetric icon={Radio} label={t("cellularStatusTechnology", { ns: "settings" })} value={formatTechnologies(modem?.accessTechnologies)} tone="text-info" />
            <StatusMetric icon={Cable} label={t("cellularDataInterface", { ns: "settings" })} value={valueOrDash(modem?.dataInterface || selected?.name)} tone="text-info" />
          </div>

          {!selected ? (
            <div className="admin-empty-state admin-empty-state--table">{t("cellularStatusNoDevice", { ns: "settings" })}</div>
          ) : (
            <>
              <div className="grid gap-3 xl:grid-cols-[minmax(0,1.05fr)_minmax(20rem,0.95fr)]">
                <div className="rounded-2xl border border-base-300 bg-base-100/60 p-3">
                  <div className="mb-3 flex items-center justify-between gap-2">
                    <h3 className="flex items-center gap-2 text-sm font-semibold"><Activity size={17} className="text-info" aria-hidden="true" />{t("cellularStatusConnection", { ns: "settings" })}</h3>
                    <Badge tone={stateTone === "text-success" ? "success" : stateTone === "text-error" ? "error" : "warning"}><span className="flex items-center gap-1"><Wifi size={12} aria-hidden="true" />{state}</span></Badge>
                  </div>
                  <SignalMeter value={signalQuality} locale={locale} t={t} />
                  <dl className="mt-2 space-y-0.5 text-sm">
                    <DetailRow icon={Globe2} label={t("cellularStatusOperator", { ns: "settings" })} value={valueOrDash(operator)} />
                    <DetailRow icon={Radio} label={t("cellularStatusRegistration", { ns: "settings" })} value={stateLabel(modem?.registrationState, t)} tone={stateTone} />
                    <DetailRow icon={Activity} label={t("cellularStatusPacketService", { ns: "settings" })} value={stateLabel(modem?.packetServiceState, t)} tone={stateTone} />
                    <DetailRow icon={Router} label={t("cellularStatusTechnology", { ns: "settings" })} value={formatTechnologies(modem?.accessTechnologies)} />
                    <DetailRow icon={Power} label={t("cellularStatusPower", { ns: "settings" })} value={stateLabel(modem?.powerState, t)} />
                    <DetailRow icon={Clock3} label={t("cellularStatusLastUpdated", { ns: "settings" })} value={lastUpdatedAt ? formatTime(locale, lastUpdatedAt) : "-"} />
                  </dl>
                </div>

                <div className="rounded-2xl border border-base-300 bg-base-100/60 p-3">
                  <h3 className="mb-2 flex items-center gap-2 text-sm font-semibold"><Server size={17} className="text-info" aria-hidden="true" />{t("cellularStatusNetwork", { ns: "settings" })}</h3>
                  <dl className="space-y-0.5 text-sm">
                    <DetailRow icon={Cable} label={t("cellularStatusInterface", { ns: "settings" })} value={valueOrDash(selected.name)} />
                    <DetailRow icon={Database} label={t("networkConnection", { ns: "settings" })} value={valueOrDash(selected.connectionName)} />
                    <DetailRow icon={Globe2} label={t("networkIPv4", { ns: "settings" })} value={formatAddresses(selected)} />
                    <DetailRow icon={Router} label={t("networkGateway", { ns: "settings" })} value={valueOrDash(selected.gateway4)} />
                    <DetailRow icon={HardDrive} label={t("networkDNS", { ns: "settings" })} value={selected.dns4.length ? selected.dns4.join(", ") : "-"} />
                    <DetailRow icon={MapPin} label={t("networkDefaultRoute", { ns: "settings" })} value={selected.defaultRoute ? t("networkDefaultRouteActive", { ns: "settings" }) : t("networkDefaultRouteInactive", { ns: "settings" })} tone={selected.defaultRoute ? "text-success" : "text-base-content/35"} />
                  </dl>
                </div>
              </div>

              <div className="grid gap-3 xl:grid-cols-2">
                <div className="rounded-2xl border border-base-300 bg-base-100/60 p-3">
                  <h3 className="mb-2 flex items-center gap-2 text-sm font-semibold"><Smartphone size={17} className="text-info" aria-hidden="true" />{t("cellularStatusModem", { ns: "settings" })}</h3>
                  <dl className="space-y-0.5 text-sm">
                    <DetailRow icon={Cpu} label={t("cellularModel", { ns: "settings" })} value={valueOrDash(modem?.model)} />
                    <DetailRow icon={HardDrive} label={t("cellularStatusManufacturer", { ns: "settings" })} value={valueOrDash(modem?.manufacturer)} />
                    <DetailRow icon={IdCard} label={t("cellularEquipmentID", { ns: "settings" })} value={valueOrDash(modem?.equipmentId)} />
                    <DetailRow icon={Radio} label={t("cellularStatusRevision", { ns: "settings" })} value={valueOrDash(modem?.revision)} />
                    <DetailRow icon={Cable} label={t("cellularPrimaryPort", { ns: "settings" })} value={valueOrDash(modem?.primaryPort)} />
                    <DetailRow icon={Cable} label={t("cellularPorts", { ns: "settings" })} value={formatPorts(modem?.ports)} />
                  </dl>
                </div>

                <div className="rounded-2xl border border-base-300 bg-base-100/60 p-3">
                  <h3 className="mb-2 flex items-center gap-2 text-sm font-semibold"><IdCard size={17} className="text-info" aria-hidden="true" />{t("cellularStatusSIM", { ns: "settings" })}</h3>
                  <dl className="space-y-0.5 text-sm">
                    <DetailRow icon={BadgeCheck} label={t("cellularSIM", { ns: "settings" })} value={modem?.simPath ? t("cellularSIMReady", { ns: "settings" }) : t("cellularSIMMissing", { ns: "settings" })} tone={modem?.simPath ? "text-success" : "text-warning"} />
                    <DetailRow icon={IdCard} label={t("cellularStatusSIMPath", { ns: "settings" })} value={valueOrDash(modem?.simPath)} />
                    <DetailRow icon={Activity} label={t("cellularStatusModemState", { ns: "settings" })} value={stateLabel(modem?.state, t)} tone={stateTone} />
                    <DetailRow icon={CircleAlert} label={t("cellularStatusFailure", { ns: "settings" })} value={valueOrDash(modem?.failedReason)} tone={modem?.failedReason ? "text-error" : "text-success"} />
                  </dl>
                </div>
              </div>

              {cellularInterfaces.length > 1 ? (
                <div className="rounded-2xl border border-base-300 bg-base-100/60 p-3">
                  <h3 className="mb-2 flex items-center gap-2 text-sm font-semibold"><Gauge size={17} className="text-info" aria-hidden="true" />{t("cellularStatusOtherInterfaces", { ns: "settings" })}</h3>
                  <div className="grid gap-2 sm:grid-cols-2 xl:grid-cols-3">
                    {cellularInterfaces.slice(1).map((item) => (
                      <div key={item.name} className="rounded-xl border border-base-300 bg-base-100/70 p-3">
                        <div className="flex items-center justify-between gap-2"><span className="font-mono font-semibold">{item.name}</span><Badge tone={item.state === "connected" ? "success" : "warning"}>{stateLabel(item.state, t)}</Badge></div>
                        <div className="mt-2 text-xs text-base-content/60">{formatAddresses(item)}</div>
                      </div>
                    ))}
                  </div>
                </div>
              ) : null}
            </>
          )}
        </PanelBody>
      </Panel>
    </section>
  );
}
