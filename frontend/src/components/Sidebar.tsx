import type { TFunction } from "i18next";
import {
  ChevronDown,
  Languages,
  Settings2,
  Shield,
  Wrench,
} from "lucide-react";

import { debugPageItems } from "../app/navigation";
import type { Page } from "../app/types";
import { cx } from "../utils/classnames";
import { detectLocaleName } from "../utils/format";

export function Sidebar({
  appTitle,
  page,
  locale,
  localeOptions,
  t,
  onLocaleChange,
  onNavigate,
}: {
  appTitle: string;
  page: Page;
  locale: string;
  localeOptions: string[];
  t: TFunction;
  onLocaleChange: (locale: string) => void;
  onNavigate: (page: Page) => void;
}) {
  const debugNavActive = debugPageItems.some((item) => item.id === page);

  return (
    <aside className="min-h-0 overflow-hidden border-b border-base-300 bg-base-200/95 xl:rounded-[28px] xl:border xl:border-base-300/80 xl:bg-base-200/85 xl:shadow-2xl xl:shadow-black/20">
      <div className="flex h-full min-h-0 flex-col gap-4 p-4">
        <div className="flex min-w-0 items-center gap-3">
          <div className="grid h-11 w-11 shrink-0 place-items-center rounded-3xl border border-primary/30 bg-primary/10 text-primary">
            <Shield size={20} />
          </div>
          <div className="min-w-0 self-center">
            <strong className="block truncate text-sm font-semibold">{appTitle}</strong>
          </div>
        </div>

        <nav className="flex min-h-0 gap-2 overflow-x-auto pb-1 xl:flex-col xl:overflow-y-auto xl:overflow-x-hidden" aria-label={appTitle}>
          <details
            className="group min-w-max rounded-3xl border border-base-300/80 bg-base-100/40 p-1 xl:min-w-0"
            open={debugNavActive}
          >
            <summary
              className={cx(
                "flex h-10 cursor-pointer list-none items-center gap-2 rounded-3xl px-3 text-sm font-medium transition-colors",
                debugNavActive
                  ? "bg-primary/10 text-primary shadow-[inset_0_0_0_1px_color-mix(in_oklab,var(--color-primary)_34%,transparent)]"
                  : "text-base-content/72 hover:bg-base-300/70 hover:text-base-content",
              )}
            >
              <Wrench size={17} />
              <span className="min-w-0 flex-1 truncate">{t("debugGroup", { ns: "nav" })}</span>
              <ChevronDown size={15} className="shrink-0 transition-transform group-open:rotate-180" />
            </summary>

            <div className="mt-1 flex gap-2 xl:flex-col xl:gap-1">
              {debugPageItems.map((item) => {
                const Icon = item.icon;
                const active = page === item.id;
                return (
                  <a
                    key={item.id}
                    href={`#/${item.id}`}
                    aria-current={active ? "page" : undefined}
                    className={cx(
                      "flex h-10 min-w-max items-center gap-2 rounded-3xl px-3 text-sm transition-colors xl:min-w-0",
                      active
                        ? "bg-primary/10 text-primary shadow-[inset_0_0_0_1px_color-mix(in_oklab,var(--color-primary)_34%,transparent)]"
                        : "text-base-content/64 hover:bg-base-300/70 hover:text-base-content",
                    )}
                    onClick={() => onNavigate(item.id)}
                  >
                    <Icon size={16} />
                    <span className="truncate">{t(item.labelKey, { ns: "nav" })}</span>
                  </a>
                );
              })}
            </div>
          </details>

          <a
            href="#/settings"
            aria-current={page === "settings" ? "page" : undefined}
            className={cx(
              "flex h-11 min-w-max items-center gap-2 rounded-3xl border border-base-300/80 px-3 text-sm font-medium transition-colors xl:w-full",
              page === "settings"
                ? "bg-primary text-primary-content"
                : "bg-base-100/35 text-base-content/72 hover:bg-base-300/70 hover:text-base-content",
            )}
            onClick={() => onNavigate("settings")}
          >
            <Settings2 size={17} />
            <span>{t("settings", { ns: "nav" })}</span>
          </a>
        </nav>

        <div className="mt-auto grid gap-3">
          <div className="rounded-3xl border border-base-300 bg-base-100/65 p-3">
            <label className="grid gap-2">
              <span className="flex items-center gap-2 text-xs font-medium text-base-content/60">
                <Languages size={15} />
                <span>{t("language", { ns: "settings" })}</span>
              </span>
              <select
                className="select select-sm select-primary w-full bg-base-100"
                value={locale}
                onChange={(event) => onLocaleChange(event.target.value)}
              >
                {localeOptions.map((option) => (
                  <option key={option} value={option}>
                    {detectLocaleName(option)}
                  </option>
                ))}
              </select>
            </label>
          </div>
        </div>
      </div>
    </aside>
  );
}
