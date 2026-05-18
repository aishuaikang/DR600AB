import type { GpioChannel } from "../types";

const BOARD_CHANNELS: GpioChannel[] = [
  {
    id: "io1",
    label: "IOC4",
    pin: 20,
    bands: ["433", "800", "900", "1.4"],
    reserved: false,
    enabled: false,
    actualLevel: "unknown",
    desiredLevel: "low",
    status: "idle",
  },
  {
    id: "io2",
    label: "IOC2",
    pin: 18,
    bands: ["1.2", "1.5"],
    reserved: false,
    enabled: false,
    actualLevel: "unknown",
    desiredLevel: "low",
    status: "idle",
  },
  {
    id: "io3",
    label: "IOC3",
    pin: 19,
    bands: ["2.4", "5.2", "5.8"],
    reserved: false,
    enabled: false,
    actualLevel: "unknown",
    desiredLevel: "low",
    status: "idle",
  },
  {
    id: "io4",
    label: "IOC5",
    pin: 21,
    bands: [],
    reserved: true,
    enabled: false,
    actualLevel: "unknown",
    desiredLevel: "low",
    status: "idle",
  },
  {
    id: "io5",
    label: "I3B4",
    pin: 108,
    bands: [],
    reserved: true,
    enabled: false,
    actualLevel: "unknown",
    desiredLevel: "low",
    status: "idle",
  },
  {
    id: "io6",
    label: "I3B5",
    pin: 109,
    bands: [],
    reserved: true,
    enabled: false,
    actualLevel: "unknown",
    desiredLevel: "low",
    status: "idle",
  },
  {
    id: "io7",
    label: "I3C0",
    pin: 112,
    bands: [],
    reserved: true,
    enabled: false,
    actualLevel: "unknown",
    desiredLevel: "low",
    status: "idle",
  },
  {
    id: "io8",
    label: "I3C1",
    pin: 113,
    bands: [],
    reserved: true,
    enabled: false,
    actualLevel: "unknown",
    desiredLevel: "low",
    status: "idle",
  },
];

export function normalizeGpioChannels(channels: GpioChannel[]) {
  const byId = new Map(channels.map((channel) => [channel.id, channel]));
  const normalized = BOARD_CHANNELS.map((definition) => {
    const current = byId.get(definition.id);
    if (!current) {
      return definition;
    }
    return {
      ...definition,
      ...current,
      bands: Array.isArray(current.bands) ? current.bands : definition.bands,
      enabled: current.enabled,
      status: current.status === "reserved" ? "idle" : current.status,
      desiredLevel: current.desiredLevel,
      actualLevel: current.actualLevel,
    };
  });

  const knownIds = new Set(BOARD_CHANNELS.map((channel) => channel.id));
  return normalized.concat(channels.filter((channel) => !knownIds.has(channel.id)));
}
