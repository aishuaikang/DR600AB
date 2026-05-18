import { Loader2 } from "lucide-react";

import { cx } from "../utils/classnames";

export function LoadingSpinner({ className, size = 16 }: { className?: string; size?: number }) {
  return <Loader2 aria-hidden="true" size={size} className={cx("app-spinner", className)} />;
}

export function PageLoading({ label }: { label: string }) {
  return (
    <section className="app-page-loading" role="status" aria-live="polite">
      <div className="app-page-loading__card">
        <LoadingSpinner size={28} />
        <strong>{label}</strong>
        <div className="app-loading-bars" aria-hidden="true">
          <span />
          <span />
          <span />
        </div>
      </div>
    </section>
  );
}

export function LoadingOverlay({ active, label }: { active: boolean; label: string }) {
  if (!active) {
    return null;
  }

  return (
    <div className="app-loading-overlay" role="status" aria-live="polite">
      <LoadingSpinner size={22} />
      <span>{label}</span>
    </div>
  );
}

export function SkeletonBlock({ className }: { className?: string }) {
  return <span className={cx("app-skeleton", className)} aria-hidden="true" />;
}
