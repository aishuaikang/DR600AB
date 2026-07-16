import { useEffect, useState } from "react";
import type { TFunction } from "i18next";
import type { LucideIcon } from "lucide-react";
import {
  Activity,
  BadgeCheck,
  BadgeX,
  Clock3,
  Compass,
  Gauge,
  MapPin,
  Mountain,
  Navigation,
  Orbit,
  Radio,
  RefreshCw,
  Satellite,
  ShieldCheck,
  ShieldX,
  Signal,
  Timer,
  TriangleAlert,
} from "lucide-react";

import { Badge } from "../components/Badge";
import { Panel, PanelBody } from "../components/Panel";
import { SectionHeader } from "../components/SectionHeader";
import type { Banner } from "../app/types";
import type { GPSRecord, NMEADetails, NMEASatellite } from "../types";
import { formatNumber, formatTime } from "../utils/format";

function latestDetails(records: GPSRecord[], sentence: string) {
  return records.find((record) => record.type === sentence && record.details)?.details;
}

function metric(value: number | undefined, locale: string, digits = 1, unit = "") {
  if (typeof value !== "number" || Number.isNaN(value)) {
    return "-";
  }
  return `${formatNumber(locale, value, digits)}${unit ? ` ${unit}` : ""}`;
}

function fixTypeLabel(value: number | undefined, t: TFunction) {
  switch (value) {
    case 2:
      return t("gpsFix2D", { ns: "settings" });
    case 3:
      return t("gpsFix3D", { ns: "settings" });
    default:
      return t("gpsNoFix", { ns: "settings" });
  }
}

function signalTone(value: number | undefined) {
  if (typeof value !== "number") return "bg-base-300";
  if (value >= 35) return "bg-success";
  if (value >= 25) return "bg-warning";
  return "bg-error";
}

function signalTextTone(value: number | undefined) {
  if (typeof value !== "number") return "text-base-content/35";
  if (value >= 35) return "text-success";
  if (value >= 25) return "text-warning";
  return "text-error";
}

function collectSatellites(records: GPSRecord[]) {
  const gsvRecords = records.filter((record) => record.type === "GSV" && record.details?.messageNumber);
  for (const anchor of gsvRecords) {
    const anchorDetails = anchor.details!;
    const totalMessages = anchorDetails.totalMessages || 1;
    const anchorTime = Date.parse(anchor.receivedAt);
    const groupKey = `${anchorDetails.talker || ""}:${anchorDetails.signalId || ""}:${totalMessages}:${anchorDetails.totalSatellites || ""}`;
    const messages = new Map<number, NMEADetails>();
    for (const candidate of gsvRecords) {
      const details = candidate.details!;
      const candidateKey = `${details.talker || ""}:${details.signalId || ""}:${details.totalMessages || 1}:${details.totalSatellites || ""}`;
      if (candidateKey !== groupKey || Math.abs(Date.parse(candidate.receivedAt) - anchorTime) > 900) continue;
      messages.set(details.messageNumber!, details);
    }
    if (messages.size < totalMessages) continue;
    return [...messages.values()]
      .sort((a, b) => (a.messageNumber || 0) - (b.messageNumber || 0))
      .flatMap((details) => details.satellites || []);
  }
  return [];
}

type PositionState = "stable" | "unstable" | "none";

function validPosition(details: NMEADetails | undefined) {
  if (!details || typeof details.latitude !== "number" || typeof details.longitude !== "number") return false;
  if (details.checksumValid === false) return false;
  if (details.sentence === "RMC") return details.status === "A";
  if (details.sentence === "GGA") return (details.fixQuality ?? 0) > 0;
  return false;
}

function positionEpochRecords(records: GPSRecord[]) {
  const positionRecords: GPSRecord[] = [];
  const epochIndexes = new Map<string, number>();
  for (const record of records) {
    if ((record.type !== "GGA" && record.type !== "RMC") || !record.details) continue;
    const epoch = record.details.utcTime || record.receivedAt;
    const existingIndex = epochIndexes.get(epoch);
    if (existingIndex === undefined) {
      epochIndexes.set(epoch, positionRecords.length);
      positionRecords.push(record);
      continue;
    }
    const existing = positionRecords[existingIndex];
    const candidateValid = validPosition(record.details);
    const existingValid = validPosition(existing.details);
    const shouldPreferValid = candidateValid && !existingValid;
    const shouldPreferGGA = candidateValid === existingValid && record.type === "GGA" && existing.type !== "GGA";
    if (shouldPreferValid || shouldPreferGGA) {
      positionRecords[existingIndex] = record;
    }
  }
  return positionRecords;
}

