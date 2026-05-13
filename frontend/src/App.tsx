import { useCallback, useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";

import {
  getChannels,
  getDetectionSettings,
  getLocales,
  getParsed,
  getPorts,
  getSession,
  openDetectionStream,
  setChannelState,
  updateDetectionSettings,
} from "./api";
import { MESSAGE_PAGE_ORDER } from "./app/message-pages";
import type { Banner } from "./app/types";
import { Sidebar } from "./components/Sidebar";
import { getStoredLocale, persistLocale, supportedLocales } from "./i18n";
import { useHashPage } from "./hooks/useHashPage";
import { InterferencePage } from "./pages/InterferencePage";
import { MessagePage } from "./pages/MessagePage";
import { SettingsPage } from "./pages/SettingsPage";
import { FIXED_SERIAL_PROFILE } from "./serial-profile";
import type {
  DetectionSessionResponse,
  GpioChannel,
  LocaleMeta,
  ParsedMessage,
  ParsedMessageType,
  PortInfo,
} from "./types";
import { cx } from "./utils/classnames";
import {
  dedupeById,
  dedupeParsed,
  extractErrorMessage,
  resolveInitialPorts,
  serialKey,
  sessionBannerKind,
  sessionBannerText,
} from "./utils/session";

function App() {
  const { t, i18n } = useTranslation();
  const [page, navigate] = useHashPage();
  const [locale, setLocale] = useState(() => getStoredLocale());
  const [meta, setMeta] = useState<LocaleMeta | null>(null);
  const [ports, setPorts] = useState<PortInfo[]>([]);
  const [session, setSession] = useState<DetectionSessionResponse | null>(null);
  const [messages, setMessages] = useState<ParsedMessage[]>([]);
  const [channels, setChannels] = useState<GpioChannel[]>([]);
  const [selectedReceivePort, setSelectedReceivePort] = useState("");
  const [selectedSendPort, setSelectedSendPort] = useState("");
  const [messageSearch, setMessageSearch] = useState("");
  const [banner, setBanner] = useState<Banner>({ kind: "idle", message: "" });
  const lastAppliedSerialRef = useRef("");

  const syncSerialSelection = useCallback((receivePort: string, sendPort: string) => {
    const nextReceivePort = receivePort.trim();
    const nextSendPort = sendPort.trim();
    lastAppliedSerialRef.current = serialKey(nextReceivePort, nextSendPort);
    setSelectedReceivePort(nextReceivePort);
    setSelectedSendPort(nextSendPort);
  }, []);

  const bootstrap = useCallback(async () => {
    setBanner({ kind: "loading", message: t("loading", { ns: "common" }) });
    try {
      const [metaRes, portsRes, sessionRes, settingsRes, parsedRes, channelsRes] = await Promise.all([
        getLocales(),
        getPorts(locale),
        getSession(locale),
        getDetectionSettings(locale),
        getParsed(locale, 400),
        getChannels(locale),
      ]);

      setMeta(metaRes);
      setPorts(portsRes.ports);
      setSession(sessionRes);
      setMessages(parsedRes.items);
      setChannels(channelsRes.channels);

      const { receivePort, sendPort } = resolveInitialPorts(sessionRes, settingsRes, portsRes.ports);
      syncSerialSelection(receivePort, sendPort);
      setBanner({
        kind: sessionBannerKind(sessionRes),
        message: sessionBannerText(sessionRes, sessionRes.active ? t("active", { ns: "common" }) : t("idle", { ns: "common" })),
      });
    } catch (error) {
      setBanner({ kind: "error", message: extractErrorMessage(error) });
    }
  }, [locale, syncSerialSelection, t]);

  useEffect(() => {
    void i18n.changeLanguage(locale);
    persistLocale(locale);
  }, [i18n, locale]);

  useEffect(() => {
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
  }, [bootstrap]);

  useEffect(() => {
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
  }, [bootstrap, locale, selectedReceivePort, selectedSendPort, t]);

  useEffect(() => {
    const close = openDetectionStream(locale, {
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
      onParsed: (event) => {
        if (event.payload) {
          setMessages((items) => dedupeParsed(items, event.payload!, 400));
        }
      },
      onChannelUpdated: (event) => {
        if (event.payload) {
          setChannels((items) => dedupeById(items, event.payload!, 16));
        }
      },
      onError: (error) => {
        setBanner({ kind: "error", message: error.message });
      },
    });

    return close;
  }, [locale, syncSerialSelection, t]);

  const sessionActive = Boolean(session?.active);
  const sessionStateLabel = session
    ? sessionBannerText(session, sessionActive ? t("active", { ns: "common" }) : t("idle", { ns: "common" }))
    : t("idle", { ns: "common" });
  const currentReceivePort = session?.rxPortName || session?.portName || selectedReceivePort;
  const currentSendPort = session?.txPortName || selectedSendPort;
  const appTitle = t("app.title", { ns: "common" });
  const isMessagePage = MESSAGE_PAGE_ORDER.includes(page as ParsedMessageType);
  const localeOptions = meta?.supportedLocales.length ? meta.supportedLocales : supportedLocales;

  useEffect(() => {
    document.title = appTitle;
  }, [appTitle]);

  const handleToggleChannel = async (channel: GpioChannel) => {
    try {
      const response = await setChannelState(channel.id, { enabled: !channel.enabled }, locale);
      setChannels((items) => dedupeById(items, response.channel, 16));
      setBanner({ kind: "success", message: response.message });
    } catch (error) {
      setBanner({ kind: "error", message: extractErrorMessage(error) });
    }
  };

  return (
    <div className="h-dvh overflow-hidden bg-base-100 text-base-content">
      <div className="grid h-full min-h-0 grid-cols-1 gap-0 overflow-hidden p-0 xl:grid-cols-[292px_minmax(0,1fr)] xl:gap-4 xl:p-4">
        <Sidebar
          appTitle={appTitle}
          page={page}
          locale={locale}
          localeOptions={localeOptions}
          t={t}
          onLocaleChange={setLocale}
          onNavigate={navigate}
        />

        <div className="flex min-h-0 min-w-0 flex-col overflow-hidden">
          <main
            className={cx(
              "flex min-h-0 min-w-0 flex-1 flex-col gap-4 overflow-x-hidden",
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
              <InterferencePage
                channels={channels}
                t={t}
                onToggleChannel={(channel) => void handleToggleChannel(channel)}
              />
            ) : null}

            {page === "settings" ? (
              <SettingsPage
                banner={banner}
                ports={ports}
                selectedReceivePort={selectedReceivePort}
                selectedSendPort={selectedSendPort}
                sessionStateLabel={sessionStateLabel}
                currentReceivePort={currentReceivePort}
                currentSendPort={currentSendPort}
                t={t}
                onRefresh={() => void bootstrap()}
                onReceivePortChange={setSelectedReceivePort}
                onSendPortChange={setSelectedSendPort}
              />
            ) : null}
          </main>
        </div>
      </div>
    </div>
  );
}

export default App;
