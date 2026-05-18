import { CheckCircle2, CircleAlert, Loader2 } from "lucide-react";

import type { Banner } from "../app/types";
import { cx } from "../utils/classnames";

export function BannerAlert({ banner }: { banner: Banner }) {
  if (!banner.message || banner.kind === "idle") {
    return null;
  }
  const toneClass = {
    loading: "alert-info",
    success: "alert-success",
    error: "alert-error",
  }[banner.kind];
  const Icon = banner.kind === "success" ? CheckCircle2 : banner.kind === "loading" ? Loader2 : CircleAlert;

  return (
    <div
      className={cx("app-banner alert alert-soft py-3 text-sm", toneClass)}
      role={banner.kind === "error" ? "alert" : "status"}
      aria-live={banner.kind === "error" ? "assertive" : "polite"}
    >
      <Icon size={16} className={banner.kind === "loading" ? "app-spinner" : undefined} />
      <span className="min-w-0 [overflow-wrap:anywhere]">{banner.message}</span>
    </div>
  );
}
