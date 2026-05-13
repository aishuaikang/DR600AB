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
    <div className="min-w-0 rounded-3xl border border-base-300 bg-base-100/70 px-4 py-3">
      <span className="block text-xs font-medium text-base-content/55">{label}</span>
      <div className="mt-2 min-w-0 break-words text-sm font-semibold text-base-content">{children ?? value}</div>
    </div>
  );
}
