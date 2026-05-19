import { useCallback, useEffect, useLayoutEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";

import { cx } from "../utils/classnames";
import { getPinyinCandidates, loadPinyinDictionary } from "../utils/pinyinCandidates";

type EditableElement = HTMLInputElement | HTMLTextAreaElement;
type KeyboardMode = "text" | "numeric" | "digits";
type LayoutName = "default" | "shift" | "symbols";
type LanguageMode = "en" | "zh";
type SupportedKeyboardLanguage = "en-US" | "zh-CN";
type TextLanguagePolicy = "auto" | "ascii";

const supportedInputTypes = new Set([
  "",
  "email",
  "number",
  "password",
  "search",
  "tel",
  "text",
  "url",
]);

const textLayouts: Record<LayoutName, string[][]> = {
  default: [
    ["1", "2", "3", "4", "5", "6", "7", "8", "9", "0", "{bksp}"],
    ["q", "w", "e", "r", "t", "y", "u", "i", "o", "p"],
    ["a", "s", "d", "f", "g", "h", "j", "k", "l"],
    ["{shift}", "z", "x", "c", "v", "b", "n", "m", "{enter}"],
    ["{lang}", "{symbols}", ",", "{space}", ".", "{hide}"],
  ],
  shift: [
    ["!", "@", "#", "$", "%", "^", "&", "*", "(", ")", "{bksp}"],
    ["Q", "W", "E", "R", "T", "Y", "U", "I", "O", "P"],
    ["A", "S", "D", "F", "G", "H", "J", "K", "L"],
    ["{shift}", "Z", "X", "C", "V", "B", "N", "M", "{enter}"],
    ["{lang}", "{symbols}", "-", "{space}", "_", "{hide}"],
  ],
  symbols: [
    ["`", "~", "[", "]", "{", "}", "\\", "|", "{bksp}"],
    ["+", "-", "=", "_", "/", "?", ":", ";"],
    ["'", "\"", "<", ">", ",", ".", "@", "#"],
    ["{abc}", "(", ")", "*", "&", "%", "$", "{enter}"],
    ["{lang}", "{abc}", "{space}", "{hide}"],
  ],
};

const numericLayout = [
  ["1", "2", "3"],
  ["4", "5", "6"],
  ["7", "8", "9"],
  ["-", "0", "."],
  ["{clear}", "{bksp}", "{hide}"],
];

const digitLayout = [
  ["1", "2", "3"],
  ["4", "5", "6"],
  ["7", "8", "9"],
  ["{clear}", "0", "{bksp}"],
  ["{hide}"],
];

const staticButtonLabels: Record<string, string> = {
  "{abc}": "ABC",
  "{bksp}": "⌫",
  "{lang}": "中",
  "{shift}": "⇧",
  "{symbols}": "#+=",
};

const emptyCandidates: string[] = [];
const keyboardLanguageByLocale: Record<SupportedKeyboardLanguage, LanguageMode> = {
  "en-US": "en",
  "zh-CN": "zh",
};
const keyboardViewportGap = 16;

function normalizeKeyboardLocale(locale: string): SupportedKeyboardLanguage {
  return locale === "zh-CN" ? "zh-CN" : "en-US";
}

function resolveLanguageMode(locale: string, localeOptions: string[]) {
  const normalizedLocale = normalizeKeyboardLocale(locale);
  if (normalizedLocale === "zh-CN" && localeOptions.includes("zh-CN")) {
    return "zh";
  }
  return "en";
}

function isEditableElement(element: EventTarget | null): element is EditableElement {
  if (!(element instanceof HTMLInputElement) && !(element instanceof HTMLTextAreaElement)) {
    return false;
  }
  if (element.readOnly || element.disabled) {
    return false;
  }
  if (element instanceof HTMLInputElement && !supportedInputTypes.has(element.type)) {
    return false;
  }
  return element.dataset.virtualKeyboard !== "off";
}

function scrollableAncestor(element: Element | null) {
  if (!(element instanceof HTMLElement)) {
    return null;
  }
  let current = element.parentElement;
  while (current && current !== document.body) {
    const style = window.getComputedStyle(current);
    if (
      ["auto", "scroll", "overlay"].includes(style.overflowY) &&
      current.scrollHeight > current.clientHeight
    ) {
      return current;
    }
    current = current.parentElement;
  }
  return document.scrollingElement instanceof HTMLElement ? document.scrollingElement : null;
}

function keepElementAboveKeyboard(element: EditableElement, keyboardHeight: number) {
  const container = scrollableAncestor(element);
  if (!container) {
    element.scrollIntoView({ block: "center", inline: "nearest" });
    return;
  }

  const elementRect = element.getBoundingClientRect();
  const containerRect = container === document.documentElement
    ? { top: 0, bottom: window.innerHeight }
    : container.getBoundingClientRect();
  const keyboardTop = window.innerHeight - keyboardHeight;
  const visibleTop = Math.max(containerRect.top, 0) + keyboardViewportGap;
  const visibleBottom = Math.min(containerRect.bottom, keyboardTop) - keyboardViewportGap;

  if (elementRect.bottom > visibleBottom) {
    container.scrollBy({ top: elementRect.bottom - visibleBottom, behavior: "auto" });
    return;
  }
  if (elementRect.top < visibleTop) {
    container.scrollBy({ top: elementRect.top - visibleTop, behavior: "auto" });
  }
}

function getKeyboardMode(element: EditableElement): KeyboardMode {
  if (element.dataset.keyboard === "digits") {
    return "digits";
  }
  if (element.dataset.keyboard === "numeric") {
    return "numeric";
  }
  const inputMode = element.getAttribute("inputmode");
  if (inputMode === "decimal" || inputMode === "tel") {
    return "numeric";
  }
  if (inputMode === "numeric") {
    return "digits";
  }
  if (element instanceof HTMLInputElement && (element.type === "number" || element.type === "tel")) {
    return "numeric";
  }
  return "text";
}

function getTextLanguagePolicy(element: EditableElement): TextLanguagePolicy {
  const keyboard = element.dataset.keyboard?.toLowerCase();
  if (keyboard === "ascii" || keyboard === "english" || keyboard === "en") {
    return "ascii";
  }
  if (element instanceof HTMLInputElement && ["email", "password", "url"].includes(element.type)) {
    return "ascii";
  }
  const inputMode = element.getAttribute("inputmode");
  if (inputMode === "email" || inputMode === "url") {
    return "ascii";
  }
  return "auto";
}

function getElementValue(element: EditableElement) {
  return element.value ?? "";
}

function clampRange(element: EditableElement) {
  const value = getElementValue(element);
  const start = element.selectionStart ?? value.length;
  const end = element.selectionEnd ?? start;
  return {
    start: Math.max(0, Math.min(start, value.length)),
    end: Math.max(0, Math.min(end, value.length)),
  };
}

function setElementValue(element: EditableElement, value: string, caret: number) {
  const nativeSetter = Object.getOwnPropertyDescriptor(Object.getPrototypeOf(element), "value")?.set;
  nativeSetter?.call(element, value);
  element.dispatchEvent(new Event("input", { bubbles: true }));
  element.focus({ preventScroll: true });
  try {
    element.setSelectionRange(caret, caret);
  } catch {
    // Some input types do not support selection ranges.
  }
}

function insertText(element: EditableElement, text: string) {
  const value = getElementValue(element);
  const { start, end } = clampRange(element);
  const maxLength = element.maxLength > -1 ? element.maxLength : Number.POSITIVE_INFINITY;
  const available = maxLength - (value.length - (end - start));
  if (available <= 0) {
    return;
  }
  const nextText = text.slice(0, available);
  setElementValue(element, `${value.slice(0, start)}${nextText}${value.slice(end)}`, start + nextText.length);
}

function backspace(element: EditableElement) {
  const value = getElementValue(element);
  const { start, end } = clampRange(element);
  if (start === 0 && end === 0) {
    return;
  }
  if (start !== end) {
    setElementValue(element, `${value.slice(0, start)}${value.slice(end)}`, start);
    return;
  }
  setElementValue(element, `${value.slice(0, start - 1)}${value.slice(end)}`, start - 1);
}

function clearInput(element: EditableElement) {
  setElementValue(element, "", 0);
}

function isActionKey(key: string) {
  return key.startsWith("{") && key.endsWith("}");
}

function actionClass(key: string, layoutName: LayoutName, languageMode: LanguageMode) {
  if (key === "{space}") {
    return "virtual-keyboard__key--space";
  }
  if (key === "{shift}" && layoutName === "shift") {
    return "virtual-keyboard__key--action virtual-keyboard__key--active";
  }
  if (key === "{lang}" && languageMode === "zh") {
    return "virtual-keyboard__key--action virtual-keyboard__key--active";
  }
  return isActionKey(key) ? "virtual-keyboard__key--action" : undefined;
}

export function VirtualKeyboard({
  locale,
  localeOptions,
}: {
  locale: string;
  localeOptions: string[];
}) {
  const { t } = useTranslation();
  const activeElementRef = useRef<EditableElement | null>(null);
  const keyboardRef = useRef<HTMLDivElement | null>(null);
  const closeTimerRef = useRef<number | null>(null);
  const supportsChinese = localeOptions.includes("zh-CN");
  const supportsEnglish = localeOptions.includes("en-US");
  const canSwitchLanguage = supportsChinese && supportsEnglish;
  const [visible, setVisible] = useState(false);
  const [mode, setMode] = useState<KeyboardMode>("text");
  const [layoutName, setLayoutName] = useState<LayoutName>("default");
  const [textLanguagePolicy, setTextLanguagePolicy] = useState<TextLanguagePolicy>("auto");
  const [languageMode, setLanguageMode] = useState<LanguageMode>(() => resolveLanguageMode(locale, localeOptions));
  const [pinyinBuffer, setPinyinBuffer] = useState("");
  const [pinyinCandidates, setPinyinCandidates] = useState<string[]>(emptyCandidates);
  const [dictionaryLoading, setDictionaryLoading] = useState(false);

  const cancelClose = useCallback(() => {
    if (closeTimerRef.current !== null) {
      window.clearTimeout(closeTimerRef.current);
      closeTimerRef.current = null;
    }
  }, []);

  const hideKeyboard = useCallback(() => {
    cancelClose();
    setVisible(false);
    setLayoutName("default");
    setPinyinBuffer("");
    setPinyinCandidates(emptyCandidates);
    activeElementRef.current = null;
  }, [cancelClose]);

  const syncKeyboardViewport = useCallback(() => {
    if (!visible) {
      document.documentElement.style.removeProperty("--virtual-keyboard-height");
      document.body.classList.remove("virtual-keyboard-open");
      return 0;
    }
    const height = Math.ceil(keyboardRef.current?.getBoundingClientRect().height ?? 0);
    document.documentElement.style.setProperty("--virtual-keyboard-height", `${height}px`);
    document.body.classList.add("virtual-keyboard-open");
    if (height > 0 && activeElementRef.current) {
      keepElementAboveKeyboard(activeElementRef.current, height);
    }
    return height;
  }, [visible]);

  const showForElement = useCallback(
    (element: EditableElement) => {
      cancelClose();
      const nextMode = getKeyboardMode(element);
      const nextTextLanguagePolicy = getTextLanguagePolicy(element);
      activeElementRef.current = element;
      setMode(nextMode);
      setLayoutName("default");
      setTextLanguagePolicy(nextTextLanguagePolicy);
      setLanguageMode(nextMode === "text" && nextTextLanguagePolicy === "auto" ? resolveLanguageMode(locale, localeOptions) : "en");
      setPinyinBuffer("");
      setPinyinCandidates(emptyCandidates);
      setVisible(true);
      window.requestAnimationFrame(() => {
        const height = Math.ceil(keyboardRef.current?.getBoundingClientRect().height ?? 0);
        if (height > 0) {
          keepElementAboveKeyboard(element, height);
        }
      });
    },
    [cancelClose, locale, localeOptions],
  );

  useEffect(() => {
    const handleFocusIn = (event: FocusEvent) => {
      if (isEditableElement(event.target)) {
        showForElement(event.target);
      }
    };

    const handlePointerDown = (event: PointerEvent) => {
      if (isEditableElement(event.target)) {
        showForElement(event.target);
        return;
      }
      window.setTimeout(() => {
        if (isEditableElement(document.activeElement)) {
          showForElement(document.activeElement);
        }
      }, 0);
    };

    const handleFocusOut = () => {
      closeTimerRef.current = window.setTimeout(() => {
        if (!isEditableElement(document.activeElement)) {
          hideKeyboard();
        }
      }, 120);
    };

    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape" && visible) {
        hideKeyboard();
      }
    };

    document.addEventListener("focusin", handleFocusIn);
    document.addEventListener("focusout", handleFocusOut);
    document.addEventListener("pointerdown", handlePointerDown, true);
    window.addEventListener("keydown", handleKeyDown);
    window.addEventListener("hashchange", hideKeyboard);
    return () => {
      document.removeEventListener("focusin", handleFocusIn);
      document.removeEventListener("focusout", handleFocusOut);
      document.removeEventListener("pointerdown", handlePointerDown, true);
      window.removeEventListener("keydown", handleKeyDown);
      window.removeEventListener("hashchange", hideKeyboard);
      cancelClose();
    };
  }, [cancelClose, hideKeyboard, showForElement, visible]);

  useLayoutEffect(() => {
    const update = () => {
      syncKeyboardViewport();
    };
    update();

    if (!visible) {
      return update;
    }

    const frame = window.requestAnimationFrame(update);
    const resizeObserver = typeof ResizeObserver === "undefined" || !keyboardRef.current
      ? null
      : new ResizeObserver(update);
    if (keyboardRef.current) {
      resizeObserver?.observe(keyboardRef.current);
    }
    window.addEventListener("resize", update);
    window.visualViewport?.addEventListener("resize", update);
    window.visualViewport?.addEventListener("scroll", update);

    return () => {
      window.cancelAnimationFrame(frame);
      resizeObserver?.disconnect();
      window.removeEventListener("resize", update);
      window.visualViewport?.removeEventListener("resize", update);
      window.visualViewport?.removeEventListener("scroll", update);
    };
  }, [syncKeyboardViewport, visible, mode, layoutName, languageMode]);

  useEffect(() => () => {
    document.documentElement.style.removeProperty("--virtual-keyboard-height");
    document.body.classList.remove("virtual-keyboard-open");
  }, []);

  useEffect(() => {
    if (!visible || mode !== "text") {
      return;
    }
    setLanguageMode(textLanguagePolicy === "auto" ? resolveLanguageMode(locale, localeOptions) : "en");
    setPinyinBuffer("");
    setPinyinCandidates(emptyCandidates);
  }, [locale, localeOptions, mode, textLanguagePolicy, visible]);

  useEffect(() => {
    if (mode !== "text" || languageMode !== "zh") {
      setPinyinCandidates(emptyCandidates);
      setDictionaryLoading(false);
      return;
    }

    let cancelled = false;
    setDictionaryLoading(true);
    void (async () => {
      try {
        const candidates = pinyinBuffer
          ? await getPinyinCandidates(pinyinBuffer)
          : (await loadPinyinDictionary(), emptyCandidates);
        if (!cancelled) {
          setPinyinCandidates(candidates);
        }
      } finally {
        if (!cancelled) {
          setDictionaryLoading(false);
        }
      }
    })();

    return () => {
      cancelled = true;
    };
  }, [languageMode, mode, pinyinBuffer]);

  const commitCandidate = useCallback((candidate: string) => {
    const element = activeElementRef.current;
    if (!element) {
      return;
    }
    insertText(element, candidate);
    setPinyinBuffer("");
  }, []);

  const handleKeyPress = useCallback(
    (key: string) => {
      const element = activeElementRef.current;
      if (!element) {
        return;
      }

      if (key === "{hide}" || key === "{enter}") {
        if (pinyinBuffer) {
          const [candidate] = pinyinCandidates;
          insertText(element, candidate || pinyinBuffer);
          setPinyinBuffer("");
          return;
        }
        hideKeyboard();
        element.blur();
        return;
      }
      if (key === "{lang}") {
        if (!canSwitchLanguage || textLanguagePolicy !== "auto") {
          return;
        }
        setLanguageMode((current) => (current === "zh" ? "en" : "zh"));
        setPinyinCandidates(emptyCandidates);
        setPinyinBuffer("");
        element.focus({ preventScroll: true });
        return;
      }
      if (key === "{shift}") {
        setLayoutName((current) => (current === "shift" ? "default" : "shift"));
        element.focus({ preventScroll: true });
        return;
      }
      if (key === "{symbols}") {
        setLayoutName("symbols");
        element.focus({ preventScroll: true });
        return;
      }
      if (key === "{abc}") {
        setLayoutName("default");
        element.focus({ preventScroll: true });
        return;
      }
      if (key === "{bksp}") {
        if (languageMode === "zh" && pinyinBuffer) {
          setPinyinBuffer((current) => current.slice(0, -1));
          element.focus({ preventScroll: true });
          return;
        }
        backspace(element);
        return;
      }
      if (key === "{clear}") {
        setPinyinBuffer("");
        clearInput(element);
        return;
      }
      if (key === "{space}") {
        if (languageMode === "zh" && pinyinBuffer) {
          const [candidate] = pinyinCandidates;
          insertText(element, candidate || pinyinBuffer);
          setPinyinBuffer("");
          return;
        }
        insertText(element, " ");
        return;
      }
      if (isActionKey(key)) {
        return;
      }
      if (languageMode === "zh" && /^[a-z]$/i.test(key)) {
        setPinyinBuffer((current) => `${current}${key.toLowerCase()}`.slice(0, 24));
        element.focus({ preventScroll: true });
        return;
      }
      if (languageMode === "zh" && /^[1-9]$/.test(key) && pinyinCandidates[Number(key) - 1]) {
        commitCandidate(pinyinCandidates[Number(key) - 1]);
        return;
      }
      if (languageMode === "zh" && pinyinBuffer) {
        const [candidate] = pinyinCandidates;
        insertText(element, candidate || pinyinBuffer);
        setPinyinBuffer("");
      }
      insertText(element, key);
      if (layoutName === "shift") {
        setLayoutName("default");
      }
    },
    [commitCandidate, hideKeyboard, languageMode, layoutName, pinyinBuffer, pinyinCandidates, textLanguagePolicy],
  );

  if (!visible) {
    return null;
  }

  const rows = mode === "numeric"
    ? numericLayout
    : mode === "digits"
      ? digitLayout
      : textLayouts[layoutName].map((row) =>
        canSwitchLanguage && textLanguagePolicy === "auto" ? row : row.filter((key) => key !== "{lang}"),
      );
  const buttonLabels: Record<string, string> = {
    ...staticButtonLabels,
    "{clear}": t("keyboard.clear", { ns: "common" }),
    "{enter}": t("keyboard.enter", { ns: "common" }),
    "{hide}": t("keyboard.close", { ns: "common" }),
    "{space}": t("keyboard.space", { ns: "common" }),
  };

  return (
    <div
      ref={keyboardRef}
      className={cx("virtual-keyboard", mode !== "text" && "virtual-keyboard--numeric")}
      onPointerDown={(event) => event.preventDefault()}
    >
      <div className="virtual-keyboard__panel" role="group" aria-label={t("keyboard.virtualKeyboard", { ns: "common" })}>
        {mode === "text" && languageMode === "zh" ? (
          <div className="virtual-keyboard__ime">
            <div className="virtual-keyboard__composition">
              <span>{pinyinBuffer || t("keyboard.pinyinInput", { ns: "common" })}</span>
            </div>
            <div className="virtual-keyboard__candidates" aria-label={t("keyboard.chineseCandidates", { ns: "common" })}>
              {pinyinCandidates.length > 0 ? (
                pinyinCandidates.map((candidate, index) => (
                  <button
                    className="virtual-keyboard__candidate"
                    key={`${candidate}-${index}`}
                    type="button"
                    tabIndex={-1}
                    onPointerDown={(event) => {
                      event.preventDefault();
                      event.stopPropagation();
                      commitCandidate(candidate);
                    }}
                  >
                    <span>{index + 1}</span>
                    {candidate}
                  </button>
                ))
              ) : (
                <span className="virtual-keyboard__candidate-empty">
                  {dictionaryLoading
                    ? t("keyboard.dictionaryLoading", { ns: "common" })
                    : t("keyboard.pinyinHint", { ns: "common" })}
                </span>
              )}
            </div>
          </div>
        ) : null}

        {rows.map((row, rowIndex) => (
          <div className="virtual-keyboard__row" key={`${mode}-${layoutName}-${rowIndex}`}>
            {row.map((key) => (
              <button
                className={cx("virtual-keyboard__key", actionClass(key, layoutName, languageMode))}
                key={`${rowIndex}-${key}`}
                type="button"
                tabIndex={-1}
                onPointerDown={(event) => {
                  event.preventDefault();
                  event.stopPropagation();
                  handleKeyPress(key);
                }}
              >
                {key === "{lang}"
                  ? keyboardLanguageByLocale[normalizeKeyboardLocale(locale)] === "zh" && languageMode === "en"
                    ? "中"
                    : languageMode === "zh"
                      ? "EN"
                      : buttonLabels[key]
                  : buttonLabels[key] ?? key}
              </button>
            ))}
          </div>
        ))}
      </div>
    </div>
  );
}
