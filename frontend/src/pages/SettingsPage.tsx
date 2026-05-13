import type { TFunction } from "i18next";
import { RefreshCw } from "lucide-react";

import { BannerAlert } from "../components/BannerAlert";
import { InfoTile } from "../components/InfoTile";
import { Panel, PanelBody } from "../components/Panel";
import { PortSelect } from "../components/PortSelect";
import { SectionHeader } from "../components/SectionHeader";
import type { Banner } from "../app/types";
import type { PortInfo } from "../types";

export function SettingsPage({
  banner,
  ports,
  selectedReceivePort,
  selectedSendPort,
  sessionStateLabel,
  currentReceivePort,
  currentSendPort,
  t,
  onRefresh,
  onReceivePortChange,
  onSendPortChange,
}: {
  banner: Banner;
  ports: PortInfo[];
  selectedReceivePort: string;
  selectedSendPort: string;
  sessionStateLabel: string;
  currentReceivePort: string;
  currentSendPort: string;
  t: TFunction;
  onRefresh: () => void;
  onReceivePortChange: (value: string) => void;
  onSendPortChange: (value: string) => void;
}) {
  return (
    <section className="grid gap-4">
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

      <Panel>
        <PanelBody>
          <SectionHeader
            title={t("serialTitle", { ns: "settings" })}
            description={t("serialDescription", { ns: "settings" })}
            action={
              <button className="btn btn-sm btn-outline btn-info" type="button" onClick={onRefresh}>
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
              onChange={onReceivePortChange}
            />
            <PortSelect
              label={t("sendPort", { ns: "settings" })}
              placeholder={t("selectSendPort", { ns: "settings" })}
              value={selectedSendPort}
              ports={ports}
              activeText={t("active", { ns: "common" })}
              onChange={onSendPortChange}
            />
          </div>

          {ports.length === 0 ? <span className="text-sm text-base-content/55">{t("noPorts", { ns: "settings" })}</span> : null}
        </PanelBody>
      </Panel>
    </section>
  );
}
