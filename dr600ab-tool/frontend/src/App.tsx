import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import type { ReactNode } from "react";
import {
  Cable,
  CheckCircle2,
  Download,
  FileArchive,
  FolderOpen,
  HardDriveUpload,
  Map,
  RefreshCw,
  Server,
  ShieldAlert,
  ShieldOff,
  Target,
  UploadCloud,
  XCircle,
} from "lucide-react";

import type {
  DeceptionReportSummary,
  InterferenceReportSummary,
  IntrusionRecord,
  Page,
  ProgressEvent,
  RemoteEntry,
  RemoteProbe,
  SavedConfig,
  SSHStatus,
} from "./types";
import { api, onProgress } from "./wails";

const pageItems: Array<{ id: Page; label: string; icon: typeof Server }> = [
  { id: "deploy", label: "部署更新", icon: HardDriveUpload },
  { id: "intrusions", label: "目标入侵", icon: ShieldAlert },
  { id: "interference-reports", label: "干扰报告", icon: ShieldOff },
  { id: "deception-reports", label: "诱骗报告", icon: Target },
  { id: "offline-map", label: "离线地图", icon: Map },
];

const uiLocale = "zh-CN";

type Notice = { tone: "idle" | "success" | "error" | "loading"; message: string };
type SSHForm = { host: string; port: number; user: string; password: string; rememberPassword: boolean };
type ProbeOptions = { auto?: boolean; installDir?: string };

function messageOf(error: unknown) {
  return error instanceof Error ? error.message : String(error);
}

function formatTime(value?: string) {
  if (!value) return "-";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return date.toLocaleString("zh-CN", { hour12: false });
}

function formatDuration(seconds: number) {
  if (!Number.isFinite(seconds) || seconds <= 0) return "-";
  const minutes = Math.floor(seconds / 60);
  const rest = Math.round(seconds % 60);
  return minutes > 0 ? `${minutes} 分 ${rest} 秒` : `${rest} 秒`;
}

function formatNumber(value?: number, digits = 1) {
  if (typeof value !== "number" || !Number.isFinite(value)) return "-";
  return value.toFixed(digits).replace(/\.0+$/, "");
}

function formatPoint(point?: { latitude: number; longitude: number }) {
  if (!point) return "-";
  return `${point.latitude.toFixed(6)}, ${point.longitude.toFixed(6)}`;
}

function formatOptionalNumber(value?: number, suffix = "", digits = 1) {
  if (typeof value !== "number" || !Number.isFinite(value)) return "-";
  return `${formatNumber(value, digits)}${suffix}`;
}

function formatFrequency(value?: number) {
  return typeof value === "number" && Number.isFinite(value) && value !== 0 ? `${formatNumber(value)} MHz` : "-";
}

function formatRSSI(value?: number) {
  return typeof value === "number" && Number.isFinite(value) && value !== 0 ? `${formatNumber(value, 0)} dBm` : "-";
}

function formatDistance(value?: number) {
  if (typeof value !== "number" || !Number.isFinite(value)) return "-";
  if (Math.abs(value) >= 1000) {
    return `${formatNumber(value / 1000, value >= 100_000 ? 0 : 1)} km`;
  }
  return `${formatNumber(value, 0)} m`;
}

function formatChannelLabels(report: InterferenceReportSummary) {
  if (report.channelLabels?.length) return report.channelLabels.join("，");
  if (report.channelIds?.length) return report.channelIds.join("，");
  if (report.channelPins?.length) return report.channelPins.map((pin) => `IO${pin}`).join("，");
  return "-";
}

function statusLightTone(status: string) {
  switch (status) {
    case "running":
      return "running";
    case "completed":
      return "completed";
    case "failed":
      return "failed";
    case "abnormal":
      return "abnormal";
    default:
      return "idle";
  }
}

function StatusLight({ label, status }: { label: string; status: string }) {
  return (
    <span className={`status-light ${statusLightTone(status)}`}>
      <span aria-hidden="true" />
      {label}
    </span>
  );
}

function coordinateSummary(record: IntrusionRecord) {
  const parts: string[] = [];
  if (record.deviceLocation?.point) {
    parts.push(`设备: ${formatPoint(record.deviceLocation.point)}`);
  }
  if (record.targetType === "position") {
    if (record.drone) parts.push(`无人机: ${formatPoint(record.drone)}`);
    if (record.pilot) parts.push(`飞手: ${formatPoint(record.pilot)}`);
    if (record.home) parts.push(`返航点: ${formatPoint(record.home)}`);
  }
  return parts.length ? parts.join(" / ") : "-";
}

function statusLabel(status: string) {
  switch (status) {
    case "running":
      return "运行中";
    case "completed":
      return "已完成";
    case "failed":
      return "启动失败";
    case "abnormal":
      return "异常闭合";
    default:
      return status || "-";
  }
}

function modeLabel(mode?: string) {
  switch (mode) {
    case "fixed_point":
      return "定点诱骗";
    case "circle":
      return "圆周诱骗";
    case "linear":
      return "线性诱骗";
    default:
      return mode || "-";
  }
}

function appendProgress(setter: (fn: (items: ProgressEvent[]) => ProgressEvent[]) => void, event: ProgressEvent) {
  setter((items) => {
    const withCompletedPrevious = items.map((item) => {
      if (item.step < event.step && item.status === "running") {
        return { ...item, status: "success", progress: 100 };
      }
      return item;
    });
    const index = withCompletedPrevious.findIndex((item) => item.step === event.step);
    if (index === -1) {
      return [...withCompletedPrevious, event].slice(-12);
    }
    const next = [...withCompletedPrevious];
    next[index] = event;
    return next;
  });
}

