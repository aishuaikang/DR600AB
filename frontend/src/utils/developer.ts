const SESSION_KEY = "dr600ab.developer.session";

export type DeveloperSession = {
  token: string;
  expiresAt: number;
};

export function readDeveloperSession(): DeveloperSession | null {
  if (typeof window === "undefined") {
    return null;
  }

  try {
    const raw = window.sessionStorage.getItem(SESSION_KEY);
    if (!raw) {
      return null;
    }
    const session = JSON.parse(raw) as DeveloperSession;
    if (!session.token || !session.expiresAt || session.expiresAt <= Date.now()) {
      window.sessionStorage.removeItem(SESSION_KEY);
      return null;
    }
    return session;
  } catch {
    return null;
  }
}

export function isDeveloperSessionActive() {
  return Boolean(readDeveloperSession());
}

export function getDeveloperSessionExpiresAt() {
  return readDeveloperSession()?.expiresAt ?? 0;
}

export function storeDeveloperSession(session: DeveloperSession) {
  if (typeof window !== "undefined") {
    window.sessionStorage.setItem(SESSION_KEY, JSON.stringify(session));
  }

  return session;
}

export function clearDeveloperSession() {
  if (typeof window === "undefined") {
    return;
  }
  window.sessionStorage.removeItem(SESSION_KEY);
}
