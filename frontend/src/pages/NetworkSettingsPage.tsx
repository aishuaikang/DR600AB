import { useEffect, useMemo, useState } from "react";
import type { TFunction } from "i18next";
import { CircleAlert, RefreshCw, Save, Wifi } from "lucide-react";

import { connectWiFi, getNetworkInterfaces, getWiFiNetworks, updateNetworkInterface } from "../api";
import { BannerAlert } from "../components/BannerAlert";
import { Panel, PanelBody } from "../components/Panel";
import { SectionHeader } from "../components/SectionHeader";
import type { Banner } from "../app/types";
import type { NetworkInterface, NetworkInterfaceUpdateRequest, WiFiNetwork } from "../types";
import { cx } from "../utils/classnames";
import { extractErrorMessage } from "../utils/session";

type Draft = {
  mode: "dhcp" | "static";
  ipv4Address: string;
  prefix: string;
  gateway4: string;
  dns4: string;
};

function toDraft(item: NetworkInterface): Draft {
  const [primary] = item.ipv4;
  return {
    mode: item.ipv4Method === "manual" ? "static" : "dhcp",
    ipv4Address: primary?.address ?? "",
    prefix: primary?.prefix ? String(primary.prefix) : "24",
    gateway4: item.gateway4 ?? "",
    dns4: item.dns4.join(", "),
  };
}

function parseDNS(value: string) {
  return value
    .split(/[,\s]+/)
    .map((item) => item.trim())
    .filter(Boolean);
}

function isIPv4(value: string) {
  const parts = value.trim().split(".");
  return parts.length === 4 && parts.every((part) => {
    if (!/^\d{1,3}$/.test(part)) {
      return false;
    }
    const next = Number(part);
    return next >= 0 && next <= 255;
  });
}

function validateDraft(draft: Draft, t: TFunction) {
  if (draft.mode === "dhcp") {
    return "";
  }
  if (!isIPv4(draft.ipv4Address)) {
    return t("networkInvalidIPv4", { ns: "settings" });
  }
  const prefix = Number(draft.prefix);
  if (!Number.isInteger(prefix) || prefix < 1 || prefix > 32) {
    return t("networkInvalidPrefix", { ns: "settings" });
  }
  if (draft.gateway4.trim() && !isIPv4(draft.gateway4)) {
    return t("networkInvalidGateway", { ns: "settings" });
  }
  const invalidDNS = parseDNS(draft.dns4).find((dns) => !isIPv4(dns));
  if (invalidDNS) {
    return t("networkInvalidDNS", { ns: "settings" });
  }
  return "";
}

function buildPayload(draft: Draft): NetworkInterfaceUpdateRequest {
  if (draft.mode === "dhcp") {
    return { mode: "dhcp" };
  }
  return {
    mode: "static",
    ipv4Address: draft.ipv4Address.trim(),
    prefix: Number(draft.prefix),
    gateway4: draft.gateway4.trim(),
    dns4: parseDNS(draft.dns4),
  };
}

function formatAddresses(addresses: NetworkInterface["ipv4"]) {
  if (!addresses.length) {
    return "-";
  }
  return addresses.map((item) => `${item.address}/${item.prefix || ""}`).join(", ");
}