function parentRemoteDir(path: string) {
  const clean = path.trim().replace(/\/+$/, "") || "/";
  if (clean === "/") return "/";
  const index = clean.lastIndexOf("/");
  return index <= 0 ? "/" : clean.slice(0, index);
}

function ProgressList({ items }: { items: ProgressEvent[] }) {
  if (items.length === 0) {
    return <div className="empty compact">暂无进度</div>;
  }
  return (
    <div className="progress-list">
      {items.map((item) => (
        <div className={`progress-row ${item.status}`} key={item.step}>
          <div className="progress-row__icon">
            {item.status === "success" ? <CheckCircle2 size={16} /> : item.status === "error" ? <XCircle size={16} /> : <RefreshCw size={16} />}
          </div>
          <div className="progress-row__body">
            <div className="progress-row__top">
              <strong>{item.stepName}</strong>
              <span>{item.progress}%</span>
            </div>
            <div className="progress-bar">
              <span style={{ width: `${item.progress}%` }} />
            </div>
            <p>{item.errorDetail || item.message}</p>
          </div>
        </div>
      ))}
    </div>
  );
}

function LoginScreen({
  busy,
  notice,
  ssh,
  onConnect,
  onSSHChange,
}: {
  busy: string;
  notice: Notice;
  ssh: SSHForm;
  onConnect: () => void;
  onSSHChange: (value: SSHForm) => void;
}) {
  return (
    <main className="login-shell">
      <section className="login-panel">
        <div className="login-brand">
          <Server size={30} />
          <div>
            <strong>连接设备</strong>
            <span>DR600AB SSH</span>
          </div>
        </div>
        {notice.message ? <div className={`notice ${notice.tone}`}>{notice.message}</div> : null}
        <form
          className="login-form"
          onSubmit={(event) => {
            event.preventDefault();
            void onConnect();
          }}
        >
          <label>
            主机
            <input value={ssh.host} onChange={(event) => onSSHChange({ ...ssh, host: event.target.value })} placeholder="192.168.100.101" />
          </label>
          <div className="login-form__row">
            <label>
              端口
              <input type="number" min={1} max={65535} value={ssh.port} onChange={(event) => onSSHChange({ ...ssh, port: Number(event.target.value) })} />
            </label>
            <label>
              用户
              <input value={ssh.user} onChange={(event) => onSSHChange({ ...ssh, user: event.target.value })} />
            </label>
          </div>
          <label>
            密码
            <input type="password" value={ssh.password} onChange={(event) => onSSHChange({ ...ssh, password: event.target.value })} />
          </label>
          <label className="checkbox">
            <input checked={ssh.rememberPassword} type="checkbox" onChange={(event) => onSSHChange({ ...ssh, rememberPassword: event.target.checked })} />
            记住密码
          </label>
          <button className="primary login-submit" type="submit" disabled={busy === "ssh"}>
            <Cable size={16} />
            {busy === "ssh" ? "正在连接" : "连接并进入"}
          </button>
        </form>
      </section>
    </main>
  );
}

