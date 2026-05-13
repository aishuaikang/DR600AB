import type { TFunction } from "i18next";

import type { Tone } from "../app/types";
import type { GpioChannel } from "../types";

export function toneForStatus(kind: string): Tone {
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

export function channelStatusLabel(channel: GpioChannel, t: TFunction) {
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
