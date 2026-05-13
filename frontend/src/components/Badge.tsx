import type { ReactNode } from "react";

import type { Tone } from "../app/types";
import { cx } from "../utils/classnames";

export function Badge({
  children,
  tone = "neutral",
  outline = false,
}: {
  children: ReactNode;
  tone?: Tone;
  outline?: boolean;
}) {
  const toneClass: Record<Tone, string> = {
    neutral: "badge-ghost",
    success: "badge-success",
    warning: "badge-warning",
    error: "badge-error",
    info: "badge-info",
  };
  const variantClass = outline ? "badge-outline" : tone === "neutral" ? "badge-ghost" : "badge-soft";

  return <span className={cx("badge badge-sm max-w-full whitespace-nowrap", toneClass[tone], variantClass)}>{children}</span>;
}
