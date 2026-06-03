import type { TFunction } from "i18next";
import { Play, Square } from "lucide-react";

import { Badge } from "../components/Badge";
import { LoadingSpinner, SkeletonBlock } from "../components/LoadingState";
import { Panel, PanelBody } from "../components/Panel";
import { SectionHeader } from "../components/SectionHeader";
import type { GpioChannel } from "../types";
import { cx } from "../utils/classnames";
import { channelStatusLabel, toneForStatus } from "../utils/channels";

function ChannelCard({
  channel,
  busy,
  disabled,
  t,
  onToggle,
}: {
  channel: GpioChannel;
  busy: boolean;
  disabled: boolean;
  t: TFunction;
  onToggle: (channel: GpioChannel) => void;
}) {
  const tone = toneForStatus(channel.status);
  const toggleLabel = channel.enabled ? t("disable", { ns: "interference" }) : t("enable", { ns: "interference" });
  const bands = Array.isArray(channel.bands) ? channel.bands : [];

  return (
    <article
      className={cx(
        "app-card-motion flex min-w-0 flex-col gap-2 rounded-2xl border p-2.5",
        busy && "app-card-motion--busy",
        channel.reserved
          ? "border-base-300/70 bg-base-100/35 text-base-content/70"
          : "border-base-300 bg-base-100/75",
      )}
    >
      <div className="flex min-w-0 items-center justify-between gap-2">
        <div className="flex min-w-0 items-center gap-2">
          <h3 className="shrink-0 text-sm font-bold leading-5 text-base-content">{channel.label}</h3>
          <span className="truncate font-mono text-xs font-semibold text-base-content/60">
            IO{channel.pin}
          </span>
          {channel.reserved ? <Badge tone="warning">{t("reserved", { ns: "common" })}</Badge> : null}
        </div>
        <div className="shrink-0">
          <Badge tone={tone}>{channelStatusLabel(channel, t)}</Badge>
        </div>
      </div>

      <div className="flex min-h-6 flex-wrap content-start gap-1.5">
        {bands.length > 0 ? (
          bands.map((band) => (
            <Badge key={band} tone="info" outline>
              {band}
            </Badge>
          ))
        ) : (
          <span className="inline-flex min-h-5 items-center rounded-full border border-base-300/80 px-2 text-[11px] font-semibold text-base-content/45">
            {t("reservedChannel", { ns: "interference" })}
          </span>
        )}
      </div>

      <div className="grid grid-cols-2 gap-1.5 text-xs">
        <div className="min-w-0 rounded-xl border border-base-300/70 bg-base-200/45 px-2 py-1.5">
          <span className="block truncate text-[11px] font-medium text-base-content/45">
            {t("desired", { ns: "interference" })}
          </span>
          <strong className="block truncate font-mono text-xs text-base-content">{channel.desiredLevel}</strong>
        </div>
        <div className="min-w-0 rounded-xl border border-base-300/70 bg-base-200/45 px-2 py-1.5">
          <span className="block truncate text-[11px] font-medium text-base-content/45">
            {t("actual", { ns: "interference" })}
          </span>
          <strong className="block truncate font-mono text-xs text-base-content">{channel.actualLevel}</strong>
        </div>
      </div>

      {channel.lastError ? <p className="rounded-xl bg-error/10 px-2 py-1.5 text-xs text-error">{channel.lastError}</p> : null}

      <div className="mt-auto grid gap-1.5">
        <button
          className={cx(
            "btn btn-xs btn-block min-h-8",
            channel.enabled ? "btn-outline btn-error" : "btn-primary",
            busy && "app-busy-button",
          )}
          type="button"
          disabled={disabled}
          onClick={() => onToggle(channel)}
        >
          {busy ? <LoadingSpinner size={14} /> : channel.enabled ? <Square size={14} /> : <Play size={14} />}
          <span>{toggleLabel}</span>
        </button>
        <span className="truncate text-[11px] leading-4 text-base-content/45">{t("toggle", { ns: "interference" })}</span>
      </div>
    </article>
  );
}

export function InterferencePage({
  channels,
  busyChannelId,
  t,
  onToggleChannel,
}: {
  channels: GpioChannel[];
  busyChannelId?: string;
  t: TFunction;
  onToggleChannel: (channel: GpioChannel) => void;
}) {
  const busy = Boolean(busyChannelId);

  return (
    <section className="grid gap-3">
      <Panel>
        <PanelBody>
          <SectionHeader
            title={t("title", { ns: "interference" })}
            description={t("description", { ns: "interference" })}
          />
          <div className="grid gap-2 sm:grid-cols-2 lg:grid-cols-3 2xl:grid-cols-4">
            {channels.length === 0
              ? Array.from({ length: 3 }, (_, index) => (
                  <div key={index} className="rounded-2xl border border-base-300 bg-base-100/45 p-2.5">
                    <SkeletonBlock className="h-5 w-28" />
                    <SkeletonBlock className="mt-3 h-6 w-full" />
                    <div className="mt-3 grid grid-cols-2 gap-1.5">
                      <SkeletonBlock className="h-10" />
                      <SkeletonBlock className="h-10" />
                    </div>
                    <SkeletonBlock className="mt-3 h-8 w-full" />
                  </div>
                ))
              : channels.map((channel) => (
                  <ChannelCard
                    key={channel.id}
                    channel={channel}
                    busy={busyChannelId === channel.id}
                    disabled={busy}
                    t={t}
                    onToggle={onToggleChannel}
                  />
                ))}
          </div>
        </PanelBody>
      </Panel>
    </section>
  );
}
