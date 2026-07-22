import type {
  DeceptionReportDetail,
  DeceptionReportQuery,
  DeployRequest,
  ExportResult,
  InterferenceReportQuery,
  InterferenceReportSummary,
  IntrusionQuery,
  IntrusionRecord,
  LicenseUploadRequest,
  OfflineMapUploadRequest,
  ProgressEvent,
  RemoteEntry,
  RemoteProbe,
  SavedConfig,
  SSHConnectRequest,
  SSHStatus,
  TimeInfo,
} from "./types";

type AppBridge = {
  LoadConfig(): Promise<SavedConfig>;
  SaveConfig(config: SavedConfig): Promise<void>;
  ConnectSSH(request: SSHConnectRequest): Promise<SSHStatus>;
  ReconnectSSH(): Promise<SSHStatus>;
  DisconnectSSH(): Promise<void>;
  GetSSHStatus(): Promise<SSHStatus>;
  GetTimeInfo(): Promise<TimeInfo>;
  SetTimezone(timezone: string): Promise<void>;
  SetNTPEnabled(enabled: boolean): Promise<void>;
  SetManualTime(datetime: string): Promise<void>;
  ProbeRemote(installDir: string): Promise<RemoteProbe>;
  BrowseRemoteDir(path: string): Promise<RemoteEntry[]>;
  StopAllServices(installDir: string): Promise<string>;
  SelectFirmwarePackage(): Promise<string>;
  DeployDR600AB(request: DeployRequest): Promise<{ installDir: string; message: string }>;
  ListIntrusions(query: IntrusionQuery): Promise<IntrusionRecord[]>;
  ExportIntrusionsCSV(query: IntrusionQuery): Promise<ExportResult>;
  ListInterferenceReports(query: InterferenceReportQuery): Promise<InterferenceReportSummary[]>;
  ExportInterferenceReportsCSV(query: InterferenceReportQuery): Promise<ExportResult>;
  ListDeceptionReports(query: DeceptionReportQuery): Promise<import("./types").DeceptionReportSummary[]>;
  GetDeceptionReport(id: string, installDir: string): Promise<DeceptionReportDetail>;
  ExportDeceptionReportsCSV(query: DeceptionReportQuery): Promise<ExportResult>;
  SelectOfflineMapPackage(): Promise<string>;
  UploadOfflineMap(request: OfflineMapUploadRequest): Promise<{ installDir: string; tileCount: number; message: string }>;
  CleanupOfflineMapBackup(request: { installDir: string }): Promise<string>;
  SelectLicenseFile(): Promise<string>;
  UploadLicense(request: LicenseUploadRequest): Promise<{ message: string }>;
};

declare global {
  interface Window {
    go?: {
      main?: {
        App?: AppBridge;
      };
    };
    runtime?: {
      EventsOn?: (name: string, callback: (data: unknown) => void) => (() => void) | void;
      EventsOff?: (name: string) => void;
    };
  }
}

function bridge(): AppBridge {
  const app = window.go?.main?.App;
  if (!app) {
    throw new Error("Wails 运行时未就绪");
  }
  return app;
}

export const api = {
  loadConfig: () => bridge().LoadConfig(),
  saveConfig: (config: SavedConfig) => bridge().SaveConfig(config),
  connectSSH: (request: SSHConnectRequest) => bridge().ConnectSSH(request),
  reconnectSSH: () => bridge().ReconnectSSH(),
  disconnectSSH: () => bridge().DisconnectSSH(),
  getSSHStatus: () => bridge().GetSSHStatus(),
  getTimeInfo: () => bridge().GetTimeInfo(),
  setTimezone: (timezone: string) => bridge().SetTimezone(timezone),
  setNTPEnabled: (enabled: boolean) => bridge().SetNTPEnabled(enabled),
  setManualTime: (datetime: string) => bridge().SetManualTime(datetime),
  probeRemote: (installDir: string) => bridge().ProbeRemote(installDir),
  browseRemoteDir: (path: string) => bridge().BrowseRemoteDir(path),
  stopAllServices: (installDir: string) => bridge().StopAllServices(installDir),
  selectFirmwarePackage: () => bridge().SelectFirmwarePackage(),
  deployDR600AB: (request: DeployRequest) => bridge().DeployDR600AB(request),
  listIntrusions: (query: IntrusionQuery) => bridge().ListIntrusions(query),
  exportIntrusionsCSV: (query: IntrusionQuery) => bridge().ExportIntrusionsCSV(query),
  listInterferenceReports: (query: InterferenceReportQuery) => bridge().ListInterferenceReports(query),
  exportInterferenceReportsCSV: (query: InterferenceReportQuery) => bridge().ExportInterferenceReportsCSV(query),
  listDeceptionReports: (query: DeceptionReportQuery) => bridge().ListDeceptionReports(query),
  getDeceptionReport: (id: string, installDir: string) => bridge().GetDeceptionReport(id, installDir),
  exportDeceptionReportsCSV: (query: DeceptionReportQuery) => bridge().ExportDeceptionReportsCSV(query),
  selectOfflineMapPackage: () => bridge().SelectOfflineMapPackage(),
  uploadOfflineMap: (request: OfflineMapUploadRequest) => bridge().UploadOfflineMap(request),
  cleanupOfflineMapBackup: (installDir: string) => bridge().CleanupOfflineMapBackup({ installDir }),
  selectLicenseFile: () => bridge().SelectLicenseFile(),
  uploadLicense: (request: LicenseUploadRequest) => bridge().UploadLicense(request),
};

export function onProgress(name: string, callback: (event: ProgressEvent) => void) {
  const off = window.runtime?.EventsOn?.(name, (data) => callback(data as ProgressEvent));
  if (typeof off === "function") {
    return off;
  }
  return () => window.runtime?.EventsOff?.(name);
}
