export interface LocaleMeta {
  defaultLocale: string;
  supportedLocales: string[];
  namespaces: string[];
}

export type ParsedMessageType =
  | "did_encrypted"
  | "rid"
  | "did_plain"
  | "detect"
  | "heartbeat";

export type DebugRecordPage = "detection-records" | "parsed-records";

export interface PortInfo {
  name: string;
  active: boolean;
}

export interface DetectionSessionRequest {
  portName?: string;
  rxPortName?: string;
  txPortName?: string;
  baudRate: number;
  dataBits: number;
  stopBits: number;
  parity: string;
  readTimeoutMs?: number;
  autoConnect?: boolean;
}

export interface DetectionSettings extends DetectionSessionRequest {}

export interface DetectionSessionResponse {
  active: boolean;
  sessionId?: string;
  portName?: string;
  rxPortName?: string;
  txPortName?: string;
  baudRate?: number;
  dataBits?: number;
  stopBits?: number;
  parity?: string;
  startedAt?: string;
  state?: "inactive" | "connecting" | "connected" | "reconnecting";
  autoReconnect?: boolean;
  lastError?: string;
  retryCount?: number;
  message: string;
}

export interface GPSSessionRequest {
  portName?: string;
  dataPortName?: string;
  controlPortName?: string;
  baudRate: number;
  dataBits: number;
  stopBits: number;
  parity: string;
  readTimeoutMs?: number;
  autoConnect?: boolean;
}

export interface GPSSettings extends GPSSessionRequest {}

export interface GPSFix {
  latitude: number;
  longitude: number;
  altitudeM?: number;
  speedKnots?: number;
  courseDegree?: number;
  fixQuality?: number;
  satellites?: number;
  valid: boolean;
}

export interface GPSRecord {
  sessionId: string;
  portName: string;
  receivedAt: string;
  type: string;
  raw: string;
  fix?: GPSFix;
}

export interface GeoPoint {
  latitude: number;
  longitude: number;
}

export interface UserSettings {
  manualDeviceLocation?: GeoPoint;
}

export interface ScreenDeviceLocationResponse {
  source: "gps" | "manual" | "none";
  point?: GeoPoint;
  updatedAt?: string;
  valid: boolean;
}

export interface GPSSessionResponse {
  active: boolean;
  sessionId?: string;
  portName?: string;
  dataPortName?: string;
  controlPortName?: string;
  baudRate?: number;
  dataBits?: number;
  stopBits?: number;
  parity?: string;
  startedAt?: string;
  state?: "inactive" | "connecting" | "connected" | "reconnecting";
  autoReconnect?: boolean;
  lastError?: string;
  retryCount?: number;
  lastNmea?: string;
  lastFix?: GPSFix;
  lastRecord?: GPSRecord;
  message: string;
}

export interface ParsedMessage {
  type: ParsedMessageType | string;
  time: string;
  raw: string;
  data: unknown;
}

export interface DetectionRecord {
  id: string;
  sessionId: string;
  portName: string;
  kind: string;
  receivedAt: string;
  device?: string;
  model?: string;
  frequency?: number;
  rssi?: number;
  summary: string;
  parsed: ParsedMessage;
}

export interface ScreenDetectionLastRecord {
  id: string;
  kind: string;
  receivedAt: string;
  device?: string;
  model?: string;
  frequency?: number;
  rssi?: number;
  summary: string;
}

export interface ScreenDetectionTarget {
  id: string;
  model: string;
  frequency: number;
  rssi: number;
  devices: string[];
  firstSeen: string;
  lastSeen: string;
  hitCount: number;
  lastRecord: ScreenDetectionLastRecord;
}

export interface ScreenPositionPoint {
  latitude: number;
  longitude: number;
}

export interface ScreenPositionLastRecord {
  type: string;
  receivedAt: string;
  device?: string;
  serial?: string;
  model?: string;
  frequency?: number;
  rssi?: number;
  cracked?: boolean;
}

export interface ScreenPositionTarget {
  id: string;
  serial: string;
  model: string;
  source: string;
  frequency?: number;
  rssi?: number;
  devices: string[];
  drone?: ScreenPositionPoint;
  pilot?: ScreenPositionPoint;
  home?: ScreenPositionPoint;
  height?: number;
  altitude?: number;
  speed?: number;
  cracked?: boolean;
  firstSeen: string;
  lastSeen: string;
  hitCount: number;
  lastRecord: ScreenPositionLastRecord;
}

