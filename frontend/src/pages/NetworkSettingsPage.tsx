import { useEffect, useMemo, useState } from "react";
import type { TFunction } from "i18next";
import { ArrowDown, ArrowUp, Cable, CircleAlert, Eye, EyeOff, Plus, RadioTower, RefreshCw, Save, Trash2, Wifi, WifiOff } from "lucide-react";

import {
  connectCellular,
  connectWiFi,
  disconnectWiFi,
  getNetworkInterfaces,
  getWiFiNetworks,
  updateNetworkInterface,
  updateNetworkInterfacePriorities,
} from "../api";
import { BannerAlert } from "../components/BannerAlert";
import { LoadingSpinner } from "../components/LoadingState";
import { Panel, PanelBody } from "../components/Panel";
import { SectionHeader } from "../components/SectionHeader";
import type { Banner } from "../app/types";
import type { NetworkAddress, NetworkInterface, NetworkInterfaceUpdateRequest, WiFiNetwork } from "../types";
import { cx } from "../utils/classnames";
import { extractErrorMessage } from "../utils/session";

type Draft = {
  mode: "dhcp" | "static";
  ipv4: Array<{ address: string; prefix: string }>;
  gateway4: string;
  dns4: string;
  routeMetric: string;
};

type CellularDraft = {
  apn: string;
  username: string;
  password: string;
  connectionName: string;
  routeMetric: string;
};

function toDraft(item: NetworkInterface): Draft {
  return {
    mode: item.ipv4Method === "manual" ? "static" : "dhcp",
    ipv4: addressesToDraft(item.ipv4),
    gateway4: item.gateway4 ?? "",
    dns4: item.dns4.join(", "),
    routeMetric: typeof item.routeMetric === "number" ? String(item.routeMetric) : "",
  };
}

function addressesToDraft(addresses: NetworkAddress[]) {
  const items = addresses.length ? addresses : [{ address: "", prefix: 24 }];
  return items.map((item) => ({
    address: item.address ?? "",
    prefix: item.prefix ? String(item.prefix) : "24",
  }));
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
    if (draft.routeMetric.trim() && !isRouteMetric(draft.routeMetric)) {
      return t("networkInvalidRouteMetric", { ns: "settings" });
    }
    return "";
  }
  if (draft.routeMetric.trim() && !isRouteMetric(draft.routeMetric)) {
    return t("networkInvalidRouteMetric", { ns: "settings" });
  }
  const addresses = draft.ipv4.map((item) => ({
    address: item.address.trim(),
    prefix: Number(item.prefix),
  }));
  if (!addresses.length || addresses.some((item) => !isIPv4(item.address))) {
    return t("networkInvalidIPv4", { ns: "settings" });
  }
  if (addresses.some((item) => !Number.isInteger(item.prefix) || item.prefix < 1 || item.prefix > 32)) {
    return t("networkInvalidPrefix", { ns: "settings" });
  }
  const seen = new Set<string>();
  for (const item of addresses) {
    const key = item.address;
    if (seen.has(key)) {
      return t("networkDuplicateIPv4", { ns: "settings" });
    }
    seen.add(key);
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
  const routeMetric = parseRouteMetric(draft.routeMetric) ?? -1;
  if (draft.mode === "dhcp") {
    return { mode: "dhcp", routeMetric };
  }
  return {
    mode: "static",
    ipv4: draft.ipv4.map((item) => ({
      address: item.address.trim(),
      prefix: Number(item.prefix),
    })),
    gateway4: draft.gateway4.trim(),
    dns4: parseDNS(draft.dns4),
    routeMetric,
  };
}

function parseRouteMetric(value: string) {
  const trimmed = value.trim();
  if (!trimmed || !isRouteMetric(trimmed)) {
    return undefined;
  }
  return Number(trimmed);
}

function isRouteMetric(value: string) {
  const trimmed = value.trim();
  if (!/^\d{1,4}$/.test(trimmed)) {
    return false;
  }
  const metric = Number(trimmed);
  return metric >= 0 && metric <= 9999;
}

function formatRouteMetric(value?: number) {
  return typeof value === "number" ? String(value) : "-";
}

function priorityLabel(value: string, t: TFunction) {
  const metric = parseRouteMetric(value);
  if (typeof metric !== "number") {
    return t("networkPriorityAuto", { ns: "settings" });
  }
  if (metric <= 200) {
    return t("networkPriorityHigh", { ns: "settings" });
  }
  if (metric <= 700) {
    return t("networkPriorityNormal", { ns: "settings" });
  }
  return t("networkPriorityLow", { ns: "settings" });
}

function formatAddresses(addresses: NetworkInterface["ipv4"]) {
  if (!addresses.length) {
    return "-";
  }
  return addresses.map((item) => `${item.address}/${item.prefix || ""}`).join(", ");
}

function updateDraftAddress(draft: Draft, index: number, patch: Partial<Draft["ipv4"][number]>): Draft {
  return {
    ...draft,
    ipv4: draft.ipv4.map((item, itemIndex) => (itemIndex === index ? { ...item, ...patch } : item)),
  };
}

function addDraftAddress(draft: Draft): Draft {
  return {
    ...draft,
    ipv4: [...draft.ipv4, { address: "", prefix: "24" }],
  };
}

function removeDraftAddress(draft: Draft, index: number): Draft {
  const next = draft.ipv4.filter((_, itemIndex) => itemIndex !== index);
  return {
    ...draft,
    ipv4: next.length ? next : [{ address: "", prefix: "24" }],
  };
}

function pickDefaultInterface(items: NetworkInterface[]) {
  return (
    items.find((item) => item.managed && item.state === "connected" && item.type === "wifi") ??
    items.find((item) => isCellularInterface(item)) ??
    items.find((item) => item.managed && item.state === "connected") ??
    items.find((item) => item.managed) ??
    items[0]
  );
}

function getPriorityMetric(index: number) {
  return 100 + index * 200;
}

function isWiFiInterface(item: NetworkInterface) {
  return item.kind === "wifi" || item.type.toLowerCase().includes("wifi") || item.type.toLowerCase().includes("wireless");
}

function isWiredInterface(item: NetworkInterface) {
  return item.kind === "ethernet" || item.type.toLowerCase().includes("ethernet") || item.name.toLowerCase().startsWith("eth");
}

function isCellularInterface(item: NetworkInterface) {
  return item.kind === "cellular" || item.type.toLowerCase().includes("gsm") || item.type.toLowerCase().includes("wwan");
}

