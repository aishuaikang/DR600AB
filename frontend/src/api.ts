import type {
  ApiErrorPayload,
  ChannelsResponse,
  DetectionRecord,
  DetectionSessionRequest,
  DetectionSessionResponse,
  DetectionSettings,
  EventMessage,
  FpvRecord,
  GpioChannel,
  GpioChannelStateRequest,
  GpioChannelStateResponse,
  ListResponse,
  LocaleMeta,
  ParsedMessage,
  PortsResponse,
  StreamHandlers,
} from "./types";

const API_PREFIX = "/api/v1";

export class ApiRequestError extends Error {
  status: number;
  code?: string;
  details?: unknown;

  constructor(message: string, status: number, code?: string, details?: unknown) {
    super(message);
    this.name = "ApiRequestError";
    this.status = status;
    this.code = code;
    this.details = details;
  }
}

async function requestJson<T>(path: string, init: RequestInit = {}, locale?: string): Promise<T> {
  const headers = new Headers(init.headers);
  headers.set("Accept", "application/json");
  if (locale) {
    headers.set("X-Locale", locale);
  }

  if (init.body && !(init.body instanceof FormData) && !headers.has("Content-Type")) {
    headers.set("Content-Type", "application/json");
  }

  const response = await fetch(`${API_PREFIX}${path}`, {
    ...init,
    headers,
  });

  if (!response.ok) {
    let payload: ApiErrorPayload | null = null;
    try {
      payload = (await response.json()) as ApiErrorPayload;
    } catch {
      payload = null;
    }
    throw new ApiRequestError(
      payload?.message || response.statusText || "Request failed",
      response.status,
      payload?.code,
      payload?.details,
    );
  }

  if (response.status === 204) {
    return undefined as T;
  }

  return (await response.json()) as T;
}

export function getLocales(): Promise<LocaleMeta> {
  return requestJson<LocaleMeta>("/meta/locales");
}

export function getPorts(locale: string): Promise<PortsResponse> {
  return requestJson<PortsResponse>("/serial/ports", {}, locale);
}

export function getSession(locale: string): Promise<DetectionSessionResponse> {
  return requestJson<DetectionSessionResponse>("/detection/session", {}, locale);
}

export function getDetectionSettings(locale: string): Promise<DetectionSettings> {
  return requestJson<DetectionSettings>("/detection/settings", {}, locale);
}

export function updateDetectionSettings(payload: DetectionSessionRequest, locale: string): Promise<DetectionSessionResponse> {
  return requestJson<DetectionSessionResponse>("/detection/settings", {
    method: "PUT",
    body: JSON.stringify(payload),
  }, locale);
}

export function startSession(payload: DetectionSessionRequest, locale: string): Promise<DetectionSessionResponse> {
  return updateDetectionSettings(payload, locale);
}

export function stopSession(locale: string): Promise<DetectionSessionResponse> {
  return requestJson<DetectionSessionResponse>("/detection/session", {
    method: "DELETE",
  }, locale);
}

export function getDetections(locale: string, limit = 200): Promise<ListResponse<DetectionRecord>> {
  return requestJson<ListResponse<DetectionRecord>>(`/detection/records?limit=${limit}`, {}, locale);
}

export function getParsed(locale: string, limit = 200): Promise<ListResponse<ParsedMessage>> {
  return requestJson<ListResponse<ParsedMessage>>(`/parsed/records?limit=${limit}`, {}, locale);
}

export function getFpv(locale: string, limit = 100): Promise<ListResponse<FpvRecord>> {
  return requestJson<ListResponse<FpvRecord>>(`/fpv/records?limit=${limit}`, {}, locale);
}

export function getChannels(locale: string): Promise<ChannelsResponse> {
  return requestJson<ChannelsResponse>("/interference/channels", {}, locale);
}

export function setChannelState(id: string, payload: GpioChannelStateRequest, locale: string): Promise<GpioChannelStateResponse> {
  return requestJson<GpioChannelStateResponse>(`/interference/channels/${encodeURIComponent(id)}/state`, {
    method: "POST",
    body: JSON.stringify(payload),
  }, locale);
}

function parseStreamEvent<T>(raw: string): EventMessage<T> | null {
  try {
    return JSON.parse(raw) as EventMessage<T>;
  } catch {
    return null;
  }
}

export function openDetectionStream(locale: string, handlers: StreamHandlers): () => void {
  const source = new EventSource(`${API_PREFIX}/detection/stream?locale=${encodeURIComponent(locale)}`);

  const bind = <T,>(type: string, handler?: (event: EventMessage<T>) => void) => {
    if (!handler) {
      return;
    }
    source.addEventListener(type, (message) => {
      const event = parseStreamEvent<T>((message as MessageEvent<string>).data);
      if (event) {
        handler(event);
      }
    });
  };

  bind("session.started", handlers.onSessionStarted);
  bind("session.stopped", handlers.onSessionStopped);
  bind("session.connecting", handlers.onSessionState);
  bind("session.reconnecting", handlers.onSessionState);
  bind("detection.parsed", handlers.onParsed);
  bind("detection.record", handlers.onDetection);
  bind("fpv.record", handlers.onFpv);
  bind("gpio.channel.updated", handlers.onChannelUpdated);

  source.onerror = () => {
    if (source.readyState === EventSource.CLOSED) {
      handlers.onError?.(new Error("实时流连接已断开"));
    }
  };

  return () => source.close();
}
