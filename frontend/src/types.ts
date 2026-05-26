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

export interface DeceptionSessionRequest {
  portName?: string;
  baudRate: number;
  dataBits: number;
  stopBits: number;
  parity: string;
  readTimeoutMs?: number;
  autoConnect?: boolean;
}

export interface DeceptionSettings extends DeceptionSessionRequest {}

export interface DeceptionSessionResponse {
  active: boolean;
  sessionId?: string;
  portName?: string;
  baudRate?: number;
  dataBits?: number;
  stopBits?: number;
  parity?: string;
  startedAt?: string;
  state?: "inactive" | "connecting" | "connected";
  autoReconnect?: boolean;
  lastError?: string;
  message: string;
}

export interface DeceptionQueryResponse {
  item: string;
  command: string;
  rawHex?: string;
  description?: string;
  message: string;
}

export type DeceptionReportStatus = "running" | "completed" | "failed" | "abnormal";
export type InterferenceReportStatus = "running" | "completed" | "failed" | "abnormal";

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

export interface CompassSessionRequest {
  portName?: string;
  baudRate: number;
  dataBits: number;
  stopBits: number;
  parity: string;
  readTimeoutMs?: number;
  autoConnect?: boolean;
}

export interface CompassSettings extends CompassSessionRequest {}

export interface CompassRecord {
  sessionId: string;
  portName: string;
  receivedAt: string;
  pitch: number;
  roll: number;
  heading: number;
  rawHex?: string;
}

export interface CompassSessionResponse {
  active: boolean;
  sessionId?: string;
  portName?: string;
  baudRate?: number;
  dataBits?: number;
  stopBits?: number;
  parity?: string;
  startedAt?: string;
  state?: "inactive" | "connecting" | "connected" | "reconnecting";
  autoReconnect?: boolean;
  lastError?: string;
  retryCount?: number;
  lastRecord?: CompassRecord;
  lastPitch?: number;
  lastRoll?: number;
  lastHeading?: number;
  lastRawHex?: string;
  lastUpdatedAt?: string;
  autoOutput: boolean;
  autoOutputRate?: number;
  message: string;
}

export interface GeoPoint {
  latitude: number;
  longitude: number;
}

export interface UserSettings {
  deviceSn?: string;
  deviceHardwareId?: string;
  manualDeviceLocation?: GeoPoint;
  screenStrikeChannelLabels?: string[];
  intrusionRetentionDays?: number;
  whitelist?: WhitelistItem[];
  screenAlarmSettings?: ScreenAlarmSettings;
}

export interface WhitelistItem {
  serial: string;
  model?: string;
  source?: string;
  createdAt?: string;
}

export interface ScreenAlarmSettings {
  detection: boolean;
  position: boolean;
  fpv: boolean;
  sound: boolean;
}

export interface ScreenDeviceLocationResponse {
  source: "gps" | "manual" | "none";
  point?: GeoPoint;
  updatedAt?: string;
  valid: boolean;
}

export interface ScreenSerialCapabilityStatus {
  configured: boolean;
  active: boolean;
  state?: "unconfigured" | "inactive" | "connecting" | "connected" | "reconnecting";
  portName?: string;
  rxPortName?: string;
  txPortName?: string;
  lastError?: string;
  headingDeg?: number;
  headingUpdatedAt?: string;
}

export interface ScreenRuntimeStatus {
  detection: ScreenSerialCapabilityStatus;
  deception: ScreenSerialCapabilityStatus;
  compass: ScreenSerialCapabilityStatus;
}

