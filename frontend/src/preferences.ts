const SETTINGS_KEY = "dr600ab.settings";

export interface StoredSettings {
  appTitle: string;
  receivePort: string;
  sendPort: string;
}

const EMPTY_SETTINGS: StoredSettings = {
  appTitle: "",
  receivePort: "",
  sendPort: "",
};

function readStorage(): StoredSettings {
  if (typeof window === "undefined") {
    return EMPTY_SETTINGS;
  }

  try {
    const raw = window.localStorage.getItem(SETTINGS_KEY);
    if (!raw) {
      return EMPTY_SETTINGS;
    }
    const parsed = JSON.parse(raw) as Partial<StoredSettings>;
    return {
      appTitle: typeof parsed.appTitle === "string" ? parsed.appTitle : "",
      receivePort: typeof parsed.receivePort === "string" ? parsed.receivePort : "",
      sendPort: typeof parsed.sendPort === "string" ? parsed.sendPort : "",
    };
  } catch {
    return EMPTY_SETTINGS;
  }
}

export function getStoredSettings(): StoredSettings {
  return readStorage();
}

export function persistSettings(settings: StoredSettings) {
  if (typeof window === "undefined") {
    return;
  }

  try {
    window.localStorage.setItem(SETTINGS_KEY, JSON.stringify(settings));
  } catch {
    // 忽略本地存储错误。
  }
}
