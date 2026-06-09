import type { DetailContent } from "../app/types";
import { X } from "lucide-react";

export function DetailDialog({ detail, onClose }: { detail: DetailContent; onClose: () => void }) {
  return (
    <div className="fixed inset-0 z-50 grid place-items-center bg-black/55 p-4" role="dialog" aria-modal="true">
      <div className="flex max-h-[80dvh] w-full max-w-3xl flex-col overflow-hidden rounded-[28px] border border-base-300 bg-base-100 shadow-2xl shadow-black/40">
        <div className="flex shrink-0 items-center justify-between gap-3 border-b border-base-300 px-5 py-4">
          <h2 className="min-w-0 truncate text-base font-semibold text-base-content">{detail.title}</h2>
          <button className="btn btn-square btn-sm btn-outline shrink-0" type="button" aria-label="关闭" onClick={onClose}>
            <X size={18} aria-hidden="true" />
          </button>
        </div>
        <pre className="min-h-0 flex-1 overflow-auto whitespace-pre-wrap break-words p-5 text-sm leading-6 text-base-content/80">
          {detail.value}
        </pre>
      </div>
    </div>
  );
}
