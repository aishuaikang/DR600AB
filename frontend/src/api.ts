import type {
  ApiErrorPayload,
  ChannelsResponse,
  DetectionRecord,
  DetectionSessionRequest,
  DetectionSessionResponse,
  DetectionSettings,
  DeveloperLoginRequest,
  DeveloperSessionResponse,
  EventMessage,
  FpvRecord,
  GpioChannel,
  GpioChannelStateRequest,
  GpioChannelStateResponse,
  GPSRecord,
  GPSSessionRequest,
  GPSSessionResponse,
  GPSSettings,
  ListResponse,
  LocaleMeta,
  NetworkInterfacesResponse,
  NetworkPriorityBatchRequest,
  NetworkPriorityBatchResponse,
  NetworkPriorityRequest,
  NetworkInterfaceUpdateRequest,
  NetworkInterfaceUpdateResponse,
  ParsedMessage,
  PortsResponse,
  StreamHandlers,
  WiFiConnectRequest,
  WiFiConnectResponse,
  WiFiNetworksResponse,
} from "./types";

const API_PREFIX = "/api/v1";

function developerHeaders(developerToken: string) {
  return developerToken ? { "X-Developer-Token": developerToken } : undefined;
}

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

export function getPorts(locale: string, developerToken: string): Promise<PortsResponse> {
  return requestJson<PortsResponse>("/serial/ports", {
    headers: developerHeaders(developerToken),
  }, locale);
}

export function getSession(locale: string, developerToken: string): Promise<DetectionSessionResponse> {
  return requestJson<DetectionSessionResponse>("/detection/session", {
    headers: developerHeaders(developerToken),
  }, locale);
}

export function getDetectionSettings(locale: string, developerToken: string): Promise<DetectionSettings> {
  return requestJson<DetectionSettings>("/detection/settings", {
    headers: developerHeaders(developerToken),
  }, locale);
}

export function updateDetectionSettings(
  payload: DetectionSessionRequest,
  locale: string,
  developerToken: string,
): Promise<DetectionSessionResponse> {
  return requestJson<DetectionSessionResponse>("/detection/settings", {
    method: "PUT",
    headers: developerHeaders(developerToken),
    body: JSON.stringify(payload),
  }, locale);
}

export function startSession(
  payload: DetectionSessionRequest,
  locale: string,
  developerToken: string,
): Promise<DetectionSessionResponse> {
  return updateDetectionSettings(payload, locale, developerToken);
}

export function stopSession(locale: string, developerToken: string): Promise<DetectionSessionResponse> {
  return requestJson<DetectionSessionResponse>("/detection/session", {
    method: "DELETE",
    headers: developerHeaders(developerToken),
  }, locale);
}

export function getGPSSession(locale: string, developerToken: string): Promise<GPSSessionResponse> {
  return requestJson<GPSSessionResponse>("/gps/session", {
    headers: developerHeaders(developerToken),
  }, locale);
}

export function getGPSSettings(locale: string, developerToken: string): Promise<GPSSettings> {
  return requestJson<GPSSettings>("/gps/settings", {
    headers: developerHeaders(developerToken),
  }, locale);
}

export function updateGPSSettings(
  payload: GPSSessionRequest,
  locale: string,
  developerToken: string,
): Promise<GPSSessionResponse> {
  return requestJson<GPSSessionResponse>("/gps/settings", {
    method: "PUT",
    headers: developerHeaders(developerToken),
    body: JSON.stringify(payload),
  }, locale);
}

export function stopGPSSession(locale: string, developerToken: string): Promise<GPSSessionResponse> {
  return requestJson<GPSSessionResponse>("/gps/session", {
    method: "DELETE",
    headers: developerHeaders(developerToken),
  }, locale);
}

export function getDetections(
  locale: string,
  developerToken: string,
  limit = 200,
): Promise<ListResponse<DetectionRecord>> {
  return requestJson<ListResponse<DetectionRecord>>(`/detection/records?limit=${limit}`, {
    headers: developerHeaders(developerToken),
  }, locale);
}

export function getParsed(locale: string, developerToken: string, limit = 200): Promise<ListResponse<ParsedMessage>> {
  return requestJson<ListResponse<ParsedMessage>>(`/parsed/records?limit=${limit}`, {
    headers: developerHeaders(developerToken),
  }, locale);
}

export function getFpv(locale: string, developerToken: string, limit = 100): Promise<ListResponse<FpvRecord>> {
  return requestJson<ListResponse<FpvRecord>>(`/fpv/records?limit=${limit}`, {
    headers: developerHeaders(developerToken),
  }, locale);
}