export function App() {
  const [page, setPage] = useState<Page>("deploy");
  const [config, setConfig] = useState<SavedConfig>({ installDir: "/opt/dr600ab" });
  const [ssh, setSSH] = useState<SSHForm>({ host: "", port: 22, user: "root", password: "", rememberPassword: false });
  const [sshStatus, setSSHStatus] = useState<SSHStatus>({ connected: false, message: "未连接" });
  const sshStatusRef = useRef<SSHStatus>(sshStatus);
  const [entered, setEntered] = useState(false);
  const [installDir, setInstallDir] = useState("/opt/dr600ab");
  const [firmwarePath, setFirmwarePath] = useState("");
  const [fullUpdate, setFullUpdate] = useState(false);
  const [mapPackage, setMapPackage] = useState("");
  const [keepMapBackup, setKeepMapBackup] = useState(false);
  const [probe, setProbe] = useState<RemoteProbe | null>(null);
  const [remoteDirModal, setRemoteDirModal] = useState<{ open: boolean; path: string; entries: RemoteEntry[]; loading: boolean }>({
    open: false,
    path: "/",
    entries: [],
    loading: false,
  });
  const [startupProbeDir, setStartupProbeDir] = useState("");
  const [notice, setNotice] = useState<Notice>({ tone: "idle", message: "" });
  const [deployProgress, setDeployProgress] = useState<ProgressEvent[]>([]);
  const [dbProgress, setDBProgress] = useState<ProgressEvent[]>([]);
  const [mapProgress, setMapProgress] = useState<ProgressEvent[]>([]);
  const [busy, setBusy] = useState("");

  const [intrusions, setIntrusions] = useState<IntrusionRecord[]>([]);
  const [intrusionQuery, setIntrusionQuery] = useState({ targetType: "all", dateFrom: "", dateTo: "", search: "" });
  const [interferenceReports, setInterferenceReports] = useState<InterferenceReportSummary[]>([]);
  const [interferenceQuery, setInterferenceQuery] = useState({ status: "all", dateFrom: "", dateTo: "" });
  const [reports, setReports] = useState<DeceptionReportSummary[]>([]);
  const [reportQuery, setReportQuery] = useState({ status: "all", mode: "all", dateFrom: "", dateTo: "" });

  const updateSSHStatus = useCallback((status: SSHStatus) => {
    sshStatusRef.current = status;
    setSSHStatus(status);
  }, []);

  useEffect(() => {
    const cleanups = [
      onProgress("deploy-progress", (event) => appendProgress(setDeployProgress, event)),
      onProgress("db-sync-progress", (event) => appendProgress(setDBProgress, event)),
      onProgress("offline-map-progress", (event) => appendProgress(setMapProgress, event)),
    ];
    return () => cleanups.forEach((cleanup) => cleanup());
  }, []);

  useEffect(() => {
    void (async () => {
      try {
        const [loaded, status] = await Promise.all([api.loadConfig(), api.getSSHStatus()]);
        setConfig(loaded);
        setSSH((current) => ({
          ...current,
          host: loaded.ssh?.host || current.host,
          port: loaded.ssh?.port || current.port,
          user: loaded.ssh?.user || current.user,
          password: loaded.ssh?.password || "",
          rememberPassword: Boolean(loaded.ssh?.rememberPassword),
        }));
        const loadedInstallDir = loaded.installDir || "/opt/dr600ab";
        setInstallDir(loadedInstallDir);
        setFirmwarePath(loaded.firmware || "");
        setMapPackage(loaded.mapPackage || "");
        updateSSHStatus(status);
        if (status.connected) {
          setEntered(true);
          setStartupProbeDir(loadedInstallDir);
        }
      } catch (error) {
        setNotice({ tone: "error", message: messageOf(error) });
      }
    })();
  }, [updateSSHStatus]);

  useEffect(() => {
    if (!entered) {
      return;
    }

    let active = true;
    const pollStatus = async () => {
      if (busy === "ssh" || busy === "reconnect" || busy === "disconnect") {
        return;
      }
      const previous = sshStatusRef.current;
      try {
        const status = await api.getSSHStatus();
        if (!active) {
          return;
        }
        updateSSHStatus(status);
        if (previous.connected && !status.connected) {
          setNotice({ tone: "error", message: "设备连接已断开，请点击重连" });
        }
      } catch (error) {
        if (!active) {
          return;
        }
        const message = messageOf(error);
        updateSSHStatus({ ...previous, connected: false, message });
        if (previous.connected) {
          setNotice({ tone: "error", message: "设备连接已断开，请点击重连" });
        }
      }
    };

    const timer = window.setInterval(() => {
      void pollStatus();
    }, 10000);
    return () => {
      active = false;
      window.clearInterval(timer);
    };
  }, [busy, entered, updateSSHStatus]);

  const connected = sshStatus.connected;

  const persistConfig = useCallback((patch: Partial<SavedConfig>) => {
    setConfig((current) => {
      const next = { ...current, ...patch };
      void api.saveConfig(next).catch(() => undefined);
      return next;
    });
  }, []);

  const connect = async () => {
    setBusy("ssh");
    setNotice({ tone: "loading", message: "正在连接 SSH" });
    try {
      const status = await api.connectSSH(ssh);
      updateSSHStatus(status);
      setEntered(true);
      setConfig((current) => ({
        ...current,
        ssh: {
          host: status.host || ssh.host,
          port: status.port || ssh.port,
          user: status.user || ssh.user,
          rememberPassword: ssh.rememberPassword,
          password: ssh.rememberPassword ? ssh.password : "",
        },
      }));
      setProbe(null);
      setIntrusions([]);
      setInterferenceReports([]);
      setReports([]);
      setDeployProgress([]);
      setDBProgress([]);
      setMapProgress([]);
      setNotice({ tone: "success", message: status.message });
      await runProbe({ auto: true });
    } catch (error) {
      setEntered(false);
      setNotice({ tone: "error", message: messageOf(error) });
    } finally {
      setBusy("");
    }
  };

  const reconnect = async () => {
    setBusy("reconnect");
    setNotice({ tone: "loading", message: "正在重连设备" });
    try {
      const status = await api.reconnectSSH();
      updateSSHStatus(status);
      setEntered(true);
      setProbe(null);
      setNotice({ tone: "success", message: status.message || "设备已重连" });
      await runProbe({ auto: true });
    } catch (error) {
      const message = messageOf(error);
      const previous = sshStatusRef.current;
      updateSSHStatus({ ...previous, connected: false, message });
      if (message.includes("密码")) {
        setEntered(false);
      }
      setNotice({ tone: "error", message });
    } finally {
      setBusy("");
    }
  };

  const disconnect = async () => {
    setBusy("disconnect");
    try {
      await api.disconnectSSH();
    } finally {
      updateSSHStatus({ connected: false, message: "未连接" });
      setEntered(false);
      setBusy("");
      setSSH((current) => ({ ...current, password: current.rememberPassword ? current.password : "" }));
      setProbe(null);
      setIntrusions([]);
      setInterferenceReports([]);
      setReports([]);
      setNotice({ tone: "idle", message: "SSH 已断开，请重新连接设备" });
    }
  };

  const runProbe = async (options: ProbeOptions = {}) => {
    const targetInstallDir = options.installDir || installDir;
    setBusy("probe");
    setNotice({ tone: "loading", message: options.auto ? "正在自动探测设备环境" : "正在探测设备环境" });
    try {
      const result = await api.probeRemote(targetInstallDir);
      setProbe(result);
      setNotice({ tone: result.warnings?.length ? "error" : "success", message: result.warnings?.join("；") || (options.auto ? "自动环境探测完成" : "环境探测完成") });
      persistConfig({ installDir: targetInstallDir });
    } catch (error) {
      const message = messageOf(error);
      setNotice({ tone: "error", message: options.auto ? `自动环境探测失败：${message}` : message });
    } finally {
      setBusy("");
    }
  };

  const browseRemoteDir = async (path: string) => {
    const targetPath = path.trim() || "/";
    setRemoteDirModal((current) => ({ ...current, path: targetPath, loading: true }));
    try {
      const entries = await api.browseRemoteDir(targetPath);
      setRemoteDirModal({
        open: true,
        path: targetPath,
        entries: entries.filter((entry) => entry.isDir),
        loading: false,
      });
    } catch (error) {
      setRemoteDirModal((current) => ({ ...current, loading: false }));
      setNotice({ tone: "error", message: messageOf(error) });
    }
  };

  const openRemoteDirPicker = () => {
    if (!connected) {
      setNotice({ tone: "error", message: "请先连接设备" });
      return;
    }
    setRemoteDirModal({ open: true, path: installDir || "/", entries: [], loading: true });
    void browseRemoteDir(installDir || "/");
  };

  const chooseRemoteDir = (path: string) => {
    setInstallDir(path);
    persistConfig({ installDir: path });
    setRemoteDirModal((current) => ({ ...current, open: false }));
  };

  useEffect(() => {
    if (!entered || !connected || !startupProbeDir) {
      return;
    }
    const targetInstallDir = startupProbeDir;
    setStartupProbeDir("");
    void runProbe({ auto: true, installDir: targetInstallDir });
  }, [connected, entered, startupProbeDir]);

  const chooseFirmware = async () => {
    try {
      const path = await api.selectFirmwarePackage();
      if (path) {
        setFirmwarePath(path);
        persistConfig({ firmware: path });
      }
    } catch (error) {
      setNotice({ tone: "error", message: messageOf(error) });
    }
  };

  const deploy = async () => {
    setBusy("deploy");
    setDeployProgress([]);
    setNotice({ tone: "loading", message: "正在部署 DR600AB" });
    try {
      const result = await api.deployDR600AB({ installDir, firmwarePath, fullUpdate });
      setNotice({ tone: "success", message: result.message });
      await runProbe();
    } catch (error) {
      setNotice({ tone: "error", message: messageOf(error) });
    } finally {
      setBusy("");
    }
  };

  const loadIntrusions = async () => {
    setBusy("intrusions");
    setDBProgress([]);
    try {
      const items = await api.listIntrusions({ installDir, ...intrusionQuery, limit: 1000, locale: uiLocale });
      const safeItems = Array.isArray(items) ? items : [];
      setIntrusions(safeItems);
      setNotice({ tone: "success", message: `已加载 ${safeItems.length} 条目标入侵记录` });
    } catch (error) {
      setNotice({ tone: "error", message: messageOf(error) });
    } finally {
      setBusy("");
    }
  };

  const exportIntrusions = async () => {
    setBusy("export-intrusions");
    try {
      const result = await api.exportIntrusionsCSV({ installDir, ...intrusionQuery, limit: 5000, locale: uiLocale });
      setNotice({ tone: "success", message: `已导出 ${result.count} 条记录：${result.path}` });
    } catch (error) {
      setNotice({ tone: "error", message: messageOf(error) });
    } finally {
      setBusy("");
    }
  };

  const loadInterferenceReports = async () => {
    setBusy("interference-reports");
    setDBProgress([]);
    try {
      const items = await api.listInterferenceReports({ installDir, ...interferenceQuery, limit: 1000, locale: uiLocale });
      const safeItems = Array.isArray(items) ? items : [];
      setInterferenceReports(safeItems);
      setNotice({ tone: "success", message: `已加载 ${safeItems.length} 条干扰报告` });
    } catch (error) {
      setNotice({ tone: "error", message: messageOf(error) });
    } finally {
      setBusy("");
    }
  };

  const exportInterferenceReports = async () => {
    setBusy("export-interference-reports");
    try {
      const result = await api.exportInterferenceReportsCSV({ installDir, ...interferenceQuery, limit: 5000, locale: uiLocale });
      setNotice({ tone: "success", message: `已导出 ${result.count} 条干扰报告：${result.path}` });
    } catch (error) {
      setNotice({ tone: "error", message: messageOf(error) });
    } finally {
      setBusy("");
    }
  };

  const loadReports = async () => {
    setBusy("deception-reports");
    setDBProgress([]);
    try {
      const items = await api.listDeceptionReports({ installDir, ...reportQuery, limit: 1000, locale: uiLocale });
      const safeItems = Array.isArray(items) ? items : [];
      setReports(safeItems);
      setNotice({ tone: "success", message: `已加载 ${safeItems.length} 条诱骗报告` });
    } catch (error) {
      setNotice({ tone: "error", message: messageOf(error) });
    } finally {
      setBusy("");
    }
  };

  const exportReports = async () => {
    setBusy("export-deception-reports");
    try {
      const result = await api.exportDeceptionReportsCSV({ installDir, ...reportQuery, limit: 5000, locale: uiLocale });
      setNotice({ tone: "success", message: `已导出 ${result.count} 条报告：${result.path}` });
    } catch (error) {
      setNotice({ tone: "error", message: messageOf(error) });
    } finally {
      setBusy("");
    }
  };

  const chooseMapPackage = async () => {
    try {
      const path = await api.selectOfflineMapPackage();
      if (path) {
        setMapPackage(path);
        persistConfig({ mapPackage: path });
      }
    } catch (error) {
      setNotice({ tone: "error", message: messageOf(error) });
    }
  };

  const uploadMap = async () => {
    setBusy("map");
    setMapProgress([]);
    setNotice({ tone: "loading", message: "正在上传离线地图" });
    try {
      const result = await api.uploadOfflineMap({ installDir, packagePath: mapPackage, keepBackup: keepMapBackup });
      setNotice({ tone: "success", message: `${result.message}，瓦片 ${result.tileCount} 个` });
      await runProbe();
    } catch (error) {
      setNotice({ tone: "error", message: messageOf(error) });
    } finally {
      setBusy("");
    }
  };

  const cleanupMapBackup = async () => {
    setBusy("map-cleanup");
    try {
      setNotice({ tone: "success", message: await api.cleanupOfflineMapBackup(installDir) });
    } catch (error) {
      setNotice({ tone: "error", message: messageOf(error) });
    } finally {
      setBusy("");
    }
  };

  const currentPage = useMemo(() => pageItems.find((item) => item.id === page) ?? pageItems[0], [page]);
  const CurrentIcon = currentPage.icon;

  if (!entered) {
    return (
      <LoginScreen
        busy={busy}
        notice={notice}
        ssh={ssh}
        onConnect={connect}
        onSSHChange={setSSH}
      />
    );
  }

  return (
    <div className="app-shell">
      <aside className="sidebar">
        <div className="brand">
          <Server size={24} />
          <div>
            <strong>DR600AB</strong>
            <span>运维工具</span>
          </div>
        </div>
        <nav>
          {pageItems.map((item) => {
            const Icon = item.icon;
            return (
              <button className={page === item.id ? "active" : ""} key={item.id} type="button" onClick={() => setPage(item.id)}>
                <Icon size={18} />
                <span>{item.label}</span>
              </button>
            );
          })}
        </nav>
      </aside>

      <main>
        <section className="topbar">
          <div className="title">
            <CurrentIcon size={22} />
            <div>
              <h1>{currentPage.label}</h1>
              <p>{sshStatus.host ? `${sshStatus.user}@${sshStatus.host}:${sshStatus.port || 22}` : "设备连接已断开"}</p>
            </div>
          </div>
          <div className="topbar-actions">
            <div className={`status-dot ${connected ? "online" : ""}`}>
              <span />
              {connected ? "已连接" : "已断开"}
            </div>
            {!connected ? (
              <button className="primary" type="button" disabled={busy === "reconnect"} onClick={() => void reconnect()}>
                <RefreshCw size={16} />
                {busy === "reconnect" ? "重连中" : "重连"}
              </button>
            ) : null}
            <button type="button" disabled={busy === "reconnect" || busy === "disconnect"} onClick={() => void disconnect()}>
              断开
            </button>
          </div>
        </section>

        <section className="workspace">
          <div className="content">
            <div className={`notice ${notice.tone}`}>{notice.message || "就绪"}</div>
            {page === "deploy" ? (
              <DeployPage
                busy={busy}
                connected={connected}
                deployProgress={deployProgress}
                firmwarePath={firmwarePath}
                fullUpdate={fullUpdate}
                installDir={installDir}
                probe={probe}
                onChooseFirmware={chooseFirmware}
                onDeploy={deploy}
                onFullUpdateChange={setFullUpdate}
                onInstallDirChange={(value) => {
                  setInstallDir(value);
                  persistConfig({ installDir: value });
                }}
                onOpenDirPicker={openRemoteDirPicker}
                onProbe={runProbe}
              />
            ) : null}
            {page === "intrusions" ? (
              <IntrusionsPage
                busy={busy}
                connected={connected}
                dbProgress={dbProgress}
                intrusions={intrusions}
                query={intrusionQuery}
                onExport={exportIntrusions}
                onLoad={loadIntrusions}
                onQueryChange={setIntrusionQuery}
              />
            ) : null}
            {page === "interference-reports" ? (
              <InterferenceReportsPage
                busy={busy}
                connected={connected}
                dbProgress={dbProgress}
                query={interferenceQuery}
                reports={interferenceReports}
                onExport={exportInterferenceReports}
                onLoad={loadInterferenceReports}
                onQueryChange={setInterferenceQuery}
              />
            ) : null}
            {page === "deception-reports" ? (
              <DeceptionReportsPage
                busy={busy}
                connected={connected}
                dbProgress={dbProgress}
                query={reportQuery}
                reports={reports}
                onExport={exportReports}
                onLoad={loadReports}
                onQueryChange={setReportQuery}
              />
            ) : null}
            {page === "offline-map" ? (
              <OfflineMapPage
                busy={busy}
                connected={connected}
                keepBackup={keepMapBackup}
                mapPackage={mapPackage}
                mapProgress={mapProgress}
                onChoosePackage={chooseMapPackage}
                onCleanupBackup={cleanupMapBackup}
                onKeepBackupChange={setKeepMapBackup}
                onUpload={uploadMap}
              />
            ) : null}
          </div>
        </section>
      </main>
      {remoteDirModal.open ? (
        <RemoteDirPicker
          entries={remoteDirModal.entries}
          loading={remoteDirModal.loading}
          path={remoteDirModal.path}
          onClose={() => setRemoteDirModal((current) => ({ ...current, open: false }))}
          onEnter={(path) => void browseRemoteDir(path)}
          onSelect={chooseRemoteDir}
          onUp={() => void browseRemoteDir(parentRemoteDir(remoteDirModal.path))}
        />
      ) : null}
    </div>
  );
}

