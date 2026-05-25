import { useMemo, useState } from "react";
import type { TFunction } from "i18next";
import { Plus, ShieldCheck, Trash2 } from "lucide-react";

import { Panel, PanelBody } from "../components/Panel";
import { SectionHeader } from "../components/SectionHeader";
import type { UserSettings } from "../types";
import { extractErrorMessage } from "../utils/session";
import { removeWhitelistSerial, upsertWhitelistItem } from "../utils/whitelist";

function formatWhitelistDate(value: string | undefined, locale: string) {
  if (!value) {
    return "-";
  }
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return "-";
  }
  return date.toLocaleString(locale.startsWith("zh") ? "zh-CN" : "en-US", { hour12: false });
}

function formatWhitelistSource(value: string | undefined, t: TFunction) {
  const source = value?.trim();
  if (!source) {
    return "-";
  }
  if (source === "manual") {
    return t("whitelistSourceManual", { ns: "settings" });
  }
  if (source === "screen_position") {
    return t("whitelistSourceScreenPosition", { ns: "settings" });
  }
  return source;
}

export function WhitelistPage({
  locale,
  userSettings,
  t,
  onUserSettingsChange,
}: {
  locale: string;
  userSettings: UserSettings;
  t: TFunction;
  onUserSettingsChange: (settings: UserSettings) => Promise<UserSettings>;
}) {
  const [serialDraft, setSerialDraft] = useState("");
  const [modelDraft, setModelDraft] = useState("");
  const [saving, setSaving] = useState(false);
  const [message, setMessage] = useState<{ kind: "idle" | "success" | "error"; text: string }>({
    kind: "idle",
    text: "",
  });
  const whitelist = userSettings.whitelist ?? [];
  const serial = serialDraft.trim();
  const sortedWhitelist = useMemo(() => {
    return [...whitelist].sort((left, right) => {
      const leftTime = left.createdAt ? new Date(left.createdAt).getTime() : 0;
      const rightTime = right.createdAt ? new Date(right.createdAt).getTime() : 0;
      return rightTime - leftTime || left.serial.localeCompare(right.serial);
    });
  }, [whitelist]);

  const addWhitelistItem = async () => {
    if (!serial) {
      setMessage({ kind: "error", text: t("whitelistSerialRequired", { ns: "settings" }) });
      return;
    }
    setSaving(true);
    setMessage({ kind: "idle", text: "" });
    try {
      await onUserSettingsChange({
        ...userSettings,
        whitelist: upsertWhitelistItem(whitelist, {
          serial,
          model: modelDraft,
          source: "manual",
        }),
      });
      setSerialDraft("");
      setModelDraft("");
      setMessage({ kind: "success", text: t("whitelistSaved", { ns: "settings" }) });
    } catch (error) {
      setMessage({ kind: "error", text: extractErrorMessage(error, t("unexpectedError", { ns: "common" })) });
    } finally {
      setSaving(false);
    }
  };

  const deleteWhitelistItem = async (targetSerial: string) => {
    setSaving(true);
    setMessage({ kind: "idle", text: "" });
    try {
      await onUserSettingsChange({
        ...userSettings,
        whitelist: removeWhitelistSerial(whitelist, targetSerial),
      });
      setMessage({ kind: "success", text: t("whitelistDeleted", { ns: "settings" }) });
    } catch (error) {
      setMessage({ kind: "error", text: extractErrorMessage(error, t("unexpectedError", { ns: "common" })) });
    } finally {
      setSaving(false);
    }
  };

  return (
    <section className="grid min-h-0 gap-3">
      <Panel>
        <PanelBody>
          <SectionHeader
            title={t("whitelistTitle", { ns: "settings" })}
            description={t("whitelistDescription", { ns: "settings" })}
            action={
              <span className="inline-flex h-8 items-center gap-2 rounded-xl border border-success/25 bg-success/10 px-3 text-xs font-semibold text-success">
                <ShieldCheck size={15} aria-hidden="true" />
                {t("whitelistCount", { ns: "settings", count: whitelist.length })}
              </span>
            }
          />

          <div className="grid gap-3 lg:grid-cols-[minmax(0,1fr)_12rem]">
            <div className="grid gap-3 sm:grid-cols-2">
              <label className="grid gap-1.5">
                <span className="text-xs font-medium text-base-content/60">{t("whitelistSerial", { ns: "settings" })}</span>
                <input
                  className="input input-bordered input-sm w-full bg-base-100"
                  value={serialDraft}
                  maxLength={128}
                  placeholder="UAV-8F12"
                  onChange={(event) => setSerialDraft(event.target.value)}
                />
              </label>
              <label className="grid gap-1.5">
                <span className="text-xs font-medium text-base-content/60">{t("whitelistModel", { ns: "settings" })}</span>
                <input
                  className="input input-bordered input-sm w-full bg-base-100"
                  value={modelDraft}
                  maxLength={64}
                  placeholder={t("whitelistModelPlaceholder", { ns: "settings" })}
                  onChange={(event) => setModelDraft(event.target.value)}
                />
              </label>
            </div>

            <button
              className="btn btn-sm btn-primary self-end"
              type="button"
              disabled={saving || !serial}
              onClick={() => void addWhitelistItem()}
            >
              <Plus size={15} aria-hidden="true" />
              {t("whitelistAdd", { ns: "settings" })}
            </button>
          </div>

          {message.text ? (
            <div className={`alert py-2 text-sm ${message.kind === "error" ? "alert-error" : "alert-success"}`}>
              {message.text}
            </div>
          ) : null}
        </PanelBody>
      </Panel>

      <Panel className="min-h-0">
        <PanelBody className="min-h-0">
          <div className="overflow-x-auto rounded-2xl border border-base-300 bg-base-100/45">
            <table className="table table-sm">
              <thead>
                <tr>
                  <th>{t("whitelistSerial", { ns: "settings" })}</th>
                  <th>{t("whitelistModel", { ns: "settings" })}</th>
                  <th>{t("whitelistSource", { ns: "settings" })}</th>
                  <th>{t("whitelistCreatedAt", { ns: "settings" })}</th>
                  <th className="text-right">{t("whitelistActions", { ns: "settings" })}</th>
                </tr>
              </thead>
              <tbody>
                {sortedWhitelist.length > 0 ? sortedWhitelist.map((item) => (
                  <tr key={`${item.serial}-${item.createdAt ?? ""}`}>
                    <td className="font-mono font-semibold">{item.serial}</td>
                    <td>{item.model || "-"}</td>
                    <td>{formatWhitelistSource(item.source, t)}</td>
                    <td>{formatWhitelistDate(item.createdAt, locale)}</td>
                    <td className="text-right">
                      <button
                        className="btn btn-ghost btn-xs text-error"
                        type="button"
                        disabled={saving}
                        aria-label={t("whitelistDelete", { ns: "settings", serial: item.serial })}
                        onClick={() => void deleteWhitelistItem(item.serial)}
                      >
                        <Trash2 size={14} aria-hidden="true" />
                      </button>
                    </td>
                  </tr>
                )) : (
                  <tr>
                    <td colSpan={5} className="py-8 text-center text-sm text-base-content/50">
                      {t("whitelistEmpty", { ns: "settings" })}
                    </td>
                  </tr>
                )}
              </tbody>
            </table>
          </div>
        </PanelBody>
      </Panel>
    </section>
  );
}