function timestamp(value: string | undefined) {
  const parsed = value ? Date.parse(value) : Number.NaN;
  return Number.isFinite(parsed) ? parsed : 0;
}

function positionSnapshot(records: GPSRecord[], elapsedSinceLatestArrivalMs: number) {
  const positionRecords = positionEpochRecords(records);
  const latest = positionRecords[0];
  const lastValid = positionRecords.find((record) => validPosition(record.details));
  const latestRecordTime = timestamp(records[0]?.receivedAt);
  const ageFromLatestRecord = (record: GPSRecord | undefined) => record
    ? Math.max(0, latestRecordTime - timestamp(record.receivedAt)) + elapsedSinceLatestArrivalMs
    : Number.POSITIVE_INFINITY;
  const lastValidAgeMs = ageFromLatestRecord(lastValid);
  const latestPositionAgeMs = ageFromLatestRecord(latest);
  const recent = positionRecords.slice(0, 3);
  const stable = recent.length === 3 && recent.every((record) => {
    if (!validPosition(record.details)) return false;
    const hdop = record.details?.hdop;
    return typeof hdop !== "number" || hdop <= 5;
  });
  let state: PositionState = "none";
  if (stable && latestPositionAgeMs <= 5_000) state = "stable";
  else if (lastValid && lastValidAgeMs <= 5_000) state = "unstable";
  return { state, latest, lastValid, lastValidAgeMs };
}

function recordStatus(details: NMEADetails | undefined, t: TFunction) {
  if (!details) return "-";
  if (details.checksumValid === false) return t("gpsChecksumInvalid", { ns: "settings" });
  switch (details.sentence) {
    case "RMC":
      return details.status === "A" ? t("gpsPositionValid", { ns: "settings" }) : t("gpsPositionInvalid", { ns: "settings" });
    case "GGA":
      return (details.fixQuality ?? 0) > 0 ? t("gpsPositionValid", { ns: "settings" }) : t("gpsPositionInvalid", { ns: "settings" });
    case "GSA":
      return fixTypeLabel(details.fixType, t);
    case "GSV":
      return `${details.totalSatellites ?? 0} ${t("gpsSatellitesVisible", { ns: "settings" })}`;
    case "VTG":
      return details.mode === "N" ? t("gpsPositionInvalid", { ns: "settings" }) : details.mode || "-";
    default:
      return details.checksumValid === true ? t("gpsChecksumValid", { ns: "settings" }) : "-";
  }
}

function sentenceIcon(sentence: string | undefined) {
  switch (sentence) {
    case "RMC":
      return Navigation;
    case "GGA":
      return MapPin;
    case "GSA":
      return Orbit;
    case "GSV":
      return Satellite;
    case "VTG":
      return Compass;
    default:
      return Radio;
  }
}

