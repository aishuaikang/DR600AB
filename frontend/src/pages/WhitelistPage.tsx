import { useEffect, useMemo, useState } from "react";
import type { TFunction } from "i18next";
import { Check, ChevronDown, Pencil, Plus, ShieldCheck, Trash2, X } from "lucide-react";

import { Panel, PanelBody } from "../components/Panel";
import { SectionHeader } from "../components/SectionHeader";
import type { UserSettings } from "../types";
import { extractErrorMessage } from "../utils/session";
import { removeWhitelistSerial, updateWhitelistItem, upsertWhitelistItem } from "../utils/whitelist";

const whitelistPageSize = 50;

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
  const [editingSerial, setEditingSerial] = useState("");
  const [editSerialDraft, setEditSerialDraft] = useState("");
  const [editModelDraft, setEditModelDraft] = useState("");
  const [saving, setSaving] = useState(false);
  const [message, setMessage] = useState<{ kind: "idle" | "success" | "error"; text: string }>({
    kind: "idle",
    text: "",
  });
  const whitelist = userSettings.whitelist ?? [];
  const sortedWhitelist = useMemo(() => {
    return [...whitelist].sort((left, right) => {
      const leftTime = left.createdAt ? new Date(left.createdAt).getTime() : 0;
      const rightTime = right.createdAt ? new Date(right.createdAt).getTime() : 0;
      return rightTime - leftTime || left.serial.localeCompare(right.serial);
    });
  }, [whitelist]);
  const [visibleLimit, setVisibleLimit] = useState(whitelistPageSize);
  const visibleWhitelist = sortedWhitelist.slice(0, visibleLimit);
  const hasMoreWhitelist = visibleLimit < sortedWhitelist.length;
  const serial = serialDraft.trim();

  useEffect(() => {
    setVisibleLimit((current) => Math.min(current, Math.max(sortedWhitelist.length, whitelistPageSize)));
  }, [sortedWhitelist.length]);

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

  const startEditWhitelistItem = (targetSerial: string, targetModel: string | undefined) => {
    setEditingSerial(targetSerial);
    setEditSerialDraft(targetSerial);
    setEditModelDraft(targetModel ?? "");
    setMessage({ kind: "idle", text: "" });
  };

  const cancelEditWhitelistItem = () => {
    setEditingSerial("");
    setEditSerialDraft("");
    setEditModelDraft("");
  };

  const saveEditWhitelistItem = async () => {
    const nextSerial = editSerialDraft.trim();
    if (!nextSerial) {
      setMessage({ kind: "error", text: t("whitelistSerialRequired", { ns: "settings" }) });
      return;
    }
    setSaving(true);
    setMessage({ kind: "idle", text: "" });
    try {
      await onUserSettingsChange({
        ...userSettings,
        whitelist: updateWhitelistItem(whitelist, editingSerial, {
          serial: nextSerial,
          model: editModelDraft,
        }),
      });
      cancelEditWhitelistItem();
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
      if (editingSerial === targetSerial) {
        cancelEditWhitelistItem();
      }
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
                  <th>{t("whitelistCreatedAt", { ns: "settings" })}</th>
                  <th className="text-right">{t("whitelistActions", { ns: "settings" })}</th>
                </tr>
              </thead>
              <tbody>
                {visibleWhitelist.length > 0 ? visibleWhitelist.map((item) => {
                  const editing = editingSerial === item.serial;
                  return (
                    <tr key={`${item.serial}-${item.createdAt ?? ""}`}>
                      <td className="min-w-52">
                        {editing ? (
                          <input
                            className="input input-bordered input-xs w-full bg-base-100 font-mono"
                            value={editSerialDraft}
                            maxLength={128}
                            onChange={(event) => setEditSerialDraft(event.target.value)}
                          />
                        ) : (
                          <span className="font-mono font-semibold">{item.serial}</span>
                        )}
                      </td>
                      <td className="min-w-44">
                        {editing ? (
                          <input
                            className="input input-bordered input-xs w-full bg-base-100"
                            value={editModelDraft}
                            maxLength={64}
                            placeholder={t("whitelistModelPlaceholder", { ns: "settings" })}
                            onChange={(event) => setEditModelDraft(event.target.value)}
                          />
                        ) : item.model || "-"}
                      </td>
                      <td>{formatWhitelistDate(item.createdAt, locale)}</td>
                      <td className="text-right">
                        <div className="flex justify-end gap-1">
                          {editing ? (
                            <>
                              <button
                                className="btn btn-ghost btn-xs text-success"
                                type="button"
                                disabled={saving}
                                aria-label={t("save", { ns: "common" })}
                                onClick={() => void saveEditWhitelistItem()}
                              >
                                <Check size={14} aria-hidden="true" />
                              </button>
                              <button
                                className="btn btn-ghost btn-xs"
                                type="button"
                                disabled={saving}
                                aria-label={t("cancel", { ns: "common" })}
                                onClick={cancelEditWhitelistItem}
                              >
                                <X size={14} aria-hidden="true" />
                              </button>
                            </>
                          ) : (
                            <>
                              <button
                                className="btn btn-ghost btn-xs"
                                type="button"
                                disabled={saving}
                                aria-label={t("whitelistEdit", { ns: "settings", serial: item.serial })}
                                onClick={() => startEditWhitelistItem(item.serial, item.model)}
                              >
                                <Pencil size={14} aria-hidden="true" />
                              </button>
                              <button
                                className="btn btn-ghost btn-xs text-error"
                                type="button"
                                disabled={saving}
                                aria-label={t("whitelistDelete", { ns: "settings", serial: item.serial })}
                                onClick={() => void deleteWhitelistItem(item.serial)}
                              >
                                <Trash2 size={14} aria-hidden="true" />
                              </button>
                            </>
                          )}
                        </div>
                      </td>
                    </tr>
                  );
                }) : (
                  <tr>
                    <td colSpan={4} className="py-8 text-center text-sm text-base-content/50">
                      {t("whitelistEmpty", { ns: "settings" })}
                    </td>
                  </tr>
                )}
              </tbody>
            </table>
          </div>
          {hasMoreWhitelist ? (
            <div className="flex justify-center">
              <button
                className="btn btn-sm btn-outline"
                type="button"
                disabled={saving}
                onClick={() => setVisibleLimit((current) => current + whitelistPageSize)}
              >
                <ChevronDown size={15} aria-hidden="true" />
                {t("loadMore", { ns: "common" })}
              </button>
            </div>
          ) : null}
        </PanelBody>
      </Panel>
    </section>
  );
}
