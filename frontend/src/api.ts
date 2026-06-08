import type {
  ApiErrorPayload,
  ChannelsResponse,
  CompassRecord,
  CompassSessionRequest,
  CompassSessionResponse,
  CompassSettings,
  DetectionRecord,
  DeceptionSessionRequest,
  DeceptionSessionResponse,
  DeceptionQueryResponse,
  DeceptionReport,
  DeceptionReportDeleteResponse,
  DeceptionReportStatus,
  DeceptionReportSummary,
  DeceptionSettings,
  DetectionSessionRequest,
  DetectionSessionResponse,
  DetectionSettings,
  DeveloperLoginRequest,
  DeveloperSessionResponse,
  EventMessage,
  FPVVideoRecord,
  FPVVideoRecordDeleteRequest,
  FPVVideoRecordDeleteResponse,
  FPVVideoRecordStatus,
  GpioChannel,
  GpioChannelStateRequest,
  GpioChannelStateResponse,
  GPSRecord,
  GPSSessionRequest,
  GPSSessionResponse,
  GPSSettings,
  IntrusionDeleteRequest,
  IntrusionDeleteResponse,
  IntrusionRecord,
  IntrusionTargetType,
  InterferenceReport,
  InterferenceReportDeleteResponse,
  InterferenceReportStatus,
  InterferenceReportSummary,
  ListResponse,
  LocaleMeta,
  NetworkInterfacesResponse,
  NetworkPriorityBatchRequest,
  NetworkPriorityBatchResponse,
  NetworkInterfaceUpdateRequest,
  NetworkInterfaceUpdateResponse,
  ParsedMessage,
  PortsResponse,
  ScreenDetectionTarget,
  ScreenDeviceLocationResponse,
  ScreenDeceptionDeviceStatus,
  ScreenDeceptionRequest,
  ScreenDeceptionResponse,
  ScreenDeceptionState,
  ScreenFpvFrame,
  ScreenFpvStatus,
  ScreenFpvVideoHandlers,
  ScreenPositionTarget,
  ScreenRuntimeStatus,
  ScreenStrikeRequest,
  ScreenStrikeResponse,
  ScreenStrikeState,
  ScreenStreamHandlers,
  StreamHandlers,
  UserSettings,
  WiFiConnectRequest,
  WiFiConnectResponse,
  WiFiNetworksResponse,
} from "./types";
import i18n from "./i18n";

const API_PREFIX = "/api/v1";

type UnauthorizedHandler = (error: ApiRequestError) => void;

let unauthorizedHandler: UnauthorizedHandler | null = null;

export function setUnauthorizedHandler(handler: UnauthorizedHandler | null) {
  unauthorizedHandler = handler;

  return () => {
    if (unauthorizedHandler === handler) {
      unauthorizedHandler = null;
    }
  };
}

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

