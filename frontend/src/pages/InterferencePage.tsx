import type { TFunction } from "i18next";
import { Play, Square } from "lucide-react";

import { Badge } from "../components/Badge";
import { InfoTile } from "../components/InfoTile";
import { Panel, PanelBody } from "../components/Panel";
import { SectionHeader } from "../components/SectionHeader";
import type { GpioChannel } from "../types";
import { cx } from "../utils/classnames";
import { channelStatusLabel, toneForStatus } from "../utils/channels";

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

export function InterferencePage({
  channels,
  t,
  onToggleChannel,
}: {
  channels: GpioChannel[];
  t: TFunction;
  onToggleChannel: (channel: GpioChannel) => void;
}) {
  return (
    <section className="grid gap-4">
      <Panel>
        <PanelBody>
          <SectionHeader
            title={t("title", { ns: "interference" })}
            description={t("description", { ns: "interference" })}
          />
          <div className="grid gap-4 lg:grid-cols-2 2xl:grid-cols-4">
            {channels.map((channel) => (
              <ChannelCard key={channel.id} channel={channel} t={t} onToggle={onToggleChannel} />
            ))}
          </div>
        </PanelBody>
      </Panel>
    </section>
  );
}
