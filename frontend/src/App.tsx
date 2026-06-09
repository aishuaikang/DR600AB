import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useTranslation } from "react-i18next";

import {
  type ApiRequestError,
  createDeveloperSessionRequest,
  deleteDeveloperSessionRequest,
  getCompassSession,
  getCompassSettings,
  getDeceptionSession,
  getDeceptionSettings,
  getLicenseStatus,
  getScreenStatus,
  getChannels,
  getDetectionSettings,
  getDetections,
  getGPSRecords,
  getGPSSession,
  getGPSSettings,
  getLocales,
  getParsed,
  getPorts,
  getSession,
  getUserSettings,
  openDetectionStream,
  setChannelState,
  setLicenseInvalidHandler,
  setUnauthorizedHandler,
  updateCompassSettings,
  updateDetectionSettings,
  updateDeceptionSettings,
  updateGPSSettings,
  updateUserSettings,
  uploadLicense,
} from "./api";
import { MESSAGE_PAGE_ORDER } from "./app/message-pages";
import { isDebugPage } from "./app/navigation";
import type { Banner } from "./app/types";
import { LoadingOverlay, PageLoading } from "./components/LoadingState";
import { Sidebar } from "./components/Sidebar";
import { VirtualKeyboard } from "./components/VirtualKeyboard";
import { getStoredLocale, persistLocale, supportedLocales } from "./i18n";
import { useHashPage } from "./hooks/useHashPage";
import { DeceptionReportsPage } from "./pages/DeceptionReportsPage";
import { FPVVideoRecordsPage } from "./pages/FPVVideoRecordsPage";
import { InterferencePage } from "./pages/InterferencePage";
import { InterferenceReportsPage } from "./pages/InterferenceReportsPage";
import { GPSRecordsPage } from "./pages/GPSRecordsPage";
import { IntrusionsPage } from "./pages/IntrusionsPage";
import { LicensePage } from "./pages/LicensePage";
import { MessagePage } from "./pages/MessagePage";
import { NetworkSettingsPage } from "./pages/NetworkSettingsPage";
import { ScreenPage } from "./pages/ScreenPage";
import { SettingsPage } from "./pages/SettingsPage";
import { UserSettingsPage } from "./pages/UserSettingsPage";
import { WhitelistPage } from "./pages/WhitelistPage";
import { getStoredSettings, persistSettings } from "./preferences";
import {
  COMPASS_SERIAL_PROFILE,
  DETECTION_DEFAULT_RX_BAUD_RATE,
  DETECTION_DEFAULT_TX_BAUD_RATE,
  FIXED_SERIAL_PROFILE,
  normalizeSerialBaudRate,
} from "./serial-profile";
import { referenceMapLayers } from "./pages/screenData";
import type {
  CompassSessionResponse,
  DetectionSessionResponse,
  DetectionRecord,
  DeceptionSessionResponse,
  DebugRecordPage,
  GpioChannel,
  GPSRecord,
  GPSSessionResponse,
  LicenseInfo,
  LocaleMeta,
  ParsedMessage,
  PortInfo,
  ScreenRuntimeStatus,
  UserSettings,
} from "./types";
import { cx } from "./utils/classnames";
import {
  clearDeveloperSession,
  readDeveloperSession,
  storeDeveloperSession,
  type DeveloperSession,
} from "./utils/developer";
import {
  getStoredVisibleLocales,
  normalizeVisibleLocales,
  persistVisibleLocales,
} from "./utils/locales";
import {
  getStoredVisibleMapLayers,
  normalizeVisibleMapLayers,
  persistVisibleMapLayers,
} from "./utils/mapLayers";
import { normalizeGpioChannels } from "./utils/gpioChannels";
import {
  dedupeById,
  dedupeDetections,
  dedupeGPSRecords,
  dedupeParsed,
  extractErrorMessage,
  compassSessionBannerKind,
  compassSessionBannerText,
  gpsSessionBannerKind,
  gpsSessionBannerText,
  resolveInitialPorts,
  resolveInitialGPSPorts,
  serialKey,
  sessionBannerKind,
  sessionBannerText,
} from "./utils/session";

function detectionBaudRates(source?: { baudRate?: number; rxBaudRate?: number; txBaudRate?: number }) {
  const legacyBaudRate = source?.baudRate ? normalizeSerialBaudRate(source.baudRate) : undefined;
  const rxBaudRate = normalizeSerialBaudRate(source?.rxBaudRate, legacyBaudRate ?? DETECTION_DEFAULT_RX_BAUD_RATE);
  const txBaudRate = normalizeSerialBaudRate(source?.txBaudRate, legacyBaudRate ?? DETECTION_DEFAULT_TX_BAUD_RATE);
  return { rxBaudRate, txBaudRate };
}