function hasCapability(item: NetworkInterface, capability: string) {
  return item.capabilities?.includes(capability) ?? false;
}

function canEditIPv4(item: NetworkInterface) {
  return item.managed && hasCapability(item, "ipv4");
}

function canConnectCellular(item: NetworkInterface) {
  return hasCapability(item, "cellular-connect") && Boolean(item.modem?.primaryPort) && item.modem?.failedReason !== "sim-missing";
}

function toCellularDraft(item?: NetworkInterface): CellularDraft {
  return {
    apn: "",
    username: "",
    password: "",
    connectionName: item?.connectionName && item.connectionName !== "--" ? item.connectionName : "",
    routeMetric: typeof item?.routeMetric === "number" ? String(item.routeMetric) : "",
  };
}

function validateCellularDraft(draft: CellularDraft, t: TFunction) {
  if (!draft.apn.trim()) {
    return t("cellularInvalidAPN", { ns: "settings" });
  }
  if (draft.routeMetric.trim() && !isRouteMetric(draft.routeMetric)) {
    return t("networkInvalidRouteMetric", { ns: "settings" });
  }
  return "";
}

function interfaceIcon(item: NetworkInterface, size = 19) {
  if (isCellularInterface(item)) {
    return <RadioTower size={size} />;
  }
  if (isWiFiInterface(item)) {
    return <Wifi size={size} />;
  }
  return <Cable size={size} />;
}

function interfaceTypeLabel(item: NetworkInterface, t: TFunction) {
  if (isCellularInterface(item)) {
    return t("networkTypeCellular", { ns: "settings" });
  }
  if (isWiFiInterface(item)) {
    return t("networkTypeWifi", { ns: "settings" });
  }
  if (isWiredInterface(item)) {
    return t("networkTypeWired", { ns: "settings" });
  }
  return item.type || "-";
}

function cellularStateLabel(item: NetworkInterface, t: TFunction) {
  const state = item.modem?.state || item.state || "-";
  const failedReason = item.modem?.failedReason;
  if (failedReason === "sim-missing") {
    return t("cellularStateSimMissing", { ns: "settings" });
  }
  if (failedReason) {
    return `${state} / ${failedReason}`;
  }
  return state;
}

function cellularSIMLabel(item: NetworkInterface, t: TFunction) {
  if (item.modem?.failedReason === "sim-missing") {
    return t("cellularSIMMissing", { ns: "settings" });
  }
  if (item.modem?.simPath) {
    return t("cellularSIMReady", { ns: "settings" });
  }
  return "-";
}

function defaultRouteLabel(item: NetworkInterface, t: TFunction) {
  if (item.defaultRoute) {
    return t("networkDefaultRouteActive", { ns: "settings" });
  }
  return t("networkDefaultRouteInactive", { ns: "settings" });
}

function interfaceSortScore(item: NetworkInterface) {
  if (item.managed && item.state === "connected") {
    return 0;
  }
  if (item.managed) {
    return 1;
  }
  return 2;
}

function sortedInterfacePickerItems(items: NetworkInterface[]) {
  return [...items].sort((a, b) => {
    const score = interfaceSortScore(a) - interfaceSortScore(b);
    if (score !== 0) {
      return score;
    }
    const leftMetric = typeof a.routeMetric === "number" ? a.routeMetric : Number.MAX_SAFE_INTEGER;
    const rightMetric = typeof b.routeMetric === "number" ? b.routeMetric : Number.MAX_SAFE_INTEGER;
    if (leftMetric !== rightMetric) {
      return leftMetric - rightMetric;
    }
    return a.name.localeCompare(b.name);
  });
}

function sortedPriorityInterfaces(items: NetworkInterface[]) {
  return [...items]
    .filter((item) => hasCapability(item, "priority") && (hasConnectionName(item) || item.defaultRoute))
    .sort((a, b) => {
      const left = typeof a.routeMetric === "number" ? a.routeMetric : Number.MAX_SAFE_INTEGER;
      const right = typeof b.routeMetric === "number" ? b.routeMetric : Number.MAX_SAFE_INTEGER;
      if (left !== right) {
        return left - right;
      }
      if (a.state === "connected" && b.state !== "connected") {
        return -1;
      }
      if (a.state !== "connected" && b.state === "connected") {
        return 1;
      }
      return a.name.localeCompare(b.name);
    });
}

function hasConnectionName(item: NetworkInterface) {
  return Boolean(item.connectionName && item.connectionName !== "--");
}

