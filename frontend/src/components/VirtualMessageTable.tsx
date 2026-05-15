import { useCallback, useEffect, useRef, useState } from "react";
import type { UIEvent } from "react";
import type { TFunction } from "i18next";

import type { DetailContent, MessagePageConfig } from "../app/types";
import type { ParsedMessage } from "../types";
import { cx } from "../utils/classnames";
import { formatTime } from "../utils/format";
import { DetailDialog } from "./DetailDialog";
import { CellValue, LongTextCell } from "./LongTextCell";

const VIRTUAL_TABLE_ROW_HEIGHT = 42;
const VIRTUAL_TABLE_OVERSCAN = 8;
const TIME_COLUMN_WIDTH = "w-[13rem]";
const DETAIL_COLUMN_WIDTH = "w-[24rem]";

export function VirtualMessageTable({
  config,
  records,
  locale,
  resetKey,
  t,
}: {
  config: MessagePageConfig;
  records: ParsedMessage[];
  locale: string;
  resetKey: string;
  t: TFunction;
}) {
  const [scrollTop, setScrollTop] = useState(0);
  const [viewportHeight, setViewportHeight] = useState(420);
  const [detail, setDetail] = useState<DetailContent | null>(null);
  const containerRef = useRef<HTMLDivElement | null>(null);
  const totalHeight = records.length * VIRTUAL_TABLE_ROW_HEIGHT;
  const visibleCount = Math.max(1, Math.ceil(viewportHeight / VIRTUAL_TABLE_ROW_HEIGHT));
  const startIndex = Math.max(0, Math.floor(scrollTop / VIRTUAL_TABLE_ROW_HEIGHT) - VIRTUAL_TABLE_OVERSCAN);
  const endIndex = Math.min(records.length, startIndex + visibleCount + VIRTUAL_TABLE_OVERSCAN * 2);
  const visibleRecords = records.slice(startIndex, endIndex);
  const topPadding = startIndex * VIRTUAL_TABLE_ROW_HEIGHT;
  const bottomPadding = Math.max(0, totalHeight - topPadding - visibleRecords.length * VIRTUAL_TABLE_ROW_HEIGHT);
  const colSpan = config.columns.length + 2;

  const measureViewport = useCallback(() => {
    const nextHeight = containerRef.current?.clientHeight;
    if (nextHeight) {
      setViewportHeight(nextHeight);
    }
  }, []);

  useEffect(() => {
    measureViewport();
    window.addEventListener("resize", measureViewport);
    return () => window.removeEventListener("resize", measureViewport);
  }, [measureViewport]);

  useEffect(() => {
    setScrollTop(0);
    if (containerRef.current) {
      containerRef.current.scrollTop = 0;
    }
  }, [resetKey]);

  const handleScroll = useCallback((event: UIEvent<HTMLDivElement>) => {
    setScrollTop(event.currentTarget.scrollTop);
  }, []);

  return (
    <>
      <div
        ref={containerRef}
        className="min-h-0 min-w-0 flex-1 overflow-auto rounded-2xl border border-base-300 bg-base-100/70"
        onScroll={handleScroll}
      >
        <table className={cx("table table-zebra table-sm w-full table-fixed whitespace-nowrap", config.tableWidth)}>
          <thead className="sticky top-0 z-10 bg-base-200">
            <tr>
              <th className={TIME_COLUMN_WIDTH}>{t("time", { ns: "common" })}</th>
              {config.columns.map((column) => (
                <th key={column.labelKey} className={column.width}>
                  {t(column.labelKey, { ns: "messages" })}
                </th>
              ))}
              <th className={DETAIL_COLUMN_WIDTH}>{t("details", { ns: "common" })}</th>
            </tr>
          </thead>
          <tbody>
            {records.length === 0 ? (
              <tr>
                <td colSpan={colSpan} className="py-8 text-center text-sm text-base-content/55">
                  {t("empty", { ns: "common" })}
                </td>
              </tr>
            ) : (
              <>
                {topPadding > 0 ? (
                  <tr aria-hidden="true">
                    <td colSpan={colSpan} style={{ height: topPadding, padding: 0 }} />
                  </tr>
                ) : null}
                {visibleRecords.map((record) => (
                  <tr
                    key={`${record.type}-${record.time}-${record.raw}`}
                    className="row-hover"
                    style={{ height: VIRTUAL_TABLE_ROW_HEIGHT }}
                  >
                    <td className={cx("align-middle tabular-nums whitespace-nowrap", TIME_COLUMN_WIDTH)}>
                      <LongTextCell value={formatTime(locale, record.time)} />
                    </td>
                    {config.columns.map((column) => {
                      const rendered = column.render(record, locale);
                      const label = t(column.labelKey, { ns: "messages" });
                      const detailValue = typeof rendered === "string" ? rendered : undefined;

                      return (
                        <td
                          key={column.labelKey}
                          className={cx(
                            "align-middle overflow-hidden whitespace-nowrap",
                            column.width,
                            column.labelKey.includes("frequency") || column.labelKey.includes("rssi") ? "tabular-nums" : "",
                          )}
                        >
                          <CellValue
                            detail={detailValue ? { title: label, value: detailValue } : undefined}
                            onOpenDetail={setDetail}
                          >
                            {rendered}
                          </CellValue>
                        </td>
                      );
                    })}
                    <td className={cx("align-middle overflow-hidden whitespace-nowrap", DETAIL_COLUMN_WIDTH)}>
                      <LongTextCell
                        value={record.raw}
                        detail={{ title: t("details", { ns: "common" }), value: JSON.stringify(record.data, null, 2) }}
                        onOpenDetail={setDetail}
                      />
                    </td>
                  </tr>
                ))}
                {bottomPadding > 0 ? (
                  <tr aria-hidden="true">
                    <td colSpan={colSpan} style={{ height: bottomPadding, padding: 0 }} />
                  </tr>
                ) : null}
              </>
            )}
          </tbody>
        </table>
      </div>

      {detail ? <DetailDialog detail={detail} onClose={() => setDetail(null)} /> : null}
    </>
  );
}
