import { useMemo, useState, type FormEvent } from "react";
import type { TFunction } from "i18next";
import { SendHorizontal } from "lucide-react";

import { MESSAGE_PAGE_CONFIG } from "../app/message-pages";
import type { Banner } from "../app/types";
import { BannerAlert } from "../components/BannerAlert";
import { Panel, PanelBody } from "../components/Panel";
import { VirtualMessageTable } from "../components/VirtualMessageTable";
import type { DebugRecord, DebugRecordPage } from "../types";
import { cx } from "../utils/classnames";
import { buildSearchText } from "../utils/records";
import { extractErrorMessage } from "../utils/session";

export function MessagePage({
  page,
  records,
  locale,
  query,
  onQueryChange,
  onSendDetectionCommand,
  t,
}: {
  page: DebugRecordPage;
  records: DebugRecord[];
  locale: string;
  query: string;
  onQueryChange: (value: string) => void;
  onSendDetectionCommand?: (command: string) => Promise<string>;
  t: TFunction;
}) {
  const config = MESSAGE_PAGE_CONFIG[page];
  const [command, setCommand] = useState("");
  const [commandBusy, setCommandBusy] = useState(false);
  const [commandBanner, setCommandBanner] = useState<Banner>({ kind: "idle", message: "" });
  const canSendCommand = page === "detection-records" && Boolean(onSendDetectionCommand);
  const filteredRecords = useMemo(() => {
    const needle = query.trim().toLowerCase();
    if (!needle) {
      return records;
    }
    return records.filter((record) => buildSearchText(record).includes(needle));
  }, [query, records]);

  const submitCommand = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (!onSendDetectionCommand) {
      return;
    }
    const nextCommand = command.trim();
    if (!nextCommand) {
      setCommandBanner({ kind: "error", message: t("manualCommand.empty", { ns: "detection" }) });
      return;
    }

    setCommandBusy(true);
    setCommandBanner({ kind: "loading", message: t("loading", { ns: "common" }) });
    try {
      const message = await onSendDetectionCommand(nextCommand);
      setCommandBanner({ kind: "success", message: message || t("manualCommand.sent", { ns: "detection" }) });
    } catch (error) {
      setCommandBanner({ kind: "error", message: extractErrorMessage(error, t("unexpectedError", { ns: "common" })) });
    } finally {
      setCommandBusy(false);
    }
  };

  return (
    <section className="flex min-h-0 min-w-0 flex-1">
      <Panel className="flex min-h-0 min-w-0 flex-1 flex-col">
        <PanelBody className="min-h-0 min-w-0 flex-1">
          <div className="grid gap-3 xl:grid-cols-[minmax(14rem,22rem)_minmax(0,1fr)]">
            <label className="grid gap-1.5">
              <span className="text-xs font-medium text-base-content/60">{t("search", { ns: "common" })}</span>
              <input
                className="input input-sm input-bordered w-full bg-base-100"
                value={query}
                onChange={(event) => onQueryChange(event.target.value)}
                placeholder={t("search", { ns: "common" })}
              />
            </label>

            {canSendCommand ? (
              <form className="grid min-w-0 gap-1.5" onSubmit={(event) => void submitCommand(event)}>
                <span className="text-xs font-medium text-base-content/60">{t("manualCommand.title", { ns: "detection" })}</span>
                <div className="flex min-w-0 gap-2">
                  <input
                    className="input input-sm input-bordered min-w-0 flex-1 bg-base-100 font-mono"
                    value={command}
                    onChange={(event) => {
                      setCommand(event.target.value);
                      setCommandBanner({ kind: "idle", message: "" });
                    }}
                    placeholder={t("manualCommand.placeholder", { ns: "detection" })}
                    disabled={commandBusy}
                  />
                  <button
                    className={cx("btn btn-primary btn-sm shrink-0", commandBusy && "app-busy-button")}
                    type="submit"
                    disabled={commandBusy || command.trim() === ""}
                  >
                    <SendHorizontal size={14} aria-hidden="true" />
                    <span>{t("manualCommand.send", { ns: "detection" })}</span>
                  </button>
                </div>
              </form>
            ) : null}
          </div>

          {canSendCommand ? <BannerAlert banner={commandBanner} /> : null}

          <VirtualMessageTable config={config} records={filteredRecords} locale={locale} resetKey={`${page}:${query}`} t={t} />
        </PanelBody>
      </Panel>
    </section>
  );
}
