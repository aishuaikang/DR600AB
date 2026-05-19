import type { TFunction } from "i18next";
import { RefreshCw } from "lucide-react";

import { Badge } from "../components/Badge";
import { Panel, PanelBody } from "../components/Panel";
import { SectionHeader } from "../components/SectionHeader";
import type { Banner } from "../app/types";
import type { GPSFix, GPSRecord } from "../types";
import { formatNumber, formatTime } from "../utils/format";

function fixLabel(fix: GPSFix | undefined, t: TFunction) {
  if (!fix || !fix.valid) {
    return t("gpsNoFix", { ns: "settings" });
  }
  return `${fix.latitude.toFixed(6)}, ${fix.longitude.toFixed(6)}`;
}

function optionalMetric(value: number | undefined, unit: string, locale: string, digits = 1) {
  if (typeof value !== "number" || Number.isNaN(value)) {
    return "-";
  }
  return `${formatNumber(locale, value, digits)} ${unit}`;
}

function satelliteLabel(fix: GPSFix | undefined, locale: string) {
  if (!fix || typeof fix.satellites !== "number") {
    return "-";
  }
  return formatNumber(locale, fix.satellites, 0);
}

function qualityLabel(fix: GPSFix | undefined) {
  if (!fix || typeof fix.fixQuality !== "number") {
    return "-";
  }
  return String(fix.fixQuality);
}

export function GPSRecordsPage({
  records,
  banner,
  loading,
  locale,
  t,
  onRefresh,
}: {
  records: GPSRecord[];
  banner: Banner;
  loading: boolean;
  locale: string;
  t: TFunction;
  onRefresh: () => void;
}) {
  return (
    <section className="flex min-h-0 min-w-0 flex-1">
      <Panel className="flex min-h-0 min-w-0 flex-1 flex-col">
        <PanelBody className="min-h-0 min-w-0 flex-1">
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
            <div className="alert alert-soft alert-error py-3 text-sm" role="alert">
              <span className="min-w-0 [overflow-wrap:anywhere]">{banner.message}</span>
            </div>
          ) : null}

          <div className="min-h-0 min-w-0 flex-1 overflow-auto rounded-2xl border border-base-300 bg-base-100/70">
            <table className="table table-zebra table-sm w-full min-w-[74rem] table-fixed whitespace-nowrap">
              <thead className="sticky top-0 z-10 bg-base-200">
                <tr>
                  <th className="w-[13rem]">{t("time", { ns: "common" })}</th>
                  <th className="w-[7rem]">{t("gpsRecordType", { ns: "settings" })}</th>
                  <th className="w-[10rem]">{t("gpsRecordPort", { ns: "settings" })}</th>
                  <th className="w-[17rem]">{t("gpsRecordFix", { ns: "settings" })}</th>
                  <th className="w-[7rem]">{t("gpsRecordAltitude", { ns: "settings" })}</th>
                  <th className="w-[7rem]">{t("gpsRecordSpeed", { ns: "settings" })}</th>
                  <th className="w-[6rem]">{t("gpsRecordSatellites", { ns: "settings" })}</th>
                  <th className="w-[6rem]">{t("gpsRecordQuality", { ns: "settings" })}</th>
                  <th className="w-[22rem]">{t("gpsRecordRaw", { ns: "settings" })}</th>
                </tr>
              </thead>
              <tbody>
                {records.length === 0 ? (
                  <tr>
                    <td colSpan={9} className="p-3">
                      <div className="admin-empty-state admin-empty-state--table">
                        {loading ? t("loading", { ns: "common" }) : t("empty", { ns: "common" })}
                      </div>
                    </td>
                  </tr>
                ) : (
                  records.map((record) => {
                    const fixed = Boolean(record.fix?.valid);
                    return (
                      <tr key={`${record.sessionId}-${record.receivedAt}-${record.raw}`} className="row-hover">
                        <td className="tabular-nums">
                          <span className="block truncate" title={formatTime(locale, record.receivedAt)}>
                            {formatTime(locale, record.receivedAt)}
                          </span>
                        </td>
                        <td>
                          <Badge tone={fixed ? "success" : "neutral"}>{record.type || "-"}</Badge>
                        </td>
                        <td>
                          <code className="block truncate rounded-xl bg-base-200/80 px-2 py-1 text-xs" title={record.portName || "-"}>
                            {record.portName || "-"}
                          </code>
                        </td>
                        <td>
                          <span
                            className="block truncate font-mono text-xs tabular-nums text-base-content/80"
                            title={fixLabel(record.fix, t)}
                          >
                            {fixLabel(record.fix, t)}
                          </span>
                        </td>
                        <td className="tabular-nums">{optionalMetric(record.fix?.altitudeM, "m", locale)}</td>
                        <td className="tabular-nums">{optionalMetric(record.fix?.speedKnots, "kn", locale)}</td>
                        <td className="tabular-nums">{satelliteLabel(record.fix, locale)}</td>
                        <td className="tabular-nums">{qualityLabel(record.fix)}</td>
                        <td>
                          <code className="block truncate rounded-xl bg-base-200/80 px-2 py-1 text-xs" title={record.raw}>
                            {record.raw || "-"}
                          </code>
                        </td>
                      </tr>
                    );
                  })
                )}
              </tbody>
            </table>
          </div>
        </PanelBody>
      </Panel>
    </section>
  );
}
