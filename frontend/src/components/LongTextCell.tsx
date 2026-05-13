import type { ReactNode } from "react";

import type { DetailContent } from "../app/types";
import { cx } from "../utils/classnames";

export function CellValue({
  children,
  detail,
  onOpenDetail,
}: {
  children: ReactNode;
  detail?: DetailContent;
  onOpenDetail?: (detail: DetailContent) => void;
}) {
  if (typeof children !== "string") {
    return <div className="max-w-full truncate whitespace-nowrap">{children}</div>;
  }

  return <LongTextCell value={children} detail={detail} onOpenDetail={onOpenDetail} />;
}

export function LongTextCell({
  value,
  detail,
  onOpenDetail,
}: {
  value: string;
  detail?: DetailContent;
  onOpenDetail?: (detail: DetailContent) => void;
}) {
  const canOpen = Boolean(detail && value !== "-");
  const content = (
    <code
      className={cx(
        "block max-w-full truncate whitespace-nowrap rounded-xl bg-base-200/80 px-2 py-1 text-xs leading-5 text-base-content/75",
        canOpen ? "cursor-pointer hover:bg-base-300/80 hover:text-base-content" : "",
      )}
      title={value === "-" ? undefined : value}
    >
      {value}
    </code>
  );

  if (!canOpen || !detail || !onOpenDetail) {
    return content;
  }

  return (
    <button
      className="block max-w-full text-left"
      type="button"
      onClick={() => onOpenDetail(detail)}
      aria-label={detail.title}
    >
      {content}
    </button>
  );
}