function InstallDirField({
  connected,
  onBrowse,
  onChange,
  value,
}: {
  connected: boolean;
  onBrowse: () => void;
  onChange: (value: string) => void;
  value: string;
}) {
  return (
    <label className="file-field wide">
      安装目录
      <div>
        <input value={value} onChange={(event) => onChange(event.target.value)} placeholder="/opt/dr600ab" />
        <button type="button" disabled={!connected} onClick={onBrowse}>
          <FolderOpen size={16} />
          选择
        </button>
      </div>
    </label>
  );
}

function DeployPage({
  busy,
  connected,
  deployProgress,
  firmwarePath,
  fullUpdate,
  installDir,
  probe,
  onChooseFirmware,
  onDeploy,
  onFullUpdateChange,
  onInstallDirChange,
  onOpenDirPicker,
  onProbe,
}: {
  busy: string;
  connected: boolean;
  deployProgress: ProgressEvent[];
  firmwarePath: string;
  fullUpdate: boolean;
  installDir: string;
  probe: RemoteProbe | null;
  onChooseFirmware: () => void;
  onDeploy: () => void;
  onFullUpdateChange: (value: boolean) => void;
  onInstallDirChange: (value: string) => void;
  onOpenDirPicker: () => void;
  onProbe: () => void;
}) {
  return (
    <div className="page-grid two">
      <section className="panel">
        <h2>部署参数</h2>
        <div className="form-grid">
          <InstallDirField connected={connected} value={installDir} onBrowse={onOpenDirPicker} onChange={onInstallDirChange} />
          <label className="file-field wide">
            固件包
            <div>
              <input readOnly value={firmwarePath} placeholder="选择 .tar.gz 固件包" />
              <button type="button" onClick={onChooseFirmware}>
                <FolderOpen size={16} />
                选择
              </button>
            </div>
          </label>
          <div className="firmware-help wide">
            <strong>固件包要求</strong>
            <span>请选择设备对应的 .tar.gz 固件包；文件名不要求固定格式，也不限制所在目录。</span>
            <span>压缩包内必须包含 dr600ab 可执行文件，部署时会自动校验。</span>
            <span>增量更新会保留 data、backend/data、static/map；全量更新会先备份再覆盖。</span>
            <span>设备端需要 systemctl、tar 和 Chromium；部署会安装 dr600ab.service，并为图形用户安装屏幕自启动项。</span>
          </div>
          <label className="checkbox wide">
            <input checked={fullUpdate} type="checkbox" onChange={(event) => onFullUpdateChange(event.target.checked)} />
            全量覆盖更新
          </label>
        </div>
        <div className="actions">
          <button type="button" disabled={!connected || busy === "probe"} onClick={() => void onProbe()}>
            <RefreshCw size={16} />
            探测环境
          </button>
          <button className="primary" type="button" disabled={!connected || !firmwarePath || busy === "deploy"} onClick={() => void onDeploy()}>
            <UploadCloud size={16} />
            上传更新
          </button>
        </div>
      </section>
      <section className="panel">
        <h2>设备状态</h2>
        {probe ? (
          <div className="probe-grid">
            <InfoTile label="服务状态" value={probe.serviceStatus || "-"} strong={probe.serviceActive} />
            <InfoTile label="屏幕状态" value={probe.kioskStatus || "-"} strong={probe.kioskActive} />
            <InfoTile label="systemd" value={probe.hasSystemd ? "可用" : "不可用"} strong={probe.hasSystemd} />
            <InfoTile label="tar" value={probe.hasTar ? "可用" : "不可用"} strong={probe.hasTar} />
            <InfoTile label="Chromium" value={probe.chromiumPath || "未找到"} strong={!!probe.chromiumPath} />
            <InfoTile label="地图解压" value={probe.hasUnzip || probe.hasBusybox ? "可用" : "不可用"} strong={probe.hasUnzip || probe.hasBusybox} />
            <InfoTile label="离线地图" value={probe.offlineMapExists ? "已安装" : "未安装"} strong={probe.offlineMapExists} />
            <InfoTile label="入侵库" value={probe.intrusionDbPath || "未找到"} />
            <InfoTile label="干扰库" value={probe.interferenceDbPath || "未找到"} />
            <InfoTile label="诱骗库" value={probe.deceptionDbPath || "未找到"} />
          </div>
        ) : (
          <div className="empty">尚未探测</div>
        )}
      </section>
      <section className="panel span">
        <h2>部署进度</h2>
        <ProgressList items={deployProgress} />
      </section>
    </div>
  );
}