function InterfaceCard({
  item,
  draft,
  busy,
  t,
  onDraftChange,
  onSave,
}: {
  item: NetworkInterface;
  draft: Draft;
  busy: boolean;
  t: TFunction;
  onDraftChange: (draft: Draft) => void;
  onSave: () => void;
}) {
  const validation = validateDraft(draft, t);
  const editable = item.managed;

  return (
    <Panel>
      <PanelBody>
        <div className="grid gap-3 xl:grid-cols-[minmax(0,1fr)_minmax(20rem,24rem)]">
          <div className="grid gap-3">
            <div className="flex flex-wrap items-start justify-between gap-2">
              <div className="min-w-0">
                <h3 className="truncate text-sm font-semibold text-base-content">{item.name}</h3>
                <p className="mt-1 text-xs leading-5 text-base-content/55">
                  {item.type || "-"} · {item.state || "-"} · {item.connectionName || t("networkUnmanaged", { ns: "settings" })}
                </p>
              </div>
              <span
                className={cx(
                  "badge badge-sm border-0 font-semibold",
                  item.state === "connected" ? "badge-success" : item.managed ? "badge-warning" : "badge-error",
                )}
              >
                {item.managed ? item.state || "-" : t("networkUnmanaged", { ns: "settings" })}
              </span>
            </div>

            <div className="grid gap-2 sm:grid-cols-2 xl:grid-cols-3">
              <Info label={t("networkIPv4", { ns: "settings" })} value={formatAddresses(item.ipv4)} />
              <Info label={t("networkGateway", { ns: "settings" })} value={item.gateway4 || "-"} />
              <Info label={t("networkDNS", { ns: "settings" })} value={item.dns4.length ? item.dns4.join(", ") : "-"} />
              <Info label={t("networkMAC", { ns: "settings" })} value={item.hardwareAddress || "-"} />
              <Info label={t("networkMTU", { ns: "settings" })} value={item.mtu ? String(item.mtu) : "-"} />
              <Info label={t("networkMethod", { ns: "settings" })} value={item.ipv4Method || "-"} />
            </div>
          </div>

          <div className="grid gap-3 rounded-2xl border border-base-300 bg-base-100/45 p-3">
            <div className="grid grid-cols-2 gap-2">
              <button
                className={cx("btn btn-sm", draft.mode === "dhcp" ? "btn-primary" : "btn-outline")}
                type="button"
                disabled={!editable || busy}
                onClick={() => onDraftChange({ ...draft, mode: "dhcp" })}
              >
                {t("networkDHCP", { ns: "settings" })}
              </button>
              <button
                className={cx("btn btn-sm", draft.mode === "static" ? "btn-primary" : "btn-outline")}
                type="button"
                disabled={!editable || busy}
                onClick={() => onDraftChange({ ...draft, mode: "static" })}
              >
                {t("networkStatic", { ns: "settings" })}
              </button>
            </div>

            <div className="grid gap-2 sm:grid-cols-[minmax(0,1fr)_5rem]">
              <label className="grid gap-1.5">
                <span className="text-xs font-medium text-base-content/60">{t("networkIPv4Address", { ns: "settings" })}</span>
                <input
                  className="input input-sm input-bordered w-full bg-base-100"
                  value={draft.ipv4Address}
                  inputMode="decimal"
                  disabled={!editable || draft.mode === "dhcp" || busy}
                  onChange={(event) => onDraftChange({ ...draft, ipv4Address: event.target.value })}
                  placeholder="192.168.10.10"
                />
              </label>
              <label className="grid gap-1.5">
                <span className="text-xs font-medium text-base-content/60">{t("networkPrefix", { ns: "settings" })}</span>
                <input
                  className="input input-sm input-bordered w-full bg-base-100"
                  value={draft.prefix}
                  inputMode="numeric"
                  disabled={!editable || draft.mode === "dhcp" || busy}
                  onChange={(event) => onDraftChange({ ...draft, prefix: event.target.value.replace(/\D/g, "").slice(0, 2) })}
                  placeholder="24"
                />
              </label>
            </div>

            <label className="grid gap-1.5">
              <span className="text-xs font-medium text-base-content/60">{t("networkGateway", { ns: "settings" })}</span>
              <input
                className="input input-sm input-bordered w-full bg-base-100"
                value={draft.gateway4}
                inputMode="decimal"
                disabled={!editable || draft.mode === "dhcp" || busy}
                onChange={(event) => onDraftChange({ ...draft, gateway4: event.target.value })}
                placeholder="192.168.10.1"
              />
            </label>

            <label className="grid gap-1.5">
              <span className="text-xs font-medium text-base-content/60">{t("networkDNS", { ns: "settings" })}</span>
              <input
                className="input input-sm input-bordered w-full bg-base-100"
                value={draft.dns4}
                inputMode="decimal"
                disabled={!editable || draft.mode === "dhcp" || busy}
                onChange={(event) => onDraftChange({ ...draft, dns4: event.target.value })}
                placeholder="8.8.8.8, 114.114.114.114"
              />
            </label>

            {!editable ? (
              <p className="flex items-start gap-2 rounded-xl bg-warning/10 px-3 py-2 text-xs leading-5 text-warning">
                <CircleAlert size={14} className="mt-0.5 shrink-0" />
                {t("networkUnmanagedHint", { ns: "settings" })}
              </p>
            ) : validation ? (
              <p className="rounded-xl bg-error/10 px-3 py-2 text-xs leading-5 text-error">{validation}</p>
            ) : null}

            <button
              className="btn btn-primary btn-sm"
              type="button"
              disabled={!editable || Boolean(validation) || busy}
              onClick={onSave}
            >
              <Save size={15} />
              {busy ? t("loading", { ns: "common" }) : t("networkApply", { ns: "settings" })}
            </button>
          </div>
        </div>
      </PanelBody>
    </Panel>
  );
}