function App() {
  const { t, i18n } = useTranslation();
  const [page, navigate] = useHashPage();
  const [mobileSidebarOpen, setMobileSidebarOpen] = useState(false);
  const [locale, setLocale] = useState(() => getStoredLocale());
  const [storedSettings, setStoredSettings] = useState(() => getStoredSettings());
  const [meta, setMeta] = useState<LocaleMeta | null>(null);
  const [visibleLocales, setVisibleLocales] = useState<string[]>(() => getStoredVisibleLocales());
  const [visibleMapLayers, setVisibleMapLayers] = useState<string[]>(() => getStoredVisibleMapLayers());
  const [developerSession, setDeveloperSession] = useState<DeveloperSession | null>(() => readDeveloperSession());
  const [ports, setPorts] = useState<PortInfo[]>([]);
  const [session, setSession] = useState<DetectionSessionResponse | null>(null);
  const [gpsSession, setGPSSession] = useState<GPSSessionResponse | null>(null);
  const [deceptionSession, setDeceptionSession] = useState<DeceptionSessionResponse | null>(null);
  const [compassSession, setCompassSession] = useState<CompassSessionResponse | null>(null);
  const [adminScreenStatus, setAdminScreenStatus] = useState<ScreenRuntimeStatus | null>(null);
  const [detections, setDetections] = useState<DetectionRecord[]>([]);
  const [messages, setMessages] = useState<ParsedMessage[]>([]);
  const [gpsRecords, setGPSRecords] = useState<GPSRecord[]>([]);
  const [channels, setChannels] = useState<GpioChannel[]>([]);
  const [userSettings, setUserSettings] = useState<UserSettings>({});
  const [selectedReceivePort, setSelectedReceivePort] = useState("");
  const [selectedSendPort, setSelectedSendPort] = useState("");
  const [selectedDetectionRxBaudRate, setSelectedDetectionRxBaudRate] = useState(DETECTION_DEFAULT_RX_BAUD_RATE);
  const [selectedDetectionTxBaudRate, setSelectedDetectionTxBaudRate] = useState(DETECTION_DEFAULT_TX_BAUD_RATE);
  const [selectedGPSDataPort, setSelectedGPSDataPort] = useState("");
  const [selectedGPSControlPort, setSelectedGPSControlPort] = useState("");
  const [selectedDeceptionPort, setSelectedDeceptionPort] = useState("");
  const [selectedCompassPort, setSelectedCompassPort] = useState("");
  const [messageSearch, setMessageSearch] = useState("");
  const [banner, setBanner] = useState<Banner>({ kind: "idle", message: "" });
  const [gpsBanner, setGPSBanner] = useState<Banner>({ kind: "idle", message: "" });
  const [compassBanner, setCompassBanner] = useState<Banner>({ kind: "idle", message: "" });
  const [gpsRecordsBanner, setGPSRecordsBanner] = useState<Banner>({ kind: "idle", message: "" });
  const [licenseBanner, setLicenseBanner] = useState<Banner>({ kind: "idle", message: "" });
  const [license, setLicense] = useState<LicenseInfo | null>(null);
  const [licenseLoading, setLicenseLoading] = useState(true);
  const [runtimeLoading, setRuntimeLoading] = useState(false);
  const [gpsRecordsLoading, setGPSRecordsLoading] = useState(false);
  const [channelBusyId, setChannelBusyId] = useState("");
  const lastAppliedSerialRef = useRef("");
  const lastAppliedGPSRef = useRef("");
  const lastAppliedDeceptionRef = useRef("");
  const lastAppliedCompassRef = useRef("");
  const developerActive = Boolean(developerSession);
  const developerToken = developerSession?.token ?? "";
  const licenseValid = license?.valid === true;
  const licenseInvalid = license !== null && !licenseValid;
  const licenseRecoveryMode = licenseInvalid && !license.deviceSn;
  const debugAccessBlocked = licenseValid && !developerActive && isDebugPage(page);
  const needsRuntimeData = licenseValid && page !== "screen" && page !== "settings" && page !== "whitelist" && page !== "intrusions" && page !== "fpv-records" && page !== "deception-reports" && !debugAccessBlocked;
  const needsSerialRecoveryData = licenseRecoveryMode;
  const serialSettingsEnabled = needsRuntimeData || needsSerialRecoveryData;
  const deceptionReportsVisible = licenseValid ? adminScreenStatus?.deception.configured !== false : false;

  const syncSerialSelection = useCallback((receivePort: string, sendPort: string, rxBaudRate?: number, txBaudRate?: number) => {
    const nextReceivePort = receivePort.trim();
    const nextSendPort = sendPort.trim();
    const nextRxBaudRate = normalizeSerialBaudRate(rxBaudRate);
    const nextTxBaudRate = normalizeSerialBaudRate(txBaudRate, nextRxBaudRate);
    lastAppliedSerialRef.current = nextReceivePort || nextSendPort ? serialKey(nextReceivePort, nextSendPort, nextRxBaudRate, nextTxBaudRate) : "";
    setSelectedReceivePort(nextReceivePort);
    setSelectedSendPort(nextSendPort);
    setSelectedDetectionRxBaudRate(nextRxBaudRate);
    setSelectedDetectionTxBaudRate(nextTxBaudRate);
  }, []);

  const syncGPSSelection = useCallback((dataPort: string, controlPort: string) => {
    const nextDataPort = dataPort.trim();
    const nextControlPort = controlPort.trim();
    lastAppliedGPSRef.current = nextDataPort || nextControlPort ? serialKey(nextDataPort, nextControlPort) : "";
    setSelectedGPSDataPort(nextDataPort);
    setSelectedGPSControlPort(nextControlPort);
  }, []);

  const syncDeceptionSelection = useCallback((port: string) => {
    const nextPort = port.trim();
    lastAppliedDeceptionRef.current = nextPort;
    setSelectedDeceptionPort(nextPort);
  }, []);

  const syncCompassSelection = useCallback((port: string) => {
    const nextPort = port.trim();
    lastAppliedCompassRef.current = nextPort;
    setSelectedCompassPort(nextPort);
  }, []);

  const handleUnauthorized = useCallback((error: Error) => {
    clearDeveloperSession();
    setDeveloperSession(null);
    setSession(null);
    setGPSSession(null);
    setDeceptionSession(null);
    setCompassSession(null);
    setDetections([]);
    setMessages([]);
    setGPSRecords([]);
    setChannels([]);
    setChannelBusyId("");
    setRuntimeLoading(false);
    setGPSRecordsLoading(false);
    setBanner({ kind: "error", message: error.message || t("developerSessionExpired", { ns: "common" }) });
    setGPSBanner({ kind: "idle", message: "" });
    setCompassBanner({ kind: "idle", message: "" });
    setGPSRecordsBanner({ kind: "idle", message: "" });
    if (isDebugPage(page)) {
      navigate("screen");
    }
  }, [navigate, page, t]);

  useEffect(() => setUnauthorizedHandler(handleUnauthorized), [handleUnauthorized]);

  const loadLicenseStatus = useCallback(async () => {
    setLicenseLoading(true);
    try {
      const response = await getLicenseStatus(locale);
      setLicense(response);
      if (response.valid) {
        setLicenseBanner({ kind: "idle", message: "" });
      } else if (response.message) {
        setLicenseBanner({ kind: "error", message: response.message });
      }
    } catch (error) {
      setLicense({
        isPermanent: false,
        valid: false,
        code: "license_verification_failed",
        message: extractErrorMessage(error, t("unexpectedError", { ns: "common" })),
      });
      setLicenseBanner({ kind: "error", message: extractErrorMessage(error, t("unexpectedError", { ns: "common" })) });
    } finally {
      setLicenseLoading(false);
    }
  }, [locale, t]);

  const handleLicenseInvalid = useCallback((error: Error) => {
    const requestError = error as ApiRequestError;
    const detailLicense = requestError.details && typeof requestError.details === "object"
      ? requestError.details as Partial<LicenseInfo>
      : null;
    setLicense((current) => ({
      isPermanent: detailLicense?.isPermanent ?? current?.isPermanent ?? false,
      valid: false,
      deviceSn: detailLicense?.deviceSn ?? current?.deviceSn,
      customer: detailLicense?.customer ?? current?.customer,
      issuedAt: detailLicense?.issuedAt ?? current?.issuedAt,
      expiresAt: detailLicense?.expiresAt ?? current?.expiresAt,
      remainingDays: detailLicense?.remainingDays ?? current?.remainingDays,
      code: requestError.code ?? detailLicense?.code ?? "license_verification_failed",
      message: detailLicense?.message ?? error.message,
    }));
    setLicenseBanner({ kind: "error", message: error.message || t("license.invalid", { ns: "common" }) });
    setRuntimeLoading(false);
    setGPSRecordsLoading(false);
  }, [t]);

  useEffect(() => setLicenseInvalidHandler(handleLicenseInvalid), [handleLicenseInvalid]);

  const loadSerialRecoveryData = useCallback(async () => {
    setRuntimeLoading(true);
    setBanner({ kind: "loading", message: t("loading", { ns: "common" }) });
    try {
      const [metaRes, portsRes, sessionRes, settingsRes] = await Promise.all([
        getLocales(),
        getPorts(locale, developerToken),
        getSession(locale, developerToken),
        getDetectionSettings(locale, developerToken),
      ]);

      setMeta(metaRes);
      setPorts(portsRes.ports);
      setSession(sessionRes);
      const { receivePort, sendPort } = resolveInitialPorts(sessionRes, settingsRes, portsRes.ports);
      const baudRates = detectionBaudRates(sessionRes.baudRate || sessionRes.rxBaudRate || sessionRes.txBaudRate ? sessionRes : settingsRes);
      syncSerialSelection(receivePort, sendPort, baudRates.rxBaudRate, baudRates.txBaudRate);
      setBanner({
        kind: sessionBannerKind(sessionRes),
        message: sessionBannerText(sessionRes, sessionRes.active ? t("active", { ns: "common" }) : t("idle", { ns: "common" })),
      });
      await loadLicenseStatus();
    } catch (error) {
      setBanner({ kind: "error", message: extractErrorMessage(error, t("unexpectedError", { ns: "common" })) });
    } finally {
      setRuntimeLoading(false);
    }
  }, [developerToken, loadLicenseStatus, locale, syncSerialSelection, t]);

  const bootstrap = useCallback(async () => {
    setRuntimeLoading(true);
    setBanner({ kind: "loading", message: t("loading", { ns: "common" }) });
    try {
      const parsedPromise = developerActive
        ? getParsed(locale, developerToken, 400)
        : Promise.resolve({ items: [] as ParsedMessage[], count: 0 });
      const detectionsPromise = developerActive
        ? getDetections(locale, developerToken, 400)
        : Promise.resolve({ items: [] as DetectionRecord[], count: 0 });
      const channelsPromise = developerActive
        ? getChannels(locale, developerToken)
        : Promise.resolve({ channels: [] as GpioChannel[], count: 0 });
      const [
        metaRes,
        portsRes,
        sessionRes,
        gpsSessionRes,
        deceptionSessionRes,
        compassSessionRes,
        settingsRes,
        gpsSettingsRes,
        deceptionSettingsRes,
        compassSettingsRes,
        parsedRes,
        detectionsRes,
        channelsRes,
      ] = await Promise.all([
        getLocales(),
        getPorts(locale, developerToken),
        getSession(locale, developerToken),
        getGPSSession(locale, developerToken),
        getDeceptionSession(locale, developerToken),
        getCompassSession(locale, developerToken),
        getDetectionSettings(locale, developerToken),
        getGPSSettings(locale, developerToken),
        getDeceptionSettings(locale, developerToken),
        getCompassSettings(locale, developerToken),
        parsedPromise,
        detectionsPromise,
        channelsPromise,
      ]);

      setMeta(metaRes);
      setPorts(portsRes.ports);
      setSession(sessionRes);
      setGPSSession(gpsSessionRes);
      setDeceptionSession(deceptionSessionRes);
      setCompassSession(compassSessionRes);
      setMessages(parsedRes.items);
      setDetections(detectionsRes.items);
      setChannels(normalizeGpioChannels(channelsRes.channels));

      const { receivePort, sendPort } = resolveInitialPorts(sessionRes, settingsRes, portsRes.ports);
      const baudRates = detectionBaudRates(sessionRes.baudRate || sessionRes.rxBaudRate || sessionRes.txBaudRate ? sessionRes : settingsRes);
      syncSerialSelection(receivePort, sendPort, baudRates.rxBaudRate, baudRates.txBaudRate);
      const { dataPort, controlPort } = resolveInitialGPSPorts(gpsSessionRes, gpsSettingsRes, portsRes.ports);
      syncGPSSelection(dataPort, controlPort);
      syncDeceptionSelection(deceptionSessionRes.portName || deceptionSettingsRes.portName || "");
      syncCompassSelection(compassSessionRes.portName || compassSettingsRes.portName || "");
      setBanner({
        kind: sessionBannerKind(sessionRes),
        message: sessionBannerText(sessionRes, sessionRes.active ? t("active", { ns: "common" }) : t("idle", { ns: "common" })),
      });
      setGPSBanner({
        kind: gpsSessionBannerKind(gpsSessionRes),
        message: gpsSessionBannerText(gpsSessionRes, gpsSessionRes.active ? t("active", { ns: "common" }) : t("idle", { ns: "common" })),
      });
      setCompassBanner({
        kind: compassSessionBannerKind(compassSessionRes),
        message: compassSessionBannerText(compassSessionRes, compassSessionRes.active ? t("active", { ns: "common" }) : t("idle", { ns: "common" })),
      });
    } catch (error) {
      setBanner({ kind: "error", message: extractErrorMessage(error, t("unexpectedError", { ns: "common" })) });
    } finally {
      setRuntimeLoading(false);
    }
  }, [developerActive, developerToken, locale, syncCompassSelection, syncDeceptionSelection, syncGPSSelection, syncSerialSelection, t]);

  const loadGPSRecords = useCallback(async () => {
    if (!developerActive) {
      setGPSRecords([]);
      return;
    }
    setGPSRecordsLoading(true);
    setGPSRecordsBanner({ kind: "loading", message: t("loading", { ns: "common" }) });
    try {
      const response = await getGPSRecords(locale, developerToken, 200);
      setGPSRecords(response.items);
      setGPSRecordsBanner({ kind: "idle", message: "" });
    } catch (error) {
      setGPSRecordsBanner({ kind: "error", message: extractErrorMessage(error, t("unexpectedError", { ns: "common" })) });
    } finally {
      setGPSRecordsLoading(false);
    }
  }, [developerActive, developerToken, locale, t]);

  const loadUserSettings = useCallback(async () => {
    try {
      const response = await getUserSettings();
      setUserSettings(response);
    } catch {
      // 用户设置不影响主页面运行，失败时保留当前值。
    }
  }, []);

  const loadAdminScreenStatus = useCallback(async () => {
    try {
      const response = await getScreenStatus(locale);
      setAdminScreenStatus(response);
    } catch {
      // 状态只影响菜单显隐，失败时沿用当前菜单状态。
    }
  }, [locale]);

  const refreshChannels = useCallback(async () => {
    if (!developerActive) {
      setChannels([]);
      return;
    }
    const response = await getChannels(locale, developerToken);
    setChannels(normalizeGpioChannels(response.channels));
  }, [developerActive, developerToken, locale]);

  useEffect(() => {
    void i18n.changeLanguage(locale);
    persistLocale(locale);
  }, [i18n, locale]);

  const handleLocaleChange = useCallback(
    (nextLocale: string) => {
      setLocale(nextLocale);
      void i18n.changeLanguage(nextLocale);
      persistLocale(nextLocale);
    },
    [i18n],
  );

  useEffect(() => {
    void loadUserSettings();
  }, [loadUserSettings]);

  useEffect(() => {
    void loadLicenseStatus();
  }, [loadLicenseStatus]);

  useEffect(() => {
    if (!licenseValid || page === "screen") {
      return;
    }
    void loadAdminScreenStatus();
    const timer = window.setInterval(() => {
      void loadAdminScreenStatus();
    }, 5000);
    return () => window.clearInterval(timer);
  }, [licenseValid, loadAdminScreenStatus, page]);

  useEffect(() => {
    if (page !== "settings") {
      return;
    }
    void loadUserSettings();
    const timer = window.setInterval(() => {
      void loadUserSettings();
    }, 3000);
    return () => window.clearInterval(timer);
  }, [loadUserSettings, page]);

  useEffect(() => {
    if (!needsSerialRecoveryData) {
      return;
    }
    let cancelled = false;
    const load = async () => {
      if (cancelled) {
        return;
      }
      await loadSerialRecoveryData();
    };
    void load();
    return () => {
      cancelled = true;
    };
  }, [loadSerialRecoveryData, needsSerialRecoveryData]);

  useEffect(() => {
    if (!licenseRecoveryMode) {
      return;
    }
    const timer = window.setInterval(() => {
      void loadLicenseStatus();
    }, 3000);
    return () => window.clearInterval(timer);
  }, [licenseRecoveryMode, loadLicenseStatus]);

  useEffect(() => {
    if (!needsRuntimeData) {
      return;
    }
    let cancelled = false;
    const load = async () => {
      if (cancelled) {
        return;
      }
      await bootstrap();
    };
    void load();
    return () => {
      cancelled = true;
    };
  }, [bootstrap, needsRuntimeData]);

  useEffect(() => {
    if (!licenseValid || page !== "gps-records" || !developerActive) {
      return;
    }
    void loadGPSRecords();
  }, [developerActive, licenseValid, loadGPSRecords, page]);

  useEffect(() => {
    if (!licenseValid || page !== "interference" || !developerActive) {
      return;
    }

    let syncing = false;
    const sync = async () => {
      if (syncing) {
        return;
      }
      syncing = true;
      try {
        await refreshChannels();
      } catch {
        // 保持当前显示，下一次轮询或页面刷新会再次同步。
      } finally {
        syncing = false;
      }
    };

    void sync();
    const timer = window.setInterval(() => {
      void sync();
    }, 1_000);
    return () => window.clearInterval(timer);
  }, [developerActive, licenseValid, page, refreshChannels]);

  useEffect(() => {
    if (!serialSettingsEnabled) {
      return;
    }
    const receivePort = selectedReceivePort.trim();
    const sendPort = selectedSendPort.trim();
    const rxBaudRate = normalizeSerialBaudRate(selectedDetectionRxBaudRate);
    const txBaudRate = normalizeSerialBaudRate(selectedDetectionTxBaudRate, rxBaudRate);
    const currentKey = receivePort || sendPort ? serialKey(receivePort, sendPort, rxBaudRate, txBaudRate) : "";
    if (currentKey === lastAppliedSerialRef.current) {
      return;
    }

    const timer = window.setTimeout(() => {
      void (async () => {
        try {
          setBanner({ kind: "loading", message: t("loading", { ns: "common" }) });
          const response = await updateDetectionSettings(
            {
              portName: receivePort,
              rxPortName: receivePort,
              txPortName: sendPort,
              baudRate: rxBaudRate,
              rxBaudRate,
              txBaudRate,
              dataBits: FIXED_SERIAL_PROFILE.dataBits,
              stopBits: FIXED_SERIAL_PROFILE.stopBits,
              parity: FIXED_SERIAL_PROFILE.parity,
              readTimeoutMs: FIXED_SERIAL_PROFILE.readTimeoutMs,
              autoConnect: true,
            },
            locale,
            developerToken,
          );
          lastAppliedSerialRef.current = currentKey;
          setSession(response);
          setBanner({
            kind: sessionBannerKind(response),
            message: sessionBannerText(response, response.message || t("active", { ns: "common" })),
          });
          if (licenseValid) {
            await bootstrap();
          } else {
            await loadSerialRecoveryData();
          }
        } catch (error) {
          setBanner({ kind: "error", message: extractErrorMessage(error, t("unexpectedError", { ns: "common" })) });
        }
      })();
    }, 350);

    return () => window.clearTimeout(timer);
  }, [bootstrap, developerToken, licenseValid, loadSerialRecoveryData, locale, selectedDetectionRxBaudRate, selectedDetectionTxBaudRate, selectedReceivePort, selectedSendPort, serialSettingsEnabled, t]);

  useEffect(() => {
    if (!needsRuntimeData) {
      return;
    }
    const dataPort = selectedGPSDataPort.trim();
    const controlPort = selectedGPSControlPort.trim();

    const currentKey = dataPort || controlPort ? serialKey(dataPort, controlPort) : "";
    if (currentKey === lastAppliedGPSRef.current) {
      return;
    }

    const timer = window.setTimeout(() => {
      void (async () => {
        try {
          setGPSBanner({ kind: "loading", message: t("loading", { ns: "common" }) });
          const response = await updateGPSSettings(
            {
              portName: dataPort,
              dataPortName: dataPort,
              controlPortName: controlPort,
              baudRate: FIXED_SERIAL_PROFILE.baudRate,
              dataBits: FIXED_SERIAL_PROFILE.dataBits,
              stopBits: FIXED_SERIAL_PROFILE.stopBits,
              parity: FIXED_SERIAL_PROFILE.parity,
              readTimeoutMs: FIXED_SERIAL_PROFILE.readTimeoutMs,
              autoConnect: true,
            },
            locale,
            developerToken,
          );
          lastAppliedGPSRef.current = currentKey;
          setGPSSession(response);
          setGPSBanner({
            kind: gpsSessionBannerKind(response),
            message: gpsSessionBannerText(response, response.message || t("active", { ns: "common" })),
          });
          await bootstrap();
        } catch (error) {
          setGPSBanner({ kind: "error", message: extractErrorMessage(error, t("unexpectedError", { ns: "common" })) });
        }
      })();
    }, 350);

    return () => window.clearTimeout(timer);
  }, [bootstrap, developerToken, locale, needsRuntimeData, selectedGPSControlPort, selectedGPSDataPort, t]);

  useEffect(() => {
    if (!needsRuntimeData) {
      return;
    }
    const portName = selectedDeceptionPort.trim();
    if (portName === lastAppliedDeceptionRef.current) {
      return;
    }

    const timer = window.setTimeout(() => {
      void (async () => {
        try {
          setBanner({ kind: "loading", message: t("loading", { ns: "common" }) });
          const response = await updateDeceptionSettings(
            {
              portName,
              baudRate: FIXED_SERIAL_PROFILE.baudRate,
              dataBits: FIXED_SERIAL_PROFILE.dataBits,
              stopBits: FIXED_SERIAL_PROFILE.stopBits,
              parity: FIXED_SERIAL_PROFILE.parity,
              readTimeoutMs: FIXED_SERIAL_PROFILE.readTimeoutMs,
              autoConnect: true,
            },
            locale,
            developerToken,
          );
          lastAppliedDeceptionRef.current = portName;
          setDeceptionSession(response);
          setBanner({ kind: response.active ? "success" : "idle", message: response.message || t("active", { ns: "common" }) });
          await bootstrap();
        } catch (error) {
          setBanner({ kind: "error", message: extractErrorMessage(error, t("unexpectedError", { ns: "common" })) });
        }
      })();
    }, 350);

    return () => window.clearTimeout(timer);
  }, [bootstrap, developerToken, locale, needsRuntimeData, selectedDeceptionPort, t]);

  useEffect(() => {
    if (!needsRuntimeData) {
      return;
    }
    const portName = selectedCompassPort.trim();
    if (portName === lastAppliedCompassRef.current) {
      return;
    }

    const timer = window.setTimeout(() => {
      void (async () => {
        try {
          setCompassBanner({ kind: "loading", message: t("loading", { ns: "common" }) });
          const response = await updateCompassSettings(
            {
              portName,
              baudRate: COMPASS_SERIAL_PROFILE.baudRate,
              dataBits: COMPASS_SERIAL_PROFILE.dataBits,
              stopBits: COMPASS_SERIAL_PROFILE.stopBits,
              parity: COMPASS_SERIAL_PROFILE.parity,
              readTimeoutMs: COMPASS_SERIAL_PROFILE.readTimeoutMs,
              autoConnect: true,
            },
            locale,
            developerToken,
          );
          lastAppliedCompassRef.current = portName;
          setCompassSession(response);
          setCompassBanner({
            kind: compassSessionBannerKind(response),
            message: compassSessionBannerText(response, response.message || t("active", { ns: "common" })),
          });
          await bootstrap();
        } catch (error) {
          setCompassBanner({ kind: "error", message: extractErrorMessage(error, t("unexpectedError", { ns: "common" })) });
        }
      })();
    }, 350);

    return () => window.clearTimeout(timer);
  }, [bootstrap, developerToken, locale, needsRuntimeData, selectedCompassPort, t]);

  useEffect(() => {
    if (!needsRuntimeData) {
      return;
    }
    const close = openDetectionStream(locale, developerToken, {
      onSessionStarted: (event) => {
        if (event.payload) {
          setSession(event.payload);
          const nextReceivePort = event.payload.rxPortName || event.payload.portName || "";
          const nextSendPort = event.payload.txPortName || nextReceivePort || "";
          if (nextReceivePort || nextSendPort) {
            const baudRates = detectionBaudRates(event.payload);
            syncSerialSelection(nextReceivePort, nextSendPort || nextReceivePort, baudRates.rxBaudRate, baudRates.txBaudRate);
          }
          setBanner({
            kind: sessionBannerKind(event.payload),
            message: sessionBannerText(event.payload, t("active", { ns: "common" })),
          });
        }
      },
      onSessionStopped: (event) => {
        if (event.payload) {
          setSession(event.payload);
          const nextReceivePort = event.payload.rxPortName || event.payload.portName || "";
          const nextSendPort = event.payload.txPortName || nextReceivePort || "";
          if (nextReceivePort || nextSendPort) {
            const baudRates = detectionBaudRates(event.payload);
            syncSerialSelection(nextReceivePort, nextSendPort || nextReceivePort, baudRates.rxBaudRate, baudRates.txBaudRate);
          }
          setBanner({
            kind: sessionBannerKind(event.payload),
            message: sessionBannerText(event.payload, t("idle", { ns: "common" })),
          });
        }
      },
      onSessionState: (event) => {
        if (event.payload) {
          setSession(event.payload);
          const nextReceivePort = event.payload.rxPortName || event.payload.portName || "";
          const nextSendPort = event.payload.txPortName || nextReceivePort || "";
          if (nextReceivePort || nextSendPort) {
            const baudRates = detectionBaudRates(event.payload);
            syncSerialSelection(nextReceivePort, nextSendPort || nextReceivePort, baudRates.rxBaudRate, baudRates.txBaudRate);
          }
          setBanner({
            kind: sessionBannerKind(event.payload),
            message: sessionBannerText(event.payload, t("loading", { ns: "common" })),
          });
        }
      },
      onGPSSessionStarted: (event) => {
        if (event.payload) {
          setGPSSession(event.payload);
          const nextDataPort = event.payload.dataPortName || event.payload.portName || "";
          const nextControlPort = event.payload.controlPortName || nextDataPort || "";
          if (nextDataPort || nextControlPort) {
            syncGPSSelection(nextDataPort, nextControlPort || nextDataPort);
          }
          setGPSBanner({
            kind: gpsSessionBannerKind(event.payload),
            message: gpsSessionBannerText(event.payload, t("active", { ns: "common" })),
          });
        }
      },
      onGPSSessionStopped: (event) => {
        if (event.payload) {
          setGPSSession(event.payload);
          const nextDataPort = event.payload.dataPortName || event.payload.portName || "";
          const nextControlPort = event.payload.controlPortName || nextDataPort || "";
          if (nextDataPort || nextControlPort) {
            syncGPSSelection(nextDataPort, nextControlPort || nextDataPort);
          }
          setGPSBanner({
            kind: gpsSessionBannerKind(event.payload),
            message: gpsSessionBannerText(event.payload, t("idle", { ns: "common" })),
          });
        }
      },
      onGPSSessionState: (event) => {
        if (event.payload) {
          setGPSSession(event.payload);
          const nextDataPort = event.payload.dataPortName || event.payload.portName || "";
          const nextControlPort = event.payload.controlPortName || nextDataPort || "";
          if (nextDataPort || nextControlPort) {
            syncGPSSelection(nextDataPort, nextControlPort || nextDataPort);
          }
          setGPSBanner({
            kind: gpsSessionBannerKind(event.payload),
            message: gpsSessionBannerText(event.payload, t("loading", { ns: "common" })),
          });
        }
      },
      onDeceptionSessionStarted: (event) => {
        if (event.payload) {
          setDeceptionSession(event.payload);
          syncDeceptionSelection(event.payload.portName || "");
        }
      },
      onDeceptionSessionStopped: (event) => {
        if (event.payload) {
          setDeceptionSession(event.payload);
          syncDeceptionSelection(event.payload.portName || "");
        }
      },
      onDeceptionSessionState: (event) => {
        if (event.payload) {
          setDeceptionSession(event.payload);
          syncDeceptionSelection(event.payload.portName || "");
        }
      },
      onCompassSessionStarted: (event) => {
        if (event.payload) {
          setCompassSession(event.payload);
          syncCompassSelection(event.payload.portName || "");
          setCompassBanner({
            kind: compassSessionBannerKind(event.payload),
            message: compassSessionBannerText(event.payload, t("active", { ns: "common" })),
          });
        }
      },
      onCompassSessionStopped: (event) => {
        if (event.payload) {
          setCompassSession(event.payload);
          syncCompassSelection(event.payload.portName || "");
          setCompassBanner({
            kind: compassSessionBannerKind(event.payload),
            message: compassSessionBannerText(event.payload, t("idle", { ns: "common" })),
          });
        }
      },
      onCompassSessionState: (event) => {
        if (event.payload) {
          setCompassSession(event.payload);
          syncCompassSelection(event.payload.portName || "");
          setCompassBanner({
            kind: compassSessionBannerKind(event.payload),
            message: compassSessionBannerText(event.payload, t("loading", { ns: "common" })),
          });
        }
      },
      onCompassRecord: (event) => {
        if (event.payload) {
          setCompassSession((current) => current ? {
            ...current,
            lastRecord: event.payload!,
            lastPitch: event.payload!.pitch,
            lastRoll: event.payload!.roll,
            lastHeading: event.payload!.heading,
            lastRawHex: event.payload!.rawHex,
            lastUpdatedAt: event.payload!.receivedAt,
          } : current);
        }
      },
      onGPSRecord: (event) => {
        if (event.payload) {
          setGPSSession((current) => current ? {
            ...current,
            lastNmea: event.payload!.raw,
            lastFix: event.payload!.fix ?? current.lastFix,
            lastRecord: event.payload!,
          } : current);
          if (developerActive) {
            setGPSRecords((items) => dedupeGPSRecords(items, event.payload!, 200));
          }
        }
      },
      onParsed: (event) => {
        if (licenseValid && developerActive && event.payload) {
          setMessages((items) => dedupeParsed(items, event.payload!, 400));
        }
      },
      onDetection: (event) => {
        if (licenseValid && developerActive && event.payload) {
          setDetections((items) => dedupeDetections(items, event.payload!, 400));
        }
      },
      onChannelUpdated: (event) => {
        if (licenseValid && developerActive && event.payload) {
          setChannels((items) => normalizeGpioChannels(dedupeById(items, event.payload!, 16)));
        }
      },
      onError: (error) => {
        setBanner({ kind: "error", message: error.message });
      },
    });

    return close;
  }, [developerActive, developerToken, licenseValid, locale, needsRuntimeData, syncCompassSelection, syncDeceptionSelection, syncGPSSelection, syncSerialSelection, t]);

  const sessionActive = Boolean(session?.active);
  const sessionStateLabel = session
    ? sessionBannerText(session, sessionActive ? t("active", { ns: "common" }) : t("idle", { ns: "common" }))
    : t("idle", { ns: "common" });
  const currentReceivePort = session?.rxPortName || session?.portName || selectedReceivePort;
  const currentSendPort = session?.txPortName || selectedSendPort;
  const currentDetectionRxBaudRate = session?.rxBaudRate || session?.baudRate || selectedDetectionRxBaudRate;
  const currentDetectionTxBaudRate = session?.txBaudRate || session?.baudRate || selectedDetectionTxBaudRate;
  const gpsActive = Boolean(gpsSession?.active);
  const gpsSessionStateLabel = gpsSession
    ? gpsSessionBannerText(gpsSession, gpsActive ? t("active", { ns: "common" }) : t("idle", { ns: "common" }))
    : t("idle", { ns: "common" });
  const currentGPSDataPort = gpsSession?.dataPortName || gpsSession?.portName || selectedGPSDataPort;
  const currentGPSControlPort = gpsSession?.controlPortName || selectedGPSControlPort;
  const deceptionActive = Boolean(deceptionSession?.active);
  const deceptionSessionStateLabel = deceptionSession
    ? deceptionSession.message || (deceptionActive ? t("active", { ns: "common" }) : t("idle", { ns: "common" }))
    : t("idle", { ns: "common" });
  const currentDeceptionPort = deceptionSession?.portName || selectedDeceptionPort;
  const compassActive = Boolean(compassSession?.active);
  const compassSessionStateLabel = compassSession
    ? compassSessionBannerText(compassSession, compassActive ? t("active", { ns: "common" }) : t("idle", { ns: "common" }))
    : t("idle", { ns: "common" });
  const currentCompassPort = compassSession?.portName || selectedCompassPort;
  const defaultAppTitle = t("app.title", { ns: "common" });
  const appTitle = storedSettings.appTitle.trim() || defaultAppTitle;
  const allLocaleOptions = meta?.supportedLocales.length ? meta.supportedLocales : supportedLocales;
  const localeOptions = normalizeVisibleLocales(allLocaleOptions, visibleLocales, locale);
  const allMapLayerOptions = referenceMapLayers;
  const mapLayerOptions = useMemo(
    () => normalizeVisibleMapLayers(allMapLayerOptions, visibleMapLayers),
    [allMapLayerOptions, visibleMapLayers],
  );
  const developerExpiresAt = developerSession?.expiresAt ?? 0;
  const isMessagePage = developerActive && MESSAGE_PAGE_ORDER.includes(page as DebugRecordPage);
  const loadingLabel = t("loading", { ns: "common" });
  const hasRuntimeData = Boolean(meta || ports.length > 0 || session || gpsSession || compassSession);
  const showRuntimeFallback = needsRuntimeData && runtimeLoading && !hasRuntimeData;
  const operationInFlight = needsRuntimeData && (
    runtimeLoading ||
    gpsRecordsLoading ||
    Boolean(channelBusyId) ||
    banner.kind === "loading" ||
    gpsBanner.kind === "loading" ||
    compassBanner.kind === "loading"
  );

  useEffect(() => {
    document.title = appTitle;
  }, [appTitle]);

  useEffect(() => {
    const normalized = normalizeVisibleLocales(allLocaleOptions, visibleLocales, locale);
    if (normalized.join("|") !== visibleLocales.join("|")) {
      setVisibleLocales(normalized);
      persistVisibleLocales(normalized);
    }
  }, [allLocaleOptions, locale, visibleLocales]);

  useEffect(() => {
    const normalized = normalizeVisibleMapLayers(allMapLayerOptions, visibleMapLayers);
    if (normalized.join("|") !== visibleMapLayers.join("|")) {
      setVisibleMapLayers(normalized);
      persistVisibleMapLayers(normalized);
    }
  }, [allMapLayerOptions, visibleMapLayers]);

  useEffect(() => {
    const sync = () => {
      setDeveloperSession(readDeveloperSession());
    };
    sync();
    const timer = window.setInterval(sync, 15_000);
    return () => window.clearInterval(timer);
  }, []);

  useEffect(() => {
    if (debugAccessBlocked) {
      navigate("screen");
    }
  }, [debugAccessBlocked, navigate]);

  useEffect(() => {
    if (page === "deception-reports" && !deceptionReportsVisible) {
      navigate("screen");
    }
  }, [deceptionReportsVisible, navigate, page]);

  useEffect(() => {
    setMobileSidebarOpen(false);
  }, [page]);

  const handleToggleChannel = async (channel: GpioChannel) => {
    setChannelBusyId(channel.id);
    setBanner({ kind: "loading", message: t("loading", { ns: "common" }) });
    try {
      const response = await setChannelState(channel.id, { enabled: !channel.enabled }, locale, developerToken);
      setChannels((items) => normalizeGpioChannels(dedupeById(items, response.channel, 16)));
      setBanner({ kind: "success", message: response.message });
    } catch (error) {
      setBanner({ kind: "error", message: extractErrorMessage(error, t("unexpectedError", { ns: "common" })) });
    } finally {
      setChannelBusyId("");
    }
  };

  const handleVisibleLocalesChange = (nextLocales: string[]) => {
    const normalized = normalizeVisibleLocales(allLocaleOptions, nextLocales, locale);
    setVisibleLocales(normalized);
    persistVisibleLocales(normalized);
  };

  const handleVisibleMapLayersChange = (nextLayers: string[]) => {
    const normalized = normalizeVisibleMapLayers(allMapLayerOptions, nextLayers);
    setVisibleMapLayers(normalized);
    persistVisibleMapLayers(normalized);
  };

  const handleAppTitleChange = (value: string) => {
    const nextSettings = {
      ...storedSettings,
      appTitle: value.trim(),
    };
    setStoredSettings(nextSettings);
    persistSettings(nextSettings);
  };

  const handleUserSettingsChange = async (nextSettings: UserSettings) => {
    const response = await updateUserSettings(nextSettings, locale);
    setUserSettings(response);
    return response;
  };

  const handleDeveloperLogin = async (code: string) => {
    const response = await createDeveloperSessionRequest({ code }, locale);
    const nextSession = storeDeveloperSession({
      token: response.token,
      expiresAt: response.expiresAt,
    });
    setDeveloperSession(nextSession);
  };

  const handleDeveloperLogout = async () => {
    const currentSession = readDeveloperSession();
    clearDeveloperSession();
    setDeveloperSession(null);
    if (isDebugPage(page)) {
      navigate("screen");
    }
    if (currentSession?.token) {
      try {
        await deleteDeveloperSessionRequest(currentSession.token, locale);
      } catch {
        // 退出本地开发者状态优先，后端会话会在短时间内过期。
      }
    }
  };

  const handleUploadLicense = async (file: File) => {
    const response = await uploadLicense(file, locale);
    setLicense(response.license);
    setLicenseBanner({ kind: "success", message: response.message });
  };

  if (licenseLoading && license === null) {
    return <PageLoading label={loadingLabel} />;
  }

  if (!licenseValid) {
    return (
      <LicensePage
        appTitle={appTitle}
        license={license}
        loading={licenseLoading}
        locale={locale}
        ports={ports}
        banner={licenseBanner.message ? licenseBanner : banner}
        session={session}
        selectedReceivePort={selectedReceivePort}
        selectedSendPort={selectedSendPort}
        selectedDetectionRxBaudRate={selectedDetectionRxBaudRate}
        selectedDetectionTxBaudRate={selectedDetectionTxBaudRate}
        t={t}
        onRefreshLicense={loadLicenseStatus}
        onUploadLicense={handleUploadLicense}
        onRefreshPorts={loadSerialRecoveryData}
        onReceivePortChange={setSelectedReceivePort}
        onSendPortChange={setSelectedSendPort}
        onDetectionRxBaudRateChange={setSelectedDetectionRxBaudRate}
        onDetectionTxBaudRateChange={setSelectedDetectionTxBaudRate}
      />
    );
  }

  if (page === "screen") {
    return (
      <ScreenPage
        appTitle={appTitle}
        t={t}
        locale={locale}
        localeOptions={localeOptions}
        developerActive={developerActive}
        visibleMapLayers={mapLayerOptions}
        userSettings={userSettings}
        onLocaleChange={handleLocaleChange}
        onUserSettingsChange={handleUserSettingsChange}
      />
    );
  }

  return (
    <div className="admin-shell h-dvh overflow-hidden bg-base-100 text-base-content">
      <div className={cx("app-top-progress", operationInFlight && "app-top-progress--active")} aria-hidden="true" />
      <div className="grid h-full min-h-0 grid-cols-1 grid-rows-[auto_minmax(0,1fr)] gap-0 overflow-hidden p-0 md:gap-2 md:p-2 xl:grid-cols-[244px_minmax(0,1fr)] xl:grid-rows-[minmax(0,1fr)] xl:gap-3 xl:p-3">
        <Sidebar
          appTitle={appTitle}
          page={page}
          locale={locale}
          localeOptions={localeOptions}
          developerActive={developerActive}
          developerExpiresAt={developerExpiresAt}
          mobileOpen={mobileSidebarOpen}
          t={t}
          onLocaleChange={handleLocaleChange}
          onNavigate={navigate}
          onMobileClose={() => setMobileSidebarOpen(false)}
          onMobileOpen={() => setMobileSidebarOpen(true)}
          onDeveloperLogin={handleDeveloperLogin}
          onDeveloperLogout={() => void handleDeveloperLogout()}
          showDeceptionReports={deceptionReportsVisible}
        />

        <div className="flex min-h-0 min-w-0 flex-col overflow-hidden">
          <main
            className={cx(
              "app-page-shell flex min-h-0 min-w-0 flex-1 flex-col gap-3 overflow-x-hidden",
              "p-2 md:p-0",
              isMessagePage ? "overflow-hidden" : "overflow-y-auto",
            )}
          >
            {showRuntimeFallback ? (
              <PageLoading label={loadingLabel} />
            ) : (
              <>
                {isMessagePage ? (
                  <MessagePage
                    page={page as DebugRecordPage}
                    records={page === "detection-records" ? detections : messages}
                    locale={locale}
                    query={messageSearch}
                    onQueryChange={setMessageSearch}
                    t={t}
                  />
                ) : null}

                {page === "interference" ? (
                  developerActive ? (
                    <InterferencePage
                      channels={channels}
                      busyChannelId={channelBusyId}
                      t={t}
                      onToggleChannel={(channel) => void handleToggleChannel(channel)}
                    />
                  ) : null
                ) : null}

                {page === "gps-records" && developerActive ? (
                  <GPSRecordsPage
                    records={gpsRecords}
                    banner={gpsRecordsBanner}
                    loading={gpsRecordsLoading}
                    locale={locale}
                    t={t}
                    onRefresh={() => void loadGPSRecords()}
                  />
                ) : null}

                {page === "intrusions" ? (
                  <IntrusionsPage
                    locale={locale}
                    userSettings={userSettings}
                    t={t}
                    onUserSettingsChange={handleUserSettingsChange}
                  />
                ) : null}

                {page === "fpv-records" ? (
                  <FPVVideoRecordsPage
                    locale={locale}
                    t={t}
                  />
                ) : null}

                {page === "deception-reports" ? (
                  <DeceptionReportsPage
                    locale={locale}
                    t={t}
                  />
                ) : null}

                {page === "interference-reports" ? (
                  <InterferenceReportsPage
                    locale={locale}
                    userSettings={userSettings}
                    t={t}
                  />
                ) : null}

                {page === "settings" ? (
                  <UserSettingsPage
                    appTitle={appTitle}
                    defaultAppTitle={defaultAppTitle}
                    userSettings={userSettings}
                    t={t}
                    onAppTitleChange={handleAppTitleChange}
                    onUserSettingsChange={handleUserSettingsChange}
                  />
                ) : null}

                {page === "whitelist" ? (
                  <WhitelistPage
                    locale={locale}
                    userSettings={userSettings}
                    t={t}
                    onUserSettingsChange={handleUserSettingsChange}
                  />
                ) : null}

                {page === "developer-settings" && developerActive ? (
                  <SettingsPage
                    banner={banner}
                    ports={ports}
                    selectedReceivePort={selectedReceivePort}
                    selectedSendPort={selectedSendPort}
                    selectedDetectionRxBaudRate={selectedDetectionRxBaudRate}
                    selectedDetectionTxBaudRate={selectedDetectionTxBaudRate}
                    selectedGPSDataPort={selectedGPSDataPort}
                    selectedGPSControlPort={selectedGPSControlPort}
                    selectedDeceptionPort={selectedDeceptionPort}
                    selectedCompassPort={selectedCompassPort}
                    sessionStateLabel={sessionStateLabel}
                    currentReceivePort={currentReceivePort}
                    currentSendPort={currentSendPort}
                    currentDetectionRxBaudRate={currentDetectionRxBaudRate}
                    currentDetectionTxBaudRate={currentDetectionTxBaudRate}
                    gpsBanner={gpsBanner}
                    gpsSession={gpsSession}
                    gpsSessionStateLabel={gpsSessionStateLabel}
                    currentGPSDataPort={currentGPSDataPort}
                    currentGPSControlPort={currentGPSControlPort}
                    deceptionSession={deceptionSession}
                    deceptionSessionStateLabel={deceptionSessionStateLabel}
                    currentDeceptionPort={currentDeceptionPort}
                    compassBanner={compassBanner}
                    compassSession={compassSession}
                    compassSessionStateLabel={compassSessionStateLabel}
                    currentCompassPort={currentCompassPort}
                    allLocaleOptions={allLocaleOptions}
                    visibleLocales={localeOptions}
                    currentLocale={locale}
                    allMapLayerOptions={allMapLayerOptions}
                    visibleMapLayers={mapLayerOptions}
                    userSettings={userSettings}
                    t={t}
                    onRefresh={() => void bootstrap()}
                    onReceivePortChange={setSelectedReceivePort}
                    onSendPortChange={setSelectedSendPort}
                    onDetectionRxBaudRateChange={setSelectedDetectionRxBaudRate}
                    onDetectionTxBaudRateChange={setSelectedDetectionTxBaudRate}
                    onGPSDataPortChange={setSelectedGPSDataPort}
                    onGPSControlPortChange={setSelectedGPSControlPort}
                    onDeceptionPortChange={setSelectedDeceptionPort}
                    onCompassPortChange={setSelectedCompassPort}
                    onUserSettingsChange={handleUserSettingsChange}
                    onVisibleLocalesChange={handleVisibleLocalesChange}
                    onVisibleMapLayersChange={handleVisibleMapLayersChange}
                  />
                ) : null}

                {page === "network-settings" && developerActive ? (
                  <NetworkSettingsPage
                    locale={locale}
                    developerToken={developerToken}
                    t={t}
                  />
                ) : null}
              </>
            )}
          </main>
        </div>
      </div>
      <LoadingOverlay active={operationInFlight && !showRuntimeFallback} label={loadingLabel} />
      <VirtualKeyboard locale={locale} localeOptions={localeOptions} />
    </div>
  );
}

export default App;
