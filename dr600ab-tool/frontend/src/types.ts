export type Page = "deploy" | "intrusions" | "interference-reports" | "deception-reports" | "offline-map";

export interface SavedConfig {
  ssh?: {
    host: string;
    port: number;
    user: string;
    rememberPassword?: boolean;
    password?: string;
  };
  installDir?: string;
  firmware?: string;
  mapPackage?: string;
}

export interface SSHConnectRequest {
  host: string;
  port: number;
  user: string;
  password: string;
  rememberPassword?: boolean;
}

export interface SSHStatus {
  connected: boolean;
  host?: string;
  port?: number;
  user?: string;
  message: string;
}

export interface RemoteProbe {
  installDir: string;
  serviceActive: boolean;
  serviceStatus: string;
  kioskActive: boolean;
  kioskStatus: string;
  hasSystemd: boolean;
  hasTar: boolean;
  hasUnzip: boolean;
  hasBusybox: boolean;
  chromiumPath?: string;
  intrusionDbPath?: string;
  interferenceDbPath?: string;
  deceptionDbPath?: string;
  offlineMapExists: boolean;
  warnings?: string[];
}

export interface RemoteEntry {
  name: string;
  path: string;
  isDir: boolean;
  size: number;
}

export interface ProgressEvent {
  step: number;
  stepName: string;
  message: string;
  status: "running" | "success" | "error" | string;
  progress: number;
  errorDetail?: string;
}

export interface DeployRequest {
  installDir: string;
  firmwarePath: string;
  fullUpdate: boolean;
}

export interface IntrusionQuery {
  installDir: string;
  targetType?: string;
  dateFrom?: string;
  dateTo?: string;
  search?: string;
  limit?: number;
  locale?: string;
}

export interface GeoPoint {
  latitude: number;
  longitude: number;
}

export interface DeviceLocation {
  source: string;
  point?: GeoPoint;
  updatedAt?: string;
  valid: boolean;
}

export interface TrackPoint {
  latitude: number;
  longitude: number;
  speed?: number;
  height?: number;
  time: string;
}

export interface IntrusionRecord {
  id: string;
  targetId: string;
  targetType: string;
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
  drone?: GeoPoint;
  pilot?: GeoPoint;
  home?: GeoPoint;
  deviceLocation?: DeviceLocation;
  droneTrajectory?: TrackPoint[];
  pilotTrajectory?: TrackPoint[];
  pilotDistanceM?: number;
  droneDistanceM?: number;
  droneDirectionDeg?: number;
  deviceDirectionDeg?: number;
  height?: number;
  altitude?: number;
  speed?: number;
  lastRecordJson?: string;
  archivedAt: string;
}

export interface DeceptionReportQuery {
  installDir: string;
  status?: string;
  mode?: string;
  dateFrom?: string;
  dateTo?: string;
  limit?: number;
  locale?: string;
}

export interface DeceptionReportSummary {
  id: string;
  status: string;
  startedAt: string;
  endedAt?: string;
  durationSeconds: number;
  targetId?: string;
  mode?: string;
  point?: GeoPoint;
  altitudeM?: number;
  signalMask?: number;
  signalNames?: string[];
  strengthPreset?: string;
  attenuationDB?: number;
  delayMode?: string;
  delayNS?: number;
  portName?: string;
  summary?: string;
  lastError?: string;
  abnormalReason?: string;
  recordCount?: number;
}

export interface DeceptionReportDetail extends DeceptionReportSummary {
  requestJson?: string;
  sessionJson?: string;
  recordsJson?: string;
}

export interface InterferenceReportQuery {
  installDir: string;
  status?: string;
  dateFrom?: string;
  dateTo?: string;
  limit?: number;
  locale?: string;
}

export interface InterferenceReportSummary {
  id: string;
  status: string;
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

export interface OfflineMapUploadRequest {
  installDir: string;
  packagePath: string;
  keepBackup: boolean;
}

export interface ExportResult {
  path: string;
  count: number;
}
