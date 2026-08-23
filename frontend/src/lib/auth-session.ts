const AUTH_SESSION_EVENT = "lanverse:auth-session-changed";

let accessToken: string | null = null;

export function getAccessToken(): string | null {
  return accessToken;
}

export function setAccessToken(token: string): void {
  accessToken = token;
  if (typeof window !== "undefined") {
    window.dispatchEvent(new Event(AUTH_SESSION_EVENT));
  }
}

export function clearAccessToken(): void {
  accessToken = null;
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