function Info({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded-2xl border border-base-300 bg-base-100/35 p-3">
      <span className="block text-[11px] font-semibold uppercase text-base-content/45">{label}</span>
      <strong className="mt-1 block min-w-0 break-words text-xs font-semibold leading-5 text-base-content">{value}</strong>
    </div>
  );
}

function WiFiPanel({
  networks,
  available,
  message,
  selectedSSID,
  password,
  busy,
  t,
  onScan,
  onSSIDChange,
  onPasswordChange,
  onConnect,
}: {
  networks: WiFiNetwork[];
  available: boolean;
  message: string;
  selectedSSID: string;
  password: string;
  busy: boolean;
  t: TFunction;
  onScan: () => void;
  onSSIDChange: (value: string) => void;
  onPasswordChange: (value: string) => void;
  onConnect: (device?: string) => void;
}) {
  const selected = networks.find((item) => item.ssid === selectedSSID);
  const requiresPassword = Boolean(selected?.security && selected.security !== "--");
  const passwordDisabled = busy || Boolean(selected && !requiresPassword);

  return (
    <Panel>
      <PanelBody>
        <SectionHeader
          title={t("wifiTitle", { ns: "settings" })}
          description={t("wifiDescription", { ns: "settings" })}
          action={
            <button className="btn btn-sm btn-outline btn-info" type="button" disabled={busy} onClick={onScan}>
              <RefreshCw size={16} />
              <span>{t("wifiScan", { ns: "settings" })}</span>
            </button>
          }
        />

        {!available ? (
          <div className="alert alert-soft alert-warning py-3 text-sm" role="status">
            <CircleAlert size={16} />
            <span className="min-w-0 [overflow-wrap:anywhere]">{message || t("wifiUnavailable", { ns: "settings" })}</span>
          </div>
        ) : (
          <div className="grid gap-3 xl:grid-cols-[minmax(0,1fr)_minmax(20rem,24rem)]">
            <div className="grid max-h-80 gap-2 overflow-y-auto pr-1">
              {networks.map((network) => (
                <button
                  className={cx(
                    "flex min-w-0 items-center justify-between gap-3 rounded-2xl border p-3 text-left",
                    network.ssid === selectedSSID
                      ? "border-primary/45 bg-primary/10 text-primary"
                      : "border-base-300 bg-base-100/35 text-base-content hover:bg-base-300/55",
                  )}
                  key={`${network.ssid}-${network.bssid || network.device || ""}`}
                  type="button"
                  onClick={() => onSSIDChange(network.ssid)}
                >
                  <span className="flex min-w-0 items-center gap-3">
                    <Wifi size={18} className="shrink-0" />
                    <span className="min-w-0">
                      <strong className="block truncate text-sm font-semibold">{network.ssid}</strong>
                      <span className="mt-0.5 block truncate text-xs text-base-content/55">
                        {network.security || t("wifiOpen", { ns: "settings" })} · {network.device || "-"} · CH {network.channel || "-"}
                      </span>
                    </span>
                  </span>
                  <span className="flex shrink-0 items-center gap-2">
                    {network.active ? <span className="badge badge-success badge-sm">{t("active", { ns: "common" })}</span> : null}
                    <span className="text-xs font-semibold">{network.signal}%</span>
                  </span>
                </button>
              ))}
              {networks.length === 0 ? (
                <div className="grid min-h-24 place-items-center rounded-2xl border border-base-300 bg-base-100/35 text-sm text-base-content/55">
                  {t("wifiEmpty", { ns: "settings" })}
                </div>
              ) : null}
            </div>

            <div className="grid content-start gap-3 rounded-2xl border border-base-300 bg-base-100/45 p-3">
              <label className="grid gap-1.5">
                <span className="text-xs font-medium text-base-content/60">{t("wifiSSID", { ns: "settings" })}</span>
                <input
                  className="input input-sm input-bordered w-full bg-base-100"
                  value={selectedSSID}
                  disabled={busy}
                  onChange={(event) => onSSIDChange(event.target.value)}
                  placeholder="SSID"
                />
              </label>
              <label className="grid gap-1.5">
                <span className="text-xs font-medium text-base-content/60">{t("wifiPassword", { ns: "settings" })}</span>
                <input
                  className="input input-sm input-bordered w-full bg-base-100"
                  value={password}
                  type="password"
                  disabled={passwordDisabled}
                  onChange={(event) => onPasswordChange(event.target.value)}
                  placeholder={passwordDisabled ? t("wifiOpen", { ns: "settings" }) : t("wifiPasswordPlaceholder", { ns: "settings" })}
                />
              </label>
              <button
                className="btn btn-primary btn-sm"
                type="button"
                disabled={busy || !selectedSSID.trim() || (requiresPassword && password.trim().length < 8)}
                onClick={() => onConnect(selected?.device)}
              >
                <Wifi size={15} />
                {busy ? t("loading", { ns: "common" }) : t("wifiConnect", { ns: "settings" })}
              </button>
            </div>
          </div>
        )}
      </PanelBody>
    </Panel>
  );
}

