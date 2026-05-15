import type { ReactNode } from "react";

import { cx } from "../utils/classnames";

export function Panel({ children, className }: { children: ReactNode; className?: string }) {
  return (
    <section className={cx("rounded-2xl border border-base-300 bg-base-200/80 shadow-sm shadow-black/20", className)}>
      {children}
    </section>
  );
}

export function PanelBody({ children, className }: { children: ReactNode; className?: string }) {
  return <div className={cx("flex flex-col gap-3 p-3 sm:p-4", className)}>{children}</div>;
}
