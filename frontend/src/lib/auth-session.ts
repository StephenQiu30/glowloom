const ACCESS_TOKEN_KEY = "lanverse.access-token";
const AUTH_SESSION_EVENT = "lanverse:auth-session-changed";

function getSessionStorage(): Storage | null {
  return typeof window === "undefined" ? null : window.sessionStorage;
}

export function getAccessToken(): string | null {
  return getSessionStorage()?.getItem(ACCESS_TOKEN_KEY) ?? null;
}

export function setAccessToken(token: string): void {
  getSessionStorage()?.setItem(ACCESS_TOKEN_KEY, token);
  if (typeof window !== "undefined") {
    window.dispatchEvent(new Event(AUTH_SESSION_EVENT));
  }
}

export function clearAccessToken(): void {
  getSessionStorage()?.removeItem(ACCESS_TOKEN_KEY);
  if (typeof window !== "undefined") {
    window.dispatchEvent(new Event(AUTH_SESSION_EVENT));
  }
}

export function hasAccessToken(): boolean {
  return getAccessToken() !== null;
}

export function subscribeAuthSession(onStoreChange: () => void): () => void {
  if (typeof window === "undefined") return () => undefined;
  window.addEventListener(AUTH_SESSION_EVENT, onStoreChange);
  return () => window.removeEventListener(AUTH_SESSION_EVENT, onStoreChange);
}
