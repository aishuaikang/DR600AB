const SETTINGS_KEY = "dr600ab.settings";

export interface StoredSettings {
  receivePort: string;
  sendPort: string;
}

function readStorage(): StoredSettings {
  if (typeof window === "undefined") {
    return { receivePort: "", sendPort: "" };
  }

  try {
    const raw = window.localStorage.getItem(SETTINGS_KEY);
    if (!raw) {
      return { receivePort: "", sendPort: "" };
    }
    const parsed = JSON.parse(raw) as Partial<StoredSettings>;
    return {
      receivePort: typeof parsed.receivePort === "string" ? parsed.receivePort : "",
      sendPort: typeof parsed.sendPort === "string" ? parsed.sendPort : "",
    };
  } catch {
    return { receivePort: "", sendPort: "" };
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