function InterfaceCard({
  item,
  draft,
  cellularDraft,
  busy,
  cellularBusy,
  t,
  onDraftChange,
  onCellularDraftChange,
  onSave,
  onCellularConnect,
}: {
  item: NetworkInterface;
  draft: Draft;
  cellularDraft: CellularDraft;
  busy: boolean;
  cellularBusy: boolean;
  t: TFunction;
  onDraftChange: (draft: Draft) => void;
  onCellularDraftChange: (draft: CellularDraft) => void;
  onSave: () => void;
  onCellularConnect: () => void;
}) {
  const validation = validateDraft(draft, t);
  const cellularAPNMissing = !cellularDraft.apn.trim();
  const cellularValidation = cellularAPNMissing ? "" : validateCellularDraft(cellularDraft, t);
  const editable = canEditIPv4(item);
  const cellularEditable = canConnectCellular(item);

  const statusLabel = item.managed || isCellularInterface(item) ? item.state || "-" : t("networkUnmanaged", { ns: "settings" });

  return (
    <div className="grid gap-3 2xl:grid-cols-[minmax(0,1fr)_minmax(20rem,24rem)]">
      <div className="grid content-start gap-3">
        <div className="rounded-2xl border border-base-300 bg-base-100/45 p-3">
          <div className="flex flex-wrap items-center justify-between gap-3">
            <div className="flex min-w-0 items-center gap-3">
              <div className="grid size-10 shrink-0 place-items-center rounded-xl border border-primary/25 bg-primary/10 text-primary">
                {interfaceIcon(item, 19)}
              </div>
              <div className="min-w-0">
                <h3 className="truncate text-base font-semibold text-base-content">{item.name}</h3>
                <p className="mt-0.5 truncate text-xs leading-5 text-base-content/55">
                  {interfaceTypeLabel(item, t)} · {item.connectionName || item.modem?.primaryPort || t("networkUnmanaged", { ns: "settings" })}
                </p>
              </div>
            </div>
            <span
              className={cx(
                "badge badge-sm shrink-0 border-0 font-semibold",
                item.state === "connected" ? "badge-success" : item.managed ? "badge-warning" : "badge-error",
              )}
            >
              {statusLabel}
            </span>
            {item.defaultRoute ? (
              <span className="badge badge-info badge-sm shrink-0 border-0 font-semibold">
                {t("networkDefaultRouteActive", { ns: "settings" })}
              </span>
            ) : null}
          </div>

          <div className="mt-3 grid gap-2">
            <DetailItem label={t("networkIPv4", { ns: "settings" })} value={formatAddresses(item.ipv4)} mono strong />
            <div className="grid gap-2 md:grid-cols-[minmax(0,1fr)_minmax(0,1fr)]">
              <DetailItem label={t("networkGateway", { ns: "settings" })} value={item.gateway4 || "-"} mono />
              <DetailItem label={t("networkRouteMetric", { ns: "settings" })} value={formatRouteMetric(item.routeMetric)} />
            </div>
            <DetailItem label={t("networkDNS", { ns: "settings" })} value={item.dns4.length ? item.dns4.join(", ") : "-"} mono />
            {item.modem ? (
              <div className="grid gap-2 md:grid-cols-[minmax(0,1fr)_minmax(0,1fr)]">
                <DetailItem label={t("cellularModel", { ns: "settings" })} value={[item.modem.manufacturer, item.modem.model].filter(Boolean).join(" ") || "-"} />
                <DetailItem label={t("cellularSIM", { ns: "settings" })} value={cellularSIMLabel(item, t)} />
                <DetailItem label={t("cellularSignal", { ns: "settings" })} value={typeof item.modem.signalQuality === "number" ? `${item.modem.signalQuality}%` : "-"} />
                <DetailItem label={t("cellularPrimaryPort", { ns: "settings" })} value={item.modem.primaryPort || "-"} mono />
              </div>
            ) : null}
          </div>
        </div>

        <details className="rounded-2xl border border-base-300 bg-base-100/35">
          <summary className="cursor-pointer px-3 py-2 text-sm font-semibold text-base-content/70">
            {t("networkAdvancedDetails", { ns: "settings" })}
          </summary>
          <div className="grid gap-2 border-t border-base-300 p-3 md:grid-cols-2">
            <DetailItem label={t("networkConnection", { ns: "settings" })} value={item.connectionName || "-"} />
            <DetailItem label={t("networkMAC", { ns: "settings" })} value={item.hardwareAddress || "-"} mono />
            <DetailItem label={t("networkMTU", { ns: "settings" })} value={item.mtu ? String(item.mtu) : "-"} />
            <DetailItem label={t("networkMethod", { ns: "settings" })} value={item.ipv4Method || "-"} />
            <DetailItem label={t("networkDefaultRoute", { ns: "settings" })} value={defaultRouteLabel(item, t)} />
            <DetailItem label={t("networkSource", { ns: "settings" })} value={item.source || "NetworkManager"} />
            <DetailItem label={t("networkCapabilities", { ns: "settings" })} value={item.capabilities?.join(", ") || "-"} />
            {item.modem ? (
              <>
                <DetailItem label={t("cellularState", { ns: "settings" })} value={cellularStateLabel(item, t)} />
                <DetailItem label={t("cellularDataInterface", { ns: "settings" })} value={item.modem.dataInterface || "-"} mono />
                <DetailItem label={t("cellularEquipmentID", { ns: "settings" })} value={item.modem.equipmentId || "-"} mono />
                <DetailItem label={t("cellularPorts", { ns: "settings" })} value={item.modem.ports?.join(", ") || "-"} />
              </>
            ) : null}
          </div>
        </details>
      </div>

      <div className="grid content-start gap-3 rounded-2xl border border-base-300 bg-base-100/45 p-3">
        {isCellularInterface(item) ? (
          <div className="grid gap-3 rounded-xl border border-base-300/80 bg-base-100/45 p-3">
            <div>
              <h4 className="text-sm font-semibold text-base-content">{t("cellularConfiguration", { ns: "settings" })}</h4>
              <p className="mt-1 text-xs leading-5 text-base-content/55">{t("cellularConfigurationHint", { ns: "settings" })}</p>
            </div>
            <label className="grid gap-1.5">
              <span className="text-xs font-medium text-base-content/60">{t("cellularAPN", { ns: "settings" })}</span>
              <input
                className="input input-sm input-bordered w-full bg-base-100"
                value={cellularDraft.apn}
                disabled={!cellularEditable || cellularBusy}
                onChange={(event) => onCellularDraftChange({ ...cellularDraft, apn: event.target.value })}
                placeholder="cmnet"
              />
            </label>
            <div className="grid gap-2 sm:grid-cols-2">
              <label className="grid gap-1.5">
                <span className="text-xs font-medium text-base-content/60">{t("cellularUsername", { ns: "settings" })}</span>
                <input
                  className="input input-sm input-bordered w-full bg-base-100"
                  value={cellularDraft.username}
                  disabled={!cellularEditable || cellularBusy}
                  onChange={(event) => onCellularDraftChange({ ...cellularDraft, username: event.target.value })}
                  placeholder={t("optional", { ns: "common" })}
                />
              </label>
              <label className="grid gap-1.5">
                <span className="text-xs font-medium text-base-content/60">{t("cellularPassword", { ns: "settings" })}</span>
                <input
                  className="input input-sm input-bordered w-full bg-base-100"
                  value={cellularDraft.password}
                  type="password"
                  disabled={!cellularEditable || cellularBusy}
                  onChange={(event) => onCellularDraftChange({ ...cellularDraft, password: event.target.value })}
                  placeholder={t("optional", { ns: "common" })}
                />
              </label>
            </div>
            <div className="grid gap-2 sm:grid-cols-2">
              <label className="grid gap-1.5">
                <span className="text-xs font-medium text-base-content/60">{t("networkConnection", { ns: "settings" })}</span>
                <input
                  className="input input-sm input-bordered w-full bg-base-100"
                  value={cellularDraft.connectionName}
                  disabled={!cellularEditable || cellularBusy}
                  onChange={(event) => onCellularDraftChange({ ...cellularDraft, connectionName: event.target.value })}
                  placeholder="4g"
                />
              </label>
              <label className="grid gap-1.5">
                <span className="text-xs font-medium text-base-content/60">{t("networkRouteMetric", { ns: "settings" })}</span>
                <input
                  className="input input-sm input-bordered w-full bg-base-100"
                  value={cellularDraft.routeMetric}
                  inputMode="numeric"
                  disabled={!cellularEditable || cellularBusy}
                  onChange={(event) => onCellularDraftChange({ ...cellularDraft, routeMetric: event.target.value.replace(/\D/g, "").slice(0, 4) })}
                  placeholder={t("networkPriorityAuto", { ns: "settings" })}
                />
              </label>
            </div>
            {!cellularEditable ? (
              <p className="flex items-start gap-2 rounded-xl bg-warning/10 px-3 py-2 text-xs leading-5 text-warning">
                <CircleAlert size={14} className="mt-0.5 shrink-0" />
                {t("cellularUnavailableHint", { ns: "settings" })}
              </p>
            ) : cellularValidation ? (
              <p className="rounded-xl bg-error/10 px-3 py-2 text-xs leading-5 text-error">{cellularValidation}</p>
            ) : null}
            <button
              className={cx("btn btn-primary btn-sm", cellularBusy && "app-busy-button")}
              type="button"
              disabled={!cellularEditable || cellularAPNMissing || Boolean(cellularValidation) || cellularBusy}
              onClick={onCellularConnect}
            >
              {cellularBusy ? <LoadingSpinner size={15} /> : <RadioTower size={15} />}
              {cellularBusy ? t("loading", { ns: "common" }) : t("cellularConnect", { ns: "settings" })}
            </button>
          </div>
        ) : null}

        <div>
          <h4 className="text-sm font-semibold text-base-content">{t("networkConfiguration", { ns: "settings" })}</h4>
          <p className="mt-1 text-xs leading-5 text-base-content/55">{t("networkConfigurationHint", { ns: "settings" })}</p>
        </div>

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

        <div className="grid gap-2">
          <div className="flex items-center justify-between gap-2">
            <span className="text-xs font-medium text-base-content/60">{t("networkIPv4Addresses", { ns: "settings" })}</span>
            <button
              className="btn btn-ghost btn-xs h-7 min-h-7 rounded-lg px-2"
              type="button"
              disabled={!editable || draft.mode === "dhcp" || busy}
              onClick={() => onDraftChange(addDraftAddress(draft))}
            >
              <Plus size={13} />
              {t("networkAddIPv4", { ns: "settings" })}
            </button>
          </div>
          {draft.ipv4.map((address, index) => (
            <div className="grid gap-2 sm:grid-cols-[minmax(0,1fr)_5rem_2rem]" key={index}>
              <label className="grid gap-1.5">
                <span className="sr-only">{t("networkIPv4Address", { ns: "settings" })}</span>
                <input
                  className="input input-sm input-bordered w-full bg-base-100"
                  value={address.address}
                  inputMode="decimal"
                  disabled={!editable || draft.mode === "dhcp" || busy}
                  onChange={(event) => onDraftChange(updateDraftAddress(draft, index, { address: event.target.value }))}
                  placeholder="192.168.10.10"
                />
              </label>
              <label className="grid gap-1.5">
                <span className="sr-only">{t("networkPrefix", { ns: "settings" })}</span>
                <input
                  className="input input-sm input-bordered w-full bg-base-100"
                  value={address.prefix}
                  inputMode="numeric"
                  disabled={!editable || draft.mode === "dhcp" || busy}
                  onChange={(event) => onDraftChange(updateDraftAddress(draft, index, { prefix: event.target.value.replace(/\D/g, "").slice(0, 2) }))}
                  placeholder="24"
                />
              </label>
              <button
                className="btn btn-ghost btn-sm h-8 min-h-8 rounded-lg px-0 text-error"
                type="button"
                disabled={!editable || draft.mode === "dhcp" || busy || draft.ipv4.length <= 1}
                aria-label={t("networkRemoveIPv4", { ns: "settings" })}
                title={t("networkRemoveIPv4", { ns: "settings" })}
                onClick={() => onDraftChange(removeDraftAddress(draft, index))}
              >
                <Trash2 size={14} />
              </button>
            </div>
          ))}
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

        <label className="grid gap-1.5">
          <span className="text-xs font-medium text-base-content/60">{t("networkRouteMetric", { ns: "settings" })}</span>
          <input
            className="input input-sm input-bordered w-full bg-base-100"
            value={draft.routeMetric}
            inputMode="numeric"
            disabled={!editable || busy}
            onChange={(event) => onDraftChange({ ...draft, routeMetric: event.target.value.replace(/\D/g, "").slice(0, 4) })}
            placeholder={t("networkPriorityAuto", { ns: "settings" })}
          />
          <span className="text-xs leading-5 text-base-content/50">
            {t("networkRouteMetricHint", { ns: "settings" })} · {priorityLabel(draft.routeMetric, t)}
          </span>
        </label>

        {!editable ? (
          <p className="flex items-start gap-2 rounded-xl bg-warning/10 px-3 py-2 text-xs leading-5 text-warning">
            <CircleAlert size={14} className="mt-0.5 shrink-0" />
            {isCellularInterface(item) ? t("cellularIPv4ManagedHint", { ns: "settings" }) : t("networkUnmanagedHint", { ns: "settings" })}
          </p>
        ) : validation ? (
          <p className="rounded-xl bg-error/10 px-3 py-2 text-xs leading-5 text-error">{validation}</p>
        ) : null}

        <button
          className={cx("btn btn-primary btn-sm", busy && "app-busy-button")}
          type="button"
          disabled={!editable || Boolean(validation) || busy}
          onClick={onSave}
        >
          {busy ? <LoadingSpinner size={15} /> : <Save size={15} />}
          {busy ? t("loading", { ns: "common" }) : t("networkApply", { ns: "settings" })}
        </button>
      </div>
    </div>
  );
}