export type DebugRecord = DetectionRecord | ParsedMessage;

export interface GpioChannel {
  id: string;
  label: string;
  pin: number;
  bands: string[] | null;
  reserved: boolean;
  enabled: boolean;
  actualLevel: string;
  desiredLevel: string;
  status: string;
  lastError?: string;
}

export interface GpioChannelStateRequest {
  enabled: boolean;
}

export interface GpioChannelStateResponse {
  channel: GpioChannel;
  message: string;
}

export interface ListResponse<T> {
  items: T[];
  count: number;
}

export interface PortsResponse {
  ports: PortInfo[];
  activeSession: DetectionSessionResponse;
}

export interface ChannelsResponse {
  channels: GpioChannel[];
  count: number;
}

export interface DeveloperLoginRequest {
  code: string;
}

export interface DeveloperSessionResponse {
  token: string;
  expiresAt: number;
  message: string;
}

export interface NetworkAddress {
  address: string;
  prefix: number;
}

export interface NetworkInterface {
  name: string;
  type: string;
  state: string;
  connectionName?: string;
  hardwareAddress?: string;
  mtu?: number;
  ipv4: NetworkAddress[];
  ipv6: NetworkAddress[];
  gateway4?: string;
  gateway6?: string;
  dns4: string[];
  dns6: string[];
  ipv4Method: string;
  routeMetric?: number;
  managed: boolean;
}

export interface NetworkInterfacesResponse {
  interfaces: NetworkInterface[];
  count: number;
  backend: string;
  available: boolean;
  readOnly: boolean;
  message?: string;
}

export interface NetworkInterfaceUpdateRequest {
  mode: "dhcp" | "static";
  ipv4Address?: string;
  prefix?: number;
  gateway4?: string;
  dns4?: string[];
  routeMetric?: number;
}

export interface NetworkInterfaceUpdateResponse {
  interface: NetworkInterface;
  message: string;
}

export interface NetworkPriorityBatchItem {
  interfaceName: string;
  routeMetric: number;
}

export interface NetworkPriorityBatchRequest {
  priorities: NetworkPriorityBatchItem[];
}

export interface NetworkPriorityBatchResponse {
  interfaces: NetworkInterface[];
  message: string;
}

export interface WiFiNetwork {
  ssid: string;
  bssid?: string;
  device?: string;
  mode?: string;
  channel?: string;
  rate?: string;
  signal: number;
  security?: string;
  active: boolean;
}

export interface WiFiNetworksResponse {
  networks: WiFiNetwork[];
  count: number;
  available: boolean;
  readOnly: boolean;
  message?: string;
}

export interface WiFiConnectRequest {
  ssid: string;
  password?: string;
  device?: string;
}

export interface WiFiConnectResponse {
  message: string;
}

export interface ApiErrorPayload {
  code: string;
  message: string;
  details?: unknown;
}

export interface EventMessage<T = unknown> {
  type: string;
  time: string;
  payload?: T;
}

export interface StreamHandlers {
  onSessionStarted?: (event: EventMessage<DetectionSessionResponse>) => void;
  onSessionStopped?: (event: EventMessage<DetectionSessionResponse>) => void;
  onSessionState?: (event: EventMessage<DetectionSessionResponse>) => void;
  onGPSSessionStarted?: (event: EventMessage<GPSSessionResponse>) => void;
  onGPSSessionStopped?: (event: EventMessage<GPSSessionResponse>) => void;
  onGPSSessionState?: (event: EventMessage<GPSSessionResponse>) => void;
  onGPSRecord?: (event: EventMessage<GPSRecord>) => void;
  onParsed?: (event: EventMessage<ParsedMessage>) => void;
  onDetection?: (event: EventMessage<DetectionRecord>) => void;
  onChannelUpdated?: (event: EventMessage<GpioChannel>) => void;
  onError?: (error: Error) => void;
}

export interface ScreenStreamHandlers {
  onDetectionUpdated?: (event: EventMessage<ScreenDetectionTarget>) => void;
  onPositionUpdated?: (event: EventMessage<ScreenPositionTarget>) => void;
  onError?: (error: Error) => void;
}