function InfoTile({ label, value, strong = false }: { label: string; value: string; strong?: boolean }) {
  return (
    <div className={`info-tile ${strong ? "strong" : ""}`}>
      <span>{label}</span>
      <strong>{value}</strong>
    </div>
  );
}

function RemoteDirPicker({
  entries,
  loading,
  onClose,
  onEnter,
  onSelect,
  onUp,
  path,
}: {
  entries: RemoteEntry[];
  loading: boolean;
  onClose: () => void;
  onEnter: (path: string) => void;
  onSelect: (path: string) => void;
  onUp: () => void;
  path: string;
}) {
  return (
    <div className="modal-scrim" role="presentation" onClick={onClose}>
      <section className="modal dir-modal" role="dialog" aria-modal="true" onClick={(event) => event.stopPropagation()}>
        <header>
          <div>
            <h2>选择远程安装目录</h2>
            <p className="modal-path">{path}</p>
          </div>
          <button type="button" onClick={onClose}>关闭</button>
        </header>
        <div className="dir-actions">
          <button type="button" disabled={loading || path === "/"} onClick={onUp}>
            上一级
          </button>
          <button className="primary" type="button" disabled={loading} onClick={() => onSelect(path)}>
            选择当前目录
          </button>
        </div>
        <div className="dir-list">
          {loading ? (
            <div className="empty compact">正在读取目录</div>
          ) : entries.length === 0 ? (
            <div className="empty compact">暂无子目录</div>
          ) : (
            entries.map((entry) => (
              <button className="dir-row" key={entry.path} type="button" onClick={() => onEnter(entry.path)}>
                <FolderOpen size={16} />
                <span>{entry.name}</span>
              </button>
            ))
          )}
        </div>
      </section>
    </div>
  );
}