export function getGPSRecords(locale: string, developerToken: string, limit = 200): Promise<ListResponse<GPSRecord>> {
  return requestJson<ListResponse<GPSRecord>>(`/gps/records?limit=${limit}`, {
    headers: developerHeaders(developerToken),
  }, locale);
}

export function getChannels(locale: string, developerToken: string): Promise<ChannelsResponse> {
  return requestJson<ChannelsResponse>("/interference/channels", {
    headers: developerHeaders(developerToken),
  }, locale);
}

export function setChannelState(
  id: string,
  payload: GpioChannelStateRequest,
  locale: string,
  developerToken: string,
): Promise<GpioChannelStateResponse> {
  return requestJson<GpioChannelStateResponse>(`/interference/channels/${encodeURIComponent(id)}/state`, {
    method: "POST",
    headers: developerHeaders(developerToken),
    body: JSON.stringify(payload),
  }, locale);
}

export function getNetworkInterfaces(locale: string, developerToken: string): Promise<NetworkInterfacesResponse> {
  return requestJson<NetworkInterfacesResponse>("/network/interfaces", {
    headers: developerHeaders(developerToken),
  }, locale);
}

export function updateNetworkInterface(
  name: string,
  payload: NetworkInterfaceUpdateRequest,
  locale: string,
  developerToken: string,
): Promise<NetworkInterfaceUpdateResponse> {
  return requestJson<NetworkInterfaceUpdateResponse>(`/network/interfaces/${encodeURIComponent(name)}`, {
    method: "PUT",
    headers: developerHeaders(developerToken),
    body: JSON.stringify(payload),
  }, locale);
}

export function updateNetworkInterfacePriority(
  name: string,
  payload: NetworkPriorityRequest,
  locale: string,
  developerToken: string,
): Promise<NetworkInterfaceUpdateResponse> {
  return requestJson<NetworkInterfaceUpdateResponse>(`/network/interfaces/${encodeURIComponent(name)}/priority`, {
    method: "PUT",
    headers: developerHeaders(developerToken),
    body: JSON.stringify(payload),
  }, locale);
}

export function updateNetworkInterfacePriorities(
  payload: NetworkPriorityBatchRequest,
  locale: string,
  developerToken: string,
): Promise<NetworkPriorityBatchResponse> {
  return requestJson<NetworkPriorityBatchResponse>("/network/priorities", {
    method: "PUT",
    headers: developerHeaders(developerToken),
    body: JSON.stringify(payload),
  }, locale);
}

export function getWiFiNetworks(locale: string, developerToken: string): Promise<WiFiNetworksResponse> {
  return requestJson<WiFiNetworksResponse>("/network/wifi", {
    headers: developerHeaders(developerToken),
  }, locale);
}

export function connectWiFi(
  payload: WiFiConnectRequest,
  locale: string,
  developerToken: string,
): Promise<WiFiConnectResponse> {
  return requestJson<WiFiConnectResponse>("/network/wifi/connect", {
    method: "POST",
    headers: developerHeaders(developerToken),
    body: JSON.stringify(payload),
  }, locale);
}

export function createDeveloperSessionRequest(
  payload: DeveloperLoginRequest,
  locale: string,
): Promise<DeveloperSessionResponse> {
  return requestJson<DeveloperSessionResponse>("/developer/session", {
    method: "POST",
    body: JSON.stringify(payload),
  }, locale);
}

export function deleteDeveloperSessionRequest(token: string, locale: string): Promise<void> {
  return requestJson<void>("/developer/session", {
    method: "DELETE",
    headers: developerHeaders(token),
  }, locale);
}

function parseStreamEvent<T>(raw: string): EventMessage<T> | null {
  try {
    return JSON.parse(raw) as EventMessage<T>;
  } catch {
    return null;
  }
}

export function openDetectionStream(locale: string, developerToken: string, handlers: StreamHandlers): () => void {
  const params = new URLSearchParams({ locale });
  if (developerToken) {
    params.set("developerToken", developerToken);
  }
  const source = new EventSource(`${API_PREFIX}/detection/stream?${params.toString()}`);

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
  bind("gps.session.started", handlers.onGPSSessionStarted);
  bind("gps.session.stopped", handlers.onGPSSessionStopped);
  bind("gps.session.connecting", handlers.onGPSSessionState);
  bind("gps.session.reconnecting", handlers.onGPSSessionState);
  bind("gps.record", handlers.onGPSRecord);
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
