import { useCallback, useEffect, useRef, useState } from "react";
import { CalendarClock, ChevronDown, Clock3, Globe2, RefreshCw, Save, Search } from "lucide-react";

import type { TimeInfo } from "./types";
import { api } from "./wails";

type NoticeMessage = {
  tone: "idle" | "success" | "error" | "loading";
  message: string;
};

type TimeConfigPageProps = {
  connected: boolean;
  connectionKey: string;
  onNotice: (notice: NoticeMessage) => void;
};

function messageOf(error: unknown) {
  return error instanceof Error ? error.message : String(error);
}

function toDateTimeLocal(value: string) {
  const normalized = value.trim().replace(" ", "T");
  return /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}$/.test(normalized) ? normalized : "";
}

function toRemoteDateTime(value: string) {
  let normalized = value.trim().replace("T", " ");
  if (/^\d{4}-\d{2}-\d{2} \d{2}:\d{2}$/.test(normalized)) {
    normalized += ":00";
  }
  return /^\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}$/.test(normalized) ? normalized : "";
}

export function TimeConfigPage({ connected, connectionKey, onNotice }: TimeConfigPageProps) {
  const [timeInfo, setTimeInfo] = useState<TimeInfo | null>(null);
  const [timezone, setTimezone] = useState("");
  const [timezoneQuery, setTimezoneQuery] = useState("");
  const [timezoneMenuOpen, setTimezoneMenuOpen] = useState(false);
  const timezonePickerRef = useRef<HTMLDivElement>(null);
  const [manualTime, setManualTime] = useState("");
  const [loading, setLoading] = useState(false);
  const [action, setAction] = useState<"ntp" | "timezone" | "manual-time" | "">("");
  const [pendingNTP, setPendingNTP] = useState<boolean | null>(null);
  const requestVersionRef = useRef(0);

  const fetchTimeInfo = useCallback(
    async (silent = false) => {
      if (!connected) return;
      const requestVersion = requestVersionRef.current;
      if (!silent) {
        setLoading(true);
        onNotice({ tone: "loading", message: "正在读取设备时间" });
      }
      try {
        const info = await api.getTimeInfo();
        if (requestVersion !== requestVersionRef.current) return null;
        setTimeInfo(info);
        setTimezone(info.timezone === "未知" ? "" : info.timezone);
        setManualTime((current) => current || toDateTimeLocal(info.currentTime));
        if (!silent) {
          onNotice({ tone: "success", message: "设备时间已刷新" });
        }
        return info;
      } catch (error) {
        if (requestVersion !== requestVersionRef.current) return null;
        if (!silent) {
          onNotice({ tone: "error", message: messageOf(error) });
        }
        return null;
      } finally {
        if (!silent && requestVersion === requestVersionRef.current) setLoading(false);
      }
    },
    [connected, connectionKey, onNotice],
  );

  useEffect(() => {
    requestVersionRef.current += 1;
    setTimeInfo(null);
    setTimezone("");
    setTimezoneQuery("");
    setTimezoneMenuOpen(false);
    setManualTime("");
    setLoading(false);
    setAction("");
    setPendingNTP(null);
    if (!connected) {
      return;
    }

    void fetchTimeInfo();
    const timer = window.setInterval(() => {
      void fetchTimeInfo(true);
    }, 30000);
    return () => window.clearInterval(timer);
  }, [connected, connectionKey, fetchTimeInfo]);

  useEffect(() => {
    if (!timezoneMenuOpen) {
      return;
    }
    const closeOnOutsideClick = (event: MouseEvent) => {
      if (!timezonePickerRef.current?.contains(event.target as Node)) {
        setTimezoneMenuOpen(false);
      }
    };
    const closeOnEscape = (event: KeyboardEvent) => {
      if (event.key === "Escape") {
        setTimezoneMenuOpen(false);
      }
    };
    document.addEventListener("mousedown", closeOnOutsideClick);
    document.addEventListener("keydown", closeOnEscape);
    return () => {
      document.removeEventListener("mousedown", closeOnOutsideClick);
      document.removeEventListener("keydown", closeOnEscape);
    };
  }, [timezoneMenuOpen]);

  const saveTimezone = async () => {
    const requestVersion = requestVersionRef.current;
    const value = timezone.trim();
    if (!value) {
      onNotice({ tone: "error", message: "请选择时区" });
      return;
    }

    setAction("timezone");
    onNotice({ tone: "loading", message: `正在设置设备时区为 ${value}` });
    try {
      await api.setTimezone(value);
      if (requestVersion !== requestVersionRef.current) return;
      onNotice({ tone: "success", message: `设备时区已设置为 ${value}` });
      await fetchTimeInfo(true);
    } catch (error) {
      if (requestVersion === requestVersionRef.current) {
        onNotice({ tone: "error", message: messageOf(error) });
      }
    } finally {
      if (requestVersion === requestVersionRef.current) setAction("");
    }
  };

  const availableTimezones = timeInfo?.timezones ?? [];
  const normalizedTimezoneQuery = timezoneQuery.trim().toLowerCase();
  const filteredTimezones = availableTimezones.filter((item) => item.toLowerCase().includes(normalizedTimezoneQuery));
  const visibleTimezones = timezone && !filteredTimezones.includes(timezone)
    ? [timezone, ...filteredTimezones]
    : filteredTimezones;
  const timezonePickerDisabled = !connected || !timeInfo || availableTimezones.length === 0 || action !== "";

  const toggleNTP = async (enabled: boolean) => {
    const requestVersion = requestVersionRef.current;
    setAction("ntp");
    setPendingNTP(enabled);
    onNotice({ tone: "loading", message: `正在${enabled ? "开启" : "关闭"} NTP 自动同步` });
    try {
      await api.setNTPEnabled(enabled);
      if (requestVersion !== requestVersionRef.current) return;
      const refreshed = await fetchTimeInfo(true);
      if (!refreshed) {
        throw new Error("NTP 已设置，但读取设备状态失败，请刷新后确认");
      }
      if (refreshed.ntpEnabled !== enabled) {
        throw new Error("NTP 状态回读与设置值不一致");
      }
      onNotice({ tone: "success", message: `NTP 自动同步已${enabled ? "开启" : "关闭"}` });
    } catch (error) {
      if (requestVersion === requestVersionRef.current) {
        onNotice({ tone: "error", message: messageOf(error) });
      }
    } finally {
      if (requestVersion === requestVersionRef.current) {
        setPendingNTP(null);
        setAction("");
      }
    }
  };

  const saveManualTime = async () => {
    const requestVersion = requestVersionRef.current;
    const value = toRemoteDateTime(manualTime);
    if (!value) {
      onNotice({ tone: "error", message: "请选择包含秒数的有效日期和时间" });
      return;
    }

    setAction("manual-time");
    onNotice({ tone: "loading", message: "正在设置设备时间" });
    try {
      await api.setManualTime(value);
      if (requestVersion !== requestVersionRef.current) return;
      onNotice({ tone: "success", message: "设备时间已更新，NTP 自动同步已关闭" });
      setManualTime("");
      await fetchTimeInfo(true);
    } catch (error) {
      if (requestVersion === requestVersionRef.current) {
        onNotice({ tone: "error", message: messageOf(error) });
      }
    } finally {
      if (requestVersion === requestVersionRef.current) setAction("");
    }
  };

  return (
    <div className="page-grid two time-page">
      <section className="panel time-status-panel">
        <div className="time-panel-heading">
          <h2>设备时间</h2>
          <button
            className="small"
            type="button"
            title="刷新设备时间"
            disabled={!connected || loading || action !== ""}
            onClick={() => void fetchTimeInfo()}
          >
            <RefreshCw className={loading ? "spin" : ""} size={15} />
            刷新
          </button>
        </div>
        {timeInfo ? (
          <div className="time-readings" aria-live="polite">
            <div className="time-reading time-reading--primary">
              <Clock3 size={20} />
              <div>
                <span>当前时间</span>
                <strong>{timeInfo.currentTime}</strong>
              </div>
            </div>
            <div className="time-reading">
              <Globe2 size={20} />
              <div>
                <span>当前时区</span>
                <strong>{timeInfo.timezone}</strong>
              </div>
            </div>
            <div className={`time-reading ${timeInfo.ntpEnabled ? "time-reading--online" : ""}`}>
              <RefreshCw size={20} />
              <div>
                <span>NTP 自动同步</span>
                <strong>{timeInfo.ntpEnabled ? "已开启" : "已关闭"}</strong>
              </div>
            </div>
          </div>
        ) : (
          <div className="empty compact">{loading ? "正在读取设备时间" : "暂无设备时间信息"}</div>
        )}
      </section>

      <section className="panel time-settings-panel">
        <h2>时间设置</h2>
        <div className="time-control-list">
          <div className="time-control-row">
            <div className="time-control-label">
              <RefreshCw size={18} />
              <div>
                <strong>NTP 自动同步</strong>
                <span>由设备自动校准系统时间</span>
              </div>
            </div>
            <div className="time-toggle-control">
              <span className={`time-toggle-state ${(pendingNTP ?? timeInfo?.ntpEnabled) ? "online" : ""}`}>
                {action === "ntp" ? "处理中" : timeInfo?.ntpEnabled ? "已开启" : "已关闭"}
              </span>
              <label className="time-toggle">
                <input
                  type="checkbox"
                  aria-label="NTP 自动同步"
                  checked={pendingNTP ?? timeInfo?.ntpEnabled ?? false}
                  disabled={!connected || !timeInfo || action !== ""}
                  onChange={(event) => void toggleNTP(event.target.checked)}
                />
                <span className="time-toggle__track" aria-hidden="true" />
              </label>
            </div>
          </div>

          <div className="time-control-row">
            <div className="time-control-label">
              <Globe2 size={18} />
              <div>
                <strong>设备时区</strong>
                <span>{timeInfo?.timezones.length || 0} 个可用时区</span>
              </div>
            </div>
            <div className="time-control-input">
              <div className="time-zone-picker" ref={timezonePickerRef}>
                <label className="time-zone-search">
                  <Search size={15} aria-hidden="true" />
                  <input
                    type="search"
                    inputMode="text"
                    data-keyboard="ascii"
                    aria-label="搜索时区名称"
                    placeholder="搜索时区名称"
                    value={timezoneQuery}
                    disabled={timezonePickerDisabled}
                    onFocus={() => setTimezoneMenuOpen(true)}
                    onChange={(event) => {
                      setTimezoneQuery(event.target.value);
                      setTimezoneMenuOpen(true);
                    }}
                  />
                </label>
                <button
                  className="time-zone-select"
                  type="button"
                  aria-label="设备时区"
                  aria-expanded={timezoneMenuOpen}
                  disabled={timezonePickerDisabled}
                  onClick={() => setTimezoneMenuOpen((open) => !open)}
                >
                  <span>{timezone || "请选择时区"}</span>
                  <ChevronDown size={16} aria-hidden="true" />
                </button>
                {timezoneMenuOpen ? (
                  <div className="time-zone-options" role="listbox" aria-label="时区候选项">
                    {visibleTimezones.length > 0 ? visibleTimezones.map((item) => (
                      <button
                        className={`time-zone-option ${item === timezone ? "selected" : ""}`}
                        key={item}
                        type="button"
                        role="option"
                        aria-selected={item === timezone}
                        onClick={() => {
                          setTimezone(item);
                          setTimezoneQuery("");
                          setTimezoneMenuOpen(false);
                        }}
                      >
                        {item}
                      </button>
                    )) : <span className="time-zone-no-match">没有匹配的时区</span>}
                  </div>
                ) : null}
              </div>
              <button
                className="primary"
                type="button"
                disabled={!connected || !timezone.trim() || action !== ""}
                onClick={() => void saveTimezone()}
              >
                <Save size={16} />
                {action === "timezone" ? "设置中" : "应用时区"}
              </button>
            </div>
          </div>

          <div className="time-control-row">
            <div className="time-control-label">
              <CalendarClock size={18} />
              <div>
                <strong>手动时间</strong>
                <span>设置后关闭 NTP 自动同步</span>
              </div>
            </div>
            <div className="time-control-input">
              <input
                type="datetime-local"
                step="1"
                value={manualTime}
                onChange={(event) => setManualTime(event.target.value)}
              />
              <button
                className="primary"
                type="button"
                disabled={!connected || !manualTime || action !== ""}
                onClick={() => void saveManualTime()}
              >
                <Clock3 size={16} />
                {action === "manual-time" ? "设置中" : "设置时间"}
              </button>
            </div>
          </div>
        </div>
      </section>
    </div>
  );
}
