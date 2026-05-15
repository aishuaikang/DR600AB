import type { Banner } from "../app/types";
import type {
  DetectionSessionResponse,
  DetectionSettings,
  GPSSessionResponse,
  GPSSettings,
  ParsedMessage,
  PortInfo,
} from "../types";

export function serialKey(receivePort: string, sendPort: string) {
  return `${receivePort.trim()}|${sendPort.trim()}`;
}

export function resolveInitialPorts(
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

export function resolveInitialGPSPorts(
  session: GPSSessionResponse | null,
  settings: GPSSettings | null,
  ports: PortInfo[],
) {
  const sessionDataPort = session?.dataPortName || session?.portName || "";
  const sessionControlPort = session?.controlPortName || "";
  const savedDataPort = settings?.dataPortName || settings?.portName || "";
  const savedControlPort = settings?.controlPortName || "";
  const firstPort = ports[0]?.name || "";

  const dataPort = sessionDataPort || savedDataPort || firstPort;
  const controlPort =
    sessionControlPort
    || savedControlPort
    || ports.find((item) => item.name !== dataPort)?.name
    || dataPort;

  return { dataPort, controlPort };
}

export function sessionBannerText(session: DetectionSessionResponse, fallback: string) {
  const message = session.message || fallback;
  if (session.lastError && session.state && session.state !== "connected" && session.state !== "inactive") {
    return `${message}：${session.lastError}`;
  }
  return message;
}

export function sessionBannerKind(session: DetectionSessionResponse): Banner["kind"] {
  if (session.state === "connected" || session.active) {
    return "success";
  }
  if (session.state === "connecting" || session.state === "reconnecting") {
    return "loading";
  }
  return "idle";
}

export function gpsSessionBannerText(session: GPSSessionResponse, fallback: string) {
  const message = session.message || fallback;
  if (session.lastError && session.state && session.state !== "connected" && session.state !== "inactive") {
    return `${message}：${session.lastError}`;
  }
  return message;
}

export function gpsSessionBannerKind(session: GPSSessionResponse): Banner["kind"] {
  if (session.state === "connected" || session.active) {
    return "success";
  }
  if (session.state === "connecting" || session.state === "reconnecting") {
    return "loading";
  }
  return "idle";
}

export function dedupeById<T extends { id: string }>(items: T[], item: T, limit: number) {
  return [item, ...items.filter((entry) => entry.id !== item.id)].slice(0, limit);
}

export function dedupeParsed(items: ParsedMessage[], item: ParsedMessage, limit: number) {
  const key = `${item.type}|${item.time}|${item.raw}`;
  return [item, ...items.filter((entry) => `${entry.type}|${entry.time}|${entry.raw}` !== key)].slice(0, limit);
}

export function extractErrorMessage(error: unknown) {
  if (error instanceof Error) {
    return error.message;
  }
  return "Unexpected error";
}
