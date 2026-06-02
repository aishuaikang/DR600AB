import type { Banner } from "../app/types";
import type {
  CompassSessionResponse,
  DetectionRecord,
  DetectionSessionResponse,
  DetectionSettings,
  GPSRecord,
  GPSSessionResponse,
  GPSSettings,
  ParsedMessage,
  PortInfo,
} from "../types";

export function serialKey(receivePort: string, sendPort: string, rxBaudRate?: number, txBaudRate?: number) {
  const rxBaudRatePart = typeof rxBaudRate === "number" ? `|${rxBaudRate}` : "";
  const txBaudRatePart = typeof txBaudRate === "number" ? `|${txBaudRate}` : "";
  return `${receivePort.trim()}|${sendPort.trim()}${rxBaudRatePart}${txBaudRatePart}`;
}

export function resolveInitialPorts(
  session: DetectionSessionResponse | null,
  settings: DetectionSettings | null,
  _ports: PortInfo[],
) {
  const sessionReceive = session?.rxPortName || session?.portName || "";
  const sessionSend = session?.txPortName || sessionReceive || "";
  const savedReceive = settings?.rxPortName || settings?.portName || "";
  const savedSend = settings?.txPortName || savedReceive || "";

  const receivePort = sessionReceive || savedReceive;
  const sendPort = sessionSend || settings?.txPortName || savedSend || receivePort;

  return { receivePort, sendPort };
}

export function resolveInitialGPSPorts(
  session: GPSSessionResponse | null,
  settings: GPSSettings | null,
  _ports: PortInfo[],
) {
  const sessionDataPort = session?.dataPortName || session?.portName || "";
  const sessionControlPort = session?.controlPortName || "";
  const savedDataPort = settings?.dataPortName || settings?.portName || "";
  const savedControlPort = settings?.controlPortName || "";

  const dataPort = sessionDataPort || savedDataPort;
  const controlPort = sessionControlPort || savedControlPort || dataPort;

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

export function compassSessionBannerText(session: CompassSessionResponse, fallback: string) {
  const message = session.message || fallback;
  if (session.lastError && session.state && session.state !== "connected" && session.state !== "inactive") {
    return `${message}：${session.lastError}`;
  }
  return message;
}

export function compassSessionBannerKind(session: CompassSessionResponse): Banner["kind"] {
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

export function dedupeDetections(items: DetectionRecord[], item: DetectionRecord, limit: number) {
  return [item, ...items.filter((entry) => entry.id !== item.id)].slice(0, limit);
}

export function dedupeGPSRecords(items: GPSRecord[], item: GPSRecord, limit: number) {
  const key = `${item.sessionId}|${item.receivedAt}|${item.raw}`;
  return [item, ...items.filter((entry) => `${entry.sessionId}|${entry.receivedAt}|${entry.raw}` !== key)].slice(0, limit);
}

export function extractErrorMessage(error: unknown, fallback: string) {
  if (error instanceof Error) {
    return error.message;
  }
  return fallback;
}