function InterfacePicker({
  items,
  selectedName,
  t,
  onSelect,
}: {
  items: NetworkInterface[];
  selectedName: string;
  t: TFunction;
  onSelect: (name: string) => void;
}) {
  return (
    <div className="grid max-h-[34rem] gap-2 overflow-y-auto pr-1">
      {items.map((item) => (
        <button
          className={cx(
            "grid min-w-0 gap-2 rounded-2xl border p-2.5 text-left",
            item.name === selectedName
              ? "border-primary/60 bg-primary/10 text-primary shadow-[0_0_0_1px_rgba(59,130,246,0.18)]"
              : "border-base-300 bg-base-100/35 text-base-content hover:border-primary/25 hover:bg-base-300/45",
          )}
          key={item.name}
          type="button"
          onClick={() => onSelect(item.name)}
        >
          <span className="flex min-w-0 items-center gap-2">
            <span
              className={cx(
                "grid size-8 shrink-0 place-items-center rounded-xl border",
                item.name === selectedName ? "border-primary/25 bg-primary/15" : "border-base-300 bg-base-100/45 text-base-content/60",
              )}
            >
              {interfaceIcon(item, 15)}
            </span>
            <span className="min-w-0 flex-1">
              <span className="flex min-w-0 items-center justify-between gap-2">
                <strong className="truncate text-sm font-semibold">{item.name}</strong>
                <span className="flex shrink-0 items-center gap-1">
                  {item.defaultRoute ? <span className="badge badge-info badge-xs">{t("networkDefaultRouteBadge", { ns: "settings" })}</span> : null}
                  <span
                    className={cx(
                      "badge badge-xs shrink-0 border-0 font-semibold",
                      item.state === "connected" ? "badge-success" : item.managed ? "badge-warning" : "badge-error",
                    )}
                  >
                    {item.managed || isCellularInterface(item) ? item.state || "-" : t("networkUnmanaged", { ns: "settings" })}
                  </span>
                </span>
              </span>
              <span className="mt-0.5 block truncate text-xs text-base-content/55">
                {interfaceTypeLabel(item, t)} · {formatAddresses(item.ipv4)}
              </span>
            </span>
          </span>
          <span className="grid grid-cols-2 gap-2 text-[11px] text-base-content/45">
            <span className="truncate">{item.connectionName || "-"}</span>
            <span
              className={cx(
                "justify-self-end rounded-full px-2 py-0.5 tabular-nums",
                item.name === selectedName ? "bg-primary/15 text-primary" : "bg-base-300/50",
              )}
            >
              {t("networkRouteMetric", { ns: "settings" })}: {formatRouteMetric(item.routeMetric)}
            </span>
          </span>
        </button>
      ))}
    </div>
  );
}

