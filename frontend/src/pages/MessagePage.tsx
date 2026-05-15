import { useMemo } from "react";
import type { TFunction } from "i18next";

import { MESSAGE_PAGE_CONFIG } from "../app/message-pages";
import { Panel, PanelBody } from "../components/Panel";
import { VirtualMessageTable } from "../components/VirtualMessageTable";
import type { ParsedMessage, ParsedMessageType } from "../types";
import { buildSearchText } from "../utils/records";

export function MessagePage({
  page,
  records,
  locale,
  query,
  onQueryChange,
  t,
}: {
  page: ParsedMessageType;
  records: ParsedMessage[];
  locale: string;
  query: string;
  onQueryChange: (value: string) => void;
  t: TFunction;
}) {
  const config = MESSAGE_PAGE_CONFIG[page];
  const filteredRecords = useMemo(() => {
    const needle = query.trim().toLowerCase();
    if (!needle) {
      return records;
    }
    return records.filter((record) => buildSearchText(record).includes(needle));
  }, [query, records]);

  return (
    <section className="flex min-h-0 min-w-0 flex-1">
      <Panel className="flex min-h-0 min-w-0 flex-1 flex-col">
        <PanelBody className="min-h-0 min-w-0 flex-1">
          <label className="grid max-w-sm gap-1.5">
            <span className="text-xs font-medium text-base-content/60">{t("search", { ns: "common" })}</span>
            <input
              className="input input-sm input-bordered w-full bg-base-100"
              value={query}
              onChange={(event) => onQueryChange(event.target.value)}
              placeholder={t("search", { ns: "common" })}
            />
          </label>

          <VirtualMessageTable config={config} records={filteredRecords} locale={locale} resetKey={`${page}:${query}`} t={t} />
        </PanelBody>
      </Panel>
    </section>
  );
}
