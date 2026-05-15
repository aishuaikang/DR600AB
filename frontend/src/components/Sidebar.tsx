import { useState } from "react";
import type { FormEvent } from "react";
import type { TFunction } from "i18next";
import {
  Check,
  ChevronDown,
  Globe2,
  KeyRound,
  LogOut,
  Monitor,
  Settings2,
  ShieldCheck,
  Wrench,
  X,
} from "lucide-react";

import { debugPageItems } from "../app/navigation";
import type { Page } from "../app/types";
import { cx } from "../utils/classnames";
import { compactLocaleName, fullLocaleName } from "../utils/locales";

function extractMessage(error: unknown) {
  return error instanceof Error ? error.message : String(error);
}

function formatDeveloperExpiry(expiresAt: number) {
  const remainSeconds = Math.max(0, Math.ceil((expiresAt - Date.now()) / 1000));
  const minutes = Math.floor(remainSeconds / 60);
  const seconds = remainSeconds % 60;
  return `${String(minutes).padStart(2, "0")}:${String(seconds).padStart(2, "0")}`;
}

export function Sidebar({
  appTitle,
  page,
  locale,
  localeOptions,
  developerActive,
  developerExpiresAt,
  t,
  onLocaleChange,
  onNavigate,
  onDeveloperLogin,
  onDeveloperLogout,
}: {
  appTitle: string;
  page: Page;
  locale: string;
  localeOptions: string[];
  developerActive: boolean;
  developerExpiresAt: number;
  t: TFunction;
  onLocaleChange: (locale: string) => void;
  onNavigate: (page: Page) => void;
  onDeveloperLogin: (code: string) => Promise<void>;
  onDeveloperLogout: () => void;
}) {
  const [languageOpen, setLanguageOpen] = useState(false);
  const [developerOpen, setDeveloperOpen] = useState(false);
  const [developerCode, setDeveloperCode] = useState("");
  const [developerBusy, setDeveloperBusy] = useState(false);
  const [developerError, setDeveloperError] = useState("");
  const debugNavActive = debugPageItems.some((item) => item.id === page);
  const developerExpiryLabel = formatDeveloperExpiry(developerExpiresAt);

  const openDeveloperLogin = () => {
    setDeveloperOpen(true);
    setDeveloperError("");
    setDeveloperCode("");
  };

  const handleDeveloperSubmit = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    const code = developerCode.trim();
    if (!code) {
      setDeveloperError(t("developerCodeRequired", { ns: "nav" }));
      return;
    }

    setDeveloperBusy(true);
    setDeveloperError("");
    try {
      await onDeveloperLogin(code);
      setDeveloperOpen(false);
      setDeveloperCode("");
    } catch (error) {
      setDeveloperError(extractMessage(error));
    } finally {
      setDeveloperBusy(false);
    }
  };

  return (
    <aside className="min-h-0 overflow-hidden border-b border-base-300 bg-base-200/95 xl:rounded-2xl xl:border xl:border-base-300/80 xl:bg-base-200/85 xl:shadow-xl xl:shadow-black/20">
      <div className="flex h-full min-h-0 flex-col gap-3 p-3">
        <div className="flex min-w-0 items-center gap-2">
          <a
            href="#/screen"
            aria-label={t("screen", { ns: "nav" })}
            title={t("screen", { ns: "nav" })}
            className="grid h-9 w-9 shrink-0 place-items-center rounded-2xl border border-primary/30 bg-primary/10 text-primary hover:border-primary/45 hover:bg-primary/15"
            onClick={() => onNavigate("screen")}
          >
            <Monitor size={18} />
          </a>
          <div className="min-w-0 flex-1">
            <strong className="block truncate text-[13px] font-semibold leading-5">{appTitle}</strong>
            {developerActive ? (
              <span className="mt-0.5 inline-flex items-center gap-1 rounded-full border border-success/25 bg-success/10 px-2 py-0.5 text-[10px] font-semibold text-success">
                <ShieldCheck size={11} />
                {t("developerActive", { ns: "nav" })} {developerExpiryLabel}
              </span>
            ) : null}
          </div>
        </div>

        <nav className="flex min-h-0 gap-2 overflow-x-auto pb-1 xl:flex-col xl:overflow-y-auto xl:overflow-x-hidden" aria-label={appTitle}>
          <a
            href="#/settings"
            aria-current={page === "settings" ? "page" : undefined}
            className={cx(
              "flex h-9 min-w-max items-center gap-2 rounded-2xl border border-base-300/80 px-2.5 text-[13px] font-medium xl:w-full",
              page === "settings"
                ? "bg-primary text-primary-content"
                : "bg-base-100/35 text-base-content/72 hover:bg-base-300/70 hover:text-base-content",
            )}
            onClick={() => onNavigate("settings")}
          >
            <Settings2 size={16} />
            <span>{t("settings", { ns: "nav" })}</span>
          </a>

          {developerActive ? (
            <details
              className="group min-w-max rounded-2xl border border-base-300/80 bg-base-100/35 p-1 xl:min-w-0"
              open={debugNavActive}
            >
              <summary
                className={cx(
                  "flex h-9 cursor-pointer list-none items-center gap-2 rounded-xl px-2.5 text-[13px] font-medium",
                  debugNavActive
                    ? "bg-primary/10 text-primary shadow-[inset_0_0_0_1px_color-mix(in_oklab,var(--color-primary)_34%,transparent)]"
                    : "text-base-content/72 hover:bg-base-300/70 hover:text-base-content",
                )}
              >
                <Wrench size={16} />
                <span className="min-w-0 flex-1 truncate">{t("debugGroup", { ns: "nav" })}</span>
                <ChevronDown size={14} className="shrink-0 group-open:rotate-180" />
              </summary>

              <div className="mt-1 flex gap-1.5 xl:flex-col xl:gap-1">
                {debugPageItems.map((item) => {
                  const Icon = item.icon;
                  const active = page === item.id;
                  return (
                    <a
                      key={item.id}
                      href={`#/${item.id}`}
                      aria-current={active ? "page" : undefined}
                      className={cx(
                        "flex h-8 min-w-max items-center gap-2 rounded-xl px-2.5 text-[13px] xl:min-w-0",
                        active
                          ? "bg-primary/10 text-primary shadow-[inset_0_0_0_1px_color-mix(in_oklab,var(--color-primary)_34%,transparent)]"
                          : "text-base-content/64 hover:bg-base-300/70 hover:text-base-content",
                      )}
                      onClick={() => onNavigate(item.id)}
                    >
                      <Icon size={15} />
                      <span className="truncate">{t(item.labelKey, { ns: "nav" })}</span>
                    </a>
                  );
                })}
              </div>
            </details>
          ) : null}
        </nav>

        <div className="mt-auto grid gap-2">
          <div className="flex items-center gap-2 rounded-2xl border border-base-300 bg-base-100/55 p-1.5">
            <div
              className="relative min-w-0 flex-1"
              onBlur={(event) => {
                const nextFocus = event.relatedTarget;
                if (!(nextFocus instanceof Node) || !event.currentTarget.contains(nextFocus)) {
                  setLanguageOpen(false);
                }
              }}
              onKeyDown={(event) => {
                if (event.key === "Escape") {
                  setLanguageOpen(false);
                }
              }}
            >
              <button
                className="flex h-8 w-full items-center justify-center gap-2 rounded-xl border border-primary/20 bg-primary/10 px-2 text-[12px] font-semibold text-primary hover:border-primary/40 hover:bg-primary/15"
                type="button"
                aria-label={t("language", { ns: "settings" })}
                aria-haspopup="listbox"
                aria-expanded={languageOpen}
                onClick={() => setLanguageOpen((value) => !value)}
              >
                <Globe2 size={15} />
                <span className="truncate">{compactLocaleName(locale)}</span>
                <ChevronDown size={13} className={cx("shrink-0", languageOpen && "rotate-180")} />
              </button>

              {languageOpen ? (
                <div
                  className="absolute bottom-[calc(100%+0.4rem)] left-0 z-40 grid w-full min-w-[9rem] gap-1 rounded-2xl border border-primary/25 bg-base-100/95 p-1.5 shadow-2xl shadow-black/35"
                  role="listbox"
                  aria-label={t("language", { ns: "settings" })}
                >
                  {localeOptions.map((option) => (
                    <button
                      key={option}
                      className={cx(
                        "flex h-8 items-center justify-between gap-2 rounded-xl px-2.5 text-left text-[12px] font-semibold",
                        option === locale
                          ? "bg-primary/15 text-primary"
                          : "text-base-content/72 hover:bg-base-300/70 hover:text-base-content",
                      )}
                      type="button"
                      role="option"
                      aria-selected={option === locale}
                      onClick={() => {
                        onLocaleChange(option);
                        setLanguageOpen(false);
                      }}
                    >
                      <span>{fullLocaleName(option)}</span>
                      {option === locale ? <Check size={14} /> : null}
                    </button>
                  ))}
                </div>
              ) : null}
            </div>

            {developerActive ? (
              <button
                className="btn btn-ghost btn-xs h-8 min-h-8 w-8 rounded-xl px-0 text-success"
                type="button"
                aria-label={t("developerLogout", { ns: "nav" })}
                title={t("developerLogout", { ns: "nav" })}
                onClick={onDeveloperLogout}
              >
                <LogOut size={15} />
              </button>
            ) : (
              <button
                className="btn btn-ghost btn-xs h-8 min-h-8 w-8 rounded-xl px-0 text-base-content/55"
                type="button"
                aria-label={t("developerLogin", { ns: "nav" })}
                title={t("developerLogin", { ns: "nav" })}
                onClick={openDeveloperLogin}
              >
                <KeyRound size={15} />
              </button>
            )}
          </div>
        </div>
      </div>

      {developerOpen ? (
        <div className="fixed inset-0 z-50 grid place-items-center bg-black/55 p-4" role="dialog" aria-modal="true">
          <form className="grid w-full max-w-sm gap-3 rounded-2xl border border-base-300 bg-base-100 p-4 shadow-2xl shadow-black/40" onSubmit={handleDeveloperSubmit}>
            <div className="flex items-start justify-between gap-3">
              <div className="min-w-0">
                <h2 className="text-sm font-semibold text-base-content">{t("developerLogin", { ns: "nav" })}</h2>
              </div>
              <button
                className="btn btn-ghost btn-xs h-7 min-h-7 w-7 rounded-xl px-0"
                type="button"
                aria-label={t("close", { ns: "common" })}
                onClick={() => setDeveloperOpen(false)}
              >
                <X size={15} />
              </button>
            </div>

            <label className="grid gap-1.5">
              <span className="text-xs font-medium text-base-content/60">{t("developerCode", { ns: "nav" })}</span>
              <input
                className="input input-sm input-bordered w-full bg-base-200 text-center font-mono text-lg tracking-[0.28em]"
                value={developerCode}
                inputMode="numeric"
                autoComplete="one-time-code"
                maxLength={6}
                onChange={(event) => setDeveloperCode(event.target.value.replace(/\D/g, ""))}
                placeholder="000000"
              />
            </label>

            {developerError ? <p className="rounded-xl bg-error/10 px-3 py-2 text-xs text-error">{developerError}</p> : null}

            <div className="flex items-center justify-end gap-2">
              <button className="btn btn-primary btn-sm" type="submit" disabled={developerBusy || developerCode.trim().length !== 6}>
                {developerBusy ? t("loading", { ns: "common" }) : t("developerLogin", { ns: "nav" })}
              </button>
            </div>
          </form>
        </div>
      ) : null}
    </aside>
  );
}
