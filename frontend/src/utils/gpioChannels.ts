import type { GpioChannel } from "../types";

function channelIdSortValue(channel: GpioChannel) {
  const match = /^io(\d+)$/.exec(channel.id);
  return match ? Number(match[1]) : Number.MAX_SAFE_INTEGER;
}

function channelPinSortValue(channel: GpioChannel) {
  return Number.isFinite(channel.pin) ? channel.pin : Number.MAX_SAFE_INTEGER;
}

export function normalizeGpioChannels(channels: GpioChannel[]) {
  return channels
    .filter(Boolean)
    .map((channel) => ({
      ...channel,
      label: channel.label || `IO${channel.pin}`,
      bands: Array.isArray(channel.bands) ? channel.bands : [],
      reserved: Boolean(channel.reserved),
      enabled: Boolean(channel.enabled),
      status: channel.status === "reserved" ? "idle" : channel.status || "idle",
      desiredLevel: channel.desiredLevel || "low",
      actualLevel: channel.actualLevel || "unknown",
    }))
    .sort((left, right) => {
      const byId = channelIdSortValue(left) - channelIdSortValue(right);
      if (byId !== 0) {
        return byId;
      }
      return channelPinSortValue(left) - channelPinSortValue(right);
    });
}