export function NetworkSettingsPage({
  locale,
  developerToken,
  t,
}: {
  locale: string;
  developerToken: string;
  t: TFunction;
}) {
  const [interfaces, setInterfaces] = useState<NetworkInterface[]>([]);
  const [wifiNetworks, setWifiNetworks] = useState<WiFiNetwork[]>([]);
  const [drafts, setDrafts] = useState<Record<string, Draft>>({});
  const [banner, setBanner] = useState<Banner>({ kind: "idle", message: "" });
  const [backendState, setBackendState] = useState({ available: true, readOnly: false, message: "" });
  const [wifiState, setWifiState] = useState({ available: true, readOnly: false, message: "" });
  const [selectedSSID, setSelectedSSID] = useState("");
  const [wifiPassword, setWifiPassword] = useState("");
  const [loading, setLoading] = useState(false);
  const [saving, setSaving] = useState("");
  const [wifiBusy, setWifiBusy] = useState(false);

  const load = async () => {
    setLoading(true);
    setBanner({ kind: "loading", message: "" });
    try {
      const response = await getNetworkInterfaces(locale, developerToken);
      setInterfaces(response.interfaces);
      setDrafts(Object.fromEntries(response.interfaces.map((item) => [item.name, toDraft(item)])));
      setBackendState({
        available: response.available,
        readOnly: response.readOnly,
        message: response.message ?? "",
      });
      setBanner({ kind: "idle", message: "" });
    } catch (error) {
      setBanner({ kind: "error", message: extractErrorMessage(error) });
    } finally {
      setLoading(false);
    }
  };

  const scanWiFi = async () => {
    setWifiBusy(true);
    try {
      const response = await getWiFiNetworks(locale, developerToken);
      setWifiNetworks(response.networks);
      setWifiState({
        available: response.available,
        readOnly: response.readOnly,
        message: response.message ?? "",
      });
      if (!selectedSSID && response.networks.length > 0) {
        setSelectedSSID(response.networks.find((item) => item.active)?.ssid ?? response.networks[0].ssid);
      }
    } catch (error) {
      setWifiState({ available: false, readOnly: true, message: extractErrorMessage(error) });
    } finally {
      setWifiBusy(false);
    }
  };

  useEffect(() => {
    void load();
    void scanWiFi();
  }, [locale, developerToken]);

  const managedCount = useMemo(() => interfaces.filter((item) => item.managed).length, [interfaces]);

  return (
    <section className="grid gap-3">
      <Panel>
        <PanelBody>
          <SectionHeader
            title={t("networkTitle", { ns: "settings" })}
            description={t("networkDescription", { ns: "settings" })}
            action={
              <button className="btn btn-sm btn-outline btn-info" type="button" disabled={loading} onClick={() => void load()}>
                <RefreshCw size={16} />
                <span>{t("refresh", { ns: "common" })}</span>
              </button>
            }
          />
          <div className="grid gap-2 sm:grid-cols-3">
            <Info label={t("networkInterfaceCount", { ns: "settings" })} value={String(interfaces.length)} />
            <Info label={t("networkManagedCount", { ns: "settings" })} value={String(managedCount)} />
            <Info label={t("networkBackend", { ns: "settings" })} value="NetworkManager" />
          </div>
          {!backendState.available || backendState.readOnly ? (
            <div className="alert alert-soft alert-warning py-3 text-sm" role="status">
              <CircleAlert size={16} />
              <span className="min-w-0 [overflow-wrap:anywhere]">
                {backendState.message || t("networkUnsupportedPlatform", { ns: "settings" })}
              </span>
            </div>
          ) : null}
          <BannerAlert banner={banner} />
        </PanelBody>
      </Panel>

      <WiFiPanel
        networks={wifiNetworks}
        available={wifiState.available}
        message={wifiState.message}
        selectedSSID={selectedSSID}
        password={wifiPassword}
        busy={wifiBusy}
        t={t}
        onScan={() => void scanWiFi()}
        onSSIDChange={(value) => {
          setSelectedSSID(value);
          setWifiPassword("");
        }}
        onPasswordChange={setWifiPassword}
        onConnect={(device) => {
          void (async () => {
            setWifiBusy(true);
            setBanner({ kind: "loading", message: "" });
            try {
              const response = await connectWiFi({ ssid: selectedSSID.trim(), password: wifiPassword, device }, locale, developerToken);
              setBanner({ kind: "success", message: response.message });
              setWifiPassword("");
              await scanWiFi();
              await load();
            } catch (error) {
              setBanner({ kind: "error", message: extractErrorMessage(error) });
            } finally {
              setWifiBusy(false);
            }
          })();
        }}
      />

      {interfaces.map((item) => (
        <InterfaceCard
          key={item.name}
          item={item}
          draft={drafts[item.name] ?? toDraft(item)}
          busy={saving === item.name}
          t={t}
          onDraftChange={(draft) => setDrafts((items) => ({ ...items, [item.name]: draft }))}
          onSave={() => {
            void (async () => {
              const draft = drafts[item.name] ?? toDraft(item);
              setSaving(item.name);
              setBanner({ kind: "loading", message: "" });
              try {
                const response = await updateNetworkInterface(item.name, buildPayload(draft), locale, developerToken);
                setInterfaces((items) => items.map((current) => (current.name === item.name ? response.interface : current)));
                setDrafts((items) => ({ ...items, [item.name]: toDraft(response.interface) }));
                setBanner({ kind: "success", message: response.message });
              } catch (error) {
                setBanner({ kind: "error", message: extractErrorMessage(error) });
              } finally {
                setSaving("");
              }
            })();
          }}
        />
      ))}

      {!loading && interfaces.length === 0 && !banner.message && backendState.available ? (
        <Panel>
          <PanelBody>
            <div className="grid min-h-32 place-items-center text-sm text-base-content/55">{t("networkEmpty", { ns: "settings" })}</div>
          </PanelBody>
        </Panel>
      ) : null}
    </section>
  );
}