function IntrusionsPage({
  busy,
  connected,
  dbProgress,
  intrusions,
  query,
  onExport,
  onLoad,
  onQueryChange,
}: {
  busy: string;
  connected: boolean;
  dbProgress: ProgressEvent[];
  intrusions: IntrusionRecord[];
  query: { targetType: string; dateFrom: string; dateTo: string; search: string };
  onExport: () => void;
  onLoad: () => void;
  onQueryChange: (value: { targetType: string; dateFrom: string; dateTo: string; search: string }) => void;
}) {
  return (
    <div className="page-grid">
      <section className="panel">
        <TableToolbar
          connected={connected}
          loading={busy === "intrusions"}
          exportBusy={busy === "export-intrusions"}
          onExport={onExport}
          onLoad={onLoad}
        >
          <select value={query.targetType} onChange={(event) => onQueryChange({ ...query, targetType: event.target.value })}>
            <option value="all">全部类型</option>
            <option value="detection">侦测目标</option>
            <option value="position">定位目标</option>
          </select>
          <input type="date" value={query.dateFrom} onChange={(event) => onQueryChange({ ...query, dateFrom: event.target.value })} />
          <input type="date" value={query.dateTo} onChange={(event) => onQueryChange({ ...query, dateTo: event.target.value })} />
          <input value={query.search} onChange={(event) => onQueryChange({ ...query, search: event.target.value })} placeholder="搜索目标/型号/序列号" />
        </TableToolbar>
        <div className="table-wrap">
          <table className="intrusion-table">
            <colgroup>
              <col style={{ width: "82px" }} />
              <col style={{ width: "180px" }} />
              <col style={{ width: "160px" }} />
              <col style={{ width: "94px" }} />
              <col style={{ width: "86px" }} />
              <col style={{ width: "150px" }} />
              <col style={{ width: "150px" }} />
              <col style={{ width: "96px" }} />
              <col style={{ width: "420px" }} />
              <col style={{ width: "96px" }} />
              <col style={{ width: "108px" }} />
              <col style={{ width: "90px" }} />
              <col style={{ width: "80px" }} />
            </colgroup>
            <thead>
              <tr>
                <th>类型</th>
                <th>型号</th>
                <th>序列号</th>
                <th>频点</th>
                <th>信号</th>
                <th>首次发现</th>
                <th>最后发现</th>
                <th>持续时间</th>
                <th>坐标</th>
                <th>飞手距离</th>
                <th>无人机距离</th>
                <th>速度</th>
                <th>高度</th>
              </tr>
            </thead>
            <tbody>
              {intrusions.length === 0 ? (
                <tr>
                  <td colSpan={13}>
                    <div className="empty compact">暂无记录</div>
                  </td>
                </tr>
              ) : (
                intrusions.map((record) => (
                  <tr key={record.id}>
                    <td><StatusLight label={record.targetType === "position" ? "定位" : "侦测"} status={record.targetType === "position" ? "completed" : "running"} /></td>
                    <td>{record.displayModel || record.model || "-"}</td>
                    <td className="mono">{record.serial || "-"}</td>
                    <td>{formatFrequency(record.frequency)}</td>
                    <td>{formatRSSI(record.rssi)}</td>
                    <td>{formatTime(record.firstSeen)}</td>
                    <td>{formatTime(record.lastSeen)}</td>
                    <td>{formatDuration(record.durationSeconds)}</td>
                    <td className="mono">{coordinateSummary(record)}</td>
                    <td>{formatDistance(record.pilotDistanceM)}</td>
                    <td>{formatDistance(record.droneDistanceM)}</td>
                    <td>{formatOptionalNumber(record.speed, " m/s")}</td>
                    <td>{formatOptionalNumber(record.height, " m", 0)}</td>
                  </tr>
                ))
              )}
            </tbody>
          </table>
        </div>
      </section>
      <section className="panel">
        <h2>数据库同步</h2>
        <ProgressList items={dbProgress} />
      </section>
    </div>
  );
}