async function readApiRequestError(response: Response, fallbackMessage: string): Promise<ApiRequestError> {
  let payload: ApiErrorPayload | null = null;
  try {
    payload = (await response.json()) as ApiErrorPayload;
  } catch {
    payload = null;
  }
  return new ApiRequestError(
    payload?.message || fallbackMessage,
    response.status,
    payload?.code,
    payload?.details,
  );
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
    const fallbackMessage = locale
      ? i18n.getFixedT(locale, "common")("requestFailed")
      : i18n.t("requestFailed", { ns: "common" });
    const error = await readApiRequestError(response, fallbackMessage);
    if (response.status === 401 && headers.has("X-Developer-Token")) {
      unauthorizedHandler?.(error);
    }
    throw error;
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

export function getDeceptionSession(locale: string, developerToken: string): Promise<DeceptionSessionResponse> {
  return requestJson<DeceptionSessionResponse>("/deception/session", {
    headers: developerHeaders(developerToken),
  }, locale);
}

export function getDeceptionSettings(locale: string, developerToken: string): Promise<DeceptionSettings> {
  return requestJson<DeceptionSettings>("/deception/settings", {
    headers: developerHeaders(developerToken),
  }, locale);
}

export function updateDeceptionSettings(
  payload: DeceptionSessionRequest,
  locale: string,
  developerToken: string,
): Promise<DeceptionSessionResponse> {
  return requestJson<DeceptionSessionResponse>("/deception/settings", {
    method: "PUT",
    headers: developerHeaders(developerToken),
    body: JSON.stringify(payload),
  }, locale);
}

export function getCompassSession(locale: string, developerToken: string): Promise<CompassSessionResponse> {
  return requestJson<CompassSessionResponse>("/compass/session", {
    headers: developerHeaders(developerToken),
  }, locale);
}

export function getCompassSettings(locale: string, developerToken: string): Promise<CompassSettings> {
  return requestJson<CompassSettings>("/compass/settings", {
    headers: developerHeaders(developerToken),
  }, locale);
}

export function updateCompassSettings(
  payload: CompassSessionRequest,
  locale: string,
  developerToken: string,
): Promise<CompassSessionResponse> {
  return requestJson<CompassSessionResponse>("/compass/settings", {
    method: "PUT",
    headers: developerHeaders(developerToken),
    body: JSON.stringify(payload),
  }, locale);
}

export function queryDeceptionDevice(
  item: string,
  locale: string,
  developerToken: string,
): Promise<DeceptionQueryResponse> {
  return requestJson<DeceptionQueryResponse>(`/deception/query/${encodeURIComponent(item)}`, {
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

export function getGPSRecords(locale: string, developerToken: string, limit = 200): Promise<ListResponse<GPSRecord>> {
  return requestJson<ListResponse<GPSRecord>>(`/gps/records?limit=${limit}`, {
    headers: developerHeaders(developerToken),
  }, locale);
}

export function getCompassRecords(
  locale: string,
  developerToken: string,
  limit = 200,
): Promise<ListResponse<CompassRecord>> {
  return requestJson<ListResponse<CompassRecord>>(`/compass/records?limit=${limit}`, {
    headers: developerHeaders(developerToken),
  }, locale);
}

export function getIntrusions(
  locale: string,
  limit = 200,
  targetType?: IntrusionTargetType | "all",
  offset = 0,
): Promise<ListResponse<IntrusionRecord>> {
  const params = new URLSearchParams({ limit: String(limit) });
  if (offset > 0) {
    params.set("offset", String(offset));
  }
  if (targetType && targetType !== "all") {
    params.set("type", targetType);
  }
  return requestJson<ListResponse<IntrusionRecord>>(`/intrusions?${params.toString()}`, {}, locale);
}

export function getDeceptionReports(
  locale: string,
  limit = 200,
  status?: DeceptionReportStatus | "all",
  offset = 0,
): Promise<ListResponse<DeceptionReportSummary>> {
  const params = new URLSearchParams({ limit: String(limit) });
  if (offset > 0) {
    params.set("offset", String(offset));
  }
  if (status && status !== "all") {
    params.set("status", status);
  }
  return requestJson<ListResponse<DeceptionReportSummary>>(`/deception-reports?${params.toString()}`, {}, locale);
}

export function getDeceptionReport(id: string, locale: string): Promise<DeceptionReport> {
  return requestJson<DeceptionReport>(`/deception-reports/${encodeURIComponent(id)}`, {}, locale);
}

export function deleteFailedDeceptionReport(id: string, locale: string): Promise<DeceptionReportDeleteResponse> {
  return requestJson<DeceptionReportDeleteResponse>(`/deception-reports/${encodeURIComponent(id)}`, {
    method: "DELETE",
  }, locale);
}

export function getInterferenceReports(
  locale: string,
  limit = 200,
  status?: InterferenceReportStatus | "all",
  offset = 0,
): Promise<ListResponse<InterferenceReportSummary>> {
  const params = new URLSearchParams({ limit: String(limit) });
  if (offset > 0) {
    params.set("offset", String(offset));
  }
  if (status && status !== "all") {
    params.set("status", status);
  }
  return requestJson<ListResponse<InterferenceReportSummary>>(`/interference-reports?${params.toString()}`, {}, locale);
}

export function getInterferenceReport(id: string, locale: string): Promise<InterferenceReport> {
  return requestJson<InterferenceReport>(`/interference-reports/${encodeURIComponent(id)}`, {}, locale);
}

export function deleteFailedInterferenceReport(id: string, locale: string): Promise<InterferenceReportDeleteResponse> {
  return requestJson<InterferenceReportDeleteResponse>(`/interference-reports/${encodeURIComponent(id)}`, {
    method: "DELETE",
  }, locale);
}

export function deleteIntrusions(payload: IntrusionDeleteRequest, locale: string): Promise<IntrusionDeleteResponse> {
  return requestJson<IntrusionDeleteResponse>("/intrusions", {
    method: "DELETE",
    body: JSON.stringify(payload),
  }, locale);
}

export function getFPVVideoRecords(
  locale: string,
  limit = 200,
  status?: FPVVideoRecordStatus | "all",
  offset = 0,
): Promise<ListResponse<FPVVideoRecord>> {
  const params = new URLSearchParams({ limit: String(limit) });
  if (offset > 0) {
    params.set("offset", String(offset));
  }
  if (status && status !== "all") {
    params.set("status", status);
  }
  return requestJson<ListResponse<FPVVideoRecord>>(`/fpv-video-records?${params.toString()}`, {}, locale);
}

export function getFPVVideoRecord(id: string, locale: string): Promise<FPVVideoRecord> {
  return requestJson<FPVVideoRecord>(`/fpv-video-records/${encodeURIComponent(id)}`, {}, locale);
}

export function deleteFPVVideoRecords(
  payload: FPVVideoRecordDeleteRequest,
  locale: string,
): Promise<FPVVideoRecordDeleteResponse> {
  return requestJson<FPVVideoRecordDeleteResponse>("/fpv-video-records", {
    method: "DELETE",
    body: JSON.stringify(payload),
  }, locale);
}

export function getUserSettings(): Promise<UserSettings> {
  return requestJson<UserSettings>("/user/settings");
}

export function updateUserSettings(payload: UserSettings, locale: string): Promise<UserSettings> {
  return requestJson<UserSettings>("/user/settings", {
    method: "PUT",
    body: JSON.stringify(payload),
  }, locale);
}

export function getScreenStatus(locale?: string): Promise<ScreenRuntimeStatus> {
  return requestJson<ScreenRuntimeStatus>("/screen/status", {}, locale);
}

export function getScreenDetections(limit = 100): Promise<ListResponse<ScreenDetectionTarget>> {
  return requestJson<ListResponse<ScreenDetectionTarget>>(`/screen/detections?limit=${limit}`);
}

export function getScreenPositions(limit = 100): Promise<ListResponse<ScreenPositionTarget>> {
  return requestJson<ListResponse<ScreenPositionTarget>>(`/screen/positions?limit=${limit}`);
}

export function getScreenDeviceLocation(): Promise<ScreenDeviceLocationResponse> {
  return requestJson<ScreenDeviceLocationResponse>("/screen/device-location");
}

export function getScreenStrike(locale?: string): Promise<ScreenStrikeState> {
  return requestJson<ScreenStrikeState>("/screen/strike", {}, locale);
}

export function getScreenDeception(locale?: string): Promise<ScreenDeceptionState> {
  return requestJson<ScreenDeceptionState>("/screen/deception", {}, locale);
}

export function getScreenDeceptionStatus(locale?: string): Promise<ScreenDeceptionDeviceStatus> {
  return requestJson<ScreenDeceptionDeviceStatus>("/screen/deception/status", {}, locale);
}

export function updateScreenStrike(
  payload: ScreenStrikeRequest,
  locale: string,
): Promise<ScreenStrikeResponse> {
  return requestJson<ScreenStrikeResponse>("/screen/strike", {
    method: "POST",
    body: JSON.stringify(payload),
  }, locale);
}

export function updateScreenDeception(
  payload: ScreenDeceptionRequest,
  locale: string,
): Promise<ScreenDeceptionResponse> {
  return requestJson<ScreenDeceptionResponse>("/screen/deception", {
    method: "POST",
    body: JSON.stringify(payload),
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

function parseSSEPayload<T>(raw: string): T | null {
  try {
    return JSON.parse(raw) as T;
  } catch {
    return null;
  }
}

function dispatchSSEBlock(raw: string, dispatch: (event: string, data: string) => void) {
  let event = "message";
  const dataLines: string[] = [];

  for (const line of raw.split("\n")) {
    if (!line || line.startsWith(":")) {
      continue;
    }
    const separator = line.indexOf(":");
    const field = separator === -1 ? line : line.slice(0, separator);
    let value = separator === -1 ? "" : line.slice(separator + 1);
    if (value.startsWith(" ")) {
      value = value.slice(1);
    }

    if (field === "event") {
      event = value;
    } else if (field === "data") {
      dataLines.push(value);
    }
  }

  if (dataLines.length) {
    dispatch(event, dataLines.join("\n"));
  }
}

async function readSSEStream(response: Response, dispatch: (event: string, data: string) => void) {
  if (!response.body) {
    throw new Error(i18n.t("fpvVideoConnectionError", { ns: "screen" }));
  }

  const reader = response.body.getReader();
  const decoder = new TextDecoder();
  let buffer = "";

  try {
    for (;;) {
      const { value, done } = await reader.read();
      if (done) {
        break;
      }
      buffer += decoder.decode(value, { stream: true });
      buffer = buffer.replace(/\r\n/g, "\n").replace(/\r/g, "\n");

      let separator = buffer.indexOf("\n\n");
      while (separator !== -1) {
        dispatchSSEBlock(buffer.slice(0, separator), dispatch);
        buffer = buffer.slice(separator + 2);
        separator = buffer.indexOf("\n\n");
      }
    }

    buffer += decoder.decode();
    buffer = buffer.replace(/\r\n/g, "\n").replace(/\r/g, "\n").trim();
    if (buffer) {
      dispatchSSEBlock(buffer, dispatch);
    }
  } finally {
    reader.releaseLock();
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
  bind("deception.session.started", handlers.onDeceptionSessionStarted);
  bind("deception.session.stopped", handlers.onDeceptionSessionStopped);
  bind("deception.session.connecting", handlers.onDeceptionSessionState);
  bind("deception.session.reconnecting", handlers.onDeceptionSessionState);
  bind("compass.session.started", handlers.onCompassSessionStarted);
  bind("compass.session.stopped", handlers.onCompassSessionStopped);
  bind("compass.session.connecting", handlers.onCompassSessionState);
  bind("compass.session.reconnecting", handlers.onCompassSessionState);
  bind("compass.record", handlers.onCompassRecord);
  bind("gps.record", handlers.onGPSRecord);
  bind("detection.parsed", handlers.onParsed);
  bind("detection.record", handlers.onDetection);
  bind("gpio.channel.updated", handlers.onChannelUpdated);

  source.onerror = () => {
    if (source.readyState === EventSource.CLOSED) {
      const t = i18n.getFixedT(locale, "common");
      handlers.onError?.(new Error(t("stream.detectionDisconnected")));
    }
  };

  return () => source.close();
}

export function openScreenStream(handlers: ScreenStreamHandlers): () => void {
  const source = new EventSource(`${API_PREFIX}/screen/stream`);

  source.addEventListener("screen.detection.updated", (message) => {
    const event = parseStreamEvent<ScreenDetectionTarget>((message as MessageEvent<string>).data);
    if (event) {
      handlers.onDetectionUpdated?.(event);
    }
  });

  source.addEventListener("screen.position.updated", (message) => {
    const event = parseStreamEvent<ScreenPositionTarget>((message as MessageEvent<string>).data);
    if (event) {
      handlers.onPositionUpdated?.(event);
    }
  });

  source.addEventListener("screen.strike.updated", (message) => {
    const event = parseStreamEvent<ScreenStrikeState>((message as MessageEvent<string>).data);
    if (event) {
      handlers.onStrikeUpdated?.(event);
    }
  });

  source.addEventListener("screen.deception.updated", (message) => {
    const event = parseStreamEvent<ScreenDeceptionState>((message as MessageEvent<string>).data);
    if (event) {
      handlers.onDeceptionUpdated?.(event);
    }
  });

  source.addEventListener("compass.record", (message) => {
    const event = parseStreamEvent<CompassRecord>((message as MessageEvent<string>).data);
    if (event) {
      handlers.onCompassRecord?.(event);
    }
  });

  source.onerror = () => {
    if (source.readyState === EventSource.CLOSED) {
      handlers.onError?.(new Error(i18n.t("stream.screenDisconnected", { ns: "common" })));
    }
  };

  return () => source.close();
}

export function openScreenFpvVideo(frequency: number, handlers: ScreenFpvVideoHandlers): () => void {
  const params = new URLSearchParams({ frequency: String(frequency) });
  if (handlers.targetId) {
    params.set("targetId", handlers.targetId);
  }
  const controller = new AbortController();
  const fallbackMessage = i18n.t("fpvVideoConnectionError", { ns: "screen" });

  void (async () => {
    try {
      const response = await fetch(`${API_PREFIX}/screen/fpv/video?${params.toString()}`, {
        headers: { Accept: "text/event-stream" },
        signal: controller.signal,
      });

      if (!response.ok) {
        throw await readApiRequestError(response, fallbackMessage);
      }

      await readSSEStream(response, (event, data) => {
        if (event === "status") {
          const status = parseSSEPayload<ScreenFpvStatus>(data);
          if (status) {
            handlers.onStatus?.(status);
          }
        } else if (event === "frame") {
          const frame = parseSSEPayload<ScreenFpvFrame>(data);
          if (frame) {
            handlers.onFrame?.(frame);
          }
        }
      });

      if (!controller.signal.aborted) {
        handlers.onError?.(new Error(fallbackMessage));
      }
    } catch (error) {
      if (controller.signal.aborted) {
        return;
      }
      handlers.onError?.(error instanceof ApiRequestError ? error : new Error(fallbackMessage));
    }
  })();

  return () => controller.abort();
}