function NetworkPriorityPanel({
  items,
  busy,
  t,
  onPreferWired,
  onPreferWiFi,
  onPreferCellular,
  onAuto,
  onMove,
}: {
  items: NetworkInterface[];
  busy: boolean;
  t: TFunction;
  onPreferWired: () => void;
  onPreferWiFi: () => void;
  onPreferCellular: () => void;
  onAuto: () => void;
  onMove: (fromIndex: number, toIndex: number) => void;
}) {
  if (!items.length) {
    return null;
  }

  return (
    <Panel>
      <PanelBody>
        <SectionHeader
          title={t("networkPriorityTitle", { ns: "settings" })}
          description={t("networkPriorityDescription", { ns: "settings" })}
        />
        <div className="grid gap-3 lg:grid-cols-[14rem_minmax(0,1fr)]">
          <div className="grid content-start gap-2">
            <button className={cx("btn btn-sm btn-outline justify-start", busy && "app-busy-button")} type="button" disabled={busy} onClick={onPreferWired}>
              {busy ? <LoadingSpinner size={15} /> : <Cable size={15} />}
              {t("networkPreferWired", { ns: "settings" })}
            </button>
            <button className={cx("btn btn-sm btn-outline justify-start", busy && "app-busy-button")} type="button" disabled={busy} onClick={onPreferWiFi}>
              {busy ? <LoadingSpinner size={15} /> : <Wifi size={15} />}
              {t("networkPreferWifi", { ns: "settings" })}
            </button>
            <button className={cx("btn btn-sm btn-outline justify-start", busy && "app-busy-button")} type="button" disabled={busy} onClick={onPreferCellular}>
              {busy ? <LoadingSpinner size={15} /> : <RadioTower size={15} />}
              {t("networkPreferCellular", { ns: "settings" })}
            </button>
            <button className={cx("btn btn-sm btn-ghost justify-start", busy && "app-busy-button")} type="button" disabled={busy} onClick={onAuto}>
              {busy ? <LoadingSpinner size={15} /> : <RefreshCw size={15} />}
              {t("networkPriorityAutoAction", { ns: "settings" })}
            </button>
          </div>
          <div className="grid gap-2">
            {items.map((item, index) => (
              <div
                className="grid gap-2 rounded-2xl border border-base-300 bg-base-100/40 p-2 sm:grid-cols-[2.25rem_minmax(0,1fr)_auto]"
                key={item.name}
              >
                <div className="grid size-9 place-items-center rounded-xl bg-primary/10 text-sm font-bold text-primary">
                  {index + 1}
                </div>
                <div className="min-w-0 self-center">
                  <div className="flex min-w-0 flex-wrap items-center gap-2">
                    <strong className="truncate text-sm font-semibold text-base-content">{item.name}</strong>
                    <span className="badge badge-xs border-0 bg-base-300 text-base-content/70">{interfaceTypeLabel(item, t)}</span>
                    {item.state === "connected" ? <span className="badge badge-success badge-xs">{t("active", { ns: "common" })}</span> : null}
                    {item.defaultRoute ? <span className="badge badge-info badge-xs">{t("networkDefaultRouteBadge", { ns: "settings" })}</span> : null}
                  </div>
                  <p className="mt-1 truncate text-xs text-base-content/50">
                    {item.connectionName || "-"} · {t("networkRouteMetric", { ns: "settings" })}: {formatRouteMetric(item.routeMetric)}
                  </p>
                </div>
                <div className="join justify-self-start sm:justify-self-end">
                  <button
                    className="btn btn-xs join-item"
                    type="button"
                    disabled={busy || index === 0}
                    aria-label={t("networkPriorityMoveUp", { ns: "settings" })}
                    title={t("networkPriorityMoveUp", { ns: "settings" })}
                    onClick={() => onMove(index, index - 1)}
                  >
                    <ArrowUp size={14} />
                  </button>
                  <button
                    className="btn btn-xs join-item"
                    type="button"
                    disabled={busy || index === items.length - 1}
                    aria-label={t("networkPriorityMoveDown", { ns: "settings" })}
                    title={t("networkPriorityMoveDown", { ns: "settings" })}
                    onClick={() => onMove(index, index + 1)}
                  >
                    <ArrowDown size={14} />
                  </button>
                </div>
              </div>
            ))}
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

function DetailItem({
  label,
  value,
  mono = false,
  strong = false,
}: {
  label: string;
  value: string;
  mono?: boolean;
  strong?: boolean;
}) {
  return (
    <div className="grid min-w-0 gap-1 rounded-xl border border-base-300/80 bg-base-100/35 px-3 py-2 sm:grid-cols-[5.75rem_minmax(0,1fr)] sm:items-center">
      <span className="text-[11px] font-semibold text-base-content/45">{label}</span>
      <strong
        className={cx(
          "min-w-0 text-sm leading-5 text-base-content",
          mono ? "font-mono tabular-nums [overflow-wrap:anywhere]" : "break-words font-semibold",
          strong ? "font-bold text-primary" : "",
        )}
      >
        {value}
      </strong>
    </div>
  );
}

function getSignalLevel(signal: number) {
  const value = Math.max(0, Math.min(100, signal || 0));
  if (value >= 75) {
    return 4;
  }
  if (value >= 50) {
    return 3;
  }
  if (value >= 25) {
    return 2;
  }
  if (value > 0) {
    return 1;
  }
  return 0;
}

function getSignalLabelKey(level: number) {
  if (level >= 4) {
    return "wifiSignalExcellent";
  }
  if (level === 3) {
    return "wifiSignalGood";
  }
  if (level === 2) {
    return "wifiSignalFair";
  }
  if (level === 1) {
    return "wifiSignalWeak";
  }
  return "wifiSignalNone";
}

function SignalStrength({ signal, t }: { signal: number; t: TFunction }) {
  const normalized = Math.max(0, Math.min(100, signal || 0));
  const level = getSignalLevel(normalized);
  const label = t(getSignalLabelKey(level), { ns: "settings" });
  const activeColor = level >= 3 ? "bg-success" : level === 2 ? "bg-warning" : "bg-error";
  const textColor = level >= 3 ? "text-success" : level === 2 ? "text-warning" : "text-error";
  const heights = ["h-1.5", "h-2.5", "h-3.5", "h-5"];

  return (
    <span className="flex shrink-0 items-center gap-2" aria-label={`${label} ${normalized}%`} title={`${label} ${normalized}%`}>
      <span className="flex h-5 items-end gap-0.5" aria-hidden="true">
        {heights.map((height, index) => (
          <span
            className={cx(
              "w-1.5 rounded-sm",
              height,
              index < level ? activeColor : "bg-base-content/15",
            )}
            key={height}
          />
        ))}
      </span>
      <span className="grid justify-items-end leading-none">
        <strong className={cx("min-w-8 text-right text-[11px] font-bold tabular-nums", textColor)}>{normalized}%</strong>
        <span className="mt-1 text-[10px] font-semibold text-base-content/45">{label}</span>
      </span>
    </span>
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
  onDisconnect,
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
  onDisconnect: (device?: string) => void;
}) {
  const selected = networks.find((item) => item.ssid === selectedSSID);
  const activeNetwork = networks.find((item) => item.active);
  const requiresPassword = Boolean(selected?.security && selected.security !== "--");
  const passwordDisabled = busy || Boolean(selected && !requiresPassword);
  const [showPassword, setShowPassword] = useState(true);

  return (
    <Panel>
      <PanelBody>
        <SectionHeader
          title={t("wifiTitle", { ns: "settings" })}
          description={t("wifiDescription", { ns: "settings" })}
          action={
            <>
              {activeNetwork ? (
                <button
                  className={cx("btn btn-sm btn-outline btn-error", busy && "app-busy-button")}
                  type="button"
                  disabled={busy}
                  onClick={() => onDisconnect(activeNetwork.device)}
                >
                  {busy ? <LoadingSpinner size={16} /> : <WifiOff size={16} />}
                  <span>{t("wifiDisconnect", { ns: "settings" })}</span>
                </button>
              ) : null}
              <button className={cx("btn btn-sm btn-outline btn-info", busy && "app-busy-button")} type="button" disabled={busy} onClick={onScan}>
                {busy ? <LoadingSpinner size={16} /> : <RefreshCw size={16} />}
                <span>{t("wifiScan", { ns: "settings" })}</span>
              </button>
            </>
          }
        />

        {!available ? (
          <div className="alert alert-soft alert-warning py-3 text-sm" role="status">
            <CircleAlert size={16} />
            <span className="min-w-0 [overflow-wrap:anywhere]">{message || t("wifiUnavailable", { ns: "settings" })}</span>
          </div>
        ) : (
          <div className="grid gap-3 lg:grid-cols-[minmax(0,1fr)_minmax(18rem,22rem)]">
            <div className="grid max-h-80 gap-2 overflow-y-auto pr-1">
              {networks.map((network) => (
                <button
                  className={cx(
                    "flex min-w-0 items-center justify-between gap-3 rounded-2xl border p-3 text-left",
                    network.active
                      ? "border-success/45 bg-success/10 text-success"
                      : network.ssid === selectedSSID
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
                    <SignalStrength signal={network.signal} t={t} />
                  </span>
                </button>
              ))}
              {networks.length === 0 ? (
                <div className="admin-empty-state admin-empty-state--compact">
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
                <div className="join w-full">
                  <input
                    className="input input-sm input-bordered join-item min-w-0 flex-1 bg-base-100"
                    value={password}
                    type={showPassword ? "text" : "password"}
                    data-keyboard="ascii"
                    disabled={passwordDisabled}
                    onChange={(event) => onPasswordChange(event.target.value)}
                    placeholder={passwordDisabled ? t("wifiOpen", { ns: "settings" }) : t("wifiPasswordPlaceholder", { ns: "settings" })}
                  />
                  <button
                    className="btn btn-sm btn-outline join-item border-base-300 px-3"
                    type="button"
                    disabled={passwordDisabled}
                    aria-label={t(showPassword ? "wifiHidePassword" : "wifiShowPassword", { ns: "settings" })}
                    title={t(showPassword ? "wifiHidePassword" : "wifiShowPassword", { ns: "settings" })}
                    onClick={() => setShowPassword((value) => !value)}
                  >
                    {showPassword ? <EyeOff size={15} /> : <Eye size={15} />}
                  </button>
                </div>
              </label>
              <button
                className={cx("btn btn-primary btn-sm", busy && "app-busy-button")}
                type="button"
                disabled={busy || !selectedSSID.trim() || (requiresPassword && password.trim().length < 8)}
                onClick={() => onConnect(selected?.device)}
              >
                {busy ? <LoadingSpinner size={15} /> : <Wifi size={15} />}
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
  const [selectedInterfaceName, setSelectedInterfaceName] = useState("");
  const [cellularDrafts, setCellularDrafts] = useState<Record<string, CellularDraft>>({});
  const [loading, setLoading] = useState(false);
  const [saving, setSaving] = useState("");
  const [cellularSaving, setCellularSaving] = useState("");
  const [prioritySaving, setPrioritySaving] = useState(false);
  const [wifiBusy, setWifiBusy] = useState(false);

  const load = async (options?: { silent?: boolean }) => {
    setLoading(true);
    if (!options?.silent) {
      setBanner({ kind: "loading", message: t("loading", { ns: "common" }) });
    }
    try {
      const response = await getNetworkInterfaces(locale, developerToken);
      setInterfaces(response.interfaces);
      setDrafts(Object.fromEntries(response.interfaces.map((item) => [item.name, toDraft(item)])));
      setCellularDrafts((items) => ({
        ...Object.fromEntries(response.interfaces.filter(isCellularInterface).map((item) => [item.name, items[item.name] ?? toCellularDraft(item)])),
      }));
      setSelectedInterfaceName((current) => {
        if (current && response.interfaces.some((item) => item.name === current)) {
          return current;
        }
        return pickDefaultInterface(response.interfaces)?.name ?? "";
      });
      setBackendState({
        available: response.available,
        readOnly: response.readOnly,
        message: response.message ?? "",
      });
      if (!options?.silent) {
        setBanner({ kind: "idle", message: "" });
      }
    } catch (error) {
      setBanner({ kind: "error", message: extractErrorMessage(error, t("unexpectedError", { ns: "common" })) });
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
        const activeNetwork = response.networks.find((item) => item.active);
        if (activeNetwork) {
          setSelectedSSID(activeNetwork.ssid);
        }
      }
    } catch (error) {
      setWifiState({ available: false, readOnly: true, message: extractErrorMessage(error, t("unexpectedError", { ns: "common" })) });
    } finally {
      setWifiBusy(false);
    }
  };

  useEffect(() => {
    void load();
    void scanWiFi();
  }, [locale, developerToken]);

  const managedCount = useMemo(() => interfaces.filter((item) => item.managed).length, [interfaces]);
  const cellularCount = useMemo(() => interfaces.filter(isCellularInterface).length, [interfaces]);
  const pickerInterfaces = useMemo(() => sortedInterfacePickerItems(interfaces), [interfaces]);
  const priorityInterfaces = useMemo(() => sortedPriorityInterfaces(interfaces), [interfaces]);
  const selectedInterface = useMemo(
    () => interfaces.find((item) => item.name === selectedInterfaceName) ?? pickDefaultInterface(interfaces),
    [interfaces, selectedInterfaceName],
  );

  const applyPriorityOrder = async (orderedItems: NetworkInterface[]) => {
    if (orderedItems.length === 0) {
      return;
    }
    setPrioritySaving(true);
    setBanner({ kind: "loading", message: t("loading", { ns: "common" }) });
    try {
      const response = await updateNetworkInterfacePriorities(
        {
          priorities: orderedItems.map((item, index) => ({
            interfaceName: item.name,
            routeMetric: getPriorityMetric(index),
          })),
        },
        locale,
        developerToken,
      );
      const latest = response.interfaces;
      setInterfaces(latest);
      setDrafts((items) => ({
        ...items,
        ...Object.fromEntries(latest.map((item) => [item.name, toDraft(item)])),
      }));
      setBanner({ kind: "success", message: response.message || t("networkPriorityUpdated", { ns: "settings" }) });
      await load({ silent: true });
    } catch (error) {
      setBanner({ kind: "error", message: extractErrorMessage(error, t("unexpectedError", { ns: "common" })) });
    } finally {
      setPrioritySaving(false);
    }
  };

  const applyAutomaticPriority = async () => {
    if (priorityInterfaces.length === 0) {
      return;
    }
    setPrioritySaving(true);
    setBanner({ kind: "loading", message: t("loading", { ns: "common" }) });
    try {
      const response = await updateNetworkInterfacePriorities(
        {
          priorities: priorityInterfaces.map((item) => ({
            interfaceName: item.name,
            routeMetric: -1,
          })),
        },
        locale,
        developerToken,
      );
      const latest = response.interfaces;
      setInterfaces(latest);
      setDrafts((items) => ({
        ...items,
        ...Object.fromEntries(latest.map((item) => [item.name, toDraft(item)])),
      }));
      setBanner({ kind: "success", message: response.message || t("networkPriorityUpdated", { ns: "settings" }) });
      await load({ silent: true });
    } catch (error) {
      setBanner({ kind: "error", message: extractErrorMessage(error, t("unexpectedError", { ns: "common" })) });
    } finally {
      setPrioritySaving(false);
    }
  };

  return (
    <section className="grid gap-3">
      <Panel>
        <PanelBody>
          <SectionHeader
            title={t("networkTitle", { ns: "settings" })}
            description={t("networkDescription", { ns: "settings" })}
            action={
              <button className={cx("btn btn-sm btn-outline btn-info", loading && "app-busy-button")} type="button" disabled={loading} onClick={() => void load()}>
                {loading ? <LoadingSpinner size={16} /> : <RefreshCw size={16} />}
                <span>{t("refresh", { ns: "common" })}</span>
              </button>
            }
          />
          <div className="grid gap-2 sm:grid-cols-2 xl:grid-cols-4">
            <Info label={t("networkInterfaceCount", { ns: "settings" })} value={String(interfaces.length)} />
            <Info label={t("networkManagedCount", { ns: "settings" })} value={String(managedCount)} />
            <Info label={t("cellularCount", { ns: "settings" })} value={String(cellularCount)} />
            <Info label={t("networkBackend", { ns: "settings" })} value="NetworkManager + ModemManager" />
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
            setBanner({ kind: "loading", message: t("loading", { ns: "common" }) });
            try {
              const response = await connectWiFi(
                {
                  ssid: selectedSSID.trim(),
                  password: wifiPassword,
                  device,
                },
                locale,
                developerToken,
              );
              setBanner({ kind: "success", message: response.message });
              setWifiPassword("");
              await scanWiFi();
              await load({ silent: true });
            } catch (error) {
              setBanner({ kind: "error", message: extractErrorMessage(error, t("unexpectedError", { ns: "common" })) });
            } finally {
              setWifiBusy(false);
            }
          })();
        }}
        onDisconnect={(device) => {
          void (async () => {
            setWifiBusy(true);
            setBanner({ kind: "loading", message: t("loading", { ns: "common" }) });
            try {
              const response = await disconnectWiFi({ device }, locale, developerToken);
              setBanner({ kind: "success", message: response.message });
              setSelectedSSID("");
              setWifiPassword("");
              await scanWiFi();
              await load({ silent: true });
            } catch (error) {
              setBanner({ kind: "error", message: extractErrorMessage(error, t("unexpectedError", { ns: "common" })) });
            } finally {
              setWifiBusy(false);
            }
          })();
        }}
      />

      <NetworkPriorityPanel
        items={priorityInterfaces}
        busy={prioritySaving || loading}
        t={t}
        onPreferWired={() => {
          const wired = priorityInterfaces.filter(isWiredInterface);
          const wifi = priorityInterfaces.filter(isWiFiInterface);
          const cellular = priorityInterfaces.filter(isCellularInterface);
          const other = priorityInterfaces.filter((item) => !isWiredInterface(item) && !isWiFiInterface(item) && !isCellularInterface(item));
          void applyPriorityOrder([...wired, ...wifi, ...cellular, ...other]);
        }}
        onPreferWiFi={() => {
          const wifi = priorityInterfaces.filter(isWiFiInterface);
          const wired = priorityInterfaces.filter(isWiredInterface);
          const cellular = priorityInterfaces.filter(isCellularInterface);
          const other = priorityInterfaces.filter((item) => !isWiredInterface(item) && !isWiFiInterface(item) && !isCellularInterface(item));
          void applyPriorityOrder([...wifi, ...wired, ...cellular, ...other]);
        }}
        onPreferCellular={() => {
          const cellular = priorityInterfaces.filter(isCellularInterface);
          const wired = priorityInterfaces.filter(isWiredInterface);
          const wifi = priorityInterfaces.filter(isWiFiInterface);
          const other = priorityInterfaces.filter((item) => !isWiredInterface(item) && !isWiFiInterface(item) && !isCellularInterface(item));
          void applyPriorityOrder([...cellular, ...wired, ...wifi, ...other]);
        }}
        onAuto={() => void applyAutomaticPriority()}
        onMove={(fromIndex, toIndex) => {
          const ordered = [...priorityInterfaces];
          const [item] = ordered.splice(fromIndex, 1);
          if (!item) {
            return;
          }
          ordered.splice(toIndex, 0, item);
          void applyPriorityOrder(ordered);
        }}
      />

      {selectedInterface ? (
        <Panel>
          <PanelBody>
            <div className="grid gap-3 lg:grid-cols-[13rem_minmax(0,1fr)]">
              <div className="grid content-start gap-3">
                <div>
                  <h3 className="text-sm font-semibold text-base-content">{t("networkSelectInterface", { ns: "settings" })}</h3>
                  <p className="mt-1 text-xs leading-5 text-base-content/55">{t("networkSelectInterfaceHint", { ns: "settings" })}</p>
                </div>
                <InterfacePicker
                  items={pickerInterfaces}
                  selectedName={selectedInterface.name}
                  t={t}
                  onSelect={setSelectedInterfaceName}
                />
              </div>
              <InterfaceCard
                item={selectedInterface}
                draft={drafts[selectedInterface.name] ?? toDraft(selectedInterface)}
                cellularDraft={cellularDrafts[selectedInterface.name] ?? toCellularDraft(selectedInterface)}
                busy={saving === selectedInterface.name}
                cellularBusy={cellularSaving === selectedInterface.name}
                t={t}
                onDraftChange={(draft) => setDrafts((items) => ({ ...items, [selectedInterface.name]: draft }))}
                onCellularDraftChange={(draft) => setCellularDrafts((items) => ({ ...items, [selectedInterface.name]: draft }))}
                onSave={() => {
                  void (async () => {
                    const draft = drafts[selectedInterface.name] ?? toDraft(selectedInterface);
                    setSaving(selectedInterface.name);
                    setBanner({ kind: "loading", message: t("loading", { ns: "common" }) });
                    try {
                      const response = await updateNetworkInterface(selectedInterface.name, buildPayload(draft), locale, developerToken);
                      setInterfaces((items) => items.map((current) => (current.name === selectedInterface.name ? response.interface : current)));
                      setDrafts((items) => ({ ...items, [selectedInterface.name]: toDraft(response.interface) }));
                      setBanner({ kind: "success", message: response.message });
                    } catch (error) {
                      setBanner({ kind: "error", message: extractErrorMessage(error, t("unexpectedError", { ns: "common" })) });
                    } finally {
                      setSaving("");
                    }
                  })();
                }}
                onCellularConnect={() => {
                  void (async () => {
                    const draft = cellularDrafts[selectedInterface.name] ?? toCellularDraft(selectedInterface);
                    setCellularSaving(selectedInterface.name);
                    setBanner({ kind: "loading", message: t("loading", { ns: "common" }) });
                    try {
                      const routeMetric = parseRouteMetric(draft.routeMetric);
                      const response = await connectCellular(
                        {
                          interfaceName: selectedInterface.name,
                          modemId: selectedInterface.modem?.id,
                          apn: draft.apn.trim(),
                          username: draft.username.trim(),
                          password: draft.password,
                          connectionName: draft.connectionName.trim(),
                          routeMetric,
                        },
                        locale,
                        developerToken,
                      );
                      setInterfaces(response.interfaces);
                      setDrafts((items) => ({
                        ...items,
                        ...Object.fromEntries(response.interfaces.map((item) => [item.name, toDraft(item)])),
                      }));
                      setBanner({ kind: "success", message: response.message });
                      await load({ silent: true });
                    } catch (error) {
                      setBanner({ kind: "error", message: extractErrorMessage(error, t("unexpectedError", { ns: "common" })) });
                    } finally {
                      setCellularSaving("");
                    }
                  })();
                }}
              />
            </div>
          </PanelBody>
        </Panel>
      ) : null}

      {!loading && interfaces.length === 0 && !banner.message && backendState.available ? (
        <Panel>
          <PanelBody>
            <div className="admin-empty-state admin-empty-state--compact">{t("networkEmpty", { ns: "settings" })}</div>
          </PanelBody>
        </Panel>
      ) : null}
    </section>
  );
}
