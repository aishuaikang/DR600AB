import { useEffect, useState } from "react";
import type { TFunction } from "i18next";

import { Panel, PanelBody } from "../components/Panel";
import { SectionHeader } from "../components/SectionHeader";

export function UserSettingsPage({
  appTitle,
  defaultAppTitle,
  t,
  onAppTitleChange,
}: {
  appTitle: string;
  defaultAppTitle: string;
  t: TFunction;
  onAppTitleChange: (value: string) => void;
}) {
  const [titleDraft, setTitleDraft] = useState(appTitle);
  const normalizedDraft = titleDraft.trim();
  const changed = normalizedDraft !== appTitle;

  useEffect(() => {
    setTitleDraft(appTitle);
  }, [appTitle]);

  return (
    <section className="grid gap-3">
      <Panel>
        <PanelBody>
          <SectionHeader
            title={t("displayTitle", { ns: "settings" })}
            description={t("displayDescription", { ns: "settings" })}
          />

          <div className="grid gap-3 lg:grid-cols-[minmax(0,1fr)_18rem]">
            <label className="grid gap-1.5">
              <span className="text-xs font-medium text-base-content/60">{t("customTitle", { ns: "settings" })}</span>
              <input
                className="input input-bordered input-sm w-full bg-base-100"
                value={titleDraft}
                maxLength={32}
                placeholder={defaultAppTitle}
                onChange={(event) => setTitleDraft(event.target.value)}
              />
              <span className="text-xs leading-5 text-base-content/50">{t("customTitleHint", { ns: "settings" })}</span>
            </label>

            <div className="rounded-2xl border border-base-300 bg-base-100/45 p-3">
              <span className="text-[11px] font-semibold uppercase tracking-wide text-base-content/45">{t("preview", { ns: "settings" })}</span>
              <strong className="mt-2 block truncate text-sm font-semibold text-base-content">
                {normalizedDraft || defaultAppTitle}
              </strong>
            </div>
          </div>

          <div className="flex flex-wrap justify-end gap-2">
            <button
              className="btn btn-sm btn-outline"
              type="button"
              onClick={() => {
                setTitleDraft(defaultAppTitle);
                onAppTitleChange("");
              }}
            >
              {t("restoreDefault", { ns: "settings" })}
            </button>
            <button
              className="btn btn-sm btn-primary"
              type="button"
              disabled={!changed}
              onClick={() => onAppTitleChange(normalizedDraft)}
            >
              {t("save", { ns: "common" })}
            </button>
          </div>
        </PanelBody>
      </Panel>
    </section>
  );
}