function InterferenceReportsPage({
  busy,
  connected,
  dbProgress,
  query,
  reports,
  onExport,
  onLoad,
  onQueryChange,
}: {
  busy: string;
  connected: boolean;
  dbProgress: ProgressEvent[];
  query: { status: string; dateFrom: string; dateTo: string };
  reports: InterferenceReportSummary[];
  onExport: () => void;
  onLoad: () => void;
  onQueryChange: (value: { status: string; dateFrom: string; dateTo: string }) => void;
}) {
  return (
    <div className="page-grid">
      <section className="panel">
        <TableToolbar connected={connected} loading={busy === "interference-reports"} exportBusy={busy === "export-interference-reports"} onExport={onExport} onLoad={onLoad}>
          <select value={query.status} onChange={(event) => onQueryChange({ ...query, status: event.target.value })}>
            <option value="all">全部状态</option>
            <option value="running">运行中</option>
            <option value="completed">已完成</option>
            <option value="failed">失败</option>
            <option value="abnormal">异常</option>
          </select>
          <input type="date" value={query.dateFrom} onChange={(event) => onQueryChange({ ...query, dateFrom: event.target.value })} />
          <input type="date" value={query.dateTo} onChange={(event) => onQueryChange({ ...query, dateTo: event.target.value })} />
        </TableToolbar>
        <div className="table-wrap">
          <table className="report-table interference-report-table">
            <colgroup>
              <col style={{ width: "110px" }} />
              <col style={{ width: "160px" }} />
              <col style={{ width: "160px" }} />
              <col style={{ width: "100px" }} />
              <col style={{ width: "260px" }} />
              <col style={{ width: "100px" }} />
              <col style={{ width: "260px" }} />
            </colgroup>
            <thead>
              <tr>
                <th>状态</th>
                <th>开始时间</th>
                <th>结束时间</th>
                <th>持续</th>
                <th>干扰频段</th>
                <th>设置时长</th>
                <th>错误</th>
              </tr>
            </thead>
            <tbody>
              {reports.length === 0 ? (
                <tr>
                  <td colSpan={7}>
                    <div className="empty compact">暂无报告</div>
                  </td>
                </tr>
              ) : (
                reports.map((report) => (
                  <tr key={report.id}>
                    <td><StatusLight label={statusLabel(report.status)} status={report.status} /></td>
                    <td>{formatTime(report.startedAt)}</td>
                    <td>{formatTime(report.endedAt)}</td>
                    <td>{formatDuration(report.durationSeconds)}</td>
                    <td>{formatChannelLabels(report)}</td>
                    <td>{formatDuration(report.requestedDurationSeconds ?? 0)}</td>
                    <td>{report.lastError || report.abnormalReason || "-"}</td>
                  </tr>
                ))
              )}
            </tbody>
          </table>
        </div>
      </section>
      <section className="panel">
        <h2>数据库同步</h2>
        <ProgressList items={dbProgress} />
      </section>
    </div>
  );
}