export interface ScreenStrikeChannel {
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

export interface ScreenStrikeState {
  active: boolean;
  channelIds: string[];
  durationSeconds: number;
  remainingSeconds: number;
  startedAt?: string;
  endsAt?: string;
  channels: ScreenStrikeChannel[];
}

export interface ScreenStrikeResponse {
  state: ScreenStrikeState;
  message: string;
}

export interface ScreenStrikeRequest {
  enabled: boolean;
  channelIds: string[];
  durationSeconds: number;
}

export interface ScreenDeceptionRequest {
  enabled: boolean;
  targetId?: string;
  mode?: ScreenDeceptionMode;
  longitude?: number;
  latitude?: number;
  altitudeM?: number;
  signalMask?: number;
  strengthPreset?: ScreenDeceptionStrengthPreset;
  attenuationDB?: number;
  delayMode?: ScreenDeceptionDelayMode;
  delayNS?: number;
  circle?: ScreenDeceptionCircleParams;
  linear?: ScreenDeceptionLinearParams;
  random?: ScreenDeceptionRandomParams;
}

export type ScreenDeceptionMode =
  | "fixed_point"
  | "circle"
  | "linear";

export type ScreenDeceptionStrengthPreset = "strong" | "standard" | "weak" | "custom";

export type ScreenDeceptionDelayMode = "auto" | "manual" | "off";

export interface ScreenDeceptionCircleParams {
  radiusM?: number;
  periodSeconds?: number;
  direction?: "cw" | "ccw";
}

export interface ScreenDeceptionLinearParams {
  speedMps?: number;
  directionDeg?: number;
  maxSpeedMps?: number;
}

export interface ScreenDeceptionRandomParams {
  enabled: boolean;
  radiusM?: number;
  refreshSeconds?: number;
}

export interface ScreenDeceptionState {
  active: boolean;
  targetId?: string;
  mode?: ScreenDeceptionMode;
  point?: GeoPoint;
  altitudeM?: number;
  signalMask?: number;
  strengthPreset?: ScreenDeceptionStrengthPreset;
  attenuationDB?: number;
  delayMode?: ScreenDeceptionDelayMode;
  delayNS?: number;
  distanceM?: number;
  summary?: string;
  unsupportedReason?: string;
  circle?: ScreenDeceptionCircleParams;
  linear?: ScreenDeceptionLinearParams;
  random?: ScreenDeceptionRandomParams;
  serialActive: boolean;
  lastAck?: string;
  lastError?: string;
}

export interface ScreenDeceptionStatusPoint {
  latitude: number;
  longitude: number;
  altitudeM: number;
}

export interface ScreenDeceptionVersionStatus {
  software?: string;
  fpga?: string;
  protocol?: string;
}

export interface ScreenDeceptionTargetStatus {
  distanceM: number;
  heightM: number;
  directionDeg: number;
  headingDeg: number;
}

export interface ScreenDeceptionSpoofCircleStatus {
  distanceM: number;
  heightM: number;
  directionDeg: number;
  headingDeg: number;
  radiusM: number;
  periodSeconds: number;
  direction?: "cw" | "ccw" | "unknown";
}

export interface ScreenDeceptionSuppressionStatus {
  waveformMask: number;
  transmitOn: boolean;
}

export interface ScreenDeceptionRandomStatus {
  enabled: boolean;
  radiusM: number;
  refreshSeconds: number;
}

export interface ScreenDeceptionSyncStatus {
  receiverWorking: boolean;
  receiverPositioned: boolean;
  leapSecondValid: boolean;
  timeSynced: boolean;
  antennaOk: boolean;
}

export interface ScreenDeceptionMotionStatus {
  maxSpeedMps?: number;
  initialSpeedMps?: number;
  initialDirectionDeg?: number;
  accelerationMps2?: number;
  accelerationDirectionDeg?: number;
  circleRadiusM?: number;
  circlePeriodSeconds?: number;
  circleDirection?: "cw" | "ccw" | "unknown";
}

export interface ScreenDeceptionAttenuationStatus {
  gps: number;
  bds: number;
  glo: number;
  gal: number;
}

export interface ScreenDeceptionDelayStatus {
  gps?: number;
  bds?: number;
  glo?: number;
  gal?: number;
}

export interface ScreenDeceptionSignalWorkStatus {
  clockOk: boolean;
  ephemerisValid: boolean;
  rfModuleOk: boolean;
  signalTransmit: boolean;
  transmitChannel: boolean;
  fpgaOk: boolean;
  raw: number;
}

export interface ScreenDeceptionDeviceSignalStatus {
  systemTime?: string;
  signalMask: number;
  signalNames?: string[];
  delayNs: number;
  workStatus: ScreenDeceptionSignalWorkStatus;
  transmitSwitch: boolean;
  attenuationDb: number;
  receivedSatelliteCount: number;
  receivedPrns?: number[];
  receivedCn0?: number[];
  transmittedCount: number;
  transmittedPrns?: number[];
}

export interface ScreenDeceptionDeviceStatus {
  serialActive: boolean;
  updatedAt?: string;
  systemTime?: string;
  reportedSystemTime?: string;
  version?: ScreenDeceptionVersionStatus;
  transmitMask?: number;
  transmitSignals?: string[];
  amplifierOn?: boolean;
  autoTransmit?: boolean;
  firstTimeSynced?: boolean;
  oscillatorState?: "warming" | "unlocked" | "tracking" | "locked" | "hold" | "unknown";
  syncStatus?: ScreenDeceptionSyncStatus;
  currentPosition?: ScreenDeceptionStatusPoint;
  simulatedPosition?: ScreenDeceptionStatusPoint;
  queriedDevicePosition?: ScreenDeceptionStatusPoint;
  queriedSimulatedPosition?: ScreenDeceptionStatusPoint;
  targetPosition?: ScreenDeceptionTargetStatus;
  temperatureC?: number;
  timePrecisionNs?: number;
  uptimeSeconds?: number;
  motion?: ScreenDeceptionMotionStatus;
  attenuation?: ScreenDeceptionAttenuationStatus;
  delayNS?: number;
  delayBySignalNs?: ScreenDeceptionDelayStatus;
  spoofCircle?: ScreenDeceptionSpoofCircleStatus;
  suppression?: ScreenDeceptionSuppressionStatus;
  random?: ScreenDeceptionRandomStatus;
  timedSearch?: boolean;
  deviceSignal?: ScreenDeceptionDeviceSignalStatus;
  deviceSignals?: ScreenDeceptionDeviceSignalStatus[];
  rawDescriptions: Record<string, string>;
  queryErrors?: Record<string, string>;
  lastError?: string;
}

export interface ScreenDeceptionResponse {
  state: ScreenDeceptionState;
  message: string;
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
  device: string;
  model?: string;
  displayModel?: string;
  frequency?: number;
  rssi?: number;
  summary: string;
  parsed: ParsedMessage;
}

export interface ScreenDetectionLastRecord {
  id: string;
  kind: string;
  receivedAt: string;
  device: string;
  model?: string;
  displayModel?: string;
  frequency?: number;
  rssi?: number;
  summary: string;
}

export interface ScreenDetectionTarget {
  id: string;
  serial?: string;
  model: string;
  displayModel?: string;
  frequency: number;
  rssi: number;
  device?: string;
  firstSeen: string;
  lastSeen: string;
  hitCount: number;
  lastRecord: ScreenDetectionLastRecord;
}

export interface ScreenPositionPoint {
  latitude: number;
  longitude: number;
}

export interface ScreenPositionTrackPoint extends ScreenPositionPoint {
  speed?: number;
  height?: number;
  time: string;
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
  correlationId?: string;
  serial: string;
  model: string;
  source: string;
  sources?: string[];
  frequency?: number;
  rssi?: number;
  device?: string;
  drone?: ScreenPositionPoint;
  pilot?: ScreenPositionPoint;
  home?: ScreenPositionPoint;
  droneTrajectory?: ScreenPositionTrackPoint[];
  pilotTrajectory?: ScreenPositionTrackPoint[];
  height?: number;
  altitude?: number;
  speed?: number;
  pilotDistanceM?: number;
  droneDistanceM?: number;
  droneDirectionDeg?: number;
  deviceDirectionDeg?: number;
  cracked?: boolean;
  firstSeen: string;
  lastSeen: string;
  hitCount: number;
  lastRecord: ScreenPositionLastRecord;
}

export type IntrusionTargetType = "detection" | "position";

export interface IntrusionRecord {
  id: string;
  targetId: string;
  targetType: IntrusionTargetType;
  model?: string;
  displayModel?: string;
  serial?: string;
  device?: string;
  frequency?: number;
  rssi?: number;
  firstSeen: string;
  lastSeen: string;
  durationSeconds: number;
  hitCount: number;
  source?: string;
  sources?: string[];
  cracked?: boolean;
  deviceLocation?: ScreenDeviceLocationResponse;
  drone?: ScreenPositionPoint;
  pilot?: ScreenPositionPoint;
  home?: ScreenPositionPoint;
  droneTrajectory?: ScreenPositionTrackPoint[];
  pilotTrajectory?: ScreenPositionTrackPoint[];
  height?: number;
  altitude?: number;
  speed?: number;
  pilotDistanceM?: number;
  droneDistanceM?: number;
  droneDirectionDeg?: number;
  deviceDirectionDeg?: number;
  lastRecord?: unknown;
  archivedAt: string;
}

export interface IntrusionDeleteRequest {
  ids: string[];
}

export interface IntrusionDeleteResponse {
  deleted: number;
}

export interface DeceptionReportDeleteResponse {
  deleted: number;
}

export interface InterferenceReportDeleteResponse {
  deleted: number;
}

export interface DeceptionRecord {
  time: string;
  direction: string;
  command?: string;
  control?: string;
  rawHex?: string;
  description?: string;
  error?: string;
}

export interface DeceptionReportSummary {
  id: string;
  status: DeceptionReportStatus;
  startedAt: string;
  endedAt?: string;
  durationSeconds: number;
  targetId?: string;
  mode?: ScreenDeceptionMode | string;
  point?: GeoPoint;
  altitudeM?: number;
  signalMask?: number;
  signalNames?: string[];
  strengthPreset?: ScreenDeceptionStrengthPreset | string;
  attenuationDB?: number;
  delayMode?: ScreenDeceptionDelayMode | string;
  delayNS?: number;
  portName?: string;
  summary?: string;
  lastError?: string;
  abnormalReason?: string;
  createdAt: string;
  updatedAt: string;
}

export interface DeceptionReport extends DeceptionReportSummary {
  request: ScreenDeceptionRequest;
  session: DeceptionSessionResponse;
  startState?: ScreenDeceptionState;
  endState?: ScreenDeceptionState;
  startDeviceStatus?: ScreenDeceptionDeviceStatus;
  beforeStopStatus?: ScreenDeceptionDeviceStatus;
  afterStopStatus?: ScreenDeceptionDeviceStatus;
  rawDescriptions?: Record<string, string>;
  queryErrors?: Record<string, string>;
  records?: DeceptionRecord[];
  recordCount: number;
}

export interface InterferenceReportSummary {
  id: string;
  status: InterferenceReportStatus;
  startedAt: string;
  endedAt?: string;
  durationSeconds: number;
  requestedDurationSeconds?: number;
  channelIds?: string[];
  channelLabels?: string[];
  channelPins?: number[];
  summary?: string;
  lastError?: string;
  abnormalReason?: string;
  createdAt: string;
  updatedAt: string;
}

export interface InterferenceReport extends InterferenceReportSummary {
  request: ScreenStrikeRequest;
  startState?: ScreenStrikeState;
  endState?: ScreenStrikeState;
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
  hasMore?: boolean;
  nextOffset?: number;
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
  onDeceptionSessionStarted?: (event: EventMessage<DeceptionSessionResponse>) => void;
  onDeceptionSessionStopped?: (event: EventMessage<DeceptionSessionResponse>) => void;
  onDeceptionSessionState?: (event: EventMessage<DeceptionSessionResponse>) => void;
  onCompassSessionStarted?: (event: EventMessage<CompassSessionResponse>) => void;
  onCompassSessionStopped?: (event: EventMessage<CompassSessionResponse>) => void;
  onCompassSessionState?: (event: EventMessage<CompassSessionResponse>) => void;
  onCompassRecord?: (event: EventMessage<CompassRecord>) => void;
  onGPSRecord?: (event: EventMessage<GPSRecord>) => void;
  onParsed?: (event: EventMessage<ParsedMessage>) => void;
  onDetection?: (event: EventMessage<DetectionRecord>) => void;
  onChannelUpdated?: (event: EventMessage<GpioChannel>) => void;
  onError?: (error: Error) => void;
}

export interface ScreenStreamHandlers {
  onDetectionUpdated?: (event: EventMessage<ScreenDetectionTarget>) => void;
  onPositionUpdated?: (event: EventMessage<ScreenPositionTarget>) => void;
  onStrikeUpdated?: (event: EventMessage<ScreenStrikeState>) => void;
  onDeceptionUpdated?: (event: EventMessage<ScreenDeceptionState>) => void;
  onCompassRecord?: (event: EventMessage<CompassRecord>) => void;
  onError?: (error: Error) => void;
}