function RecordStatusIcon({ details, positioned }: { details: NMEADetails | undefined; positioned: boolean | undefined }) {
  if (details?.checksumValid === false) return <ShieldX size={14} aria-hidden="true" />;
  if (positioned) return <BadgeCheck size={14} aria-hidden="true" />;
  if (details?.sentence === "GSV") return <Satellite size={14} aria-hidden="true" />;
  if (details?.sentence === "GSA") return <Orbit size={14} aria-hidden="true" />;
  return <BadgeX size={14} aria-hidden="true" />;
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

function sentenceSummary(details: NMEADetails | undefined, locale: string, t: TFunction) {
  if (!details) return "-";
  switch (details.sentence) {
    case "RMC":
      return details.status === "A"
        ? `${metric(details.speedKnots, locale, 1, "kn")} · ${metric(details.courseTrue, locale, 1, "°")}`
        : t("gpsPositionInvalid", { ns: "settings" });
    case "GGA":
      return `${t("gpsRecordQuality", { ns: "settings" })} ${details.fixQuality ?? "-"} · HDOP ${metric(details.hdop, locale, 2)}`;
    case "GSA":
      return `${fixTypeLabel(details.fixType, t)} · PDOP ${metric(details.pdop, locale, 2)} / HDOP ${metric(details.hdop, locale, 2)} / VDOP ${metric(details.vdop, locale, 2)}`;
    case "GSV":
      return `${t("gpsSatellitesVisible", { ns: "settings" })} ${details.totalSatellites ?? "-"} · ${t("gpsGsvPage", { ns: "settings", current: details.messageNumber ?? "-", total: details.totalMessages ?? "-" })}`;
    case "VTG":
      return `${metric(details.speedKph, locale, 1, "km/h")} · ${metric(details.courseTrue, locale, 1, "°")} · ${details.mode || "-"}`;
    default:
      return details.sentence;
  }
}

function SatelliteSignal({ satellite, locale }: { satellite: NMEASatellite; locale: string }) {
  const strength = satellite.signalDbHz;
  const width = typeof strength === "number" ? Math.min(100, Math.max(4, strength * 2)) : 0;
  return (
    <div className="rounded-xl border border-base-300 bg-base-100/70 p-2">
      <div className="flex items-center justify-between gap-2 text-xs">
        <span className="flex items-center gap-1.5 font-mono font-semibold"><Satellite size={13} className="text-info" aria-hidden="true" />{satellite.id}</span>
        <span className={`flex items-center gap-1 tabular-nums ${signalTextTone(strength)}`}><Signal size={13} aria-hidden="true" />{metric(strength, locale, 0, "dB-Hz")}</span>
      </div>
      <div className="mt-2 h-1.5 overflow-hidden rounded-full bg-base-300/70">
        <div className={`h-full rounded-full ${signalTone(strength)}`} style={{ width: `${width}%` }} />
      </div>
      <div className="mt-1 flex justify-between text-[10px] text-base-content/50">
        <span className="flex items-center gap-1"><Mountain size={10} aria-hidden="true" />{metric(satellite.elevation, locale, 0, "°")}</span>
        <span className="flex items-center gap-1"><Compass size={10} aria-hidden="true" />{metric(satellite.azimuth, locale, 0, "°")}</span>
      </div>
    </div>
  );
}

export function GPSRecordsPage({ records, banner, loading, locale, t, onRefresh }: {
  records: GPSRecord[];
  banner: Banner;
  loading: boolean;
  locale: string;
  t: TFunction;
  onRefresh: () => void;
}) {
  const [now, setNow] = useState(() => Date.now());
  const [lastRecordArrivalMs, setLastRecordArrivalMs] = useState(() => Date.now());
  const latestRecordKey = records[0]
    ? `${records[0].sessionId}:${records[0].receivedAt}:${records[0].raw}`
    : "";
  useEffect(() => {
    const timer = window.setInterval(() => setNow(Date.now()), 1_000);
    return () => window.clearInterval(timer);
  }, []);
  useEffect(() => {
    setLastRecordArrivalMs(Date.now());
  }, [latestRecordKey]);

  const snapshot = positionSnapshot(records, Math.max(0, now - lastRecordArrivalMs));
  const latestRMC = latestDetails(records, "RMC");
  const lastValidRMC = records.find((record) => record.type === "RMC" && validPosition(record.details))?.details;
  const displayPosition = snapshot.lastValid?.details;
  const latestGSA = records.find((record) => record.type === "GSA" && record.details?.checksumValid !== false)?.details;
  const lastValidGSA = records.find((record) => record.type === "GSA" && record.details?.checksumValid !== false && (record.details?.fixType ?? 1) > 1)?.details;
  const displayGSA = snapshot.state === "none" ? latestGSA : lastValidGSA ?? latestGSA;
  const satellites = collectSatellites(records);
  const positioned = snapshot.state === "stable";
  const unstable = snapshot.state === "unstable";
  const positionLabel = positioned
    ? t("gpsPositionValid", { ns: "settings" })
    : unstable
      ? t("gpsSignalUnstable", { ns: "settings" })
      : t("gpsPositionInvalid", { ns: "settings" });
  const positionTone = positioned ? "text-success" : unstable ? "text-warning" : "text-error";
  const hdop = displayGSA?.hdop ?? displayPosition?.hdop;
  const accuracyPoor = typeof hdop === "number" && hdop > 5;
  const strongestSignal = satellites.reduce<number | undefined>((best, item) => {
    if (typeof item.signalDbHz !== "number") return best;
    return typeof best === "number" ? Math.max(best, item.signalDbHz) : item.signalDbHz;
  }, undefined);

  return (
    <section className="flex min-h-0 min-w-0 flex-1">
      <Panel className="flex min-h-0 min-w-0 flex-1 flex-col overflow-hidden">
        <PanelBody className="min-h-0 min-w-0 flex-1 overflow-auto">
          <SectionHeader
            title={t("gpsRecordsTitle", { ns: "settings" })}
            description={t("gpsRecordsDescription", { ns: "settings" })}
            action={
              <button className="btn btn-sm btn-outline btn-info" type="button" onClick={onRefresh} disabled={loading}>
                <RefreshCw size={16} className={loading ? "animate-spin" : undefined} />
                <span>{t("refresh", { ns: "common" })}</span>
              </button>
            }
          />

          {banner.kind === "error" && banner.message ? (
            <div className="alert alert-soft alert-error py-3 text-sm" role="alert"><span>{banner.message}</span></div>
          ) : null}

          <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-5">
            <StatusMetric icon={positioned ? BadgeCheck : unstable ? TriangleAlert : BadgeX} label={t("gpsPositionState", { ns: "settings" })} value={positionLabel} tone={positionTone} />
            <StatusMetric icon={Orbit} label={t("gpsFixMode", { ns: "settings" })} value={fixTypeLabel(displayGSA?.fixType, t)} tone={(displayGSA?.fixType ?? 1) > 1 ? positionTone : "text-error"} />
            <StatusMetric icon={Satellite} label={t("gpsSatellitesUsed", { ns: "settings" })} value={`${displayPosition?.satellitesUsed ?? displayGSA?.satelliteIds?.length ?? 0} / ${satellites.length || latestDetails(records, "GSV")?.totalSatellites || 0}`} tone="text-info" />
            <StatusMetric icon={Signal} label={t("gpsStrongestSignal", { ns: "settings" })} value={metric(strongestSignal, locale, 0, "dB-Hz")} tone={signalTextTone(strongestSignal)} />
            <StatusMetric icon={accuracyPoor ? TriangleAlert : Gauge} label="PDOP / HDOP / VDOP" value={`${metric(displayGSA?.pdop, locale, 2)} / ${metric(hdop, locale, 2)} / ${metric(displayGSA?.vdop, locale, 2)}`} tone={accuracyPoor ? "text-error" : "text-success"} />
          </div>

          <div className="grid gap-3 xl:grid-cols-[minmax(0,1.35fr)_minmax(20rem,0.65fr)]">
            <div className="rounded-2xl border border-base-300 bg-base-100/60 p-3">
              <div className="mb-3 flex items-center justify-between">
                <h3 className="flex items-center gap-2 text-sm font-semibold"><Satellite size={17} className="text-info" aria-hidden="true" />{t("gpsSatelliteSignals", { ns: "settings" })}</h3>
                <Badge tone={satellites.length ? "info" : "neutral"}><span className="flex items-center gap-1"><Signal size={12} aria-hidden="true" />{satellites.length}</span></Badge>
              </div>
              {satellites.length ? (
                <div className="grid gap-2 sm:grid-cols-3 lg:grid-cols-4 2xl:grid-cols-6">
                  {satellites.map((satellite, index) => <SatelliteSignal key={`${satellite.id}-${index}`} satellite={satellite} locale={locale} />)}
                </div>
              ) : <div className="admin-empty-state admin-empty-state--table">{t("gpsNoSatelliteDetails", { ns: "settings" })}</div>}
            </div>

            <div className="rounded-2xl border border-base-300 bg-base-100/60 p-3">
              <h3 className="mb-2 flex items-center gap-2 text-sm font-semibold"><Activity size={17} className="text-info" aria-hidden="true" />{t("gpsLatestStatus", { ns: "settings" })}</h3>
              <dl className="space-y-0.5 text-sm">
                <DetailRow icon={Clock3} label="UTC" value={displayPosition?.utcTime || lastValidRMC?.utcTime || latestRMC?.utcTime || "-"} />
                <DetailRow icon={positioned ? MapPin : unstable ? TriangleAlert : BadgeX} label={t("gpsRecordFix", { ns: "settings" })} value={displayPosition && typeof displayPosition.latitude === "number" ? `${displayPosition.latitude.toFixed(6)}, ${displayPosition.longitude?.toFixed(6)}` : t("gpsNoFix", { ns: "settings" })} tone={positionTone} />
                <DetailRow icon={Timer} label={t("gpsLastValidAge", { ns: "settings" })} value={Number.isFinite(snapshot.lastValidAgeMs) ? t("gpsSecondsAgo", { ns: "settings", seconds: Math.floor(snapshot.lastValidAgeMs / 1_000) }) : "-"} tone={positionTone} />
                <DetailRow icon={Mountain} label={t("gpsRecordAltitude", { ns: "settings" })} value={metric(displayPosition?.altitudeM, locale, 1, "m")} />
                <DetailRow icon={Timer} label={t("gpsRecordSpeed", { ns: "settings" })} value={metric(lastValidRMC?.speedKnots, locale, 1, "kn")} />
                <DetailRow icon={Compass} label={t("gpsCourse", { ns: "settings" })} value={metric(lastValidRMC?.courseTrue, locale, 1, "°")} />
                <DetailRow icon={accuracyPoor ? TriangleAlert : Gauge} label={t("gpsAccuracyState", { ns: "settings" })} value={accuracyPoor ? t("gpsAccuracyPoor", { ns: "settings" }) : t("gpsAccuracyNormal", { ns: "settings" })} tone={accuracyPoor ? "text-error" : "text-success"} />
                <DetailRow icon={records[0]?.details?.checksumValid === false ? ShieldX : ShieldCheck} label={t("gpsChecksum", { ns: "settings" })} value={records[0]?.details?.checksumValid === false ? t("gpsChecksumInvalid", { ns: "settings" }) : t("gpsChecksumValid", { ns: "settings" })} tone={records[0]?.details?.checksumValid === false ? "text-error" : "text-success"} />
              </dl>
            </div>
          </div>

          <div className="min-h-[18rem] min-w-0 overflow-auto rounded-2xl border border-base-300 bg-base-100/70">
            <table className="table table-zebra table-sm w-full min-w-[72rem] table-fixed whitespace-nowrap">
              <thead className="sticky top-0 z-10 bg-base-200"><tr>
                <th className="w-[12rem]"><span className="flex items-center gap-1.5"><Clock3 size={14} aria-hidden="true" />{t("time", { ns: "common" })}</span></th>
                <th className="w-[6rem]"><span className="flex items-center gap-1.5"><Radio size={14} aria-hidden="true" />{t("gpsRecordType", { ns: "settings" })}</span></th>
                <th className="w-[10rem]"><span className="flex items-center gap-1.5"><Activity size={14} aria-hidden="true" />{t("gpsRecordStatus", { ns: "settings" })}</span></th>
                <th className="w-[30rem]"><span className="flex items-center gap-1.5"><Gauge size={14} aria-hidden="true" />{t("gpsParsedDetails", { ns: "settings" })}</span></th>
                <th className="w-[28rem]"><span className="flex items-center gap-1.5"><Radio size={14} aria-hidden="true" />{t("gpsRecordRaw", { ns: "settings" })}</span></th>
              </tr></thead>
              <tbody>
                {records.length === 0 ? <tr><td colSpan={5}><div className="admin-empty-state admin-empty-state--table">{loading ? t("loading", { ns: "common" }) : t("empty", { ns: "common" })}</div></td></tr> : records.map((record) => {
                  const valid = record.details?.checksumValid;
                  const positionedRecord = record.fix?.valid;
                  const SentenceIcon = sentenceIcon(record.type);
                  return <tr key={`${record.sessionId}-${record.receivedAt}-${record.raw}`} className="row-hover">
                    <td className="truncate tabular-nums" title={formatTime(locale, record.receivedAt)}>{formatTime(locale, record.receivedAt)}</td>
                    <td><Badge tone={record.fix?.valid ? "success" : "neutral"}><span className="flex items-center gap-1"><SentenceIcon size={13} aria-hidden="true" />{record.details?.talker || ""}{record.type || "-"}</span></Badge></td>
                    <td><Badge tone={valid === false ? "error" : positionedRecord ? "success" : "warning"}><span className="flex items-center gap-1"><RecordStatusIcon details={record.details} positioned={positionedRecord} />{recordStatus(record.details, t)}</span></Badge></td>
                    <td><span className="block truncate text-xs text-base-content/75" title={sentenceSummary(record.details, locale, t)}>{sentenceSummary(record.details, locale, t)}</span></td>
                    <td><code className="block truncate rounded-xl bg-base-200/80 px-2 py-1 text-xs" title={record.raw}>{record.raw || "-"}</code></td>
                  </tr>;
                })}
              </tbody>
            </table>
          </div>
        </PanelBody>
      </Panel>
    </section>
  );
}
