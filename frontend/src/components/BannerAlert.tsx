import { CircleAlert } from "lucide-react";

import type { Banner } from "../app/types";

export function BannerAlert({ banner }: { banner: Banner }) {
  if (!banner.message || banner.kind !== "error") {
    return null;
  }

  return (
    <div
      className="alert alert-soft alert-error py-3 text-sm"
      role="alert"
      aria-live="assertive"
    >
      <CircleAlert size={16} />
      <span className="min-w-0 [overflow-wrap:anywhere]">{banner.message}</span>
    </div>
  );
}
