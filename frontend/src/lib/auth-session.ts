const ACCESS_TOKEN_KEY = "lanverse.access-token";

function getSessionStorage(): Storage | null {
  return typeof window === "undefined" ? null : window.sessionStorage;
}

export function getAccessToken(): string | null {
  return getSessionStorage()?.getItem(ACCESS_TOKEN_KEY) ?? null;
}

export function setAccessToken(token: string): void {
  getSessionStorage()?.setItem(ACCESS_TOKEN_KEY, token);
}

export function clearAccessToken(): void {
  getSessionStorage()?.removeItem(ACCESS_TOKEN_KEY);
}

export function hasAccessToken(): boolean {
  return getAccessToken() !== null;
}
