import { useCallback, useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";

import {
  createDeveloperSessionRequest,
  deleteDeveloperSessionRequest,
  getChannels,
  getDetectionSettings,
  getGPSSession,
  getGPSSettings,
  getLocales,
  getParsed,
  getPorts,
  getSession,
  openDetectionStream,
  setChannelState,
  updateDetectionSettings,
  updateGPSSettings,
} from "./api";
import { MESSAGE_PAGE_ORDER } from "./app/message-pages";
import { isDebugPage } from "./app/navigation";
import type { Banner } from "./app/types";
import { Sidebar } from "./components/Sidebar";
import { VirtualKeyboard } from "./components/VirtualKeyboard";
import { getStoredLocale, persistLocale, supportedLocales } from "./i18n";
import { useHashPage } from "./hooks/useHashPage";
import { InterferencePage } from "./pages/InterferencePage";
import { MessagePage } from "./pages/MessagePage";
import { NetworkSettingsPage } from "./pages/NetworkSettingsPage";
import { ScreenPage } from "./pages/ScreenPage";
import { SettingsPage } from "./pages/SettingsPage";
import { UserSettingsPage } from "./pages/UserSettingsPage";
import { getStoredSettings, persistSettings } from "./preferences";
import { FIXED_SERIAL_PROFILE } from "./serial-profile";
import type {
  DetectionSessionResponse,
  GpioChannel,
  GPSSessionResponse,
  LocaleMeta,
  ParsedMessage,
  ParsedMessageType,
  PortInfo,
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
  dedupeById,
  dedupeParsed,
  extractErrorMessage,
  gpsSessionBannerKind,
  gpsSessionBannerText,
  resolveInitialPorts,
  resolveInitialGPSPorts,
  serialKey,
  sessionBannerKind,
  sessionBannerText,
} from "./utils/session";

function App() {
  const { t, i18n } = useTranslation();
  const [page, navigate] = useHashPage();
  const [locale, setLocale] = useState(() => getStoredLocale());
  const [storedSettings, setStoredSettings] = useState(() => getStoredSettings());
  const [meta, setMeta] = useState<LocaleMeta | null>(null);
  const [visibleLocales, setVisibleLocales] = useState<string[]>(() => getStoredVisibleLocales());
  const [developerSession, setDeveloperSession] = useState<DeveloperSession | null>(() => readDeveloperSession());
  const [ports, setPorts] = useState<PortInfo[]>([]);
  const [session, setSession] = useState<DetectionSessionResponse | null>(null);
  const [gpsSession, setGPSSession] = useState<GPSSessionResponse | null>(null);
  const [messages, setMessages] = useState<ParsedMessage[]>([]);
  const [channels, setChannels] = useState<GpioChannel[]>([]);
  const [selectedReceivePort, setSelectedReceivePort] = useState("");
  const [selectedSendPort, setSelectedSendPort] = useState("");
  const [selectedGPSDataPort, setSelectedGPSDataPort] = useState("");
  const [selectedGPSControlPort, setSelectedGPSControlPort] = useState("");
  const [messageSearch, setMessageSearch] = useState("");
  const [banner, setBanner] = useState<Banner>({ kind: "idle", message: "" });
  const [gpsBanner, setGPSBanner] = useState<Banner>({ kind: "idle", message: "" });
  const lastAppliedSerialRef = useRef("");
  const lastAppliedGPSRef = useRef("");
  const developerActive = Boolean(developerSession);
  const developerToken = developerSession?.token ?? "";
  const debugAccessBlocked = !developerActive && isDebugPage(page);
  const needsRuntimeData = page !== "screen" && page !== "settings" && !debugAccessBlocked;

  const syncSerialSelection = useCallback((receivePort: string, sendPort: string) => {
    const nextReceivePort = receivePort.trim();
    const nextSendPort = sendPort.trim();
    lastAppliedSerialRef.current = serialKey(nextReceivePort, nextSendPort);
    setSelectedReceivePort(nextReceivePort);
    setSelectedSendPort(nextSendPort);
  }, []);

  const syncGPSSelection = useCallback((dataPort: string, controlPort: string) => {
    const nextDataPort = dataPort.trim();
    const nextControlPort = controlPort.trim();
    lastAppliedGPSRef.current = serialKey(nextDataPort, nextControlPort);
    setSelectedGPSDataPort(nextDataPort);
    setSelectedGPSControlPort(nextControlPort);
  }, []);

  const bootstrap = useCallback(async () => {
    setBanner({ kind: "loading", message: t("loading", { ns: "common" }) });
    try {
      const parsedPromise = developerActive
        ? getParsed(locale, developerToken, 400)
        : Promise.resolve({ items: [] as ParsedMessage[], count: 0 });
      const channelsPromise = developerActive
        ? getChannels(locale, developerToken)
        : Promise.resolve({ channels: [] as GpioChannel[], count: 0 });
      const [metaRes, portsRes, sessionRes, gpsSessionRes, settingsRes, gpsSettingsRes, parsedRes, channelsRes] = await Promise.all([
        getLocales(),
        getPorts(locale, developerToken),
        getSession(locale, developerToken),
        getGPSSession(locale, developerToken),
        getDetectionSettings(locale, developerToken),
        getGPSSettings(locale, developerToken),
        parsedPromise,
        channelsPromise,
      ]);

      setMeta(metaRes);
      setPorts(portsRes.ports);
      setSession(sessionRes);
      setGPSSession(gpsSessionRes);
      setMessages(parsedRes.items);
      setChannels(channelsRes.channels);

      const { receivePort, sendPort } = resolveInitialPorts(sessionRes, settingsRes, portsRes.ports);
      syncSerialSelection(receivePort, sendPort);
      const { dataPort, controlPort } = resolveInitialGPSPorts(gpsSessionRes, gpsSettingsRes, portsRes.ports);
      syncGPSSelection(dataPort, controlPort);
      setBanner({
        kind: sessionBannerKind(sessionRes),
        message: sessionBannerText(sessionRes, sessionRes.active ? t("active", { ns: "common" }) : t("idle", { ns: "common" })),
      });
      setGPSBanner({
        kind: gpsSessionBannerKind(gpsSessionRes),
        message: gpsSessionBannerText(gpsSessionRes, gpsSessionRes.active ? t("active", { ns: "common" }) : t("idle", { ns: "common" })),
      });
    } catch (error) {
      setBanner({ kind: "error", message: extractErrorMessage(error) });
    }
  }, [developerActive, developerToken, locale, syncGPSSelection, syncSerialSelection, t]);

  useEffect(() => {
    void i18n.changeLanguage(locale);
    persistLocale(locale);
  }, [i18n, locale]);

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
    if (!needsRuntimeData) {
      return;
    }
    const receivePort = selectedReceivePort.trim();
    const sendPort = selectedSendPort.trim();
    if (!receivePort || !sendPort) {
      return;
    }

    const currentKey = serialKey(receivePort, sendPort);
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
          lastAppliedSerialRef.current = currentKey;
          setSession(response);
          setBanner({
            kind: sessionBannerKind(response),
            message: sessionBannerText(response, response.message || t("active", { ns: "common" })),
          });
          await bootstrap();
        } catch (error) {
          setBanner({ kind: "error", message: extractErrorMessage(error) });
        }
      })();
    }, 350);

    return () => window.clearTimeout(timer);
  }, [bootstrap, developerToken, locale, needsRuntimeData, selectedReceivePort, selectedSendPort, t]);

  useEffect(() => {
    if (!needsRuntimeData) {
      return;
    }
    const dataPort = selectedGPSDataPort.trim();
    const controlPort = selectedGPSControlPort.trim();
    if (!dataPort || !controlPort) {
      return;
    }

    const currentKey = serialKey(dataPort, controlPort);
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
          setGPSBanner({ kind: "error", message: extractErrorMessage(error) });
        }
      })();
    }, 350);

    return () => window.clearTimeout(timer);
  }, [bootstrap, developerToken, locale, needsRuntimeData, selectedGPSControlPort, selectedGPSDataPort, t]);

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
            syncSerialSelection(nextReceivePort, nextSendPort || nextReceivePort);
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
            syncSerialSelection(nextReceivePort, nextSendPort || nextReceivePort);
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
            syncSerialSelection(nextReceivePort, nextSendPort || nextReceivePort);
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
      onGPSRecord: (event) => {
        if (event.payload) {
          setGPSSession((current) => current ? {
            ...current,
            lastNmea: event.payload!.raw,
            lastFix: event.payload!.fix ?? current.lastFix,
            lastRecord: event.payload!,
          } : current);
        }
      },
      onParsed: (event) => {
        if (developerActive && event.payload) {
          setMessages((items) => dedupeParsed(items, event.payload!, 400));
        }
      },
      onChannelUpdated: (event) => {
        if (developerActive && event.payload) {
          setChannels((items) => dedupeById(items, event.payload!, 16));
        }
      },
      onError: (error) => {
        setBanner({ kind: "error", message: error.message });
      },
    });

    return close;
  }, [developerActive, developerToken, locale, needsRuntimeData, syncGPSSelection, syncSerialSelection, t]);

  const sessionActive = Boolean(session?.active);
  const sessionStateLabel = session
    ? sessionBannerText(session, sessionActive ? t("active", { ns: "common" }) : t("idle", { ns: "common" }))
    : t("idle", { ns: "common" });
  const currentReceivePort = session?.rxPortName || session?.portName || selectedReceivePort;
  const currentSendPort = session?.txPortName || selectedSendPort;
  const gpsActive = Boolean(gpsSession?.active);
  const gpsSessionStateLabel = gpsSession
    ? gpsSessionBannerText(gpsSession, gpsActive ? t("active", { ns: "common" }) : t("idle", { ns: "common" }))
    : t("idle", { ns: "common" });
  const currentGPSDataPort = gpsSession?.dataPortName || gpsSession?.portName || selectedGPSDataPort;
  const currentGPSControlPort = gpsSession?.controlPortName || selectedGPSControlPort;
  const defaultAppTitle = t("app.title", { ns: "common" });
  const appTitle = storedSettings.appTitle.trim() || defaultAppTitle;
  const allLocaleOptions = meta?.supportedLocales.length ? meta.supportedLocales : supportedLocales;
  const localeOptions = normalizeVisibleLocales(allLocaleOptions, visibleLocales, locale);
  const developerExpiresAt = developerSession?.expiresAt ?? 0;
  const isMessagePage = developerActive && MESSAGE_PAGE_ORDER.includes(page as ParsedMessageType);

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

  const handleToggleChannel = async (channel: GpioChannel) => {
    try {
      const response = await setChannelState(channel.id, { enabled: !channel.enabled }, locale, developerToken);
      setChannels((items) => dedupeById(items, response.channel, 16));
      setBanner({ kind: "success", message: response.message });
    } catch (error) {
      setBanner({ kind: "error", message: extractErrorMessage(error) });
    }
  };

  const handleVisibleLocalesChange = (nextLocales: string[]) => {
    const normalized = normalizeVisibleLocales(allLocaleOptions, nextLocales, locale);
    setVisibleLocales(normalized);
    persistVisibleLocales(normalized);
  };

  const handleAppTitleChange = (value: string) => {
    const nextSettings = {
      ...storedSettings,
      appTitle: value.trim(),
    };
    setStoredSettings(nextSettings);
    persistSettings(nextSettings);
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

  if (page === "screen") {
    return <ScreenPage appTitle={appTitle} t={t} locale={locale} localeOptions={localeOptions} onLocaleChange={setLocale} />;
  }

  return (
    <div className="h-dvh overflow-hidden bg-base-100 text-base-content">
      <div className="grid h-full min-h-0 grid-cols-1 gap-0 overflow-hidden p-0 xl:grid-cols-[244px_minmax(0,1fr)] xl:gap-3 xl:p-3">
        <Sidebar
          appTitle={appTitle}
          page={page}
          locale={locale}
          localeOptions={localeOptions}
          developerActive={developerActive}
          developerExpiresAt={developerExpiresAt}
          t={t}
          onLocaleChange={setLocale}
          onNavigate={navigate}
          onDeveloperLogin={handleDeveloperLogin}
          onDeveloperLogout={() => void handleDeveloperLogout()}
        />

        <div className="flex min-h-0 min-w-0 flex-col overflow-hidden">
          <main
            className={cx(
              "flex min-h-0 min-w-0 flex-1 flex-col gap-3 overflow-x-hidden",
              isMessagePage ? "overflow-hidden" : "overflow-y-auto",
            )}
          >
            {isMessagePage ? (
              <MessagePage
                page={page as ParsedMessageType}
                records={messages.filter((item) => item.type === page)}
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
                  t={t}
                  onToggleChannel={(channel) => void handleToggleChannel(channel)}
                />
              ) : null
            ) : null}

            {page === "settings" ? (
              <UserSettingsPage
                appTitle={appTitle}
                defaultAppTitle={defaultAppTitle}
                t={t}
                onAppTitleChange={handleAppTitleChange}
              />
            ) : null}

            {page === "developer-settings" && developerActive ? (
              <SettingsPage
                banner={banner}
                ports={ports}
                selectedReceivePort={selectedReceivePort}
                selectedSendPort={selectedSendPort}
                selectedGPSDataPort={selectedGPSDataPort}
                selectedGPSControlPort={selectedGPSControlPort}
                sessionStateLabel={sessionStateLabel}
                currentReceivePort={currentReceivePort}
                currentSendPort={currentSendPort}
                gpsBanner={gpsBanner}
                gpsSession={gpsSession}
                gpsSessionStateLabel={gpsSessionStateLabel}
                currentGPSDataPort={currentGPSDataPort}
                currentGPSControlPort={currentGPSControlPort}
                allLocaleOptions={allLocaleOptions}
                visibleLocales={localeOptions}
                currentLocale={locale}
                t={t}
                onRefresh={() => void bootstrap()}
                onReceivePortChange={setSelectedReceivePort}
                onSendPortChange={setSelectedSendPort}
                onGPSDataPortChange={setSelectedGPSDataPort}
                onGPSControlPortChange={setSelectedGPSControlPort}
                onVisibleLocalesChange={handleVisibleLocalesChange}
              />
            ) : null}

            {page === "network-settings" && developerActive ? (
              <NetworkSettingsPage
                locale={locale}
                developerToken={developerToken}
                t={t}
              />
            ) : null}
          </main>
        </div>
      </div>
      <VirtualKeyboard locale={locale} localeOptions={localeOptions} />
    </div>
  );
}

export default App;