function DeceptionReportsPage({
  busy,
  connected,
  dbProgress,
  query,
  reports,
  onExport,
  onLoad,
  onQueryChange,
}: {
  busy: string;
  connected: boolean;
  dbProgress: ProgressEvent[];
  query: { status: string; mode: string; dateFrom: string; dateTo: string };
  reports: DeceptionReportSummary[];
  onExport: () => void;
  onLoad: () => void;
  onQueryChange: (value: { status: string; mode: string; dateFrom: string; dateTo: string }) => void;
}) {
  return (
    <div className="page-grid">
      <section className="panel">
        <TableToolbar connected={connected} loading={busy === "deception-reports"} exportBusy={busy === "export-deception-reports"} onExport={onExport} onLoad={onLoad}>
          <select value={query.status} onChange={(event) => onQueryChange({ ...query, status: event.target.value })}>
            <option value="all">全部状态</option>
            <option value="running">运行中</option>
            <option value="completed">已完成</option>
            <option value="failed">失败</option>
            <option value="abnormal">异常</option>
          </select>
          <select value={query.mode} onChange={(event) => onQueryChange({ ...query, mode: event.target.value })}>
            <option value="all">全部模式</option>
            <option value="fixed_point">定点诱骗</option>
            <option value="circle">圆周诱骗</option>
            <option value="linear">线性诱骗</option>
          </select>
          <input type="date" value={query.dateFrom} onChange={(event) => onQueryChange({ ...query, dateFrom: event.target.value })} />
          <input type="date" value={query.dateTo} onChange={(event) => onQueryChange({ ...query, dateTo: event.target.value })} />
        </TableToolbar>
        <div className="table-wrap">
          <table className="report-table deception-report-table">
            <colgroup>
              <col style={{ width: "110px" }} />
              <col style={{ width: "170px" }} />
              <col style={{ width: "170px" }} />
              <col style={{ width: "110px" }} />
              <col style={{ width: "130px" }} />
              <col style={{ width: "320px" }} />
            </colgroup>
            <thead>
              <tr>
                <th>状态</th>
                <th>开始时间</th>
                <th>结束时间</th>
                <th>持续</th>
                <th>模式</th>
                <th>错误</th>
              </tr>
            </thead>
            <tbody>
              {reports.length === 0 ? (
                <tr>
                  <td colSpan={6}>
                    <div className="empty compact">暂无报告</div>
                  </td>
                </tr>
              ) : (
                reports.map((report) => (
                  <tr key={report.id}>
                    <td><StatusLight label={statusLabel(report.status)} status={report.status} /></td>
                    <td>{formatTime(report.startedAt)}</td>
                    <td>{formatTime(report.endedAt)}</td>
                    <td>{formatDuration(report.durationSeconds)}</td>
                    <td>{modeLabel(report.mode)}</td>
                    <td>{report.lastError || report.abnormalReason || "-"}</td>
                  </tr>
                ))
              )}
            </tbody>
          </table>
        </div>
      </section>
      <section className="panel">
        <h2>数据库同步</h2>
        <ProgressList items={dbProgress} />
      </section>
    </div>
  );
}

function TableToolbar({
  children,
  connected,
  exportBusy,
  loading,
  onExport,
  onLoad,
}: {
  children: ReactNode;
  connected: boolean;
  exportBusy: boolean;
  loading: boolean;
  onExport: () => void;
  onLoad: () => void;
}) {
  return (
    <div className="table-toolbar">
      <div className="filters">{children}</div>
      <div className="actions">
        <button type="button" disabled={!connected || loading} onClick={() => void onLoad()}>
          <RefreshCw size={16} />
          加载
        </button>
        <button className="primary" type="button" disabled={!connected || exportBusy} onClick={() => void onExport()}>
          <Download size={16} />
          导出 CSV
        </button>
      </div>
    </div>
  );
}

function OfflineMapPage({
  busy,
  connected,
  keepBackup,
  mapPackage,
  mapProgress,
  onChoosePackage,
  onCleanupBackup,
  onKeepBackupChange,
  onUpload,
}: {
  busy: string;
  connected: boolean;
  keepBackup: boolean;
  mapPackage: string;
  mapProgress: ProgressEvent[];
  onChoosePackage: () => void;
  onCleanupBackup: () => void;
  onKeepBackupChange: (value: boolean) => void;
  onUpload: () => void;
}) {
  return (
    <div className="page-grid two">
      <section className="panel">
        <h2>上传离线地图</h2>
        <div className="form-grid">
          <label className="file-field wide">
            地图 ZIP 包
            <div>
              <input readOnly value={mapPackage} placeholder="选择包含 dt/{z}/{x}/{y}.jpg 的 ZIP 包" />
              <button type="button" onClick={onChoosePackage}>
                <FileArchive size={16} />
                选择
              </button>
            </div>
          </label>
          <label className="checkbox wide">
            <input checked={keepBackup} type="checkbox" onChange={(event) => onKeepBackupChange(event.target.checked)} />
            保留旧地图备份
          </label>
        </div>
        <div className="actions">
          <button className="primary" type="button" disabled={!connected || !mapPackage || busy === "map"} onClick={() => void onUpload()}>
            <UploadCloud size={16} />
            上传并切换
          </button>
          <button type="button" disabled={!connected || busy === "map-cleanup"} onClick={() => void onCleanupBackup()}>
            清理地图备份
          </button>
        </div>
      </section>
      <section className="panel">
        <h2>地图进度</h2>
        <ProgressList items={mapProgress} />
      </section>
    </div>
  );
}
