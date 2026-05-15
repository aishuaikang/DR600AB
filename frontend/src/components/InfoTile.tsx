import type { ReactNode } from "react";

export function InfoTile({
  label,
  value,
  children,
}: {
  label: string;
  value?: ReactNode;
  children?: ReactNode;
}) {
  return (
    <div className="min-w-0 rounded-2xl border border-base-300 bg-base-100/70 px-3 py-2.5">
      <span className="block text-xs font-medium text-base-content/55">{label}</span>
      <div className="mt-1.5 min-w-0 break-words text-sm font-semibold text-base-content">{children ?? value}</div>
    </div>
  );
}
